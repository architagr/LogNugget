# LogNugget — Low-Level Design (v1)

Status: Draft for EM review.
Owner: Chief Architect.
Source PRD: `docs/prds/lognugget.md`. Source Wiki: `docs/wiki/lognugget.md`.
Hot-path SLO: < 1 µs p99 (CLAUDE.md / PRD §4.1 NF1). Bench gate ceiling 1,000 ns/op.
Current bench: ~1,246 ns/op @ ~25 allocs/op — gate already RED before any new work.

This LLD is structure & mechanics only. PRD owns intent. F-IDs reference PRD §3.

---

## 1. Scope

LogNugget v1 library. Hot path = `*LogEntry.Log` returning to caller. PRD §1, §3, §4. F1–F31 traced below.

---

## 2. Package layout

| Package | Role | Pattern | LOC budget |
|---|---|---|---|
| `entry` | Public call-site API; `LogEntry` struct; pool warm-up. Hot path origin. | Object Pool + Facade | ~280 |
| `config` | Startup-only setters; dispatch channel; dispatcher goroutine; field rendering helpers; reserved-key collision logic. | Singleton + Strategy (encoder) + Producer | ~360 |
| `pipeline_stage` | Pre-processor (fan-out to hooks); default `unsetLogEventPostProcessor` (batch + flush); hook register/deregister. | Observer + Producer-Consumer + Chain | ~260 |
| `encoder` | Encoder interface + JSON (RFC 8259 F10) + Text + factory. | Strategy + Factory Method | ~340 |
| `model` | `LogAttr` user-facing field type. | Value Object | ~25 |
| `enum` | Levels (bitmask), encoder type, default-key vocabulary. | Constant table | ~120 |
| `custom_time` | UTC-clamped `TimeNow`; format helper. | Pure helper | ~30 |
| `lognugget` (NEW, top-level one file) | `Shutdown()` thin wrapper; default post-processor handle. F31. | Facade | ~50 |
| `examples/` (NEW) | Relocated `logger.go` (Gin demo). NF12 isolation. | Sample app | n/a |
| `test/benchmark` | `Benchmark_Log` + `Benchmark_LogParallel` (NEW). | Bench harness | ~150 |

Module root stays `github.com/architagr/lognugget`. Library `go.mod` retains stdlib-only runtime deps. Demo deps move to `examples/go.mod` or `// +build ignore` (PRD NF12).

---

## 3. Hot-path design

Reference: `diagrams/hot-path-flow.svg`. Stage budget for `addSource=false`, JSON, 16 fields, 2 ctx-fields, 1 static-string append, no error.

| # | Stage | Pattern | Target ns | Allocs | Notes |
|---|---|---|---|---|---|
| 1 | `entry.Info(ctx,...)` call | direct | ~10 | 0 | tail call into `Log` |
| 2 | `entryPool.Get` + `reset()` | Object Pool | ~30 | 0 amortized | sync.Pool P-local |
| 3 | min-level gate (F6) | Guard | ~5 | 0 | `if cfg.minLevel > level { Put; return }` — < 100 ns total filtered path |
| 4 | `time.Now().UTC().AppendFormat` (F28 RFC3339) | Strategy | ~80 | 0 | append into pooled buf, no `string` round-trip |
| 5 | `runtime.Caller(skip)` (F17) | Lazy compute | ~250 if on / 0 if off | 1 if on | gated by `cfg.addSource`; never `runtime.CallersFrames` |
| 6 | render fields → pooled `[]byte` (F12, F14, F27) | Builder | ~250 | 0 amortized | escape per-byte, escape-table lookup |
| 7 | ctx parser invoke (F16) | Strategy callback | ~120 | 1 (map alloc) | only if `ctx != nil && parser != nil` |
| 8 | encoder.Write (F7, F9, F10) | Strategy | ~150 | 1 (output `[]byte`) | JSON wraps `{`+buf+`}`; Text passthrough |
| 9 | `ch <- LogEvent{Level,Data}` (F21–F23) | Producer | ~120 | 0 | channel-send sched cost dominant |
| 10 | `LogEntry.Put()` | Object Pool | ~25 | 0 | last op |
| **Total** | | | **~890 ns** addSource off / **~1,140 ns** on | ≤ 5 | gate is 1,000 → addSource=true is RED — see §3.1 |

### 3.1 Hitting the < 1,000 ns ceiling

Only viable plan:

1. Rewrite `ParseLogField` (config.go:155) — eliminate `fmt.Sprintf` for ints/floats. Use `strconv.AppendInt`/`AppendFloat`. Save ~200 ns per int field.
2. Drop `strings.Builder` pattern entirely. Single pooled `[]byte` per `LogEntry`, all renders `append`-only. Save ~150 ns + ≥ 5 allocs.
3. Drop `strings.Join(data, ", ")` (entry.go:91). The `data []string` slice is wasted intermediate. Save ~100 ns + 1 alloc.
4. JSON encoder writes directly into pooled buf instead of returning a fresh `[]byte` (encoder/json_encoder.go:21). Save 1 alloc + ~50 ns.
5. `LogEvent.Data` is a sub-slice over a freshly `copy`-ed buf. The pooled buf returns to pool inside `Log`; the channel-side payload is independent. Save: avoids retaining pool buf across goroutine boundary.
6. Defer `runtime.Caller` to F17 — never invoke when `cfg.addSource == false`. Today `cfg.addSource` defaults to `true` in `config.go:20` (DEFECT D-7). Flip default to `false`; opt-in.
7. ctx parser map: callers' parsers return `map[string]any`. v1 keeps map (F16 contract). Mitigation: encourage 2-key map; iteration cost is the bound.

