# Low-Level Design: Epic V2 — Performance

| Field | Value |
|---|---|
| Document type | Low-Level Design |
| Feature slug | `v2-performance` |
| HLD | `docs/hld/v2-performance.md` |
| Status | DRAFT |
| Author | Chief Architect |
| Date | 2026-05-21 |

This document expands the five HLD components into exact Go signatures, concurrency invariants, request lifecycle steps, and test strategy. It does not restate HLD context, goals, scalability narrative, or alternatives. Read the HLD first.

---

## 1. Module / Package Layout

No new packages. No new files except benchmark files. Modifications are confined to:

```
config/
  config.go          — P1 (atomicMinLevel, HotSnapshot, SetContextFieldsAppender, SetChannelCapacity)
  parse_field.go     — P2/P3 (AppendAttr, validateAndAppendField internal variant)
  atomic_snapshot_bench_test.go   — P1 bench
entry/
  entry.go           — P1/P2/P3/P4 (logWithSkip rewrite)
  typed_fields_bench_test.go      — P2 bench
  ctx_appender_bench_test.go      — P3 bench
  inline_framing_bench_test.go    — P4 bench
  parallel_bench_test.go          — P5 bench
model/
  attr.go            — P2 (AttrKind, typed union fields, constructors, accessors)
encoder/
  encoder.go         — P4 (OpenBytes/CloseBytes added to Encoder interface; NoopFramer)
  json_encoder.go    — P4 (OpenBytes/CloseBytes on JSONEncoder)
  text_encoder.go    — P4 (OpenBytes/CloseBytes on TextEncoder)
```

---

## 2. Component A — config package: Atomic minLevel + HotSnapshot (P1)

### 2.1 New package-level variable

```go
// atomicMinLevel mirrors defaultConfig.minLevel for lock-free reads on the
// hot path. Written under configMu.Lock() in SetMinLevel and resetConfig.
// Read with atomic.Load on the level gate — 0 locks, ~5 ns.
var atomicMinLevel atomic.Int64
```

`atomic.Int64` stores `int64(enum.LogLevel)`. `enum.LogLevel` is defined as `type LogLevel int`, which is 64 bits on all modern Go platforms. Using `atomic.Int64` matches the underlying type width exactly and eliminates any narrowing cast.

The implementation must include the following compile-time assertion in `config/config.go` (or `config/atomic_level.go`):

```go
// Verify LogLevel fits in int64 (atomic storage type).
var _ int64 = int64(enum.LevelFatal) // compile-time width check
```

This guards against any future broadening of `enum.LogLevel` to a wider type.

### 2.2 GetAtomicMinLevel

```go
// GetAtomicMinLevel returns the current minimum log level using a single
// atomic load. Zero locks acquired. Safe for concurrent use.
// Used exclusively by entry.logWithSkip for the level gate.
func GetAtomicMinLevel() enum.LogLevel {
    return enum.LogLevel(atomicMinLevel.Load())  // int64 → LogLevel (int); same width, no truncation
}
```

### 2.3 HotSnapshot struct

```go
// HotSnapshot is a value-type snapshot of all Config fields consumed by
// entry.logWithSkip on the hot path. It is populated by a single
// configMu.RLock acquisition and then used lock-free for the remainder
// of the log call.
//
// Fields are ordered to minimise struct padding on 64-bit platforms:
// pointer-sized fields first, then slices/maps, then scalars.
type HotSnapshot struct {
    Encoder          encoder.Encoder                   // never nil
    ContextAppender  ContextFieldsAppender             // nil if not registered
    ContextParser    ContextFieldsParser               // nil if not registered
    Rendered         map[enum.DefaultLogKey][]byte     // pre-rendered "key": prefixes
    DefaultFields    map[enum.DefaultLogKey]string     // key name set for collision check
    RestrictedFields map[string]struct{}               // O(1) restricted key set
    EncoderOpen      []byte                            // cached cfg.Encoder.OpenBytes()
    EncoderClose     []byte                            // cached cfg.Encoder.CloseBytes()
    StaticFields     string                            // pre-rendered static field fragment
    TimeFormat       string                            // e.g. time.RFC3339
    AddSource        bool
}
```

`EncoderOpen` and `EncoderClose` are cached inside `GetHotSnapshot()` by calling `OpenBytes()` and `CloseBytes()` once — this avoids repeated interface dispatch inside the hot loop.

### 2.4 GetHotSnapshot

