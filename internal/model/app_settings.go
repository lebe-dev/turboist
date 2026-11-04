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
	InboxProcessing    InboxProcessingSettings `json:"inboxProcessing"`
	LinkTitles         LinkTitlesSettings      `json:"linkTitles"`
}

// LinkTitleRule overrides the title template for one domain: a task whose URL
// host is Domain or any of its subdomains is titled with Template instead of
// the default one. Templates carry a {title} placeholder.
type LinkTitleRule struct {
	Domain   string `json:"domain"`
	Template string `json:"template"`
}

// LinkTitlesSettings configures the link-title job: a task created with a bare
// https:// URL as its title gets the page's title and the URL moves into the
// description. Off by default.
type LinkTitlesSettings struct {
	Enabled bool `json:"enabled"`
	// Template is the default title template. Empty means "use the built-in
	// default for the user's language", so an untouched setting keeps following
	// the default across releases.
	Template string          `json:"template"`
	Rules    []LinkTitleRule `json:"rules"`
}