After (1)–(6): projected ~720 ns addSource off, ~970 ns addSource on. ~30 ns headroom.

### 3.2 Filtered-path budget (F6)

`if cfg.minLevel > level { Put(); return }`. No work past gate. Target: < 80 ns, 0 allocs. Current code at entry.go:62 also touches `config.EventPreProcessors` — keep both checks behind level gate, do NOT short-circuit on processors==nil before level check (lower cost).

### 3.3 Allocation budget

NF3 ceiling: 30 allocs/op, 2 KB/op. Per-op accounting after rewrite:

| Source | Allocs | Bytes |
|---|---|---|
| `LogEvent` payload `[]byte` (final) | 1 | ~600 |
| ctx parser `map` (user code) | 1 | ~200 |
| ctx parser `map` iter `string` keys | 0 | 0 |
| `runtime.Caller` Frame.Function | 1 | ~32 |
| field-key boxes via `any` | up to 16 | ~256 |
| Total addSource off | ~10 | ~800 |
| Total addSource on | ~12 | ~900 |

20-alloc / 1.2 KB headroom vs gate.

---

## 4. Type model

### 4.1 `entry` package

| Symbol | Kind | Purpose | Key fields/methods | F-ID |
|---|---|---|---|---|
| `LogEntry` | struct | per-call working buffer; pooled | `caller *runtime.Frame`, `buf []byte` (NEW pooled), `level enum.LogLevel` | F3, F25, F27 |
| `entryPool` | var sync.Pool | reservoir | `New: initLogEntry` | F25 |
| `NewLogEntry()` | func | Get + reset | returns `*LogEntry` | F3 |
| `GenerateInitialPool(n)` | func | warm-up | `for i<n: Put(initLogEntry())` | F4 |
| `(*LogEntry).reset()` | method | clear all per-call state | nil caller, `buf=buf[:0]`, level=0 | F26 |
| `(*LogEntry).Put()` | method | return to pool | `entryPool.Put(e)` | F25 |
| `(*LogEntry).Log(level, ctx, msg, err, fields...)` | method | hot path | gate, render, encode, send, Put | F2, F6, F12, F17, F21 |
| `(*LogEntry).Debug/Info/Warn` | method | sugar | call `Log(level, ctx, msg, nil, fields)` | F2 |
| `(*LogEntry).Error(ctx, err, msg, fields)` | method | with err | `Log(LevelError, ctx, msg, err, fields)` | F2 |
| `(*LogEntry).Fatal/Panic` | method | err+terminate | `Error` then `runtime.Goexit()` / `panic(err)` | F2 |

### 4.2 `config` package

| Symbol | Kind | Purpose | Key fields/methods | F-ID |
|---|---|---|---|---|
| `Config` | struct (singleton) | startup config | `minLevel, encoderType, encoderObj, addSource, output, logBufferMaxSize, rate, parsedStaticFields, contextParser, defaultFields, timeFormat`, `defaultFieldsRendered [N]string` (NEW pre-rendered key prefix cache) | N6, F5–F29 |
| `defaultConfig` | var *Config | singleton instance | per-process | N6 |
| `LogEvent` | struct | channel payload | `Level enum.LogLevel; Data []byte` | F21 |
| `ch` | var chan LogEvent | dispatch channel | buffer 10, started in init | F21, F22 |
| `EventPreProcessors` | var map[string]preProcessingObserverContract | dispatcher targets | populated by `InitPreProcessors` | F21 |
| `PublishLogMessageHookContract` | iface | hook contract | `PublishLogMessage([]byte); Name() string` | F18 |
| `preProcessingObserverContract` | iface | pre-proc observer | `PreProcess(level, []byte); Name() string` | F21 |
| `StaticEnvFieldsParser` | type | startup parser | `func() map[string]any` | F15 |
| `ContextFieldsParser` | type | per-call parser | `func(ctx) map[string]any` | F16 |
| `Set*` setters | func | startup wiring | `SetMinLevel, SetEncoderType, SetAddSource, SetOutput, SetLogBufferMaxSize, SetRate, SetTimeFormat, SetStaticEnvFieldsParser, SetContextFieldsParser, SetDefaultFields, RegisterHook, DeRegisterHook` | F2, F7, F13, F15, F16, F17, F18, F22, F24, F29 |
| `PublishLog(level, data)` | func | channel send | `ch <- LogEvent{level,data}` blocking | F21, F23 |
| `ProcessLogEvent()` | func | dispatcher loop | `for e := range ch { for _,o := range obs { o.PreProcess(e.Level,e.Data) } }` | F21 |
| `ParseLogField(key, value any) []byte` (RENAMED, signature change) | func | render single k:v into byte slice | strconv-based, no fmt.Sprintf | F10 |
| `AppendField(buf []byte, key string, value any) []byte` (NEW) | func | render into caller-owned buf | append-only, no alloc | F10, F27 |
| `ValidateandParseLogField` | func | F14 collision wrap | uses `restrictedFieldsSet` (NEW: map-set, not slice) | F14 |
| `MinLevel/Encoder/...` accessors | method | read singleton | direct field reads | hot-path |
| `restrictedFieldsSet` | var map[string]struct{} | F14 lookup | populated by `SetDefaultFields` | F14 |

### 4.3 `pipeline_stage` package

