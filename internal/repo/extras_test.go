package repo

import (
	"context"
	"testing"
	"time"

	"github.com/lebe-dev/turboist/internal/model"
)

func TestLabelRepo_Update_AllFields(t *testing.T) {
	d := setupTestDB(t)
	r := NewLabelRepo(d)
	ctx := context.Background()

	l, err := r.Create(ctx, "old", "blue", false)
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	newName := "new"
	newColor := "red"
	fav := true
	got, err := r.Update(ctx, l.ID, LabelUpdate{Name: &newName, Color: &newColor, IsFavourite: &fav})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if got.Name != "new" || got.Color != "red" || !got.IsFavourite {
		t.Errorf("got %+v", got)
	}

	// noop update
	got2, err := r.Update(ctx, l.ID, LabelUpdate{})
	if err != nil {
		t.Fatalf("noop: %v", err)
	}
	if got2.Name != "new" {
		t.Errorf("noop changed: %+v", got2)
	}

	// missing
	if _, err := r.Update(ctx, 99999, LabelUpdate{Name: &newName}); err == nil {
		t.Error("expected error for missing label")
	}
}

func TestSectionRepo_Update(t *testing.T) {
	f := newTaskFixture(t)
	ctx := context.Background()

	newTitle := "renamed"
	got, err := f.sections.Update(ctx, f.sectionID, SectionUpdate{Title: &newTitle})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if got.Title != "renamed" {
		t.Errorf("title: got %q", got.Title)
	}

	// noop
	got2, err := f.sections.Update(ctx, f.sectionID, SectionUpdate{})
	if err != nil {
		t.Fatalf("noop: %v", err)
	}
	if got2.Title != "renamed" {
		t.Errorf("noop changed: %+v", got2)
	}

	// missing
	if _, err := f.sections.Update(ctx, 99999, SectionUpdate{Title: &newTitle}); err == nil {
		t.Error("expected error for missing section")
	}
}

func TestSessionRepo_Rotate_AndRevoke_AndList(t *testing.T) {
	d := setupTestDB(t)
	users := NewUserRepo(d)
	sessions := NewSessionRepo(d)
	ctx := context.Background()

	u, _ := users.Create(ctx, "u@test", "h")
	s, err := sessions.Create(ctx, CreateSessionParams{
		UserID:     u.ID,
		TokenHash:  "h1",
		ClientKind: model.ClientWeb,
		ExpiresAt:  time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	if err := sessions.Rotate(ctx, s.ID, "h2", time.Now().Add(2*time.Hour)); err != nil {
		t.Fatalf("rotate: %v", err)
	}

	active, err := sessions.ListActiveForUser(ctx, u.ID)
	if err != nil {
		t.Fatalf("list active: %v", err)
	}
	if len(active) != 1 {
		t.Errorf("active: got %d, want 1", len(active))
	}

	if err := sessions.Revoke(ctx, s.ID); err != nil {
		t.Fatalf("revoke: %v", err)
	}

	active2, _ := sessions.ListActiveForUser(ctx, u.ID)
	if len(active2) != 0 {
		t.Errorf("after revoke: got %d, want 0", len(active2))
	}
}

func TestSessionRepo_EnforceLimit(t *testing.T) {
	d := setupTestDB(t)
	users := NewUserRepo(d)
	sessions := NewSessionRepo(d)
	ctx := context.Background()

	u, _ := users.Create(ctx, "u@test", "h")
	for i := range 6 {
		_, err := sessions.Create(ctx, CreateSessionParams{
			UserID:     u.ID,
			TokenHash:  string(rune('a' + i)),
			ClientKind: model.ClientWeb,
			ExpiresAt:  time.Now().Add(time.Hour),
		})
		if err != nil {
			t.Fatalf("create %d: %v", i, err)
		}
		time.Sleep(2 * time.Millisecond)
	}
	if err := sessions.EnforceLimit(ctx, u.ID, model.ClientWeb, 5); err != nil {
		t.Fatalf("enforce: %v", err)
	}
	active, _ := sessions.ListActiveForUser(ctx, u.ID)
	if len(active) > 5 {
		t.Errorf("active after enforce: got %d, want ≤5", len(active))
	}
}
