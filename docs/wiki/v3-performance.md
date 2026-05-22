# Epic V3 — Sub-500 ns Hot Path + First-Class OTel Distributed Tracing

Status: IN PROGRESS (2026-05-22)
Owner: Project Lead
Umbrella issue: [#111](https://github.com/architagr/LogNugget/issues/111)
Sources of truth: `docs/STATUS.md`, `docs/prds/v3-performance.md`, `config/config.go`, `entry/entry.go`, `config/parse_field.go`

---

## 1. Problem Statement

LogNugget V2 achieves ~1,090 ns/op on the parallel no-context hot path and ~1,280 ns/op with 10 context fields, with 5–7 allocs/op. This is a major improvement over V1 (~3,440 ns/op, 55 allocs), but still 10× slower than zerolog on raw ns/op.

The V3 goal is ≤ 500 ns/op / ≤ 1 alloc on the parallel hot path — cutting the remaining gap in half — while adding first-class OpenTelemetry distributed tracing support.

### V2 vs V3 targets

| Metric | zerolog | V2 baseline | V3 target |
|--------|---------|-------------|-----------|
| ns/op (parallel NoCtx) | ~110 | ~1,090 | ≤ 500 |
| ns/op (parallel 10ctx appender) | ~110 | ~1,280 | ≤ 500 |
| allocs/op (parallel NoCtx) | 0 | 5 | ≤ 1 |
| B/op (parallel hot path) | 0 | ~1,411 | ≤ 300 |

### Remaining bottlenecks (post-V2 analysis)

| # | Bottleneck | Location | Est. saving |
|---|-----------|----------|-------------|
| P1 | `HasEventPreProcessors()` acquires `configMu.RLock` every call | `config/config.go:273` | ~80 ns |
| P2 | `GetHotSnapshot()` acquires `configMu.RLock` + copies 13-field struct | `config/config.go:153` | ~120 ns |
| P3 | `PublishLog()` acquires `configMu.RLock` for channel pointer | `config/config.go:358` | ~50 ns |
| P4 | `customTime.Format()` returns string; `AppendQuotedString` converts back to `[]byte` | `entry/entry.go:179` | ~40 ns, 2 allocs |
| P5 | `AppendQuotedString` calls `[]byte(s)` on every string field | `config/parse_field.go:26` | ~20 ns, 2 allocs |
| P6 | `level.String()` + `AppendQuotedString` — 5 known-constant values | `entry/entry.go:182` | ~15 ns, 1 alloc |
| P7 | No built-in OTel; benchmark uses legacy map parser | `test/benchmark/` | ~100 ns, 4 allocs |
| P8 | Alias severance allocates 1024 B regardless of actual line size (~200 B) | `entry/entry.go:240` | ~800 B/call |
| P9 | Go channel contention under 8-goroutine load ~130 ns | `config/config.go:362` | ~130 ns |

Total theoretical saving: ~555 ns + 9 allocs + 1100 B/call. Achieves ≤ 500 ns/op target.

---

## 2. Goals

- G1. Achieve `Benchmark_Log_Parallel_NoCtx` ≤ 500 ns/op (GOMAXPROCS=8).
- G2. Achieve `Benchmark_Log_Parallel_10CtxFields` (OTel appender) ≤ 500 ns/op, ≤ 1 alloc/op.
- G3. Eliminate every `configMu.RLock` from the hot path (P1, P2, P3).
- G4. Eliminate every string→[]byte and []byte→string conversion on the hot path (P4, P5, P6).
- G5. Provide a first-class OTel tracing integration example and benchmark (P7).
- G6. Right-size the per-call pool buffer allocation (P8).
- G7. Replace channel dispatch with a lock-free MPSC ring buffer (P9).
- G8. Preserve full backward compatibility: all V2 public APIs unchanged.

## 3. Non-Goals

- N1. Closing the full gap to zerolog (zerolog's 0 alloc advantage comes from sync write; LogNugget's async model is a structural trade-off).
- N2. Bundling OTel SDK in the core module.
- N3. New encoder formats.
- N4. New log levels.
- N5. Raising the bench-check gate threshold.

---

## 4. Actors

| Actor | Role |
|---|---|
| HTTP handler goroutines | Producers: call `entry.NewLogEntry().Info(ctx, msg)` |
| Background ProcessLogEvent goroutine | Consumer: drains ring buffer (post-P9) |
| Pre-processors (hooks) | Route events to registered hooks |
| OTel SDK (user-provided) | Populates span context; extracted by `ContextFieldsAppender` |

---

## 5. Architecture — V3 Hot Path

### V2 hot path (3 RLocks remaining)

```
NewLogEntry()                           // pool get — 0 allocs (warm)
  → atomic.Int64.Load(minLevel)         // P1 gate — 0 locks ✅ (V2)
  → configMu.RLock (HasPreProcessors)   // ⚠️ RLock #1 — P1 target
  → configMu.RLock (GetHotSnapshot)     // ⚠️ RLock #2 — P2 target
  → []byte(timeStr) in AppendQuotedString  // ⚠️ alloc — P4/P5 target
  → level.String() + []byte(levelStr)   // ⚠️ alloc — P6 target
  → context fields via map parser       // ⚠️ +4 allocs — P7 target
  → configMu.RLock (PublishLog ch)      // ⚠️ RLock #3 — P3 target
  → ch <- LogEvent{...}                 // ⚠️ channel contention — P9 target
  → make([]byte, 0, 1024)               // ⚠️ 1024B alloc — P8 target
```

### V3 hot path (0 RLocks, 0 allocs target)

```
NewLogEntry()                           // pool get — 0 allocs (warm)
  → atomic.Int64.Load(minLevel)         // 0 locks ✅
  → atomic.Bool.Load(hasPreProcessors)  // ✅ P1 — 0 locks
  → hotSnapshotPtr.Load()               // ✅ P2 — 0 locks, returns *HotSnapshot
  → append(buf, '"'); t.AppendFormat(); append('"')  // ✅ P4 — 0 allocs
  → appendJSONStringStr(buf, level)     // ✅ P5+P6 — 0 allocs
  → contextAppender(ctx, buf)           // ✅ P7 — 0 allocs (appender path)
  → atomicCh.Load().(chan LogEvent) <-  // ✅ P3 — no RLock before send
    ... or ring.Push(event)             // ✅ P9 — lock-free MPSC
  → make([]byte, 0, len(data))          // ✅ P8 — right-sized
```

---

## 6. Design Details by Story

### P1 — atomic.Bool pre-processor gate

**Current:** `HasEventPreProcessors()` at `config/config.go:273` acquires `configMu.RLock()`, reads `len(EventPreProcessors)`, releases the lock. Called once per log event on the hot path.

**Change:**
```go
// New package-level atomic
var hasPreProcessorsAtomic atomic.Bool

// HasEventPreProcessors — lock-free read
func HasEventPreProcessors() bool { return hasPreProcessorsAtomic.Load() }

// Updated mutators (under configMu.Lock):
// InitPreProcessors: hasPreProcessorsAtomic.Store(len(observers) > 0)
// AddPreProcessors:  hasPreProcessorsAtomic.Store(true)
// RemovePreProcessor: hasPreProcessorsAtomic.Store(len(EventPreProcessors) > 0)
```

**Files:** `config/config.go` (add var, update 3 mutators, rewrite `HasEventPreProcessors`).
New test file: `config/atomic_preprocessor_bench_test.go`.

### P2 — atomic.Pointer[HotSnapshot] copy-on-write

**Current:** `GetHotSnapshot()` at `config/config.go:153` acquires `configMu.RLock()`, copies a 13-field struct, releases lock, then calls `Encoder.OpenBytes()` / `CloseBytes()`.

**Change:**
```go
// New package-level atomic
var hotSnapshotPtr atomic.Pointer[HotSnapshot]

// GetHotSnapshot — lock-free read
func GetHotSnapshot() HotSnapshot { return *hotSnapshotPtr.Load() }

// New helper called by all Set* mutators and resetConfig (under Lock):
func storeHotSnapshot() {
    snap := HotSnapshot{
        AddSource:       defaultConfig.addSource,
        TimeFormat:      defaultConfig.timeFormat,
        // ... all 13 fields
    }
    snap.EncoderOpen  = snap.Encoder.OpenBytes()
    snap.EncoderClose = snap.Encoder.CloseBytes()
    hotSnapshotPtr.Store(&snap)
}
```

**Files:** `config/config.go` — add var, add `storeHotSnapshot()`, update `resetConfig` + all `Set*` mutators (10 functions).
New test file: `config/atomic_snapshot_v3_bench_test.go`.

### P3 — atomic channel pointer

**Current:** `PublishLog()` acquires `configMu.RLock()` to snapshot `ch`, releases lock, then sends.

**Change:**
```go
var atomicCh atomic.Value  // stores chan LogEvent

func PublishLog(Level enum.LogLevel, Data []byte) {
    atomicCh.Load().(chan LogEvent) <- LogEvent{Level: Level, Data: Data}
}

// In resetConfig (under Lock):
ch = newCh
atomicCh.Store(newCh)
```

**Files:** `config/config.go` — add var, update `resetConfig`, rewrite `PublishLog`.

### P4 — direct timestamp append

**Current:** `entry/entry.go:179`:
```go
e.buf = config.AppendQuotedString(e.buf, customTime.Format(customTime.TimeNow(), snap.TimeFormat))
```
`customTime.Format` returns a string (1 alloc); `AppendQuotedString` calls `[]byte(s)` (1 alloc).

**Change:**
```go
now := customTime.TimeNow()
e.buf = append(e.buf, '"')
e.buf = now.AppendFormat(e.buf, snap.TimeFormat)
e.buf = append(e.buf, '"')
```
Zero allocations. RFC3339 output contains no JSON-unsafe characters.

**Files:** `entry/entry.go` (3-line change at logWithSkip timestamp section).

### P5 — string-native JSON escape

**Current:** `config/parse_field.go:26`:
```go
func AppendQuotedString(dst []byte, s string) []byte {
    return appendJSONString(dst, []byte(s))  // []byte(s) = alloc
}
```

**Change:** Add `appendJSONStringStr(dst []byte, s string) []byte` that iterates over `s` as a string (no conversion). Update `AppendQuotedString`, `AppendAttr` (KindStr), and `AppendField` (string case) to use it.

**Files:** `config/parse_field.go`.

### P6 — pre-rendered level bytes

**Current:** `entry/entry.go:182`:
```go
e.buf = config.AppendQuotedString(e.buf, level.String())
```
`level.String()` calls `fmt.Sprintf` (1 alloc for non-named levels; 0 for named but still produces a string).

**Change:** Add to `config/parse_field.go`:
```go
func AppendQuotedLevel(dst []byte, l enum.LogLevel) []byte {
    switch l {
    case enum.LevelDebug: return append(dst, `"DEBUG"`...)
    case enum.LevelInfo:  return append(dst, `"INFO"`...)
    case enum.LevelWarn:  return append(dst, `"WARN"`...)
    case enum.LevelError: return append(dst, `"ERROR"`...)
    case enum.LevelFatal: return append(dst, `"FATAL"`...)
    default: return AppendQuotedString(dst, l.String())
    }
}
```
Then in `entry.go`: `e.buf = config.AppendQuotedLevel(e.buf, level)`.

**Files:** `config/parse_field.go` (new function), `entry/entry.go` (1-line change).

### P7 — first-class OTel ContextFieldsAppender

**Current:** `Benchmark_Log_Parallel_10CtxFields` uses `config.SetContextFieldsParser` (map-based, ~100 ns, 4 allocs). No OTel example exists.

**Change:**
1. Update both `Benchmark_Log_Parallel_10CtxFields` benches to use `SetContextFieldsAppender`.
2. Add `Benchmark_Log_Parallel_OtelCtx` showing OTel span extraction.
3. Create `examples/otel-appender/main.go` demonstrating the pattern.

**Example OTel appender pattern:**
```go
config.SetContextFieldsAppender(func(ctx context.Context, dst []byte) []byte {
    span := trace.SpanFromContext(ctx)
    if !span.IsRecording() {
        return dst
    }
    sc := span.SpanContext()
    dst = append(dst, `,"trace_id":"`...)
    dst = append(dst, sc.TraceID().String()...)
    dst = append(dst, `","span_id":"`...)
    dst = append(dst, sc.SpanID().String()...)
    dst = append(dst, '"')
    return dst
})
```

**Files:** `entry/parallel_bench_test.go`, `test/benchmark/log_parallel_bench_test.go`, new `test/benchmark/otel_bench_test.go`, new `examples/otel-appender/`.

### P8 — exact-size buffer alias severance

**Current:** `entry/entry.go:240`:
```go
e.buf = make([]byte, 0, initBufCap)  // always 1024 B
```

**Change:**
```go
e.buf = make([]byte, 0, len(data))   // right-sized to actual log line
```

Over time the pool slots naturally grow to hold actual line sizes; first reuse of a larger line causes one realloc, then stabilises.

**Files:** `entry/entry.go` (1-line change).

### P9 — lock-free MPSC ring buffer

**Current:** `chan LogEvent` with capacity 1000. Channel send costs ~130 ns under 8-goroutine contention (internal mutex).

**Change:** Replace with a lock-free MPSC ring buffer in `config/ring_buffer.go`:

```go
const (
    ringSize = 4096       // power of 2; must be > realistic burst
    ringMask = ringSize - 1
)

