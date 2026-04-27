package model

import "time"

const TimeFormat = "2006-01-02T15:04:05.000Z"

func FormatUTC(t time.Time) string {
	return t.UTC().Format(TimeFormat)
}

func ParseUTC(s string) (time.Time, error) {
	return time.Parse(TimeFormat, s)
}
