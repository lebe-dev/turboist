package federation

import (
	"context"
	"crypto/ed25519"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/lebe-dev/turboist/internal/crypto"
	"github.com/lebe-dev/turboist/internal/db"
	"github.com/lebe-dev/turboist/internal/federation/events"
	"github.com/lebe-dev/turboist/internal/federation/hlc"
	"github.com/lebe-dev/turboist/internal/federation/protocol"
	"github.com/lebe-dev/turboist/internal/federation/store"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
)

// localEmitProtocolVersion is the protocol version the originating instance
// serialises its OWN outbound events at (Federation v1 F6.1 encoder seam). It is
// the highest version this build speaks; in v1 the only value is 1 and
// protocol.Encode is the identity transform. Routing emit serialisation through
// the seam (rather than calling events.Marshal directly) is what makes a future
// dual-write a localised change here instead of a scatter across emit sites.
func localEmitProtocolVersion() int {
	max := 0
	for _, v := range protocol.SupportedProtocolVersions {
		if v > max {
			max = v
		}
	}
	if max == 0 {
		return 1
	}
	return max
}

// Emitter wraps a domain mutation on a federated entity so that, in ONE
// transaction (NFR-2 crash-safety), it: (a) runs the domain write through the
// caller's closure (invariants run in the service layer that built the closure),
// (b) bumps the per-field entity_field_hlc, and (c) writes a canonical, signed
// event to federation_outbox (Federation v1 F3.1, US-3.2 AC1).
//
// Federation is a SCOPED OVERLAY: a mutation on a non-federated project runs the
// domain write only — no HLC bump, no outbox event (US-3.2 AC1). This keeps the
// single-user hot path at zero federation overhead (§3).
//
// physical_ms in the minted HLC comes from the SAME time.Now() the domain write
// stamps updated_at with, so the wall clock and the HLC never drift (§3 / R11).
type Emitter struct {
	db          *sql.DB
	keys        *repo.FederationKeysRepo
	cipher      *crypto.TokenCipher
	clock       *hlc.Store
	store       *store.Store
	instanceURL string

	// onCommit is fired (best-effort, non-blocking) after a federated event is
	// committed to the outbox so the publisher worker drains immediately rather
	// than on its next tick — the source of the NFR-1.1 push <5s budget. nil when
	// no worker is wired (tests / federation-off).
	onCommit func()
}

// NewEmitter constructs the transactional emitter. cipher is the
// FEDERATION_KEY-derived TokenCipher used to load the instance signing key; clock
// is the HLC store (advances hlc_state); instanceURL is this instance's
// federation identity (author/origin of locally-originated events).
func NewEmitter(database *sql.DB, keys *repo.FederationKeysRepo, cipher *crypto.TokenCipher, clock *hlc.Store, instanceURL string) *Emitter {
	return &Emitter{
		db:          database,
		keys:        keys,
		cipher:      cipher,
		clock:       clock,
		store:       store.New(database),
		instanceURL: instanceURL,
	}
}

// WithCommitPing wires a commit-ping callback (the publisher worker's Ping) so a
// freshly emitted federated event is pushed immediately. Returns the emitter for
// chaining. ping must be non-blocking (Worker.Ping is).
func (e *Emitter) WithCommitPing(ping func()) *Emitter {
	e.onCommit = ping
	return e
}

// MutationSpec describes the federated entity a mutation touches and the field
// values it changed. EntityID is the entity's cross-instance client_id. Fields
// maps federated field name → its NEW value; only the federated field set is
// carried (turboist-local fields are excluded, §3). Op is create/update/delete.
type MutationSpec struct {
	LocalProjectID int64
	EntityType     events.EntityType
	EntityID       string
	Op             events.Op
	Fields         map[string]any
}

// EmitMutation runs write inside one transaction and, when the project is
// federated, stamps the per-field HLC and appends a signed outbox event.
//
// write performs the actual domain write (it receives the same *sql.Tx so the
// whole thing is atomic). It MUST be the real service/repo write so the domain
// invariants run; EmitMutation only adds the federation sidecar around it.
func (e *Emitter) EmitMutation(ctx context.Context, spec MutationSpec, write func(tx *sql.Tx) error) error {
	return e.EmitMutations(ctx, []MutationSpec{spec}, write)
}

