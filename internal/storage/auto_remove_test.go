package storage

import (
	"testing"
	"time"
)

func TestAutoRemoveFirstSeen_UpsertAndGet(t *testing.T) {
	s := newTestStore(t)
	now := time.Now().Truncate(time.Second)

	if err := s.UpsertAutoRemoveFirstSeen("t1", "urgent", now); err != nil {
		t.Fatal(err)
	}

	got, err := s.GetAutoRemoveFirstSeen()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d entries, want 1", len(got))
	}
	if !got["t1:urgent"].Equal(now) {
		t.Errorf("got %v, want %v", got["t1:urgent"], now)
	}
}

func TestAutoRemoveFirstSeen_InsertOrIgnore(t *testing.T) {
	s := newTestStore(t)
	t1 := time.Now().Truncate(time.Second)
	t2 := t1.Add(time.Hour)

	if err := s.UpsertAutoRemoveFirstSeen("t1", "urgent", t1); err != nil {
		t.Fatal(err)
	}
	// Second upsert should not overwrite
	if err := s.UpsertAutoRemoveFirstSeen("t1", "urgent", t2); err != nil {
		t.Fatal(err)
	}

	got, err := s.GetAutoRemoveFirstSeen()
	if err != nil {
		t.Fatal(err)
	}
	if !got["t1:urgent"].Equal(t1) {
		t.Errorf("original timestamp overwritten: got %v, want %v", got["t1:urgent"], t1)
	}
}

func TestAutoRemoveFirstSeen_Delete(t *testing.T) {
	s := newTestStore(t)
	now := time.Now().Truncate(time.Second)

	if err := s.UpsertAutoRemoveFirstSeen("t1", "urgent", now); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteAutoRemoveFirstSeen("t1", "urgent"); err != nil {
		t.Fatal(err)
	}

	got, err := s.GetAutoRemoveFirstSeen()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("got %d entries after delete, want 0", len(got))
	}
}

func TestAutoRemoveFirstSeen_Cleanup(t *testing.T) {
	s := newTestStore(t)
	now := time.Now().Truncate(time.Second)

	if err := s.UpsertAutoRemoveFirstSeen("t1", "urgent", now); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertAutoRemoveFirstSeen("t2", "hot", now); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertAutoRemoveFirstSeen("t3", "urgent", now); err != nil {
		t.Fatal(err)
	}

	// Only t1:urgent is active
	active := map[string]bool{"t1:urgent": true}
	if err := s.CleanupAutoRemoveFirstSeen(active); err != nil {
		t.Fatal(err)
	}

	got, err := s.GetAutoRemoveFirstSeen()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d entries after cleanup, want 1", len(got))
	}
	if _, ok := got["t1:urgent"]; !ok {
		t.Error("t1:urgent should survive cleanup")
	}
}

func TestAutoRemoveFirstSeen_MultipleLabels(t *testing.T) {
	s := newTestStore(t)
	now := time.Now().Truncate(time.Second)
	later := now.Add(time.Hour)

	if err := s.UpsertAutoRemoveFirstSeen("t1", "urgent", now); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertAutoRemoveFirstSeen("t1", "hot", later); err != nil {
		t.Fatal(err)
	}

	got, err := s.GetAutoRemoveFirstSeen()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d entries, want 2", len(got))
	}
	if !got["t1:urgent"].Equal(now) {
		t.Errorf("t1:urgent: got %v, want %v", got["t1:urgent"], now)
	}
	if !got["t1:hot"].Equal(later) {
		t.Errorf("t1:hot: got %v, want %v", got["t1:hot"], later)
	}
}
