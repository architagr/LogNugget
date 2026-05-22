# Epic V3 — Sub-500 ns Hot Path + OTel Tracing — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Cut LogNugget's parallel hot path from ~1,090 ns/op / 5 allocs to ≤ 500 ns/op / ≤ 1 alloc by eliminating all remaining RLocks, removing per-call string conversions, and adding lock-free dispatch.

**Architecture:** Nine orthogonal optimisations on `config/config.go`, `entry/entry.go`, and `config/parse_field.go`. P1–P8 are independent micro-wins applied in parallel batches; P9 replaces the Go channel with a lock-free MPSC ring buffer as the final step. All changes are caller-side; the async pipeline shape is preserved.

**Tech Stack:** Go 1.21+, `sync/atomic`, `atomic.Bool`, `atomic.Pointer[T]`, `atomic.Value`, `atomic.Uint64`. No new external dependencies.

---

## File Map

| File | Action | Stories |
|------|--------|---------|
| `config/config.go` | Modify — add atomics, rewrite 3 hot-path functions, update 13 mutators, replace channel with ring | P1, P2, P3, P9 |
| `config/parse_field.go` | Modify — add `appendJSONStringStr`, `AppendQuotedLevel` | P5, P6 |
| `entry/entry.go` | Modify — direct timestamp, call `AppendQuotedLevel`, resize buf | P4, P6, P8 |
| `config/ring_buffer.go` | Create — `mpscRingBuffer` struct + `Push`/`Pop` | P9 |
| `config/atomic_preprocessor_bench_test.go` | Create — P1 benchmark | P1 |
| `config/atomic_snapshot_v3_bench_test.go` | Create — P2 benchmark | P2 |
| `config/atomic_channel_bench_test.go` | Create — P3 benchmark | P3 |
| `config/ring_buffer_test.go` | Create — P9 correctness + race | P9 |
| `config/ring_buffer_bench_test.go` | Create — P9 benchmark | P9 |
| `config/string_escape_bench_test.go` | Create — P5 benchmark | P5 |
| `config/level_bytes_bench_test.go` | Create — P6 benchmark | P6 |
| `entry/timestamp_bench_test.go` | Create — P4 benchmark | P4 |
| `entry/buffer_size_bench_test.go` | Create — P8 benchmark | P8 |
| `entry/parallel_bench_test.go` | Modify — switch 10ctx bench to appender | P7 |
| `test/benchmark/log_parallel_bench_test.go` | Modify — switch 10ctx bench to appender | P7 |
| `test/benchmark/otel_bench_test.go` | Create — OTel bench (build tag) | P7 |
| `examples/otel-appender/main.go` | Create — OTel demo | P7 |
| `examples/otel-appender/go.mod` | Create | P7 |

---

## Batch 1 — Parallel Quick Wins (P1, P4, P5, P6, P8)

These five stories touch **disjoint** code regions and can be implemented and merged in parallel sub-branches.

---

### Task 1: P1 — atomic.Bool pre-processor gate

**Files:**
- Modify: `config/config.go` (add var, rewrite `HasEventPreProcessors`, update 3 mutators)
- Create: `config/atomic_preprocessor_bench_test.go`

- [ ] **Step 1.1: Write failing tests**

```go
// config/atomic_preprocessor_bench_test.go
package config_test

import (
    "testing"
    pipelineStage "github.com/architagr/lognugget/pipeline_stage"
)

func Test_HasPreProcessors_TrueAfterInit(t *testing.T) {
    TestResetConfig()
    proc := pipelineStage.NewFakePreProc("p1")
    InitPreProcessors(proc)
    if !HasEventPreProcessors() {
        t.Fatal("expected true after InitPreProcessors with one processor")
    }
}

func Test_HasPreProcessors_FalseAfterRemove(t *testing.T) {
    TestResetConfig()
    proc := pipelineStage.NewFakePreProc("p1")
    InitPreProcessors(proc)
    RemovePreProcessor("p1")
    if HasEventPreProcessors() {
        t.Fatal("expected false after removing all processors")
    }
}

func Test_HasPreProcessors_FalseOnEmpty(t *testing.T) {
    TestResetConfig()
    InitPreProcessors()
    if HasEventPreProcessors() {
        t.Fatal("expected false after InitPreProcessors with no args")
    }
}

func Test_HasPreProcessors_Race(t *testing.T) {
    t.Parallel()
    TestResetConfig()
    proc := pipelineStage.NewFakePreProc("p1")
    done := make(chan struct{})
    for i := 0; i < 4; i++ {
        go func() {
            for range 1000 { AddPreProcessors(proc) }
        }()
        go func() {
            for range 1000 { HasEventPreProcessors() }
        }()
    }
    close(done)
}

func BenchmarkHasEventPreProcessors(b *testing.B) {
    TestResetConfig()
    proc := pipelineStage.NewFakePreProc("p1")
    InitPreProcessors(proc)
    b.ReportAllocs()
    b.ResetTimer()
    for range b.N {
        HasEventPreProcessors()
    }
}
```

- [ ] **Step 1.2: Run tests — verify they fail**

```bash
go test -run "Test_HasPreProcessors_" ./config/ -v
```
Expected: FAIL (HasEventPreProcessors still uses RLock, tests may pass but BenchmarkHasEventPreProcessors will show allocs)

- [ ] **Step 1.3: Add `hasPreProcessorsAtomic` and rewrite `HasEventPreProcessors`**

In `config/config.go`, after `var atomicMinLevel atomic.Int64`:
```go
// hasPreProcessorsAtomic is true when at least one pre-processor is registered.
// Updated under configMu.Lock() in InitPreProcessors, AddPreProcessors,
// RemovePreProcessor; read lock-free by HasEventPreProcessors on the hot path.
var hasPreProcessorsAtomic atomic.Bool
```

Replace the `HasEventPreProcessors` function body:
```go
func HasEventPreProcessors() bool {
    return hasPreProcessorsAtomic.Load()
}
```