// EmitMutations runs write inside one transaction and, when the project is
// federated, stamps the per-field HLC and appends a signed outbox event for EACH
// spec — all under ONE minted HLC moment (NFR-2 crash-safety). It is the
// multi-entity generalisation of EmitMutation: a single domain write closure may
// touch several federated entities at once (e.g. a recurring complete that
// advances the parent task IN PLACE and CREATES a new occurrence snapshot, so the
// parent's op=update and the snapshot's op=create must be emitted together,
// TASK A). All specs MUST share the same LocalProjectID (the federation gate and
// the outbox row are keyed on it); the first spec's project decides federation.
//
// A non-federated project runs the domain write only — no HLC bump, no outbox
// events (US-3.2 AC1). An empty specs slice is a programming error.
func (e *Emitter) EmitMutations(ctx context.Context, specs []MutationSpec, write func(tx *sql.Tx) error) error {
	if len(specs) == 0 {
		return fmt.Errorf("emit: no mutation specs")
	}
	federated, projectClientID, err := e.projectFederation(ctx, specs[0].LocalProjectID)
	if err != nil {
		return err
	}

	if !federated {
		// Non-federated: domain write only, no federation sidecar (US-3.2 AC1).
		return db.WithTx(ctx, e.db, write)
	}

	// Mint ONE HLC for this mutation BEFORE opening the emit tx. hlc.Store.Now
	// advances hlc_state in its own short tx; on SetMaxOpenConns(1) it must not be
	// called while the emit tx holds the connection. The minted HLC stamps every
	// field this mutation touched (one local event = one HLC moment). If the emit
	// tx later rolls back, the advanced clock is harmless — an HLC only ever moves
	// forward and an unused value is never reused.
	stamp, err := e.clock.Now(ctx)
	if err != nil {
		return fmt.Errorf("emit mint hlc: %w", err)
	}
	hlcStr := stamp.String()

	priv, err := e.loadPrivateKey(ctx)
	if err != nil {
		return err
	}

	// Build + sign every event up front (no DB access) so the emit tx only writes.
	type signedSpec struct {
		spec    MutationSpec
		signed  events.Event
		payload string
	}
	prepared := make([]signedSpec, 0, len(specs))
	for _, spec := range specs {
		evt := e.buildEvent(spec, projectClientID, hlcStr)
		signed, serr := events.Sign(evt, priv)
		if serr != nil {
			return fmt.Errorf("emit sign event: %w", serr)
		}
		payload, merr := protocol.Encode(signed, localEmitProtocolVersion())
		if merr != nil {
			return fmt.Errorf("emit encode event: %w", merr)
		}
		prepared = append(prepared, signedSpec{spec: spec, signed: signed, payload: string(payload)})
	}

	nowStr := model.FormatUTC(time.Now())
	if err := db.WithTx(ctx, e.db, func(tx *sql.Tx) error {
		if err := write(tx); err != nil {
			return err
		}
		for _, p := range prepared {
			// Bump the per-field HLC for every field this mutation touched (and the
			// synthetic _deleted field for a delete) so future inbound events resolve
			// per-field LWW against the value we just wrote.
			for name := range p.signed.Fields {
				if _, err := e.store.CASFieldHLCTx(ctx, tx, string(p.spec.EntityType), p.spec.EntityID, name, hlcStr); err != nil {
					return err
				}
			}
			if err := e.store.InsertOutboxTx(ctx, tx, p.signed.EventID, p.spec.LocalProjectID, p.payload, localEmitProtocolVersion(), nowStr); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return err
	}

	// The outbox row is durably committed — wake the publisher so push is
	// immediate, not tick-delayed (NFR-1.1). Best-effort + non-blocking: a lost
	// ping only delays delivery to the next tick (the outbox is the source of
	// truth, never the ping).
	if e.onCommit != nil {
		e.onCommit()
	}
	return nil
}

// ChildTombstone identifies one child entity (a comment or checklist item) whose
// tombstone must be cascade-emitted when its parent task is deleted (§8.4, US-3.7
// AC3). EntityID is the child's cross-instance client_id.
type ChildTombstone struct {
	EntityType events.EntityType
	EntityID   string
}

// EmitDeleteCascade deletes a federated task and, in the SAME transaction, emits
// an op=delete event for the task PLUS one op=delete event per child comment /
// checklist item (§8.4, US-3.7 AC3). Emitting all tombstones in one tx mitigates
// a crash between the parent and child emits (NFR-2): either every tombstone is
// queued or none is. write performs the real domain soft-delete of the task and
// its children (invariants run in the caller's service); EmitDeleteCascade adds
// the federation sidecar around it.
//
// As with EmitMutation, a non-federated project runs the domain write only — no
// HLC bump, no outbox events (US-3.2 AC1).
func (e *Emitter) EmitDeleteCascade(ctx context.Context, spec MutationSpec, children []ChildTombstone, write func(tx *sql.Tx) error) error {
	federated, projectClientID, err := e.projectFederation(ctx, spec.LocalProjectID)
	if err != nil {
		return err
	}
	if !federated {
		return db.WithTx(ctx, e.db, write)
	}

	// One HLC moment stamps the whole cascade (parent + children deleted together).
	stamp, err := e.clock.Now(ctx)
	if err != nil {
		return fmt.Errorf("emit cascade mint hlc: %w", err)
	}
	hlcStr := stamp.String()

	priv, err := e.loadPrivateKey(ctx)
	if err != nil {
		return err
	}

	// Build a signed op=delete event for the parent and each child up front (no DB
	// access), so the emit tx only does writes.
	specs := make([]MutationSpec, 0, len(children)+1)
	specs = append(specs, MutationSpec{EntityType: spec.EntityType, EntityID: spec.EntityID, Op: events.OpDelete})
	for _, c := range children {
		specs = append(specs, MutationSpec{EntityType: c.EntityType, EntityID: c.EntityID, Op: events.OpDelete})
	}

	type outboxRow struct {
		spec    MutationSpec
		eventID string
		payload string
	}
	rows := make([]outboxRow, 0, len(specs))
	for _, ms := range specs {
		evt := e.buildEvent(ms, projectClientID, hlcStr)
		signed, serr := events.Sign(evt, priv)
		if serr != nil {
			return fmt.Errorf("emit cascade sign event: %w", serr)
		}
		payload, merr := protocol.Encode(signed, localEmitProtocolVersion())
		if merr != nil {
			return fmt.Errorf("emit cascade encode event: %w", merr)
		}
		rows = append(rows, outboxRow{spec: ms, eventID: signed.EventID, payload: string(payload)})
	}

	nowStr := model.FormatUTC(time.Now())
	if err := db.WithTx(ctx, e.db, func(tx *sql.Tx) error {
		if err := write(tx); err != nil {
			return err
		}
		for _, r := range rows {
			// Stamp the synthetic _deleted field HLC for every tombstoned entity so
			// future inbound events resolve the tombstone per-field LWW.
			if _, err := e.store.CASFieldHLCTx(ctx, tx, string(r.spec.EntityType), r.spec.EntityID, events.FieldDeleted, hlcStr); err != nil {
				return err
			}
			if err := e.store.InsertOutboxTx(ctx, tx, r.eventID, spec.LocalProjectID, r.payload, localEmitProtocolVersion(), nowStr); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return err
	}

	if e.onCommit != nil {
		e.onCommit()
	}
	return nil
}

// buildEvent assembles the canonical event for a local mutation, stamping every
// field with the minted HLC. A delete carries the synthetic _deleted field.
func (e *Emitter) buildEvent(spec MutationSpec, projectClientID, hlcStr string) events.Event {
	fields := map[string]events.Field{}
	if spec.Op == events.OpDelete {
		fields[events.FieldDeleted] = events.Field{Value: true, HLC: hlcStr}
	} else {
		for name, val := range spec.Fields {
			fields[name] = events.Field{Value: val, HLC: hlcStr}
		}
	}
	return events.Event{
		EventID:         model.NewClientID(),
		Op:              spec.Op,
		EntityType:      spec.EntityType,
		EntityID:        spec.EntityID,
		ProjectClientID: projectClientID,
		Author:          e.instanceURL,
		OriginInstance:  e.instanceURL,
		CreatedAt:       model.FormatUTC(time.Now()),
		Fields:          fields,
	}
}

// projectFederation reports whether the project is federated AND still active for
// outbound sync and, if so, its cross-instance client_id (the event's
// project_client_id). A non-existent / tombstoned project is treated as
// non-federated. A project whose JOINED copy has gone "lost" (the joiner
// voluntarily LEFT it — Federation v1 F5.5, US-6.3 AC3 — or the owner revoked it)
// is also treated as non-federated for emit purposes: a lost-left copy is a plain
// editable LOCAL project that must NOT emit outbound events ("stop sending"). The
// guard keys on the lost flag, not merely is_federated, exactly as the plan
// requires. The owner's own self-row carries is_owner=1 and is never lost, so the
// owner keeps emitting normally.
func (e *Emitter) projectFederation(ctx context.Context, projectID int64) (bool, string, error) {
	var isFederated int
	var clientID string
	err := e.db.QueryRowContext(ctx,
		`SELECT is_federated, client_id FROM projects WHERE id = ? AND deleted_at IS NULL`, projectID).
		Scan(&isFederated, &clientID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, "", nil
	}
	if err != nil {
		return false, "", fmt.Errorf("emit project federation lookup: %w", err)
	}
	if isFederated != 1 {
		return false, "", nil
	}
	// Stop emitting once a joined copy is lost (left/revoked): it is a local project
	// now (US-6.3 AC3). A lost row is the is_owner=0 mapping with lost=1; the owner's
	// is_owner=1 self-row is never lost, so this only suppresses joiner-side emit.
	var lost int
	err = e.db.QueryRowContext(ctx,
		`SELECT COUNT(1) FROM federated_projects
		  WHERE local_project_id = ? AND is_owner = 0 AND lost = 1`, projectID).Scan(&lost)
	if err != nil {
		return false, "", fmt.Errorf("emit project lost lookup: %w", err)
	}
	if lost > 0 {
		return false, "", nil
	}
	return true, clientID, nil
}

// loadPrivateKey loads and decrypts this instance's Ed25519 signing key. The
// keypair must already exist (EnableForProject ensures it); a missing key is
// reported as ErrKeyMissing.
func (e *Emitter) loadPrivateKey(ctx context.Context) (ed25519.PrivateKey, error) {
	if e.cipher == nil {
		return nil, ErrKeyMissing
	}
	fk, err := e.keys.Get(ctx)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return nil, ErrKeyMissing
		}
		return nil, fmt.Errorf("emit load keys: %w", err)
	}
	priv, _, err := crypto.LoadInstanceKeypair(e.cipher, fk.PublicKey, fk.PrivateSeedEnc)
	if err != nil {
		return nil, fmt.Errorf("emit load keypair: %w", err)
	}
	return priv, nil
}
