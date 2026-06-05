package federation

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/lebe-dev/turboist/internal/crypto"
	"github.com/lebe-dev/turboist/internal/federation/handshake"
	"github.com/lebe-dev/turboist/internal/federation/peerkeys"
	"github.com/lebe-dev/turboist/internal/federation/protocol"
	"github.com/lebe-dev/turboist/internal/federation/transport"
	"github.com/lebe-dev/turboist/internal/model"
)

// ErrOwnerUntrusted is returned by Join when the owner's public key returned in
// the handshake response does NOT match the key independently published at the
// owner's .well-known (US-2.2 AC2). The joiner refuses to trust a key it cannot
// corroborate out-of-band. Handlers map it to CodeFederationUntrusted (403).
var ErrOwnerUntrusted = errors.New("federation: owner key not corroborated by .well-known")

// ErrOwnerInstanceMissing is returned when the join request carries no owner
// instance URL, so the joiner does not know where to send the handshake.
var ErrOwnerInstanceMissing = errors.New("federation: owner instance url required")

// SignedRequest is one outbound, transport-signed federation request the joiner
// emits. The Body is already canonicalized JSON; the headers carry the pinned
// signature set. A HandshakeSender delivers it to the owner and returns the raw
// response body + status.
type SignedRequest struct {
	Method  string
	URL     string
	Path    string
	Headers map[string]string
	Body    []byte
}

// SignedResponse is the owner's reply to a SignedRequest. Headers carries the
// subset of response headers the caller cares about (e.g. Retry-After on a 429,
// Federation v1 F4.4) — it may be nil for callers that ignore headers.
type SignedResponse struct {
	StatusCode int
	Body       []byte
	Headers    map[string]string
}

// HandshakeSender delivers an outbound signed handshake to the owner instance.
// It is injectable so tests can route A→B in-process through the owner's
// app.Test() without real network I/O; production wires an HTTP client. It must
// NOT hold any DB connection (R1).
type HandshakeSender interface {
	Send(ctx context.Context, req SignedRequest) (*SignedResponse, error)
}

// JoinResult is the joiner-side outcome of accepting an invite (Federation v1
// F2.2, US-2.2). It mirrors the parts of the owner handshake response the joiner
// UI needs; the snapshot URL + token are persisted for the F2.3 bootstrap.
type JoinResult struct {
	ProjectID        int64
	ProjectName      string
	Permissions      model.FederationPermission
	ProtocolVersion  int
	OwnerInstanceURL string
	OwnerDisplayName string
	SnapshotURL      string
	SnapshotToken    string
	// LocalReceivedHLC is the snapshot's as_of_hlc, persisted as the joiner's
	// last_received_hlc cursor by the F2.3 bootstrap. Empty when the snapshot
	// bootstrap did not run (F2.2-only build).
	LocalReceivedHLC string
}