```go
// GetHotSnapshot acquires configMu.RLock exactly once, copies all fields
// needed by entry.logWithSkip into a HotSnapshot value, and returns it.
// The RLock is released before returning; no lock is held by the caller.
//
// Invariant: this function is the first of two RLock acquisitions on the
// hot path. The second occurs in PublishLog, which snapshots ch under its
// own RLock immediately before the channel send. Total: 2 RLocks per log
// call on the hot path, down from 6+. All field reads after GetHotSnapshot
// returns use the snapshot directly (0 additional locks).
func GetHotSnapshot() HotSnapshot {
    configMu.RLock()
    snap := HotSnapshot{
        Encoder:         defaultConfig.encoderObj,
        ContextAppender: defaultConfig.contextAppender,
        ContextParser:   defaultConfig.contextParser,
        Rendered:        defaultConfig.defaultFieldsRendered,
        DefaultFields:   defaultConfig.defaultFields,
        RestrictedFields: restrictedFieldsSet,
        StaticFields:    defaultConfig.parsedStaticFields,
        TimeFormat:      defaultConfig.timeFormat,
        AddSource:       defaultConfig.addSource,
    }
    configMu.RUnlock()
    // Cache framing bytes outside the lock; these are constant after
    // encoder construction and safe to read without the lock.
    snap.EncoderOpen = snap.Encoder.OpenBytes()
    snap.EncoderClose = snap.Encoder.CloseBytes()
    return snap
}
```

Note: `snap.Encoder.OpenBytes()` / `CloseBytes()` are called after the lock is released. This is safe because `Encoder` is an interface value whose underlying pointer does not change after snapshot; the methods return constant byte slices that do not touch mutable state.

### 2.5 SetMinLevel — dual write

```go
func SetMinLevel(level enum.LogLevel) {
    configMu.Lock()
    defer configMu.Unlock()
    defaultConfig.minLevel = level
    atomicMinLevel.Store(int64(level))   // ← P1 addition; int64 matches LogLevel's underlying type
}
```

Both writes occur under the same `configMu.Lock()`. A reader that atomically loads `atomicMinLevel` observes the new value as soon as `configMu.Unlock()` returns; at most one log call may execute against the stale value during the lock hold time.

### 2.6 resetConfig — dual write + channel capacity

```go
func resetConfig() {
    newCh := make(chan LogEvent, channelCapacity)  // P5: reads package-level var
    // ... build newCfg ...
    configMu.Lock()
    ch = newCh
    defaultConfig = newCfg
    EventPreProcessors = make(map[string]preProcessingObserverContract)
    restrictedFieldsSet = buildRestrictedSet(newCfg.defaultFields)
    atomicMinLevel.Store(int64(newCfg.minLevel))  // ← P1 addition; int64 matches LogLevel's underlying type
    configMu.Unlock()
    go ProcessLogEvent()
}
```

### 2.7 ContextFieldsAppender type and SetContextFieldsAppender

```go
// ContextFieldsAppender is a function that appends per-request context
// fields directly to dst and returns the extended slice. It is the
// zero-alloc alternative to ContextFieldsParser.
//
// When registered via SetContextFieldsAppender, the hot path calls:
//   e.buf = appender(ctx, e.buf)
// No map[string]any or []string is allocated on the hot path.
//
// If both SetContextFieldsAppender and SetContextFieldsParser are
// registered, the appender takes priority and the parser is not called.
// This priority is documented here and enforced in logWithSkip.
type ContextFieldsAppender = func(ctx context.Context, dst []byte) []byte

// SetContextFieldsAppender registers the zero-alloc context appender.
// Calling with nil clears the registration.
// Safe for concurrent use.
func SetContextFieldsAppender(appender ContextFieldsAppender) {
    configMu.Lock()
    defer configMu.Unlock()
    defaultConfig.contextAppender = appender
}
```

`contextAppender ContextFieldsAppender` is added as an unexported field on `Config` immediately after `contextParser`, preserving struct layout proximity for readability.

### 2.8 Config struct addition

The only new field on `Config`:

```go
type Config struct {
    // ... existing fields unchanged ...
    contextParser    ContextFieldsParser    // legacy path
    contextAppender  ContextFieldsAppender  // P3 zero-alloc path; wins if both set
}
```

### 2.9 Concurrency invariants for Component A

| Invariant | Enforcement |
|---|---|
| Filtered path: 0 locks | Level gate uses `GetAtomicMinLevel()` only |
| Hot path: 2 RLocks total | `GetHotSnapshot()` acquires the first RLock in `logWithSkip`; `PublishLog` acquires a second RLock to snapshot `ch`. Moving `ch` into `HotSnapshot` would race with `resetConfig`; do not attempt. |
| `atomicMinLevel` always consistent with `defaultConfig.minLevel` | Both written under `configMu.Lock()` in `SetMinLevel` and `resetConfig` |
| `GetConfig()` unaffected | Retains its own RLock; not called from hot path after P1 |

---

## 3. Component B — model and entry packages: Typed field union (P2)

### 3.1 AttrKind and updated LogAttr — model/attr.go

