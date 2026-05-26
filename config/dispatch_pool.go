package config

import "sync"

const (
	// DefaultDispatchBufCap is the initial capacity of pooled dispatch buffers.
	// Matches entry.initBufCap (1024 B) so the pooled buffer fits a typical log line
	// without reallocation on the first append inside logWithSkip.
	DefaultDispatchBufCap = 1024

	// MaxDispatchBufRetainCap is the upper cap threshold for recycling dispatch
	// buffers. Buffers larger than this are dropped rather than returned to the
	// pool to prevent unbounded memory retention from rare oversized log lines.
	MaxDispatchBufRetainCap = 4096
)

// dispatchBufPool pools []byte slices used to carry log events through the
// async ring buffer. Producers call GetDispatchBuf to swap the LogEntry's
// buffer without a per-call make(); consumers call ReturnDispatchBuf after
// all pre-processors have read the data (refs #133 / V4-P2).
var dispatchBufPool = sync.Pool{
	New: func() any {
		b := make([]byte, 0, DefaultDispatchBufCap)
		return b
	},
}

// GetDispatchBuf returns a len=0 []byte from the pool, ready to receive a log
// line. The caller must eventually pass the slice to ReturnDispatchBuf.
func GetDispatchBuf() []byte {
	return dispatchBufPool.Get().([]byte)[:0]
}

// ReturnDispatchBuf returns b to the pool after the consumer has finished
// reading it. Buffers whose capacity exceeds MaxDispatchBufRetainCap are
// silently dropped to prevent memory bloat from large one-off log lines.
func ReturnDispatchBuf(b []byte) {
	if cap(b) >= DefaultDispatchBufCap && cap(b) <= MaxDispatchBufRetainCap {
		dispatchBufPool.Put(b[:0])
	}
}
