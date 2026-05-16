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

// AppSettings holds global, server-wide settings persisted in the app_settings
// table (single-row, id=1).
type AppSettings struct {
	AutoLabels []AutoLabelRule `json:"autoLabels"`
}
