package snapshot

import (
	"bufio"
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/lebe-dev/turboist/internal/model"
)

// TestReApply_OverwritesExistingProjectAndAppliesTombstones drives the F4.2
// re-bootstrap consume: a peer that fell behind retention re-fetches the owner
// snapshot and OVERWRITES its EXISTING local project state in one transaction
// (US-4.2 AC2). Unlike the initial Apply (which creates a brand-new project),
// ReApply targets the project already mapped to the owner: it upserts the live
// entities by their cross-instance client_id (no duplicate rows), applies the
// snapshot's tombstones (a task deleted on the owner since the last sync becomes
// soft-deleted locally — no resurrection), rewrites the per-field HLC up to
// as_of, and stamps the re-bootstrap marker (cutoff X). The federation_outbox is
// NOT touched (asserted separately in TestReApply_PreservesOutbox).
func TestReApply_OverwritesExistingProjectAndAppliesTombstones(t *testing.T) {
	owner, ownerProjectID := fixtures(t)
	ctx := context.Background()

	// The joiner already holds a stale copy of the project (an earlier bootstrap).
	// Seed it: same project client_id, one live task that the owner has since
	// DELETED (so the re-bootstrap must tombstone it) and one task whose title is
	// stale (the owner renamed it); the existing tasks are reconciled by client_id.
	// (The companion survival case — a locally-created task with an unsent outbox
	// op=create that is absent from the owner snapshot and MUST survive the
	// convergence sweep — is exercised in
	// TestReApply_PreservesLocallyCreatedTaskWithUnsentOutbox.)
	jd := openMigrated(t)
	joiner := newOwnerDeps(jd)

	ownerSnap, err := Build(ctx, owner.db, ownerProjectID, "owner-node")
	if err != nil {
		t.Fatalf("owner build: %v", err)
	}

	// Initial bootstrap into the joiner so there is an EXISTING local project to
	// overwrite (mapped to the owner).
	var initBuf bytes.Buffer
	iw := bufio.NewWriter(&initBuf)
	if err := WriteNDJSON(iw, ownerSnap); err != nil {
		t.Fatalf("write initial ndjson: %v", err)
	}
	_ = iw.Flush()
	initRes, err := Apply(ctx, joinerDeps(joiner), ApplyParams{
		OwnerInstanceURL: "https://alice.example",
		RemoteProjectID:  "9",
		Permissions:      model.FederationPermissionWrite,
		Reader:           bytes.NewReader(initBuf.Bytes()),
	})
	if err != nil {
		t.Fatalf("initial apply: %v", err)
	}
	localProjectID := initRes.LocalProjectID

	// The owner now DELETES one of its live tasks and renames the project, then
	// the joiner falls behind retention and re-bootstraps from a fresh snapshot.
	var goneClientID string
	if err := owner.db.QueryRow(
		`SELECT client_id FROM tasks WHERE project_id = ? AND title = 'Live two'`, ownerProjectID).Scan(&goneClientID); err != nil {
		t.Fatalf("find live two: %v", err)
	}
	if _, err := owner.db.Exec(`UPDATE tasks SET deleted_at = '2026-06-02T00:00:00.000Z' WHERE client_id = ?`, goneClientID); err != nil {
		t.Fatalf("owner delete live two: %v", err)
	}
	if _, err := owner.db.Exec(`UPDATE projects SET title = 'Roadmap v2' WHERE id = ?`, ownerProjectID); err != nil {
		t.Fatalf("owner rename project: %v", err)
	}
	// Bump the project title field HLC so the rename wins LWW on re-bootstrap.
	if _, err := owner.db.Exec(
		`UPDATE entity_field_hlc SET hlc = '00000000099999-0000-owner' WHERE entity_type = 'project' AND field_name = 'title'`); err != nil {
		t.Fatalf("bump project title hlc: %v", err)
	}

	reSnap, err := Build(ctx, owner.db, ownerProjectID, "owner-node")
	if err != nil {
		t.Fatalf("owner re-build: %v", err)
	}
	var reBuf bytes.Buffer
	rw := bufio.NewWriter(&reBuf)
	if err := WriteNDJSON(rw, reSnap); err != nil {
		t.Fatalf("write re ndjson: %v", err)
	}
	_ = rw.Flush()

	at := time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC)
	res, err := ReApply(ctx, joinerDeps(joiner), ReApplyParams{
		LocalProjectID:   localProjectID,
		OwnerInstanceURL: "https://alice.example",
		Reader:           bytes.NewReader(reBuf.Bytes()),
		Now:              func() time.Time { return at },
	})
	if err != nil {
		t.Fatalf("re-apply: %v", err)
	}
	if res.LocalProjectID != localProjectID {
		t.Errorf("re-apply local project id: got %d, want %d (must overwrite the SAME project)", res.LocalProjectID, localProjectID)
	}

	// The existing project was overwritten (same id, new title) — NOT duplicated.
	var projCount int
	if err := jd.QueryRow(`SELECT COUNT(*) FROM projects WHERE client_id = ?`, ownerSnap.Project.ClientID).Scan(&projCount); err != nil {
		t.Fatalf("count project by client_id: %v", err)
	}
	if projCount != 1 {
		t.Errorf("project rows for the same client_id: got %d, want 1 (re-bootstrap must overwrite, not duplicate)", projCount)
	}
	lp, err := joiner.projects.Get(ctx, localProjectID)
	if err != nil {
		t.Fatalf("load local project: %v", err)
	}
	if lp.Title != "Roadmap v2" {
		t.Errorf("re-bootstrapped project title: got %q, want %q", lp.Title, "Roadmap v2")
	}

	// The owner-deleted task is now soft-deleted locally and NOT resurrected.
	var goneDeleted bool
	var deletedAt *string
	if err := jd.QueryRow(`SELECT deleted_at FROM tasks WHERE client_id = ?`, goneClientID).Scan(&deletedAt); err != nil {
		t.Fatalf("read gone task: %v", err)
	}
	goneDeleted = deletedAt != nil
	if !goneDeleted {
		t.Errorf("owner-deleted task not tombstoned on re-bootstrap (resurrection): deleted_at=%v", deletedAt)
	}

	// last_received_hlc advanced to the snapshot's as_of_hlc and the re-bootstrap
	// marker carries the cutoff X (as_of_hlc + wall-clock).
	fp, err := joiner.fedProjects.Get(ctx, localProjectID, "https://alice.example")
	if err != nil {
		t.Fatalf("load mapping: %v", err)
	}
	if fp.LastReceivedHLC != reSnap.AsOfHLC {
		t.Errorf("last_received_hlc after re-bootstrap: got %q, want %q", fp.LastReceivedHLC, reSnap.AsOfHLC)
	}
	if res.CutoffHLC != reSnap.AsOfHLC {
		t.Errorf("re-bootstrap cutoff hlc: got %q, want %q", res.CutoffHLC, reSnap.AsOfHLC)
	}
	if res.RebootstrappedAt != model.FormatUTC(at) {
		t.Errorf("re-bootstrap wall-clock cutoff: got %q, want %q (the cutoff X must be a real persisted value)", res.RebootstrappedAt, model.FormatUTC(at))
	}
}

