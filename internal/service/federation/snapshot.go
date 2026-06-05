package federation

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/url"
	"time"

	"github.com/lebe-dev/turboist/internal/crypto"
	"github.com/lebe-dev/turboist/internal/federation/protocol"
	"github.com/lebe-dev/turboist/internal/federation/snapshot"
	"github.com/lebe-dev/turboist/internal/federation/snapshottoken"
	"github.com/lebe-dev/turboist/internal/federation/transport"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
)

// ErrSnapshotTokenExpired is returned by BuildSnapshot when the presented
// snapshot token has passed its 15-minute TTL (US-2.3 AC4). The handler maps it
// to a 401 so the joiner re-handshakes for a fresh token.
var ErrSnapshotTokenExpired = errors.New("federation: snapshot token expired")

// ErrSnapshotTokenInvalid is returned for a malformed snapshot token, a token
// signed by a different key, or one bound to a different project id.
var ErrSnapshotTokenInvalid = errors.New("federation: snapshot token invalid")

// snapshotDeps are the owner-side collaborators the snapshot build path uses.
// They are wired additively (WithSnapshotDeps) so the existing constructors are
// untouched; nil deps mean the snapshot endpoint is not available on this build.
type snapshotDeps struct {
	tasks    *repo.TaskRepo
	sections *repo.ProjectSectionRepo
	contexts *repo.ContextRepo
	snapshot *repo.FederationSnapshotRepo
}

// WithSnapshotDeps wires the snapshot build/consume repos onto the service
// (Federation v1 F2.3). It returns the same *Service for chaining.
func (s *Service) WithSnapshotDeps(tasks *repo.TaskRepo, sections *repo.ProjectSectionRepo, contexts *repo.ContextRepo, snap *repo.FederationSnapshotRepo) *Service {
	s.snap = &snapshotDeps{tasks: tasks, sections: sections, contexts: contexts, snapshot: snap}
	return s
}

// BuildSnapshot verifies the 15-min snapshot token and returns the buffer-first,
// consistent-as-of snapshot of the project (Federation v1 F2.3, US-2.3). The
// token is verified under THIS instance's own public key (the owner minted it at
// handshake). An expired token → ErrSnapshotTokenExpired (401, US-2.3 AC4); a
// malformed token, a token for a different project, or a key mismatch →
// ErrSnapshotTokenInvalid. The read is buffer-first (it holds the lone writer
// connection only for the consistent read, never across streaming — §3 / R1).
func (s *Service) BuildSnapshot(ctx context.Context, projectID int64, token string, now time.Time) (*snapshot.Snapshot, error) {
	if s.cipher == nil {
		return nil, ErrKeyMissing
	}
	keys, err := s.keys.Get(ctx)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return nil, ErrKeyMissing
		}
		return nil, fmt.Errorf("load federation keys: %w", err)
	}
	_, pub, err := crypto.LoadInstanceKeypair(s.cipher, keys.PublicKey, keys.PrivateSeedEnc)
	if err != nil {
		return nil, fmt.Errorf("load instance keypair: %w", err)
	}

	tokenProjectID, err := snapshottoken.Verify(pub, token, now)
	if err != nil {
		if errors.Is(err, snapshottoken.ErrExpired) {
			return nil, ErrSnapshotTokenExpired
		}
		return nil, ErrSnapshotTokenInvalid
	}
	if tokenProjectID != projectID {
		return nil, ErrSnapshotTokenInvalid
	}

	snap, err := snapshot.Build(ctx, s.db, projectID, keys.NodeID)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return nil, ErrProjectNotFound
		}
		return nil, fmt.Errorf("build snapshot: %w", err)
	}
	return snap, nil
}

// BuildSnapshotForMember builds the buffer-first snapshot of a project for a
// caller the HTTP-signature middleware has ALREADY proven to be a trusted,
// non-revoked member (Federation v1 F4.2 re-bootstrap). It is the no-token
// sibling of BuildSnapshot: a joiner whose 15-min handshake token expired during
// a > retention offline cannot present a fresh token, so the owner re-snapshots
// to the verified member directly — the same trust the pull endpoint already
// extends (the pull endpoint serves the full retained event log to a signed
// member with no token; serving the snapshot to that member is consistent). The
// caller (handler) is responsible for the membership + non-revoked check; this
// method only builds. The read is buffer-first (R1).
func (s *Service) BuildSnapshotForMember(ctx context.Context, projectID int64) (*snapshot.Snapshot, error) {
	if s.cipher == nil {
		return nil, ErrKeyMissing
	}
	keys, err := s.keys.Get(ctx)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return nil, ErrKeyMissing
		}
		return nil, fmt.Errorf("load federation keys: %w", err)
	}
	snap, err := snapshot.Build(ctx, s.db, projectID, keys.NodeID)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return nil, ErrProjectNotFound
		}
		return nil, fmt.Errorf("build snapshot: %w", err)
	}
	return snap, nil
}

