package handlers

import (
	"testing"

	calendar "google.golang.org/api/calendar/v3"
)

func TestGoogleEventTimesDateTime(t *testing.T) {
	start, end, startDate, endDate, allDay, ok := googleEventTimes(&calendar.Event{
		Start: &calendar.EventDateTime{DateTime: "2026-05-15T09:30:00+03:00"},
		End:   &calendar.EventDateTime{DateTime: "2026-05-15T10:15:00+03:00"},
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
	start, end, startDate, endDate, allDay, ok := googleEventTimes(&calendar.Event{
		Start: &calendar.EventDateTime{Date: "2026-05-15"},
		End:   &calendar.EventDateTime{Date: "2026-05-16"},
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

func TestCalendarTokenCipherRoundTrip(t *testing.T) {
	cipher := newCalendarTokenCipher("01234567890123456789012345678901")
	encrypted, err := cipher.encrypt("secret-token")
	if err != nil {
		t.Fatalf("encrypt token: %v", err)
	}
	if encrypted == "secret-token" {
		t.Fatal("token was not encrypted")
	}
	decrypted, err := cipher.decrypt(encrypted)
	if err != nil {
		t.Fatalf("decrypt token: %v", err)
	}
	if decrypted != "secret-token" {
		t.Fatalf("decrypted token = %q; want secret-token", decrypted)
	}
}

func TestCalendarTokenCipherAllowsLegacyPlaintext(t *testing.T) {
	cipher := newCalendarTokenCipher("01234567890123456789012345678901")
	decrypted, err := cipher.decrypt("legacy-token")
	if err != nil {
		t.Fatalf("decrypt legacy token: %v", err)
	}
	if decrypted != "legacy-token" {
		t.Fatalf("decrypted token = %q; want legacy-token", decrypted)
	}
}
