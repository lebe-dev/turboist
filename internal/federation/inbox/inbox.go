// Package inbox applies received federation events to the local domain via
// per-field Last-Writer-Wins (Federation v1 F3.1, US-3.2/US-3.3).
//
// Apply runs in ONE transaction so the field-HLC compare-and-set, the domain
// write, and the inbox bookkeeping commit or roll back together. For each field
// the incoming HLC is CAS'd against the stored per-field HLC: a strictly-greater
// HLC wins and the field value is written; a stale field is skipped without
// touching the others (US-3.3 AC2). Disjoint-field edits from two instances both
// land (US-3.3 AC1) and applying the same events in any order converges to the
// same state (US-3.3 AC3).
//
// op=create on an entity the receiver has never seen creates a GHOST ROW
// carrying the event's client_id so a later update resolves to it (§10.4a).
// op=delete soft-deletes the entity and records a synthetic _deleted field HLC
// so a later stale update cannot resurrect it (US-3.7 / §8).
//
// The full received event payload is retained in federation_inbox until GC —
// that retained payload IS the loser-record history a stale (skipped) field is
// preserved in (required for the US-3.4 history view later); no field value is
// silently discarded, only not applied to the live row.
//
// turboist-specific fields (troiki/plan/day-part/postpone/pin) are NOT in any
// entity's federated field whitelist, so a peer event that names them is ignored
// rather than rejected (§3 cross-app compatibility, W-8).
//
// DELIBERATE DEVIATION FROM "routes through services": per-field apply is a
// DIRECT column write (a raw UPDATE on the entity table inside the apply tx, after
// the per-field HLC CAS), NOT a call through internal/service. It therefore does
// NOT re-run any of the local domain invariants that the service layer enforces —
// troiki capacity, section/project placement caps, RRULE advance, or auto-label
// derivation. This is intentional: a federated edit is a CONVERGENT replication of
// a value already validated on the authoring instance, applied by per-field LWW;
// re-running placement/cap/troiki logic here would (a) be non-deterministic across
// instances and break US-3.3 AC3 convergence, and (b) risk rejecting a legitimately
// replicated value. Field VALUES are still bounded by columnValidators (poison
// guard) so an out-of-domain enum/color cannot land. A future reader must NOT
// assume the service invariants run on the inbox-apply path — they do not.
//
// Unknown field NAMES are ignored; out-of-domain field VALUES are not blindly
// trusted. A recognised constrained field (status/priority/color) carrying a
// value outside the local whitelist is a permanent PoisonError (see errors.go),
// classified do-not-retry so the F3.2 inbox-apply worker drops/quarantines the
// event instead of retrying it forever and head-of-line blocking the per-project
// queue. Transient errors (DB busy) stay retryable; use IsPoison to distinguish.
package inbox

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/lebe-dev/turboist/internal/db"
	"github.com/lebe-dev/turboist/internal/federation/events"
	"github.com/lebe-dev/turboist/internal/federation/hlc"
	"github.com/lebe-dev/turboist/internal/federation/store"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
)

// ErrUnknownProject is returned when an event targets a project_client_id the
// receiver has no local mapping for. The transport/membership checks (F3.2a)
// normally reject this earlier; Apply guards it as defense in depth.
var ErrUnknownProject = errors.New("inbox: event targets unknown federated project")

// ReBroadcaster re-enqueues a relayed peer event into the outbox so the owner can
// fan it out to the OTHER peers (Federation v1 F5.1, US-5.2 AC2). It is satisfied
// by *store.Store; kept as an interface so the applier's re-broadcast logic stays
// testable and the dependency direction stays inbox→store.
type ReBroadcaster interface {
	ReBroadcastOutboxTx(ctx context.Context, tx store.Querier, eventID string, localProjectID int64, payload, originInstance, createdAt string) error
}