type ringSlot struct {
    seq  atomic.Uint64
    data LogEvent
    _    [40]byte  // pad to 64-byte cache line
}

type mpscRingBuffer struct {
    _     [64]byte       // padding
    tail  atomic.Uint64  // producers fetch-and-add
    _     [56]byte
    head  atomic.Uint64  // consumer reads
    _     [56]byte
    slots [ringSize]ringSlot
}
```

**Producer (Push — called by many goroutines):**
```go
func (r *mpscRingBuffer) Push(e LogEvent) {
    pos := r.tail.Add(1) - 1
    slot := &r.slots[pos&ringMask]
    for slot.seq.Load() != pos { runtime.Gosched() }
    slot.data = e
    slot.seq.Store(pos + 1)
}
```

**Consumer (Pop — single goroutine only):**
```go
func (r *mpscRingBuffer) Pop() (LogEvent, bool) {
    pos := r.head.Load()
    slot := &r.slots[pos&ringMask]
    if slot.seq.Load() != pos+1 { return LogEvent{}, false }
    e := slot.data
    slot.seq.Store(pos + ringSize)
    r.head.Store(pos + 1)
    return e, true
}
```

`ProcessLogEvent` updated to loop on `Pop()` with `runtime.Gosched()` on empty.
`Stop()`/drain semantics preserved: consumer flushes until ring is empty before signalling done.

**Files:** new `config/ring_buffer.go`, update `config/config.go` (`resetConfig`, `PublishLog`, `ProcessLogEvent`), new `config/ring_buffer_test.go`.

---

## 7. Open Questions (for implementers)

1. **P9 drain-on-stop**: The existing `doneCh`/`flushWg` pattern must be adapted to the ring buffer. The consumer goroutine must signal completion after draining the ring to empty. How does it detect "last event written"? Recommend: add a sentinel `LogEvent{Level: -1}` that tells the consumer to exit.

2. **P8 minimum buf size**: Should `len(data)` be floored at some minimum (e.g. 64 B)? Very short test log lines could cause repeated small-alloc pool slots. Recommend: `max(64, len(data))`.

3. **P2 + P9 interaction**: After P9, `HotSnapshot` no longer needs `ch` in it (P3 made it an `atomicCh`). The snapshot struct may be slimmed. Confirm no field removals break tests.

---

## 8. Testing Strategy

| Layer | Scope | Tool |
|---|---|---|
| Unit | Lock-free correctness for P1/P2/P3/P9 | `go test -race -count=1000` |
| Bench | Per-story hot-path microbench | `go test -bench=. -benchmem -count=10` |
| Integration | Full pipeline (publish → drain → hook) | `test/integration/` |
| Gate | All packages, no regression | `./scripts/bench-check.sh` |
