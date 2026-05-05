package repo

import (
	"context"
	"testing"
)

func TestTaskLabelsRepo_SetAndHydrate(t *testing.T) {
	f := newTaskFixture(t)
	ctx := context.Background()

	l1, _ := f.labels.Create(ctx, "x", "blue", false)
	l2, _ := f.labels.Create(ctx, "y", "red", false)
	task, _ := f.tasks.Create(ctx, CreateTask{
		Placement: Placement{ContextID: &f.contextID},
		Title:     "t",
	})

	if err := f.tlabels.SetForTask(ctx, task.ID, []int64{l1.ID, l2.ID}); err != nil {
		t.Fatalf("set: %v", err)
	}
	got, err := f.tasks.Get(ctx, task.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(got.Labels) != 2 {
		t.Errorf("labels: got %d, want 2", len(got.Labels))
	}

	// replace with single
	if err := f.tlabels.SetForTask(ctx, task.ID, []int64{l1.ID}); err != nil {
		t.Fatalf("re-set: %v", err)
	}
	got, _ = f.tasks.Get(ctx, task.ID)
	if len(got.Labels) != 1 || got.Labels[0].ID != l1.ID {
		t.Errorf("after replace: %+v", got.Labels)
	}
}

func TestTaskLabelsRepo_LabelsByTaskIDs_Empty(t *testing.T) {
	f := newTaskFixture(t)
	out, err := f.tlabels.LabelsByTaskIDs(context.Background(), nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(out) != 0 {
		t.Errorf("expected empty, got %+v", out)
	}
}