| Symbol | Kind | Purpose | Key fields/methods | F-ID |
|---|---|---|---|---|
| `eventPreProcessorObserver` | struct | hook fan-out | `mu sync.RWMutex; hooks map[enum.LogLevel]map[string]hookContract` (NEW: add RWMutex for safe register at runtime — but documented as startup-only) | F18, F19 |
| `EventPreProcessorObj` | var *eventPreProcessorObserver | singleton | `sync.Once` init | NF8 |
| `RegisterHook(level, hook)` / `DeRegisterHook(level, name)` | method | manage hooks | startup-only | F18 |
| `PreProcess(level, []byte)` | method | dispatch | `publish(unsetHooks); publish(levelHooks)` | F19 |
| `unsetLogEventPostProcessor` | struct | default sink | `mu sync.Mutex; activeBucket [][]byte; maxBucketSize int; rate Duration; ticker *Ticker; output io.Writer; stopCh chan struct{}; doneCh chan struct{}` (NEW doneCh) | F20, F24, F30 |
| `NewUnsetLogEventPostProcessor(rate, max, w)` | func | constructor | starts watcher | F20 |
| `PublishLogMessage(entry []byte)` | method | append to bucket | locks, appends, conditional flush | F24 |
| `flushLogMessages()` | method | swap + spawn writer | swaps bucket, `go printMessage(swap)` | F24 |
| `printMessage(data [][]byte)` | method | sync write | best-effort write per entry, ignore errors | NF10 |
| `activeBucketWatcher()` | method | ticker loop | select ticker / stopCh; drain on stop | F30 |
| `Stop()` | method | close stopCh, wait doneCh | drains active bucket then signals | F30, SC5 |

### 4.4 `encoder` package

| Symbol | Kind | Purpose | Key fields/methods | F-ID |
|---|---|---|---|---|
| `Encoder` | iface | strategy | `Append(dst []byte, body []byte) []byte` (NEW signature) and legacy `Write(string) ([]byte, error)` (kept until v1 cutover) | F7, F8 |
| `JSONEncoder` | struct | RFC 8259 (F10) | `escapeTable [256]uint8` precomputed; `Append` wraps `{` + body + `}` | F9, F10 |
| `TextEncoder` | struct | passthrough | `Append` returns body as-is | F9 |
| `DefaultEncoderFactory(t LogEncodeType)` | func | factory | switch by type | F7 |
| `escapeJSON(dst, src []byte) []byte` | func | per-byte escape | uses precomputed table; handles `"`, `\`, `\b\f\n\r\t`, U+0000–U+001F, valid UTF-8 passthrough; non-UTF-8 → `�` | F10 |
| `appendNumber(dst, value any) []byte` | func | unquoted numeric | `strconv.AppendInt/Float`; nan/inf → JSON null | F10 |

### 4.5 `model`, `enum`, `custom_time`, `lognugget`

| Symbol | Kind | Purpose | F-ID |
|---|---|---|---|
| `model.LogAttr{Key LogAttrKey; Value LogAttrValue}` | struct | user field | F2 |
| `enum.LogLevel` (bitmask) + 6 constants | type+const | levels | F5 |
| `enum.DefaultLogKey` + 40+ const | type+const | reserved keys | F13 |
| `enum.LogEncodeType` + 2 const | type+const | encoder selector | F7 |
| `customTime.TimeNow() time.Time` | func | UTC clamp | F28 |
| `customTime.AppendFormat(dst []byte, t time.Time, layout string) []byte` (NEW) | func | alloc-free formatting | F27, F28 |
| `lognugget.defaultPostProc *unsetLogEventPostProcessor` | var | zero-config sink handle | F20, F31 |
| `lognugget.init()` | func | wire defaultPostProc, register on LevelUnSet, InitPreProcessors | F20 |
| `lognugget.Shutdown()` | func | `defaultPostProc.Stop()` | F31 |

---

## 5. Concurrency model

### 5.1 Goroutines

| Goroutine | Started by | Started when | Stopped how |
|---|---|---|---|
| dispatcher (`config.ProcessLogEvent`) | `config.init` (or `ResetConfig`) | package init | not stopped in v1 (process-lifetime); v2 may add stop |
| watcher (`activeBucketWatcher`) | `NewUnsetLogEventPostProcessor` | per post-processor | `Stop()` closes `stopCh`, watcher drains then returns |
| flush worker (`printMessage`) | `flushLogMessages` | per flush | exits after writing bucket |
| caller goroutines (N) | consumer | per request | n/a |

### 5.2 Channels

| Channel | Direction | Buffer | Purpose | Backpressure |
|---|---|---|---|---|
| `config.ch chan LogEvent` | callers → dispatcher | 10 (F22) | hand-off | blocks caller (F23, NF5) |
| `unsetLogEventPostProcessor.stopCh chan struct{}` | Stop() → watcher | 0 | shutdown signal | close-only |
| `unsetLogEventPostProcessor.doneCh chan struct{}` (NEW) | watcher → Stop() | 0 | drain ack | enables SC5 sync |

### 5.3 Locks

| Lock | Owner | Scope | Held during |
|---|---|---|---|
| `entryPool` (sync.Pool internal) | runtime | per-P slabs | Get/Put |
| `eventPreProcessorObserver.mu sync.RWMutex` (NEW) | preprocessor | hooks map | RLock on PreProcess, Lock on Register/DeRegister; document register is startup-only but `RWMutex` keeps `-race` clean |
| `unsetLogEventPostProcessor.mu sync.Mutex` | postProc | activeBucket | append + conditional swap (NF9) |

Hot path: ZERO lock contention on caller side. Channel send is the only cross-goroutine sync.

### 5.4 sync.Once usage

| Site | Guards |
|---|---|
| `pipelineStage.init` | `EventPreProcessorObj` construction (NF8) |
| `lognugget.init` (NEW) | default post-processor + register-on-LevelUnSet (idempotent) |
| `config.init` | `ch`, dispatcher start (currently re-runs in `ResetConfig` — DEFECT D-9) |

### 5.5 Race-freedom checklist

- All `Set*` setters documented startup-only (NF7) — emit `// startup-only; not safe to call concurrently with active logging` doc on each.
- `go test -race` covers: `Benchmark_LogParallel`, `TestPostProcessorConcurrent`, `TestHookRegisterRace`.
- `restrictedFieldsSet` populated under `SetDefaultFields`; lookups are read-only after; no race in steady-state.
- `LogEvent.Data` is a fresh `[]byte` (copy of pooled buf at send time) → no aliasing across goroutines.