// Applier merges federation events into the local domain. It is fed by the
// single inbox-apply goroutine (F3.2) so apply runs off the HTTP path; here in
// F3.1 it is exercised directly by Apply.
//
// When configured with WithReBroadcast (owner-hub mode, F5.1), an apply that
// CHANGES an entity of a project THIS instance OWNS is re-enqueued to the outbox,
// pre-stamped delivered-to-origin, so the owner relays a peer's edit to every
// OTHER peer without echoing it back (US-5.2 AC2). A non-owner (joined copy) never
// re-broadcasts — only the owner is the hub (W-7).
type Applier struct {
	db          *sql.DB
	tasks       *repo.TaskRepo
	projects    *repo.ProjectRepo
	sections    *repo.ProjectSectionRepo
	fedProjects *repo.FederatedProjectRepo
	store       *store.Store

	// reBroadcast + ownInstanceURL + commitPing drive the owner-hub re-broadcast
	// (F5.1). reBroadcast is nil when re-broadcast is disabled (a joined-only
	// instance, or tests that only assert the merge). ownInstanceURL identifies
	// which federated_projects self-row marks ownership. commitPing (optional) wakes
	// the outbox publisher after a re-broadcast commit so the relay pushes immediately
	// (NFR-1.1) rather than on the next safety-net tick.
	reBroadcast    ReBroadcaster
	ownInstanceURL string
	commitPing     func()
}

// NewApplier constructs the inbox applier. Re-broadcast is OFF by default; enable
// it with WithReBroadcast on the owner instance.
func NewApplier(database *sql.DB, tasks *repo.TaskRepo, projects *repo.ProjectRepo, sections *repo.ProjectSectionRepo, fedProjects *repo.FederatedProjectRepo, st *store.Store) *Applier {
	return &Applier{db: database, tasks: tasks, projects: projects, sections: sections, fedProjects: fedProjects, store: st}
}

// WithReBroadcast enables owner-hub re-broadcast (Federation v1 F5.1, US-5.2 AC2):
// an apply that changes an entity of a project this instance OWNS re-enqueues the
// relayed event to the outbox (pre-stamped delivered-to-origin) so the publisher
// fans it out to the OTHER peers. rb is the outbox writer (satisfied by
// *store.Store), ownInstanceURL is this instance's federation URL (the is_owner=1
// self-row peer_instance_url), and commitPing (may be nil) wakes the publisher
// after commit so the relay is immediate. It returns the applier for chaining.
func (a *Applier) WithReBroadcast(rb ReBroadcaster, ownInstanceURL string, commitPing func()) *Applier {
	a.reBroadcast = rb
	a.ownInstanceURL = ownInstanceURL
	a.commitPing = commitPing
	return a
}

// ApplyResult reports which fields of the event were applied (won per-field LWW)
// versus skipped as stale. The caller (and the F3.4 open-card notice / US-3.4
// history later) uses it to know whether anything changed.
type ApplyResult struct {
	// AppliedFields[name] is true when that field's incoming HLC won the CAS and
	// its value was written to the live row. A skipped (stale) field is absent.
	AppliedFields map[string]bool
	// EntityCreated is true when an op=create materialised a new ghost row.
	EntityCreated bool
	// EntityDeleted is true when an op=delete soft-deleted the entity.
	EntityDeleted bool
	// ProjectLost is true when an op=revoke control event marked this joiner's local
	// copy federation_lost for the FIRST time (the transition, Federation v1 F5.4,
	// US-6.2 AC3). A redelivered revoke that lands on an already-lost copy leaves it
	// false (idempotent), so no redundant refresh SSE is published.
	ProjectLost bool
}

