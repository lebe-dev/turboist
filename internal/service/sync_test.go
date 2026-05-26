package service_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/lebe-dev/turboist/internal/model"
	"github.com/lebe-dev/turboist/internal/repo"
	"github.com/lebe-dev/turboist/internal/service"
)

type syncFixtures struct {
	db       *sql.DB
	svc      *service.SyncService
	tasks    *repo.TaskRepo
	projects *repo.ProjectRepo
	sections *repo.ProjectSectionRepo
	labels   *repo.LabelRepo
	contexts *repo.ContextRepo
}

func setupSyncService(t *testing.T) *syncFixtures {
	t.Helper()
	d := setupTestDB(t)
	tlabels := repo.NewTaskLabelsRepo(d)
	plabels := repo.NewProjectLabelsRepo(d)
	tasks := repo.NewTaskRepo(d, tlabels)
	projects := repo.NewProjectRepo(d, plabels)
	sections := repo.NewProjectSectionRepo(d)
	labels := repo.NewLabelRepo(d)
	contexts := repo.NewContextRepo(d)
	syncRepo := repo.NewSyncRepo(d, tlabels, plabels)
	return &syncFixtures{
		db:       d,
		svc:      service.NewSyncService(syncRepo),
		tasks:    tasks,
		projects: projects,
		sections: sections,
		labels:   labels,
		contexts: contexts,
	}
}

func syncPtr[T any](v T) *T { return &v }

func TestSyncService_Pull_Initial_BundleHasAllEntities(t *testing.T) {
	f := setupSyncService(t)
	ctx := context.Background()

	c, err := f.contexts.Create(ctx, "work", "blue", false)
	if err != nil {
		t.Fatalf("context: %v", err)
	}
	p, err := f.projects.Create(ctx, repo.CreateProject{ContextID: c.ID, Title: "alpha", Color: "blue"})
	if err != nil {
		t.Fatalf("project: %v", err)
	}
	if _, err := f.sections.Create(ctx, p.ID, "todo"); err != nil {
		t.Fatalf("section: %v", err)
	}
	if _, err := f.labels.Create(ctx, "urgent", "red", false); err != nil {
		t.Fatalf("label: %v", err)
	}
	if _, err := f.tasks.Create(ctx, repo.CreateTask{
		Placement: repo.Placement{ContextID: &c.ID},
		Title:     "buy milk",
	}); err != nil {
		t.Fatalf("task: %v", err)
	}

	before := time.Now().UTC()
	bundle, err := f.svc.Pull(ctx, nil)
	after := time.Now().UTC()
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}

	if bundle.Now.Before(before) || bundle.Now.After(after) {
		t.Errorf("bundle.Now out of range: got %v, want in [%v, %v]", bundle.Now, before, after)
	}
	if len(bundle.Tasks) != 1 {
		t.Errorf("tasks: got %d, want 1", len(bundle.Tasks))
	}
	if len(bundle.Projects) != 1 {
		t.Errorf("projects: got %d, want 1", len(bundle.Projects))
	}
	if len(bundle.Sections) != 1 {
		t.Errorf("sections: got %d, want 1", len(bundle.Sections))
	}
	if len(bundle.Labels) != 1 {
		t.Errorf("labels: got %d, want 1", len(bundle.Labels))
	}
	if len(bundle.Contexts) != 1 {
		t.Errorf("contexts: got %d, want 1", len(bundle.Contexts))
	}
}

func TestSyncService_Pull_Initial_AppliesCompletedCutoff(t *testing.T) {
	f := setupSyncService(t)
	ctx := context.Background()
	c, err := f.contexts.Create(ctx, "work", "blue", false)
	if err != nil {
		t.Fatalf("context: %v", err)
	}

	recent, err := f.tasks.Create(ctx, repo.CreateTask{
		Placement: repo.Placement{ContextID: &c.ID},
		Title:     "recent",
	})
	if err != nil {
		t.Fatalf("recent: %v", err)
	}
	old, err := f.tasks.Create(ctx, repo.CreateTask{
		Placement: repo.Placement{ContextID: &c.ID},
		Title:     "old",
	})
	if err != nil {
		t.Fatalf("old: %v", err)
	}

	now := time.Now().UTC()
	if _, err := f.tasks.Update(ctx, recent.ID, repo.TaskUpdate{
		Status:      syncPtr(model.TaskStatusCompleted),
		CompletedAt: syncPtr(now.Add(-7 * 24 * time.Hour)),
	}); err != nil {
		t.Fatalf("complete recent: %v", err)
	}
	if _, err := f.tasks.Update(ctx, old.ID, repo.TaskUpdate{
		Status:      syncPtr(model.TaskStatusCompleted),
		CompletedAt: syncPtr(now.Add(-100 * 24 * time.Hour)),
	}); err != nil {
		t.Fatalf("complete old: %v", err)
	}

	bundle, err := f.svc.Pull(ctx, nil)
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}
	ids := map[int64]bool{}
	for _, tk := range bundle.Tasks {
		ids[tk.ID] = true
	}
	if !ids[recent.ID] {
		t.Errorf("recent (within 30d) must be in bundle: id=%d", recent.ID)
	}
	if ids[old.ID] {
		t.Errorf("old (outside 30d) must be excluded: id=%d", old.ID)
	}
}

func TestSyncService_Pull_Incremental_FiltersBySince(t *testing.T) {
	f := setupSyncService(t)
	ctx := context.Background()

	since := time.Now().UTC().Add(-1 * time.Hour)
	c, err := f.contexts.Create(ctx, "work", "blue", false)
	if err != nil {
		t.Fatalf("context: %v", err)
	}
	if _, err := f.db.ExecContext(ctx,
		`UPDATE contexts SET updated_at = ? WHERE id = ?`,
		model.FormatUTC(since.Add(-2*time.Hour)), c.ID); err != nil {
		t.Fatalf("backdate ctx: %v", err)
	}

	bundle, err := f.svc.Pull(ctx, &since)
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}
	for _, ctxRow := range bundle.Contexts {
		if ctxRow.ID == c.ID {
			t.Errorf("backdated context must be excluded from incremental: id=%d", ctxRow.ID)
		}
	}
}

func TestSyncService_Pull_BundleNowIsUTC(t *testing.T) {
	f := setupSyncService(t)
	bundle, err := f.svc.Pull(context.Background(), nil)
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}
	if bundle.Now.Location() != time.UTC {
		t.Errorf("bundle.Now must be UTC, got %v", bundle.Now.Location())
	}
}

func TestSyncService_Pull_EmptyDB_ReturnsEmptyBundle(t *testing.T) {
	f := setupSyncService(t)
	bundle, err := f.svc.Pull(context.Background(), nil)
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}
	if len(bundle.Tasks) != 0 || len(bundle.Projects) != 0 || len(bundle.Sections) != 0 ||
		len(bundle.Labels) != 0 || len(bundle.Contexts) != 0 {
		t.Errorf("expected empty bundle, got tasks=%d projects=%d sections=%d labels=%d contexts=%d",
			len(bundle.Tasks), len(bundle.Projects), len(bundle.Sections), len(bundle.Labels), len(bundle.Contexts))
	}
}