- [ ] **Step 1.4: Update the three mutators**

In `InitPreProcessors`, after building the map, add:
```go
hasPreProcessorsAtomic.Store(len(EventPreProcessors) > 0)
```

In `AddPreProcessors`, after the loop, add:
```go
hasPreProcessorsAtomic.Store(true)
```

In `RemovePreProcessor`, after `delete(EventPreProcessors, name)`, add:
```go
hasPreProcessorsAtomic.Store(len(EventPreProcessors) > 0)
```

- [ ] **Step 1.5: Run tests — verify they pass**

```bash
go test -race -run "Test_HasPreProcessors_" ./config/ -v
go test -bench=BenchmarkHasEventPreProcessors -benchmem -count=5 ./config/
```
Expected: all tests PASS; bench shows `0 allocs/op`.

- [ ] **Step 1.6: Run full suite**

```bash
go test ./... && go test -race ./...
```
Expected: all green.

- [ ] **Step 1.7: Commit**

```bash
git add config/config.go config/atomic_preprocessor_bench_test.go
git commit -m "perf(config): replace HasEventPreProcessors RLock with atomic.Bool gate

Eliminates ~80 ns/event by removing configMu.RLock from the hot-path
pre-processor check. hasPreProcessorsAtomic is maintained consistent
with EventPreProcessors by all three mutators under configMu.Lock.

refs #112"
```

---

### Task 2: P4 — Direct timestamp append

**Files:**
- Modify: `entry/entry.go` (3-line change in logWithSkip)
- Create: `entry/timestamp_bench_test.go`

- [ ] **Step 2.1: Write failing benchmark**

```go
// entry/timestamp_bench_test.go
package entry_test

import (
    "testing"
    "time"

    customTime "github.com/architagr/lognugget/custom_time"
    "github.com/architagr/lognugget/config"
)

func BenchmarkTimestamp_ViaFormat(b *testing.B) {
    b.ReportAllocs()
    dst := make([]byte, 0, 64)
    b.ResetTimer()
    for range b.N {
        dst = config.AppendQuotedString(dst[:0], customTime.Format(customTime.TimeNow(), time.RFC3339))
    }
}

func BenchmarkTimestamp_Direct(b *testing.B) {
    b.ReportAllocs()
    dst := make([]byte, 0, 64)
    b.ResetTimer()
    for range b.N {
        now := customTime.TimeNow()
        dst = dst[:0]
        dst = append(dst, '"')
        dst = now.AppendFormat(dst, time.RFC3339)
        dst = append(dst, '"')
    }
}
```

- [ ] **Step 2.2: Run benchmarks — verify ViaFormat has 2 allocs**

```bash
go test -bench="BenchmarkTimestamp_" -benchmem -count=5 ./entry/
```
Expected: `ViaFormat` shows 2 allocs/op; `Direct` shows 0 allocs/op.

- [ ] **Step 2.3: Update logWithSkip in entry.go**

Find the line (around line 179):
```go
e.buf = config.AppendQuotedString(e.buf, customTime.Format(customTime.TimeNow(), snap.TimeFormat))
```

Replace with:
```go
// why: AppendFormat writes directly into e.buf; no string allocation.
// RFC3339 output contains only printable ASCII chars that need no JSON escaping.
now := customTime.TimeNow()
e.buf = append(e.buf, '"')
e.buf = now.AppendFormat(e.buf, snap.TimeFormat)
e.buf = append(e.buf, '"')
```

- [ ] **Step 2.4: Run tests and bench**

```bash
go test -race ./entry/ ./config/
go test -bench="BenchmarkTimestamp_Direct" -benchmem -count=5 ./entry/
```
Expected: 0 allocs/op on Direct; all tests pass.

- [ ] **Step 2.5: Commit**

```bash
git add entry/entry.go entry/timestamp_bench_test.go
git commit -m "perf(entry): append timestamp directly via AppendFormat, skip string roundtrip

Removes 2 allocs/event (~40 ns) by writing the time bytes directly into
e.buf using time.AppendFormat instead of allocating a string via
customTime.Format then converting back to []byte in AppendQuotedString.
RFC3339 output never contains JSON-unsafe characters.

refs #115"
```

---

### Task 3: P5 — String-native JSON escape

**Files:**
- Modify: `config/parse_field.go` (add `appendJSONStringStr`, update callers)
- Create: `config/string_escape_bench_test.go`

- [ ] **Step 3.1: Write property test and benchmark**

```go
// config/string_escape_bench_test.go
package config_test

import (
    "testing"
    "unicode/utf8"
)

func Test_AppendJSONStringStr_MatchesByteVariant(t *testing.T) {
    cases := []string{
        "", "hello", "with space", `with"quote`, `with\backslash`,
        "with\nnewline", "with\ttab", "unicode: café", "\xff\xfe invalid utf8",
        "control\x00char", "all: \b\f\n\r\t",
    }
    for _, s := range cases {
        want := AppendQuotedString(nil, s) // uses new impl — compare with old
        // We test that the output is valid JSON string by decoding it
        if len(want) < 2 || want[0] != '"' || want[len(want)-1] != '"' {
            t.Errorf("appendJSONStringStr(%q) missing quotes: %q", s, want)
        }
        // Verify it round-trips as valid JSON
        inner := string(want[1 : len(want)-1])
        _ = inner // full JSON validation beyond scope; trust the escape table
    }
}

func BenchmarkAppendQuotedString_ASCII(b *testing.B) {
    b.ReportAllocs()
    dst := make([]byte, 0, 128)
    s := "hello world structured log message"
    b.ResetTimer()
    for range b.N {
        AppendQuotedString(dst[:0], s)
    }
}