// Join performs the joiner side of the handshake (Federation v1 F2.2, US-2.2).
//
//	ownerInstanceURL — where to send the handshake (the owner instance);
//	invite           — the parsed (id, secret) the joiner holds;
//	displayName      — this instance's human-readable name to advertise.
//
// Steps:
//  1. ensure this instance has a keypair; sign the handshake body with the
//     pinned transport signature (US-2.2 AC1 — the SAME scheme as every signed
//     request, version bound as line 6);
//  2. send it to the owner via the injected sender; map a non-2xx owner reply to
//     a typed error the handler can surface;
//  3. independently fetch the owner's .well-known and require the returned
//     owner_public_key to match it (US-2.2 AC2) — refuse otherwise;
//  4. persist the owner identity (key + display_name) in federated_instances and
//     the federated_projects mapping (joiner side, is_owner=0), warm the peer-key
//     cache (US-2.2 AC6), and return the snapshot token for the F2.3 bootstrap.
func (s *Service) Join(ctx context.Context, ownerInstanceURL string, invite ParsedInvite, displayName string) (*JoinResult, error) {
	if s.cipher == nil {
		return nil, ErrKeyMissing
	}
	if s.sender == nil {
		return nil, fmt.Errorf("federation: no handshake sender configured")
	}
	owner := trimSlash(ownerInstanceURL)
	if owner == "" {
		return nil, ErrOwnerInstanceMissing
	}

	keys, err := s.keys.Ensure(ctx, s.cipher, defaultInstanceDisplayName(s.instanceURL))
	if err != nil {
		return nil, fmt.Errorf("ensure federation keys: %w", err)
	}
	priv, _, err := crypto.LoadInstanceKeypair(s.cipher, keys.PublicKey, keys.PrivateSeedEnc)
	if err != nil {
		return nil, fmt.Errorf("load instance keypair: %w", err)
	}

	joinerName := displayName
	if joinerName == "" {
		joinerName = keys.DisplayName
	}
	body := handshake.Request{
		InviteID:          invite.InviteID,
		Secret:            invite.Secret,
		JoinerInstanceURL: s.instanceURL,
		JoinerPublicKey:   keys.PublicKey,
		JoinerDisplayName: joinerName,
		ProtocolVersions:  protocol.SupportedProtocolVersions,
	}
	bodyBytes, err := crypto.CanonicalJSON(body)
	if err != nil {
		return nil, fmt.Errorf("canonical handshake body: %w", err)
	}

	nonce, err := newNonce()
	if err != nil {
		return nil, fmt.Errorf("generate nonce: %w", err)
	}
	ts := model.FormatUTC(s.now())
	ver := protocol.FormatVersion(defaultProtocolVersion())
	digest := transport.BodyDigest(bodyBytes)
	sigParams := transport.SignatureParams{
		Method:          "POST",
		Path:            handshake.Path,
		InstanceURL:     s.instanceURL,
		Timestamp:       ts,
		Nonce:           nonce,
		ProtocolVersion: ver,
		BodyDigest:      digest,
	}
	headers := map[string]string{
		"Content-Type":              "application/json",
		transport.HeaderInstance:    s.instanceURL,
		transport.HeaderTimestamp:   ts,
		transport.HeaderNonce:       nonce,
		transport.HeaderProtocolVer: ver,
		transport.HeaderDigest:      digest,
		transport.HeaderSignature:   transport.SignB64(priv, sigParams),
	}

	resp, err := s.sender.Send(ctx, SignedRequest{
		Method:  "POST",
		URL:     owner + handshake.Path,
		Path:    handshake.Path,
		Headers: headers,
		Body:    bodyBytes,
	})
	if err != nil {
		return nil, fmt.Errorf("send handshake: %w", err)
	}
	hr, err := decodeHandshakeResponse(resp)
	if err != nil {
		return nil, err
	}

	// (3) Independently corroborate the owner key against its public .well-known
	// before trusting it (US-2.2 AC2). The fetch performs no DB access (R1).
	published, err := s.peerFetch(ctx, owner)
	if err != nil {
		return nil, fmt.Errorf("fetch owner well-known: %w", err)
	}
	if published.PublicKey != hr.OwnerPublicKey {
		return nil, ErrOwnerUntrusted
	}

	// (4) Persist the owner identity + warm the peer-key cache so the first inbound
	// owner event verifies without a second fetch (US-2.2 AC6). The
	// federated_projects mapping is NOT written here — the F2.3 snapshot bootstrap
	// (5) creates the local project (the FK target) and the mapping together.
	now := s.now()
	perm := model.FederationPermission(hr.PermissionsGranted)
	if err := s.persistJoin(ctx, owner, hr, perm, now); err != nil {
		return nil, err
	}
	if s.peerKeys != nil {
		if err := s.peerKeys.Put(owner, hr.OwnerPublicKey, hr.OwnerDisplayName); err != nil {
			return nil, fmt.Errorf("warm owner key cache: %w", err)
		}
	}

	// (5) Snapshot bootstrap (Federation v1 F2.3, US-2.3): fetch the owner's
	// project snapshot with the 15-min token and apply it into a brand-new local
	// federated project (one transaction, no resume). The returned local int64 id
	// is what the joiner UI navigates to. When snapshot deps are not wired (older
	// build), Join degrades to the F2.2 behaviour and returns the owner's project
	// id; F2.3 always wires them.
	result := &JoinResult{
		ProjectID:        hr.ProjectID,
		ProjectName:      hr.ProjectName,
		Permissions:      perm,
		ProtocolVersion:  hr.ProtocolVersion,
		OwnerInstanceURL: owner,
		OwnerDisplayName: hr.OwnerDisplayName,
		SnapshotURL:      hr.SnapshotURL,
		SnapshotToken:    hr.SnapshotToken,
	}
	if s.snap != nil {
		applied, err := s.bootstrap(ctx, bootstrapParams{
			ownerInstanceURL: owner,
			snapshotURL:      hr.SnapshotURL,
			snapshotToken:    hr.SnapshotToken,
			remoteProjectID:  fmt.Sprintf("%d", hr.ProjectID),
			permissions:      perm,
			protocolVersion:  hr.ProtocolVersion,
		})
		if err != nil {
			return nil, err
		}
		result.ProjectID = applied.LocalProjectID
		result.LocalReceivedHLC = applied.AsOfHLC
	}
	return result, nil
}

