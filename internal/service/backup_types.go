package service

import (
	"github.com/lebe-dev/turboist/internal/model"
)

// BackupSchemaVersion is the on-disk schema version for backup payloads. Bump
// when the JSON structure changes in a non-additive way.
const BackupSchemaVersion = 1

// BackupPayload is the full export envelope written to disk and consumed on
// restore. Times are encoded as the same UTC strings used in the SQLite schema
// so a round-trip preserves the database byte-for-byte (modulo autoincrement
// sequence rows). The Settings field is optional and controlled by the
// IncludeSettings export option.
type BackupPayload struct {
	Version    int           `json:"version"`
	ExportedAt string        `json:"exportedAt"`
	Data       BackupData    `json:"data"`
	Settings   *BackupConfig `json:"settings,omitempty"`
}

type BackupData struct {
	Contexts        []BackupContext        `json:"contexts"`
	Labels          []BackupLabel          `json:"labels"`
	Projects        []BackupProject        `json:"projects"`
	ProjectSections []BackupProjectSection `json:"projectSections"`
	Tasks           []BackupTask           `json:"tasks"`
	TaskLabels      []BackupTaskLabel      `json:"taskLabels"`
	ProjectLabels   []BackupProjectLabel   `json:"projectLabels"`
}

type BackupConfig struct {
	User *model.UserSettings `json:"user,omitempty"`
	App  *model.AppSettings  `json:"app,omitempty"`
}

type BackupContext struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Color       string `json:"color"`
	IsFavourite bool   `json:"isFavourite"`
	CreatedAt   string `json:"createdAt"`
	UpdatedAt   string `json:"updatedAt"`
}

type BackupLabel struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Color       string `json:"color"`
	IsFavourite bool   `json:"isFavourite"`
	IsPrivate   bool   `json:"isPrivate"`
	CreatedAt   string `json:"createdAt"`
	UpdatedAt   string `json:"updatedAt"`
}

type BackupProject struct {
	ID             int64   `json:"id"`
	ContextID      int64   `json:"contextId"`
	Title          string  `json:"title"`
	Description    string  `json:"description"`
	Color          string  `json:"color"`
	Status         string  `json:"status"`
	IsPinned       bool    `json:"isPinned"`
	PinnedAt       *string `json:"pinnedAt,omitempty"`
	IsPrivate      bool    `json:"isPrivate"`
	ProjectType    string  `json:"projectType"`
	TroikiCategory *string `json:"troikiCategory,omitempty"`
	CreatedAt      string  `json:"createdAt"`
	UpdatedAt      string  `json:"updatedAt"`
}

type BackupProjectSection struct {
	ID        int64  `json:"id"`
	ProjectID int64  `json:"projectId"`
	Title     string `json:"title"`
	Position  int    `json:"position"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

type BackupTask struct {
	ID                    int64   `json:"id"`
	Title                 string  `json:"title"`
	Description           string  `json:"description"`
	InboxID               *int64  `json:"inboxId,omitempty"`
	ContextID             *int64  `json:"contextId,omitempty"`
	ProjectID             *int64  `json:"projectId,omitempty"`
	SectionID             *int64  `json:"sectionId,omitempty"`
	ParentID              *int64  `json:"parentId,omitempty"`
	Priority              string  `json:"priority"`
	Status                string  `json:"status"`
	DueAt                 *string `json:"dueAt,omitempty"`
	DueHasTime            bool    `json:"dueHasTime"`
	DeadlineAt            *string `json:"deadlineAt,omitempty"`
	DeadlineHasTime       bool    `json:"deadlineHasTime"`
	DayPart               string  `json:"dayPart"`
	PlanState             string  `json:"planState"`
	IsPinned              bool    `json:"isPinned"`
	PinnedAt              *string `json:"pinnedAt,omitempty"`
	IsPrivate             bool    `json:"isPrivate"`
	RecurrenceRule        *string `json:"recurrenceRule,omitempty"`
	CompletedAt           *string `json:"completedAt,omitempty"`
	PostponeCount         int     `json:"postponeCount"`
	TroikiCategory        *string `json:"troikiCategory,omitempty"`
	TroikiCapacityGranted bool    `json:"troikiCapacityGranted"`
	SourceTaskID          *int64  `json:"sourceTaskId,omitempty"`
	CreatedAt             string  `json:"createdAt"`
	UpdatedAt             string  `json:"updatedAt"`
}

type BackupTaskLabel struct {
	TaskID  int64 `json:"taskId"`
	LabelID int64 `json:"labelId"`
}

type BackupProjectLabel struct {
	ProjectID int64 `json:"projectId"`
	LabelID   int64 `json:"labelId"`
}

// ExportOptions controls what is included in the backup.
type ExportOptions struct {
	// IncludeSettings adds per-user and global app settings to the payload.
	IncludeSettings bool
}
