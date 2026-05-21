# High-Level Design: Epic V2 — Performance: Close the 31× Throughput Gap vs zerolog

| Field | Value |
|---|---|
| Document type | High-Level Design |
| Feature slug | `v2-performance` |
| Umbrella issue | [#81](https://github.com/architagr/LogNugget/issues/81) |
| Status | READY FOR EM REVIEW |
| Author | Architect |
| Date | 2026-05-21 |
| PRD | `docs/prds/v2-performance.md` |
| Target branch | `feat/81-v2-performance` → `develop` |

---

## 1. Context and Problem Statement

LogNugget v1.0.0 is an embedded Go structured logger with an async dispatch pipeline. It beats logrus on all metrics and beats zerolog on the filtered (level-gated) path. On the parallel hot path — the path that actually encodes and publishes a log event — it runs at ~3,440 ns/op with 55 allocs/op, versus zerolog at ~110 ns/op with 0 allocs. That is a 31× gap in throughput and a 3.4× breach of the project-wide < 1 µs p99 SLO declared in `CLAUDE.md`.

The gap is exclusively caller-side. LogNugget's async architecture means IO flush cost falls on a background goroutine, not the caller. The 31× gap is therefore pure encoding and synchronisation overhead paid before the byte lands in the channel. Profiling (bench data in `docs/STATUS.md` lines 201–243) identifies five independent root causes, each traceable to a specific code location.

This HLD describes the architectural approach to eliminating those five root causes across five stories (P1–P5). No pipeline architecture changes, no new dependencies, no public API breakage (with one narrow, compile-time-detectable interface extension in P4).

---

## 2. Goals and Non-Goals

### Goals

- G1. Bring `BenchmarkLognugget_Parallel_10CtxFields` to <= 1,000 ns/op (within < 1 µs SLO) and >= 3 M ops/sec throughput.
- G2. Reduce allocs/op on the parallel 10-field path from 55 to <= 5.
- G3. Preserve the filtered-path guarantee: <= 35 ns/op, 0 allocs — the best-in-class result must not regress.
- G4. Maintain full backward compatibility for all exported APIs except the narrow P4 `encoder.Encoder` interface extension.
- G5. Deliver all changes under the existing Go stdlib stack; no new module dependencies.

### Non-Goals

- Pipeline architecture changes (channel shape, background goroutine, flush loop).
- New encoder formats.
- Lock-free ring buffer as a channel replacement (future epic).
- New log levels, `Fatal`/`Panic` semantic changes, OTel SDK integration.
- Bypassing or raising the 1,000 ns/op bench-check gate threshold.

---

## 3. Assumptions

- A1. The v1.0.0 benchmark baseline (`bench-baseline.txt`, ~3,440 ns/op) remains the comparison reference until the Project Lead explicitly updates it after all five stories merge.
- A2. `sync/atomic` sequential-consistency is sufficient for `minLevel` reads. Level changes are startup-time configuration events; callers observing a stale level for one log call during a live `SetMinLevel` is an acceptable, documented behaviour (consistent with zerolog).
- A3. `model.LogAttr` will not be removed or made internal. Backward compat via `KindAny` is a hard constraint.
- A4. The `ContextFieldsParser` public type (`func(context.Context) map[string]any`) cannot change signature. Any P3 solution wraps or coexists with it.
- A5. The existing `sync.Pool` pool strategy in `entry/pool.go` (1 KB pre-grown `buf`, reset to `len=0` on each cycle) is preserved. P4 changes ownership semantics on `e.buf` but not pool construction.
- A6. `./scripts/bench-check.sh` is the single source of truth for performance gates. No story may weaken the gate.

---

## 4. System Context

LogNugget is an embedded library — it runs in the same process as the calling service. There is no network boundary, no sidecar, no agent. The hot path is synchronous from the caller's perspective up to the channel send; all IO is async.

```
Caller goroutine (HTTP handler / gRPC interceptor / event loop)
  │
  ├─ NewLogEntry()        ← sync.Pool get
  ├─ entry.Info(ctx, msg, fields...)
  │     │
  │     ├─ level gate     ← atomic.Int32 load (P1)
  │     ├─ config snapshot ← single configMu.RLock (P1)
  │     ├─ append fields  ← typed methods, no boxing (P2)
  │     ├─ append ctx     ← direct buf append, no map (P3)
  │     ├─ frame buf      ← OpenBytes/CloseBytes inline (P4)
  │     └─ ch <- event    ← non-blocking at cap=1000 (P5)
  └─ e.Put()              ← sync.Pool put, fresh backing slice (P4)

Background goroutine (ProcessLogEvent)
  └─ drains ch → PreProcessors → output writer
```

---

## 5. Major Components

Five components are modified. No new packages are created. All changes are internal to the existing package boundaries.

### 5.1 Component A — `config` package: atomic minLevel + config snapshot (P1)

**Current state:** `minLevel` lives in `*Config`; reads go through `GetConfig()` which acquires `configMu.RLock()`. `logWithSkip` calls `GetConfig()` (and therefore `RLock/RUnlock`) six times: at the level gate (line 117), at the `AddSource` check (line 129), and four more times to extract config fields (lines 142–144, 250).

**Proposed change:** Introduce a package-level `atomic.Int32` variable `atomicMinLevel` in `config.go`. This variable is written under `configMu.Lock()` in `SetMinLevel` (keeping the write serialised) and read without any lock in the hot path. The six `GetConfig()` calls in `logWithSkip` are replaced by a single `configMu.RLock / snapshot / configMu.RUnlock` at the top of the function body (after the atomic gate passes). The snapshot is a plain local `cfg := defaultConfig` pointer copy — cheap, not a deep clone.

**Why not atomic.Pointer[Config]:** Copy-on-write with `atomic.Pointer` would require allocating a new `*Config` on every `Set*` call. The snapshot pattern (take one `RLock`, copy the pointer, release) achieves the same single-read guarantee without any allocation on the write path. The `RWMutex` is already present; we are reducing its hold duration and acquisition count, not replacing it.

**Affected files:** `config/config.go`, `entry/entry.go`.

### 5.2 Component B — `model` and `entry` packages: typed field union API (P2)

**Current state:** `model.LogAttr.Value` is `any` (alias `LogAttrValue`). In `logWithSkip`, each field goes through `config.AppendField(e.buf, key, field.Value)` which performs a `reflect`-free type-switch. The boxing of the concrete value into `interface{}` at the call site (`model.LogAttr{Key: "k", Value: 42}`) is the source of ~30 allocs/op.

**Proposed change — union struct on `model.LogAttr`:**

Extend `model.LogAttr` with a `Kind` discriminant (`uint8` or typed const) and four typed value fields (`strVal string`, `intVal int64`, `boolVal bool`, `floatVal float64`). The existing `Value any` field is retained and maps to `KindAny`. Zero value of `Kind` is `KindAny` so all existing struct-literal call sites (`model.LogAttr{Key: "k", Value: v}`) continue to work without change — `Kind` defaults to 0 (`KindAny`), the `AppendField` type-switch path fires as before.

**New methods on `LogEntry`:**

Add `Str`, `Int`, `Bool`, `Float64` as chainable methods directly on `*LogEntry`. Each method constructs a `model.LogAttr` with the appropriate `Kind` and typed value field, then appends the pre-rendered key and the unboxed value directly to `e.buf` without touching `interface{}`. The field-level loop in `logWithSkip` is updated to branch on `Kind`: typed kinds go to a direct append path; `KindAny` falls through to the existing `config.AppendField` type-switch.

**Call-site shape comparison:**

```
// legacy — still works, no source change needed
entry.Info(ctx, "req", model.LogAttr{Key: "status", Value: 200})

// new typed path — zero allocs on append
entry.Info(ctx, "req")  // then chain:
e.Int("status", 200).Str("path", "/api/v1").Info(ctx, "req")
```

Method chaining is enabled by returning `*LogEntry` from each typed method.

**Why direct methods on LogEntry vs a Fields builder:** A separate `Fields` builder would require passing the builder to `logWithSkip`, either as a variadic argument (new slice allocation) or as a field on `LogEntry` (struct size growth). Direct methods write immediately to `e.buf`, which is already allocated and in-scope — no intermediate allocation.

**Affected files:** `model/attr.go`, `entry/entry.go`.

### 5.3 Component C — `config` and `entry` packages: append-to-buf context API (P3)

**Current state:** `setLogContextFields` (entry.go:249–258) calls `GetConfig().ContextParser()` (one more `RLock`), then calls the parser to get `map[string]any`, then iterates the map building a `[]string` of rendered key-value fragments, then returns the slice. The `logWithSkip` loop (lines 183–186) then appends each string to `e.buf`. Cost: one `map[string]any` allocation (GC-managed), one `[]string` allocation, and O(n) interface-boxing inside `ValidateandParseLogField`. Combined: ~600 ns, ~23 allocs.

**Proposed change — dual-registration model:**

Add a new exported type and registration function to `config`:

```
ContextFieldsAppender = func(ctx context.Context, buf []byte) []byte
SetContextFieldsAppender(appender ContextFieldsAppender)
```

Store `contextAppender` as a field on `Config` alongside `contextParser`. In `setLogContextFields` (renamed or restructured), check for `contextAppender` first; if present, call it and return the extended `e.buf` directly — no intermediate slice. Only if no appender is registered does the old `ContextFieldsParser` path fire.

The `contextAppender` field is read in the config snapshot taken in P1; no additional lock is needed.

**Priority rule:** Appender wins. If both are registered, the appender is called and the legacy parser is ignored. This is documented in godoc.

**Why not a pure internal adapter:** Wrapping `ContextFieldsParser` output (`map[string]any`) into a byte-append adapter internally would still allocate the map on every call — the adapter receives the map from the caller's function and cannot avoid it. The dual-registration approach lets the caller skip the map entirely.

**Why not deprecate `ContextFieldsParser` immediately:** The type is part of the public API. Removing or ignoring it without a major version bump would violate backward compatibility. The dual-registration model is the minimal, non-breaking migration path.

**Affected files:** `config/config.go`, `entry/entry.go`.

### 5.4 Component D — `encoder` and `entry` packages: inline framing, ownership transfer (P4)

**Current state:** `logWithSkip` accumulates the log body (fields without braces) into `e.buf`, then calls `en.Append(nil, e.buf)` which allocates a new `[]byte` to wrap the body in `{…}\n` (`byteData`). Then `config.PublishLog` makes a second `append([]byte(nil), Data...)` defensive copy before sending to the channel. Total: 2 allocs/event.

**Proposed change — OpenBytes/CloseBytes + ownership transfer:**

Extend `encoder.Encoder` with two new methods:

```
OpenBytes() []byte   // JSONEncoder: []byte("{")   TextEncoder: nil
CloseBytes() []byte  // JSONEncoder: []byte("}\n")  TextEncoder: []byte("\n")
```

In `logWithSkip`, after the config snapshot is taken (P1), call `cfg.Encoder()` once to get `en`, then:
1. Prepend `en.OpenBytes()` to `e.buf` before writing any fields.
2. Append fields and context as normal.
3. Append `en.CloseBytes()` after the last field.
4. Pass `e.buf` directly to a revised `PublishLog`.

`PublishLog` is changed to send `e.buf` directly without a second copy. The ownership contract changes: `e.buf` is transferred to the `LogEvent` sent on the channel. Before calling `e.Put()`, `logWithSkip` assigns a fresh backing slice to `e.buf` (`e.buf = make([]byte, 0, initBufCap)`) so the pool slot does not share the backing array with the in-flight channel event.

**Why OpenBytes/CloseBytes instead of removing Append entirely:** `Append` is already part of the public interface. Removing it breaks external implementations. Keeping it but not calling it on the hot path preserves backward compatibility while eliminating the allocation. External encoders that do not yet implement `OpenBytes`/`CloseBytes` will get a compile-time error — an explicit, detectable break preferable to a silent runtime divergence.

**Why prepend OpenBytes before encoding vs append after:** JSON framing requires `{` to precede the body content. Prepending at the start (before the time field is written) is a single `append(e.buf, '{')` — one byte, zero allocation if `e.buf` has slack capacity (which the 1 KB pre-growth ensures). Alternatively, framing can be a separate `open` call before encoding starts. The chosen approach avoids any reordering of `e.buf` contents.

**Ownership transfer correctness:** The Go `chan<-` send copies the `LogEvent` struct (which contains a slice header). The slice header copy is shallow — both the channel value and `e.buf` point to the same backing array until `e.buf` is reassigned. The reassignment (`e.buf = make(...)`) before `e.Put()` severs that alias. The background `ProcessLogEvent` goroutine reads `LogEvent.Data` (the old backing array) safely because the sender has already relinquished its reference. This is not a data race because the channel send happens-before the receive.

**Affected files:** `encoder/encoder.go`, `encoder/json_encoder.go`, `encoder/text_encoder.go`, `entry/entry.go`, `config/config.go`.

### 5.5 Component E — `config` package: channel capacity (P5)

**Current state:** `resetConfig` hardcodes `make(chan LogEvent, 10)`. Under 8-goroutine parallel load, the channel fills in < 1 µs and all callers block on `ch <-`, dominating wall time.

**Proposed change — configurable capacity, init-time only:**

Add a package-level variable `channelCapacity int` (not a field on `Config`) initialised to `1000`. Add `SetChannelCapacity(n int)` which validates and stores into this variable. `resetConfig` reads `channelCapacity` when constructing the channel: `make(chan LogEvent, channelCapacity)`. Change `DafaultLogBuffer` from `20` to `1000`.

The variable is package-level (not on `Config`) for the same reason `restrictedFieldsSet` is package-level: it is consumed by `resetConfig` at `init()` time, before any `*Config` pointer exists. Placing it on `Config` would require `resetConfig` to read a field from an object it is in the process of constructing.

**Guard against post-init calls:** `SetChannelCapacity` takes effect only at the next `resetConfig` call. In production, `resetConfig` is called exactly once (from `init()`). The godoc for `SetChannelCapacity` documents this: "Must be called before any log call. Has no effect if the package has already been initialised." This is consistent with the existing `DafaultLevel`, `DafaultEncoderType`, and `DefaultAddSource` package-level var pattern.

**Validation:** Values <= 0 are clamped to 1000; values > 100,000 are clamped to 100,000. A `log.Printf` warning is emitted on clamp. No panic — panic in a logging library is unacceptable.

**Affected files:** `config/config.go`.

---

## 6. Data Flow

### Hot path after all five stories

```
logWithSkip(level, ctx, msg, err, skip, fields...)
  │
  ├── 1. atomic.Int32.Load(atomicMinLevel)     [0 allocs, 0 locks]
  │      ── level < min → e.Put(); return
  │
  ├── 2. configMu.RLock()                      [1 RLock total]
  │      cfg := defaultConfig                  [pointer copy]
  │      configMu.RUnlock()
  │
  ├── 3. optional: runtime.Caller(skip)        [only if cfg.addSource]
  │
  ├── 4. e.buf = append(e.buf, cfg.encoder.OpenBytes()...)
  │      [prepend "{" for JSON; no-op for text]
  │
  ├── 5. append time, level, message           [pre-rendered key prefixes]
  │
  ├── 6. for each field in fields:
  │      ├── Kind == KindStr/Int/Bool/Float64 → direct typed append [0 allocs]
  │      └── Kind == KindAny                  → AppendField type-switch [legacy]
  │
  ├── 7. if cfg.contextAppender != nil:
  │      e.buf = cfg.contextAppender(ctx, e.buf)  [0 allocs, caller-controlled]
  │   else if cfg.contextParser != nil:
  │      legacy map[string]any path              [allocates, legacy compat]
  │
  ├── 8. optional: error field, caller field, static fields
  │
  ├── 9. e.buf = append(e.buf, cfg.encoder.CloseBytes()...)
  │      [append "}\n" for JSON; "\n" for text]
  │
  ├── 10. ch <- LogEvent{Level: level, Data: e.buf}   [non-blocking at cap=1000]
  │       (ownership of e.buf transferred to channel)
  │
  ├── 11. e.buf = make([]byte, 0, initBufCap)          [fresh backing slice]
  │
  └── 12. e.Put()                                       [return to pool]

Background (ProcessLogEvent):
  for e := range ch:
    for each PreProcessor: observer.PreProcess(e.Level, e.Data)
    → output writer
```

### Filtered path (unchanged)

```
logWithSkip(level below minLevel, ...)
  │
  ├── atomic.Int32.Load(atomicMinLevel)   [0 allocs, 0 locks]
  │      level < min → e.Put(); return
  │
  └── total: ~35 ns, 0 allocs
```

---

## 7. APIs and Integration Points

### 7.1 New public APIs

| Symbol | Package | Type | Story |
|---|---|---|---|
| `atomicMinLevel` | `config` | `atomic.Int32` (unexported) | P1 |
| `SetContextFieldsAppender(ContextFieldsAppender)` | `config` | Function | P3 |
| `ContextFieldsAppender` | `config` | Type alias `func(context.Context, []byte) []byte` | P3 |
| `OpenBytes() []byte` | `encoder.Encoder` | Interface method | P4 |
| `CloseBytes() []byte` | `encoder.Encoder` | Interface method | P4 |
| `SetChannelCapacity(n int)` | `config` | Function | P5 |
| `Str(key, val string) *LogEntry` | `entry.LogEntry` | Method | P2 |
| `Int(key string, val int64) *LogEntry` | `entry.LogEntry` | Method | P2 |
| `Bool(key string, val bool) *LogEntry` | `entry.LogEntry` | Method | P2 |
| `Float64(key string, val float64) *LogEntry` | `entry.LogEntry` | Method | P2 |

### 7.2 Changed public APIs (backward compatible)

| Symbol | Change | Backward compat |
|---|---|---|
| `model.LogAttr` | New fields `Kind`, `strVal`, `intVal`, `boolVal`, `floatVal` added | Struct literal `{Key:..., Value:...}` still compiles; `Kind` zero-value = `KindAny` |
| `config.DafaultLogBuffer` | Default value raised from `20` to `1000` | Behavioural change; programs relying on buffer=20 for backpressure semantics will see reduced blocking |
| `encoder.Encoder` interface | Two new methods added | Breaking for external implementations — compile-time error, not runtime |

### 7.3 Unchanged public APIs

`SetContextFieldsParser`, `ContextFieldsParser`, `GetConfig`, `PublishLog`, `SetMinLevel`, `SetLogBufferMaxSize`, all `Config` accessor methods, `entry.Log`/`Debug`/`Info`/`Warn`/`Error`/`Fatal`/`Panic`.

---

## 8. Data Model Overview

### 8.1 model.LogAttr (after P2)

```
LogAttr {
  Key      LogAttrKey    // string type alias — unchanged
  Value    LogAttrValue  // any — retained for KindAny backward compat
  Kind     uint8         // 0=KindAny 1=KindStr 2=KindInt 3=KindBool 4=KindFloat64
  strVal   string        // populated when Kind=KindStr
  intVal   int64         // populated when Kind=KindInt
  boolVal  bool          // populated when Kind=KindBool
  floatVal float64       // populated when Kind=KindFloat64
}
```

Zero value: `Kind=0` (`KindAny`), all typed fields zero. Existing struct literals produce a zero `Kind`, which maps to the existing `AppendField` path — no observable behaviour change.

Struct size increase: 8 fields vs current 2 fields. On a 64-bit system: `Key` (16 B, string header) + `Value` (16 B, interface) + `Kind` (1 B + 7 B padding) + `strVal` (16 B) + `intVal` (8 B) + `boolVal` (1 B + 7 B padding) + `floatVal` (8 B) = ~80 B per `LogAttr`, up from ~32 B. The variadic `fields ...model.LogAttr` slice is stack-allocated when <= 2 fields (Go escape analysis); for more fields the slice is heap-allocated regardless of struct size. The alloc budget impact is negligible relative to the 30 allocs saved.

### 8.2 Config (after P1 + P3)

`Config` gains one new field: `contextAppender ContextFieldsAppender`. All other fields are unchanged. The `atomicMinLevel` lives as a package-level `atomic.Int32`, not on `Config`, to avoid requiring the hot path to dereference `*Config` before the gate check.

### 8.3 LogEvent (unchanged)

```
LogEvent {
  Level enum.LogLevel
  Data  []byte          // after P4: owned by sender; no second copy in PublishLog
}
```

---

## 9. Scalability Expectations

LogNugget is an embedded library; scale is measured in concurrent goroutines sharing a single `entryPool` and channel.

| Dimension | Before Epic V2 | After Epic V2 |
|---|---|---|
| Parallel throughput (8 goroutines) | ~290 K ops/sec | >= 3 M ops/sec |
| Channel fill-up point | 10 events (~3 µs at 3,440 ns/op) | 1,000 events (~333 µs at 333 ns/op) |
| Blocking callers under burst | > 8 goroutines almost always blocked | > 1,000 concurrent events in-flight before any blocking |
| Pool pressure | High (55 allocs/op increase GC pressure) | Low (<=5 allocs/op, mostly stack) |

The 3 M ops/sec target is an intermediate milestone. The zerolog ceiling (~9 M ops/sec) is achievable with a subsequent lock-free ring buffer (future epic) that removes the final channel-send cost.

---

## 10. Reliability and Failure Modes

### 10.1 Level change visibility (P1 TOCTOU)

After P1, `SetMinLevel` writes `atomicMinLevel` under `configMu.Lock()` and reads from the hot path use `atomic.Load` with no lock. A goroutine that reads the old level value after a `SetMinLevel` call will log one event it should have suppressed, or suppress one event it should have logged. This is a single-event eventual-consistency window, lasting for at most one goroutine scheduling quantum. This is the same behaviour as zerolog and is acceptable for a logging library. Documented in godoc.

### 10.2 Channel backpressure (P5)

At capacity=1000, a burst of > 1,000 concurrent events still blocks callers. The library makes no lossless guarantee — this is by design (see PRD Non-Goal N4). Services with sustained throughput > 1,000 log events/ms should use `SetChannelCapacity` to tune upward, or defer to the lock-free ring buffer epic.

### 10.3 Buffer ownership race (P4)

The ownership transfer (`e.buf` transferred to channel, then reassigned before `e.Put()`) is safe if and only if:
1. The reassignment happens before `e.Put()`.
2. The pool's `reset()` method is updated to not use `e.buf[:0]` after the reassignment.

Specifically, `reset()` currently does `retained := e.buf[:0]; *e = LogEntry{}; e.buf = retained`. After P4, `logWithSkip` has already replaced `e.buf` with a new backing slice before calling `e.Put()`. `reset()` will therefore retain the new empty slice — correct. The race detector (`go test -race ./...`) is the mandatory acceptance check.

### 10.4 Encoder interface extension (P4)

Third-party `encoder.Encoder` implementations will fail to compile after P4. This is a detectable, build-time break — not a silent runtime failure. The feature branch PR description must list this as a known break.

---

## 11. Security and Compliance Considerations

- No new network surfaces, no new file I/O, no new goroutines beyond the existing `ProcessLogEvent` background goroutine.
- No external dependencies added; attack surface is unchanged.
- `SetChannelCapacity(n)` clamps to 100,000 to prevent OOM from a misconfigured capacity. The clamp is the only user-input guard added.
- No personally identifiable data is logged by the library itself; context field content is caller-controlled and unchanged by this epic.

---

## 12. Observability

- The bench-check gate (`./scripts/bench-check.sh`) is the primary observability instrument for this epic. It runs on every PR and reports ns/op and allocs/op per benchmark.
- Each story ships a `*_bench_test.go` file. The benchmark coverage plan is described in section 16.
- The `go test -race ./...` output is the race-safety instrument.
- No OTel metrics are added in this epic (OTel integration is Non-Goal N5). Future: a `config.Hooks` mechanism already exists and could be used to emit `log.published_events` and `log.channel_depth` metrics in a follow-on epic.

---

## 13. Deployment and Environments

LogNugget is a library, not a service. There is no deployment in the traditional sense. The library is imported by downstream Go modules.

- Development: work on `feat/81-v2-performance` sub-branches; bench gate verified locally before each PR.
- CI: bench gate runs on every PR to the feature branch. Merging to `develop` is blocked on a green gate.
- Release: feature branch merges to `develop` via squash merge after all five stories are in and the bench gate is green. The baseline update commit (`bench-baseline.txt`) lands on the feature branch before the PR is marked ready for merge.

---

## 14. Key Architectural Decisions

### KD-1: Atomic package-level var for minLevel, not atomic.Pointer[Config]

**Decision:** Package-level `atomic.Int32` for `minLevel` only; all other config fields remain on `*Config` guarded by `configMu`.

**Tradeoff:** Copy-on-write (`atomic.Pointer[Config]`) would eliminate all RLock acquisitions but requires allocating a new `*Config` on every `Set*` call. For a library used in throughput-sensitive services, allocation on the write path (startup configuration) is acceptable; allocation on the read path (every log call) is not. The hybrid approach — atomic for the hot-path gate, one-shot RLock snapshot for the full config — gives the best of both worlds at the cost of slightly more complex `SetMinLevel`.

### KD-2: Union struct on LogAttr, not a separate typed-field sum type

**Decision:** Extend `model.LogAttr` with a `Kind` discriminant and typed value fields rather than introducing a new type.

**Tradeoff:** A new `model.TypedField` type would have a cleaner API but would require callers to change all call sites to use the new type. The union struct approach threads the needle: zero call-site changes for `KindAny`, zero boxing for the four typed kinds.

### KD-3: OpenBytes/CloseBytes over removing Append from encoder.Encoder

**Decision:** Add two new methods; keep `Append`.

**Tradeoff:** Removing `Append` would be a cleaner interface but breaks external implementations silently at runtime (panic or wrong output). Adding two methods produces a compile-time error for external implementations — explicit, fixable, and detectable in CI.

### KD-4: Dual-registration for context fields, not internal adapter

**Decision:** New `ContextFieldsAppender` registration coexists with `ContextFieldsParser`; appender wins when both are registered.

**Tradeoff:** An internal adapter (`func(ctx context.Context, buf []byte) []byte { for k,v := range parser(ctx) { ... } }`) would be transparent to callers but still allocates the `map[string]any` inside the parser call. The dual-registration approach requires callers to opt in, but the zero-alloc path is then truly zero-alloc.

### KD-5: Init-time-only channel capacity, no runtime resize

**Decision:** `SetChannelCapacity` stores a package-level var consumed by `resetConfig`; no drain-and-replace at runtime.

**Tradeoff:** Runtime resize would allow capacity tuning without restart but requires draining the old channel under lock while new sends could arrive — a complex coordination problem with non-zero event-drop risk. Init-time-only is simple, safe, and sufficient for the intended use case (tuning at program startup).

---

## 15. Alternatives Considered

### Alt-1: sync/atomic.Pointer[Config] for full config snapshot (rejected)

Would eliminate all `configMu.RLock` acquisitions on the hot path. Rejected because `Set*` calls become allocation-heavy (must clone the full `Config` struct), and the current single-snapshot pattern (one RLock per log call) already reduces the cost from 6 acquisitions to 1, which is sufficient to meet the SLO.

### Alt-2: Per-goroutine config cache via goroutine-local storage (rejected)

No stable goroutine-local storage mechanism exists in Go without cgo or assembly. Rejected as incompatible with "no new dependencies" and unsupportable.

### Alt-3: Fields builder pattern (zerolog-style chain from the logger object, not LogEntry) (rejected)

Would require the log entry to accumulate fields in a separate builder before the level call, changing the call-site API more significantly. The PRD mandates backward compat; the typed methods on `LogEntry` are the minimal, non-breaking extension.

### Alt-4: Replace `chan LogEvent` with `sync/atomic` ring buffer (lock-free) (deferred)

Would eliminate the remaining channel-send cost (~10–20 ns at cap=1000). Deferred to a future epic (PRD Non-Goal N4). P5's capacity increase is the 80%-solution that unblocks the SLO without the complexity of a custom ring buffer.

### Alt-5: Precompute encoder framing bytes once at config init (rejected for P4)

`OpenBytes`/`CloseBytes` return constant byte slices (`{`, `}\n`). Caching them in `logWithSkip` as local variables after the config snapshot is retrieved (one call per log line) is equivalent and simpler. No additional caching layer is needed.

---

## 16. Risks and Mitigations

See PRD Section 10 for the full risk table. The two highest-severity risks from an architectural standpoint:

**Risk: P4 buffer ownership race**
The `e.buf` slice header is shared between the `LogEntry` pool and the `LogEvent` channel between the `ch <-` send and the `e.buf = make(...)` reassignment. This window is within a single goroutine with no concurrent access — the pool cannot recycle `e` before `e.Put()` is called. The reassignment severs the alias before `e.Put()`. Mitigation: mandatory `-race` pass on the benchmark; explicit audit of `reset()` in `entry/pool.go`.

**Risk: Gate fails mid-epic (not cleared until all five stories merge)**
P1 alone saves ~150–250 ns; the gate threshold is 1,000 ns. The gate is not expected to clear until P1+P3 are both on the feature branch (~750–850 ns combined saving). Individual story PRs are not required to pass the gate against the current baseline — only the feature branch as a whole must pass before the PR to `develop` is marked ready. This should be documented in each story PR description.

---

## 17. Rollout Plan

```
Phase 1 (P1 only): atomic minLevel + single snapshot
  Branch: feat/81-v2-performance/p1-atomic-minlevel
  Gate: go test -race ./...; filtered-path bench <= 35 ns/op
  Merges to: feat/81-v2-performance

Phase 2 (P2 + P3 + P4 in parallel, after P1 merges):
  Branch per story (p2, p3, p4 sub-branches off feat/81-v2-performance)
  Gate per PR: go test -race ./...; golangci-lint run
  Note: full bench gate not required per-story PR; evaluated at feature branch
  Merges to: feat/81-v2-performance (each story PR separately)

Phase 3 (P5, after P2+P3+P4 merge):
  Branch: feat/81-v2-performance/p5-channel-capacity
  Gate: BenchmarkLognugget_Parallel_10CtxFields >= 3 M ops/sec; bench-check.sh exits 0
  Merges to: feat/81-v2-performance

Phase 4 (baseline update):
  Project Lead runs: ./scripts/bench-check.sh --update-baseline
  Commits bench-baseline.txt to feat/81-v2-performance
  Refs: #81

Phase 5 (merge to develop):
  feat/81-v2-performance PR to develop
  All checks green; umbrella issue #81 closed
```

---

## 18. Open Questions

All five open questions from the wiki have been resolved in PRD Section 3. The following architectural sub-questions remain for LLD elaboration:

| ID | Question | Assigned to |
|---|---|---|
| AQ-1 | Should `Kind` on `model.LogAttr` be a `uint8` or a named type (`type AttrKind uint8`)? Named type prevents accidental arithmetic on kind values. Recommendation: named type. | LLD — P2 |
| AQ-2 | Should the typed value fields on `LogAttr` be exported (`StrVal`, `IntVal`) or unexported (`strVal`, `intVal`)? Unexported fields prevent external packages from constructing `LogAttr` with inconsistent `Kind`/value combinations. The `Str`/`Int`/`Bool`/`Float64` constructors become the only safe typed-field construction path. Recommendation: unexported. | LLD — P2 |
| AQ-3 | Should `e.buf = make([]byte, 0, initBufCap)` in P4 use `initBufCap` directly or grow to `len(e.buf)` (the just-sent event size) as a warm-start hint? Growing to the previous event size reduces reallocation on the next call but retains memory proportional to the largest event seen. Recommendation: use `initBufCap` for simplicity; revisit if bench shows pool-warm reallocation. | LLD — P4 |
| AQ-4 | `ValidateandParseLogField` (called from the legacy `setLogContextFields` path) acquires a separate `configMu.RLock` to check `restrictedFieldsSet`. After P1, the config snapshot is already held as a local pointer. Should `ValidateandParseLogField` be refactored to accept the restricted set from the snapshot, eliminating this extra lock? Recommendation: yes — pass `restrictedFieldsSet` as a parameter to the internal path in P3. | LLD — P3 |
