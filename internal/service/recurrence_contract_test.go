package service

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lebe-dev/turboist/internal/model"
)

// Completing a recurring task advances it to its next occurrence. The native
// mobile client does that arithmetic on the device, before the completion has
// reached the server, so that a task ticked off with no connection shows its
// next run straight away instead of an hour later. Two implementations of the
// same calendar rules drift silently, and a wrong next date reads to the user
// as a bug in their plan rather than in the app.
//
// So both sides answer one shared list of (rule, anchor) questions and must
// produce the same instants. This is the Go half: it runs every case through
// the very functions the completion path calls and diffs the answers against
// the file. The mobile client reads the same file and diffs its own.
//
// Rerun with `go test ./internal/service -run TestRecurrenceContract -update`
// after a deliberate change to the advance, and review the diff — every changed
// line is a behaviour change the mobile client has to follow.
var updateRecurrenceContract = flag.Bool("update", false, "rewrite the shared recurrence-contract expectations")

const recurrenceContractPath = "../../testdata/recurrence-contract/fixture.json"

type recurrenceContract struct {
	Cases []recurrenceCase `json:"cases"`
}

// recurrenceCase is one question and its answer.
//
// The question is the rule plus the two facts the anchor is derived from: where
// the task currently sits (dueAt, absent when it has no date) and when it was
// ticked off. Anchor and next are the answer, both written as UTC instants so
// neither side can pass by agreeing with its own idea of a wall clock. An empty
// next means the series has run out and the completion closes the task.
type recurrenceCase struct {
	Key         string `json:"key"`
	Notes       string `json:"notes"`
	Rule        string `json:"rule"`
	Timezone    string `json:"timezone"`
	DueAt       string `json:"dueAt"`
	CompletedAt string `json:"completedAt"`
	Anchor      string `json:"anchor"`
	Next        string `json:"next"`
}

func readRecurrenceContract(t *testing.T) *recurrenceContract {
	t.Helper()
	raw, err := os.ReadFile(recurrenceContractPath)
	if err != nil {
		t.Fatalf("read recurrence contract: %v", err)
	}
	var c recurrenceContract
	if err := json.Unmarshal(raw, &c); err != nil {
		t.Fatalf("parse recurrence contract: %v", err)
	}
	return &c
}

// answer computes the two instants a case asks for, using the same helpers the
// completion path uses. Anything it cannot compute is a failed test rather than
// a silently empty expectation.
func (c recurrenceCase) answer(t *testing.T) (string, string) {
	t.Helper()
	loc, err := time.LoadLocation(c.Timezone)
	if err != nil {
		t.Fatalf("%s: load timezone %q: %v", c.Key, c.Timezone, err)
	}
	completedAt, err := model.ParseUTC(c.CompletedAt)
	if err != nil {
		t.Fatalf("%s: parse completedAt %q: %v", c.Key, c.CompletedAt, err)
	}
	var dueAt *time.Time
	if c.DueAt != "" {
		parsed, err := model.ParseUTC(c.DueAt)
		if err != nil {
			t.Fatalf("%s: parse dueAt %q: %v", c.Key, c.DueAt, err)
		}
		dueAt = &parsed
	}
	anchor := RecurrenceAnchor(completedAt, dueAt, loc)
	next, err := NextOccurrence(c.Rule, anchor)
	if err != nil {
		t.Fatalf("%s: compute next occurrence of %q: %v", c.Key, c.Rule, err)
	}
	if next.IsZero() {
		return model.FormatUTC(anchor), ""
	}
	return model.FormatUTC(anchor), model.FormatUTC(next)
}

func TestRecurrenceContract_Expectations(t *testing.T) {
	contract := readRecurrenceContract(t)
	if *updateRecurrenceContract {
		for i := range contract.Cases {
			contract.Cases[i].Anchor, contract.Cases[i].Next = contract.Cases[i].answer(t)
		}
		encoded, err := json.MarshalIndent(contract, "", "  ")
		if err != nil {
			t.Fatalf("encode recurrence contract: %v", err)
		}
		if err := os.WriteFile(filepath.Clean(recurrenceContractPath), append(encoded, '\n'), 0o600); err != nil {
			t.Fatalf("write recurrence contract: %v", err)
		}
		return
	}
	for _, c := range contract.Cases {
		anchor, next := c.answer(t)
		if anchor != c.Anchor {
			t.Errorf("case %s: anchor is %q, the contract says %q", c.Key, anchor, c.Anchor)
		}
		if next != c.Next {
			t.Errorf("case %s: next occurrence is %q, the contract says %q", c.Key, next, c.Next)
		}
	}
}

// TestRecurrenceContract_IsUsable keeps the shared file readable from both
// sides: a case with no key cannot be named in a failure, one with no note
// cannot be understood by whoever changed the behaviour it pins, and a
// duplicate key would hide one of the two cases behind the other.
func TestRecurrenceContract_IsUsable(t *testing.T) {
	contract := readRecurrenceContract(t)
	if len(contract.Cases) == 0 {
		t.Fatal("the recurrence contract declares no cases")
	}
	seen := map[string]bool{}
	for _, c := range contract.Cases {
		if c.Key == "" {
			t.Fatal("every recurrence case needs a key")
		}
		if seen[c.Key] {
			t.Errorf("duplicate recurrence case key %q", c.Key)
		}
		seen[c.Key] = true
		if c.Notes == "" {
			t.Errorf("case %s: a note saying what it pins is required", c.Key)
		}
		if c.Rule == "" {
			t.Errorf("case %s: a rule is required", c.Key)
		}
		if c.Timezone == "" {
			t.Errorf("case %s: a timezone is required", c.Key)
		}
		if c.CompletedAt == "" {
			t.Errorf("case %s: the moment it was ticked off is required", c.Key)
		}
	}
}