// Apply merges one event into the local domain in a single transaction. peerURL
// is the instance the event arrived from (used only for the project mapping
// lookup; the per-event author/origin checks are F3.2a). The returned result
// records per-field win/skip outcomes.
func (a *Applier) Apply(ctx context.Context, e events.Event, peerURL string) (*ApplyResult, error) {
	res := &ApplyResult{AppliedFields: map[string]bool{}}

	localProjectID, err := a.resolveProject(ctx, e.ProjectClientID)
	if err != nil {
		return nil, err
	}

	err = db.WithTx(ctx, a.db, func(tx *sql.Tx) error {
		switch e.Op {
		case events.OpDelete:
			if err := a.applyDelete(ctx, tx, e, res); err != nil {
				return err
			}
		case events.OpCreate, events.OpUpdate:
			if err := a.applyUpsert(ctx, tx, e, localProjectID, peerURL, res); err != nil {
				return err
			}
		case events.OpRevoke:
			// Owner→joiner CONTROL event (Federation v1 F5.4, US-6.2 AC3): mark this
			// joiner's local copy federation_lost (read-only). It carries no per-field
			// LWW and is NEVER re-broadcast (point-to-point to the revoked peer), so it
			// returns BEFORE the re-broadcast leg below.
			if err := a.applyRevoke(ctx, tx, e, localProjectID, res); err != nil {
				return err
			}
			return a.store.MarkInboxAppliedTx(ctx, tx, e.EventID, model.FormatUTC(now()))
		case events.OpLeave:
			// Joiner→owner CONTROL event (Federation v1 F5.5, US-6.3 AC2): mark the
			// LEAVING peer's mapping federation_lost (reason=left) so the owner stops
			// fanning out to it and the peers list renders it "left". It carries no
			// per-field LWW and is point-to-point from the leaver to the owner, so it is
			// NEVER re-broadcast — it returns BEFORE the re-broadcast leg below.
			if err := a.applyLeave(ctx, tx, e, localProjectID, peerURL, res); err != nil {
				return err
			}
			return a.store.MarkInboxAppliedTx(ctx, tx, e.EventID, model.FormatUTC(now()))
		default:
			return fmt.Errorf("inbox: unknown op %q", e.Op)
		}
		// Owner-hub re-broadcast (F5.1, US-5.2 AC2): in the SAME tx as the merge so a
		// relay is crash-atomic with the change it relays. Only fires when this
		// instance owns the project AND the merge actually changed something.
		if err := a.reBroadcastTx(ctx, tx, e, localProjectID, res); err != nil {
			return err
		}
		// Stamp applied_at in the SAME tx as the merge so the dedup log records the
		// event as terminal atomically with the domain write (NFR-2): if the merge
		// commits, the event is no longer re-driven by ListUnappliedInbox; if the tx
		// rolls back (transient error), applied_at stays NULL and the event is retried.
		return a.store.MarkInboxAppliedTx(ctx, tx, e.EventID, model.FormatUTC(now()))
	})
	if err != nil {
		return nil, err
	}
	// Wake the publisher AFTER commit so the re-broadcast relay pushes immediately
	// (NFR-1.1) — never inside the tx (the worker reads on its own connection). A
	// ping when nothing was re-broadcast is harmless (a no-op drain), so this is kept
	// simple: ping whenever re-broadcast is wired and the merge changed something.
	if a.reBroadcast != nil && a.commitPing != nil && changed(res) {
		a.commitPing()
	}
	return res, nil
}

// reBroadcastTx re-enqueues a relayed peer event to the outbox so the owner fans
// it out to the OTHER peers (Federation v1 F5.1, US-5.2 AC2). It runs INSIDE the
// apply tx so the relay commits or rolls back atomically with the merge. It is a
// no-op unless ALL of: re-broadcast is enabled, this instance OWNS the project
// (an is_owner=1 self-row), and the merge actually CHANGED something (a stale
// no-op is never relayed). The echo-loop guard is delegated to the store, which
// pre-stamps delivered_to with the event's OriginInstance so the publisher never
// pushes the event back to where it came from.
func (a *Applier) reBroadcastTx(ctx context.Context, tx *sql.Tx, e events.Event, localProjectID int64, res *ApplyResult) error {
	if a.reBroadcast == nil || !changed(res) {
		return nil
	}
	owned, err := a.isOwnedLocally(ctx, tx, localProjectID)
	if err != nil {
		return err
	}
	if !owned {
		return nil // a joined copy is not the hub — only the owner re-broadcasts (W-7).
	}
	payload, err := events.Marshal(e)
	if err != nil {
		return fmt.Errorf("inbox re-broadcast marshal %q: %w", e.EventID, err)
	}
	return a.reBroadcast.ReBroadcastOutboxTx(ctx, tx, e.EventID, localProjectID, string(payload), e.OriginInstance, model.FormatUTC(now()))
}