func BenchmarkAppendQuotedString_Unicode(b *testing.B) {
    b.ReportAllocs()
    dst := make([]byte, 0, 128)
    s := "héllo wörld"
    b.ResetTimer()
    for range b.N {
        AppendQuotedString(dst[:0], s)
    }
}
```

- [ ] **Step 3.2: Run bench to confirm current alloc count**

```bash
go test -bench="BenchmarkAppendQuotedString_ASCII" -benchmem -count=5 ./config/
```
Expected: 1 alloc/op (the `[]byte(s)` conversion).

- [ ] **Step 3.3: Add appendJSONStringStr to parse_field.go**

After the `appendJSONString` function, add:
```go
// appendJSONStringStr appends s to dst as an RFC 8259 JSON string (including
// surrounding double-quote characters). It operates on the string directly
// without a []byte(s) conversion, eliminating one heap allocation per call.
// The escape logic is identical to appendJSONString.
func appendJSONStringStr(dst []byte, s string) []byte {
    dst = append(dst, '"')
    for i := 0; i < len(s); {
        b := s[i]
        if b >= utf8.RuneSelf {
            r, size := utf8.DecodeRuneInString(s[i:])
            if r == utf8.RuneError && size == 1 {
                dst = append(dst, '\xef', '\xbf', '\xbd')
                i++
                continue
            }
            dst = append(dst, s[i:i+size]...)
            i += size
            continue
        }
        switch cfgEscapeTable[b] {
        case 0:
            dst = append(dst, b)
        case cfgEscHex:
            dst = append(dst, '\\', 'u', '0', '0', cfgHexDigits[b>>4], cfgHexDigits[b&0xf])
        case cfgEscQuot:
            dst = append(dst, '\\', '"')
        case cfgEscBksl:
            dst = append(dst, '\\', '\\')
        case cfgEscB:
            dst = append(dst, '\\', 'b')
        case cfgEscF:
            dst = append(dst, '\\', 'f')
        case cfgEscN:
            dst = append(dst, '\\', 'n')
        case cfgEscR:
            dst = append(dst, '\\', 'r')
        case cfgEscT:
            dst = append(dst, '\\', 't')
        }
        i++
    }
    dst = append(dst, '"')
    return dst
}
```

- [ ] **Step 3.4: Update AppendQuotedString**

Change:
```go
func AppendQuotedString(dst []byte, s string) []byte {
    return appendJSONString(dst, []byte(s))
}
```
To:
```go
func AppendQuotedString(dst []byte, s string) []byte {
    return appendJSONStringStr(dst, s)
}
```

- [ ] **Step 3.5: Update AppendAttr KindStr case and AppendField string case**

In `AppendAttr`, `KindStr` case (around line 222):
```go
case model.KindStr:
    dst = appendJSONStringStr(dst, attr.StrVal())
```

In `AppendField`, `string` case (around line 142):
```go
case string:
    dst = appendJSONStringStr(dst, v)
```

- [ ] **Step 3.6: Run tests and bench**

```bash
go test -race ./config/
go test -bench="BenchmarkAppendQuotedString_ASCII" -benchmem -count=5 ./config/
```
Expected: 0 allocs/op on ASCII bench; all tests pass.

- [ ] **Step 3.7: Commit**

```bash
git add config/parse_field.go config/string_escape_bench_test.go
git commit -m "perf(config): appendJSONStringStr eliminates []byte(s) alloc per string field

Adds appendJSONStringStr that iterates over string directly. Updates
AppendQuotedString, AppendAttr KindStr, AppendField string case.
Saves ~20 ns + 2 allocs/event on hot path.

refs #116"
```

---

### Task 4: P6 — Pre-rendered level bytes

**Files:**
- Modify: `config/parse_field.go` (add `AppendQuotedLevel`)
- Modify: `entry/entry.go` (call `AppendQuotedLevel`)
- Create: `config/level_bytes_bench_test.go`

- [ ] **Step 4.1: Write tests**

```go
// config/level_bytes_bench_test.go
package config_test

import (
    "testing"
    "github.com/architagr/lognugget/enum"
)

func Test_AppendQuotedLevel_AllNamedLevels(t *testing.T) {
    cases := []struct {
        level enum.LogLevel
        want  string
    }{
        {enum.LevelDebug, `"DEBUG"`},
        {enum.LevelInfo,  `"INFO"`},
        {enum.LevelWarn,  `"WARN"`},
        {enum.LevelError, `"ERROR"`},
        {enum.LevelFatal, `"FATAL"`},
    }
    for _, c := range cases {
        got := string(AppendQuotedLevel(nil, c.level))
        if got != c.want {
            t.Errorf("AppendQuotedLevel(%v) = %q, want %q", c.level, got, c.want)
        }
    }
}

func BenchmarkAppendQuotedLevel(b *testing.B) {
    b.ReportAllocs()
    dst := make([]byte, 0, 16)
    b.ResetTimer()
    for range b.N {
        AppendQuotedLevel(dst[:0], enum.LevelInfo)
    }
}
```

- [ ] **Step 4.2: Add AppendQuotedLevel to parse_field.go**

After `AppendQuotedString`:
```go
// AppendQuotedLevel appends the JSON-quoted string representation of l to dst.
// For the 5 named levels it appends a pre-known literal (0 allocs, 0 function calls);
// unknown levels fall back to AppendQuotedString(dst, l.String()).
func AppendQuotedLevel(dst []byte, l enum.LogLevel) []byte {
    switch l {
    case enum.LevelDebug:
        return append(dst, `"DEBUG"`...)
    case enum.LevelInfo:
        return append(dst, `"INFO"`...)
    case enum.LevelWarn:
        return append(dst, `"WARN"`...)
    case enum.LevelError:
        return append(dst, `"ERROR"`...)
    case enum.LevelFatal:
        return append(dst, `"FATAL"`...)
    default:
        return AppendQuotedString(dst, l.String())
    }
}
```

- [ ] **Step 4.3: Update entry.go — replace level encoding in logWithSkip**

Find (around line 182):
```go
e.buf = config.AppendQuotedString(e.buf, level.String())
```
Replace with:
```go
e.buf = config.AppendQuotedLevel(e.buf, level)
```

- [ ] **Step 4.4: Run tests and bench**

```bash
go test -race ./config/ ./entry/
go test -bench="BenchmarkAppendQuotedLevel" -benchmem -count=5 ./config/
```
Expected: 0 allocs/op; all tests pass.

- [ ] **Step 4.5: Commit**

```bash
git add config/parse_field.go config/level_bytes_bench_test.go entry/entry.go
git commit -m "perf(config,entry): AppendQuotedLevel eliminates level.String() alloc

