package federation

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
)

// ErrFederationNotEnabled is returned by CreateInvite when the target project
// has not been enabled for federation (US-1.1 AC3). Handlers map it to 400
// CodeFederationNotEnabled.
var ErrFederationNotEnabled = errors.New("federation: project not enabled for federation")

// ErrInvalidPermissions is returned when the requested invite permission grade
// is not one of read/write/admin. Handlers map it to 400.
var ErrInvalidPermissions = errors.New("federation: invalid permissions")

// ErrInviteNotFound is returned when an invite id does not exist (or does not
// belong to the given project). Handlers map it to 404. It is distinct from
// ErrProjectNotFound so the manage handlers can 404 a stray invite id without
// claiming the project is missing.
var ErrInviteNotFound = errors.New("federation: invite not found")

// ErrInviteExpiryInPast is returned when a caller-supplied expires_at is not in
// the future. Minting an already-expired invite only fails confusingly later at
// the Phase-2 handshake-consume step ("invite invalid") with no creation-time
// feedback, so it is rejected up front (US-1.2). Handlers map it to 400.
var ErrInviteExpiryInPast = errors.New("federation: invite expiry must be in the future")

const (
	// inviteSecretBytes is the byte length of the random invite secret. 32 bytes
	// = 256 bits of entropy (NFR-3.2).
	inviteSecretBytes = 32
	// defaultInviteTTL is the lifetime applied when the caller does not pin an
	// explicit expiry (US-1.2 AC4).
	defaultInviteTTL = 7 * 24 * time.Hour
	// defaultInviteMaxUses is the number of consumptions allowed when the caller
	// does not pin an explicit max (US-1.2 AC1, single-use by default).
	defaultInviteMaxUses = 1
)

// CreateInviteParams carries the owner's invite-creation choices. Permissions is
// required (read|write|admin). When MaxUses <= 0 it defaults to 1; when
// ExpiresAt is nil it defaults to now+7d.
type CreateInviteParams struct {
	Permissions model.FederationPermission
	MaxUses     int
	ExpiresAt   *time.Time
}

// CreateInviteResult is the one-time invite-creation output. Secret is the
// plaintext 256-bit secret, returned to the owner UI exactly once (it is never
// re-derivable from storage, which keeps only its SHA-256 hash). Link is the
// shareable join URL `<instanceURL>/federation/join#invite=<InviteID>.<Secret>`
// with the secret in the URL FRAGMENT (US-1.2 AC1, AC6) — built here from the
// service's authoritative instance URL so the secret framing has one source.
type CreateInviteResult struct {
	InviteID    string
	Secret      string
	Link        string
	Permissions model.FederationPermission
	MaxUses     int
	ExpiresAt   *time.Time
	CreatedAt   time.Time
}

// CreateInvite mints a per-project share invite (Federation v1 F1.2, US-1.2).
//
//   - The project must exist and be federated, else ErrProjectNotFound /
//     ErrFederationNotEnabled (US-1.1 AC3).
//   - The secret is 256 random bits (NFR-3.2); only its SHA-256 hash is stored
//     (US-1.2 AC2). The plaintext is returned to the caller once and never again.
//   - Defaults: max_uses=1, expires_at=now+7d (US-1.2 AC1, AC4).
//
// The plaintext secret never touches the DB and is never logged; it leaves the
// process only in the response body to the authenticated owner.
func (s *Service) CreateInvite(ctx context.Context, projectID int64, params CreateInviteParams) (*CreateInviteResult, error) {
	if !params.Permissions.IsValid() {
		return nil, ErrInvalidPermissions
	}

	p, err := s.projects.Get(ctx, projectID)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return nil, ErrProjectNotFound
		}
		return nil, fmt.Errorf("load project: %w", err)
	}
	if !p.IsFederated {
		return nil, ErrFederationNotEnabled
	}
	// Reject a caller-supplied past expiry before minting anything — a born-dead
	// invite would otherwise only surface as a confusing handshake failure later
	// (US-1.2). A nil expiry defaults to now+7d below and is always future.
	if params.ExpiresAt != nil && !params.ExpiresAt.After(time.Now()) {
		return nil, ErrInviteExpiryInPast
	}

	secret, err := generateInviteSecret()
	if err != nil {
		return nil, fmt.Errorf("generate invite secret: %w", err)
	}
	hash := sha256.Sum256([]byte(secret))

	maxUses := params.MaxUses
	if maxUses <= 0 {
		maxUses = defaultInviteMaxUses
	}
	now := time.Now()
	expiresAt := params.ExpiresAt
	if expiresAt == nil {
		exp := now.Add(defaultInviteTTL)
		expiresAt = &exp
	}

	inv := model.FederationInvite{
		InviteID:       model.NewClientID(),
		LocalProjectID: projectID,
		SecretHash:     hex.EncodeToString(hash[:]),
		Permissions:    params.Permissions,
		MaxUses:        maxUses,
		UsedCount:      0,
		ExpiresAt:      expiresAt,
		CreatedAt:      now,
	}
	if _, err := s.invites.Create(ctx, inv); err != nil {
		return nil, fmt.Errorf("create invite: %w", err)
	}

	return &CreateInviteResult{
		InviteID:    inv.InviteID,
		Secret:      secret,
		Link:        s.inviteLink(inv.InviteID, secret),
		Permissions: inv.Permissions,
		MaxUses:     inv.MaxUses,
		ExpiresAt:   inv.ExpiresAt,
		CreatedAt:   inv.CreatedAt,
	}, nil
}

