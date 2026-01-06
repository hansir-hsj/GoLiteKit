package logger

import "time"

// Truncate to the minute boundary
func truncateToMinute(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), 0, 0, t.Location())
}

// Truncate to the specified minute interval boundary (e.g. 5 minutes: 12:07 -> 12:05)
func truncateToMinuteInterval(t time.Time, interval int) time.Time {
	aligned := (t.Minute() / interval) * interval
	return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), aligned, 0, 0, t.Location())
}

// Truncate to the hour boundary
func truncateToHour(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), 0, 0, 0, t.Location())
}

// Truncate to the day boundary
func truncateToDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}