// bootstrapParams carry the handshake outcome the joiner needs to fetch + apply
// the owner's snapshot.
type bootstrapParams struct {
	ownerInstanceURL string
	snapshotURL      string
	snapshotToken    string
	remoteProjectID  string
	permissions      model.FederationPermission
	protocolVersion  int
}

// bootstrap fetches the owner's project snapshot using the handshake-issued
// token and applies it into a brand-new local federated project (Federation v1
// F2.3, US-2.3). It signs the snapshot GET with the SAME pinned transport
// signature as every other federation request (the token rides as a query
// param), reads the NDJSON body, and applies it in one local transaction through
// the snapshot consume path. Returns the local project id mapped to the owner.
func (s *Service) bootstrap(ctx context.Context, p bootstrapParams) (*snapshot.ApplyResult, error) {
	if s.snap == nil || s.snap.snapshot == nil {
		return nil, fmt.Errorf("federation: snapshot deps not configured")
	}
	if s.sender == nil {
		return nil, fmt.Errorf("federation: no handshake sender configured")
	}

	body, err := s.fetchSnapshot(ctx, p.snapshotURL, p.snapshotToken)
	if err != nil {
		return nil, err
	}

	res, err := snapshot.Apply(ctx, snapshot.ApplyDeps{
		DB:          s.db,
		Projects:    s.projects,
		Sections:    s.snap.sections,
		Tasks:       s.snap.tasks,
		Contexts:    s.snap.contexts,
		FedProjects: s.fedProjects,
		Snapshot:    s.snap.snapshot,
	}, snapshot.ApplyParams{
		OwnerInstanceURL: p.ownerInstanceURL,
		RemoteProjectID:  p.remoteProjectID,
		Permissions:      p.permissions,
		ProtocolVersion:  p.protocolVersion,
		Reader:           bytes.NewReader(body),
		Now:              s.now,
	})
	if err != nil {
		return nil, fmt.Errorf("apply snapshot: %w", err)
	}
	return res, nil
}

// fetchSnapshot signs and sends the snapshot GET to the owner, returning the raw
// NDJSON body. The request is transport-signed (empty body, so the digest is
// SHA256("")) and the 15-min token rides as the `token` query param.
func (s *Service) fetchSnapshot(ctx context.Context, snapshotURL, token string) ([]byte, error) {
	keys, err := s.keys.Ensure(ctx, s.cipher, defaultInstanceDisplayName(s.instanceURL))
	if err != nil {
		return nil, fmt.Errorf("ensure federation keys: %w", err)
	}
	priv, _, err := crypto.LoadInstanceKeypair(s.cipher, keys.PublicKey, keys.PrivateSeedEnc)
	if err != nil {
		return nil, fmt.Errorf("load instance keypair: %w", err)
	}

	full, path, err := snapshotRequestURL(snapshotURL, token)
	if err != nil {
		return nil, err
	}

	nonce, err := newNonce()
	if err != nil {
		return nil, fmt.Errorf("generate nonce: %w", err)
	}
	ts := model.FormatUTC(s.now())
	ver := protocol.FormatVersion(defaultProtocolVersion())
	digest := transport.BodyDigest(nil)
	sigParams := transport.SignatureParams{
		Method:          "GET",
		Path:            path,
		InstanceURL:     s.instanceURL,
		Timestamp:       ts,
		Nonce:           nonce,
		ProtocolVersion: ver,
		BodyDigest:      digest,
	}
	headers := map[string]string{
		transport.HeaderInstance:    s.instanceURL,
		transport.HeaderTimestamp:   ts,
		transport.HeaderNonce:       nonce,
		transport.HeaderProtocolVer: ver,
		transport.HeaderDigest:      digest,
		transport.HeaderSignature:   transport.SignB64(priv, sigParams),
	}

	resp, err := s.sender.Send(ctx, SignedRequest{
		Method:  "GET",
		URL:     full,
		Path:    path,
		Headers: headers,
	})
	if err != nil {
		return nil, fmt.Errorf("send snapshot request: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &RemoteHandshakeError{StatusCode: resp.StatusCode, Code: errorCodeOf(resp.Body)}
	}
	return resp.Body, nil
}

// snapshotRequestURL appends the token query param to the snapshot URL and
// returns the full URL plus the concrete request path (what the signature
// covers — the path WITHOUT the query, per the pinned canonical string, R4). An
// EMPTY token (the F4.2 re-bootstrap fetch, whose 15-min handshake token expired
// long ago) is intentionally NOT appended: the owner serves a re-snapshot to a
// signature-verified, non-revoked member without a token.
func snapshotRequestURL(snapshotURL, token string) (full, path string, err error) {
	u, err := url.Parse(snapshotURL)
	if err != nil {
		return "", "", fmt.Errorf("parse snapshot url: %w", err)
	}
	if token != "" {
		q := u.Query()
		q.Set("token", token)
		u.RawQuery = q.Encode()
	}
	return u.String(), u.Path, nil
}
