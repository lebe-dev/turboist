package model

// HarpoonKind discriminates the entity a harpoon reference points at.
type HarpoonKind string

const (
	HarpoonKindTask    HarpoonKind = "task"
	HarpoonKindProject HarpoonKind = "project"
)

// HarpoonRef is a single harpooned entity. It is stored verbatim in
// UserSettings.Harpoon; the human-readable title is hydrated on read rather
// than persisted, so it never goes stale.
type HarpoonRef struct {
	Kind HarpoonKind `json:"kind"`
	ID   int64       `json:"id"`
}

// UserSettings holds user-configurable application preferences persisted on the server.
type UserSettings struct {
	WeeklyUnplannedExcludedLabelIDs []int64 `json:"weeklyUnplannedExcludedLabelIds"`
	BugLabelIDs                     []int64 `json:"bugLabelIds"`
	Locale                          string  `json:"locale"`
	PublicView                      bool    `json:"publicView"`
	BannerText                      string  `json:"bannerText"`
	BannerPublished                 bool    `json:"bannerPublished"`
	CalendarEnabled                 bool    `json:"calendarEnabled"`
	CalendarHidePastEvents          bool    `json:"calendarHidePastEvents"`
	TroikiEnabled                   bool    `json:"troikiEnabled"`

	// Harpoon is the ordered "jump pair": at most two references the user can
	// quickly hop between. Order is significant — slot 0 is the first member,
	// slot 1 the second. Adding a third reference evicts slot 0 (FIFO).
	Harpoon []HarpoonRef `json:"harpoon,omitempty"`
}
