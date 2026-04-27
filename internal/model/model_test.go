package model

import (
	"testing"
	"time"
)

func TestPriorityIsValid(t *testing.T) {
	valid := []Priority{PriorityHigh, PriorityMedium, PriorityLow, PriorityNone}
	for _, p := range valid {
		if !p.IsValid() {
			t.Errorf("expected %q valid", p)
		}
	}
	if Priority("urgent").IsValid() {
		t.Error("unexpected valid")
	}
	if Priority("").IsValid() {
		t.Error("empty must be invalid")
	}
}

func TestTaskStatusIsValid(t *testing.T) {
	for _, s := range []TaskStatus{TaskStatusOpen, TaskStatusCompleted, TaskStatusCancelled} {
		if !s.IsValid() {
			t.Errorf("expected %q valid", s)
		}
	}
	if TaskStatus("archived").IsValid() {
		t.Error("unexpected valid")
	}
}

func TestProjectStatusIsValid(t *testing.T) {
	for _, s := range []ProjectStatus{ProjectStatusOpen, ProjectStatusCompleted, ProjectStatusArchived, ProjectStatusCancelled} {
		if !s.IsValid() {
			t.Errorf("expected %q valid", s)
		}
	}
	if ProjectStatus("paused").IsValid() {
		t.Error("unexpected valid")
	}
}

func TestDayPartIsValid(t *testing.T) {
	for _, d := range []DayPart{DayPartNone, DayPartMorning, DayPartAfternoon, DayPartEvening} {
		if !d.IsValid() {
			t.Errorf("expected %q valid", d)
		}
	}
	if DayPart("night").IsValid() {
		t.Error("unexpected valid")
	}
}

func TestPlanStateIsValid(t *testing.T) {
	for _, p := range []PlanState{PlanStateNone, PlanStateWeek, PlanStateBacklog} {
		if !p.IsValid() {
			t.Errorf("expected %q valid", p)
		}
	}
	if PlanState("month").IsValid() {
		t.Error("unexpected valid")
	}
}

func TestClientKindIsValid(t *testing.T) {
	for _, c := range []ClientKind{ClientWeb, ClientIOS, ClientCLI} {
		if !c.IsValid() {
			t.Errorf("expected %q valid", c)
		}
	}
	if ClientKind("android").IsValid() {
		t.Error("unexpected valid")
	}
}

func TestFormatUTCRoundTrip(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}
	in := time.Date(2026, 4, 27, 10, 30, 45, 123_000_000, loc)
	s := FormatUTC(in)
	want := "2026-04-27T14:30:45.123Z"
	if s != want {
		t.Errorf("format: got %q, want %q", s, want)
	}
	parsed, err := ParseUTC(s)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !parsed.Equal(in) {
		t.Errorf("round-trip: parsed %v != input %v", parsed, in)
	}
	if parsed.Location() != time.UTC {
		t.Errorf("expected UTC, got %v", parsed.Location())
	}
}

func TestParseUTCInvalid(t *testing.T) {
	if _, err := ParseUTC("not-a-time"); err == nil {
		t.Error("expected error")
	}
	if _, err := ParseUTC("2026-04-27T14:30:45Z"); err == nil {
		t.Error("expected error for missing milliseconds")
	}
}

func TestTaskURL(t *testing.T) {
	tk := &Task{ID: 42}
	got := tk.URL("https://example.com")
	want := "https://example.com/task/42"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSessionIsActive(t *testing.T) {
	now := time.Date(2026, 4, 27, 12, 0, 0, 0, time.UTC)
	future := now.Add(time.Hour)
	past := now.Add(-time.Hour)

	active := &Session{ExpiresAt: future}
	if !active.IsActive(now) {
		t.Error("expected active")
	}

	expired := &Session{ExpiresAt: past}
	if expired.IsActive(now) {
		t.Error("expected inactive (expired)")
	}

	revoked := &Session{ExpiresAt: future, RevokedAt: &now}
	if revoked.IsActive(now) {
		t.Error("expected inactive (revoked)")
	}
}
