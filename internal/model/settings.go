package model

// UserSettings holds user-configurable application preferences persisted on the server.
type UserSettings struct {
	WeeklyUnplannedExcludedLabelIDs []int64 `json:"weeklyUnplannedExcludedLabelIds"`
	BugLabelIDs                     []int64 `json:"bugLabelIds"`
	Locale                          string  `json:"locale"`
	PublicView                      bool    `json:"publicView"`
	BannerText                      string  `json:"bannerText"`
	BannerPublished                 bool    `json:"bannerPublished"`
	CalendarEnabled                 bool    `json:"calendarEnabled"`
}