---

## 6. Pipeline & hooks

Reference: `diagrams/pipeline-overview.svg`.

### 6.1 Dispatch table (F19)

| Hook registered at | Receives |
|---|---|
| `LevelUnSet` | every event (Debug, Info, Warn, Error, Fatal) |
| `LevelDebug` | Debug events only |
| `LevelInfo` | Info events only |
| `LevelWarn` | Warn events only |
| `LevelError` | Error + Fatal + Panic events |
| `LevelFatal` | Fatal events only |

`PreProcess` order: `publish(hooks[LevelUnSet]); publish(hooks[level])`. Iteration order over hooks within a level is undefined (Go map). Callers must not assume ordering. Documented.

### 6.2 Hook contract (F18)

```go
type PublishLogMessageHookContract interface {
    PublishLogMessage(entry []byte)  // MUST be fast or fan-out internally; runs on dispatcher goroutine
    Name() string                     // unique key for register/deregister
}
```

Doc: hook is invoked on the single dispatcher goroutine. Slow hooks block channel drain → callers block on `ch <-`. PRD R2.

### 6.3 Pre-processor (Observer)

Pattern: Observer. Why: dispatcher fans events to N independent destinations without coupling.

```go
func (e *eventPreProcessorObserver) PreProcess(level enum.LogLevel, msg []byte) {
    e.mu.RLock()
    unset := e.hooks[enum.LevelUnSet]
    lvl   := e.hooks[level]
    e.mu.RUnlock()
    publish(msg, unset)
    publish(msg, lvl)
}
```

### 6.4 Post-processor (Producer-Consumer + double-buffer)

Pattern: Producer-Consumer with double-buffered swap. Why: append-only producer + ticker/size-triggered consumer; eliminates lock held during IO.

Flush triggers (F24):
- `len(activeBucket) >= maxBucketSize` after append → swap and spawn flush goroutine
- `ticker.C` fires → swap (skip if empty) and spawn flush goroutine

Empty-bucket short-circuit (R5): `if len == 0 { return }` after lock acquired — already in current code; protected by `TestPostProcessorIdleNoFlush`.

### 6.5 Pre-rendered key prefix optimization

For each `enum.DefaultLogKey`, on `SetDefaultFields` (or first read), pre-build `[]byte("\"<key>\":")` and store in `Config.defaultFieldsRendered`. `Log` appends prefix + value-render directly — saves ~80 ns over `ParseLogField`-style per-call assembly. F12 / F13.

---

## 7. Encoders

### 7.1 Strategy

Pattern: Strategy + Factory Method.
Why: encoder swap at startup via `SetEncoderType`. Hot path holds the resolved `encoder.Encoder` in `defaultConfig.encoderObj` — no per-call lookup.

### 7.2 Interface (signature change)

Current: `Write(string) ([]byte, error)` — forces stringification of buf and a `[]byte` alloc per call.

LLD v1:
```go
type Encoder interface {
    // Append wraps `body` (rendered key-value bytes, no surrounding braces)
    // into final payload, appending into dst and returning the extended slice.
    Append(dst []byte, body []byte) []byte
    Name() string
}
```

`Write(string) ([]byte, error)` becomes a thin shim during M2 cutover. Drop after callers migrated (story-cuttable: 1 story per encoder).

### 7.3 JSON encoder (F10, RFC 8259)

```go
func (e *JSONEncoder) Append(dst, body []byte) []byte {
    dst = append(dst, '{')
    dst = append(dst, body...)
    dst = append(dst, '}', '\n')   // newline lives here; SC1
    return dst
}
```

`body` is built by `config.AppendField` which itself is RFC 8259 compliant:

