package model

type Priority string

const (
	PriorityHigh   Priority = "high"
	PriorityMedium Priority = "medium"
	PriorityLow    Priority = "low"
	PriorityNone   Priority = "no-priority"
)

func (p Priority) IsValid() bool {
	switch p {
	case PriorityHigh, PriorityMedium, PriorityLow, PriorityNone:
		return true
	}
	return false
}

type TaskStatus string

const (
	TaskStatusOpen      TaskStatus = "open"
	TaskStatusCompleted TaskStatus = "completed"
	TaskStatusCancelled TaskStatus = "cancelled"
)

func (s TaskStatus) IsValid() bool {
	switch s {
	case TaskStatusOpen, TaskStatusCompleted, TaskStatusCancelled:
		return true
	}
	return false
}

type ProjectType string

const (
	ProjectTypeGeneric  ProjectType = "generic"
	ProjectTypeSoftware ProjectType = "software"
)

func (t ProjectType) IsValid() bool {
	switch t {
	case ProjectTypeGeneric, ProjectTypeSoftware:
		return true
	}
	return false
}

type ProjectStatus string

const (
	ProjectStatusOpen      ProjectStatus = "open"
	ProjectStatusCompleted ProjectStatus = "completed"
	ProjectStatusArchived  ProjectStatus = "archived"
	ProjectStatusCancelled ProjectStatus = "cancelled"
)

func (s ProjectStatus) IsValid() bool {
	switch s {
	case ProjectStatusOpen, ProjectStatusCompleted, ProjectStatusArchived, ProjectStatusCancelled:
		return true
	}
	return false
}

type DayPart string

const (
	DayPartNone      DayPart = "none"
	DayPartMorning   DayPart = "morning"
	DayPartAfternoon DayPart = "afternoon"
	DayPartEvening   DayPart = "evening"
)

func (d DayPart) IsValid() bool {
	switch d {
	case DayPartNone, DayPartMorning, DayPartAfternoon, DayPartEvening:
		return true
	}
	return false
}

type PlanState string

const (
	PlanStateNone    PlanState = "none"
	PlanStateWeek    PlanState = "week"
	PlanStateBacklog PlanState = "backlog"
)

func (p PlanState) IsValid() bool {
	switch p {
	case PlanStateNone, PlanStateWeek, PlanStateBacklog:
		return true
	}
	return false
}

type TroikiCategory string

const (
	TroikiCategoryImportant TroikiCategory = "important"
	TroikiCategoryMedium    TroikiCategory = "medium"
	TroikiCategoryRest      TroikiCategory = "rest"
)

func (c TroikiCategory) IsValid() bool {
	switch c {
	case TroikiCategoryImportant, TroikiCategoryMedium, TroikiCategoryRest:
		return true
	}
	return false
}

// RelationType is the kind of link between two tasks. `related` is symmetric and
// purely informational; `blocks` is directed and enforced — a task cannot be
// completed while any task blocking it is still open.
type RelationType string

const (
	RelationTypeRelated RelationType = "related"
	RelationTypeBlocks  RelationType = "blocks"
)

func (r RelationType) IsValid() bool {
	switch r {
	case RelationTypeRelated, RelationTypeBlocks:
		return true
	}
	return false
}

// RelationDirection expresses a `blocks` relation from the point of view of the
// task being looked at: outgoing = this task blocks the peer, incoming = the peer
// blocks this task. Meaningless for `related`, which is symmetric.
type RelationDirection string

const (
	RelationDirectionOutgoing RelationDirection = "outgoing"
	RelationDirectionIncoming RelationDirection = "incoming"
)

func (d RelationDirection) IsValid() bool {
	switch d {
	case RelationDirectionOutgoing, RelationDirectionIncoming:
		return true
	}
	return false
}

type ClientKind string

const (
	ClientWeb     ClientKind = "web"
	ClientIOS     ClientKind = "ios"
	ClientCLI     ClientKind = "cli"
	ClientAndroid ClientKind = "android"
)

func (c ClientKind) IsValid() bool {
	switch c {
	case ClientWeb, ClientIOS, ClientCLI, ClientAndroid:
		return true
	}
	return false
}
