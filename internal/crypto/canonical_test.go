package crypto

import (
	"strings"
	"testing"
)

func TestCanonicalJSON_SortsKeysDeterministically(t *testing.T) {
	// Two maps with the same content in different insertion order must produce
	// identical canonical bytes (sorted keys).
	a := map[string]any{"b": 2, "a": 1, "c": 3}
	b := map[string]any{"c": 3, "b": 2, "a": 1}

	gotA, err := CanonicalJSON(a)
	if err != nil {
		t.Fatalf("canonical a: %v", err)
	}
	gotB, err := CanonicalJSON(b)
	if err != nil {
		t.Fatalf("canonical b: %v", err)
	}
	if string(gotA) != string(gotB) {
		t.Errorf("canonical output not deterministic: %q vs %q", gotA, gotB)
	}
	if want := `{"a":1,"b":2,"c":3}`; string(gotA) != want {
		t.Errorf("canonical: got %q, want %q", gotA, want)
	}
}

func TestCanonicalJSON_NestedKeysSorted(t *testing.T) {
	v := map[string]any{
		"outer": map[string]any{"z": 1, "a": 2},
		"first": "x",
	}
	got, err := CanonicalJSON(v)
	if err != nil {
		t.Fatalf("canonical: %v", err)
	}
	if want := `{"first":"x","outer":{"a":2,"z":1}}`; string(got) != want {
		t.Errorf("nested canonical: got %q, want %q", got, want)
	}
}

func TestCanonicalJSON_DoesNotEscapeHTML(t *testing.T) {
	// SetEscapeHTML(false): the ampersand, less-than, and greater-than runes
	// must survive verbatim so peers on a strict canonicalizer compute the same
	// digest. The stdlib default would emit the & / < / >
	// escape sequences instead.
	amp := string(rune(0x26)) // &
	lt := string(rune(0x3c))  // <
	gt := string(rune(0x3e))  // >
	v := map[string]any{"url": "https://a.example/?x=1" + amp + "y=2" + lt + gt}
	got, err := CanonicalJSON(v)
	if err != nil {
		t.Fatalf("canonical: %v", err)
	}
	// The stdlib HTML escapes are the 6-byte unicode sequences &, <,
	// >. Build them from bytes so the literal backslash is unambiguous.
	escapes := []string{
		string([]byte{'\\', 'u', '0', '0', '2', '6'}),
		string([]byte{'\\', 'u', '0', '0', '3', 'c'}),
		string([]byte{'\\', 'u', '0', '0', '3', 'e'}),
	}
	for _, escSeq := range escapes {
		if strings.Contains(string(got), escSeq) {
			t.Errorf("canonical emitted escape sequence %q: %q", escSeq, got)
		}
	}
	want := `{"url":"https://a.example/?x=1` + amp + "y=2" + lt + gt + `"}`
	if string(got) != want {
		t.Errorf("canonical: got %q, want %q", got, want)
	}
}

func TestCanonicalJSON_NoTrailingNewlineOrWhitespace(t *testing.T) {
	got, err := CanonicalJSON(map[string]any{"a": 1})
	if err != nil {
		t.Fatalf("canonical: %v", err)
	}
	if strings.ContainsAny(string(got), "\n\t ") {
		t.Errorf("canonical contains whitespace: %q", got)
	}
}
