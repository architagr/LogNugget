# High-Level Design: Epic V2 — Performance: Close the 31× Throughput Gap vs zerolog

| Field | Value |
|---|---|
| Document type | High-Level Design (Authoritative) |
| Feature slug | `v2-performance` |
| Umbrella issue | [#81](https://github.com/architagr/LogNugget/issues/81) |
| Status | READY FOR EM REVIEW |
| Author | Chief Architect |
| Date | 2026-05-21 |
| PRD | `docs/prds/v2-performance.md` |
| Wiki | `docs/wiki/v2-performance.md` |
| DM draft reference | `docs/epics/v2-performance-hld.md` (superseded by this document) |
| Target branch | `feat/81-v2-performance` → `develop` |

---

## 1. Context and Problem Statement

LogNugget v1.0.0 is an embedded Go structured logger with an async dispatch pipeline. On its best dimension — the filtered (level-gated) path — it runs at ~35 ns/op with 0 allocs and beats zerolog. On its worst dimension — the parallel hot path that actually encodes and publishes a log event under GOMAXPROCS=8 — it runs at ~3,440 ns/op with 55 allocs/op, versus zerolog at ~110 ns/op with 0 allocs/op. That is a 31× throughput gap and a 3.4× breach of the project-wide < 1 µs p99 SLO declared in `CLAUDE.md`.

The gap is **exclusively caller-side**. LogNugget's async architecture means IO flush cost falls on a background goroutine, not the caller. The 31× gap is pure encoding and synchronisation overhead paid synchronously by the calling goroutine before the byte lands on the channel. This is strategically important: under real IO load, zerolog's synchronous write blocks the caller on every event whereas LogNugget returns immediately after the channel send. The SLO target is therefore not zerolog parity but compliance with the < 1 µs gate while retaining the async structural advantage.

Profiling identifies five independent root causes, all traceable to specific lines in the existing codebase:

| Priority | Root cause | Location | Estimated saving |
|---|---|---|---|
| RC-1 (largest) | `map[string]any` context iteration + `[]string` intermediate slice in `setLogContextFields` | `entry/entry.go:249–258` | ~600 ns, ~23 allocs/op |
| RC-2 | `interface{}` boxing in `model.LogAttr.Value` dispatched via type-switch | `model/attr.go`, `entry/entry.go:169–178` | ~30 allocs/op |
| RC-3 | 6× `configMu.RLock` acquisitions per log call via repeated `GetConfig()` calls | `entry/entry.go:117,129,142,143,144,250` | ~150–250 ns |
| RC-4 | Double-buffer: `en.Append(nil, e.buf)` allocates a new slice to frame the body, then `append([]byte(nil), Data...)` in `PublishLog` makes a second defensive copy | `entry/entry.go:208`, `config/config.go:204` | 2 allocs/event |
| RC-5 | Channel cap=10 blocks all goroutines under 8-goroutine load; callers spend majority of wall time blocked on `ch <-` rather than encoding | `config/config.go:456` | unblocks callers |

This HLD describes the architectural approach to eliminating all five root causes across five ordered stories (P1–P5). No pipeline architecture changes, no new dependencies, and no public API breakage beyond one narrow compile-time-detectable interface extension in P4.

---

## 2. Goals and Non-Goals

### Goals

- **G1.** Bring `BenchmarkLognugget_Parallel_10CtxFields` to <= 1,000 ns/op (within the < 1 µs SLO) and >= 3 M ops/sec at GOMAXPROCS=8.
- **G2.** Reduce allocs/op on the parallel 10-field path from 55 to <= 5.
- **G3.** Preserve the filtered-path guarantee: <= 35 ns/op, 0 allocs — the best-in-class result must not regress.
- **G4.** Maintain full backward compatibility for all exported APIs except the narrow P4 `encoder.Encoder` interface extension (which is compile-time-detectable).
- **G5.** Deliver all changes under the existing Go stdlib stack — no new module dependencies.

### Non-Goals

- Pipeline architecture changes (channel shape, background goroutine, flush loop).
- New encoder formats (CBOR, Protobuf, plain-text variants).
- Lock-free ring buffer as a channel replacement (future epic, PRD N4).
- New log levels, `Fatal`/`Panic` semantic changes, OTel SDK integration.
- Bypassing or raising the 1,000 ns/op bench-check gate threshold.
- Runtime channel drain-and-replace (deferred; init-time-only capacity is sufficient for the target use case).

---

## 3. Assumptions

- **A1.** The v1.0.0 benchmark baseline (`bench-baseline.txt`, ~3,440 ns/op) remains the comparison reference until the Project Lead explicitly updates it after all five stories merge to the feature branch.
- **A2.** `sync/atomic` sequential-consistency is sufficient for `minLevel` reads. Level changes are startup-time configuration events. A caller observing a stale level for a single log call during a live `SetMinLevel` call is an acceptable, documented eventual-consistency window — consistent with zerolog's behaviour.
- **A3.** `model.LogAttr` will not be removed or made internal. Backward compatibility via `KindAny` is a hard constraint (PRD A3).
- **A4.** The `ContextFieldsParser` public type (`func(context.Context) map[string]any`) cannot change signature without a major version bump. Any P3 solution coexists with it.
- **A5.** The existing `sync.Pool` pool strategy (`entry/pool.go`; 1 KB pre-grown `buf`, reset to `len=0` on each cycle) is preserved. P4 changes ownership semantics on `e.buf` but not pool construction logic.
- **A6.** `./scripts/bench-check.sh` is the single source of truth for performance gates. No story may weaken or bypass the gate's 1,000 ns/op threshold.
- **A7.** The `encoder.Encoder` interface extension in P4 is an acceptable minor-version break. Internal implementations (`JSONEncoder`, `TextEncoder`) are updated atomically. External consumers receive a compile-time error — detectable, not silent.

---

## 4. System Context

LogNugget is an embedded library — it runs in the same process as the calling service. There is no network boundary, no sidecar, no agent. The hot path is synchronous from the caller's perspective up to the channel send; all IO is async via a background goroutine.

```
Caller goroutine (HTTP handler / gRPC interceptor / event loop)
  │
  ├─ NewLogEntry()              ← sync.Pool get
  ├─ [optional] e.Str/Int/Bool/Float64(...)  ← P2: typed field methods
  ├─ entry.Info(ctx, msg, fields...)
  │     │
  │     ├─ level gate           ← atomic.Int32 load, 0 locks (P1)
  │     ├─ config snapshot      ← 1× configMu.RLock, pointer copy (P1)
  │     ├─ append open frame    ← OpenBytes() direct to e.buf (P4)
  │     ├─ append time/level/msg ← pre-rendered key prefixes (existing)
  │     ├─ append typed fields  ← Kind dispatch, no boxing (P2)
  │     ├─ append ctx fields    ← direct buf append, no map (P3)
  │     ├─ append optional fields (err, caller, static)
  │     ├─ append close frame   ← CloseBytes() direct to e.buf (P4)
  │     └─ ch <- LogEvent       ← non-blocking at cap=1000 (P5)
  ├─ e.buf = make(...)          ← fresh backing slice (P4 ownership transfer)
  └─ e.Put()                    ← sync.Pool put

Background goroutine (ProcessLogEvent)
  └─ for e := range ch:
       for each PreProcessor: observer.PreProcess(e.Level, e.Data)
       → output writer
```

The library occupies five packages relevant to this epic: `config`, `model`, `entry`, `encoder`, and the benchmark-only `entry` test files. No new packages are created.

---

## 5. Major Components

Five components are modified. No new packages are created. All changes are internal to the existing package boundaries.

### Component A — `config` package: Atomic minLevel + single config snapshot (P1)

**Root cause addressed:** RC-3 — 6× `configMu.RLock` acquisitions per log call via repeated `GetConfig()` calls.

**Current state:** `minLevel` lives in `*Config`; reads go through `GetConfig()` which acquires `configMu.RLock()` on every call. `entry.logWithSkip` calls `GetConfig()` six times: at the level gate (line 117), at the `AddSource` check (line 129), and four more times to extract config fields (lines 142–144, 250). Each RLock/RUnlock pair costs ~40–50 ns under contention at GOMAXPROCS=8, accounting for ~150–250 ns of the 3,440 ns/op total.

**Proposed change:**

1. Introduce a package-level `atomic.Int64` variable `atomicMinLevel` in `config.go`. This variable mirrors `Config.minLevel` and is written under `configMu.Lock()` in `SetMinLevel` (writes stay serialised) and read without any lock on the hot path. `atomic.Int64` is used because `enum.LogLevel` is `type LogLevel int` (64 bits); no narrowing cast is required.
2. Replace all six `GetConfig()` calls in `logWithSkip` with a single `configMu.RLock / cfg := defaultConfig / configMu.RUnlock` at the top of the function body — immediately after the atomic gate check passes. The snapshot `cfg` is a plain pointer copy (8 bytes, no deep clone). All subsequent reads within `logWithSkip` use `cfg` directly, eliminating five of the six RLock acquisitions.
3. `GetConfig()` is retained for external callers and test helpers; it is no longer called on the hot path.
4. The `Config.MinLevel()` accessor method retains its `configMu.RLock` for external use but is not called on the hot path after this change.

**Invariants:**
- The filtered path (`logWithSkip` early return) acquires exactly 0 locks.
- The hot path acquires exactly 2 RLocks total (down from 6+): one in `GetHotSnapshot()` (which snapshots all config fields used by `logWithSkip`) and one in `PublishLog()` (which snapshots `ch` before the channel send). Moving `ch` into `HotSnapshot` would race with `resetConfig`, which replaces `ch` wholesale; do not attempt.
- `SetMinLevel` remains under `configMu.Lock()` and also writes `atomicMinLevel`; both must be updated atomically within the lock.

**Affected files:** `config/config.go`, `entry/entry.go`.

---

### Component B — `model` and `entry` packages: Typed field union API (P2)

**Root cause addressed:** RC-2 — `interface{}` boxing in `model.LogAttr.Value` causes ~30 allocs/op.

**Current state:** `model.LogAttr.Value` is `any` (alias `LogAttrValue`). In `logWithSkip`, each field is dispatched through `config.AppendField(e.buf, key, field.Value)`, which performs a reflect-free type-switch. The `interface{}` boxing at the call site (`model.LogAttr{Key: "k", Value: 42}`) is the source of ~30 allocs/op: each concrete value is heap-escaped when assigned to an `any` field.

**Proposed change — union struct on `model.LogAttr`:**

Extend `model.LogAttr` with:
- A named `AttrKind` discriminant type (`type AttrKind uint8`) with constants `KindAny = 0` (zero value), `KindStr = 1`, `KindInt = 2`, `KindBool = 3`, `KindFloat64 = 4`.
- Four unexported typed value fields: `strVal string`, `intVal int64`, `boolVal bool`, `floatVal float64`.
- The existing `Value any` field is retained; it is used only when `Kind == KindAny`.

Zero value: `Kind = 0` (`KindAny`), all typed fields zero. All existing struct-literal call sites (`model.LogAttr{Key: "k", Value: v}`) continue to work without any source change — `Kind` defaults to `KindAny`, the existing `config.AppendField` type-switch fires as before.

**New methods on `*LogEntry`:**

Add four chainable typed field methods directly on `*LogEntry`:
- `Str(key string, val string) *LogEntry`
- `Int(key string, val int64) *LogEntry`
- `Bool(key string, val bool) *LogEntry`
- `Float64(key string, val float64) *LogEntry`

Each method appends the pre-rendered key prefix and the unboxed typed value directly to `e.buf` without touching `interface{}`. The backing array is already allocated (1 KB pre-growth); no allocation occurs for the append itself. Methods return `*LogEntry` to enable chaining.

The field dispatch loop in `logWithSkip` is updated to branch on `Kind`:
- `KindStr`, `KindInt`, `KindBool`, `KindFloat64` → direct typed append path (0 allocs).
- `KindAny` → existing `config.AppendField` type-switch (backward compat path).

**Why typed value fields are unexported:** Unexported fields prevent external packages from constructing `LogAttr` with inconsistent `Kind`/value combinations (e.g., `Kind=KindStr` with no `strVal`). The `Str`/`Int`/`Bool`/`Float64` constructor methods become the only safe typed-field construction path. This is the recommendation from AQ-2.

**Why direct methods on LogEntry vs a Fields builder:** A separate `Fields` builder would require passing it to `logWithSkip` via a variadic argument (new slice allocation) or as a field on `LogEntry` (struct size growth without benefit). Direct methods write immediately to `e.buf`, which is already allocated and in scope — no intermediate allocation.

**Struct size impact:** `LogAttr` grows from ~32 B (2 fields) to ~80 B (8 fields on 64-bit). The variadic `fields ...model.LogAttr` slice stack-allocates for <= 2 fields; beyond that it heap-allocates regardless of struct size. Net impact on alloc budget is negligible relative to the 30 allocs saved by eliminating boxing.

**Affected files:** `model/attr.go`, `entry/entry.go`.

---

### Component C — `config` and `entry` packages: Append-to-buf context API (P3)

**Root cause addressed:** RC-1 — `map[string]any` context iteration + `[]string` intermediate slice (~600 ns, ~23 allocs/op — the single largest saving).

**Current state:** `setLogContextFields` (entry.go:249–258) calls `GetConfig().ContextParser()` (a sixth `RLock`), calls the registered parser to obtain `map[string]any`, iterates the map building a `[]string` of rendered key-value fragments, and returns the slice. The `logWithSkip` loop (lines 183–186) appends each string to `e.buf`. Total cost: one `map[string]any` allocation, one `[]string` allocation, and O(n) interface-boxing inside `ValidateandParseLogField` per field. Additionally, `ValidateandParseLogField` acquires its own `configMu.RLock` to check `restrictedFieldsSet`, adding one extra lock acquisition per context field.

**Proposed change — dual-registration model:**

Add to `config`:
```
type ContextFieldsAppender = func(ctx context.Context, buf []byte) []byte
func SetContextFieldsAppender(appender ContextFieldsAppender)
```

Store `contextAppender ContextFieldsAppender` as a field on `Config` alongside `contextParser`. The field is snapshotted via the P1 config snapshot; no additional lock is needed when calling the appender.

In `setLogContextFields` (restructured):
1. If `cfg.contextAppender != nil`: call it directly with `(ctx, e.buf)`; return the extended buffer. 0 map allocations, 0 slice allocations. This is the zero-alloc fast path for new callers.
2. Else if `cfg.contextParser != nil`: call the parser to get `map[string]any`, iterate, and append fields to `e.buf` via an inline version of `ValidateandParseLogField` that accepts the already-snapshotted `restrictedFieldsSet` as a parameter — eliminating the extra `configMu.RLock` per field. This is the legacy path; it allocates but preserves backward compatibility.

**Priority rule:** Appender wins. If both are registered, the appender is called and the legacy parser is ignored. Documented in godoc.

**Why not an internal adapter:** Wrapping `ContextFieldsParser` output into a byte-append adapter would still allocate the `map[string]any` inside every parser call — the adapter cannot avoid it because the parser's return type is `map[string]any`. The dual-registration approach gives new callers a true zero-alloc path while legacy callers continue to work unchanged.

**AQ-4 resolution:** An internal variant of `ValidateandParseLogField` accepts `restrictedSet map[string]struct{}` as a parameter, eliminating the extra `configMu.RLock` per context field on the legacy path. The exported `ValidateandParseLogField` retains its existing signature for external callers.

**Affected files:** `config/config.go`, `entry/entry.go`.

---

### Component D — `encoder` and `entry` packages: Inline framing + ownership transfer (P4)

**Root cause addressed:** RC-4 — double-buffer: `en.Append(nil, e.buf)` allocates a new `[]byte` to frame the body; `PublishLog` makes a second `append([]byte(nil), Data...)` defensive copy. Total: 2 allocs/event.

**Current state:** `logWithSkip` accumulates the body into `e.buf` (fields without braces), then calls `en.Append(nil, e.buf)` which allocates a fresh `byteData` wrapping the body in `{…}\n`. `PublishLog` makes a second defensive copy (`dataCopy := append([]byte(nil), Data...)`) before sending to the channel. The second copy exists because `e.buf` and the pool recycler could race: without it, the background goroutine and `e.Put()` would share the same backing array.

**Proposed change:**

1. **Extend `encoder.Encoder` interface** with two new methods:
   - `OpenBytes() []byte` — returns the opening frame delimiter(s).
   - `CloseBytes() []byte` — returns the closing frame delimiter(s) including any trailing newline.

   Implementations: `JSONEncoder.OpenBytes()` returns `[]byte{'{'}`, `JSONEncoder.CloseBytes()` returns `[]byte{'}', '\n'}`. `TextEncoder.OpenBytes()` returns `nil`, `TextEncoder.CloseBytes()` returns `[]byte{'\n'}`.

   The existing `Append` method is retained on the interface for backward compatibility. External implementations that have not added `OpenBytes`/`CloseBytes` will get a compile-time error — explicit and fixable.

2. **Inline framing in `logWithSkip`:** After the config snapshot is taken (P1), obtain `en := cfg.Encoder()` once. Then:
   - Prepend `en.OpenBytes()` to `e.buf` before writing any fields (single `append(e.buf, openBytes...)` — zero allocation if `e.buf` has slack, which the 1 KB pre-growth ensures).
   - Append fields and context as normal.
   - Append `en.CloseBytes()` after the last field.
   - `e.buf` now holds the complete, framed, ready-to-publish event.

3. **Eliminate second copy in `PublishLog`:** Remove `dataCopy := append([]byte(nil), Data...)`. Instead, `logWithSkip` sends `e.buf` directly as `LogEvent.Data`. The ownership of the backing array is transferred to the channel on the send.

4. **Sever pool alias before `e.Put()`:** Immediately after the channel send, `logWithSkip` assigns a fresh backing slice: `e.buf = make([]byte, 0, initBufCap)`. This severs the alias between the pool slot and the in-flight `LogEvent.Data`. `reset()` in the pool then retains the new empty slice (not the transferred one).

**Ownership transfer correctness:** The Go channel send copies the `LogEvent` struct (slice header: pointer + length + capacity). Both the channel value and `e.buf` temporarily point to the same backing array. The `make(...)` assignment before `e.Put()` replaces `e.buf`'s pointer with a new allocation. The background `ProcessLogEvent` goroutine reads the old backing array via `LogEvent.Data`. The channel send happens-before the receive; the backing array has no writer after the send. This is not a data race and is verified by `go test -race ./...`.

**Why `make([]byte, 0, initBufCap)` and not `e.buf[:0]`:** `e.buf[:0]` would create a slice with `len=0` but the same backing array as the in-flight event — re-establishing the alias. The fresh `make` is mandatory. This is the resolution of AQ-3: use `initBufCap` for simplicity; revisit if bench shows pool-warm reallocation cost (unlikely given 1 KB capacity covers ~80% of log lines).

**Affected files:** `encoder/encoder.go`, `encoder/json_encoder.go`, `encoder/text_encoder.go`, `entry/entry.go`, `config/config.go`.

---

### Component E — `config` package: Channel capacity (P5)

**Root cause addressed:** RC-5 — channel cap=10 blocks all goroutines under 8-goroutine parallel load; callers spend the majority of wall time blocked on `ch <-`.

**Current state:** `resetConfig` hardcodes `make(chan LogEvent, 10)`. Under GOMAXPROCS=8 parallel load, the channel fills in < 1 µs and all callers block, dominating wall time.

**Proposed change:**

1. Change `DafaultLogBuffer` from `20` to `1000` (the authoritative default).
2. Add a package-level variable `channelCapacity int` (not on `Config`) initialised to `1000`. Pattern is consistent with existing package-level config vars (`restrictedFieldsSet`, `DafaultLevel`).
3. Add `SetChannelCapacity(n int)` that validates and stores into `channelCapacity`. Validation: values <= 0 clamped to 1000; values > 100,000 clamped to 100,000; `log.Printf` warning emitted on clamp; no panic.
4. `resetConfig` reads `channelCapacity` when constructing the channel: `make(chan LogEvent, channelCapacity)`.

**Init-time-only semantics:** `SetChannelCapacity` takes effect only at the next `resetConfig` call. In production, `resetConfig` is called exactly once from `init()`. The godoc documents: "Must be called before any log call. Has no effect if the package has already been initialised." This is consistent with the `DafaultLevel` / `DafaultEncoderType` / `DefaultAddSource` package-level var pattern.

**Why package-level var, not on `Config`:** `channelCapacity` is consumed by `resetConfig` at `init()` time, before any `*Config` pointer exists. Placing it on `Config` would require `resetConfig` to read a field from an object it is in the process of constructing.

**Expected impact:** Under GOMAXPROCS=8 with 10 context fields, after P1–P4 reduce per-call cost to ~333 ns/op, the channel capacity of 1000 can absorb ~3 ms of burst before any caller blocks. This makes channel blocking negligible for the benchmark and for typical service traffic patterns.

**Affected files:** `config/config.go`.

---

## 6. Data Flow

### Hot path after all five stories

```
entry.logWithSkip(level, ctx, msg, err, skip, fields...)
  │
  ├── 1. atomic.Int32.Load(&atomicMinLevel)       ← 0 allocs, 0 locks
  │      level < min → e.Put(); return            ← filtered path exits here
  │
  ├── 2. config.EventPreProcessors == nil?        ← map read, 0 locks (checked once)
  │      nil → e.Put(); return
  │
  ├── 3. configMu.RLock()                         ← RLock #1 of 2 for this call
  │      cfg := defaultConfig                     ← pointer copy (8 bytes)
  │      configMu.RUnlock()
  │
  ├── 4. [if cfg.addSource] runtime.Caller(skip)  ← optional, off by default
  │
  ├── 5. e.buf = e.buf[:0]                        ← reset len, retain backing array
  │      e.buf = append(e.buf, cfg.encoder.OpenBytes()...)
  │      [JSON: prepend "{"; text: no-op]
  │
  ├── 6. append time/level/msg                    ← pre-rendered key prefixes, 0 allocs
  │
  ├── 7. for each field in fields:
  │      ├── Kind == KindStr/Int/Bool/Float64      ← direct typed append, 0 allocs
  │      └── Kind == KindAny (zero value)          ← AppendField type-switch (legacy)
  │
  ├── 8. if cfg.contextAppender != nil:           ← new zero-alloc path (P3)
  │         e.buf = cfg.contextAppender(ctx, e.buf)
  │      else if cfg.contextParser != nil:        ← legacy map[string]any path
  │         [allocates; backward compat]
  │
  ├── 9. [if err != nil] append error field
  │      [if e.caller != nil] append caller field
  │      [if cfg.staticFields != ""] append static fields
  │
  ├── 10. e.buf = append(e.buf, cfg.encoder.CloseBytes()...)
  │       [JSON: append "}\n"; text: append "\n"]
  │
  ├── 11. PublishLog: configMu.RLock() → snapshot ch → configMu.RUnlock()  ← RLock #2 of 2
  │        ch <- LogEvent{Level: level, Data: e.buf}  ← non-blocking at cap=1000
  │        [ownership of backing array transferred to channel]
  │
  ├── 12. e.buf = make([]byte, 0, initBufCap)          ← sever pool alias
  │
  └── 13. e.Put()                                       ← return to pool

Background goroutine (ProcessLogEvent):
  for event := range ch:
    processors := EventPreProcessors  [snapshot under RLock]
    for each observer: observer.PreProcess(event.Level, event.Data)
    → output writer (via PreProcessor)
```

### Filtered path (must not regress from ~35 ns/op, 0 allocs)

```
entry.logWithSkip(level below minLevel, ...)
  │
  ├── atomic.Int32.Load(&atomicMinLevel)   ← 0 allocs, 0 locks
  │      level < min → e.Put(); return    ← exits here
  │
  └── total: ~35 ns, 0 allocs
```

The filtered path is unchanged in structure from v1.0.0. The only difference is that `config.GetConfig().MinLevel() > level` (which acquired RLock) is replaced by `enum.LogLevel(atomicMinLevel.Load()) > level` (which acquires no lock). This strictly reduces latency; regression is not possible.

### Performance budget breakdown (target: <= 1,000 ns/op total)

| Step | Current cost | After fix | Story |
|---|---|---|---|
| Level gate (atomic load) | ~50 ns (RLock) | ~5 ns | P1 |
| Config snapshot (1 RLock) | ~250 ns (6 RLocks) | ~50 ns | P1 |
| Typed field append (10 fields) | ~300 ns (boxing + type-switch) | ~30 ns | P2 |
| Context field append (10 fields) | ~600 ns (map alloc + iteration) | ~30 ns | P3 |
| Frame + channel copy (2 allocs) | ~150 ns | ~30 ns | P4 |
| Channel send (blocked) | ~2,000 ns (cap=10 block) | ~10 ns | P5 |
| time.RFC3339 format, pool, misc | ~90 ns | ~90 ns | — |
| **Total** | **~3,440 ns** | **~245 ns** | — |

The ~245 ns estimate provides ~4× headroom under the 1,000 ns/op gate.

---

## 7. APIs and Integration Points

### 7.1 New public APIs (additions)

| Symbol | Package | Shape | Story |
|---|---|---|---|
| `ContextFieldsAppender` | `config` | `type ContextFieldsAppender = func(context.Context, []byte) []byte` | P3 |
| `SetContextFieldsAppender` | `config` | `func SetContextFieldsAppender(ContextFieldsAppender)` | P3 |
| `OpenBytes` | `encoder.Encoder` | `OpenBytes() []byte` (interface method) | P4 |
| `CloseBytes` | `encoder.Encoder` | `CloseBytes() []byte` (interface method) | P4 |
| `SetChannelCapacity` | `config` | `func SetChannelCapacity(n int)` | P5 |
| `Str` | `entry.LogEntry` | `func (e *LogEntry) Str(key string, val string) *LogEntry` | P2 |
| `Int` | `entry.LogEntry` | `func (e *LogEntry) Int(key string, val int64) *LogEntry` | P2 |
| `Bool` | `entry.LogEntry` | `func (e *LogEntry) Bool(key string, val bool) *LogEntry` | P2 |
| `Float64` | `entry.LogEntry` | `func (e *LogEntry) Float64(key string, val float64) *LogEntry` | P2 |

`atomicMinLevel` is package-level unexported; it is not part of the public API surface.

### 7.2 Changed public APIs (backward compatible)

| Symbol | Change | Backward compat guarantee |
|---|---|---|
| `model.LogAttr` | New fields `Kind AttrKind`, `strVal string`, `intVal int64`, `boolVal bool`, `floatVal float64` added | Struct literal `{Key: ..., Value: ...}` still compiles; `Kind` zero-value = `KindAny`; existing call sites require no change |
| `config.DafaultLogBuffer` | Default value raised from `20` to `1000` | Behavioural change: programs relying on `DafaultLogBuffer=20` for backpressure semantics see reduced blocking. This is the intended outcome of P5. |
| `encoder.Encoder` interface | Two new methods `OpenBytes()` and `CloseBytes()` added | **Known compile-time break for external implementations.** Internal implementations updated atomically. Must be documented in release notes. |

### 7.3 Unchanged public APIs

`SetContextFieldsParser`, `ContextFieldsParser`, `GetConfig`, `PublishLog` (signature unchanged; internal implementation changes), `SetMinLevel`, `SetLogBufferMaxSize`, all `Config` accessor methods (`MinLevel`, `AddSource`, `Encoder`, `DefaultFields`, `DefaultFieldsRendered`, `TimeFormat`, `StaticFields`, `ContextParser`), `entry.Log`/`Debug`/`Info`/`Warn`/`Error`/`Fatal`/`Panic`.

---

## 8. Data Model Overview

### 8.1 model.LogAttr (after P2)

Before:
```
LogAttr {
  Key   LogAttrKey   // type alias for string
  Value LogAttrValue // type alias for any
}
```

After:
```
LogAttr {
  Key      LogAttrKey   // unchanged
  Value    LogAttrValue // unchanged; populated when Kind == KindAny
  Kind     AttrKind     // type AttrKind uint8; 0=KindAny (zero value)
  strVal   string       // populated when Kind == KindStr
  intVal   int64        // populated when Kind == KindInt
  boolVal  bool         // populated when Kind == KindBool
  floatVal float64      // populated when Kind == KindFloat64
}
```

`AttrKind` constants: `KindAny = 0`, `KindStr = 1`, `KindInt = 2`, `KindBool = 3`, `KindFloat64 = 4`.

Zero-value invariant: a `LogAttr{}` literal or `model.LogAttr{Key: "k", Value: v}` produces `Kind=0` (`KindAny`), routing through the existing type-switch path. No observable behaviour change for existing callers.

### 8.2 config.Config (after P1 + P3)

`Config` gains exactly one new field:
```
contextAppender ContextFieldsAppender   // nil if not registered; appender wins over contextParser
```

All other fields are unchanged. `atomicMinLevel` lives as a package-level `atomic.Int64`, not on `Config`, to avoid requiring the hot path to dereference `*Config` before the gate check. `atomic.Int64` is used because `enum.LogLevel` is `type LogLevel int`, which is 64 bits on all modern Go platforms; using `Int64` avoids any narrowing cast.

### 8.3 config.LogEvent (unchanged)

```
LogEvent {
  Level enum.LogLevel
  Data  []byte   // after P4: caller transfers ownership; no second copy in PublishLog
}
```

The only change is semantic: after P4, `Data` is no longer a freshly-allocated defensive copy made by `PublishLog`. It is the original `e.buf` backing array, transferred by the sender. The receiver (`ProcessLogEvent`) continues to use `Data` unchanged.

---

## 9. Scalability Expectations

LogNugget is an embedded library; scale is measured in concurrent goroutines sharing a single `entryPool` and dispatch channel.

| Dimension | Before Epic V2 | After Epic V2 |
|---|---|---|
| Parallel throughput (GOMAXPROCS=8) | ~290 K ops/sec | >= 3 M ops/sec (10×) |
| Channel fill-up latency | ~3 µs (10 events × 3,440 ns/op) | ~333 µs (1,000 events × ~333 ns/op) |
| Blocking callers at burst | > 8 goroutines almost always blocked | > 1,000 in-flight events before any caller blocks |
| allocs/op (parallel 10-field path) | 55 | <= 5 |
| GC pressure | High (55 allocs/op × volume) | Low (<=5 allocs/op) |

The 3 M ops/sec target is an intermediate milestone. The zerolog ceiling (~9 M ops/sec) requires eliminating the remaining channel-send cost (~10–20 ns/send) via a lock-free ring buffer — deferred to a future epic (PRD N4). P5's capacity increase is the 80%-solution that unblocks the SLO.

---

## 10. Reliability and Failure Modes

### 10.1 Level change eventual consistency (P1)

After P1, `SetMinLevel` writes both `defaultConfig.minLevel` (under `configMu.Lock()`) and `atomicMinLevel` (also under `configMu.Lock()`). A goroutine that reads `atomicMinLevel` atomically may observe the old level for at most one log call during a concurrent `SetMinLevel`. This is a single-event eventual-consistency window. Documented in godoc. Consistent with zerolog behaviour.

### 10.2 Buffer ownership race (P4)

The `e.buf` backing array is shared between the `LogEntry` pool and the `LogEvent` channel from the moment of `ch <-` until `e.buf = make(...)` reassigns the field. This window is within a single goroutine with no concurrent access — the pool cannot recycle `e` before `e.Put()` is called. The `make(...)` reassignment severs the alias before `e.Put()`.

Mitigation: mandatory `go test -race ./...` on the benchmark; explicit audit of `reset()` in `entry/pool.go` to ensure it retains the new (not transferred) backing slice.

### 10.3 Channel backpressure (P5)

At capacity=1000, a burst of > 1,000 concurrent events blocks callers. This is by design — the library makes no lossless guarantee under extreme burst (PRD N4). Services with sustained throughput > 1,000 log events/ms should use `SetChannelCapacity` to tune upward (max 100,000) or await the lock-free ring buffer epic.

### 10.4 Encoder interface extension (P4)

External `encoder.Encoder` implementations that do not add `OpenBytes`/`CloseBytes` will fail to compile. This is a build-time failure — detectable, not silent, fixable. Must be documented in the release notes and the feature branch PR description. Mitigation: provide a `NoopFramer` embed struct with default `OpenBytes() []byte { return nil }` and `CloseBytes() []byte { return []byte{'\n'} }` implementations to ease migration for external implementors.

### 10.5 Dual-registration priority (P3)

If both `SetContextFieldsParser` and `SetContextFieldsAppender` are registered, the appender silently wins and the parser is not called. Documented in godoc. A test that registers both and asserts appender output wins is required.

---

## 11. Security and Compliance Considerations

- No new network surfaces. No new file I/O. No new goroutines beyond the existing `ProcessLogEvent` background goroutine.
- No new external dependencies. Attack surface is unchanged.
- `SetChannelCapacity(n)` clamps to 100,000 to prevent OOM from a misconfigured capacity. This is the only user-input guard added in this epic.
- Context field content is caller-controlled and unchanged by this epic. No personally identifiable data is logged by the library itself.
- `go mod tidy` must produce no diff after all five stories are merged — verified by CI.

---

## 12. Observability

- `./scripts/bench-check.sh` is the primary observability instrument. It runs on every PR to the feature branch and reports ns/op, allocs/op, and B/op per benchmark. Merging to `develop` is blocked on a green gate.
- Each of the five stories ships a `*_bench_test.go` file (see benchmark coverage plan below).
- `go test -race ./...` output is the race-safety instrument; it is run on every PR.
- No OTel metrics are added in this epic (Non-Goal N5). The existing `config.Hooks` mechanism could emit `log.published_events` and `log.channel_depth` metrics in a future epic.

### Benchmark coverage plan

| Story | Package | Benchmark file | Key benchmark(s) |
|---|---|---|---|
| P1 | `entry/` | `atomic_snapshot_bench_test.go` | `BenchmarkLogEntry_FilteredPath`, `BenchmarkLogEntry_HotPath_NoCtx` |
| P2 | `entry/` | `typed_fields_bench_test.go` | `BenchmarkLogEntry_TypedFields_10`, `BenchmarkLogEntry_LegacyAttrs_10` |
| P3 | `entry/` | `ctx_appender_bench_test.go` | `BenchmarkLogEntry_CtxAppender_10`, `BenchmarkLogEntry_CtxParser_10` |
| P4 | `entry/` | `inline_framing_bench_test.go` | `BenchmarkLogEntry_InlineFraming_10` |
| P5 | `entry/` | `parallel_bench_test.go` | `BenchmarkLognugget_Parallel_10CtxFields` (GOMAXPROCS=8) |

The `BenchmarkLognugget_Parallel_10CtxFields` benchmark is the authoritative Definition of Done benchmark. It should be updated incrementally: after P1 it measures P1 savings; after P5 it measures the full combined effect. All benchmarks must include `b.ReportAllocs()`.

---

## 13. Deployment and Environments

LogNugget is a library, not a service. There is no deployment in the traditional sense. The library is imported by downstream Go modules.

- **Development:** Work on `feat/81-v2-performance` sub-branches (`/p1-atomic-minlevel`, `/p2-typed-field-api`, `/p3-ctx-appender`, `/p4-inline-framing`, `/p5-channel-capacity`). Bench gate verified locally before each PR.
- **CI:** Bench gate (`./scripts/bench-check.sh`) runs on every PR to the feature branch. Individual story PRs are not required to pass against the v1.0.0 baseline (the gate is not expected to clear until P1+P3 merge together). The full feature branch must pass before the PR to `develop` is marked ready.
- **Release:** Feature branch merges to `develop` via squash merge after all five stories are in, the bench gate is green, and the baseline update commit has landed on the feature branch.

---

## 14. Key Architectural Decisions

### KD-1: Atomic package-level var for minLevel, not atomic.Pointer[Config]

**Decision:** Package-level `atomic.Int32` for `minLevel` only; all other config fields remain on `*Config` guarded by `configMu`. The hot path takes one RLock snapshot for the full config after the atomic gate.

**Tradeoff:** `atomic.Pointer[Config]` (copy-on-write) would eliminate all RLock acquisitions on the hot path but requires cloning the full `Config` struct on every `Set*` call. For a library used in throughput-sensitive services, allocation on the write path (startup configuration) is acceptable. The hybrid approach — atomic gate, one-shot RLock snapshot — delivers equivalent hot-path benefit at zero write-path cost, and is simpler to audit.

### KD-2: Union struct on LogAttr, not a separate typed-field sum type

**Decision:** Extend `model.LogAttr` with `Kind AttrKind` + four unexported typed value fields rather than introducing a new `model.TypedField` type.

**Tradeoff:** A new type would have a cleaner API surface but would require callers to change all call sites. The union struct threads the needle: zero call-site changes for `KindAny` (backward compat), zero boxing for the four typed kinds (performance win). The typed value fields are unexported to prevent inconsistent construction.

### KD-3: OpenBytes/CloseBytes over removing Append from encoder.Encoder

**Decision:** Add two new interface methods; keep `Append`.

**Tradeoff:** Removing `Append` is cleaner but breaks external implementations silently at runtime. Adding two methods produces a compile-time error — explicit, fixable, detectable in CI. Retaining `Append` preserves the interface contract for existing external consumers who wrap the encoder.

### KD-4: Dual-registration for context fields, not internal adapter

**Decision:** New `ContextFieldsAppender` registration coexists with `ContextFieldsParser`; appender wins when both are registered.

**Tradeoff:** An internal adapter wrapping `ContextFieldsParser` output into a byte-append call would be transparent to callers but still allocates `map[string]any` inside every parser call. The dual-registration approach requires new callers to opt in but gives them a true zero-alloc path. Legacy callers continue to work unchanged.

### KD-5: Init-time-only channel capacity, no runtime resize

**Decision:** `SetChannelCapacity` stores a package-level var consumed by `resetConfig` at `init()` time. No drain-and-replace mechanism.

**Tradeoff:** Runtime resize would allow tuning without restart but requires draining the old channel under lock while new sends could arrive — a non-trivial coordination problem with non-zero event-drop risk. Init-time-only is simple, safe, and sufficient for the intended use case (tuning at program startup alongside `DafaultLevel`).

### KD-6: Ownership transfer via fresh make, not e.buf[:0]

**Decision:** After the channel send, `e.buf = make([]byte, 0, initBufCap)` replaces the backing array rather than `e.buf = e.buf[:0]`.

**Tradeoff:** `e.buf[:0]` is cheaper (~1 ns) but silently re-establishes the alias with the in-flight channel event. The `make` (~5 ns) is marginally more expensive but is the only safe option. The performance cost is within the noise floor of the ~245 ns total target.

---

## 15. Alternatives Considered

### Alt-1: atomic.Pointer[Config] for full config snapshot (rejected)

Would eliminate all `configMu.RLock` acquisitions on the hot path. Rejected because `Set*` calls become allocation-heavy (must clone the full `Config` struct, which contains maps and slices). The current single-snapshot pattern (one RLock per log call) already reduces from 6 acquisitions to 1, which is sufficient to meet the SLO.

### Alt-2: Per-goroutine config cache via goroutine-local storage (rejected)

No stable goroutine-local storage exists in Go without cgo or assembly. Rejected as incompatible with "no new dependencies" and unsupportable across Go versions.

### Alt-3: Fields builder pattern (zerolog-style chain from the logger object, not LogEntry) (rejected)

Would require a builder struct accumulating fields before the level call. The PRD mandates backward compat with the existing `fields ...model.LogAttr` variadic pattern; a builder would change call-site shape significantly. Direct typed methods on `LogEntry` are the minimal, non-breaking extension.

### Alt-4: Replace chan LogEvent with lock-free ring buffer (deferred)

Would eliminate the remaining channel-send cost (~10–20 ns/send) and the final scaling constraint. Deferred to a future epic (PRD N4). P5's capacity increase is the 80%-solution that unblocks the SLO.

### Alt-5: Precompute framing bytes once at config init, store on Config (rejected for P4)

`OpenBytes`/`CloseBytes` return constant byte slices that do not change after encoder construction. Caching them in `logWithSkip` as local variables after the config snapshot (two extra local assignments) is equivalent and simpler than adding fields to `Config`. No additional caching layer needed.

### Alt-6: Internal adapter wrapping ContextFieldsParser into ContextFieldsAppender shape (rejected for P3)

See KD-4. The adapter still allocates `map[string]any`; it cannot avoid it because that is the parser's return type. Rejected in favour of dual registration.

---

## 16. Risks and Mitigations

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| P4 buffer ownership race: `e.buf` backing array shared with in-flight channel event until `make(...)` reassignment | High without explicit mitigation | High (data corruption, race detector alert) | Mandatory `go test -race ./...` on the benchmark before marking P4 PR ready; explicit audit of `reset()` in `entry/pool.go` to confirm it retains the new, not transferred, slice |
| Bench gate fails mid-epic: individual story PRs cause regression without clearing the gate | Medium | Medium (blocks merge to develop) | Document in each story PR that the full bench gate is evaluated at the feature branch level, not per-story. Individual story PRs require only `go test -race ./...` and `golangci-lint run`. |
| P1 TOCTOU window: level read atomically, body encoded with stale config snapshot | Low | Low (at most one mis-logged/mis-suppressed event during `SetMinLevel`) | Document in godoc as intentional eventual consistency. Consistent with zerolog. |
| encoder.Encoder interface extension breaks external implementations | Medium (unknown forks) | Low (compile-time break, not runtime) | Document in feature branch PR description and release notes. Provide `NoopFramer` embed helper to ease migration. |
| P3 dual-registration: appender-wins behaviour surprising to callers registering both | Low | Low | Godoc clearly states priority. Test asserts appender wins when both registered. |
| P2 LogAttr struct growth from ~32 B to ~80 B causes unexpected GC pressure | Low | Low | Benchmark `NewLogEntry()`/`e.Put()` round-trip alloc count before and after; confirm 0 new allocs on filtered path. |
| Channel at cap=1000 under sustained burst (> 1000 events/ms) still blocks | Known, accepted | Low | RC-5 fix is a throughput fix, not a lossless guarantee. Document in `SetChannelCapacity` godoc. Lock-free ring buffer deferred to future epic. |

---

## 17. Rollout Plan

```
Phase 1 — P1 (blocks P2, P3, P4)
  Branch:  feat/81-v2-performance/p1-atomic-minlevel
  Gate:    go test -race ./...; BenchmarkLogEntry_FilteredPath <= 35 ns/op, 0 allocs
  Merges to: feat/81-v2-performance
  Note: P2, P3, P4 sub-branches must be cut off feat/81-v2-performance AFTER P1 merges

Phase 2 — P2 + P3 + P4 (parallel, after P1 merges to feature branch)
  Branches: .../p2-typed-field-api, .../p3-ctx-appender, .../p4-inline-framing
  Gate per PR: go test -race ./...; golangci-lint run
  Note: full bench gate NOT required per-story; evaluated at feature branch level
  Merges to: feat/81-v2-performance (each story PR separately, any order)
  Milestone M2: after all three merge → allocs/op <= 5; hot path <= 1,000 ns/op

Phase 3 — P5 (after P2+P3+P4 merge to feature branch)
  Branch:  feat/81-v2-performance/p5-channel-capacity
  Gate:    BenchmarkLognugget_Parallel_10CtxFields >= 3 M ops/sec; bench-check.sh exits 0
  Merges to: feat/81-v2-performance
  Milestone M3: green bench gate

Phase 4 — Baseline update (Project Lead action)
  Action:  ./scripts/bench-check.sh --update-baseline
  Commits: bench-baseline.txt to feat/81-v2-performance
  Ref:     #81

Phase 5 — Merge to develop
  PR:      feat/81-v2-performance → develop (squash merge)
  Gate:    all five story PRs merged; bench gate green; baseline updated
  Outcome: umbrella issue #81 closed
```

---

## 18. Open Questions

All five open questions from the wiki (Q1–Q5) have been resolved in PRD Section 3. The following architectural sub-questions are resolved here and assigned to the LLD for elaboration:

| ID | Question | Resolution | LLD section |
|---|---|---|---|
| AQ-1 | `Kind` field type: `uint8` or named type? | Named type `AttrKind` prevents accidental arithmetic. **Resolved: named type.** | LLD — P2 |
| AQ-2 | Typed value fields on `LogAttr`: exported or unexported? | Unexported prevents inconsistent construction. **Resolved: unexported.** | LLD — P2 |
| AQ-3 | Fresh `make` capacity after ownership transfer: `initBufCap` or previous event size? | Use `initBufCap` for simplicity. Warm-start hint deferred. **Resolved: `initBufCap`.** | LLD — P4 |
| AQ-4 | Should `ValidateandParseLogField` be refactored to accept `restrictedSet` as a parameter to eliminate its extra `configMu.RLock`? | Yes — add an internal variant `validateAndAppendField(dst []byte, key string, value any, restrictedSet map[string]struct{}) []byte` used by the legacy P3 context path. **Resolved: add internal variant.** | LLD — P3 |
