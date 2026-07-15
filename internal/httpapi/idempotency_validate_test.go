package httpapi

import (
	"strings"
	"testing"
)

func TestIsValidIdempotencyKey(t *testing.T) {
	valid := []string{
		"abcdefgh",                             // exactly the 8-char minimum
		"1234-5678_ABCD",                       // digits, dash, underscore, upper
		"550e8400-e29b-41d4-a716-446655440000", // UUID
		strings.Repeat("a", 128),               // exactly the 128-char maximum
	}
	for _, k := range valid {
		if !isValidIdempotencyKey(k) {
			t.Errorf("key %q: got invalid, want valid", k)
		}
	}

	invalid := []string{
		"",                       // empty
		"short",                  // 5 chars
		"1234567",                // 7 chars (below minimum)
		strings.Repeat("a", 129), // 129 chars (above maximum)
		"has space",
		"has/slash",
		"semi;colon",
		"emoji\U0001F600key",
	}
	for _, k := range invalid {
		if isValidIdempotencyKey(k) {
			t.Errorf("key %q: got valid, want invalid", k)
		}
	}
}