| Value type | Render | Escaping |
|---|---|---|
| `string` | `"..."` | per-byte: `"`, `\`, `\b\f\n\r\t`, ctrl 0x00–0x1F → `\u00xx`, valid UTF-8 passthrough, invalid UTF-8 byte → `�` |
| `int*`,`uint*` | unquoted, `strconv.AppendInt` | n/a |
| `float32/64` | unquoted, `strconv.AppendFloat`; NaN/±Inf → `null` | n/a |
| `bool` | `true`/`false` | n/a |
| `error` | `.Error()` then string-rules | per-byte |
| `time.Time` | `.AppendFormat(layout)` then string-rules | per-byte |
| default | `fmt.Append("%+v", ...)` then string-rules | per-byte; documented as slow path |

Escape table: `var jsonEscapeTable [256]uint8` initialised at package init. 0 = passthrough, 1 = backslash escape, 2 = `\u00xx` form. Lookup is one indexed read per byte.

### 7.4 Text encoder

Pattern: Null Object / Passthrough.

```go
func (e *TextEncoder) Append(dst, body []byte) []byte {
    dst = append(dst, body...)
    dst = append(dst, '\n')
    return dst
}
```

Text encoder does not escape — caller-supplied newlines pass through (documented).

### 7.5 Factory

Pattern: Factory Method. Switch on `enum.LogEncodeType`. Falls back to JSON on unknown (current behavior, preserved).

---

## 8. Pool & allocation strategy

Reference: `diagrams/pool-state.svg`.

### 8.1 sync.Pool

Pattern: Object Pool.
Why: the only viable way to amortize `LogEntry` + `[]byte` allocation under > 100k events/sec/instance.

Pools (3):

| Pool | Element | Reset policy | Cap policy |
|---|---|---|---|
| `entryPool sync.Pool` | `*LogEntry` with attached `buf []byte` (cap 1KB initial) | reset(): caller=nil, buf=buf[:0], level=0, err=nil | grows naturally; never shrinks cap |
| `fieldSlicePool sync.Pool` (NEW, conditional) | `[]string` cap 32 | reset to `s[:0]` | drop entries with `cap > 256` to avoid retention |
| `ctxMapBufPool sync.Pool` (NEW, conditional) | `[]byte` cap 256 | `b[:0]` | n/a |

`fieldSlicePool` and `ctxMapBufPool` only land if M2 baseline shows > 25 allocs after F10 (PRD §3.9 F27 SHOULD).

### 8.2 Reset semantics (F26)

`reset()` MUST clear every per-call field. Test `TestLogEntryResetExhaustive` enumerates every field via reflection and asserts zero-value after `reset()`.

### 8.3 Pre-sized buffers

`initLogEntry()` returns `&LogEntry{buf: make([]byte, 0, 1024)}`. Sized to median-rendered event (16 fields × ~50 bytes ≈ 800 bytes + overhead). Saves first-N grow allocations on cold pool entries.

### 8.4 Channel payload decoupling

`LogEvent.Data` MUST be a fresh `[]byte` (`copy(make([]byte, len(buf)), buf)`) — NOT a sub-slice of the pooled buf. Why: pooled buf returns to pool inside `Log`; if `LogEvent.Data` aliased it, dispatcher would race with next caller's writes. 1 alloc per send; counted in NF3.

### 8.5 GenerateInitialPool sizing (F4)

Recommended call: `entry.GenerateInitialPool(GOMAXPROCS * 64)`. Rationale: 64 entries per P covers steady-state working set without sync.Pool victim-cache thrash. Documented in README.

---

## 9. Lifecycle

Reference: `diagrams/lifecycle.svg`.

### 9.1 Init order

1. `enum`, `model`, `custom_time` packages — pure consts/types, no init logic.
2. `encoder.init` — populate `jsonEscapeTable`.
3. `config.init` (currently re-uses `ResetConfig` — DEFECT D-9) — build `defaultConfig`, allocate `ch=make(chan LogEvent, 10)`, `go ProcessLogEvent()`. Must happen exactly once; `sync.Once` wrap.
4. `entry.init` — `entryPool` with `New: initLogEntry`.
5. `pipelineStage.init` — `EventPreProcessorObj` via `sync.Once` (existing pattern is broken — see DEFECT D-3).
6. `lognugget.init` (NEW) — construct `defaultPostProc = NewUnsetLogEventPostProcessor(1*time.Second, 20, os.Stdout)`, `EventPreProcessorObj.RegisterHook(LevelUnSet, defaultPostProc)`, `config.InitPreProcessors(EventPreProcessorObj)`.

Step 6 is what makes SC1 (zero-config emits a JSON line on stdout) green without consumer wiring.

### 9.2 Stop / Shutdown

- `unsetLogEventPostProcessor.Stop()` (F30): close stopCh; watcher drains active bucket via repeated `flushLogMessages` until `len==0`; ticker.Stop; close doneCh; `Stop()` waits on doneCh (synchronous return).
- `lognugget.Shutdown()` (F31): `defaultPostProc.Stop()`. Idempotent via `sync.Once`.
- `config.ch` is NOT closed in v1 — closing risks `send on closed channel` panic from late callers (PRD N5: dynamic reconfig out-of-scope). Documented as known limitation.

### 9.3 SC5 drain test

`TestStopDrainsAllQueuedEvents`: enqueue `2 * maxBufferSize` events, call `Stop`, assert observed write count equals enqueued count. Passes iff Stop is synchronous.

---

## 10. Error handling & backpressure

| Failure mode | v1 policy | F-ID / NF-ID |
|---|---|---|
| dispatch channel full | block caller goroutine on `ch <-` | F23, NF5 |
| user hook slow / blocked | dispatcher blocks → callers block | R2; documented |
| `io.Writer.Write` returns error | error silently dropped | NF10, Wiki §7.6 |
| `io.Writer.Write` short-write | no retry; partial bytes lost | NF10 |
| `ContextFieldsParser` panics | recovered? | LLD decision: NO, propagates. Rationale: panic in user code is a bug; recover would mask it. Doc on `SetContextFieldsParser` |
| `runtime.Caller` returns ok=false | emit `caller="unknown"` | F17 |
| `encoder.Append` panics | not recovered (impossible with built-in encoders; user-defined encoders v2) | n/a |
| `Fatal` called | `Error` then `runtime.Goexit()`; deferred funcs run | F2 |
| `Panic` called | `Error` then `panic(err)` | F2 |
| level above LevelFatal | `level.String()` fallback `ERROR+N` (existing) | F5 |

Channel-full block is opt-out via post-processor sizing (`SetLogBufferMaxSize`) — not via dispatch channel buffer (deliberate, F22). `SetOverflowPolicy` is v2 (PRD NF5).

---

## 11. Test strategy (summary)

Full design: [`docs/lld/test-framework.md`](./test-framework.md). Diagrams: `diagrams/test-pyramid.svg`, `diagrams/bench-gate-flow.svg`, `diagrams/test-fixture-tree.svg`, `diagrams/coverage-map.svg`.

| Layer | Tool | Gate |
|---|---|---|
| Unit | testify + table-driven | green required (SC6) |
| Race overlay | `go test -race` | green required (NF9, SC6) |
| Integration | full pipeline + FakeWriter | green required |
| Property / Fuzz | `testing.F` (Go 1.21) | green required (SC4) |
| Bench | `testing.B` + benchstat | gated by `bench-check.sh` (NF1, NF3, NF6) |

CI sequence (fail-fast): `golangci-lint` → `go vet` → `go build` → `go test` → `go test -race` → `bench-check.sh`. CLAUDE.md bypass-not-permitted applies.

Tooling decisions: testify IN, `testing.F` IN, `go-cmp` IN, `benchstat` IN; `goleak` DEFERRED-V2 (process-lifetime dispatcher); `synctest` OUT (1.24+); `gotestsum`/`counterfeiter` OUT; `go-mutesting` DEFERRED-V2.

Patterns in use: Table-driven, Subtests, Parallel matrix, Builder (`ConfigBuilder`/`EntryBuilder`), Spy (`SpyHook`), Fake (`FakeWriter`/`FakePostProcessor`), Stub (`Stub*Parser`/`StubEncoder`/`FakeClock`), Golden Master, Property/Fuzz, Test Harness, Recording channel.

SC traceability:

| SC | Test (see test-framework.md §16) |
|---|---|
| SC1 | `Test_ZeroConfig_EmitsJSONLineOnStdout` + `Test_Integration_ZeroConfigEmitsJSONLine` |
| SC2 | `Benchmark_Log/_Parallel/_Filtered/_AddSourceTrue` + `bench-check.sh` |
| SC3 | `Test_LogEntry_CallerCapture_WhenAddSource` |
| SC4 | `Test_JSONEncoder_RFC8259_Roundtrip` + `FuzzJSONEncoder` (1000-event corpus) |
| SC5 | `Test_Integration_StopDrainsAllQueued` (≤ `maxBufferSize × 2`) |
| SC6 | CI green (lint + test + race) |
| SC7 | `Test_ValidateAndParse_PrefixesCollidingKey` + `Test_Integration_CollisionPrefix` |
| SC8 | `Test_PreProcess_UnsetGetsAll/_LevelOnly/_DeRegister` + `Test_Integration_HookFanout` |

Story-cuttable test work: 33 stories, each ≤ 300 LOC, enumerated in test-framework.md §18 (TS-01 … TS-33). Existing-test defect register: test-framework.md §17 (15 defects T-1 … T-15). Top-3 defects pulled forward: T-7 (false-green count assertion in post-processor test), T-3 (caller assertion only passes because F17 non-functional), T-9 (zerolog runtime dep in library bench harness — NF12 violation).

---

## 12. Current-code defect register

Each defect must be fixed in M1/M2 per PRD §7.2. Story-cuttable.

| ID | File:line | Defect | Required fix | F-ID / NF-ID |
|---|---|---|---|---|
| D-1 | `entry/entry.go:29` | `caller` declared; `TODO: add a function to set caller from runtime.Caller` — feature non-functional | Implement F17: lazy `runtime.Caller(skip)` in `Log` when `cfg.addSource`; emit `caller` field | F17, SC3 |
| D-2 | `encoder/json_encoder.go:21` | `Write` returns `[]byte("{" + entryData + "}")` — string-concat shortcut. NO escaping of `"`, `\`, ctrl chars; numerics quoted upstream by `ParseLogField`; not RFC 8259. | Replace with `Append(dst, body)` + escape table; numerics unquoted via `strconv.AppendInt/Float`. | F10, SC4 |
| D-3 | `pipeline_stage/pre_processing_stage.go:11-15` | `(&sync.Once{}).Do(...)` — fresh `sync.Once` per init call → does not provide singleton guarantee. Works only because `init` runs once but pattern is wrong and obscures intent. | Make `EventPreProcessorObj` package var; assign directly in `init`. Remove fake `sync.Once`. Document singleton (N6). | NF8 |
| D-4 | `config/config.go:21` | `DefaultTimeFormat = time.RFC822` — PRD F28 mandates `time.RFC3339`. | Change constant to `time.RFC3339`. | F28 |
| D-5 | `config/config.go:155` | `ParseLogField` uses `fmt.Sprintf("%d", value)` / `fmt.Sprintf("%f", value)` — > 200 ns per int-field, > 5 allocs. Also wraps numerics in `"..."` (not JSON-numeric). | Replace with `strconv.AppendInt/AppendFloat` writing into pooled `[]byte`. Numerics unquoted. F10. | F10, NF1 |
| D-6 | `entry/entry.go:68` | `data := make([]string, 3+len(fields)+len(ctxData), len(fields)+5)` — capacity LESS than length when `ctxData` non-empty → silent reslice / panic risk; also allocates per-call. | Drop `[]string` aggregator entirely; render directly into pooled `[]byte buf` with `AppendField`. F27. | F27, NF3 |
| D-7 | `config/config.go:20` | `DafaultAddSource = true` — defaults `addSource` ON, blowing the < 1µs budget by ~250 ns on zero-config. | Default OFF (`false`). README/doc updated. Consumer flips with `SetAddSource(true)`. | NF1, R4 |
| D-8 | `config/config.go:146` | `restrictedFields []string` — F14 lookup is `slices.Contains` (O(n) over 5 keys). Tolerable but dirty. | Convert to `map[string]struct{}` populated by `SetDefaultFields`. O(1). | F14, NF1 |
| D-9 | `config/config.go:33-35`, `:250` | `init` calls `ResetConfig`, which `make(chan, 10)` and `go ProcessLogEvent()`. `ResetConfig` is exported and callable at runtime → would re-create channel under live callers, leaking goroutines and dropping inflight events. | Split: `init` does one-shot setup via `sync.Once`; `ResetConfig` becomes test-only helper or removed. | NF5, NF7, NF8 |
| D-10 | `config/config.go:96` | `SetEncoderType` swallows factory error and silently falls back to JSON; no log to stderr. | Keep fallback behavior (graceful) but add a doc-comment + return error variant for tests. | F7 |
| D-11 | `pipeline_stage/unset_log_post_processor_hook.go:43-45` | `for len(h.activeBucket) > 0 { time.Sleep(h.rate) }` — `Stop` busy-waits at flush-rate granularity (worst case 1s); racy read on `activeBucket` len without lock. | Use `doneCh chan struct{}`; flush in a loop calling `flushLogMessages` until empty under lock; close `doneCh`; `Stop` waits on `doneCh`. | F30, SC5 |
| D-12 | `pipeline_stage/unset_log_post_processor_hook.go:81-93` | `PublishLogMessage` does `Unlock`+`flushLogMessages`+`Lock` mid-method — non-atomic; second goroutine can append between, exceeding `maxBucketSize`. | Refactor: while holding lock, if at cap, swap bucket inline (no `Unlock`); spawn flush goroutine after `Unlock` defer. | F24, NF9 |
| D-13 | `entry/entry.go:62` | `config.EventPreProcessors == nil` check inside `Log`; nil-check costs nothing but mixed with level gate. Combined with D-9: at runtime if `ResetConfig` re-runs after preprocessors were set, processors will be wiped. | After D-9 fix this race goes away; keep nil-check after level gate so filtered-path stays cheap. | F6, NF8 |
| D-14 | `go.mod:3` | `go 1.18` — PRD NF11 mandates 1.21. | Bump to `go 1.21`; `go mod tidy`. | NF11 |
| D-15 | `go.mod:38-40` | `gin`, `zerolog` are runtime deps of the LIBRARY module. Should belong only to the demo. | Relocate `logger.go` to `examples/`; either give `examples/` its own `go.mod` or use `// +build ignore`. Library `go.mod` stdlib + `testify` only (test). | NF12 |
| D-16 | `entry/entry.go:91` | `strings.Join(data, ", ")` — separator is `, ` not the field separator JSON expects (between key:value pairs). After encoder wraps with `{}` you get `{"a": "1", "b": "2"}` which IS valid JSON. But this is fragile: any field rendered with embedded comma breaks downstream parsers because escaping is broken (D-5). | After D-5 fix, separator is comma between AppendField calls written directly into pooled buf. | F10 |
| D-17 | `encoder/json_encoder.go:9-12` | `json.NewEncoder(nil)` — passes `nil` to `json.NewEncoder`; the encoder is held but never used. Dead field, will panic if invoked. | Remove field; `JSONEncoder` becomes `struct{}`. | F7 |
| D-18 | `entry/entry.go:53-55` | `reset()` only clears `caller`; will leak any future fields (buf, level, err) added by F17/F27. | Make `reset()` exhaustive; keep `TestLogEntryResetExhaustive` to enforce. | F26 |
| D-19 | `entry/entry.go:82-84` | Loop writes ctxData using `i+x` where `i` was last user-field index after a `+= 2` block — index arithmetic is brittle, will silently overwrite or panic if `len(ctxData)==0` and `len(fields)==0`. (Today `ctxData` is `nil` when ctx is nil, masking the bug.) | After D-6 (drop intermediate slice), this whole indexing scheme is gone. | F27 |
| D-20 | `pipeline_stage/pre_processing_stage.go:58-61` | `publish` iterates Go map → unspecified order. Acceptable but undocumented; tests must not assume order. | Documented in §6.1; add comment in code; tests use set-equality not ordered-equality. | F19 |

