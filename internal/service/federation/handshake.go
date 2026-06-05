package federation

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/lebe-dev/turboist/internal/crypto"
	"github.com/lebe-dev/turboist/internal/db"
	"github.com/lebe-dev/turboist/internal/federation/handshake"
	"github.com/lebe-dev/turboist/internal/federation/protocol"
	"github.com/lebe-dev/turboist/internal/federation/snapshottoken"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
)

// ErrHandshakeInvalid is returned by Handshake for any invite-validation failure
// the owner must NOT disambiguate to the caller: unknown invite id, wrong secret,
// expired/revoked/fully-consumed invite (US-2.2 AC4 — a generic 401 so a probe
// cannot tell "bad id" from "bad secret"). Handlers map it to a generic 401.
var ErrHandshakeInvalid = errors.New("federation: handshake invite invalid")

// ErrHandshakeKeyMismatch is returned when the joining instance_url is already
// known with a DIFFERENT public key than the one presented now (US-2.2 AC5). The
// owner refuses to silently overwrite a pinned peer key — this is a 409 + WARN,
// not a trust upgrade. Re-presenting the SAME key is idempotent and allowed.
var ErrHandshakeKeyMismatch = errors.New("federation: peer key mismatch")

// ErrVersionUnsupported wraps the protocol negotiation no-overlap result so the
// owner can reject BEFORE consuming the invite (US-9.1 AC2 / R23 atomicity).
var ErrVersionUnsupported = protocol.ErrNoVersionOverlap

// HandshakeInput is the verified handshake the owner acts on. Body is the decoded
// request body; VerifiedPeerURL / VerifiedPeerKey are the instance_url and
// Ed25519 public key the transport signature middleware already proved (so the
// owner enforces body.JoinerPublicKey == the signed key, defense-in-depth).
type HandshakeInput struct {
	Body            handshake.Request
	VerifiedPeerURL string
	VerifiedPeerKey string
}

