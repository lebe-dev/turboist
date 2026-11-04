package repo

import (
	"context"
	"testing"
)

func TestLabelRepo_Description_CreateUpdateList(t *testing.T) {
	d := setupTestDB(t)
	r := NewLabelRepo(d)
	ctx := context.Background()

	l, err := r.CreateDescribed(ctx, "deep", "blue", "Needs two focused hours", false)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if l.Description != "Needs two focused hours" {
		t.Fatalf("description after create: got %q", l.Description)
	}

	plain, err := r.Create(ctx, "plain", "red", false)
	if err != nil {
		t.Fatalf("create plain: %v", err)
	}
	if plain.Description != "" {
		t.Fatalf("plain description: got %q, want empty", plain.Description)
	}

	desc := "Requires a quiet room"
	if _, err := r.Update(ctx, l.ID, LabelUpdate{Description: &desc}); err != nil {
		t.Fatalf("update: %v", err)
	}
	// An update that leaves Description nil keeps the stored text.
	name := "deep-work"
	got, err := r.Update(ctx, l.ID, LabelUpdate{Name: &name})
	if err != nil {
		t.Fatalf("rename: %v", err)
	}
	if got.Description != desc {
		t.Fatalf("description after rename: got %q, want %q", got.Description, desc)
	}

	items, _, err := r.List(ctx, LabelListFilter{}, Page{Limit: 10})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	found := false
	for _, it := range items {
		if it.ID == l.ID {
			found = it.Description == desc
		}
	}
	if !found {
		t.Fatalf("list did not carry the description: %+v", items)
	}
}

func TestTaskLabelsRepo_HydrateCarriesDescription(t *testing.T) {
	f := newTaskFixture(t)
	ctx := context.Background()

	l, err := f.labels.CreateDescribed(ctx, "agent", "blue", "Safe for an AI agent", false)
	if err != nil {
		t.Fatalf("create label: %v", err)
	}
	task, err := f.tasks.Create(ctx, CreateTask{Placement: Placement{ContextID: &f.contextID}, Title: "t"})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := f.tlabels.SetForTask(ctx, task.ID, []int64{l.ID}); err != nil {
		t.Fatalf("set labels: %v", err)
	}
	got, err := f.tasks.Get(ctx, task.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(got.Labels) != 1 || got.Labels[0].Description != "Safe for an AI agent" {
		t.Fatalf("task labels: %+v", got.Labels)
	}
}
