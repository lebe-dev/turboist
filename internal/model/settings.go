package model

// UserSettings holds user-configurable application preferences persisted on the server.
type UserSettings struct {
	WeeklyUnplannedExcludedLabelIDs []int64 `json:"weeklyUnplannedExcludedLabelIds"`
	Locale                          string  `json:"locale"`
	PublicView                      bool    `json:"publicView"`
	BannerText                      string  `json:"bannerText"`
	BannerPublished                 bool    `json:"bannerPublished"`
}