---

## 13. Open trade-offs / decisions

| # | Decision | Rationale |
|---|---|---|
| ARCH-1 | Singleton `defaultConfig` retained; no per-instance logger in v1 | PRD N6; per-instance pointer chasing breaks < 1 µs |
| ARCH-2 | Encoder interface signature changes from `Write(string)` to `Append(dst, body []byte) []byte` | NF1 demands zero per-call alloc through encoder; `Write(string)` forces a `string([]byte)` copy |
| ARCH-3 | `runtime.Caller` over `runtime.Callers + CallersFrames` for F17 | PRD R4: single frame is ~250 ns; multi-frame iterator is 500 ns+ |
| ARCH-4 | `addSource` default flips to `false` | NF1 budget; opt-in cost; matches PRD R4 |
| ARCH-5 | New top-level `lognugget` package houses default post-processor handle + `Shutdown()` | F31; gives zero-config path a Stop handle without exporting from `pipeline_stage` |
| ARCH-6 | Pre-render default-key prefixes (`"\"time\":"` etc.) at `SetDefaultFields` time | Saves ~80 ns / event over per-call assembly |
| ARCH-7 | LogEvent.Data is a copy of pooled buf, not a reslice | Decouples buf lifetime from channel; alternative would force buf-lifetime to span dispatcher (worse) |
| ARCH-8 | F27 pool of `[]byte`/`[]string` pulled into M2 if NF3 is in jeopardy after F10 lands | PRD §7.2 milestone gate |
| ARCH-9 | sync.Pool over per-P slab pool | sync.Pool is P-local + GC-aware; slab pattern only adopted if R3 parallel-bench regresses > 10% |
| ARCH-10 | Channel-send remains blocking (F23, NF5); no v1 drop policy | Loss observability is v2; explicit caller-visible blocking is the better v1 default |
| ARCH-11 | Hook iteration order is undefined (Go map) | Acceptable; documented; consumers must not rely on order |
| ARCH-12 | ctx parser map allocation is consumer's concern | F16 contract returns `map[string]any`; v2 may add `func(ctx, dst []model.LogAttr) []model.LogAttr` shape to skip the map |
| ARCH-13 | Errors during `output.Write` silently dropped | NF10; matches existing behavior; v2 may surface |
| ARCH-14 | Newline appended inside encoder `Append`, not by post-processor | Centralises framing; allows text encoder to emit `\n` once; SC1 expects trailing `\n` |
| ARCH-15 | `ResetConfig` becomes test-only (D-9 fix) | Public exported `ResetConfig` is a foot-gun under live traffic |