// Handshake validates an invite presented by a joining peer and, on success,
// atomically records the federation relationship (Federation v1 F2.2, US-2.2).
//
// Validation order (all rejecting BEFORE any row/invite mutation — R23):
//  1. body joiner_public_key must equal the transport-verified key, and the
//     verified peer URL must equal the body joiner_instance_url (defense-in-depth
//     against a header/body split) → ErrHandshakeInvalid;
//  2. protocol.Negotiate first — no common version → ErrVersionUnsupported (400,
//     nothing consumed, US-9.1 AC2);
//  3. invite exists, belongs to a federated project, is consumable (active —
//     not expired/revoked/fully-used), and SHA-256(secret) constant-time-matches
//     the stored hash → otherwise ErrHandshakeInvalid (generic 401, US-2.2 AC4);
//  4. if the peer instance_url is already known with a different key →
//     ErrHandshakeKeyMismatch (409 + WARN, US-2.2 AC5); same key is idempotent.
//
// Then, in ONE transaction (US-2.2 AC3): bump used_count (+ consumed_at when the
// invite is now fully used), upsert federated_instances (persist the joiner
// display_name — R24), and insert the federated_projects peer mapping. The
// response carries the owner identity, the negotiated version, and a 15-min
// snapshot token (consumed in F2.3).
func (s *Service) Handshake(ctx context.Context, in HandshakeInput, now time.Time) (*handshake.Response, error) {
	if s.cipher == nil {
		return nil, ErrKeyMissing
	}

	// (1) The body key must match the signed transport key, and the body's
	// claimed instance_url must match the verified one. A mismatch is treated as
	// a generic invalid handshake (no disclosure of which half is wrong).
	if subtle.ConstantTimeCompare([]byte(in.Body.JoinerPublicKey), []byte(in.VerifiedPeerKey)) != 1 {
		return nil, ErrHandshakeInvalid
	}
	if in.Body.JoinerInstanceURL != in.VerifiedPeerURL {
		return nil, ErrHandshakeInvalid
	}

	// Load the owner's identity up front — needed for negotiation and the
	// response. Without a keypair the instance is not federation-capable.
	keys, err := s.keys.Get(ctx)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return nil, ErrKeyMissing
		}
		return nil, fmt.Errorf("load federation keys: %w", err)
	}

	// (2) Negotiate the protocol version BEFORE consuming anything (US-9.1 AC2).
	negotiated, err := protocol.Negotiate(protocol.SupportedProtocolVersions, in.Body.ProtocolVersions)
	if err != nil {
		return nil, ErrVersionUnsupported
	}

	// (3) Load + validate the invite. Every failure collapses to
	// ErrHandshakeInvalid so the caller cannot distinguish id-vs-secret (AC4).
	inv, err := s.invites.Get(ctx, in.Body.InviteID)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return nil, ErrHandshakeInvalid
		}
		return nil, fmt.Errorf("load invite: %w", err)
	}
	if !inv.IsConsumable(now) {
		return nil, ErrHandshakeInvalid
	}
	wantHash := sha256.Sum256([]byte(in.Body.Secret))
	if subtle.ConstantTimeCompare([]byte(hex.EncodeToString(wantHash[:])), []byte(inv.SecretHash)) != 1 {
		return nil, ErrHandshakeInvalid
	}

	// The invite must point at a still-federated project.
	project, err := s.projects.Get(ctx, inv.LocalProjectID)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return nil, ErrHandshakeInvalid
		}
		return nil, fmt.Errorf("load project: %w", err)
	}
	if !project.IsFederated {
		return nil, ErrFederationNotEnabled
	}

	// (4) Key-pinning: a known peer presenting a NEW key is a 409 (US-2.2 AC5),
	// never a silent key rotation. An unknown peer (ErrNotFound) is fine.
	existing, err := s.fedInstances.Get(ctx, in.Body.JoinerInstanceURL)
	if err != nil && !errors.Is(err, repo.ErrNotFound) {
		return nil, fmt.Errorf("load peer instance: %w", err)
	}
	if existing != nil && existing.PublicKey != in.Body.JoinerPublicKey {
		return nil, ErrHandshakeKeyMismatch
	}

	// Commit: consume the invite + record the peer, atomically (US-2.2 AC3).
	//
	// The consumability check at (3) above ran on a NON-transactional read, so it
	// cannot by itself uphold the single-use invariant: two concurrent handshakes
	// presenting the same leaked single-use secret from DISTINCT joiner URLs can
	// both read used_count=0 and both pass IsConsumable before either consumes
	// (TOCTOU). We close that window inside the tx — re-load the invite with GetTx
	// and re-run IsConsumable under the SAME transaction, and rely on the
	// self-guarding ConsumeTx (UPDATE ... WHERE used_count < max_uses AND active)
	// as the authoritative serialization point. The race loser sees either a
	// no-longer-consumable invite here or a zero-row ConsumeTx, both of which
	// collapse to the generic ErrHandshakeInvalid (a 401, no disclosure — AC4).
	err = db.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		txInv, err := s.invites.GetTx(ctx, tx, inv.InviteID)
		if err != nil {
			if errors.Is(err, repo.ErrNotFound) {
				return ErrHandshakeInvalid
			}
			return fmt.Errorf("reload invite: %w", err)
		}
		if !txInv.IsConsumable(now) {
			return ErrHandshakeInvalid
		}
		if err := s.invites.ConsumeTx(ctx, tx, inv.InviteID, now); err != nil {
			if errors.Is(err, repo.ErrNotFound) || errors.Is(err, repo.ErrInviteNotConsumable) {
				return ErrHandshakeInvalid
			}
			return fmt.Errorf("consume invite: %w", err)
		}
		if err := s.fedInstances.UpsertTx(ctx, tx, model.FederatedInstance{
			InstanceURL:   in.Body.JoinerInstanceURL,
			PublicKey:     in.Body.JoinerPublicKey,
			DisplayName:   in.Body.JoinerDisplayName,
			LastContactAt: &now,
			CreatedAt:     now,
			UpdatedAt:     now,
		}); err != nil {
			return fmt.Errorf("upsert peer instance: %w", err)
		}
		return s.fedProjects.UpsertPeerRowTx(ctx, tx, model.FederatedProject{
			LocalProjectID:    inv.LocalProjectID,
			PeerInstanceURL:   in.Body.JoinerInstanceURL,
			RemoteProjectID:   "",
			IsOwner:           false,
			OriginInstanceURL: s.instanceURL,
			Permissions:       inv.Permissions,
			ProtocolVersion:   negotiated,
			JoinedAt:          now,
		})
	})
	if err != nil {
		return nil, err
	}

	// Audit the accepted handshake (Federation v1 F6.3, US-7.4 AC1): a new trust
	// relationship was established. Recorded with the handshake's own clock so the
	// timestamp matches the consume. The detail carries no secret (the invite secret
	// is never logged).
	if s.auditor != nil {
		s.auditor.Record(repo.AuditEntry{
			Kind:            repo.AuditKindHandshake,
			Outcome:         repo.AuditOutcomeAccepted,
			PeerInstanceURL: in.Body.JoinerInstanceURL,
			Detail:          "handshake accepted",
			CreatedAt:       now,
		})
	}

	priv, _, err := crypto.LoadInstanceKeypair(s.cipher, keys.PublicKey, keys.PrivateSeedEnc)
	if err != nil {
		return nil, fmt.Errorf("load instance private key: %w", err)
	}
	token, err := snapshottoken.Mint(priv, inv.LocalProjectID, now)
	if err != nil {
		return nil, fmt.Errorf("mint snapshot token: %w", err)
	}

	return &handshake.Response{
		ProjectID:          inv.LocalProjectID,
		ProjectName:        project.Title,
		OwnerPublicKey:     keys.PublicKey,
		OwnerDisplayName:   keys.DisplayName,
		SnapshotURL:        s.snapshotURL(inv.LocalProjectID),
		SnapshotToken:      token,
		PermissionsGranted: string(inv.Permissions),
		ProtocolVersion:    negotiated,
	}, nil
}

// snapshotURL composes the owner-side snapshot endpoint URL for a project. The
// concrete endpoint is served in F2.3; F2.2 only advertises it so the joiner can
// store it alongside the token.
func (s *Service) snapshotURL(projectID int64) string {
	return fmt.Sprintf("%s/federation/projects/%d/snapshot", trimSlash(s.instanceURL), projectID)
}
