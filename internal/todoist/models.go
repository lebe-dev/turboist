package todoist

// TaskAddArgs are arguments for creating a new task.
type TaskAddArgs struct {
	Content     string
	Description string
	ProjectID   string
	SectionID   string
	ParentID    string
	Labels      []string
	Priority    int
	DueDate     string // "2006-01-02" format
	DueString   string // human-readable due string
	DueLang     string // language for parsing DueString
}

// TaskUpdateArgs are arguments for updating an existing task.
type TaskUpdateArgs struct {
	ID          string
	Content     *string
	Description *string
	Labels      []string
	Priority    *int
	DueDate     *string // "2006-01-02" format, empty string to clear
	DueString   *string // human-readable due string
	DueLang     *string // language for parsing DueString
}

// syncDue represents a due date in the Todoist Sync API response.
type syncDue struct {
	Date        string `json:"date"`
	IsRecurring bool   `json:"is_recurring"`
	String      string `json:"string"`
	Lang        string `json:"lang"`
}

// syncItem represents a task in the Todoist Sync API response.
type syncItem struct {
	ID          string   `json:"id"`
	Content     string   `json:"content"`
	Description string   `json:"description"`
	ProjectID   string   `json:"project_id"`
	SectionID   *string  `json:"section_id"`
	ParentID    *string  `json:"parent_id"`
	Labels      []string `json:"labels"`
	Priority    int      `json:"priority"`
	Due         *syncDue `json:"due"`
	Checked     bool     `json:"checked"`
	IsDeleted   bool     `json:"is_deleted"`
	AddedAt     string   `json:"added_at"`
	CompletedAt *string  `json:"completed_at"`
}

// syncProject represents a project in the Todoist Sync API response.
type syncProject struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	InboxProject bool   `json:"inbox_project"`
	IsDeleted    bool   `json:"is_deleted"`
	IsArchived   bool   `json:"is_archived"`
}

// syncSection represents a section in the Todoist Sync API response.
type syncSection struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	ProjectID string `json:"project_id"`
	IsDeleted bool   `json:"is_deleted"`
}

// syncLabel represents a label in the Todoist Sync API response.
type syncLabel struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	IsDeleted bool   `json:"is_deleted"`
}

// Due represents task due date.
type Due struct {
	Date      string `json:"date"`
	Recurring bool   `json:"recurring"`
}

// Task is an internal representation of a Todoist task.
type Task struct {
	ID                    string   `json:"id"`
	Content               string   `json:"content"`
	Description           string   `json:"description"`
	ProjectID             string   `json:"project_id"`
	SectionID             *string  `json:"section_id"`
	ParentID              *string  `json:"parent_id"`
	Labels                []string `json:"labels"`
	Priority              int      `json:"priority"`
	Due                   *Due     `json:"due"`
	SubTaskCount          int      `json:"sub_task_count"`
	CompletedSubTaskCount int      `json:"completed_sub_task_count"`
	CompletedAt           *string  `json:"completed_at"`
	AddedAt               string   `json:"added_at"`
	Children              []*Task  `json:"children"`
	PostponeCount         int      `json:"postpone_count"`
	ExpiresAt             *string  `json:"expires_at,omitempty"`
}

// Project is an internal representation of a Todoist project.
type Project struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	IsInbox bool   `json:"is_inbox,omitempty"`
}

// Section is an internal representation of a Todoist section.
type Section struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	ProjectID string `json:"project_id"`
}

// DecomposeOpts holds optional overrides for decomposed tasks.
type DecomposeOpts struct {
	Priority *int
	DueDate  *string // "2006-01-02" format
}

// MoveTarget describes where a task should be moved.
// When SectionID is non-empty, the task moves to that section (project is implicit).
// When SectionID is empty, the task moves to ProjectID only.
type MoveTarget struct {
	ProjectID string
	SectionID string
}

// Label is an internal representation of a Todoist label.
type Label struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// DeltaResult holds incremental sync changes.
// If FullSync is true, the server returned a full dataset (token expired) and
// Result contains the complete state — delta fields should be ignored.
type DeltaResult struct {
	FullSync          bool
	Result            *SyncResult
	UpsertedTasks     []*Task
	RemovedTaskIDs    []string
	UpsertedProjects  []*Project
	RemovedProjectIDs []string
	UpsertedSections  []*Section
	RemovedSectionIDs []string
	UpsertedLabels    []*Label
	RemovedLabelIDs   []string
}

// TaskFromSync maps a syncItem to our internal Task model.
func TaskFromSync(t *syncItem) *Task {
	task := &Task{
		ID:          t.ID,
		Content:     t.Content,
		Description: t.Description,
		ProjectID:   t.ProjectID,
		SectionID:   t.SectionID,
		ParentID:    t.ParentID,
		Labels:      t.Labels,
		Priority:    t.Priority,
		CompletedAt: t.CompletedAt,
		AddedAt:     t.AddedAt,
		Children:    []*Task{},
	}

	if t.Due != nil && t.Due.Date != "" {
		date := t.Due.Date
		// Extract date-only part from full datetime strings
		if len(date) > 10 {
			date = date[:10]
		}
		task.Due = &Due{
			Date:      date,
			Recurring: t.Due.IsRecurring,
		}
	}

	if task.Labels == nil {
		task.Labels = []string{}
	}

	return task
}

// ProjectFromSync maps a syncProject to our internal Project model.
func ProjectFromSync(p *syncProject) *Project {
	return &Project{
		ID:      p.ID,
		Name:    p.Name,
		IsInbox: p.InboxProject,
	}
}

// SectionFromSync maps a syncSection to our internal Section model.
func SectionFromSync(s *syncSection) *Section {
	return &Section{
		ID:        s.ID,
		Name:      s.Name,
		ProjectID: s.ProjectID,
	}
}

// LabelFromSync maps a syncLabel to our internal Label model.
func LabelFromSync(l *syncLabel) *Label {
	return &Label{
		ID:   l.ID,
		Name: l.Name,
	}
}
