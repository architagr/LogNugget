package customTime

import "time"

func TimeNow() time.Time {
	return time.Now().UTC()
}

func Format(t time.Time, format string) string {
	return t.Format(format)
}