// isOwnedLocally reports whether this instance is the OWNER of the project — it
// holds the is_owner=1 self-row whose peer_instance_url is this instance's own
// federation URL. Only the owner re-broadcasts (hub-and-spoke, W-7). The check
// runs inside the apply tx on the project's int64 id.
func (a *Applier) isOwnedLocally(ctx context.Context, tx *sql.Tx, localProjectID int64) (bool, error) {
	var n int
	err := tx.QueryRowContext(ctx,
		`SELECT COUNT(1) FROM federated_projects
		  WHERE local_project_id = ? AND is_owner = 1 AND peer_instance_url = ?`,
		localProjectID, a.ownInstanceURL).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("inbox owner check for project %d: %w", localProjectID, err)
	}
	return n > 0, nil
}

// resolveProject maps the event's project_client_id to the local int64 project
// id. The project must exist locally and be federated.
func (a *Applier) resolveProject(ctx context.Context, projectClientID string) (int64, error) {
	var id int64
	err := a.db.QueryRowContext(ctx,
		`SELECT id FROM projects WHERE client_id = ? AND deleted_at IS NULL`, projectClientID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrUnknownProject
	}
	if err != nil {
		return 0, fmt.Errorf("inbox resolve project %q: %w", projectClientID, err)
	}
	return id, nil
}

// applyUpsert handles op=create / op=update: per-field LWW over the entity's
// federated columns, creating a ghost row first when the entity is missing.
//
// Before any write it validates every recognised field's VALUE against the
// local whitelist (columnValidators). An out-of-domain enum/color value is a
// permanent PoisonError (do-not-retry) rather than a CHECK-constraint rollback
// that would head-of-line block the per-project queue forever (§3/W-8). The
// validation pass runs FIRST, across all fields, so the whole event is rejected
// before any partial write — a poison field cannot leave a half-applied row.
func (a *Applier) applyUpsert(ctx context.Context, tx *sql.Tx, e events.Event, localProjectID int64, peerURL string, res *ApplyResult) error {
	spec, ok := entitySpecs[e.EntityType]
	if !ok {
		// An entity type this build does not federate (e.g. comments/checklist when
		// the F0.2 schema is absent) — graceful field-set degradation (§3 / F3.1).
		return nil
	}

	// Defense in depth (the F3.2a Validator already rejects a field-less create/
	// update): never materialise a ghost row for an event carrying no per-field HLC.
	// A recovery re-drive of a durably-recorded inbox row reaches Apply without
	// re-running the Validator, so this guard keeps an empty-ghost event a no-op even
	// on that path.
	if !hasFieldHLC(e) {
		return nil
	}

	if err := validateFields(e, spec, peerURL); err != nil {
		return err
	}

	localID, exists, err := a.resolveEntity(ctx, tx, spec, e.EntityID)
	if err != nil {
		return err
	}
	if !exists {
		// Ghost row: materialise a minimal row carrying the cross-instance client_id
		// so this and future per-field events resolve to it (§10.4a). The fields are
		// then applied on top below.
		localID, err = a.createGhost(ctx, tx, spec, e.EntityID, localProjectID)
		if err != nil {
			return err
		}
		// §10.4 protective tombstone: if a delete for this entity already arrived
		// (an op=delete out-of-order before this create — orphan delete), the
		// recorded _deleted field HLC is the resurrection guard. When that tombstone
		// HLC is at-or-above this event's highest field HLC, the create LOSES per-field
		// LWW: materialise the ghost already soft-deleted so a stale create cannot
		// resurrect a deleted entity.
		deleted, derr := a.ghostBornDeleted(ctx, tx, e)
		if derr != nil {
			return derr
		}
		if deleted {
			nowStr := model.FormatUTC(now())
			if _, err := tx.ExecContext(ctx,
				fmt.Sprintf(`UPDATE %s SET deleted_at = ?, updated_at = ? WHERE id = ?`, spec.table),
				nowStr, nowStr, localID); err != nil {
				return fmt.Errorf("inbox ghost-born-deleted %s %q: %w", spec.table, e.EntityID, err)
			}
		}
		res.EntityCreated = true
	}

	// Resurrection guard (US-3.7 AC2 / §10.4): read the entity's tombstone HLC once.
	// A field whose HLC is at-or-below the _deleted HLC is a STALE pre-deletion
	// edit and must not modify the tombstoned row (it would "resurrect" a value the
	// delete already superseded). Its own per-field CAS would otherwise win (the
	// field may never have been written), so the tombstone HLC is the additional
	// gate. A field strictly newer than the tombstone is a legitimate later edit and
	// is allowed through (the row stays deleted; v1 does not auto-undelete).
	tombstoneHLC, err := a.fieldHLCTx(ctx, tx, string(e.EntityType), e.EntityID, events.FieldDeleted)
	if err != nil {
		return err
	}

	for name, field := range e.Fields {
		col, ok := spec.fields[name]
		if !ok {
			// Not a federated column for this entity type (turboist-local or unknown):
			// ignore the field but never reject the event (W-8 / forward-compat).
			continue
		}
		if tombstoneHLC != "" && hlc.CompareString(field.HLC, tombstoneHLC) <= 0 {
			// Stale relative to the tombstone — skip without resurrecting (US-3.7 AC2).
			continue
		}
		won, err := a.store.CASFieldHLCTx(ctx, tx, string(e.EntityType), e.EntityID, name, field.HLC)
		if err != nil {
			return casError(e, peerURL, name, field.HLC, err)
		}
		if !won {
			continue // stale field — skip, leave the live value untouched.
		}
		if err := setColumn(ctx, tx, spec.table, col, localID, field.Value); err != nil {
			if errors.Is(err, errValueShape) {
				// A wrong-typed value (e.g. a string for an integer/bool column) is a
				// PERMANENT data error — classify it as poison (do-not-retry) so the F3.2
				// worker drops the event instead of retrying a CHECK/NOT-NULL rollback
				// forever and head-of-line blocking the queue (§3/W-8). The tx rolls back,
				// so the field HLC CAS just done is undone — no sticky advance.
				ectx := eventCtx{eventID: e.EventID, peerURL: peerURL, entityType: string(e.EntityType), entityID: e.EntityID}
				return newPoison(ectx, name, field.Value, "unexpected value type for column")
			}
			return err
		}
		res.AppliedFields[name] = true
	}
	return nil
}

