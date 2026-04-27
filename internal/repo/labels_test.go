package repo

import (
	"context"
	"errors"
	"testing"
)

func TestLabelRepo_CreateGetByName(t *testing.T) {
	d := setupTestDB(t)
	r := NewLabelRepo(d)
	ctx := context.Background()

	l, err := r.Create(ctx, "urgent", "red", true)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	got, err := r.GetByName(ctx, "urgent")
	if err != nil {
		t.Fatalf("get by name: %v", err)
	}
	if got.ID != l.ID {
		t.Errorf("id: got %d, want %d", got.ID, l.ID)
	}
}

func TestLabelRepo_Create_UniqueConflict(t *testing.T) {
	d := setupTestDB(t)
	r := NewLabelRepo(d)
	ctx := context.Background()

	if _, err := r.Create(ctx, "urgent", "red", false); err != nil {
		t.Fatalf("seed: %v", err)
	}
	_, err := r.Create(ctx, "urgent", "blue", false)
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("expected ErrConflict, got %v", err)
	}
}

func TestLabelRepo_List_FilterAndPaginate(t *testing.T) {
	d := setupTestDB(t)
	r := NewLabelRepo(d)
	ctx := context.Background()

	names := []string{"work", "work-deep", "home", "errand"}
	for _, n := range names {
		if _, err := r.Create(ctx, n, "blue", false); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	items, total, err := r.List(ctx, LabelListFilter{Query: "work"}, Page{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 2 || len(items) != 2 {
		t.Errorf("got total=%d len=%d, want 2/2", total, len(items))
	}

	items, total, err = r.List(ctx, LabelListFilter{}, Page{Limit: 2, Offset: 2})
	if err != nil {
		t.Fatalf("paginate: %v", err)
	}
	if total != 4 {
		t.Errorf("total: got %d, want 4", total)
	}
	if len(items) != 2 {
		t.Errorf("page len: got %d, want 2", len(items))
	}
}

func TestLabelRepo_Update_AndDelete(t *testing.T) {
	d := setupTestDB(t)
	r := NewLabelRepo(d)
	ctx := context.Background()

	l, err := r.Create(ctx, "x", "blue", false)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	updated, err := r.Update(ctx, l.ID, LabelUpdate{Name: ptr("y")})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Name != "y" {
		t.Errorf("name: got %s, want y", updated.Name)
	}
	if err := r.Delete(ctx, l.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := r.Get(ctx, l.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestLabelRepo_GetByIDs(t *testing.T) {
	d := setupTestDB(t)
	r := NewLabelRepo(d)
	ctx := context.Background()

	a, _ := r.Create(ctx, "a", "blue", false)
	b, _ := r.Create(ctx, "b", "red", false)

	got, err := r.GetByIDs(ctx, []int64{a.ID, b.ID, 9999})
	if err != nil {
		t.Fatalf("get by ids: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("len: got %d, want 2", len(got))
	}
}
