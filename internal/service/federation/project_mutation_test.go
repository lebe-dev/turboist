package federation_test

import (
	"context"
	"testing"

	"github.com/lebe-dev/turboist/internal/federation/events"
	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
	fedsvc "github.com/lebe-dev/turboist/internal/service/federation"
)

// projectClientID reads a project's cross-instance client_id.
func projectClientID(t *testing.T, env *emitEnv, id int64) string {
	t.Helper()
	var c string
	if err := env.db.QueryRow(`SELECT client_id FROM projects WHERE id = ?`, id).Scan(&c); err != nil {
		t.Fatalf("project client_id: %v", err)
	}
	return c
}

func newProjectRepoFor(env *emitEnv) *repo.ProjectRepo {
	return repo.NewProjectRepo(env.db, repo.NewProjectLabelsRepo(env.db))
}

// TestProjectMutator_UpdateFederatedEmitsOutbox asserts an Update on a FEDERATED
// project emits a signed op=update event carrying the changed federated fields
// (title/description/color), excluding local-only fields (US-3.2 AC1).
func TestProjectMutator_UpdateFederatedEmitsOutbox(t *testing.T) {
	env := newEmitEnv(t)
	ctx := context.Background()
	projects := newProjectRepoFor(env)
	mut := fedsvc.NewProjectMutator(env.emitter, projects)

	clientID := projectClientID(t, env, env.fedProject)
	newTitle := "Renamed project"
	newColor := "green"
	if err := mut.Update(ctx, env.fedProject, repo.ProjectUpdate{Title: &newTitle, Color: &newColor}); err != nil {
		t.Fatalf("update: %v", err)
	}

	got, err := projects.Get(ctx, env.fedProject)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Title != newTitle || got.Color != newColor {
		t.Errorf("domain write: got title=%q color=%q", got.Title, got.Color)
	}

	evts := outboxEvents(t, env, env.fedProject)
	if len(evts) != 1 {
		t.Fatalf("outbox count: got %d, want 1", len(evts))
	}
	e := evts[0]
	if e.Op != events.OpUpdate {
		t.Errorf("op: got %q, want update", e.Op)
	}
	if e.EntityType != events.EntityProject {
		t.Errorf("entity_type: got %q, want project", e.EntityType)
	}
	if e.EntityID != clientID {
		t.Errorf("entity_id: got %q, want %q", e.EntityID, clientID)
	}
	if e.Signature == "" {
		t.Errorf("event must be signed")
	}
	if f, ok := e.Fields["title"]; !ok || f.Value != newTitle {
		t.Errorf("title field: got %+v", e.Fields["title"])
	}
	if f, ok := e.Fields["color"]; !ok || f.Value != newColor {
		t.Errorf("color field: got %+v", e.Fields["color"])
	}
	for _, banned := range []string{"project_type", "is_private", "troiki_category", "context_id"} {
		if _, ok := e.Fields[banned]; ok {
			t.Errorf("local-only field %q must not be emitted", banned)
		}
	}
}

func TestProjectMutator_UpdateNonFederatedNoOutbox(t *testing.T) {
	env := newEmitEnv(t)
	ctx := context.Background()
	projects := newProjectRepoFor(env)
	mut := fedsvc.NewProjectMutator(env.emitter, projects)

	newTitle := "plain rename"
	if err := mut.Update(ctx, env.plainProj, repo.ProjectUpdate{Title: &newTitle}); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, err := projects.Get(ctx, env.plainProj)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Title != newTitle {
		t.Errorf("domain write must still happen: got %q", got.Title)
	}
	if n := outboxCount(t, env.db, env.plainProj); n != 0 {
		t.Errorf("non-federated update outbox: got %d, want 0", n)
	}
}