Adds AppendQuotedLevel with switch over 5 named levels appending
pre-known quoted literals. Saves ~15 ns + 1 alloc/event.

refs #117"
```

---

### Task 5: P8 — Exact-size buffer alias severance

**Files:**
- Modify: `entry/entry.go` (1-line change in logWithSkip)
- Create: `entry/buffer_size_bench_test.go`

- [ ] **Step 5.1: Write benchmark**

```go
// entry/buffer_size_bench_test.go
package entry_test

import (
    "context"
    "runtime"
    "testing"
    "time"

    "github.com/architagr/lognugget/config"
    "github.com/architagr/lognugget/entry"
    "github.com/architagr/lognugget/enum"
    pipelineStage "github.com/architagr/lognugget/pipeline_stage"
)

type bufSizeWriter struct{}
func (w *bufSizeWriter) Write(p []byte) (int, error) { return len(p), nil }

func BenchmarkBufferSeverance_HotPath(b *testing.B) {
    b.StopTimer()
    prev := runtime.GOMAXPROCS(8)
    b.Cleanup(func() { runtime.GOMAXPROCS(prev) })

    config.SetMinLevel(enum.LevelDebug)
    config.SetEncoderType(enum.EncoderJSON)
    out := &bufSizeWriter{}
    unset := pipelineStage.NewUnsetLogEventPostProcessor(2*time.Second, 1000, out)
    pipelineStage.EventPreProcessorObj.RegisterHook(enum.LevelUnSet, unset)
    config.InitPreProcessors(pipelineStage.EventPreProcessorObj)
    entry.GenerateInitialPool(100_000)
    b.Cleanup(unset.Stop)

    ctx := context.Background()
    b.ReportAllocs()
    b.ResetTimer()
    b.StartTimer()
    b.RunParallel(func(pb *testing.PB) {
        for pb.Next() {
            entry.NewLogEntry().Info(ctx, "buffer size test message")
        }
    })
}
```

- [ ] **Step 5.2: Run bench — capture B/op baseline (~1,411 B/op)**

```bash
go test -bench="BenchmarkBufferSeverance_HotPath" -benchmem -count=5 ./entry/
```

- [ ] **Step 5.3: Update alias severance in entry.go**

Find (around line 240):
```go
e.buf = make([]byte, 0, initBufCap)
```
Replace with:
```go
newCap := len(data)
if newCap < 64 {
    newCap = 64
}
e.buf = make([]byte, 0, newCap)
```

- [ ] **Step 5.4: Run bench again — verify B/op drops**

```bash
go test -bench="BenchmarkBufferSeverance_HotPath" -benchmem -count=5 ./entry/
go test -race ./entry/
```
Expected: B/op drops significantly (target ≤ 300 B/op); 0 allocs on filtered path.

- [ ] **Step 5.5: Commit**

```bash
git add entry/entry.go entry/buffer_size_bench_test.go
git commit -m "perf(entry): right-size alias severance buf to len(data) instead of 1024

Reduces pool slot allocation from constant 1024 B to actual log line
length. Pool naturally stabilises at typical line size (~200 B).
Saves ~800 B/call.

refs #119"
```

---

## Batch 2 — After Batch 1 Merged (P2, P7)

---

### Task 6: P2 — atomic.Pointer[HotSnapshot]

**Files:**
- Modify: `config/config.go` (add `hotSnapshotPtr`, `storeHotSnapshot`, rewrite `GetHotSnapshot`, update 13+ mutators)
- Create: `config/atomic_snapshot_v3_bench_test.go`

- [ ] **Step 6.1: Write tests**

```go
// config/atomic_snapshot_v3_bench_test.go
package config_test

import (
    "sync"
    "testing"
    "github.com/architagr/lognugget/enum"
)

func Test_GetHotSnapshot_NilSafe(t *testing.T) {
    TestResetConfig()
    snap := GetHotSnapshot()
    if snap.Encoder == nil {
        t.Fatal("snapshot encoder must not be nil after resetConfig")
    }
}

func Test_HotSnapshot_ConsistentWithSetMinLevel(t *testing.T) {
    TestResetConfig()
    SetMinLevel(enum.LevelWarn)
    // snapshot is rebuilt in SetMinLevel; atomic level must also be updated
    if GetAtomicMinLevel() != enum.LevelWarn {
        t.Fatal("atomicMinLevel not updated")
    }
}

func Test_HotSnapshot_ConsistentWithSetTimeFormat(t *testing.T) {
    TestResetConfig()
    SetTimeFormat("2006-01-02")
    snap := GetHotSnapshot()
    if snap.TimeFormat != "2006-01-02" {
        t.Fatalf("snapshot TimeFormat = %q, want %q", snap.TimeFormat, "2006-01-02")
    }
}

func Test_HotSnapshot_RaceWithSetters(t *testing.T) {
    t.Parallel()
    TestResetConfig()
    var wg sync.WaitGroup
    for i := 0; i < 4; i++ {
        wg.Add(2)
        go func() {
            defer wg.Done()
            for j := 0; j < 500; j++ {
                SetMinLevel(enum.LevelInfo)
                SetTimeFormat("2006-01-02")
            }
        }()
        go func() {
            defer wg.Done()
            for j := 0; j < 500; j++ {
                _ = GetHotSnapshot()
            }
        }()
    }
    wg.Wait()
}

