package handlers

import "testing"

func TestDuplicateTitle(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"Title", "Title (2)"},
		{"Title (2)", "Title (3)"},
		{"Title (3)", "Title (4)"},
		{"Title (10)", "Title (11)"},
		{"Buy milk", "Buy milk (2)"},
		{"Task (1)", "Task (2)"},
	}
	for _, tc := range cases {
		got := duplicateTitle(tc.input)
		if got != tc.want {
			t.Errorf("duplicateTitle(%q): got %q, want %q", tc.input, got, tc.want)
		}
	}
}