---

## 14. Diagram index

| File | Caption |
|---|---|
| `diagrams/hot-path-flow.svg` | Caller-goroutine path: pool Get → reset → gate → time → caller → render → encode → channel send → Put. ns budget per stage. |
| `diagrams/pipeline-overview.svg` | Async pipeline: caller goroutines → buffered channel → dispatcher → pre-processor → user hooks + default `unsetLogEventPostProcessor` → io.Writer. |
| `diagrams/lifecycle.svg` | init via package init + sync.Once → steady-state → `Stop()` drain sequence. |
| `diagrams/pool-state.svg` | sync.Pool entry lifecycle: Get → reset → Log → Put; risk panel for false-sharing, pool churn, GC clear. |

---

## Pattern catalog (one-liner per pattern used)

| Pattern | Site | Why |
|---|---|---|
| Object Pool | `entryPool`, `fieldSlicePool`, `ctxMapBufPool` | amortize allocation under > 100k events/sec |
| Singleton | `config.defaultConfig`, `pipelineStage.EventPreProcessorObj` | one logger per process (N6); zero pointer-chase on hot path |
| Strategy | `encoder.Encoder` (JSON / Text) | swap encoding at startup without conditionals on hot path |
| Factory Method | `encoder.DefaultEncoderFactory` | resolve `enum.LogEncodeType` to concrete encoder |
| Observer | `eventPreProcessorObserver` fan-out to N hooks | decouple emission from N destinations |
| Producer-Consumer | `ch chan LogEvent` + dispatcher goroutine | non-blocking IO on caller |
| Double-buffered swap | `unsetLogEventPostProcessor.activeBucket` flush | flush without holding lock during IO |
| Chain of Responsibility | hook chain at `LevelUnSet` then per-level | each hook handles independently |
| Builder | `*LogEntry` accumulates bytes via `AppendField` | construct payload incrementally |
| Facade | `lognugget.Shutdown()` over `unsetLogEventPostProcessor.Stop` | hide pipeline detail from zero-config consumers |
| Guard | min-level gate at top of `Log` | < 100 ns filtered-path |
| Null Object | `TextEncoder` passthrough | identity strategy with no branching cost |
| Value Object | `model.LogAttr`, `enum.LogLevel` | immutable carriers |