```go
package model

// AttrKind is the discriminant for LogAttr's typed union. Using a named
// type (not a bare uint8) prevents accidental arithmetic on kind values
// and makes switch cases self-documenting.
type AttrKind uint8

const (
    KindAny     AttrKind = 0 // zero value; Value any field is populated
    KindStr     AttrKind = 1
    KindInt     AttrKind = 2
    KindUint    AttrKind = 3
    KindFloat64 AttrKind = 4
    KindBool    AttrKind = 5
)

// LogAttrKey and LogAttrValue are unchanged type aliases.
type LogAttrKey   string
type LogAttrValue any

// LogAttr is the structured log field type.
//
// Zero-value invariant: a struct literal {Key: "k", Value: v} produces
// Kind=KindAny (zero), routing through the existing AppendField type-switch.
// No existing call site requires modification.
//
// Typed construction uses the package-level constructors (Str, Int, Uint,
// Float64, Bool) which are the only safe way to set Kind + the corresponding
// value field simultaneously.
type LogAttr struct {
    Key   LogAttrKey
    Value LogAttrValue // populated when Kind == KindAny; zero otherwise
    Kind  AttrKind     // discriminant; 0 == KindAny is the backward-compat default

    // Unexported typed value fields. Setting these fields directly from
    // outside the package is intentionally prevented to avoid inconsistent
    // Kind/value combinations.
    strVal   string
    intVal   int64
    uintVal  uint64
    floatVal float64
    boolVal  bool
}

// Kind accessor — returns the discriminant.
func (a LogAttr) AttrKind() AttrKind { return a.Kind }

// Typed value accessors. Callers should check Kind before calling.
func (a LogAttr) StrVal()   string  { return a.strVal }
func (a LogAttr) IntVal()   int64   { return a.intVal }
func (a LogAttr) UintVal()  uint64  { return a.uintVal }
func (a LogAttr) FloatVal() float64 { return a.floatVal }
func (a LogAttr) BoolVal()  bool    { return a.boolVal }

// Package-level constructors. Each sets Kind and exactly one typed field;
// Value (any) is left as nil to avoid boxing.

func Str(key string, val string) LogAttr {
    return LogAttr{Key: LogAttrKey(key), Kind: KindStr, strVal: val}
}

func Int(key string, val int64) LogAttr {
    return LogAttr{Key: LogAttrKey(key), Kind: KindInt, intVal: val}
}

func Uint(key string, val uint64) LogAttr {
    return LogAttr{Key: LogAttrKey(key), Kind: KindUint, uintVal: val}
}

func Float64(key string, val float64) LogAttr {
    return LogAttr{Key: LogAttrKey(key), Kind: KindFloat64, floatVal: val}
}

func Bool(key string, val bool) LogAttr {
    return LogAttr{Key: LogAttrKey(key), Kind: KindBool, boolVal: val}
}
```

Struct size on 64-bit: Key (16B string header) + Value (16B iface) + Kind (1B + 7B pad) + strVal (16B) + intVal (8B) + uintVal (8B) + floatVal (8B) + boolVal (1B + 7B pad) = ~81 B. The variadic slice stack-allocates for ≤ 2 fields (compiler vet-confirmed). No new heap allocation is introduced on the filtered path because the typed constructors do not box.

### 3.2 AppendAttr — config/parse_field.go

```go
// AppendAttr appends a JSON key-value fragment for attr to dst using
// kind-based dispatch. For Kind != KindAny, no interface boxing occurs.
// For KindAny, delegates to AppendField (the existing type-switch path)
// for backward compatibility.
//
// Called from entry.logWithSkip in the variadic fields loop.
// Must remain allocation-free for all typed kinds (verified by bench).
func AppendAttr(dst []byte, attr model.LogAttr) []byte {
    key := string(attr.Key)
    switch attr.Kind {
    case model.KindStr:
        dst = appendJSONString(dst, []byte(key))
        dst = append(dst, ':')
        dst = appendJSONString(dst, []byte(attr.StrVal()))
    case model.KindInt:
        dst = appendJSONString(dst, []byte(key))
        dst = append(dst, ':')
        dst = strconv.AppendInt(dst, attr.IntVal(), 10)
    case model.KindUint:
        dst = appendJSONString(dst, []byte(key))
        dst = append(dst, ':')
        dst = strconv.AppendUint(dst, attr.UintVal(), 10)
    case model.KindFloat64:
        dst = appendJSONString(dst, []byte(key))
        dst = append(dst, ':')
        f := attr.FloatVal()
        if math.IsNaN(f) || math.IsInf(f, 0) {
            dst = append(dst, "null"...)
        } else {
            dst = strconv.AppendFloat(dst, f, 'f', -1, 64)
        }
    case model.KindBool:
        dst = appendJSONString(dst, []byte(key))
        dst = append(dst, ':')
        dst = strconv.AppendBool(dst, attr.BoolVal())
    default: // KindAny
        dst = AppendField(dst, key, attr.Value)
    }
    return dst
}
```

