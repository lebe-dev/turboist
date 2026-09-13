package inboxproc

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/lebe-dev/turboist/internal/model"
)

// MinConfidence is the threshold under which a "sort" answer is treated as
// "keep": filing a task in the wrong place is worse than leaving it.
const MinConfidence = 0.5

// ErrInvalidDecision is an answer that parsed but cannot be applied — the model
// named a project it was never offered, or an action that does not exist. It is
// a model error, not a deliberate "keep", so the task is retried with backoff.
var ErrInvalidDecision = errors.New("inboxproc: invalid decision")

const (
	actionSort = "sort"
	actionKeep = "keep"
)

// Decision is the model's answer as described by the output contract.
type Decision struct {
	Action     string  `json:"action"`
	ProjectID  *int64  `json:"projectId"`
	LabelIDs   []int64 `json:"labelIds"`
	Priority   *string `json:"priority"`
	DueDate    *string `json:"dueDate"`
	Confidence float64 `json:"confidence"`
	Reason     string  `json:"reason"`
}

// Verdict is a validated decision, expressed in terms the processor can apply.
type Verdict struct {
	Keep       bool
	Project    *model.Project
	LabelIDs   []int64
	Priority   *model.Priority
	DueAt      *time.Time
	Confidence float64
	Reason     string
	// Warnings name the parts of the answer that were dropped.
	Warnings []string
}

// ParseDecision extracts the first JSON object from the model's text. Models
// wrap answers in markdown fences or a sentence often enough that a strict
// json.Unmarshal of the whole content would fail needlessly.
func ParseDecision(content string) (Decision, error) {
	obj, ok := firstJSONObject(content)
	if !ok {
		return Decision{}, fmt.Errorf("%w: no JSON object in the answer", ErrBadResponse)
	}
	var d Decision
	if err := json.Unmarshal([]byte(obj), &d); err != nil {
		return Decision{}, fmt.Errorf("%w: decode answer: %v", ErrBadResponse, err)
	}
	d.Action = strings.ToLower(strings.TrimSpace(d.Action))
	d.Reason = strings.TrimSpace(d.Reason)
	return d, nil
}

// firstJSONObject returns the first balanced {...} span, skipping braces that
// sit inside JSON strings.
func firstJSONObject(s string) (string, bool) {
	start := strings.IndexByte(s, '{')
	if start < 0 {
		return "", false
	}
	depth := 0
	inString := false
	escaped := false
	for i := start; i < len(s); i++ {
		ch := s[i]
		if inString {
			switch {
			case escaped:
				escaped = false
			case ch == '\\':
				escaped = true
			case ch == '"':
				inString = false
			}
			continue
		}
		switch ch {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return s[start : i+1], true
			}
		}
	}
	return "", false
}

// Validate checks the decision against the catalogue it was made from.
func (d Decision) Validate(c *Catalogue, now time.Time, loc *time.Location) (Verdict, error) {
	if loc == nil {
		loc = time.UTC
	}
	v := Verdict{Confidence: d.Confidence, Reason: d.Reason}
	switch d.Action {
	case actionKeep:
		v.Keep = true
		return v, nil
	case actionSort:
	default:
		return v, fmt.Errorf("%w: unknown action %q", ErrInvalidDecision, d.Action)
	}
	if d.Confidence < MinConfidence {
		v.Keep = true
		return v, nil
	}
	if d.ProjectID == nil {
		return v, fmt.Errorf("%w: sort without a project", ErrInvalidDecision)
	}
	project, ok := c.Project(*d.ProjectID)
	if !ok {
		return v, fmt.Errorf("%w: unknown project %d", ErrInvalidDecision, *d.ProjectID)
	}
	v.Project = project

	seen := make(map[int64]struct{}, len(d.LabelIDs))
	v.LabelIDs = make([]int64, 0, len(d.LabelIDs))
	for _, id := range d.LabelIDs {
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		if _, ok := c.Label(id); !ok {
			v.Warnings = append(v.Warnings, fmt.Sprintf("unknown label %d dropped", id))
			continue
		}
		v.LabelIDs = append(v.LabelIDs, id)
	}

	if d.Priority != nil {
		p := model.Priority(strings.ToLower(strings.TrimSpace(*d.Priority)))
		switch {
		case !p.IsValid():
			v.Warnings = append(v.Warnings, fmt.Sprintf("invalid priority %q ignored", *d.Priority))
		case project.TroikiCategory != nil:
			// The daily-plan bucket pins the priority of its projects; MoveService
			// applies it, and the model's opinion would only be overwritten.
		default:
			v.Priority = &p
		}
	}

	if d.DueDate != nil && strings.TrimSpace(*d.DueDate) != "" {
		due, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(*d.DueDate), loc)
		local := now.In(loc)
		today := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
		switch {
		case err != nil:
			v.Warnings = append(v.Warnings, fmt.Sprintf("unparseable due date %q ignored", *d.DueDate))
		case due.Before(today):
			v.Warnings = append(v.Warnings, fmt.Sprintf("past due date %q ignored", *d.DueDate))
		default:
			v.DueAt = &due
		}
	}
	return v, nil
}
