package model

import (
	"crypto/sha256"
	"encoding/hex"
	"time"
)

// InboxProcessingSettings is the server-wide part of the LLM Inbox processor
// that is edited from the UI. Provider, key and model live in env only.
type InboxProcessingSettings struct {
	// Prompt is the editable system-message template (Go text/template). An empty
	// string means "use the built-in default", so improvements to the default
	// reach everyone who never customised it.
	Prompt string `json:"prompt"`
	// Paused stops the scheduled runs without touching the environment
	// configuration. A manual "process now" still runs.
	Paused bool `json:"paused"`
}

// InboxStateStatus is why a task that is still in the Inbox is not pending.
type InboxStateStatus string

const (
	InboxStateKept     InboxStateStatus = "kept"
	InboxStateFailed   InboxStateStatus = "failed"
	InboxStateReverted InboxStateStatus = "reverted"
)

// InboxOutcome is the kind of a decision journal row.
type InboxOutcome string

const (
	InboxOutcomeSorted InboxOutcome = "sorted"
	InboxOutcomeKept   InboxOutcome = "kept"
	InboxOutcomeFailed InboxOutcome = "failed"
)

// InboxProcessingState is the processor's memory about one Inbox task.
type InboxProcessingState struct {
	TaskID      int64
	Fingerprint string
	Status      InboxStateStatus
	Attempts    int
	// NextAttemptAt is set only for failed tasks still inside their retry budget;
	// nil means "wait until the task is edited".
	NextAttemptAt *time.Time
	LastError     *string
	UpdatedAt     time.Time
}

// InboxTaskBefore is what the processor needs to put a filed task back.
type InboxTaskBefore struct {
	LabelIDs   []int64    `json:"labelIds"`
	Priority   Priority   `json:"priority"`
	DueAt      *time.Time `json:"dueAt"`
	DueHasTime bool       `json:"dueHasTime"`
}

// InboxTaskAfter is what a sorted decision actually applied.
type InboxTaskAfter struct {
	ContextID int64      `json:"contextId"`
	ProjectID int64      `json:"projectId"`
	LabelIDs  []int64    `json:"labelIds"`
	Priority  Priority   `json:"priority"`
	DueAt     *time.Time `json:"dueAt"`
}

// InboxProcessingLogEntry is one row of the decision journal.
type InboxProcessingLogEntry struct {
	ID               int64
	TaskID           *int64
	TaskTitle        string
	Outcome          InboxOutcome
	Model            string
	Reason           string
	Confidence       *float64
	Before           InboxTaskBefore
	After            *InboxTaskAfter
	Error            *string
	PromptTokens     *int
	CompletionTokens *int
	RevertedAt       *time.Time
	CreatedAt        time.Time
}

// InboxFingerprint identifies the wording a decision was made on. A task whose
// title or description changed since is reconsidered.
func InboxFingerprint(title, description string) string {
	sum := sha256.Sum256([]byte(title + "\n" + description))
	return hex.EncodeToString(sum[:])
}
