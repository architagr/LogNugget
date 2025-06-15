package lognugget

import (
	"fmt"
	"log/slog"
)

type LogLevel int

const (
	LevelDebug LogLevel = 0
	LevelInfo  LogLevel = 2
	LevelWarn  LogLevel = 4
	LevelError LogLevel = 8
	LevelFatal LogLevel = 16
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
func (l LogLevel) ToSlogLeveler() slog.Leveler {
	switch l {
	case LevelDebug:
		return slog.LevelDebug
	case LevelInfo:
		return slog.LevelInfo
	case LevelWarn:
		return slog.LevelWarn
	case LevelError:
		return slog.LevelError
	case LevelFatal:
		return slog.LevelError
	default:
		return slog.LevelInfo // Default to Info level if unknown
	}
}
