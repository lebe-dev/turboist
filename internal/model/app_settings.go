package model

// AutoLabelRule is a single auto-label match rule: when a task title contains
// Mask, the labels with the listed IDs are attached. If IgnoreCase is true,
// comparison is case-insensitive. Labels are never auto-created — IDs must
// reference existing labels; missing IDs are silently skipped at apply time.
type AutoLabelRule struct {
	Mask       string  `json:"mask"`
	LabelIDs   []int64 `json:"labelIds"`
	IgnoreCase bool    `json:"ignoreCase"`
}

// ProjectSuggestionRule is a single project-suggestion match rule: when a task
// title contains Mask, the projects with the listed IDs are offered to the user
// as a choice. If IgnoreCase is true, comparison is case-insensitive. Unlike
// auto-labels, nothing is applied automatically — the rules only feed the
// suggestion chips in the quick-add dialog; missing IDs are silently skipped.
type ProjectSuggestionRule struct {
	Mask       string  `json:"mask"`
	ProjectIDs []int64 `json:"projectIds"`
	IgnoreCase bool    `json:"ignoreCase"`
}

// AppSettings holds global, server-wide settings persisted in the app_settings
// table (single-row, id=1).
type AppSettings struct {
	AutoLabels         []AutoLabelRule         `json:"autoLabels"`
	ProjectSuggestions []ProjectSuggestionRule `json:"projectSuggestions"`
}
