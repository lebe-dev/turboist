package repo

import (
	"context"
	"errors"
	"testing"
)

func setupSectionsFixture(t *testing.T) (*ProjectSectionRepo, int64) {
	t.Helper()
	d := setupTestDB(t)
	cr := NewContextRepo(d)
	pr := NewProjectRepo(d, NewProjectLabelsRepo(d))
	sr := NewProjectSectionRepo(d)
	ctx := context.Background()
	c, err := cr.Create(ctx, "work", "blue", false)
	if err != nil {
		t.Fatalf("create context: %v", err)
	}
	p, err := pr.Create(ctx, CreateProject{ContextID: c.ID, Title: "a", Color: "blue"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	return sr, p.ID
}

func TestSectionRepo_CRUD(t *testing.T) {
	sr, projectID := setupSectionsFixture(t)
	ctx := context.Background()

	s, err := sr.Create(ctx, projectID, "todo")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	got, err := sr.Get(ctx, s.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Title != "todo" {
		t.Errorf("title: got %q, want todo", got.Title)
	}

	updated, err := sr.Update(ctx, s.ID, SectionUpdate{Title: ptr("doing")})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Title != "doing" {
		t.Errorf("update title: got %q, want doing", updated.Title)
	}

	if err := sr.Delete(ctx, s.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := sr.Get(ctx, s.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestSectionRepo_ListByProject_Pagination(t *testing.T) {
	sr, projectID := setupSectionsFixture(t)
	ctx := context.Background()

	for _, title := range []string{"a", "b", "c", "d"} {
		if _, err := sr.Create(ctx, projectID, title); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	items, total, err := sr.ListByProject(ctx, projectID, Page{Limit: 2, Offset: 1})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 4 {
		t.Errorf("total: got %d, want 4", total)
	}
	if len(items) != 2 {
		t.Errorf("len: got %d, want 2", len(items))
	}
}

func TestSectionRepo_Delete_NotFound(t *testing.T) {
	sr, _ := setupSectionsFixture(t)
	if err := sr.Delete(context.Background(), 9999); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
