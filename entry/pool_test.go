//go:build testing

// Package entry provides white-box pool tests for TS-08. These tests cover
// the sync.Pool lifecycle: pre-warming via GenerateInitialPool, zero-value
// guarantee of NewLogEntry, round-trip Put/Get behaviour, and the
// zero-argument no-op path of GenerateInitialPool(0).
//
// All tests are in package entry (white-box) so they can inspect unexported
// fields (buf, caller) and the unexported constant initBufCap.
package entry

import (
	"testing"
)

// Test_NewLogEntry_ReturnsResetEntry verifies that a freshly obtained *LogEntry
// has buf length zero and a nil caller field. This guards the post-reset
// zero-value contract (F3 / F25).
func Test_NewLogEntry_ReturnsResetEntry(t *testing.T) {
	t.Parallel()

	e := NewLogEntry()
	defer e.Put()

	if len(e.buf) != 0 {
		t.Errorf("NewLogEntry: buf len = %d, want 0 (entry must be reset before returning)", len(e.buf))
	}
	if e.caller != nil {
		t.Errorf("NewLogEntry: caller = %v, want nil (entry must be reset before returning)", e.caller)
	}
}

// Test_GenerateInitialPool_PrePopulatesN verifies that after GenerateInitialPool(n),
// n consecutive NewLogEntry calls each return an entry whose buf backing array
// was pre-grown to at least initBufCap capacity. This confirms the pool was
// pre-warmed with fully initialised entries (F4 / TS-08).
//
// why: sync.Pool does not guarantee LIFO or any ordering, but a freshly warmed
// pool on a single goroutine will return the pre-allocated entries before the
// GC has a chance to collect them. Asserting cap >= initBufCap (not pointer
// equality) keeps the assertion GC-safe.
func Test_GenerateInitialPool_PrePopulatesN(t *testing.T) {
	t.Parallel()

	const n = 5
	GenerateInitialPool(n)

	for i := 0; i < n; i++ {
		e := NewLogEntry()
		if cap(e.buf) < initBufCap {
			t.Errorf("NewLogEntry call %d after GenerateInitialPool(%d): buf cap = %d, want >= %d (pool not pre-warmed)", i+1, n, cap(e.buf), initBufCap)
		}
		e.Put()
	}
}

// Test_LogEntry_Put_ReturnsToPool verifies that Put followed by NewLogEntry
// returns an entry with buf capacity >= initBufCap and len == 0.
// This is a best-effort assertion: sync.Pool offers no hard guarantee that
// the exact same object is returned, but it will be on any uncontended path
// where the GC has not run between Put and Get (F25 / TS-08).
func Test_LogEntry_Put_ReturnsToPool(t *testing.T) {
	t.Parallel()

	e := NewLogEntry()
	// Grow buf so the capacity is observable after the pool round-trip.
	e.buf = append(e.buf, make([]byte, 64)...)
	e.Put()

	e2 := NewLogEntry()
	defer e2.Put()

	if cap(e2.buf) < initBufCap {
		t.Errorf("NewLogEntry after Put: buf cap = %d, want >= %d (backing array must survive pool cycle)", cap(e2.buf), initBufCap)
	}
	if len(e2.buf) != 0 {
		t.Errorf("NewLogEntry after Put: buf len = %d, want 0 (reset must clear buf)", len(e2.buf))
	}
}

// Test_GenerateInitialPool_ZeroSafeWithN_0 verifies that GenerateInitialPool(0)
// completes without panicking or blocking (F4 / TS-08 zero-argument no-op path).
func Test_GenerateInitialPool_ZeroSafeWithN_0(t *testing.T) {
	t.Parallel()

	// Must not panic.
	GenerateInitialPool(0)
}

// Benchmark_Pool_GetPut measures the round-trip cost of NewLogEntry + Put on
// the pre-warmed pool under zero contention. Input shape: single goroutine,
// no fields, no logging — pure pool mechanics.
//
// Budget: informational only; < 30 ns/op is the target (not a build gate).
func Benchmark_Pool_GetPut(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		e := NewLogEntry()
		e.Put()
	}
}
