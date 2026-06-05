package snapshot

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lebe-dev/turboist/internal/db"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
)

// fixtures builds a federated project with two live tasks, one section, one
// soft-deleted (tombstoned) task, and a couple of field_hlc rows, returning the
// owner-side dependencies and the project id.
func fixtures(t *testing.T) (*ownerDeps, int64) {
	t.Helper()
	d := openMigrated(t)
	deps := newOwnerDeps(d)
	ctx := context.Background()

	cx, err := deps.contexts.Create(ctx, "Work", "blue", false)
	if err != nil {
		t.Fatalf("create context: %v", err)
	}
	deps.firstContextID = cx.ID
	p, err := deps.projects.Create(ctx, repo.CreateProject{ContextID: cx.ID, Title: "Roadmap", Color: "blue"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	sec, err := deps.sections.Create(ctx, p.ID, "Backlog")
	if err != nil {
		t.Fatalf("create section: %v", err)
	}
	t1, err := deps.tasks.Create(ctx, repo.CreateTask{Placement: repo.Placement{ContextID: &cx.ID, ProjectID: &p.ID, SectionID: &sec.ID}, Title: "Live one"})
	if err != nil {
		t.Fatalf("create task1: %v", err)
	}
	if _, err := deps.tasks.Create(ctx, repo.CreateTask{Placement: repo.Placement{ContextID: &cx.ID, ProjectID: &p.ID}, Title: "Live two"}); err != nil {
		t.Fatalf("create task2: %v", err)
	}
	gone, err := deps.tasks.Create(ctx, repo.CreateTask{Placement: repo.Placement{ContextID: &cx.ID, ProjectID: &p.ID}, Title: "Deleted"})
	if err != nil {
		t.Fatalf("create gone task: %v", err)
	}
	if err := deps.tasks.Delete(ctx, gone.ID); err != nil {
		t.Fatalf("soft-delete task: %v", err)
	}

	// Seed a couple of field_hlc rows so the snapshot carries them.
	if _, err := d.Exec(
		`INSERT INTO entity_field_hlc (entity_type, entity_id, field_name, hlc) VALUES
		 ('task', ?, 'title', '00000000000100-0000-owner'),
		 ('project', ?, 'title', '00000000000100-0000-owner')`,
		t1.ClientID, p.ClientID,
	); err != nil {
		t.Fatalf("seed field_hlc: %v", err)
	}
	return deps, p.ID
}

// TestBuild_NDJSONShape asserts the buffer-first build emits NDJSON: a project
// line, task lines, a section line, a tombstone line for the deleted task,
// field_hlc lines, and a terminating sentinel `{"type":"end","as_of_hlc":...}`
// (US-2.3 AC2/AC3). Soft-deleted tasks appear ONLY as tombstones, never as live
// entity lines (US-2.3 AC3).
func TestBuild_NDJSONShape(t *testing.T) {
	deps, projectID := fixtures(t)
	ctx := context.Background()

	snap, err := Build(ctx, deps.db, projectID, "owner-node")
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	var buf bytes.Buffer
	w := bufio.NewWriter(&buf)
	if err := WriteNDJSON(w, snap); err != nil {
		t.Fatalf("write ndjson: %v", err)
	}
	_ = w.Flush()

	lines := splitLines(buf.String())
	if len(lines) < 5 {
		t.Fatalf("expected several NDJSON lines, got %d:\n%s", len(lines), buf.String())
	}

	types := map[string]int{}
	var endAsOf string
	var deletedClientIDs []string
	liveTaskTitles := map[string]bool{}
	for _, ln := range lines {
		var probe struct {
			Type     string `json:"type"`
			AsOfHLC  string `json:"as_of_hlc"`
			EntityID string `json:"entity_id"`
			Entity   struct {
				Title string `json:"title"`
			} `json:"entity"`
		}
		if err := json.Unmarshal([]byte(ln), &probe); err != nil {
			t.Fatalf("decode line %q: %v", ln, err)
		}
		types[probe.Type]++
		switch probe.Type {
		case "end":
			endAsOf = probe.AsOfHLC
		case "tombstone":
			deletedClientIDs = append(deletedClientIDs, probe.EntityID)
		case "task":
			liveTaskTitles[probe.Entity.Title] = true
		}
	}

	// The first line must be the project, the last must be the end sentinel.
	if !strings.Contains(lines[0], `"type":"project"`) {
		t.Errorf("first line is not the project: %q", lines[0])
	}
	if types["end"] != 1 {
		t.Errorf("end sentinel count: got %d, want 1", types["end"])
	}
	if endAsOf == "" {
		t.Errorf("end sentinel missing as_of_hlc")
	}
	if types["task"] != 2 {
		t.Errorf("live task lines: got %d, want 2", types["task"])
	}
	if types["section"] != 1 {
		t.Errorf("section lines: got %d, want 1", types["section"])
	}
	if types["field_hlc"] < 2 {
		t.Errorf("field_hlc lines: got %d, want >= 2", types["field_hlc"])
	}
	if len(deletedClientIDs) != 1 {
		t.Errorf("tombstone lines: got %d, want 1", len(deletedClientIDs))
	}
	if liveTaskTitles["Deleted"] {
		t.Errorf("soft-deleted task appeared as a live task line")
	}
}

// TestBuild_ConsistentAsOfUnderConcurrentWrite asserts the buffer-first build
// (a) takes a consistent read and (b) does NOT hold the lone writer connection
// across the whole stream, so concurrent app writes proceed during/after the
// build (§7.4 consistency + NFR-1.4 availability / R1).
func TestBuild_ConsistentAsOfUnderConcurrentWrite(t *testing.T) {
	deps, projectID := fixtures(t)
	ctx := context.Background()

	snap, err := Build(ctx, deps.db, projectID, "owner-node")
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	// A concurrent app write must succeed immediately after the read is buffered
	// — proving the build did not hold the single connection for streaming.
	done := make(chan error, 1)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		pid := projectID
		cid := deps.firstContextID
		_, err := deps.tasks.Create(ctx, repo.CreateTask{Placement: repo.Placement{ContextID: &cid, ProjectID: &pid}, Title: "Added during stream"})
		done <- err
	}()

	// Stream from the buffer (no DB access here).
	var buf bytes.Buffer
	w := bufio.NewWriter(&buf)
	if err := WriteNDJSON(w, snap); err != nil {
		t.Fatalf("write ndjson: %v", err)
	}
	_ = w.Flush()
	wg.Wait()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("concurrent write failed (connection held across stream?): %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("concurrent write blocked — buffer-first invariant violated")
	}

	// The buffered snapshot reflects the as-of moment: the late insert is NOT in it.
	if strings.Contains(buf.String(), "Added during stream") {
		t.Errorf("snapshot is not a consistent as-of read; it captured a later write")
	}
}

// TestApply_CreatesProjectAndEntities asserts the joiner-side consume: it creates
// a local project mapped to (origin_instance_url, origin_client_id), applies the
// tasks/sections through the repos, writes field_hlc + tombstones, sets
// last_received_hlc=as_of, and that soft-deleted entities are NOT listed live
// (US-2.3 AC3, AC5). The whole apply is one transaction.
func TestApply_CreatesProjectAndEntities(t *testing.T) {
	owner, projectID := fixtures(t)
	ctx := context.Background()
	snap, err := Build(ctx, owner.db, projectID, "owner-node")
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	var buf bytes.Buffer
	w := bufio.NewWriter(&buf)
	if err := WriteNDJSON(w, snap); err != nil {
		t.Fatalf("write ndjson: %v", err)
	}
	_ = w.Flush()

	// Joiner instance.
	jd := openMigrated(t)
	joiner := newOwnerDeps(jd)
	res, err := Apply(ctx, ApplyDeps{
		DB:          jd,
		Projects:    joiner.projects,
		Sections:    joiner.sections,
		Tasks:       joiner.tasks,
		Contexts:    joiner.contexts,
		FedProjects: joiner.fedProjects,
	}, ApplyParams{
		OwnerInstanceURL: "https://alice.example",
		Reader:           bytes.NewReader(buf.Bytes()),
	})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if res.LocalProjectID == 0 {
		t.Fatalf("apply did not return a local project id")
	}
	if res.AsOfHLC == "" {
		t.Errorf("apply did not return as_of_hlc")
	}

	// The local project exists and is federated.
	lp, err := joiner.projects.Get(ctx, res.LocalProjectID)
	if err != nil {
		t.Fatalf("load local project: %v", err)
	}
	if lp.Title != "Roadmap" {
		t.Errorf("local project title: got %q, want Roadmap", lp.Title)
	}

	// Two live tasks landed; the tombstoned one did NOT (no resurrection).
	tasks, _, err := joiner.tasks.ListByProject(ctx, res.LocalProjectID, repo.TaskFilter{}, repo.Page{Limit: 100})
	if err != nil {
		t.Fatalf("list tasks: %v", err)
	}
	if len(tasks) != 2 {
		t.Errorf("live tasks after apply: got %d, want 2", len(tasks))
	}
	for _, tk := range tasks {
		if tk.Title == "Deleted" {
			t.Errorf("soft-deleted task resurrected in joiner project")
		}
	}

	// A federated_projects mapping (is_owner=0) was written with last_received_hlc.
	fp, err := joiner.fedProjects.Get(ctx, res.LocalProjectID, "https://alice.example")
	if err != nil {
		t.Fatalf("load federated mapping: %v", err)
	}
	if fp.IsOwner {
		t.Errorf("joiner mapping should not be is_owner")
	}
	if fp.LastReceivedHLC != res.AsOfHLC {
		t.Errorf("last_received_hlc: got %q, want %q", fp.LastReceivedHLC, res.AsOfHLC)
	}

	// field_hlc rows landed.
	var fhlc int
	if err := jd.QueryRow(`SELECT COUNT(*) FROM entity_field_hlc`).Scan(&fhlc); err != nil {
		t.Fatalf("count field_hlc: %v", err)
	}
	if fhlc < 2 {
		t.Errorf("field_hlc rows applied: got %d, want >= 2", fhlc)
	}
}

// TestApply_MidStreamFailureRollsBack asserts that a malformed line mid-stream
// rolls the WHOLE apply back: no local project, no tasks, no mapping (US-2.3
// AC5 — full rollback, no resume).
func TestApply_MidStreamFailureRollsBack(t *testing.T) {
	owner, projectID := fixtures(t)
	ctx := context.Background()
	snap, err := Build(ctx, owner.db, projectID, "owner-node")
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	var buf bytes.Buffer
	w := bufio.NewWriter(&buf)
	if err := WriteNDJSON(w, snap); err != nil {
		t.Fatalf("write ndjson: %v", err)
	}
	_ = w.Flush()

	// Corrupt the stream: keep the project line (so a project starts being
	// created) but inject a garbage line before the sentinel.
	lines := splitLines(buf.String())
	corrupted := lines[0] + "\n" + `{"type":"task","entity":{` + "\n" + lines[len(lines)-1] + "\n"

	jd := openMigrated(t)
	joiner := newOwnerDeps(jd)
	_, err = Apply(ctx, ApplyDeps{
		DB:          jd,
		Projects:    joiner.projects,
		Sections:    joiner.sections,
		Tasks:       joiner.tasks,
		Contexts:    joiner.contexts,
		FedProjects: joiner.fedProjects,
	}, ApplyParams{
		OwnerInstanceURL: "https://alice.example",
		Reader:           strings.NewReader(corrupted),
	})
	if err == nil {
		t.Fatalf("expected apply to fail on a corrupted stream")
	}

	// Nothing persisted — the project insert rolled back with the failed line.
	var projN, taskN, fpN int
	if err := jd.QueryRow(`SELECT COUNT(*) FROM projects WHERE title = 'Roadmap'`).Scan(&projN); err != nil {
		t.Fatalf("count projects: %v", err)
	}
	if projN != 0 {
		t.Errorf("project persisted after rollback: got %d, want 0", projN)
	}
	if err := jd.QueryRow(`SELECT COUNT(*) FROM tasks`).Scan(&taskN); err != nil {
		t.Fatalf("count tasks: %v", err)
	}
	if taskN != 0 {
		t.Errorf("tasks persisted after rollback: got %d, want 0", taskN)
	}
	if err := jd.QueryRow(`SELECT COUNT(*) FROM federated_projects`).Scan(&fpN); err != nil {
		t.Fatalf("count federated_projects: %v", err)
	}
	if fpN != 0 {
		t.Errorf("federated mapping persisted after rollback: got %d, want 0", fpN)
	}
}

// TestApply_RejectsMalformedAsOfHLC asserts that a malformed as_of_hlc in the end
// sentinel aborts the whole apply (ErrInvalidHLC) and persists nothing — a
// signature-trusted-but-buggy owner must not corrupt the joiner's pull cursor /
// per-field LWW with a non-canonical HLC string (F2.3 #7).
func TestApply_RejectsMalformedAsOfHLC(t *testing.T) {
	owner, projectID := fixtures(t)
	ctx := context.Background()
	snap, err := Build(ctx, owner.db, projectID, "owner-node")
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	// Replace the as_of in the buffered snapshot with garbage and serialise it.
	snap.AsOfHLC = "not-an-hlc"
	var buf bytes.Buffer
	w := bufio.NewWriter(&buf)
	if err := WriteNDJSON(w, snap); err != nil {
		t.Fatalf("write ndjson: %v", err)
	}
	_ = w.Flush()

	assertApplyRejectsHLC(t, buf.String())
}

// TestApply_RejectsMalformedFieldHLC asserts that a malformed per-field HLC line
// aborts the apply and persists nothing (F2.3 #7).
func TestApply_RejectsMalformedFieldHLC(t *testing.T) {
	owner, projectID := fixtures(t)
	ctx := context.Background()
	snap, err := Build(ctx, owner.db, projectID, "owner-node")
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if len(snap.FieldHLCs) == 0 {
		t.Fatalf("fixture produced no field_hlc lines to corrupt")
	}
	// Corrupt one per-field HLC; as_of stays valid so we isolate the field check.
	snap.FieldHLCs[0].HLC = ""
	var buf bytes.Buffer
	w := bufio.NewWriter(&buf)
	if err := WriteNDJSON(w, snap); err != nil {
		t.Fatalf("write ndjson: %v", err)
	}
	_ = w.Flush()

	assertApplyRejectsHLC(t, buf.String())
}

// assertApplyRejectsHLC applies stream into a fresh joiner and asserts Apply
// returns ErrInvalidHLC and persists nothing (no project, tasks, mapping, or
// field_hlc rows) — mirroring TestApply_MidStreamFailureRollsBack.
func assertApplyRejectsHLC(t *testing.T, stream string) {
	t.Helper()
	ctx := context.Background()
	jd := openMigrated(t)
	joiner := newOwnerDeps(jd)
	_, err := Apply(ctx, ApplyDeps{
		DB:          jd,
		Projects:    joiner.projects,
		Sections:    joiner.sections,
		Tasks:       joiner.tasks,
		Contexts:    joiner.contexts,
		FedProjects: joiner.fedProjects,
	}, ApplyParams{
		OwnerInstanceURL: "https://alice.example",
		Reader:           strings.NewReader(stream),
	})
	if !errors.Is(err, ErrInvalidHLC) {
		t.Fatalf("apply error: got %v, want ErrInvalidHLC", err)
	}

	var projN, taskN, fpN, fhN int
	if err := jd.QueryRow(`SELECT COUNT(*) FROM projects WHERE title = 'Roadmap'`).Scan(&projN); err != nil {
		t.Fatalf("count projects: %v", err)
	}
	if projN != 0 {
		t.Errorf("project persisted after rejected HLC: got %d, want 0", projN)
	}
	if err := jd.QueryRow(`SELECT COUNT(*) FROM tasks`).Scan(&taskN); err != nil {
		t.Fatalf("count tasks: %v", err)
	}
	if taskN != 0 {
		t.Errorf("tasks persisted after rejected HLC: got %d, want 0", taskN)
	}
	if err := jd.QueryRow(`SELECT COUNT(*) FROM federated_projects`).Scan(&fpN); err != nil {
		t.Fatalf("count federated_projects: %v", err)
	}
	if fpN != 0 {
		t.Errorf("federated mapping persisted after rejected HLC: got %d, want 0", fpN)
	}
	if err := jd.QueryRow(`SELECT COUNT(*) FROM entity_field_hlc`).Scan(&fhN); err != nil {
		t.Fatalf("count entity_field_hlc: %v", err)
	}
	if fhN != 0 {
		t.Errorf("field_hlc rows persisted after rejected HLC: got %d, want 0", fhN)
	}
}

// TestSnapshot_PreservesSubtaskHierarchy asserts that a project subtask
// (a task carrying parent_id — allowed by the schema for project tasks) survives
// the build→apply round-trip with its parent link intact, instead of being
// silently flattened to a top-level task (US-2.3 — no structural data loss).
func TestSnapshot_PreservesSubtaskHierarchy(t *testing.T) {
	d := openMigrated(t)
	owner := newOwnerDeps(d)
	ctx := context.Background()

	cx, err := owner.contexts.Create(ctx, "Work", "blue", false)
	if err != nil {
		t.Fatalf("create context: %v", err)
	}
	p, err := owner.projects.Create(ctx, repo.CreateProject{ContextID: cx.ID, Title: "Roadmap", Color: "blue"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	parent, err := owner.tasks.Create(ctx, repo.CreateTask{Placement: repo.Placement{ContextID: &cx.ID, ProjectID: &p.ID}, Title: "Parent task"})
	if err != nil {
		t.Fatalf("create parent: %v", err)
	}
	child, err := owner.tasks.Create(ctx, repo.CreateTask{Placement: repo.Placement{ContextID: &cx.ID, ProjectID: &p.ID, ParentID: &parent.ID}, Title: "Child task"})
	if err != nil {
		t.Fatalf("create child: %v", err)
	}

	snap, err := Build(ctx, d, p.ID, "owner-node")
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	// The build must carry the child's parent_client_id (= the parent's client_id).
	var carried bool
	for _, tk := range snap.Tasks {
		if tk.ClientID == child.ClientID {
			if tk.ParentClientID != parent.ClientID {
				t.Fatalf("child parent_client_id: got %q, want %q (parent link dropped in build)", tk.ParentClientID, parent.ClientID)
			}
			carried = true
		}
		if tk.ClientID == parent.ClientID && tk.ParentClientID != "" {
			t.Errorf("top-level parent carried a parent_client_id: %q", tk.ParentClientID)
		}
	}
	if !carried {
		t.Fatalf("child task not present in snapshot")
	}

	var buf bytes.Buffer
	w := bufio.NewWriter(&buf)
	if err := WriteNDJSON(w, snap); err != nil {
		t.Fatalf("write ndjson: %v", err)
	}
	_ = w.Flush()

	// Joiner applies it; the child must land as a subtask of the local parent.
	jd := openMigrated(t)
	joiner := newOwnerDeps(jd)
	res, err := Apply(ctx, ApplyDeps{
		DB:          jd,
		Projects:    joiner.projects,
		Sections:    joiner.sections,
		Tasks:       joiner.tasks,
		Contexts:    joiner.contexts,
		FedProjects: joiner.fedProjects,
	}, ApplyParams{
		OwnerInstanceURL: "https://alice.example",
		Reader:           bytes.NewReader(buf.Bytes()),
	})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}

	// Resolve the local parent + child by client_id and assert the parent_id link.
	var localParentID, localChildID int64
	var childParentID sql.NullInt64
	if err := jd.QueryRow(`SELECT id FROM tasks WHERE client_id = ?`, parent.ClientID).Scan(&localParentID); err != nil {
		t.Fatalf("load local parent: %v", err)
	}
	if err := jd.QueryRow(`SELECT id, parent_id FROM tasks WHERE client_id = ?`, child.ClientID).Scan(&localChildID, &childParentID); err != nil {
		t.Fatalf("load local child: %v", err)
	}
	if !childParentID.Valid {
		t.Fatalf("subtask flattened: local child has NULL parent_id after bootstrap")
	}
	if childParentID.Int64 != localParentID {
		t.Errorf("subtask parent_id: got %d, want %d (local parent)", childParentID.Int64, localParentID)
	}
	_ = res
}

func splitLines(s string) []string {
	out := make([]string, 0)
	for _, ln := range strings.Split(strings.TrimRight(s, "\n"), "\n") {
		if strings.TrimSpace(ln) != "" {
			out = append(out, ln)
		}
	}
	return out
}

// ownerDeps groups the repos a snapshot build/consume needs.
type ownerDeps struct {
	db             *sql.DB
	contexts       *repo.ContextRepo
	projects       *repo.ProjectRepo
	sections       *repo.ProjectSectionRepo
	tasks          *repo.TaskRepo
	fedProjects    *repo.FederatedProjectRepo
	firstContextID int64
}

func newOwnerDeps(d *sql.DB) *ownerDeps {
	plabels := repo.NewProjectLabelsRepo(d)
	tlabels := repo.NewTaskLabelsRepo(d)
	return &ownerDeps{
		db:          d,
		contexts:    repo.NewContextRepo(d),
		projects:    repo.NewProjectRepo(d, plabels),
		sections:    repo.NewProjectSectionRepo(d),
		tasks:       repo.NewTaskRepo(d, tlabels),
		fedProjects: repo.NewFederatedProjectRepo(d),
	}
}

func openMigrated(t *testing.T) *sql.DB {
	t.Helper()
	dir := t.TempDir()
	d, err := db.Open(filepath.Join(dir, "snap.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	if err := db.RunMigrations(context.Background(), d); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return d
}

// model import kept referenced for clarity in assertions on entity shapes.
var _ = model.FederationPermissionRead
