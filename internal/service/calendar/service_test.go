package calendar

import (
	"testing"

	gcal "google.golang.org/api/calendar/v3"
)

// --- googleEventTimes tests ---

func TestGoogleEventTimesDateTime(t *testing.T) {
	start, end, startDate, endDate, allDay, ok := googleEventTimes(&gcal.Event{
		Start: &gcal.EventDateTime{DateTime: "2026-05-15T09:30:00+03:00"},
		End:   &gcal.EventDateTime{DateTime: "2026-05-15T10:15:00+03:00"},
	})
	if !ok {
		t.Fatal("expected event times to parse")
	}
	if allDay {
		t.Fatal("expected timed event")
	}
	if startDate != "" || endDate != "" {
		t.Fatalf("timed date fields = %q/%q; want empty", startDate, endDate)
	}
	if got := start.UTC().Format("15:04"); got != "06:30" {
		t.Fatalf("start UTC = %s; want 06:30", got)
	}
	if got := end.UTC().Format("15:04"); got != "07:15" {
		t.Fatalf("end UTC = %s; want 07:15", got)
	}
}

func TestGoogleEventTimesAllDay(t *testing.T) {
	start, end, startDate, endDate, allDay, ok := googleEventTimes(&gcal.Event{
		Start: &gcal.EventDateTime{Date: "2026-05-15"},
		End:   &gcal.EventDateTime{Date: "2026-05-16"},
	})
	if !ok {
		t.Fatal("expected event times to parse")
	}
	if !allDay {
		t.Fatal("expected all-day event")
	}
	if startDate != "2026-05-15" || endDate != "2026-05-16" {
		t.Fatalf("all-day date fields = %q/%q; want 2026-05-15/2026-05-16", startDate, endDate)
	}
	if got := start.Format("2006-01-02"); got != "2026-05-15" {
		t.Fatalf("start date = %s; want 2026-05-15", got)
	}
	if got := end.Format("2006-01-02"); got != "2026-05-16" {
		t.Fatalf("end date = %s; want 2026-05-16", got)
	}
}

func TestGoogleEventTimesNilStartEnd(t *testing.T) {
	_, _, _, _, _, ok := googleEventTimes(&gcal.Event{})
	if ok {
		t.Fatal("expected false for nil Start/End")
	}
}

func TestGoogleEventTimesInvalidDateTime(t *testing.T) {
	_, _, _, _, _, ok := googleEventTimes(&gcal.Event{
		Start: &gcal.EventDateTime{DateTime: "not-a-time"},
		End:   &gcal.EventDateTime{DateTime: "2026-05-15T10:00:00Z"},
	})
	if ok {
		t.Fatal("expected false for invalid datetime")
	}
}