`AppendAttr` lives in `config/parse_field.go` alongside `AppendField` to avoid an import cycle (`entry` imports `config`; `model` has no imports; `config` may import `model`).

### 3.3 LogEntry typed field methods — entry/entry.go

```go
// Str appends a string field directly to e.buf without interface boxing.
// Returns e to enable method chaining. Must be called before a level method.
func (e *LogEntry) Str(key string, val string) *LogEntry {
    e.buf = append(e.buf, ',')
    e.buf = config.AppendAttr(e.buf, model.Str(key, val))
    return e
}

// Int appends an int64 field directly to e.buf without interface boxing.
func (e *LogEntry) Int(key string, val int64) *LogEntry {
    e.buf = append(e.buf, ',')
    e.buf = config.AppendAttr(e.buf, model.Int(key, val))
    return e
}

// Uint appends a uint64 field directly to e.buf without interface boxing.
func (e *LogEntry) Uint(key string, val uint64) *LogEntry {
    e.buf = append(e.buf, ',')
    e.buf = config.AppendAttr(e.buf, model.Uint(key, val))
    return e
}

// Bool appends a bool field directly to e.buf without interface boxing.
func (e *LogEntry) Bool(key string, val bool) *LogEntry {
    e.buf = append(e.buf, ',')
    e.buf = config.AppendAttr(e.buf, model.Bool(key, val))
    return e
}

// Float64 appends a float64 field directly to e.buf without interface boxing.
func (e *LogEntry) Float64(key string, val float64) *LogEntry {
    e.buf = append(e.buf, ',')
    e.buf = config.AppendAttr(e.buf, model.Float64(key, val))
    return e
}
```

These methods write into `e.buf` before the level method is called. The `logWithSkip` field dispatch loop processes the variadic `fields ...model.LogAttr` separately via `AppendAttr` — the typed methods and the variadic path are independent and both zero-alloc for typed kinds.

### 3.4 Field dispatch loop in logWithSkip (P2 update)

Replace the existing loop (entry.go:169–178):

```go
// Before (v1.0.0):
for _, field := range fields {
    key := string(field.Key)
    if _, ok := snap.DefaultFields[enum.DefaultLogKey(field.Key)]; ok {
        key = config.DefaultPrefix + key
    }
    e.buf = append(e.buf, ',')
    e.buf = config.AppendField(e.buf, key, field.Value)
}

// After (P2):
for _, field := range fields {
    e.buf = append(e.buf, ',')
    if field.Kind != model.KindAny {
        // Fast path: typed field — no interface boxing, 0 allocs.
        // Key collision check is skipped for typed fields: typed field
        // constructors (model.Str etc.) do not use DefaultLogKey names
        // by convention. If a caller constructs model.Str("time", ...) they
        // accept the collision — this is consistent with the opt-in design.
        e.buf = config.AppendAttr(e.buf, field)
    } else {
        // Legacy path: KindAny — check collision then dispatch via type-switch.
        key := string(field.Key)
        if _, ok := snap.DefaultFields[enum.DefaultLogKey(field.Key)]; ok {
            key = config.DefaultPrefix + key
        }
        e.buf = config.AppendField(e.buf, key, field.Value)
    }
}
```

The key collision check is retained on the `KindAny` path for full backward compatibility. Typed paths intentionally skip it (documented above).

---

## 4. Component C — config and entry packages: Context appender (P3)

### 4.1 validateAndAppendField — internal variant (AQ-4 resolution)

```go
// validateAndAppendField is the internal variant of ValidateandParseLogField
// used by the legacy context parser path in setLogContextFields.
//
// It accepts a pre-snapshotted restrictedSet so no configMu.RLock is
// acquired per field — eliminating the N extra lock acquisitions that
// the legacy path previously paid for N context fields.
//
// The exported ValidateandParseLogField retains its own RLock for
// external callers.
func validateAndAppendField(dst []byte, key string, value any, restrictedSet map[string]struct{}) []byte {
    if _, restricted := restrictedSet[key]; restricted {
        key = DefaultPrefix + key
    }
    return AppendField(dst, key, value)
}
```

### 4.2 setLogContextFields restructure — entry/entry.go

The function signature changes from returning `[]string` to writing directly to `e.buf`:

```go
// appendContextFields appends per-request context fields to buf and returns
// the extended slice. It is called from logWithSkip after the config snapshot
// is taken; it uses snap to avoid any additional lock acquisitions.
//
// Priority (documented in config.SetContextFieldsAppender godoc):
//   1. If snap.ContextAppender != nil: call appender(ctx, buf) — 0 allocs.
//   2. Else if snap.ContextParser != nil: call parser, iterate map — allocates.
//   3. Otherwise: return buf unchanged.
func appendContextFields(ctx context.Context, buf []byte, snap config.HotSnapshot) []byte {
    if ctx == nil {
        return buf
    }
    if snap.ContextAppender != nil {
        // Zero-alloc fast path: appender writes directly to buf.
        return snap.ContextAppender(ctx, buf)
    }
    if snap.ContextParser != nil {
        // Legacy path: allocates map[string]any and iterates.
        // Uses pre-snapshotted RestrictedFields to avoid per-field RLock.
        for key, value := range snap.ContextParser(ctx) {
            buf = append(buf, ',')
            buf = config.validateAndAppendField(buf, string(key), value, snap.RestrictedFields)
        }
    }
    return buf
}
```