// TestReApply_PreservesOutbox is the highest-impact F4.2 regression (R3): a
// re-bootstrap must NOT clear federation_outbox — the user's unsent edits made
// before the re-snapshot survive and are still delivered afterwards
// (US-4.2 AC2/AC3). Clearing the outbox on re-bootstrap is "the highest-impact
// bug", so this asserts the row count is identical before and after ReApply.
func TestReApply_PreservesOutbox(t *testing.T) {
	owner, ownerProjectID := fixtures(t)
	ctx := context.Background()

	jd := openMigrated(t)
	joiner := newOwnerDeps(jd)

	snap, err := Build(ctx, owner.db, ownerProjectID, "owner-node")
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	var buf bytes.Buffer
	w := bufio.NewWriter(&buf)
	if err := WriteNDJSON(w, snap); err != nil {
		t.Fatalf("write ndjson: %v", err)
	}
	_ = w.Flush()
	initRes, err := Apply(ctx, joinerDeps(joiner), ApplyParams{
		OwnerInstanceURL: "https://alice.example",
		Reader:           bytes.NewReader(buf.Bytes()),
	})
	if err != nil {
		t.Fatalf("initial apply: %v", err)
	}
	localProjectID := initRes.LocalProjectID

	// The joiner has TWO unsent outbox events (local edits awaiting delivery).
	for _, id := range []string{"unsent-1", "unsent-2"} {
		if _, err := jd.Exec(
			`INSERT INTO federation_outbox (event_id, local_project_id, payload, delivered_to, created_at)
			 VALUES (?, ?, '{}', '', '2026-06-03T00:00:00.000Z')`, id, localProjectID); err != nil {
			t.Fatalf("seed outbox %s: %v", id, err)
		}
	}
	var before int
	if err := jd.QueryRow(`SELECT COUNT(*) FROM federation_outbox`).Scan(&before); err != nil {
		t.Fatalf("count outbox before: %v", err)
	}
	if before != 2 {
		t.Fatalf("outbox precondition: got %d, want 2", before)
	}

	reSnap, err := Build(ctx, owner.db, ownerProjectID, "owner-node")
	if err != nil {
		t.Fatalf("re-build: %v", err)
	}
	var reBuf bytes.Buffer
	rw := bufio.NewWriter(&reBuf)
	if err := WriteNDJSON(rw, reSnap); err != nil {
		t.Fatalf("write re ndjson: %v", err)
	}
	_ = rw.Flush()

	if _, err := ReApply(ctx, joinerDeps(joiner), ReApplyParams{
		LocalProjectID:   localProjectID,
		OwnerInstanceURL: "https://alice.example",
		Reader:           bytes.NewReader(reBuf.Bytes()),
		Now:              time.Now,
	}); err != nil {
		t.Fatalf("re-apply: %v", err)
	}

	var after int
	if err := jd.QueryRow(`SELECT COUNT(*) FROM federation_outbox`).Scan(&after); err != nil {
		t.Fatalf("count outbox after: %v", err)
	}
	if after != before {
		t.Errorf("federation_outbox count changed by re-bootstrap: got %d, want %d (UNSENT edits must survive — R3)", after, before)
	}
	// Both unsent rows are still individually present (not just the count).
	for _, id := range []string{"unsent-1", "unsent-2"} {
		var n int
		if err := jd.QueryRow(`SELECT COUNT(*) FROM federation_outbox WHERE event_id = ?`, id).Scan(&n); err != nil {
			t.Fatalf("check outbox %s: %v", id, err)
		}
		if n != 1 {
			t.Errorf("unsent outbox event %s missing after re-bootstrap (got %d)", id, n)
		}
	}
}

