package entry

import (
	"sync"

	"github.com/architagr/lognugget/v4/config"
)

// entryPool is the package-level sync.Pool backing NewLogEntry and Put.
// The New func is set in init so the pool always returns a fully initialised
// *LogEntry even on the first cold Get.
var entryPool sync.Pool

func init() {
	entryPool = sync.Pool{
		New: func() any {
			return initLogEntry()
		},
	}
}

// initLogEntry allocates a new *LogEntry with 1 heap allocation.
// buf comes from dispatchBufPool (0 extra allocs when pool is warm);
// pendingBuf is backed by the inline pendingBufSlab — no separate alloc (V4-P3).
// It is called exclusively by the pool's New func and by GenerateInitialPool;
// callers outside the pool lifecycle must use NewLogEntry instead.
func initLogEntry() *LogEntry {
	e := &LogEntry{}
	e.buf = config.GetDispatchBuf()
	e.pendingBuf = e.pendingBufSlab[:0]
	return e
}

// GenerateInitialPool pre-warms the internal sync.Pool with n ready-to-use
// *LogEntry instances. Call once at program startup — before the first burst
// of concurrent log calls — to avoid cold-start allocation pressure.
//
// Sizing recommendation: n = runtime.GOMAXPROCS(0) * 64 covers the typical
// burst window on a fully-loaded machine without over-allocating on single-
// core deployments. Tune upward if your p99 latency at startup shows pool
// misses (observable via runtime.ReadMemStats, NumMallocs delta).
//
// GenerateInitialPool(0) is a safe no-op.
func GenerateInitialPool(n int) {
	for i := 0; i < n; i++ {
		entryPool.Put(initLogEntry())
	}
}
