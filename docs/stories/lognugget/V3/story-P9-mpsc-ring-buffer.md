# P9 — Lock-Free MPSC Ring Buffer Dispatch Queue

Owner-role: engineer.
Issue: [#120](https://github.com/architagr/LogNugget/issues/120).
Branch: `feat/120-p9-mpsc-ring-buffer` (cut from `feat/111-v3-performance`, PRs back to `feat/111-v3-performance`).
Depends: P1–P8 merged to feat/111-v3-performance. P9 is the final story; implement after all others are merged and the bench gate is green without P9.
LOC est: 280.

## Summary

Go channel send under 8-goroutine contention costs ~130 ns due to the channel's internal mutex. Replace `chan LogEvent` with a lock-free MPSC (multiple-producer, single-consumer) ring buffer. Multiple goroutines push events lock-free via fetch-and-add on the tail; the single consumer goroutine (`ProcessLogEvent`) pops via a spinning read.

**Saving:** ~130 ns/call on the parallel hot path; this story brings the total V3 saving to ~555 ns (P1–P9 combined), achieving the ≤ 500 ns/op target.

## Architecture — MPSC Ring Buffer

```
Producers (N goroutines)          Consumer (1 goroutine)
     │                                    │
     │  tail.Add(1)                       │  head.Load()
     ▼                                    ▼
  ┌──────────────────────────────────────────────────────┐
  │  ringSlot[0]  ringSlot[1]  ...  ringSlot[4095]       │
  │  seq:0        seq:1            seq:4095               │
  └──────────────────────────────────────────────────────┘
       ↑                                    ↑
   tail (producers write here)          head (consumer reads here)
```

Each slot has a `seq atomic.Uint64` that acts as a handoff flag:
- Initial: `slot.seq = slotIndex` (set at ring init)
- Producer writes slot N: spins until `slot.seq.Load() == N`, writes data, stores `seq = N+1`
- Consumer reads slot N: checks `slot.seq.Load() == N+1`, reads data, stores `seq = N+ringSize`

This is the Dmitry Vyukov MPSC queue pattern adapted for fixed-size bounded rings.

## Files touched

- NEW `config/ring_buffer.go` — `mpscRingBuffer` struct and `Push`/`Pop` methods
- NEW `config/ring_buffer_test.go` — correctness + race tests
- NEW `config/ring_buffer_bench_test.go` — `BenchmarkRingBuffer_Push`
- `config/config.go`:
  - Replace `ch chan LogEvent` with `ring *mpscRingBuffer`
  - Update `resetConfig` — init ring, start consumer
  - Rewrite `PublishLog` — calls `ring.Push`
  - Rewrite `ProcessLogEvent` — loops on `ring.Pop()` with `runtime.Gosched()` on empty
  - Update `Stop()` drain logic (see below)

## Ring buffer implementation

```go
// config/ring_buffer.go
package config

import (
    "runtime"
    "sync/atomic"
)

const (
    ringSize = 4096
    ringMask = uint64(ringSize - 1)
)

type ringSlot struct {
    seq  atomic.Uint64
    data LogEvent
    _    [40]byte // pad seq(8)+data(16)+pad(40)=64 bytes = 1 cache line
}

type mpscRingBuffer struct {
    _     [64]byte      // isolate from preceding vars
    tail  atomic.Uint64 // producers: fetch-and-add
    _     [56]byte      // pad tail to its own cache line
    head  atomic.Uint64 // consumer: load/store
    _     [56]byte      // pad head to its own cache line
    slots [ringSize]ringSlot
}

func newMpscRingBuffer() *mpscRingBuffer {
    r := &mpscRingBuffer{}
    for i := uint64(0); i < ringSize; i++ {
        r.slots[i].seq.Store(i)
    }
    return r
}

// Push enqueues e. Spins (with Gosched) if all slots are full.
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

// Pop dequeues the next event. Returns false if the ring is empty.
// Must be called from a single goroutine only.
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

// Len returns the approximate number of items in the ring.
// Not precise under concurrent producers.
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
```

## Updated ProcessLogEvent

```go
func ProcessLogEvent() {
    configMu.RLock()
    currentRing := ring
    configMu.RUnlock()

    for {
        e, ok := currentRing.Pop()
        if !ok {
            // check done signal
            select {
            case <-doneCh:
                // drain remaining
                for currentRing.Len() > 0 {
                    if ev, ok := currentRing.Pop(); ok {
                        dispatchEvent(ev)
                    }
                }
                return
            default:
                runtime.Gosched()
                continue
            }
        }
        dispatchEvent(e)
    }
}

func dispatchEvent(e LogEvent) {
    configMu.RLock()
    processors := EventPreProcessors
    configMu.RUnlock()
    for _, p := range processors {
        p.PreProcess(e.Level, e.Data)
    }
}
```

Note: `doneCh` is the existing stop channel; the drain pattern is the same as the current channel-based implementation.

## Tests-first gate

1. `Test_RingBuffer_PushPop_Sequential` — push N items, pop N items; values match in FIFO order.
2. `Test_RingBuffer_Empty` — `Pop()` on empty ring returns `(LogEvent{}, false)`.
3. `Test_RingBuffer_Full_Spins` — fill ring to capacity; verify next `Push` eventually succeeds after `Pop` frees a slot.
4. `Test_RingBuffer_MPSC_Race` — 8 goroutines pushing 1000 events each, 1 goroutine popping; no data race under `-race`; all 8000 events received.
5. `Test_RingBuffer_DrainOnStop` — push 100 events, trigger stop; verify all 100 events are dispatched before `ProcessLogEvent` returns.
6. `BenchmarkRingBuffer_Push` — ≤ 100 ns/op under 8-goroutine parallel (GOMAXPROCS=8).
7. `Benchmark_Log_Parallel_NoCtx` (full stack with ring): ≤ 500 ns/op.

## Acceptance criteria

1. `chan LogEvent` fully replaced by `*mpscRingBuffer` in `config/config.go`.
2. `SetChannelCapacity` remains callable; godoc notes it does not affect ring buffer size post-P9.
3. `Test_RingBuffer_MPSC_Race` passes under `go test -race`.
4. `Test_RingBuffer_DrainOnStop` passes — stop/drain semantics preserved.
5. `Benchmark_Log_Parallel_NoCtx` (GOMAXPROCS=8): ≤ 500 ns/op.
6. All integration tests (stop drain, fan-out) pass.
7. `go test ./...` green, `go test -race ./...` green.

## Assignment

Assigned to: agent-engineer
Branch: feat/120-p9-mpsc-ring-buffer
Target PR: feat/111-v3-performance
Date: 2026-05-22
PL instruction: This is the most complex story. Write ring buffer tests FIRST (especially the MPSC race test) before touching config.go. Present ring_buffer_test.go for review before any config.go changes. The drain-on-stop test is critical — the existing integration test `Test_Integration_StopIdempotent` must pass unchanged.