// TestReApply_PreservesLocallyCreatedTaskWithUnsentOutbox is the R3 convergence
// regression (the highest-impact F4.2 bug): a write-permission joiner that created
// a task while offline past retention has a task that (a) carries a client_id,
// (b) has an unsent op=create in federation_outbox (preserved by R3), but (c) is
// NOT in the owner snapshot (the owner never received it). The convergence sweep
// must NOT soft-delete that task — it is the user's own unsent work and must stay
// live in their UI until the preserved outbox event flushes and the owner echoes
// it back (US-4.2 AC2/AC3). As a control, a federated task with NO outbox event
// AND absent from the snapshot is genuine upstream removal and IS soft-deleted.
func TestReApply_PreservesLocallyCreatedTaskWithUnsentOutbox(t *testing.T) {
	owner, ownerProjectID := fixtures(t)
	ctx := context.Background()

	jd := openMigrated(t)
	joiner := newOwnerDeps(jd)

	snap, err := Build(ctx, owner.db, ownerProjectID, "owner-node")
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	var buf bytes.Buffer
	w := bufio.NewWriter(&buf)
	if err := WriteNDJSON(w, snap); err != nil {
		t.Fatalf("write ndjson: %v", err)
	}
	_ = w.Flush()
	initRes, err := Apply(ctx, joinerDeps(joiner), ApplyParams{
		OwnerInstanceURL: "https://alice.example",
		Permissions:      model.FederationPermissionWrite,
		Reader:           bytes.NewReader(buf.Bytes()),
	})
	if err != nil {
		t.Fatalf("initial apply: %v", err)
	}
	localProjectID := initRes.LocalProjectID

	var contextID int64
	if err := jd.QueryRow(`SELECT context_id FROM projects WHERE id = ?`, localProjectID).Scan(&contextID); err != nil {
		t.Fatalf("read project context: %v", err)
	}

	// (a) The joiner created a task locally while offline. It carries a client_id
	// and is absent from the owner snapshot (the owner never received it).
	const localTaskCID = "01JLOCALCREATEDOFFLINETASK00"
	if _, err := jd.Exec(
		`INSERT INTO tasks (title, description, context_id, project_id, priority, status, day_part, plan_state, is_pinned, client_id, created_at, updated_at)
		 VALUES ('Offline create', '', ?, ?, 'no-priority', 'open', 'none', 'none', 0, ?, '2026-06-03T00:00:00.000Z', '2026-06-03T00:00:00.000Z')`,
		contextID, localProjectID, localTaskCID); err != nil {
		t.Fatalf("seed local task: %v", err)
	}
	// Its unsent op=create sits in federation_outbox (preserved by R3). The payload's
	// entity_id is the task's client_id — the join the convergence sweep uses.
	if _, err := jd.Exec(
		`INSERT INTO federation_outbox (event_id, local_project_id, payload, delivered_to, created_at)
		 VALUES ('evt-local-create', ?, ?, '', '2026-06-03T00:00:01.000Z')`,
		localProjectID,
		`{"event_id":"evt-local-create","op":"create","entity_type":"task","entity_id":"`+localTaskCID+`"}`); err != nil {
		t.Fatalf("seed outbox create: %v", err)
	}

	// (control) A federated task absent from the snapshot WITHOUT any outbox event:
	// genuine upstream removal — the sweep must soft-delete it.
	const goneTaskCID = "01JREMOVEDUPSTREAMTASK000000"
	if _, err := jd.Exec(
		`INSERT INTO tasks (title, description, context_id, project_id, priority, status, day_part, plan_state, is_pinned, client_id, created_at, updated_at)
		 VALUES ('Removed upstream', '', ?, ?, 'no-priority', 'open', 'none', 'none', 0, ?, '2026-06-03T00:00:00.000Z', '2026-06-03T00:00:00.000Z')`,
		contextID, localProjectID, goneTaskCID); err != nil {
		t.Fatalf("seed gone task: %v", err)
	}

	reSnap, err := Build(ctx, owner.db, ownerProjectID, "owner-node")
	if err != nil {
		t.Fatalf("re-build: %v", err)
	}
	var reBuf bytes.Buffer
	rw := bufio.NewWriter(&reBuf)
	if err := WriteNDJSON(rw, reSnap); err != nil {
		t.Fatalf("write re ndjson: %v", err)
	}
	_ = rw.Flush()

	at := time.Date(2026, 6, 4, 12, 0, 0, 0, time.UTC)
	if _, err := ReApply(ctx, joinerDeps(joiner), ReApplyParams{
		LocalProjectID:   localProjectID,
		OwnerInstanceURL: "https://alice.example",
		Reader:           bytes.NewReader(reBuf.Bytes()),
		Now:              func() time.Time { return at },
	}); err != nil {
		t.Fatalf("re-apply: %v", err)
	}

	// The locally-created task (unsent outbox create, absent from snapshot) is STILL
	// LIVE — the convergence sweep preserved it (it is the user's own unsent work).
	var localDeletedAt *string
	if err := jd.QueryRow(`SELECT deleted_at FROM tasks WHERE client_id = ?`, localTaskCID).Scan(&localDeletedAt); err != nil {
		t.Fatalf("read local task: %v", err)
	}
	if localDeletedAt != nil {
		t.Errorf("re-bootstrap soft-deleted the joiner's locally-created task (it has an unsent outbox create + is absent from the snapshot): deleted_at=%v — unsent local work must survive (R3)", *localDeletedAt)
	}

	// The control task (no outbox event, absent from snapshot) WAS soft-deleted —
	// genuine upstream-removal convergence still works.
	var goneDeletedAt *string
	if err := jd.QueryRow(`SELECT deleted_at FROM tasks WHERE client_id = ?`, goneTaskCID).Scan(&goneDeletedAt); err != nil {
		t.Fatalf("read gone task: %v", err)
	}
	if goneDeletedAt == nil {
		t.Errorf("re-bootstrap did NOT converge a genuinely upstream-removed federated task (no outbox, absent from snapshot): it should be soft-deleted")
	}
}