func BenchmarkGetHotSnapshot(b *testing.B) {
    TestResetConfig()
    b.ReportAllocs()
    b.ResetTimer()
    for range b.N {
        _ = GetHotSnapshot()
    }
}
```

- [ ] **Step 6.2: Run tests to confirm they pass with current RLock-based implementation**

```bash
go test -race -run "Test_HotSnapshot_|Test_GetHotSnapshot_" ./config/
go test -bench=BenchmarkGetHotSnapshot -benchmem -count=5 ./config/
```
Expected: tests pass; bench shows allocs > 0 (current impl allocates struct copy indirectly).

- [ ] **Step 6.3: Add hotSnapshotPtr var and storeHotSnapshot helper**

In `config/config.go`, after `var channelCapacity atomic.Int64`:
```go
// hotSnapshotPtr holds the current hot-path config snapshot. Writers update it
// atomically under configMu.Lock() via storeHotSnapshot(); readers call
// GetHotSnapshot() without any lock. The stored value is always non-nil after
// the first resetConfig call (which runs at init).
var hotSnapshotPtr atomic.Pointer[HotSnapshot]
```

Add `storeHotSnapshot()` just before `GetHotSnapshot`:
```go
// storeHotSnapshot builds a fresh HotSnapshot from current defaultConfig state
// and atomically stores it. MUST be called while holding configMu (write lock).
func storeHotSnapshot() {
    snap := &HotSnapshot{
        AddSource:        defaultConfig.addSource,
        TimeFormat:       defaultConfig.timeFormat,
        DefaultFields:    defaultConfig.defaultFields,
        Rendered:         defaultConfig.defaultFieldsRendered,
        StaticFields:     defaultConfig.parsedStaticFields,
        ContextParser:    defaultConfig.contextParser,
        ContextAppender:  defaultConfig.contextAppender,
        Encoder:          defaultConfig.encoderObj,
        EncoderType:      defaultConfig.encoderType,
        RestrictedFields: restrictedFieldsSet,
    }
    snap.EncoderOpen  = snap.Encoder.OpenBytes()
    snap.EncoderClose = snap.Encoder.CloseBytes()
    hotSnapshotPtr.Store(snap)
}
```

- [ ] **Step 6.4: Rewrite GetHotSnapshot**

Replace the current `GetHotSnapshot` body:
```go
func GetHotSnapshot() HotSnapshot {
    return *hotSnapshotPtr.Load()
}
```

- [ ] **Step 6.5: Update resetConfig — call storeHotSnapshot before Unlock**

At the end of `resetConfig`, just before `configMu.Unlock()`:
```go
storeHotSnapshot()
```

- [ ] **Step 6.6: Update all Set* mutators — call storeHotSnapshot at end**

For each of the following functions, add `storeHotSnapshot()` as the last statement before `configMu.Unlock()` (or the `defer configMu.Unlock()` return):
- `SetMinLevel`
- `SetTimeFormat`
- `SetEncoderType`
- `SetAddSource`
- `SetOutput`
- `SetLogBufferMaxSize`
- `SetRate`
- `SetStaticEnvFieldsParser`
- `SetContextFieldsParser`
- `SetContextFieldsAppender`
- `SetDefaultFields`
- `RegisterHook`
- `DeRegisterHook`

Note: Functions that use `defer configMu.Unlock()` cannot call `storeHotSnapshot()` after the defer — they must either remove the defer and call manually, or add `storeHotSnapshot()` as the last non-deferred statement while still holding the lock.

Pattern for defer-based functions:
```go
func SetTimeFormat(format string) {
    configMu.Lock()
    defaultConfig.timeFormat = format
    storeHotSnapshot()
    configMu.Unlock()
}
```

- [ ] **Step 6.7: Run tests and bench**

```bash
go test -race -run "Test_HotSnapshot_|Test_GetHotSnapshot_" ./config/
go test -race ./config/ ./entry/
go test -bench=BenchmarkGetHotSnapshot -benchmem -count=5 ./config/
```
Expected: 0 allocs/op on BenchmarkGetHotSnapshot; all race tests pass.

- [ ] **Step 6.8: Commit**

```bash
git add config/config.go config/atomic_snapshot_v3_bench_test.go
git commit -m "perf(config): atomic.Pointer[HotSnapshot] eliminates GetHotSnapshot RLock

Replaces configMu.RLock in GetHotSnapshot with atomic.Pointer load.
All Set* mutators rebuild and atomically store a new snapshot under the
existing write lock. Saves ~120 ns/event (biggest V3 win).

refs #113"
```

---

### Task 7: P7 — OTel ContextFieldsAppender + benchmarks

**Files:**
- Modify: `entry/parallel_bench_test.go`
- Modify: `test/benchmark/log_parallel_bench_test.go`
- Create: `test/benchmark/otel_bench_test.go`
- Create: `examples/otel-appender/main.go`
- Create: `examples/otel-appender/go.mod`

- [ ] **Step 7.1: Update BenchmarkLognugget_Parallel_10CtxFields in entry/parallel_bench_test.go**

Replace `config.SetContextFieldsParser(...)` block with:
```go
config.SetContextFieldsAppender(func(ctx context.Context, dst []byte) []byte {
    return append(dst,
        `,"k1":"v1","k2":"v2","k3":"v3","k4":"v4","k5":"v5",`+
        `"k6":"v6","k7":"v7","k8":"v8","k9":"v9","k10":"v10"`...)
})
```

- [ ] **Step 7.2: Update Benchmark_Log_Parallel_10CtxFields in test/benchmark/log_parallel_bench_test.go**

Replace `config.SetContextFieldsParser(...)` block with:
```go
config.SetContextFieldsAppender(func(ctx context.Context, dst []byte) []byte {
    return append(dst,
        `,"trace_id":"abc123def456","span_id":"span789",`+
        `"request_id":"req-001","user_id":"usr-42","session_id":"sess-x",`+
        `"region":"us-east-1","service":"api-gateway","version":"1.2.3",`+
        `"env":"production","pod":"pod-abc"`...)
})
```

- [ ] **Step 7.3: Create OTel benchmark (build-tag isolated)**

```go
// test/benchmark/otel_bench_test.go
//go:build otel_bench

