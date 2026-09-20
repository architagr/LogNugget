package config

import (
	"sync/atomic"

	"github.com/architagr/lognugget/v4/enum"
)

// syncModeAtomic gates the synchronous dispatch path. Read on every log call,
// so it is an atomic.Bool rather than a mutex-guarded field.
var syncModeAtomic atomic.Bool

// SetSyncMode selects how a published record reaches the registered hooks.
//
// Default (false): the record is pushed onto the lock-free MPSC ring and a
// background goroutine delivers it. The caller returns without touching the
// sink. This is the behaviour the rest of the library is designed around, and
// the reason handler latency is independent of how slow the sink is.
//
// When true: the caller delivers the record to the pre-processors itself and
// the ring is not used at all. The ring push (~94 ns under 8 producers)
// disappears from the hot path; in exchange the caller now pays for whatever
// the hooks do, which for the built-in collector is an arena copy under a
// mutex, and a write whenever a batch fills.
//
// why hooks still run: an earlier design for this option wrote straight to the
// configured io.Writer, which would have silently stopped every registered
// hook from receiving records and bypassed the collector's batching. Running
// the existing dispatch inline removes the ring without changing what a record
// does once published.
//
// Choose sync mode only when the sink is fast and local (an in-memory buffer,
// a discard writer, a file on a warm page cache) and single-call latency
// matters more than isolation from the sink. With a network sink it is the
// wrong choice: every logging goroutine then blocks on that sink, which is the
// failure mode LogNugget exists to avoid (see the Loki benchmark in the
// README, where the synchronous logger sustains 1,094 rps against LogNugget's
// 6,016).
//
// Safe to call at any time, including while other goroutines are logging.
// Records already in the ring are still drained by the consumer.
func SetSyncMode(on bool) {
	syncModeAtomic.Store(on)
}

// SyncMode reports whether the synchronous dispatch path is active.
func SyncMode() bool {
	return syncModeAtomic.Load()
}

// publishSync delivers e to the current pre-processor snapshot on the caller's
// goroutine and returns the buffer to the pool.
func publishSync(level enum.LogLevel, data []byte) {
	dispatchTo(LogEvent{Level: level, Data: data}, atomicProcsSlice.Load())
	ReturnDispatchBuf(data)
}