// TestReApply_OldHLCFieldLosesLWWGracefully asserts a re-bootstrap does NOT
// regress a field the joiner has already advanced locally past the snapshot's
// HLC (US-4.2 AC3 — an old-HLC value loses LWW gracefully). The local title HLC
// is higher than the snapshot's, so the higher-wins field_hlc upsert keeps the
// local value rather than clobbering it back to the (older) snapshot value.
func TestReApply_OldHLCFieldLosesLWWGracefully(t *testing.T) {
	owner, ownerProjectID := fixtures(t)
	ctx := context.Background()

	jd := openMigrated(t)
	joiner := newOwnerDeps(jd)

	snap, err := Build(ctx, owner.db, ownerProjectID, "owner-node")
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	var buf bytes.Buffer
	w := bufio.NewWriter(&buf)
	if err := WriteNDJSON(w, snap); err != nil {
		t.Fatalf("write ndjson: %v", err)
	}
	_ = w.Flush()
	initRes, err := Apply(ctx, joinerDeps(joiner), ApplyParams{OwnerInstanceURL: "https://alice.example", Reader: bytes.NewReader(buf.Bytes())})
	if err != nil {
		t.Fatalf("initial apply: %v", err)
	}
	localProjectID := initRes.LocalProjectID

	// The joiner has a LOCAL edit ahead of the snapshot: a far-future project-title
	// HLC (an unsent local rename). On re-bootstrap the snapshot's older project
	// title must NOT regress this field.
	if _, err := jd.Exec(
		`UPDATE entity_field_hlc SET hlc = '99999999999999-9999-joiner' WHERE entity_type = 'project' AND field_name = 'title'`); err != nil {
		t.Fatalf("advance local title hlc: %v", err)
	}
	if _, err := jd.Exec(`UPDATE projects SET title = 'Local rename' WHERE id = ?`, localProjectID); err != nil {
		t.Fatalf("apply local rename: %v", err)
	}

	if _, err := ReApply(ctx, joinerDeps(joiner), ReApplyParams{
		LocalProjectID:   localProjectID,
		OwnerInstanceURL: "https://alice.example",
		Reader:           bytes.NewReader(buf.Bytes()),
		Now:              time.Now,
	}); err != nil {
		t.Fatalf("re-apply: %v", err)
	}

	// The local rename survived (the snapshot's lower-HLC title lost LWW).
	var hlc string
	if err := jd.QueryRow(
		`SELECT hlc FROM entity_field_hlc WHERE entity_type = 'project' AND field_name = 'title'`).Scan(&hlc); err != nil {
		t.Fatalf("read project title hlc: %v", err)
	}
	if hlc != "99999999999999-9999-joiner" {
		t.Errorf("re-bootstrap regressed a locally-advanced field HLC: got %q, want the higher local HLC", hlc)
	}

	// The COLUMN VALUE must also still be the local rename — not silently regressed
	// to the snapshot's losing value. The field that WON LWW (higher HLC) must show
	// the value that won; otherwise the column and HLC diverge and no future peer
	// event at HLC <= the joiner's clock could ever repair it (US-4.2 AC3, R2 fix).
	var title string
	if err := jd.QueryRow(`SELECT title FROM projects WHERE id = ?`, localProjectID).Scan(&title); err != nil {
		t.Fatalf("read project title column: %v", err)
	}
	if title != "Local rename" {
		t.Errorf("re-bootstrap clobbered a locally-WON column to the snapshot's losing value: got %q, want %q", title, "Local rename")
	}
}