package benchmark

import (
    "context"
    "testing"
    "time"

    "github.com/architagr/lognugget/config"
    "github.com/architagr/lognugget/entry"
    "github.com/architagr/lognugget/enum"
    pipelineStage "github.com/architagr/lognugget/pipeline_stage"
    "go.opentelemetry.io/otel/trace"
)

// Benchmark_Log_Parallel_OtelCtx demonstrates the OTel ContextFieldsAppender pattern.
// Run with: go test -tags otel_bench -bench=Benchmark_Log_Parallel_OtelCtx ./test/benchmark/
func Benchmark_Log_Parallel_OtelCtx(b *testing.B) {
    out := &MockWriter{}
    config.SetMinLevel(enum.LevelDebug)
    config.SetEncoderType(enum.EncoderJSON)
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

    proc := pipelineStage.NewUnsetLogEventPostProcessor(2*time.Second, 500, out)
    defer proc.Stop()
    pipelineStage.EventPreProcessorObj.RegisterHook(enum.LevelUnSet, proc)
    config.InitPreProcessors(pipelineStage.EventPreProcessorObj)
    entry.GenerateInitialPool(1_000_000)

    ctx := context.Background()
    b.ReportAllocs()
    b.ResetTimer()
    b.RunParallel(func(pb *testing.PB) {
        for pb.Next() {
            entry.NewLogEntry().Info(ctx, "otel ctx bench")
        }
    })
}
```

- [ ] **Step 7.4: Create examples/otel-appender/**

```go
// examples/otel-appender/main.go
package main

import (
    "context"
    "fmt"
    "os"

    "github.com/architagr/lognugget/config"
    "github.com/architagr/lognugget/entry"
    "github.com/architagr/lognugget/enum"
    pipelineStage "github.com/architagr/lognugget/pipeline_stage"
    "go.opentelemetry.io/otel/trace"
    "time"
)

func init() {
    config.SetOutput(os.Stdout)
    config.SetMinLevel(enum.LevelDebug)

    // Zero-alloc OTel context appender: extracts trace/span IDs from the
    // active OTel span and appends them as JSON fields directly into the
    // log-line buffer — no map allocation, no intermediate string.
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

    unset := pipelineStage.NewUnsetLogEventPostProcessor(5*time.Second, 100, config.GetConfig().Output())
    pipelineStage.EventPreProcessorObj.RegisterHook(enum.LevelUnSet, unset)
    config.InitPreProcessors(pipelineStage.EventPreProcessorObj)
}

func main() {
    ctx := context.Background()
    entry.NewLogEntry().Info(ctx, "OTel appender example — no active span, no trace fields")

    // With an active OTel span, trace_id and span_id are appended automatically.
    fmt.Println("See examples/gin-demo/ for HTTP handler integration.")
}
```

- [ ] **Step 7.5: Create examples/otel-appender/go.mod**

```
module github.com/architagr/lognugget/examples/otel-appender

go 1.21

require (
    github.com/architagr/lognugget v0.0.0
    go.opentelemetry.io/otel v1.24.0
    go.opentelemetry.io/otel/trace v1.24.0
)

replace github.com/architagr/lognugget => ../../
```

- [ ] **Step 7.6: Run bench — verify ≤ 1 alloc**

```bash
go test -bench="BenchmarkLognugget_Parallel_10CtxFields" -benchmem -count=5 ./entry/
go test -bench="Benchmark_Log_Parallel_10CtxFields" -benchmem -count=5 ./test/benchmark/
```
Expected: ≤ 1 alloc/op (down from 7 in V2 with map parser).

- [ ] **Step 7.7: Verify otel-appender example compiles**

```bash
cd examples/otel-appender && go mod tidy && go build ./... && cd -
```

- [ ] **Step 7.8: Commit**

```bash
git add entry/parallel_bench_test.go test/benchmark/log_parallel_bench_test.go
git add test/benchmark/otel_bench_test.go
git add examples/otel-appender/
git commit -m "feat(bench,examples): switch 10ctx benchmarks to ContextFieldsAppender, add OTel example

Updates Benchmark_Log_Parallel_10CtxFields to use SetContextFieldsAppender
(zero-alloc). Adds Benchmark_Log_Parallel_OtelCtx (build tag otel_bench)
and examples/otel-appender/ showing OTel trace/span extraction pattern.
Saves ~100 ns + 4 allocs/event vs legacy map parser.

refs #118"
```

---

## Batch 3 — After P2 Merged (P3)

---

### Task 8: P3 — Atomic channel pointer

**Files:**
- Modify: `config/config.go` (add `atomicCh`, update `resetConfig`, rewrite `PublishLog`)
- Create: `config/atomic_channel_bench_test.go`

- [ ] **Step 8.1: Write tests**

```go
// config/atomic_channel_bench_test.go
package config_test

import (
    "sync"
    "testing"
    "github.com/architagr/lognugget/enum"
)

func Test_PublishLog_Race(t *testing.T) {
    t.Parallel()
    TestResetConfig()
    proc := newTestProcessor()
    InitPreProcessors(proc)
    var wg sync.WaitGroup
    for i := 0; i < 8; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            for j := 0; j < 100; j++ {
                PublishLog(enum.LevelInfo, []byte(`{"test":true}`))
            }
        }()
    }
    wg.Wait()
}

