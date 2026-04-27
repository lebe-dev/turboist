package repo

import (
	"context"
	"errors"
	"testing"
)

func TestContextRepo_CreateAndGet(t *testing.T) {
	d := setupTestDB(t)
	r := NewContextRepo(d)
	ctx := context.Background()

	c, err := r.Create(ctx, "work", "blue", true)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if c.ID == 0 {
		t.Fatalf("expected ID assigned")
	}
	if c.Name != "work" || c.Color != "blue" || !c.IsFavourite {
		t.Errorf("got %+v", c)
	}

	got, err := r.Get(ctx, c.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.ID != c.ID {
		t.Errorf("id: got %d, want %d", got.ID, c.ID)
	}
}

func TestContextRepo_Create_UniqueConflict(t *testing.T) {
	d := setupTestDB(t)
	r := NewContextRepo(d)
	ctx := context.Background()

	if _, err := r.Create(ctx, "work", "blue", false); err != nil {
		t.Fatalf("first: %v", err)
	}
	_, err := r.Create(ctx, "work", "red", false)
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("expected ErrConflict, got %v", err)
	}
}

func TestContextRepo_Get_NotFound(t *testing.T) {
	d := setupTestDB(t)
	r := NewContextRepo(d)
	_, err := r.Get(context.Background(), 9999)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestContextRepo_List_PaginationBoundaries(t *testing.T) {
	d := setupTestDB(t)
	r := NewContextRepo(d)
	ctx := context.Background()

	for _, name := range []string{"a", "b", "c", "d", "e"} {
		if _, err := r.Create(ctx, name, "blue", false); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	items, total, err := r.List(ctx, Page{Limit: 2, Offset: 0})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 5 {
		t.Errorf("total: got %d, want 5", total)
	}
	if len(items) != 2 {
		t.Errorf("len: got %d, want 2", len(items))
	}

	items, _, err = r.List(ctx, Page{Limit: 10, Offset: 4})
	if err != nil {
		t.Fatalf("list offset: %v", err)
	}
	if len(items) != 1 {
		t.Errorf("offset len: got %d, want 1", len(items))
	}

	// negative offset normalised
	items, _, err = r.List(ctx, Page{Limit: 0, Offset: -1})
	if err != nil {
		t.Fatalf("list defaults: %v", err)
	}
	if len(items) != 5 {
		t.Errorf("defaults len: got %d, want 5", len(items))
	}
}

func TestContextRepo_Update(t *testing.T) {
	d := setupTestDB(t)
	r := NewContextRepo(d)
	ctx := context.Background()

	c, err := r.Create(ctx, "work", "blue", false)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	updated, err := r.Update(ctx, c.ID, ContextUpdate{Color: ptr("red"), IsFavourite: ptr(true)})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Color != "red" || !updated.IsFavourite || updated.Name != "work" {
		t.Errorf("got %+v", updated)
	}
}

func TestContextRepo_Update_UniqueConflict(t *testing.T) {
	d := setupTestDB(t)
	r := NewContextRepo(d)
	ctx := context.Background()

	if _, err := r.Create(ctx, "work", "blue", false); err != nil {
		t.Fatalf("seed1: %v", err)
	}
	c2, err := r.Create(ctx, "home", "green", false)
	if err != nil {
		t.Fatalf("seed2: %v", err)
	}
	_, err = r.Update(ctx, c2.ID, ContextUpdate{Name: ptr("work")})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("expected ErrConflict, got %v", err)
	}
}

func TestContextRepo_Delete_NotFound(t *testing.T) {
	d := setupTestDB(t)
	r := NewContextRepo(d)
	if err := r.Delete(context.Background(), 9999); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestContextRepo_Delete_CascadesProjects(t *testing.T) {
	d := setupTestDB(t)
	r := NewContextRepo(d)
	ctx := context.Background()

	c, err := r.Create(ctx, "work", "blue", false)
	if err != nil {
		t.Fatalf("create context: %v", err)
	}
	pr := NewProjectRepo(d, NewProjectLabelsRepo(d))
	if _, err := pr.Create(ctx, CreateProject{ContextID: c.ID, Title: "p", Color: "blue"}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	if err := r.Delete(ctx, c.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, total, err := pr.List(ctx, ProjectListFilter{}, Page{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 0 {
		t.Errorf("expected cascade, got total=%d", total)
	}
}
