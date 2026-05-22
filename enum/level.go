package enum

import (
	"fmt"
)

// LogLevel is a numeric severity level used to filter and label log entries.
// Higher values are more severe. Use the named constants (LevelDebug through
// LevelFatal) rather than raw integers.
type LogLevel int

const (
	// LevelUnSet is the zero-config catch-all level that matches every log event.
	LevelUnSet LogLevel = 1 << iota
	// LevelDebug is the lowest named severity, intended for verbose diagnostic output.
	LevelDebug LogLevel = 1 << iota
	// LevelInfo is the standard informational severity for routine operational messages.
	LevelInfo LogLevel = 1 << iota
	// LevelWarn indicates a recoverable condition that may warrant attention.
	LevelWarn LogLevel = 1 << iota
	// LevelError indicates a failure that has been handled but should be investigated.
	LevelError LogLevel = 1 << iota
	// LevelFatal is the highest named severity; callers typically exit after logging at this level.
	LevelFatal LogLevel = 1 << iota
)

// String returns a name for the level.
// If the level has a name, then that name
// in uppercase is returned.
// If the level is between named values, then
// an integer is appended to the uppercased name.
// Examples:
//
//	LevelWarn.String() => "WARN"
//	(LevelInfo+2).String() => "INFO+2"
func (l LogLevel) String() string {
	str := func(base string, val LogLevel) string {
		if val == 0 {
			return base
		}
		return fmt.Sprintf("%s%+d", base, val)
	}

	switch l {
	case LevelInfo:
		return "INFO"
	case LevelDebug:
		return "DEBUG"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	case LevelFatal:
		return "FATAL"
	default:
		return str("ERROR", l-LevelError)
	}
}
