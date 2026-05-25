package calendar

import (
	"strings"
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

// --- stripHTML tests ---

func TestStripHTML(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
		{
			name:  "plain text unchanged",
			input: "hello world",
			want:  "hello world",
		},
		{
			name:  "simple inline tag removed",
			input: "<b>bold</b> and <i>italic</i>",
			want:  "bold and italic",
		},
		{
			name:  "nested tags",
			input: "<b><i>nested</i></b>",
			want:  "nested",
		},
		{
			name:  "HTML entities decoded",
			input: "a &amp; b &lt;c&gt; &quot;d&quot; &#39;e&#39;",
			want:  "a & b <c> \"d\" 'e'",
		},
		{
			name:  "nbsp becomes space",
			input: "a&nbsp;b",
			want:  "a b",
		},
		{
			name:  "br creates newline",
			input: "line1<br>line2<br/>line3",
			want:  "line1\nline2\nline3",
		},
		{
			name:  "p tags create newlines",
			input: "<p>paragraph one</p><p>paragraph two</p>",
			want:  "paragraph one\nparagraph two",
		},
		{
			name:  "div tags create newlines",
			input: "<div>block one</div><div>block two</div>",
			want:  "block one\nblock two",
		},
		{
			name:  "li tags create newlines",
			input: "<ul><li>item one</li><li>item two</li></ul>",
			want:  "item one\nitem two",
		},
		{
			name:  "script content removed entirely",
			input: "<script>alert(1)</script>text",
			want:  "text",
		},
		{
			name:  "style content removed entirely",
			input: "<style>.a { color: red }</style>text",
			want:  "text",
		},
		{
			name:  "multiline script removed",
			input: "<script>\nfoo();\nbar();\n</script>after",
			want:  "after",
		},
		{
			name:  "mixed inline and block",
			input: "<p><b>Title</b></p><p>Body with <a href=\"x\">link</a> text.</p>",
			want:  "Title\nBody with link text.",
		},
		{
			name:  "extra whitespace collapsed",
			input: "  hello   world  ",
			want:  "hello world",
		},
		{
			name:  "blank lines dropped",
			input: "<p>one</p><p></p><p>two</p>",
			want:  "one\ntwo",
		},
		{
			name:  "length capped at 1000 with ellipsis",
			input: "<p>" + strings.Repeat("a", 1100) + "</p>",
			want:  strings.Repeat("a", 1000) + "…",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := stripHTML(tc.input)
			if got != tc.want {
				t.Errorf("stripHTML(%q)\n got  %q\n want %q", tc.input, got, tc.want)
			}
		})
	}
}