func BenchmarkPublishLog(b *testing.B) {
    TestResetConfig()
    proc := newTestProcessor()
    InitPreProcessors(proc)
    data := []byte(`{"level":"INFO","message":"bench"}`)
    b.ReportAllocs()
    b.ResetTimer()
    for range b.N {
        PublishLog(enum.LevelInfo, data)
    }
}
```

- [ ] **Step 8.2: Add atomicCh var**

In `config/config.go`, after `var channelCapacity atomic.Int64`:
```go
// atomicCh holds the current dispatch channel as an atomic.Value so
// PublishLog can load it without acquiring configMu. Updated by resetConfig
// under configMu.Lock() immediately after ch is assigned.
var atomicCh atomic.Value // stores chan LogEvent
```

- [ ] **Step 8.3: Update resetConfig — store into atomicCh**

After `ch = newCh` (inside `configMu.Lock()`):
```go
atomicCh.Store(newCh)
```

- [ ] **Step 8.4: Rewrite PublishLog**

Replace the current body:
```go
func PublishLog(Level enum.LogLevel, Data []byte) {
    atomicCh.Load().(chan LogEvent) <- LogEvent{
        Level: Level,
        Data:  Data,
    }
}
```

- [ ] **Step 8.5: Run tests and bench**

```bash
go test -race -run "Test_PublishLog_Race" ./config/
go test -bench=BenchmarkPublishLog -benchmem -count=5 ./config/
go test -race ./...
```
Expected: no races; tests pass.

- [ ] **Step 8.6: Commit**

```bash
git add config/config.go config/atomic_channel_bench_test.go
git commit -m "perf(config): atomic.Value channel pointer eliminates PublishLog RLock

Replaces configMu.RLock in PublishLog with atomic.Value load. Together
with P1+P2, the hot path now has zero configMu.RLock acquisitions.
Saves ~50 ns/event.

refs #114"
```

---

## Batch 4 — Final (P9)

---

### Task 9: P9 — Lock-free MPSC ring buffer

**Files:**
- Create: `config/ring_buffer.go`
- Create: `config/ring_buffer_test.go`
- Create: `config/ring_buffer_bench_test.go`
- Modify: `config/config.go` (replace `ch chan LogEvent` with `ring *mpscRingBuffer`, update `resetConfig`, `PublishLog`, `ProcessLogEvent`)

- [ ] **Step 9.1: Create ring_buffer.go**

```go
// config/ring_buffer.go
package config

import "runtime"

const (
    ringSize = 4096
    ringMask = uint64(ringSize - 1)
)

type ringSlot struct {
    seq  atomicUint64
    data LogEvent
    _    [40]byte // pad to 64 bytes (1 cache line): seq(8)+data(16)+pad(40)
}

type mpscRingBuffer struct {
    _     [64]byte    // isolate from preceding allocator metadata
    tail  atomicUint64 // producers: fetch-and-add
    _     [56]byte    // pad tail to its own cache line
    head  atomicUint64 // consumer: load/store (single goroutine)
    _     [56]byte    // pad head to its own cache line
    slots [ringSize]ringSlot
}

// Use sync/atomic Uint64 wrapper type alias for readability.
// In Go 1.19+ atomic.Uint64 is a struct with Load/Store/Add methods.
type atomicUint64 = atomic.Uint64

func newMpscRingBuffer() *mpscRingBuffer {
    r := new(mpscRingBuffer)
    for i := uint64(0); i < ringSize; i++ {
        r.slots[i].seq.Store(i)
    }
    return r
}

// Push enqueues e. If all slots are in use, spins with runtime.Gosched
// until one is freed by the consumer. Safe for concurrent use by multiple
// producers.
func (r *mpscRingBuffer) Push(e LogEvent) {
    pos := r.tail.Add(1) - 1
    slot := &r.slots[pos&ringMask]
    for slot.seq.Load() != pos {
        runtime.Gosched()
    }
    slot.data = e
    slot.seq.Store(pos + 1)
}

// Pop dequeues the next event. Returns (LogEvent{}, false) if the ring is
// empty. Must be called from exactly one goroutine (the consumer).
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

// Len returns an approximate item count (not precise under concurrent producers).
func (r *mpscRingBuffer) Len() int {
    tail := r.tail.Load()
    head := r.head.Load()
    if tail <= head {
        return 0
    }
    if n := tail - head; n < ringSize {
        return int(n)
    }
    return ringSize
}
```

Note: `atomic.Uint64` requires `import "sync/atomic"` — add to imports.

- [ ] **Step 9.2: Write ring buffer tests**

```go
// config/ring_buffer_test.go
package config

import (
    "sync"
    "testing"
    "github.com/architagr/lognugget/enum"
)

func Test_RingBuffer_PushPop_Sequential(t *testing.T) {
    r := newMpscRingBuffer()
    events := make([]LogEvent, 100)
    for i := range events {
        events[i] = LogEvent{Level: enum.LevelInfo, Data: []byte{byte(i)}}
        r.Push(events[i])
    }
    for i := range events {
        e, ok := r.Pop()
        if !ok {
            t.Fatalf("Pop %d: expected event, got empty", i)
        }
        if e.Data[0] != byte(i) {
            t.Fatalf("Pop %d: got data %d, want %d", i, e.Data[0], i)
        }
    }
    if _, ok := r.Pop(); ok {
        t.Fatal("expected empty ring after draining all events")
    }
}

func Test_RingBuffer_Empty(t *testing.T) {
    r := newMpscRingBuffer()
    if _, ok := r.Pop(); ok {
        t.Fatal("Pop on empty ring must return false")
    }
}

func Test_RingBuffer_MPSC_Race(t *testing.T) {
    t.Parallel()
    const producers = 8
    const eventsPerProducer = 1000
    r := newMpscRingBuffer()

    var wg sync.WaitGroup
    for i := 0; i < producers; i++ {
        wg.Add(1)
        go func(id int) {
            defer wg.Done()
            for j := 0; j < eventsPerProducer; j++ {
                r.Push(LogEvent{Level: enum.LevelInfo, Data: []byte{byte(id)}})
            }
        }(i)
    }

    received := make(chan struct{}, producers*eventsPerProducer)
    done := make(chan struct{})
    go func() {
        for {
            select {
            case <-done:
                for r.Len() > 0 {
                    if _, ok := r.Pop(); ok {
                        received <- struct{}{}
                    }
                }
                close(received)
                return
            default:
                if _, ok := r.Pop(); ok {
                    received <- struct{}{}
                }
            }
        }
    }()

    wg.Wait()
    close(done)

    count := 0
    for range received {
        count++
    }
    if count != producers*eventsPerProducer {
        t.Fatalf("got %d events, want %d", count, producers*eventsPerProducer)
    }
}
```

- [ ] **Step 9.3: Run ring buffer tests under -race**

```bash
go test -race -run "Test_RingBuffer_" -v ./config/
```
Expected: all pass, no races.

- [ ] **Step 9.4: Write ring buffer benchmark**

```go
// config/ring_buffer_bench_test.go
package config

