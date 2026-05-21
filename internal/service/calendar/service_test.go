package calendar

import (
	"testing"

	gcal "google.golang.org/api/calendar/v3"
)

// --- TokenCipher tests ---

func TestTokenCipherRoundTrip(t *testing.T) {
	c := NewTokenCipher("01234567890123456789012345678901")
	encrypted, err := c.Encrypt("secret-token")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if encrypted == "secret-token" {
		t.Fatal("token was not encrypted")
	}
	decrypted, err := c.Decrypt(encrypted)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if decrypted != "secret-token" {
		t.Fatalf("decrypted = %q; want secret-token", decrypted)
	}
}

func TestTokenCipherAllowsLegacyPlaintext(t *testing.T) {
	c := NewTokenCipher("01234567890123456789012345678901")
	decrypted, err := c.Decrypt("legacy-token")
	if err != nil {
		t.Fatalf("Decrypt legacy token: %v", err)
	}
	if decrypted != "legacy-token" {
		t.Fatalf("decrypted = %q; want legacy-token", decrypted)
	}
}

func TestTokenCipherEmpty(t *testing.T) {
	c := NewTokenCipher("key")
	enc, err := c.Encrypt("")
	if err != nil {
		t.Fatalf("Encrypt empty: %v", err)
	}
	if enc != "" {
		t.Fatalf("encrypted empty = %q; want empty", enc)
	}
	dec, err := c.Decrypt("")
	if err != nil {
		t.Fatalf("Decrypt empty: %v", err)
	}
	if dec != "" {
		t.Fatalf("decrypted empty = %q; want empty", dec)
	}
}

func TestIsEncrypted(t *testing.T) {
	if !IsEncrypted("") {
		t.Error("empty should be considered encrypted")
	}
	if !IsEncrypted(EncryptedTokenPrefix + "something") {
		t.Error("prefixed value should be considered encrypted")
	}
	if IsEncrypted("plain-token") {
		t.Error("plain token should not be considered encrypted")
	}
}

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
