package config

import "sync"

const (
	// DefaultDispatchBufCap is the initial capacity of pooled dispatch buffers.
	// 1 KB covers ~80% of real-world structured log lines without reallocation
	// on the first append inside logWithSkip.
	DefaultDispatchBufCap = 1024

	// MaxDispatchBufRetainCap is the upper cap threshold for recycling dispatch
	// buffers. Buffers larger than this are dropped rather than returned to the
	// pool to prevent unbounded memory retention from rare oversized log lines.
	MaxDispatchBufRetainCap = 4096
)

// dispatchBufPool pools *[]byte to avoid the interface-boxing allocation that
// occurs when a []byte value (3-word struct) is stored via sync.Pool.Put.
// Producers call GetDispatchBuf; consumers call ReturnDispatchBuf after all
// pre-processors have read the data (refs #133 / V4-P2).
var dispatchBufPool = sync.Pool{
	New: func() any {
		b := make([]byte, 0, DefaultDispatchBufCap)
		return &b
	},
}

// GetDispatchBuf returns a len=0 []byte from the pool, ready to receive a log
// line. The caller must eventually pass the slice to ReturnDispatchBuf.
func GetDispatchBuf() []byte {
	return *dispatchBufPool.Get().(*[]byte)
}

// ReturnDispatchBuf returns b to the pool after the consumer has finished
// reading it. Buffers whose capacity exceeds MaxDispatchBufRetainCap are
// silently dropped to prevent memory bloat from large one-off log lines.
func ReturnDispatchBuf(b []byte) {
	if cap(b) >= DefaultDispatchBufCap && cap(b) <= MaxDispatchBufRetainCap {
		b = b[:0]
		dispatchBufPool.Put(&b)
	}
}
