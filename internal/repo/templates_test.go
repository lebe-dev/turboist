package repo

import (
	"context"
	"errors"
	"testing"

	"github.com/lebe-dev/turboist/internal/model"
)

func seedLabel(t *testing.T, r *LabelRepo, name string) int64 {
	t.Helper()
	l, err := r.Create(context.Background(), name, "blue", false)
	if err != nil {
		t.Fatalf("seed label %q: %v", name, err)
	}
	return l.ID
}

func TestTemplateRepo_CreateGet(t *testing.T) {
	d := setupTestDB(t)
	r := NewTemplateRepo(d)
	lr := NewLabelRepo(d)
	ctx := context.Background()

	l1 := seedLabel(t, lr, "work")
	l2 := seedLabel(t, lr, "urgent")

	tmpl, err := r.Create(ctx, TemplateInput{
		Name:        "Onboard client",
		Description: "root desc",
		Priority:    model.PriorityHigh,
		DayPart:     model.DayPartMorning,
		LabelIDs:    []int64{l1},
		Subtasks: []TemplateSubtaskInput{
			{Title: "Kickoff call", Priority: model.PriorityMedium, LabelIDs: []int64{l2}},
			{Title: "Send contract"},
		},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := r.Get(ctx, tmpl.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Name != "Onboard client" || got.Priority != model.PriorityHigh || got.DayPart != model.DayPartMorning {
		t.Errorf("root fields: got %+v", got)
	}
	if len(got.Labels) != 1 || got.Labels[0].ID != l1 {
		t.Errorf("root labels: got %+v, want [%d]", got.Labels, l1)
	}
	if len(got.Subtasks) != 2 {
		t.Fatalf("subtasks: got %d, want 2", len(got.Subtasks))
	}
	if got.Subtasks[0].Title != "Kickoff call" || got.Subtasks[0].Position != 0 {
		t.Errorf("subtask0: got %+v", got.Subtasks[0])
	}
	if len(got.Subtasks[0].Labels) != 1 || got.Subtasks[0].Labels[0].ID != l2 {
		t.Errorf("subtask0 labels: got %+v, want [%d]", got.Subtasks[0].Labels, l2)
	}
	if got.Subtasks[1].Title != "Send contract" || got.Subtasks[1].Position != 1 {
		t.Errorf("subtask1: got %+v", got.Subtasks[1])
	}
}

func TestTemplateRepo_UpdateFullReplace(t *testing.T) {
	d := setupTestDB(t)
	r := NewTemplateRepo(d)
	lr := NewLabelRepo(d)
	ctx := context.Background()

	l1 := seedLabel(t, lr, "a")
	tmpl, err := r.Create(ctx, TemplateInput{
		Name:     "v1",
		LabelIDs: []int64{l1},
		Subtasks: []TemplateSubtaskInput{{Title: "old1"}, {Title: "old2"}},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	updated, err := r.Update(ctx, tmpl.ID, TemplateInput{
		Name:     "v2",
		Subtasks: []TemplateSubtaskInput{{Title: "new1"}},
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Name != "v2" {
		t.Errorf("name: got %q, want v2", updated.Name)
	}
	if len(updated.Labels) != 0 {
		t.Errorf("labels after replace: got %d, want 0", len(updated.Labels))
	}
	if len(updated.Subtasks) != 1 || updated.Subtasks[0].Title != "new1" {
		t.Errorf("subtasks after replace: got %+v, want [new1]", updated.Subtasks)
	}
}

func TestTemplateRepo_Update_NotFound(t *testing.T) {
	d := setupTestDB(t)
	r := NewTemplateRepo(d)
	_, err := r.Update(context.Background(), 999, TemplateInput{Name: "x"})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestTemplateRepo_DeleteCascade(t *testing.T) {
	d := setupTestDB(t)
	r := NewTemplateRepo(d)
	lr := NewLabelRepo(d)
	ctx := context.Background()

	l1 := seedLabel(t, lr, "x")
	tmpl, err := r.Create(ctx, TemplateInput{
		Name:     "t",
		LabelIDs: []int64{l1},
		Subtasks: []TemplateSubtaskInput{{Title: "s", LabelIDs: []int64{l1}}},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := r.Delete(ctx, tmpl.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := r.Get(ctx, tmpl.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
	// Subtasks and label links cascade away.
	var n int
	if err := d.QueryRow(`SELECT COUNT(*) FROM task_template_subtasks`).Scan(&n); err != nil {
		t.Fatalf("count subtasks: %v", err)
	}
	if n != 0 {
		t.Errorf("orphan subtasks: got %d, want 0", n)
	}
}

func TestTemplateRepo_LabelDeleteCascades(t *testing.T) {
	d := setupTestDB(t)
	r := NewTemplateRepo(d)
	lr := NewLabelRepo(d)
	ctx := context.Background()

	l1 := seedLabel(t, lr, "tmp")
	tmpl, err := r.Create(ctx, TemplateInput{Name: "t", LabelIDs: []int64{l1}})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := lr.Delete(ctx, l1); err != nil {
		t.Fatalf("delete label: %v", err)
	}
	got, err := r.Get(ctx, tmpl.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(got.Labels) != 0 {
		t.Errorf("labels after label delete: got %d, want 0", len(got.Labels))
	}
}

func TestTemplateRepo_ListOrdered(t *testing.T) {
	d := setupTestDB(t)
	r := NewTemplateRepo(d)
	ctx := context.Background()

	if _, err := r.Create(ctx, TemplateInput{Name: "first"}); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := r.Create(ctx, TemplateInput{Name: "second"}); err != nil {
		t.Fatalf("create: %v", err)
	}
	items, err := r.List(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("list len: got %d, want 2", len(items))
	}
	if items[0].Name != "first" || items[1].Name != "second" {
		t.Errorf("order: got %q, %q", items[0].Name, items[1].Name)
	}
}