// validateFields checks every recognised field's VALUE against the local
// whitelist before any domain write. A constrained column (status/priority/
// color) carrying an out-of-domain value is a permanent PoisonError; an unknown
// field NAME is skipped here (it is ignored, not rejected — W-8). Free-text and
// shape-coerced columns are left to coerceValue at write time.
func validateFields(e events.Event, spec entitySpec, peerURL string) error {
	ectx := eventCtx{
		eventID:    e.EventID,
		peerURL:    peerURL,
		entityType: string(e.EntityType),
		entityID:   e.EntityID,
	}
	for name, field := range e.Fields {
		col, ok := spec.fields[name]
		if !ok {
			continue // unknown field name — ignored, never rejected (W-8).
		}
		validate, constrained := columnValidators[col]
		if !constrained {
			continue
		}
		if !validate(field.Value) {
			return newPoison(ectx, name, field.Value, fmt.Sprintf("out-of-domain value for %s", col))
		}
	}
	return nil
}

// applyRevoke handles the owner→joiner op=revoke CONTROL event (Federation v1
// F5.4, US-6.2 AC3): it marks THIS joiner's local copy of the project
// federation_lost (reason=revoked) so the copy renders read-only. It targets the
// is_owner=0 mapping whose origin_instance_url is the event's OriginInstance (the
// owner that revoked us). It runs inside the apply tx so the marker commits
// atomically with the dedup applied_at stamp. It is IDEMPOTENT: a redelivered
// revoke (at-least-once) that lands on an already-lost copy is a no-op transition
// (ProjectLost stays false), so no redundant refresh SSE is published. A revoke
// for an entity-mapping the joiner does not hold is a silent no-op (defense in
// depth — the validator already gated membership).
func (a *Applier) applyRevoke(ctx context.Context, tx *sql.Tx, e events.Event, localProjectID int64, res *ApplyResult) error {
	// The origin owner is the authority that revoked us; the joiner's lost row is
	// keyed on its mapping to that owner (origin_instance_url), never the int64 PK.
	res2, err := tx.ExecContext(ctx,
		`UPDATE federated_projects
		    SET lost = 1, lost_reason = ?
		  WHERE local_project_id = ? AND origin_instance_url = ? AND is_owner = 0 AND lost = 0`,
		string(model.FederationLostRevoked), localProjectID, e.OriginInstance)
	if err != nil {
		return fmt.Errorf("inbox revoke mark-lost project %d: %w", localProjectID, err)
	}
	n, err := res2.RowsAffected()
	if err != nil {
		return fmt.Errorf("inbox revoke mark-lost rows: %w", err)
	}
	res.ProjectLost = n > 0
	return nil
}

