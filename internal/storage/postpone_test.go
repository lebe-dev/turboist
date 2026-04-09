package storage

import "testing"

func TestIncrementPostponeCount(t *testing.T) {
	s := newTestStore(t)

	if err := s.IncrementPostponeCount("t1"); err != nil {
		t.Fatal(err)
	}
	if err := s.IncrementPostponeCount("t1"); err != nil {
		t.Fatal(err)
	}

	counts, err := s.GetPostponeCounts()
	if err != nil {
		t.Fatal(err)
	}
	if counts["t1"] != 2 {
		t.Errorf("got %d, want 2", counts["t1"])
	}
}

func TestResetPostponeCount(t *testing.T) {
	s := newTestStore(t)

	if err := s.IncrementPostponeCount("t1"); err != nil {
		t.Fatal(err)
	}
	if err := s.IncrementPostponeCount("t1"); err != nil {
		t.Fatal(err)
	}

	if err := s.ResetPostponeCount("t1"); err != nil {
		t.Fatal(err)
	}

	counts, err := s.GetPostponeCounts()
	if err != nil {
		t.Fatal(err)
	}
	if counts["t1"] != 0 {
		t.Errorf("got %d, want 0", counts["t1"])
	}
}

func TestResetPostponeCount_NoEntry(t *testing.T) {
	s := newTestStore(t)

	// Resetting non-existent entry should not error
	if err := s.ResetPostponeCount("nonexistent"); err != nil {
		t.Fatal(err)
	}
}
