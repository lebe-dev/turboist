package inboxproc

import (
	"errors"
	"testing"
	"time"

	"github.com/lebe-dev/turboist/internal/model"
)

func moscow(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}
	return loc
}

// decisionNow is 2026-09-13 01:30 in Moscow — still the 12th in UTC, which is
// what makes the "today" boundary observable.
var decisionNow = time.Date(2026, 9, 12, 22, 30, 0, 0, time.UTC)

func TestParseDecision_PlainJSON(t *testing.T) {
	d, err := ParseDecision(`{"action":"sort","projectId":11,"labelIds":[5],"priority":null,"dueDate":null,"confidence":0.8,"reason":"garden work"}`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if d.Action != "sort" || d.ProjectID == nil || *d.ProjectID != 11 || len(d.LabelIDs) != 1 || d.Confidence != 0.8 {
		t.Errorf("decision: got %+v", d)
	}
}

func TestParseDecision_FencedAndProse(t *testing.T) {
	for name, raw := range map[string]string{
		"fence":        "```json\n{\"action\":\"keep\",\"reason\":\"a {brace} in text\"}\n```",
		"prose around": "Sure! Here it is: {\"action\":\"keep\",\"reason\":\"x\"} Hope it helps.",
	} {
		t.Run(name, func(t *testing.T) {
			d, err := ParseDecision(raw)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if d.Action != "keep" {
				t.Errorf("action: got %q, want keep", d.Action)
			}
		})
	}
}

func TestParseDecision_NoJSON(t *testing.T) {
	if _, err := ParseDecision("I cannot decide."); !errors.Is(err, ErrBadResponse) {
		t.Fatalf("got %v, want ErrBadResponse", err)
	}
}

func TestValidate_Keep(t *testing.T) {
	d := Decision{Action: "keep", Confidence: 0.9, Reason: "ambiguous"}
	got, err := d.Validate(sampleCatalogue(), decisionNow, moscow(t))
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if !got.Keep {
		t.Errorf("got %+v, want keep", got)
	}
}

func TestValidate_LowConfidenceIsKeep(t *testing.T) {
	d := Decision{Action: "sort", ProjectID: ptr(int64(11)), Confidence: 0.4}
	got, err := d.Validate(sampleCatalogue(), decisionNow, moscow(t))
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if !got.Keep {
		t.Errorf("got %+v, want keep below MinConfidence", got)
	}
}

func TestValidate_UnknownProjectFails(t *testing.T) {
	for name, d := range map[string]Decision{
		"unknown id":     {Action: "sort", ProjectID: ptr(int64(999)), Confidence: 0.9},
		"archived id":    {Action: "sort", ProjectID: ptr(int64(12)), Confidence: 0.9},
		"missing id":     {Action: "sort", Confidence: 0.9},
		"unknown action": {Action: "delete", ProjectID: ptr(int64(11)), Confidence: 0.9},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := d.Validate(sampleCatalogue(), decisionNow, moscow(t)); !errors.Is(err, ErrInvalidDecision) {
				t.Fatalf("got %v, want ErrInvalidDecision", err)
			}
		})
	}
}

func TestValidate_SortFiltersLabelsAndParsesFields(t *testing.T) {
	d := Decision{
		Action: "sort", ProjectID: ptr(int64(11)), LabelIDs: []int64{6, 999, 6, 5},
		Priority: ptr("high"), DueDate: ptr("2026-09-20"), Confidence: 0.75, Reason: "garden",
	}
	got, err := d.Validate(sampleCatalogue(), decisionNow, moscow(t))
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if got.Keep || got.Project == nil || got.Project.ID != 11 {
		t.Fatalf("got %+v, want a sort into 11", got)
	}
	if len(got.LabelIDs) != 2 || got.LabelIDs[0] != 6 || got.LabelIDs[1] != 5 {
		t.Errorf("labels: got %v, want [6 5]", got.LabelIDs)
	}
	if got.Priority == nil || *got.Priority != model.PriorityHigh {
		t.Errorf("priority: got %v, want high", got.Priority)
	}
	want := time.Date(2026, 9, 20, 0, 0, 0, 0, moscow(t))
	if got.DueAt == nil || !got.DueAt.Equal(want) {
		t.Errorf("due: got %v, want %v", got.DueAt, want)
	}
	if len(got.Warnings) == 0 {
		t.Error("an unknown label must leave a warning")
	}
}

func TestValidate_InvalidPriorityIgnored(t *testing.T) {
	d := Decision{Action: "sort", ProjectID: ptr(int64(11)), Priority: ptr("urgent"), Confidence: 0.9}
	got, err := d.Validate(sampleCatalogue(), decisionNow, moscow(t))
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if got.Priority != nil {
		t.Errorf("priority: got %v, want nil", *got.Priority)
	}
	if len(got.Warnings) == 0 {
		t.Error("an invalid priority must leave a warning")
	}
}

func TestValidate_TroikiProjectIgnoresPriority(t *testing.T) {
	d := Decision{Action: "sort", ProjectID: ptr(int64(10)), Priority: ptr("low"), Confidence: 0.9}
	got, err := d.Validate(sampleCatalogue(), decisionNow, moscow(t))
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if got.Priority != nil {
		t.Errorf("priority: got %v, want nil for a daily-plan project", *got.Priority)
	}
}

func TestValidate_DueDates(t *testing.T) {
	loc := moscow(t)
	cases := map[string]struct {
		raw  string
		want *time.Time
	}{
		// In Moscow it is already the 13th, so the 12th is in the past.
		"yesterday dropped": {"2026-09-12", nil},
		"today kept":        {"2026-09-13", ptr(time.Date(2026, 9, 13, 0, 0, 0, 0, loc))},
		"garbage dropped":   {"next friday", nil},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			d := Decision{Action: "sort", ProjectID: ptr(int64(11)), DueDate: ptr(tc.raw), Confidence: 0.9}
			got, err := d.Validate(sampleCatalogue(), decisionNow, loc)
			if err != nil {
				t.Fatalf("validate: %v", err)
			}
			switch {
			case tc.want == nil && got.DueAt != nil:
				t.Errorf("due: got %v, want nil", got.DueAt)
			case tc.want != nil && (got.DueAt == nil || !got.DueAt.Equal(*tc.want)):
				t.Errorf("due: got %v, want %v", got.DueAt, *tc.want)
			}
		})
	}
}

func ptr[T any](v T) *T { return &v }