// TestProjectMutator_UpdateStatusFederatedEmitsStatus asserts a project status
// change (archive/complete/cancel/open) on a FEDERATED project emits a signed
// op=update event carrying ONLY the status field, and the domain write (status,
// troiki_category clear) ran (US-3.2 AC1, TASK B).
func TestProjectMutator_UpdateStatusFederatedEmitsStatus(t *testing.T) {
	env := newEmitEnv(t)
	ctx := context.Background()
	projects := newProjectRepoFor(env)
	mut := fedsvc.NewProjectMutator(env.emitter, projects)

	clientID := projectClientID(t, env, env.fedProject)
	if err := mut.UpdateStatus(ctx, env.fedProject, model.ProjectStatusArchived); err != nil {
		t.Fatalf("update status: %v", err)
	}

	got, err := projects.Get(ctx, env.fedProject)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Status != model.ProjectStatusArchived {
		t.Errorf("domain write: got status=%q, want archived", got.Status)
	}

	evts := outboxEvents(t, env, env.fedProject)
	if len(evts) != 1 {
		t.Fatalf("outbox count: got %d, want 1", len(evts))
	}
	e := evts[0]
	if e.Op != events.OpUpdate {
		t.Errorf("op: got %q, want update", e.Op)
	}
	if e.EntityType != events.EntityProject {
		t.Errorf("entity_type: got %q, want project", e.EntityType)
	}
	if e.EntityID != clientID {
		t.Errorf("entity_id: got %q, want %q", e.EntityID, clientID)
	}
	if e.Signature == "" {
		t.Errorf("event must be signed")
	}
	if f, ok := e.Fields["status"]; !ok || f.Value != "archived" {
		t.Errorf("status field: got %+v", e.Fields["status"])
	}
	if len(e.Fields) != 1 {
		t.Errorf("status update must carry only the status field: got %d (%v)", len(e.Fields), e.Fields)
	}
	// troiki_category is local-only and must never be emitted even though the
	// status change clears it on the local row.
	if _, ok := e.Fields["troiki_category"]; ok {
		t.Errorf("local-only troiki_category must not be emitted")
	}
}

func TestProjectMutator_UpdateStatusNonFederatedNoOutbox(t *testing.T) {
	env := newEmitEnv(t)
	ctx := context.Background()
	projects := newProjectRepoFor(env)
	mut := fedsvc.NewProjectMutator(env.emitter, projects)

	if err := mut.UpdateStatus(ctx, env.plainProj, model.ProjectStatusCompleted); err != nil {
		t.Fatalf("update status: %v", err)
	}
	got, err := projects.Get(ctx, env.plainProj)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Status != model.ProjectStatusCompleted {
		t.Errorf("domain write must still happen: got status=%q", got.Status)
	}
	if n := outboxCount(t, env.db, env.plainProj); n != 0 {
		t.Errorf("non-federated status outbox: got %d, want 0", n)
	}
}

// TestProjectMutator_DeleteFederatedEmitsDelete asserts deleting a FEDERATED
// project emits a signed op=delete event for the project entity, and the domain
// soft-delete (project + cascade) ran.
func TestProjectMutator_DeleteFederatedEmitsDelete(t *testing.T) {
	env := newEmitEnv(t)
	ctx := context.Background()
	projects := newProjectRepoFor(env)
	mut := fedsvc.NewProjectMutator(env.emitter, projects)

	clientID := projectClientID(t, env, env.fedProject)
	if err := mut.Delete(ctx, env.fedProject); err != nil {
		t.Fatalf("delete: %v", err)
	}

	if _, err := projects.Get(ctx, env.fedProject); err == nil {
		t.Errorf("federated project must be soft-deleted")
	}

	evts := outboxEvents(t, env, env.fedProject)
	if len(evts) != 1 {
		t.Fatalf("outbox count: got %d, want 1", len(evts))
	}
	e := evts[0]
	if e.Op != events.OpDelete {
		t.Errorf("op: got %q, want delete", e.Op)
	}
	if e.EntityType != events.EntityProject {
		t.Errorf("entity_type: got %q, want project", e.EntityType)
	}
	if e.EntityID != clientID {
		t.Errorf("entity_id: got %q, want %q", e.EntityID, clientID)
	}
	if f, ok := e.Fields[events.FieldDeleted]; !ok || f.HLC == "" {
		t.Errorf("delete event must carry _deleted HLC: got %+v", e.Fields)
	}
}

func TestProjectMutator_DeleteNonFederatedNoOutbox(t *testing.T) {
	env := newEmitEnv(t)
	ctx := context.Background()
	projects := newProjectRepoFor(env)
	mut := fedsvc.NewProjectMutator(env.emitter, projects)

	if err := mut.Delete(ctx, env.plainProj); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := projects.Get(ctx, env.plainProj); err == nil {
		t.Errorf("plain project must be soft-deleted")
	}
	if n := outboxCount(t, env.db, env.plainProj); n != 0 {
		t.Errorf("non-federated delete outbox: got %d, want 0", n)
	}
}
