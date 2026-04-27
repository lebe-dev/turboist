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

type ClientKind string

const (
	ClientWeb ClientKind = "web"
	ClientIOS ClientKind = "ios"
	ClientCLI ClientKind = "cli"
)

func (c ClientKind) IsValid() bool {
	switch c {
	case ClientWeb, ClientIOS, ClientCLI:
		return true
	}
	return false
}
