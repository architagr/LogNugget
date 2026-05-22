package config

import (
	"runtime"
	"sync/atomic"
)

const (
	ringSize = 4096
	ringMask = uint64(ringSize - 1)
)

// ringSlot is one entry in the MPSC ring buffer. Each slot is padded to a
// single cache line (64 bytes) to eliminate false sharing between adjacent
// slots accessed by different producer goroutines.
//
// Layout on amd64/arm64:
//
//	seq  atomic.Uint64 = 8 bytes
//	data LogEvent       = 32 bytes (Level int64=8 + Data []byte=24)
//	_    [24]byte       = padding
//	total               = 64 bytes (1 cache line)
type ringSlot struct {
	seq  atomic.Uint64
	data LogEvent
	_    [24]byte // cache-line padding
}

// mpscRingBuffer is a bounded, lock-free multiple-producer single-consumer
// queue following the Dmitry Vyukov MPSC ring buffer pattern. Producers call
// Push concurrently; the consumer calls Pop from a single goroutine only.
//
// Each field group is padded to its own cache line to prevent the producers'
// tail pointer from interfering with the consumer's head pointer.
type mpscRingBuffer struct {
	_     [64]byte      // isolate from adjacent heap objects
	tail  atomic.Uint64 // fetch-and-add by producers
	_     [56]byte      // pad tail to its own 64-byte cache line
	head  atomic.Uint64 // load/store by the consumer only
	_     [56]byte      // pad head to its own 64-byte cache line
	slots [ringSize]ringSlot
}

// newMpscRingBuffer allocates and initialises an mpscRingBuffer. Each slot's
// seq is set to its index, marking all slots as available for the first round
// of producers.
func newMpscRingBuffer() *mpscRingBuffer {
	r := &mpscRingBuffer{}
	for i := uint64(0); i < ringSize; i++ {
		r.slots[i].seq.Store(i)
	}
	return r
}

// Push enqueues e. It claims a slot via atomic fetch-and-add on tail, then
// spins (yielding with runtime.Gosched) until the slot's seq matches the
// claimed position — this spin is bounded by the consumer's Pop rate and is
// extremely rare in practice (only when the ring is full).
//
// Safe for concurrent use by multiple producers.
func (r *mpscRingBuffer) Push(e LogEvent) {
	pos := r.tail.Add(1) - 1
	slot := &r.slots[pos&ringMask]
	for slot.seq.Load() != pos {
		runtime.Gosched()
	}
	slot.data = e
	slot.seq.Store(pos + 1)
}

// Pop dequeues the next event. Returns (event, true) if an event is available,
// (LogEvent{}, false) if the ring is empty.
//
// Must be called from a single goroutine only (single-consumer invariant).
func (r *mpscRingBuffer) Pop() (LogEvent, bool) {
	pos := r.head.Load()
	slot := &r.slots[pos&ringMask]
	if slot.seq.Load() != pos+1 {
		return LogEvent{}, false
	}
	e := slot.data
	slot.seq.Store(pos + ringSize)
	r.head.Store(pos + 1)
	return e, true
}

// Len returns the approximate number of unread items in the ring.
// The value is not precise under concurrent producers but is safe to read
// from any goroutine (used for drain checks on the stop path).
func (r *mpscRingBuffer) Len() int {
	tail := r.tail.Load()
	head := r.head.Load()
	if tail <= head {
		return 0
	}
	n := tail - head
	if n > ringSize {
		return ringSize
	}
	return int(n)
}

// atomicLoadUint64 is a helper for reading atomic.Uint64 fields via the
// standard sync/atomic package — used only in the padding-size check below.
var _ = atomic.LoadUint64 // ensure sync/atomic is imported
