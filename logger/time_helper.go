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

// nextRotation returns the next rotation time after t, or zero if rule never
// rotates. The loop keeps the result after t across DST changes.
func nextRotation(t time.Time, rule string) time.Time {
	var next time.Time
	var period time.Duration
	switch rule {
	case "1min":
		period = time.Minute
		next = truncateToMinute(t)
	case "5min":
		period = 5 * time.Minute
		next = truncateToMinuteInterval(t, 5)
	case "10min":
		period = 10 * time.Minute
		next = truncateToMinuteInterval(t, 10)
	case "30min":
		period = 30 * time.Minute
		next = truncateToMinuteInterval(t, 30)
	case "1hour":
		period = time.Hour
		next = truncateToHour(t)
	case "1day":
		next = truncateToDay(t)
	default:
		return time.Time{}
	}
	for !next.After(t) {
		if period == 0 {
			next = next.AddDate(0, 0, 1)
		} else {
			next = next.Add(period)
		}
	}
	return next
}

// rotationDue reports whether now has reached next (a nextRotation result).
func rotationDue(next, now time.Time) bool {
	return !next.IsZero() && !now.Before(next)
}