`appendContextFields` is a package-level function in `entry/entry.go` (not a method on `LogEntry`) to make it independently testable without a pool entry.

`setLogContextFields` (the old method) is deleted. All call sites in `logWithSkip` are replaced with the `appendContextFields` call.

### 4.3 logWithSkip context call site

```go
// P3: replace the old ctxData := e.setLogContextFields(ctx) + loop
e.buf = appendContextFields(ctx, e.buf, snap)
```

This replaces both the `ctxData := ...` assignment and the subsequent `for _, d := range ctxData` loop with a single append call.

### 4.4 Concurrency invariants for Component C

| Invariant | Enforcement |
|---|---|
| Appender path: 0 map allocs, 0 RLocks | Appender is a function pointer in the snapshot; snapshot is already held |
| Legacy path: N fields, 0 extra RLocks | `validateAndAppendField` accepts the already-snapshotted `restrictedSet` |
| Priority: appender wins | `if snap.ContextAppender != nil` is checked first unconditionally |
| Both registered — parser not called | Enforced by the `if/else if` structure; verified by test |

---

## 5. Component D — encoder and entry packages: Inline framing + ownership transfer (P4)

### 5.1 Encoder interface extension — encoder/encoder.go

```go
// Encoder formats a pre-rendered log body and appends it to dst.
//
// OpenBytes returns the opening frame delimiter bytes for this encoder.
// For JSON this is []byte{'{'}, for Text this is nil.
// The returned slice is constant after encoder construction; callers
// may cache it.
//
// CloseBytes returns the closing frame delimiter bytes including any
// trailing newline. For JSON this is []byte{'}', '\n'}, for Text
// this is []byte{'\n'}.
// The returned slice is constant after encoder construction; callers
// may cache it.
//
// Append is retained for backward compatibility; it is no longer called
// on the hot path after P4.
//
// BREAKING CHANGE: external Encoder implementations must add OpenBytes
// and CloseBytes or compilation fails. Provide NoopFramer embed to ease
// migration.
type Encoder interface {
    Append(dst, body []byte) []byte
    Name() string
    OpenBytes() []byte
    CloseBytes() []byte
}

// NoopFramer is an embeddable struct that provides default OpenBytes and
// CloseBytes implementations for external Encoder implementations that
// do not produce structured framing. Embed it to satisfy the extended
// interface with minimal change:
//
//   type MyEncoder struct {
//       encoder.NoopFramer
//       // ...
//   }
//
// NoopFramer.OpenBytes() returns nil; NoopFramer.CloseBytes() returns
// []byte{'\n'}.
type NoopFramer struct{}

func (NoopFramer) OpenBytes() []byte  { return nil }
func (NoopFramer) CloseBytes() []byte { return []byte{'\n'} }
```

### 5.2 JSONEncoder additions — encoder/json_encoder.go

```go
// compile-time proof that JSONEncoder satisfies the extended Encoder interface.
var _ Encoder = (*JSONEncoder)(nil)

// jsonOpen and jsonClose are package-level constant slices returned by
// OpenBytes/CloseBytes. Package-level vars avoid allocating a new slice
// on each call; the caller caches them in the HotSnapshot anyway.
var (
    jsonOpen  = []byte{'{'}
    jsonClose = []byte{'}', '\n'}
)

func (e *JSONEncoder) OpenBytes() []byte  { return jsonOpen }
func (e *JSONEncoder) CloseBytes() []byte { return jsonClose }
```

### 5.3 TextEncoder additions — encoder/text_encoder.go

```go
var _ Encoder = (*TextEncoder)(nil)

var textClose = []byte{'\n'}

func (e *TextEncoder) OpenBytes() []byte  { return nil }
func (e *TextEncoder) CloseBytes() []byte { return textClose }
```

### 5.4 logWithSkip framing + PublishLog changes — entry/entry.go

The framing sequence in `logWithSkip` after P4:

```go
// Step 5: reset buf and prepend open frame
e.buf = e.buf[:0]
e.buf = append(e.buf, snap.EncoderOpen...)

// Steps 6–9: append fields as before (time/level/msg, typed fields,
//            context fields, error, caller, static) — unchanged content,
//            but snap.Rendered, snap.DefaultFields etc. come from HotSnapshot

// Step 10: append close frame
e.buf = append(e.buf, snap.EncoderClose...)

// Step 11: channel send — transfer ownership of backing array
config.PublishLog(level, e.buf)

// Step 12: sever pool alias — MUST happen before e.Put()
e.buf = make([]byte, 0, initBufCap)

// Step 13: return to pool — sees the fresh backing slice, not the transferred one
e.Put()
```

