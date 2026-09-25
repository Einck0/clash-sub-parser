package domain

import (
	"time"
)

// NowUTC returns the current time truncated to millisecond in UTC.
func NowUTC() time.Time {
	return time.Now().UTC().Truncate(time.Millisecond)
}

// FormatTime formats a time.Time in RFC 3339 UTC.
func FormatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}

// ParseTime parses an RFC 3339 string and returns UTC time.
func ParseTime(s string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}, NewValidationError("invalid_time_format", "invalid RFC 3339 time string")
	}
	return t.UTC(), nil
}
