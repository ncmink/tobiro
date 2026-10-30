package update

import "time"

// AutoCheckMinInterval is how long to wait between automatic GitHub release checks.
const AutoCheckMinInterval = 24 * time.Hour

// DueForAutoCheck reports whether an automatic check should run now.
func DueForAutoCheck(lastCheckRFC3339 string, now time.Time) bool {
	if lastCheckRFC3339 == "" {
		return true
	}
	last, err := time.Parse(time.RFC3339, lastCheckRFC3339)
	if err != nil {
		return true
	}
	return !now.Before(last.Add(AutoCheckMinInterval))
}
