// Package customTime provides time utilities for LogNugget log formatting.
// It exposes Now (UTC clock), Format (string return, kept for compatibility),
// and AppendFormat (zero-alloc, preferred on hot paths). It does not own any
// configuration state — callers pass the layout string explicitly.
package customTime

import "time"

// TimeNow returns the current time in UTC. Using UTC ensures log timestamps
// are timezone-neutral and comparable across services.
func TimeNow() time.Time {
	return time.Now().UTC()
}

// Format formats t using layout and returns the result as a new string.
// Prefer AppendFormat on hot paths to avoid the string allocation.
func Format(t time.Time, format string) string {
	return t.Format(format)
}

// AppendFormat appends the textual representation of t, formatted according
// to layout, to dst and returns the extended buffer. It performs zero heap
// allocations when dst has sufficient capacity, making it safe for the hot
// log-formatting path.
//
// why: time.Time.AppendFormat writes directly into the provided byte slice,
// avoiding the string allocation that time.Time.Format incurs on every call.
// On the hot path this saves ~1 alloc / ~30 ns per log entry (see story 037
// bench results).
func AppendFormat(dst []byte, t time.Time, layout string) []byte {
	return t.AppendFormat(dst, layout)
}
