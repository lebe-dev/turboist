package storage

import (
	"testing"
	"time"
)

func TestGetLabelBlocks_Empty(t *testing.T) {
	s := newTestStore(t)

	blocks, err := s.GetLabelBlocks()
	if err != nil {
		t.Fatalf("get label blocks: %v", err)
	}
	if len(blocks) != 0 {
		t.Errorf("expected empty, got %d entries", len(blocks))
	}
}

func TestUpsertLabelBlock_NewEntry(t *testing.T) {
	s := newTestStore(t)
	now := time.Now().Truncate(time.Second)

	if err := s.UpsertLabelBlock("health", now); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	blocks, err := s.GetLabelBlocks()
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(blocks))
	}
	if blocks[0].Label != "health" {
		t.Errorf("label: got %q, want %q", blocks[0].Label, "health")
	}
	if !blocks[0].StartedAt.Equal(now) {
		t.Errorf("started_at: got %v, want %v", blocks[0].StartedAt, now)
	}
}

func TestUpsertLabelBlock_PreservesOriginal(t *testing.T) {
	s := newTestStore(t)
	original := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	later := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)

	if err := s.UpsertLabelBlock("health", original); err != nil {
		t.Fatalf("upsert 1: %v", err)
	}
	if err := s.UpsertLabelBlock("health", later); err != nil {
		t.Fatalf("upsert 2: %v", err)
	}

	blocks, err := s.GetLabelBlocks()
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(blocks))
	}
	if !blocks[0].StartedAt.Equal(original) {
		t.Errorf("started_at: got %v, want %v (original should be preserved)", blocks[0].StartedAt, original)
	}
}

func TestDeleteLabelBlock(t *testing.T) {
	s := newTestStore(t)
	now := time.Now().Truncate(time.Second)

	if err := s.UpsertLabelBlock("health", now); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if err := s.UpsertLabelBlock("shopping", now); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	if err := s.DeleteLabelBlock("health"); err != nil {
		t.Fatalf("delete: %v", err)
	}

	blocks, err := s.GetLabelBlocks()
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(blocks))
	}
	if blocks[0].Label != "shopping" {
		t.Errorf("label: got %q, want %q", blocks[0].Label, "shopping")
	}
}

func TestDeleteUnconfiguredLabelBlocks(t *testing.T) {
	s := newTestStore(t)
	now := time.Now().Truncate(time.Second)

	if err := s.UpsertLabelBlock("health", now); err != nil {
		t.Fatalf("upsert health: %v", err)
	}
	if err := s.UpsertLabelBlock("shopping", now); err != nil {
		t.Fatalf("upsert shopping: %v", err)
	}
	if err := s.UpsertLabelBlock("work", now); err != nil {
		t.Fatalf("upsert work: %v", err)
	}

	// Only "health" is still configured
	if err := s.DeleteUnconfiguredLabelBlocks([]string{"health"}); err != nil {
		t.Fatalf("delete unconfigured: %v", err)
	}

	blocks, err := s.GetLabelBlocks()
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(blocks))
	}
	if blocks[0].Label != "health" {
		t.Errorf("label: got %q, want %q", blocks[0].Label, "health")
	}
}

func TestDailyConstraints_RoundTrip(t *testing.T) {
	s := newTestStore(t)

	state := &DailyConstraintsState{
		Date:        "2025-01-15",
		Items:       []string{"no phone", "read 30 min", "exercise"},
		RerollsUsed: 1,
		Confirmed:   true,
	}

	if err := s.SetDailyConstraints(state); err != nil {
		t.Fatalf("set: %v", err)
	}

	got, err := s.GetDailyConstraints()
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil state")
	}
	if got.Date != "2025-01-15" {
		t.Errorf("date: got %q, want %q", got.Date, "2025-01-15")
	}
	if len(got.Items) != 3 {
		t.Fatalf("items: got %d, want 3", len(got.Items))
	}
	if got.Items[0] != "no phone" {
		t.Errorf("items[0]: got %q, want %q", got.Items[0], "no phone")
	}
	if got.RerollsUsed != 1 {
		t.Errorf("rerolls_used: got %d, want 1", got.RerollsUsed)
	}
	if !got.Confirmed {
		t.Error("expected confirmed=true")
	}
}

func TestDailyConstraints_NotFound(t *testing.T) {
	s := newTestStore(t)

	got, err := s.GetDailyConstraints()
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil, got %+v", got)
	}
}

func TestPostponeBudget_RoundTrip(t *testing.T) {
	s := newTestStore(t)

	state := &PostponeBudgetState{
		Date: "2025-01-15",
		Used: 3,
	}

	if err := s.SetPostponeBudget(state); err != nil {
		t.Fatalf("set: %v", err)
	}

	got, err := s.GetPostponeBudget()
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil state")
	}
	if got.Date != "2025-01-15" {
		t.Errorf("date: got %q, want %q", got.Date, "2025-01-15")
	}
	if got.Used != 3 {
		t.Errorf("used: got %d, want 3", got.Used)
	}
}

func TestPostponeBudget_NotFound(t *testing.T) {
	s := newTestStore(t)

	got, err := s.GetPostponeBudget()
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil, got %+v", got)
	}
}

func TestConstraintPool_RoundTrip(t *testing.T) {
	s := newTestStore(t)

	pool := []string{"no phone", "read 30 min", "exercise", "meditate", "journal"}

	if err := s.SetConstraintPool(pool); err != nil {
		t.Fatalf("set: %v", err)
	}

	got, err := s.GetConstraintPool()
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(got) != 5 {
		t.Fatalf("pool: got %d, want 5", len(got))
	}
	if got[0] != "no phone" {
		t.Errorf("pool[0]: got %q, want %q", got[0], "no phone")
	}
	if got[4] != "journal" {
		t.Errorf("pool[4]: got %q, want %q", got[4], "journal")
	}
}

func TestConstraintPool_NotFound(t *testing.T) {
	s := newTestStore(t)

	got, err := s.GetConstraintPool()
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

func TestConstraintPool_Overwrite(t *testing.T) {
	s := newTestStore(t)

	if err := s.SetConstraintPool([]string{"a", "b"}); err != nil {
		t.Fatalf("set 1: %v", err)
	}
	if err := s.SetConstraintPool([]string{"x", "y", "z"}); err != nil {
		t.Fatalf("set 2: %v", err)
	}

	got, err := s.GetConstraintPool()
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("pool: got %d, want 3", len(got))
	}
	if got[0] != "x" {
		t.Errorf("pool[0]: got %q, want %q", got[0], "x")
	}
}
