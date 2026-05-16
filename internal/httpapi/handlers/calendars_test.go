package handlers

import (
	"testing"

	calendar "google.golang.org/api/calendar/v3"
)

func TestGoogleEventTimesDateTime(t *testing.T) {
	start, end, allDay, ok := googleEventTimes(&calendar.Event{
		Start: &calendar.EventDateTime{DateTime: "2026-05-15T09:30:00+03:00"},
		End:   &calendar.EventDateTime{DateTime: "2026-05-15T10:15:00+03:00"},
	})
	if !ok {
		t.Fatal("expected event times to parse")
	}
	if allDay {
		t.Fatal("expected timed event")
	}
	if got := start.UTC().Format("15:04"); got != "06:30" {
		t.Fatalf("start UTC = %s; want 06:30", got)
	}
	if got := end.UTC().Format("15:04"); got != "07:15" {
		t.Fatalf("end UTC = %s; want 07:15", got)
	}
}

func TestGoogleEventTimesAllDay(t *testing.T) {
	start, end, allDay, ok := googleEventTimes(&calendar.Event{
		Start: &calendar.EventDateTime{Date: "2026-05-15"},
		End:   &calendar.EventDateTime{Date: "2026-05-16"},
	})
	if !ok {
		t.Fatal("expected event times to parse")
	}
	if !allDay {
		t.Fatal("expected all-day event")
	}
	if got := start.Format("2006-01-02"); got != "2026-05-15" {
		t.Fatalf("start date = %s; want 2026-05-15", got)
	}
	if got := end.Format("2006-01-02"); got != "2026-05-16" {
		t.Fatalf("end date = %s; want 2026-05-16", got)
	}
}
