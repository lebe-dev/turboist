package model

import (
	"strconv"
	"time"
)

type Context struct {
	ID          int64
	Name        string
	Color       string
	IsFavourite bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type Label struct {
	ID          int64
	Name        string
	Color       string
	IsFavourite bool
	IsPrivate   bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type Project struct {
	ID             int64
	ContextID      int64
	Title          string
	Description    string
	Color          string
	Status         ProjectStatus
	Type           ProjectType
	IsPinned       bool
	PinnedAt       *time.Time
	IsPrivate      bool
	TroikiCategory *TroikiCategory
	Labels         []Label
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type ProjectSection struct {
	ID        int64
	ProjectID int64
	Title     string
	Position  int
	CreatedAt time.Time
	UpdatedAt time.Time
}

type Task struct {
	ID          int64
	Title       string
	Description string

	InboxID   *int64
	ContextID *int64
	ProjectID *int64
	SectionID *int64
	ParentID  *int64

	Priority Priority
	Status   TaskStatus

	DueAt           *time.Time
	DueHasTime      bool
	DeadlineAt      *time.Time
	DeadlineHasTime bool

	DayPart   DayPart
	PlanState PlanState

	IsPinned bool
	PinnedAt *time.Time

	IsPrivate bool

	// IsComplex flags a task the user considers hard/demanding; surfaced in the
	// UI with a brain marker. Purely descriptive — no invariants attached.
	IsComplex bool

	CompletedAt *time.Time

	RecurrenceRule *string

	// SourceTaskID points snapshot rows back to the parent recurring task they
	// were created from. Nil for non-snapshot rows.
	SourceTaskID *int64

	PostponeCount int

	TroikiCategory *TroikiCategory

	Labels []Label

	// RelationSummary is the cheap rollup hydrated on every read path (single get
	// and every list view) so a blocked task can be rendered as such without
	// loading the relations themselves.
	RelationSummary TaskRelationSummary

	// Relations is hydrated only on the single-task detail path — list views would
	// pay for a join nobody reads there.
	Relations []TaskRelation

	CreatedAt time.Time
	UpdatedAt time.Time
}

// TaskRelation is one edge of the task graph, resolved relative to the task it
// was loaded for: Direction and Other describe the *peer* end, so the same stored
// row reads as "blocked by X" on one task and "blocks Y" on the other.
type TaskRelation struct {
	ID           int64
	SourceTaskID int64
	TargetTaskID int64
	Type         RelationType
	Direction    RelationDirection
	CreatedAt    time.Time
	Other        *Task
}

// TaskRelationSummary counts what a task's relations imply without listing them.
type TaskRelationSummary struct {
	// BlockedByOpen is how many still-open tasks block this one. Non-zero means
	// completion is refused.
	BlockedByOpen int
	// Total is every relation touching this task, both directions, both types.
	Total int
}

func (t *Task) URL(baseURL string) string {
	return baseURL + "/task/" + strconv.FormatInt(t.ID, 10)
}

type User struct {
	ID                   int64
	Username             string
	PasswordHash         string
	TroikiMediumCapacity int
	TroikiRestCapacity   int
	TroikiStarted        bool
	TOTPSecret           string
	TOTPEnabled          bool
	TOTPEnabledAt        *time.Time
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

type Session struct {
	ID         int64
	UserID     int64
	TokenHash  string
	ClientKind ClientKind
	UserAgent  string
	IPAddress  string
	CreatedAt  time.Time
	LastUsedAt time.Time
	ExpiresAt  time.Time
	RevokedAt  *time.Time
}

func (s *Session) IsActive(now time.Time) bool {
	return s.RevokedAt == nil && now.Before(s.ExpiresAt)
}

type APIToken struct {
	ID        int64
	UserID    int64
	Name      string
	TokenHash string
	Scopes    []string
	CreatedAt time.Time
}

// IdempotencyRecord is a reserved idempotency key. Status 0 means the request
// is still in flight (reserved before the handler ran); a non-zero Status plus
// Response is the stored 2xx result replayed on a duplicate request.
type IdempotencyRecord struct {
	Key       string
	UserID    int64
	Method    string
	Path      string
	Status    int
	Response  string
	CreatedAt time.Time
}