// applyLeave handles the joiner→owner op=leave CONTROL event (Federation v1 F5.5,
// US-6.3 AC2): the OWNER marks the LEAVING peer's mapping federation_lost
// (reason=left) so the owner stops fanning out to it and the peers list renders it
// "left". The leaving peer is identified by the event's Author (== OriginInstance,
// the F3.2a validator already enforced equality) and by the transport peerURL the
// event arrived on; both are the same instance for a directly-pushed leave. We key
// the mark on the EVENT AUTHOR (the leaver) so a relayed leave (should one ever be
// fanned through a hub) still targets the correct peer rather than the relay. It
// runs inside the apply tx so the marker commits atomically with the dedup
// applied_at stamp. It is IDEMPOTENT: a redelivered leave (at-least-once) that
// lands on an already-left peer is a no-op transition (ProjectLost stays false),
// so no redundant refresh SSE is published. A leave for a peer this instance does
// not hold is a silent no-op (the validator already gated membership).
func (a *Applier) applyLeave(ctx context.Context, tx *sql.Tx, e events.Event, localProjectID int64, peerURL string, res *ApplyResult) error {
	// The leaving peer is the event author (origin); fall back to the transport peer
	// when the author is blank (defense in depth — the validator requires them equal).
	leaver := e.Author
	if leaver == "" {
		leaver = peerURL
	}
	transitioned, err := a.fedProjects.MarkLeftByPeerTx(ctx, tx, localProjectID, leaver)
	if err != nil {
		return fmt.Errorf("inbox leave mark-left project %d peer %q: %w", localProjectID, leaver, err)
	}
	// ProjectLost reuses the control-event "this peer's link transitioned to lost"
	// signal so the owner's open tabs refresh the peers list once (US-6.3 AC2).
	res.ProjectLost = transitioned
	return nil
}

// applyDelete handles op=delete: CAS the synthetic _deleted field HLC, and if it
// wins, soft-delete the live row (US-3.7 / §8). A stale delete is a no-op.
//
// The CAS records a PROTECTIVE tombstone HLC even when the receiver has never
// seen the entity (an orphan delete, §10.4): the live UPDATE then affects zero
// rows, but the _deleted field HLC is on record so a LATER lower-HLC create
// cannot resurrect it (per-field LWW).
//
// When a TASK is deleted, its local comments + checklist_items are cascade-
// soft-deleted in the SAME tx so the receiver never shows orphan children (§8.4,
// US-3.7 AC3). The cascade is LOCAL ONLY — the receiver writes NO new federation
// events; the origin emits the child tombstones explicitly (re-emitting here would
// create echo loops).
func (a *Applier) applyDelete(ctx context.Context, tx *sql.Tx, e events.Event, res *ApplyResult) error {
	spec, ok := entitySpecs[e.EntityType]
	if !ok {
		return nil
	}
	delHLC := e.MaxFieldHLC()
	if f, present := e.Fields[events.FieldDeleted]; present && f.HLC != "" {
		delHLC = f.HLC
	}
	won, err := a.store.CASFieldHLCTx(ctx, tx, string(e.EntityType), e.EntityID, events.FieldDeleted, delHLC)
	if err != nil {
		return casError(e, "", events.FieldDeleted, delHLC, err)
	}
	if !won {
		return nil // stale delete (or already-deleted at a higher HLC) — no-op.
	}
	now := model.FormatUTC(now())
	resExec, err := tx.ExecContext(ctx,
		fmt.Sprintf(`UPDATE %s SET deleted_at = ?, updated_at = ? WHERE client_id = ? AND deleted_at IS NULL`, spec.table),
		now, now, e.EntityID)
	if err != nil {
		return fmt.Errorf("inbox soft-delete %s %q: %w", spec.table, e.EntityID, err)
	}
	affected, err := resExec.RowsAffected()
	if err != nil {
		return fmt.Errorf("inbox soft-delete rows %s %q: %w", spec.table, e.EntityID, err)
	}
	// A live row actually transitioned to tombstoned only when a row was affected.
	// The CAS can win the per-field HLC while the row is already soft-deleted (or
	// absent) — that affects 0 rows, so it is NOT a fresh deletion and must not flip
	// EntityDeleted (which drives a redundant SSE refresh). Cascade still runs only
	// when a real task deletion happened.
	if affected == 0 {
		return nil
	}
	if e.EntityType == events.EntityTask {
		if err := a.cascadeChildTombstones(ctx, tx, e.EntityID, now); err != nil {
			return err
		}
	}
	res.EntityDeleted = true
	return nil
}

