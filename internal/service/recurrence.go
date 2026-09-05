package service

import (
	"time"

	rrule "github.com/teambition/rrule-go"
)

// RecurrenceAnchor returns the instant a recurring task's next occurrence is
// measured from.
//
// The anchor is the occurrence the task currently sits on when that is still
// ahead, and otherwise the moment it was ticked off. Expanding the rule from
// there rather than from the series' original start is what makes a task
// completed three weeks late resume from today instead of marching through
// every run it missed.
//
// The anchor is expressed in loc because a rule reads against a wall clock:
// "every weekday at nine" means the user's nine, not UTC's, and a rule with no
// time parts of its own inherits the anchor's.
func RecurrenceAnchor(now time.Time, dueAt *time.Time, loc *time.Location) time.Time {
	if loc == nil {
		loc = time.UTC
	}
	anchor := now.In(loc)
	if dueAt != nil && dueAt.After(anchor) {
		anchor = dueAt.In(loc)
	}
	return anchor
}

// NextOccurrence returns the first occurrence of rule strictly after anchor.
//
// The anchor doubles as the series start, so every rule is read relative to the
// task's own position rather than to a start date the tasks table does not
// keep. A zero time means the series has run out — the caller closes the task
// for good instead of scheduling it again.
func NextOccurrence(rule string, anchor time.Time) (time.Time, error) {
	r, err := rrule.StrToRRule(rule)
	if err != nil {
		return time.Time{}, err
	}
	r.DTStart(anchor)
	return r.After(anchor, false), nil
}