// InviteView is one row of the invite list (Federation v1 F1.3, US-1.3). It
// carries only id + metadata + the derived lifecycle status — NEVER the secret
// or its hash (US-1.3 AC5), so the secret is visible to the owner exactly once
// at creation and never re-served by the list endpoint.
type InviteView struct {
	InviteID    string
	Permissions model.FederationPermission
	MaxUses     int
	UsedCount   int
	Status      model.InviteStatus
	ExpiresAt   *time.Time
	RevokedAt   *time.Time
	ConsumedAt  *time.Time
	CreatedAt   time.Time
}

// ListInvites returns every invite for a project with its derived lifecycle
// status (Federation v1 F1.3, US-1.3 AC1). The project must exist, else
// ErrProjectNotFound (→404); any other project-Get failure is wrapped so the
// handler returns 500 rather than masking infrastructure faults as 404. The
// secret is never reconstructable — the view omits the secret hash entirely
// (US-1.3 AC5). Status is computed via the single canonical helper
// model.FederationInvite.Status so the list and the consume path can never
// disagree.
func (s *Service) ListInvites(ctx context.Context, projectID int64) ([]InviteView, error) {
	if _, err := s.projects.Get(ctx, projectID); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return nil, ErrProjectNotFound
		}
		return nil, fmt.Errorf("load project: %w", err)
	}

	rows, err := s.invites.ListByProject(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("list invites: %w", err)
	}

	now := time.Now()
	out := make([]InviteView, 0, len(rows))
	for _, inv := range rows {
		out = append(out, InviteView{
			InviteID:    inv.InviteID,
			Permissions: inv.Permissions,
			MaxUses:     inv.MaxUses,
			UsedCount:   inv.UsedCount,
			Status:      inv.Status(now),
			ExpiresAt:   inv.ExpiresAt,
			RevokedAt:   inv.RevokedAt,
			ConsumedAt:  inv.ConsumedAt,
			CreatedAt:   inv.CreatedAt,
		})
	}
	return out, nil
}

// RevokeInvite stamps revoked_at on an invite, flipping its derived status to
// revoked (Federation v1 F1.3, US-1.3 AC2). It is idempotent — re-revoking keeps
// the original revoked_at. The invite must exist AND belong to projectID, else
// ErrInviteNotFound (→404), so one project's invite id cannot be revoked through
// another project's route.
func (s *Service) RevokeInvite(ctx context.Context, projectID int64, inviteID string) error {
	if err := s.ownInvite(ctx, projectID, inviteID); err != nil {
		return err
	}
	if err := s.invites.Revoke(ctx, inviteID, time.Now()); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return ErrInviteNotFound
		}
		return fmt.Errorf("revoke invite: %w", err)
	}
	return nil
}

// DeleteInvite hard-removes an invite row (Federation v1 F1.3, US-1.3 AC3). It
// does NOT touch federated_projects: a peer that already consumed the invite
// stays joined. The invite must exist AND belong to projectID, else
// ErrInviteNotFound (→404).
func (s *Service) DeleteInvite(ctx context.Context, projectID int64, inviteID string) error {
	if err := s.ownInvite(ctx, projectID, inviteID); err != nil {
		return err
	}
	if err := s.invites.Delete(ctx, inviteID); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return ErrInviteNotFound
		}
		return fmt.Errorf("delete invite: %w", err)
	}
	return nil
}

// ownInvite verifies the invite exists and belongs to projectID, returning
// ErrInviteNotFound otherwise. This is the ownership guard the revoke/delete
// paths share so a stray id never leaks across projects.
func (s *Service) ownInvite(ctx context.Context, projectID int64, inviteID string) error {
	inv, err := s.invites.Get(ctx, inviteID)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return ErrInviteNotFound
		}
		return fmt.Errorf("get invite: %w", err)
	}
	if inv.LocalProjectID != projectID {
		return ErrInviteNotFound
	}
	return nil
}

// inviteLink composes the shareable join URL carrying the secret in the URL
// fragment: `<instanceURL>/federation/join#invite=<id>.<secret>`. The fragment
// (everything after '#') is never sent to the server in the HTTP request line,
// so the secret cannot appear in access logs (US-1.2 AC6).
func (s *Service) inviteLink(inviteID, secret string) string {
	base := strings.TrimRight(s.instanceURL, "/")
	return fmt.Sprintf("%s/federation/join#invite=%s.%s", base, inviteID, secret)
}

// generateInviteSecret returns a hex-encoded 256-bit secret from crypto/rand.
// Hex (not base64url) keeps the join-link fragment URL-safe with no padding and
// no '.'/'#' collisions with the `<id>.<secret>` framing.
func generateInviteSecret() (string, error) {
	buf := make([]byte, inviteSecretBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
