package model

// UserSettings holds user-configurable application preferences persisted on the server.
type UserSettings struct {
	WeeklyUnplannedExcludedLabelIDs []int64 `json:"weeklyUnplannedExcludedLabelIds"`
}
