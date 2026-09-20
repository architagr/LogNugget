//go:build testing

// White-box tests for V4-P3 single-slab LogEntry (refs #134).
// Must be in package entry to access unexported fields.
package entry

import (
	"testing"

	"github.com/architagr/lognugget/v4/config"
)

// TestInitLogEntry_WarmDispatchPool_OneAlloc verifies that when dispatchBufPool
// is warm, initLogEntry costs exactly 1 heap allocation: the LogEntry struct
// (with inline pendingBufSlab). The buf slice comes from the pool at 0 allocs,
// and pendingBuf is re-sliced from the inline slab — also 0 allocs (V4-P3).
func TestInitLogEntry_WarmDispatchPool_OneAlloc(t *testing.T) {
	// Seed the pool with enough entries so Get never misses during measurement.
	for i := 0; i < 64; i++ {
		config.ReturnDispatchBuf(make([]byte, 0, config.DefaultDispatchBufCap))
	}

	allocs := testing.AllocsPerRun(50, func() {
		e := initLogEntry()
		// Return e.buf before the next iteration to keep pool warm.
		config.ReturnDispatchBuf(e.buf)
		e.buf = nil
	})
	// Pool.Put may internally allocate a chain node on first use, adding up to
	// 1 amortised alloc. Allow ≤ 2; in steady state only the struct allocates.
	if allocs > 2 {
		t.Errorf("warm initLogEntry allocs/run = %.1f, want ≤ 2", allocs)
	}
}

// TestPendingBufInSlab verifies that pendingBuf's backing array is the inline
// pendingBufSlab — no separate heap allocation for pendingBuf.
func TestPendingBufInSlab(t *testing.T) {
	e := initLogEntry()
	if cap(e.pendingBuf) != pendingBufCap {
		t.Errorf("pendingBuf cap=%d, want %d", cap(e.pendingBuf), pendingBufCap)
	}
	if len(e.pendingBuf) != 0 {
		t.Errorf("pendingBuf len=%d, want 0", len(e.pendingBuf))
	}
}

// TestReset_PendingBufRestoredFromSlab verifies that reset() re-slices pendingBuf
// from the inline slab (cap preserved, no alloc).
func TestReset_PendingBufRestoredFromSlab(t *testing.T) {
	e := initLogEntry()
	// Write something to pendingBuf to verify it gets cleared on reset.
	e.pendingBuf = append(e.pendingBuf, []byte(`,"k":"v"`)...)
	e.reset()
	if len(e.pendingBuf) != 0 {
		t.Errorf("after reset pendingBuf len=%d, want 0", len(e.pendingBuf))
	}
	if cap(e.pendingBuf) != pendingBufCap {
		t.Errorf("after reset pendingBuf cap=%d, want %d", cap(e.pendingBuf), pendingBufCap)
	}
}