---

## F-ID → section trace (story-cuttable index)

| F-ID | Section(s) |
|---|---|
| F1 | §2 |
| F2 | §4.1, §10 |
| F3, F4 | §4.1, §8 |
| F5 | §4.5 |
| F6 | §3.2, §4.1 |
| F7, F8, F9 | §4.4, §7.1–§7.5 |
| F10 | §3.1 step 4, §4.4, §7.3, §11.1, §12 D-2/D-5 |
| F11 | out of scope (PRD COULD) |
| F12 | §3, §6.5 |
| F13 | §4.2, §6.5 |
| F14 | §4.2, §11.1, §12 D-8 |
| F15, F16 | §4.2, §3 step 7, §11.1 |
| F17 | §3 step 5, §3.1, §11.1, §12 D-1 |
| F18, F19, F20 | §4.3, §6.1–§6.3 |
| F21, F22, F23 | §4.2, §5.2, §10 |
| F24 | §6.4, §11.1 |
| F25, F26, F27 | §4.1, §8 |
| F28 | §3 step 4, §4.5, §12 D-4 |
| F29 | §4.2 |
| F30 | §6.4, §9.2, §12 D-11 |
| F31 | §4.5, §9.1, §9.2 |

NF traces: NF1/NF3 → §3, §11.3; NF5/NF7/NF8/NF9 → §5; NF10 → §10; NF11/NF12 → §12 D-14/D-15.
