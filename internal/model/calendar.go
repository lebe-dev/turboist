package model

import "time"

type CalendarProvider string

const CalendarProviderGoogle CalendarProvider = "google"

type CalendarAccount struct {
	ID           int64
	UserID       int64
	Provider     CalendarProvider
	Email        string
	DisplayName  string
	AccessToken  string
	RefreshToken string
	Expiry       time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type CalendarSource struct {
	ID         int64
	AccountID  int64
	UserID     int64
	Provider   CalendarProvider
	ExternalID string
	Summary    string
	Color      string
	Selected   bool
	IsPrimary  bool
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type CalendarOAuthState struct {
	State     string
	UserID    int64
	Provider  CalendarProvider
	ExpiresAt time.Time
	CreatedAt time.Time
}
