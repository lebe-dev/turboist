package inbox

import (
	"context"
	"crypto/ed25519"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/lebe-dev/turboist/internal/federation/peerkeys"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
)

// Production wiring for the per-event payload validator (Federation v1 F3.2a).
//
// These adapters bind the abstract KeyResolver / MembershipLookup to the real
// peer-key cache and federation repos so the F3.2 POST /federation/events handler
// can construct a Validator in one line. They are kept out of validate.go so the
// validator core stays DB-free and unit-testable.

// NewDBValidator builds a Validator backed by the shared peer-key cache and the
// federated-project repo. peerKeys resolves the event author's Ed25519 key
// (fetch-once via .well-known); fedProjects + database resolve the (project, peer)
// membership row. now may be nil (defaults to time.Now).
func NewDBValidator(database *sql.DB, fedProjects *repo.FederatedProjectRepo, peerKeys *peerkeys.Cache, now func() time.Time) *Validator {
	return NewValidator(
		PeerKeyResolver(peerKeys),
		DBMembershipLookup(database, fedProjects),
		now,
	)
}

// PeerKeyResolver adapts a peerkeys.Cache to the validator's KeyResolver. The
// cache fetches a peer's published key once on a miss and never holds a DB
// connection across the fetch (R1).
func PeerKeyResolver(cache *peerkeys.Cache) KeyResolver {
	return func(ctx context.Context, instanceURL string) (ed25519.PublicKey, error) {
		rk, err := cache.Resolve(ctx, instanceURL)
		if err != nil {
			return nil, err
		}
		return rk.Key, nil
	}
}

// DBMembershipLookup adapts the federated-project repo to the validator's
// MembershipLookup. It resolves the event's project_client_id to the local int64
// project id, then fetches the (local project, peer) federation mapping. A
// missing project OR a missing peer row collapses to ErrNotMember so the handler
// returns a uniform 403 (a probing peer cannot distinguish "no such project" from
// "not a member of it").
func DBMembershipLookup(database *sql.DB, fedProjects *repo.FederatedProjectRepo) MembershipLookup {
	return func(ctx context.Context, projectClientID, peerURL string) (*model.FederatedProject, error) {
		localID, err := resolveLocalProjectID(ctx, database, projectClientID)
		if errors.Is(err, ErrNotMember) {
			return nil, ErrNotMember
		}
		if err != nil {
			return nil, err
		}
		fp, err := fedProjects.Get(ctx, localID, peerURL)
		if errors.Is(err, repo.ErrNotFound) {
			return nil, ErrNotMember
		}
		if err != nil {
			return nil, fmt.Errorf("membership get %d/%q: %w", localID, peerURL, err)
		}
		return fp, nil
	}
}

// resolveLocalProjectID maps a federated project's client_id to the local int64
// projects.id. A missing/tombstoned project is ErrNotMember (the sender cannot be
// a member of a project this instance does not hold).
func resolveLocalProjectID(ctx context.Context, database *sql.DB, projectClientID string) (int64, error) {
	var id int64
	err := database.QueryRowContext(ctx,
		`SELECT id FROM projects WHERE client_id = ? AND is_federated = 1 AND deleted_at IS NULL`,
		projectClientID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotMember
	}
	if err != nil {
		return 0, fmt.Errorf("resolve local project %q: %w", projectClientID, err)
	}
	return id, nil
}