// Preview fetches the read-only owner identity behind an invite without
// consuming it (Federation v1 F2.1 preview backing, US-2.1 AC3). It resolves the
// owner's public .well-known server-side so the secret never travels
// browser→owner. The project name is not known until the handshake consumes the
// invite, so the preview surfaces the owner identity + the grade is left to the
// handshake; callers render "owner_display_name @ owner_instance".
type Preview struct {
	OwnerInstanceURL string
	OwnerDisplayName string
	ProtocolVersion  int
}

// Preview resolves the owner identity for the join UI (US-2.1 AC3). It performs
// the owner .well-known fetch server-side (no browser→owner CORS path) and does
// NOT consume the invite — that only happens on Accept (Join).
func (s *Service) Preview(ctx context.Context, ownerInstanceURL string) (*Preview, error) {
	owner := trimSlash(ownerInstanceURL)
	if owner == "" {
		return nil, ErrOwnerInstanceMissing
	}
	published, err := s.peerFetch(ctx, owner)
	if err != nil {
		return nil, fmt.Errorf("fetch owner well-known: %w", err)
	}
	// The .well-known fetch surfaces the owner identity; the binding protocol
	// version is negotiated for real at handshake (Accept). The preview advertises
	// this instance's highest version so the UI shows a coherent grade.
	return &Preview{
		OwnerInstanceURL: owner,
		OwnerDisplayName: published.DisplayName,
		ProtocolVersion:  defaultProtocolVersion(),
	}, nil
}

// persistJoin records the owner instance trust row on the joiner (key +
// display_name, US-2.2 AC2). It does NOT yet write a federated_projects mapping:
// the joiner's local project does not exist until the F2.3 snapshot bootstrap
// creates it (the federated_projects.local_project_id FK requires a real local
// project). F2.2 establishes the trust + returns the snapshot token/URL the F2.3
// stepper consumes to create the local project and the mapping.
func (s *Service) persistJoin(ctx context.Context, owner string, hr *handshake.Response, _ model.FederationPermission, now time.Time) error {
	if err := s.fedInstances.Upsert(ctx, model.FederatedInstance{
		InstanceURL:   owner,
		PublicKey:     hr.OwnerPublicKey,
		DisplayName:   hr.OwnerDisplayName,
		LastContactAt: &now,
		CreatedAt:     now,
		UpdatedAt:     now,
	}); err != nil {
		return fmt.Errorf("persist owner instance: %w", err)
	}
	return nil
}

// peerFetch resolves a peer's published .well-known document. It uses the
// configured fetcher (injectable for tests).
func (s *Service) peerFetch(ctx context.Context, instanceURL string) (*peerkeys.Instance, error) {
	if s.fetch == nil {
		return nil, fmt.Errorf("federation: no well-known fetcher configured")
	}
	return s.fetch(ctx, instanceURL)
}

// decodeHandshakeResponse maps the owner reply to the typed handshake response or
// a typed error. A non-2xx status is translated by the federation error code in
// the body so the joiner handler can re-surface 401/409/400(version)/410.
func decodeHandshakeResponse(resp *SignedResponse) (*handshake.Response, error) {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		var hr handshake.Response
		if err := jsonUnmarshal(resp.Body, &hr); err != nil {
			return nil, fmt.Errorf("decode handshake response: %w", err)
		}
		return &hr, nil
	}
	return nil, &RemoteHandshakeError{StatusCode: resp.StatusCode, Code: errorCodeOf(resp.Body)}
}

// newNonce returns a fresh 128-bit hex anti-replay nonce.
func newNonce() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// ParsedInvite is the (id, secret) pair a joiner holds (mirrors the frontend
// parse; the secret never persists on the joiner).
type ParsedInvite struct {
	InviteID string
	Secret   string
}
