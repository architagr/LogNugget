package enum

import (
	"fmt"
)

type LogLevel int

const (
	LevelUnSet LogLevel = 1 << iota
	LevelDebug LogLevel = 1 << iota
	LevelInfo  LogLevel = 1 << iota
	LevelWarn  LogLevel = 1 << iota
	LevelError LogLevel = 1 << iota
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

	switch {
	case l < LevelInfo:
		return str("DEBUG", l-LevelDebug)
	case l < LevelWarn:
		return str("INFO", l-LevelInfo)
	case l < LevelError:
		return str("WARN", l-LevelWarn)
	default:
		return str("ERROR", l-LevelError)
	}
}
