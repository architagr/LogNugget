//go:build testing

package entry

import (
	"context"

	"github.com/architagr/lognugget/enum"
)

// LogWithBadSkip calls logWithSkip with an artificially large skip depth so
// that runtime.Caller returns ok=false. The resulting log line contains
// caller="unknown". Used exclusively by TS-06 to drive acceptance criterion #3.
//
// This function is test-only (build tag `testing`) and must not appear in
// production binaries.
func LogWithBadSkip(ctx context.Context, message string) {
	e := NewLogEntry()
	e.logWithSkip(enum.LevelInfo, ctx, message, nil, 10_000)
}
