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
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type Project struct {
	ID          int64
	ContextID   int64
	Title       string
	Description string
	Color       string
	Status      ProjectStatus
	IsPinned    bool
	PinnedAt    *time.Time
	Labels      []Label
	CreatedAt   time.Time
	UpdatedAt   time.Time
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

	CompletedAt *time.Time

	RecurrenceRule *string

	PostponeCount int

	TroikiCategory *TroikiCategory

	Labels []Label

	CreatedAt time.Time
	UpdatedAt time.Time
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
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

type Session struct {
	ID         int64
	UserID     int64
	TokenHash  string
	ClientKind ClientKind
	UserAgent  string
	CreatedAt  time.Time
	LastUsedAt time.Time
	ExpiresAt  time.Time
	RevokedAt  *time.Time
}

func (s *Session) IsActive(now time.Time) bool {
	return s.RevokedAt == nil && now.Before(s.ExpiresAt)
}