// TestReApply_ConvergesDeletedAndGhostSections asserts re-bootstrap converges
// SECTION deletions the same way it does tasks (Federation v1 F4.2): an
// owner-deleted section travels as a snapshot tombstone and is soft-deleted on the
// joiner; a federated section absent from the snapshot with no outbox event
// (genuine upstream removal) is swept; BUT the joiner's own locally-created section
// with an unsent op=create survives (R3, US-4.2 AC2/AC3). Before this fix,
// owner-deleted sections lingered as ghosts (readTombstones emitted no section
// tombstones and there was no section convergence sweep).
func TestReApply_ConvergesDeletedAndGhostSections(t *testing.T) {
	owner, ownerProjectID := fixtures(t)
	ctx := context.Background()

	var ownerSectionID int64
	var ownerSectionCID string
	if err := owner.db.QueryRow(
		`SELECT id, client_id FROM project_sections WHERE project_id = ? AND deleted_at IS NULL`,
		ownerProjectID).Scan(&ownerSectionID, &ownerSectionCID); err != nil {
		t.Fatalf("read owner section: %v", err)
	}
	if ownerSectionCID == "" {
		t.Fatalf("owner section has no client_id")
	}

	jd := openMigrated(t)
	joiner := newOwnerDeps(jd)

	snap, err := Build(ctx, owner.db, ownerProjectID, "owner-node")
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	var buf bytes.Buffer
	w := bufio.NewWriter(&buf)
	if err := WriteNDJSON(w, snap); err != nil {
		t.Fatalf("write ndjson: %v", err)
	}
	_ = w.Flush()
	initRes, err := Apply(ctx, joinerDeps(joiner), ApplyParams{
		OwnerInstanceURL: "https://alice.example",
		Permissions:      model.FederationPermissionWrite,
		Reader:           bytes.NewReader(buf.Bytes()),
	})
	if err != nil {
		t.Fatalf("initial apply: %v", err)
	}
	localProjectID := initRes.LocalProjectID

	var bootDeletedAt *string
	if err := jd.QueryRow(`SELECT deleted_at FROM project_sections WHERE client_id = ?`, ownerSectionCID).Scan(&bootDeletedAt); err != nil {
		t.Fatalf("read joiner section after bootstrap: %v", err)
	}
	if bootDeletedAt != nil {
		t.Fatalf("owner section did not materialize live on the joiner")
	}

	// (a) Ghost section: federated, absent from the snapshot, NO outbox → must be swept.
	const ghostSectionCID = "01JGHOSTSECTION000000000000"
	if _, err := jd.Exec(
		`INSERT INTO project_sections (project_id, title, position, client_id, created_at, updated_at)
		 VALUES (?, 'Ghost', 50, ?, '2026-06-03T00:00:00.000Z', '2026-06-03T00:00:00.000Z')`,
		localProjectID, ghostSectionCID); err != nil {
		t.Fatalf("seed ghost section: %v", err)
	}

	// (b) Local section with an unsent op=create in the outbox → must survive (R3).
	const localSectionCID = "01JLOCALSECTIONUNSENT0000000"
	if _, err := jd.Exec(
		`INSERT INTO project_sections (project_id, title, position, client_id, created_at, updated_at)
		 VALUES (?, 'Local unsent', 51, ?, '2026-06-03T00:00:00.000Z', '2026-06-03T00:00:00.000Z')`,
		localProjectID, localSectionCID); err != nil {
		t.Fatalf("seed local section: %v", err)
	}
	if _, err := jd.Exec(
		`INSERT INTO federation_outbox (event_id, local_project_id, payload, delivered_to, created_at)
		 VALUES ('evt-local-section', ?, ?, '', '2026-06-03T00:00:01.000Z')`,
		localProjectID,
		`{"event_id":"evt-local-section","op":"create","entity_type":"section","entity_id":"`+localSectionCID+`"}`); err != nil {
		t.Fatalf("seed outbox section create: %v", err)
	}

	// Owner deletes its "Backlog" section → the re-snapshot carries a section tombstone.
	if err := owner.sections.Delete(ctx, ownerSectionID); err != nil {
		t.Fatalf("owner delete section: %v", err)
	}

	reSnap, err := Build(ctx, owner.db, ownerProjectID, "owner-node")
	if err != nil {
		t.Fatalf("re-build: %v", err)
	}
	var reBuf bytes.Buffer
	rw := bufio.NewWriter(&reBuf)
	if err := WriteNDJSON(rw, reSnap); err != nil {
		t.Fatalf("write re ndjson: %v", err)
	}
	_ = rw.Flush()

	at := time.Date(2026, 6, 4, 12, 0, 0, 0, time.UTC)
	if _, err := ReApply(ctx, joinerDeps(joiner), ReApplyParams{
		LocalProjectID:   localProjectID,
		OwnerInstanceURL: "https://alice.example",
		Reader:           bytes.NewReader(reBuf.Bytes()),
		Now:              func() time.Time { return at },
	}); err != nil {
		t.Fatalf("re-apply: %v", err)
	}

	sectionDeleted := func(cid string) bool {
		var del *string
		if err := jd.QueryRow(`SELECT deleted_at FROM project_sections WHERE client_id = ?`, cid).Scan(&del); err != nil {
			t.Fatalf("read section %s: %v", cid, err)
		}
		return del != nil
	}

	if !sectionDeleted(ownerSectionCID) {
		t.Errorf("owner-deleted section lingered as a ghost on the joiner (no section tombstone applied)")
	}
	if !sectionDeleted(ghostSectionCID) {
		t.Errorf("re-bootstrap did NOT converge an upstream-removed section (no outbox, absent from snapshot)")
	}
	if sectionDeleted(localSectionCID) {
		t.Errorf("re-bootstrap soft-deleted the joiner's locally-created section with an unsent outbox create — unsent local work must survive (R3)")
	}
}

// joinerDeps adapts an ownerDeps to the snapshot ApplyDeps/ReApplyDeps shape.
func joinerDeps(d *ownerDeps) ApplyDeps {
	return ApplyDeps{
		DB:          d.db,
		Projects:    d.projects,
		Sections:    d.sections,
		Tasks:       d.tasks,
		Contexts:    d.contexts,
		FedProjects: d.fedProjects,
	}
}
