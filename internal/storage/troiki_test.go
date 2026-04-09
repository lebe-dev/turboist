package storage

import "testing"

func TestGetAllTroikiCapacity_Empty(t *testing.T) {
	s := newTestStore(t)

	caps, err := s.GetAllTroikiCapacity()
	if err != nil {
		t.Fatalf("get all troiki capacity: %v", err)
	}
	if len(caps) != 0 {
		t.Errorf("expected empty map, got %d entries", len(caps))
	}
}

func TestIncrementTroikiCapacity_NewEntry(t *testing.T) {
	s := newTestStore(t)

	if err := s.IncrementTroikiCapacity("medium"); err != nil {
		t.Fatalf("increment: %v", err)
	}

	caps, err := s.GetAllTroikiCapacity()
	if err != nil {
		t.Fatalf("get all: %v", err)
	}
	if caps["medium"] != 1 {
		t.Errorf("expected medium=1, got %d", caps["medium"])
	}
}

func TestIncrementTroikiCapacity_MultipleIncrements(t *testing.T) {
	s := newTestStore(t)

	for i := range 5 {
		if err := s.IncrementTroikiCapacity("rest"); err != nil {
			t.Fatalf("increment %d: %v", i, err)
		}
	}

	caps, err := s.GetAllTroikiCapacity()
	if err != nil {
		t.Fatalf("get all: %v", err)
	}
	if caps["rest"] != 5 {
		t.Errorf("expected rest=5, got %d", caps["rest"])
	}
}

func TestIncrementTroikiCapacity_MultipleSections(t *testing.T) {
	s := newTestStore(t)

	if err := s.IncrementTroikiCapacity("medium"); err != nil {
		t.Fatalf("increment medium: %v", err)
	}
	if err := s.IncrementTroikiCapacity("medium"); err != nil {
		t.Fatalf("increment medium 2: %v", err)
	}
	if err := s.IncrementTroikiCapacity("rest"); err != nil {
		t.Fatalf("increment rest: %v", err)
	}

	caps, err := s.GetAllTroikiCapacity()
	if err != nil {
		t.Fatalf("get all: %v", err)
	}
	if caps["medium"] != 2 {
		t.Errorf("expected medium=2, got %d", caps["medium"])
	}
	if caps["rest"] != 1 {
		t.Errorf("expected rest=1, got %d", caps["rest"])
	}
}

func TestDecrementTroikiCapacity_Decrements(t *testing.T) {
	s := newTestStore(t)

	for range 3 {
		if err := s.IncrementTroikiCapacity("medium"); err != nil {
			t.Fatalf("increment: %v", err)
		}
	}

	if err := s.DecrementTroikiCapacity("medium"); err != nil {
		t.Fatalf("decrement: %v", err)
	}

	caps, err := s.GetAllTroikiCapacity()
	if err != nil {
		t.Fatalf("get all: %v", err)
	}
	if caps["medium"] != 2 {
		t.Errorf("expected medium=2, got %d", caps["medium"])
	}
}

func TestDecrementTroikiCapacity_FloorsAtZero(t *testing.T) {
	s := newTestStore(t)

	if err := s.IncrementTroikiCapacity("medium"); err != nil {
		t.Fatalf("increment: %v", err)
	}
	for range 5 {
		if err := s.DecrementTroikiCapacity("medium"); err != nil {
			t.Fatalf("decrement: %v", err)
		}
	}

	caps, err := s.GetAllTroikiCapacity()
	if err != nil {
		t.Fatalf("get all: %v", err)
	}
	if caps["medium"] != 0 {
		t.Errorf("expected medium=0, got %d", caps["medium"])
	}
}

func TestDecrementTroikiCapacity_MissingRowNoOp(t *testing.T) {
	s := newTestStore(t)

	if err := s.DecrementTroikiCapacity("medium"); err != nil {
		t.Fatalf("decrement on missing row: %v", err)
	}

	caps, err := s.GetAllTroikiCapacity()
	if err != nil {
		t.Fatalf("get all: %v", err)
	}
	if caps["medium"] != 0 {
		t.Errorf("expected medium=0 (absent key), got %d", caps["medium"])
	}
}

func TestEnsureMinTroikiCapacity_SetsNew(t *testing.T) {
	s := newTestStore(t)

	if err := s.EnsureMinTroikiCapacity("medium", 3); err != nil {
		t.Fatalf("ensure min: %v", err)
	}

	caps, err := s.GetAllTroikiCapacity()
	if err != nil {
		t.Fatalf("get all: %v", err)
	}
	if caps["medium"] != 3 {
		t.Errorf("expected medium=3, got %d", caps["medium"])
	}
}

func TestEnsureMinTroikiCapacity_DoesNotDecrease(t *testing.T) {
	s := newTestStore(t)

	for range 5 {
		if err := s.IncrementTroikiCapacity("medium"); err != nil {
			t.Fatalf("increment: %v", err)
		}
	}

	if err := s.EnsureMinTroikiCapacity("medium", 3); err != nil {
		t.Fatalf("ensure min: %v", err)
	}

	caps, err := s.GetAllTroikiCapacity()
	if err != nil {
		t.Fatalf("get all: %v", err)
	}
	if caps["medium"] != 5 {
		t.Errorf("expected medium to stay at 5, got %d", caps["medium"])
	}
}