`PublishLog` after P4 — remove the defensive copy:

```go
// PublishLog sends the event onto the dispatch channel. After P4, Data
// is already a privately-owned buffer whose backing array was transferred
// by entry.logWithSkip immediately after calling this function. The
// defensive copy (append([]byte(nil), Data...)) is removed; the caller
// severs the alias via make([]byte, 0, initBufCap) before e.Put().
func PublishLog(Level enum.LogLevel, Data []byte) {
    configMu.RLock()
    currentCh := ch
    configMu.RUnlock()
    currentCh <- LogEvent{Level: Level, Data: Data}
}
```

### 5.5 Ownership transfer invariant (race safety)

The sequence is:
1. `ch <- LogEvent{..., Data: e.buf}` — channel send copies the slice header (pointer, len, cap). Both `ch` item and `e.buf` now point to the same backing array.
2. `e.buf = make([]byte, 0, initBufCap)` — `e.buf` now points to a fresh backing array. The channel item's `Data` still points to the old array.
3. `e.Put()` — the pool slot gets `e` with the fresh `e.buf`. `reset()` retains `e.buf[:0]` (the fresh slice).
4. `ProcessLogEvent` goroutine: `event.Data` points to the old array, which has no concurrent writer after step 1. The Go memory model guarantees the channel send happens-before the receive, so the receiver observes the fully-written byte content.

This is correct and is not a data race. The race detector (`go test -race`) confirms because: (a) the only writer of the old array is the sender goroutine, which finishes writing before the send; (b) the receiver goroutine only reads after the receive; (c) `e.buf = make(...)` does not write to the old array.

`reset()` in `entry/pool.go` is unchanged; it retains `e.buf[:0]` which is now the fresh slice:

```go
func (e *LogEntry) reset() {
    retained := e.buf[:0]  // retains fresh backing array after P4
    *e = LogEntry{}
    e.buf = retained
}
```

No modification needed to `pool.go`. The existing `reset()` logic is already correct because `e.buf` points to the fresh slice at the time `e.Put()` is called.

---

## 6. Component E — config package: Channel capacity (P5)

### 6.1 channelCapacity package-level variable

```go
// channelCapacity is the buffer size used by resetConfig when creating
// the dispatch channel. It is consumed at init() time only; calls to
// SetChannelCapacity after the package has been initialised have no effect.
//
// Why package-level: resetConfig creates the *Config and channel together;
// a field on Config would require reading from an object being constructed.
// This is consistent with DafaultLevel, DafaultEncoderType, DefaultAddSource.
var channelCapacity int = 1000
```

`DafaultLogBuffer` is updated from `20` to `1000` for consistency with package documentation and external callers that read it, but `resetConfig` now reads `channelCapacity` (not `DafaultLogBuffer`) to construct the channel.

### 6.2 SetChannelCapacity

```go
// SetChannelCapacity sets the dispatch channel buffer size used by the
// next resetConfig call. In production, resetConfig is called exactly
// once from init(); this function must therefore be called before any
// log call to take effect.
//
// Values <= 0 are clamped to 1000 and a log.Printf warning is emitted.
// Values > 100_000 are clamped to 100_000 and a log.Printf warning is emitted.
// No panic is ever raised.
func SetChannelCapacity(n int) {
    const (
        minCap = 1000
        maxCap = 100_000
    )
    if n <= 0 {
        log.Printf("lognugget: SetChannelCapacity(%d): value <= 0, clamped to %d", n, minCap)
        n = minCap
    } else if n > maxCap {
        log.Printf("lognugget: SetChannelCapacity(%d): value > %d, clamped to %d", n, maxCap, maxCap)
        n = maxCap
    }
    channelCapacity = n
}
```

`SetChannelCapacity` does not acquire `configMu` because `channelCapacity` is not a `Config` field and is not accessed concurrently once the package is initialised. It is written before `init()` fires (called from program `init()` ordering) and read inside `resetConfig` which runs as part of this package's own `init()`. The Go specification guarantees that a package's `init()` functions run after all variable initialisation in the package; calling `SetChannelCapacity` from a downstream package's `init()` that runs before `lognugget`'s `init()` will be effective.

### 6.3 resetConfig channel construction

```go
newCh := make(chan LogEvent, channelCapacity)  // was: make(chan LogEvent, 10)
```

---

## 7. Request Lifecycle End-to-End (all five stories combined)