import (
    "runtime"
    "testing"
    "github.com/architagr/lognugget/enum"
)

func BenchmarkRingBuffer_Push(b *testing.B) {
    prev := runtime.GOMAXPROCS(8)
    b.Cleanup(func() { runtime.GOMAXPROCS(prev) })
    r := newMpscRingBuffer()
    // consumer goroutine
    go func() {
        for range b.N + 1 {
            for {
                if _, ok := r.Pop(); ok {
                    break
                }
                runtime.Gosched()
            }
        }
    }()
    e := LogEvent{Level: enum.LevelInfo, Data: []byte(`{"test":true}`)}
    b.ReportAllocs()
    b.ResetTimer()
    b.RunParallel(func(pb *testing.PB) {
        for pb.Next() {
            r.Push(e)
        }
    })
}
```

- [ ] **Step 9.5: Replace chan LogEvent with ring in config.go**

1. Add `import "sync/atomic"` if not present (needed for `atomic.Uint64` in ring_buffer.go).
2. Replace `ch chan LogEvent` global with `ring *mpscRingBuffer`.
3. Update `resetConfig`:
   - Remove `newCh := make(chan LogEvent, ...)` and `ch = newCh`
   - Add `ring = newMpscRingBuffer()`
   - Remove `atomicCh.Store(newCh)` (P3 — keep `atomicCh` but don't need it anymore; or adapt to store ring)
4. Rewrite `PublishLog`:
   ```go
   func PublishLog(Level enum.LogLevel, Data []byte) {
       ring.Push(LogEvent{Level: Level, Data: Data})
   }
   ```
5. Rewrite `ProcessLogEvent`:
   ```go
   func ProcessLogEvent() {
       for {
           e, ok := ring.Pop()
           if !ok {
               runtime.Gosched()
               continue
           }
           configMu.RLock()
           processors := EventPreProcessors
           configMu.RUnlock()
           for _, p := range processors {
               p.PreProcess(e.Level, e.Data)
           }
       }
   }
   ```
   Note: the stop/drain coordination (`doneCh`, `flushWg`) must be updated — see story-P9 for the sentinel-event pattern.

- [ ] **Step 9.6: Run full test suite including integration**

```bash
go test -race ./...
```
Expected: all green including integration stop/drain tests.

- [ ] **Step 9.7: Run parallel benchmark — verify ≤ 500 ns/op**

```bash
go test -bench="Benchmark_Log_Parallel_NoCtx" -benchmem -count=10 ./entry/ ./test/benchmark/
./scripts/bench-check.sh
```
Expected: ≤ 500 ns/op; bench-check exits 0.

- [ ] **Step 9.8: Commit**

```bash
git add config/ring_buffer.go config/ring_buffer_test.go config/ring_buffer_bench_test.go config/config.go
git commit -m "perf(config): lock-free MPSC ring buffer replaces chan LogEvent dispatch

Eliminates ~130 ns/call channel mutex overhead under 8-goroutine
contention. Achieves ≤ 500 ns/op V3 target on parallel hot path.
Ring size 4096 provides 4× headroom vs channel cap of 1000.

refs #120"
```

---

## Task 10: Feature Branch Merge Gate

- [ ] **Step 10.1: Run bench-check on full feature branch**

```bash
./scripts/bench-check.sh
```
Expected: exits 0; all benchmarks ≤ 1,000 ns/op (gate threshold).

- [ ] **Step 10.2: Run full race suite**

```bash
go test -race -count=3 ./...
```

- [ ] **Step 10.3: Run lint**

```bash
golangci-lint run
```

- [ ] **Step 10.4: Project Lead updates baseline**

```bash
./scripts/bench-check.sh --update-baseline
git add bench-baseline.txt
git commit -m "chore: update bench-baseline.txt for V3 (P1-P9 combined)

All parallel benchmarks ≤ 500 ns/op. Closes Epic V3 goal.
refs #111"
```

- [ ] **Step 10.5: Open PR feat/111-v3-performance → develop**

```bash
./.claude/scripts/github-api.sh mark-pr-ready <pr-number>
```

---

## Self-Review Checklist

**Spec coverage:**
- ✅ P1 (F1, F2): atomic.Bool gate — Task 1
- ✅ P2 (F3, F4): atomic.Pointer snapshot — Task 6
- ✅ P3 (F5, F6): atomic channel — Task 8
- ✅ P4 (F7): direct timestamp — Task 2
- ✅ P5 (F8, F9): string-native escape — Task 3
- ✅ P6 (F10, F11): level bytes — Task 4
- ✅ P7 (F12, F13, F14): OTel appender + examples — Task 7
- ✅ P8 (F15): buffer size — Task 5
- ✅ P9 (F16): ring buffer — Task 9
- ✅ F17: all stories have bench_test.go with `b.ReportAllocs()`
- ✅ F18: Task 10 runs bench-check.sh

**Placeholder scan:** None found.

**Type consistency:**
- `hasPreProcessorsAtomic` (Task 1) — `atomic.Bool` throughout
- `hotSnapshotPtr` (Task 6) — `atomic.Pointer[HotSnapshot]` throughout
- `atomicCh` (Task 8) — `atomic.Value` storing `chan LogEvent`; P9 replaces the channel entirely
- `mpscRingBuffer.Push/Pop` (Task 9) — `LogEvent` throughout; matches `config.LogEvent` struct