// cascadeChildTombstones soft-deletes the local comments + checklist_items of a
// task identified by its cross-instance client_id, without emitting any event
// (§8.4 receiver leg of US-3.7 AC3). The child tables may be absent on a build
// without the F0.2 schema; the UPDATE is a no-op if there are no rows. The local
// task int64 id is resolved from the client_id; a missing local task (orphan
// delete) means there are no local children to cascade.
func (a *Applier) cascadeChildTombstones(ctx context.Context, tx *sql.Tx, taskClientID, now string) error {
	var localTaskID int64
	err := tx.QueryRowContext(ctx, `SELECT id FROM tasks WHERE client_id = ?`, taskClientID).Scan(&localTaskID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil // orphan delete — no local task, so no local children.
	}
	if err != nil {
		return fmt.Errorf("inbox cascade resolve task %q: %w", taskClientID, err)
	}
	for _, table := range []string{"comments", "checklist_items"} {
		if _, err := tx.ExecContext(ctx,
			fmt.Sprintf(`UPDATE %s SET deleted_at = ?, updated_at = ? WHERE task_id = ? AND deleted_at IS NULL`, table),
			now, now, localTaskID); err != nil {
			return fmt.Errorf("inbox cascade %s for task %q: %w", table, taskClientID, err)
		}
	}
	return nil
}

// ghostBornDeleted reports whether a freshly created ghost must be born
// soft-deleted: a protective _deleted tombstone HLC is already on record (an
// out-of-order orphan delete arrived before this create) AND it is at-or-above
// this create event's greatest field HLC, so the tombstone wins per-field LWW
// (§10.4). When the create's fields are newer than the tombstone, the entity is
// legitimately re-created and this returns false.
func (a *Applier) ghostBornDeleted(ctx context.Context, tx *sql.Tx, e events.Event) (bool, error) {
	stored, err := a.fieldHLCTx(ctx, tx, string(e.EntityType), e.EntityID, events.FieldDeleted)
	if err != nil {
		return false, err
	}
	if stored == "" {
		return false, nil // no tombstone on record.
	}
	return hlc.CompareString(stored, e.MaxFieldHLC()) >= 0, nil
}

// fieldHLCTx reads a single field's stored HLC inside the caller tx, returning
// the empty string when no row exists.
func (a *Applier) fieldHLCTx(ctx context.Context, tx *sql.Tx, entityType, entityID, field string) (string, error) {
	var hlc string
	err := tx.QueryRowContext(ctx,
		`SELECT hlc FROM entity_field_hlc WHERE entity_type = ? AND entity_id = ? AND field_name = ?`,
		entityType, entityID, field).Scan(&hlc)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read field_hlc %s/%s/%s: %w", entityType, entityID, field, err)
	}
	return hlc, nil
}

// resolveEntity finds the local int64 id of an entity by its cross-instance
// client_id. exists=false means the receiver has never seen it (ghost-row path).
func (a *Applier) resolveEntity(ctx context.Context, tx *sql.Tx, spec entitySpec, clientID string) (int64, bool, error) {
	var id int64
	err := tx.QueryRowContext(ctx,
		fmt.Sprintf(`SELECT id FROM %s WHERE client_id = ?`, spec.table), clientID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("inbox resolve %s %q: %w", spec.table, clientID, err)
	}
	return id, true, nil
}