```
Caller goroutine
│
├─ NewLogEntry()
│   entryPool.Get() → e.reset() → e.buf[:0] retains warm backing array
│
├─ [optional] e.Str("k","v").Int("n",42)...
│   → each appends comma + key:val to e.buf; 0 allocs (typed paths)
│
└─ e.Info(ctx, "msg", model.LogAttr{Key:"legacy", Value:true})
    │
    └─ logWithSkip(LevelInfo, ctx, "msg", nil, 2, fields...)
        │
        ├─ STEP 1: level gate
        │   GetAtomicMinLevel() → atomic.Load → enum.LogLevel(int32)
        │   if level < min: e.Put(); return   ← 0 allocs, 0 locks, ~5 ns
        │
        ├─ STEP 2: pre-processor nil check
        │   if EventPreProcessors == nil: e.Put(); return
        │
        ├─ STEP 3: config snapshot
        │   snap := config.GetHotSnapshot()   ← 1 RLock, pointer copy, ~50 ns
        │
        ├─ STEP 4: [optional] source capture
        │   if snap.AddSource: runtime.Caller(skip) → e.caller
        │
        ├─ STEP 5: open frame
        │   e.buf = e.buf[:0]
        │   e.buf = append(e.buf, snap.EncoderOpen...)
        │   [JSON: '{'; Text: no-op]
        │
        ├─ STEP 6: mandatory fields
        │   e.buf = append(e.buf, snap.Rendered[Time]...)
        │   e.buf = config.AppendQuotedString(e.buf, time)
        │   e.buf = append(e.buf, ',', snap.Rendered[Level]...)
        │   e.buf = config.AppendQuotedString(e.buf, level.String())
        │   e.buf = append(e.buf, ',', snap.Rendered[Message]...)
        │   e.buf = config.AppendQuotedString(e.buf, msg)
        │
        ├─ STEP 7: variadic fields
        │   for each field: append comma
        │   ├─ Kind != KindAny: config.AppendAttr → direct typed append, 0 allocs
        │   └─ Kind == KindAny: collision check + config.AppendField type-switch
        │
        ├─ STEP 8: context fields
        │   e.buf = appendContextFields(ctx, e.buf, snap)
        │   ├─ snap.ContextAppender != nil: appender(ctx, e.buf) — 0 allocs
        │   └─ snap.ContextParser != nil: iterate map[string]any — allocates
        │
        ├─ STEP 9: optional fields
        │   [err != nil] append comma + error field
        │   [e.caller != nil] append comma + caller field
        │   [snap.StaticFields != ""] append comma + static fields
        │
        ├─ STEP 10: close frame
        │   e.buf = append(e.buf, snap.EncoderClose...)
        │   [JSON: "}\n"; Text: "\n"]
        │
        ├─ STEP 11: channel send (ownership transfer)
        │   config.PublishLog(level, e.buf)
        │   → currentCh <- LogEvent{Level: level, Data: e.buf}
        │   [non-blocking at cap=1000 under typical load]
        │
        ├─ STEP 12: sever pool alias
        │   e.buf = make([]byte, 0, initBufCap)   ← fresh backing array
        │
        └─ STEP 13: return to pool
            e.Put() → entryPool.Put(e)
            reset() retains e.buf[:0] (the fresh slice)

Background goroutine (ProcessLogEvent):
└─ for event := range currentCh:
     snap processors under RLock
     for each observer: observer.PreProcess(event.Level, event.Data)
     [event.Data is the old backing array; no concurrent writer]
```

---

## 8. Error Handling Strategy

| Scenario | Handling |
|---|---|
| `runtime.Caller(skip)` returns `ok=false` | `e.caller = &runtime.Frame{Function: "unknown"}` (unchanged from v1.0.0) |
| `Encoder.OpenBytes()` / `CloseBytes()` returns nil | `append(e.buf, nil...)` is a no-op; safe |
| `ContextFieldsAppender` panics | Propagates to caller goroutine — same as if the appender were called directly. Not recovered; consistent with Go convention for user-supplied callbacks |
| Channel send blocks (capacity full) | Caller goroutine blocks on `ch <-`; this is by design. Not recovered |
| `model.Float64` with NaN/Inf | `AppendAttr` KindFloat64 branch emits `null` — same rule as `AppendField` |
| `SetChannelCapacity` with out-of-range value | Clamped + `log.Printf` warning; no panic |

---

## 9. Concurrency Summary

| Path | Locks acquired | Atomics |
|---|---|---|
| Filtered path (level gate) | 0 | 1 `atomic.Int64.Load` |
| Hot path (full encoding) | 2 `configMu.RLock` total: 1 in `GetHotSnapshot()`, 1 in `PublishLog()` to snapshot `ch` | 1 `atomic.Int64.Load` |
| `SetMinLevel` | 1 `configMu.Lock` | 1 `atomic.Int64.Store` (under the lock) |
| `resetConfig` | 1 `configMu.Lock` | 1 `atomic.Int64.Store` (under the lock) |
| `ProcessLogEvent` loop | 1 `configMu.RLock` per event (processor snapshot) | 0 |
| `PublishLog` (after P4) | 1 `configMu.RLock` (ch pointer snapshot) | 0 |
| All other `Set*` functions | 1 `configMu.Lock` each | 0 |

The reduction from 6 `configMu.RLock` to 2 per hot-path call is the key P1 saving (~150–250 ns at GOMAXPROCS=8 under contention). The second RLock in `PublishLog` cannot be eliminated by moving `ch` into `HotSnapshot` because `ch` is replaced wholesale by `resetConfig` — snapshotting the pointer without a lock would race with that replacement.

---

## 10. Testing Strategy

### 10.1 Unit tests (correctness)

| File | Coverage |
|---|---|
| `config/atomic_level_test.go` | `GetAtomicMinLevel()` races with `SetMinLevel` under GOMAXPROCS=8; at most one event mis-level-gated |
| `config/hot_snapshot_test.go` | `GetHotSnapshot()` returns consistent fields; `snap.EncoderOpen` matches `enc.OpenBytes()` |
| `model/attr_test.go` | `model.Str/Int/Uint/Float64/Bool` constructors set correct Kind + value; `KindAny` zero-value invariant |
| `config/parse_field_test.go` | `AppendAttr` for each kind + edge cases (NaN, empty string, empty key) |
| `config/ctx_appender_test.go` | Appender-wins when both registered; appender path produces same JSON as parser path for same keys; `nil` ctx is safe |
| `encoder/framing_test.go` | `JSONEncoder.OpenBytes()` + `CloseBytes()` round-trip; `TextEncoder` same; `NoopFramer` default impls |
| `entry/ownership_transfer_test.go` | With `-race`: send buf to channel, then `make(...)`, then `e.Put()` — race detector clean |
| `config/channel_capacity_test.go` | `SetChannelCapacity(0)` → 1000; `SetChannelCapacity(200000)` → 100000; log warning emitted |

### 10.2 Benchmark files

All benchmarks call `b.ReportAllocs()` and are gated by `./scripts/bench-check.sh` (<=1000 ns/op).

| File | Key benchmarks | Acceptance targets |
|---|---|---|
| `entry/atomic_snapshot_bench_test.go` | `BenchmarkLogEntry_FilteredPath`, `BenchmarkLogEntry_HotPath_NoCtx` | Filtered: 0 allocs, ≤35 ns/op; hot: 1 RLock verifiable via `sync.Mutex` contention metric |
| `entry/typed_fields_bench_test.go` | `BenchmarkLogEntry_TypedFields_10`, `BenchmarkLogEntry_LegacyAttrs_10` | Typed: 0 allocs for field appends; legacy: unchanged alloc count |
| `entry/ctx_appender_bench_test.go` | `BenchmarkLogEntry_CtxAppender_10`, `BenchmarkLogEntry_CtxParser_10` | Appender: ≥20 fewer allocs/op than parser path |
| `entry/inline_framing_bench_test.go` | `BenchmarkLogEntry_InlineFraming_10` | allocs/op drops by ≥2 vs P3 baseline (RC-4 savings) |
| `entry/parallel_bench_test.go` | `BenchmarkLognugget_Parallel_10CtxFields` (GOMAXPROCS=8) | ≤1000 ns/op, ≥3 M ops/sec |

### 10.3 Race detector

Every story PR runs `go test -race ./...`. P4 specifically requires a race-clean benchmark run: `go test -race -bench=BenchmarkLogEntry_InlineFraming_10 -benchmem -count=3 ./entry/`.

### 10.4 Backward compatibility tests

Existing tests in `entry/ctx_fields_test.go`, `entry/levels_test.go`, and `config/parsers_test.go` must pass without modification. These serve as the regression suite for `KindAny` zero-value behaviour, `SetContextFieldsParser` legacy path, and existing `LogAttr` struct-literal call sites.

---

## 11. HLD Open Questions — LLD Resolutions

| AQ | Resolution in LLD |
|---|---|
| AQ-1: `Kind` field type | Named type `AttrKind uint8`; constants `KindAny=0..KindBool=5`. See §3.1. |
| AQ-2: typed value fields exported or not | Unexported (`strVal`, `intVal`, `uintVal`, `floatVal`, `boolVal`). Package-level constructors (`model.Str`, etc.) are the only construction path. See §3.1. |
| AQ-3: fresh `make` capacity | `initBufCap` (1024). See §5.4. `reset()` in pool.go unchanged. |
| AQ-4: internal variant of `ValidateandParseLogField` | `validateAndAppendField(dst []byte, key string, value any, restrictedSet map[string]struct{}) []byte` in `config/parse_field.go`. Used by the legacy P3 path only. See §4.1. |
