# Epic V2 Performance — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close the 31× throughput gap vs zerolog by eliminating the 5 root-cause allocations/locks in the hot log path, getting from ~2,000 ns/op → < 1,000 ns/op.

**Architecture:** Five independent stories attack five root causes in priority order: (1) replace 6× RLocks with one atomic + one snapshot, (2) eliminate `interface{}` boxing with typed field constructors, (3) replace `map[string]any` context API with a direct-append API, (4) eliminate the encoder double-buffer by inline-framing into `e.buf`, (5) raise channel capacity from 10 → 1,000 to stop blocking under parallel load.

**Tech Stack:** Go 1.21+, `sync/atomic`, existing `config/`, `entry/`, `model/`, `encoder/` packages. No new dependencies.

---

## Branch map

| Story | Issue | Branch | Target |
|-------|-------|--------|--------|
| P1 | #82 | `feat/82-p1-atomic-min-level` | `feat/81-v2-performance` |
| P2 | #83 | `feat/83-p2-typed-field-api` | `feat/81-v2-performance` |
| P3 | #84 | `feat/84-p3-ctx-append-buf` | `feat/81-v2-performance` |
| P4 | #85 | `feat/85-p4-inline-framing` | `feat/81-v2-performance` |
| P5 | #86 | `feat/86-p5-channel-capacity` | `feat/81-v2-performance` |

Each task is self-contained; implement P1 → P2 → P3 → P4 → P5 in order (later tasks build on earlier snapshots).

---

## Task 1 (P1): Atomic minLevel + single config snapshot

**Issue:** #82  
**Branch:** `feat/82-p1-atomic-min-level`

**Root cause:** `entry.logWithSkip` calls `config.GetConfig()` twice and then calls 5 individual methods on the returned `*Config`, each method taking its own `configMu.RLock/RUnlock`. Total: ~6+ RLock/RUnlock pairs per hot-path call. Fix: (a) atomic int64 for the level gate (zero locks on filtered path), (b) one `GetHotSnapshot()` that takes a single RLock and returns all fields needed by the hot path.

**Files:**
- Modify: `config/config.go`
- Modify: `entry/entry.go`
- Create: `config/hot_snapshot_test.go`
- Modify: `entry/level_gate_test.go` (verify 0 allocs on filtered path still holds)

### Step 1.1: Checkout story branch

```bash
git checkout feat/82-p1-atomic-min-level
```

### Step 1.2: Write failing test — verify snapshot returns consistent values

- [ ] Create `config/hot_snapshot_test.go`:

```go
//go:build testing

package config_test

import (
	"testing"

	"github.com/architagr/lognugget/config"
	"github.com/architagr/lognugget/enum"
)

func Test_GetHotSnapshot_ReflectsCurrentConfig(t *testing.T) {
	config.TestResetConfig()

	config.SetMinLevel(enum.LevelWarn)
	config.SetAddSource(true)

	snap := config.GetHotSnapshot()

	if snap.AddSource != true {
		t.Errorf("AddSource = %v, want true", snap.AddSource)
	}
	if snap.Encoder == nil {
		t.Error("Encoder should not be nil")
	}
	if snap.Rendered == nil {
		t.Error("Rendered should not be nil")
	}
}

func Test_AtomicMinLevel_ReflectsSetMinLevel(t *testing.T) {
	config.TestResetConfig()
	config.SetMinLevel(enum.LevelWarn)
	got := config.GetAtomicMinLevel()
	if got != enum.LevelWarn {
		t.Errorf("GetAtomicMinLevel() = %v, want %v", got, enum.LevelWarn)
	}
}
```

### Step 1.3: Run test to confirm it fails

```bash
go test -tags testing -run "Test_GetHotSnapshot|Test_AtomicMinLevel" ./config/ -v
```
Expected: FAIL — `config.GetHotSnapshot` and `config.GetAtomicMinLevel` undefined.

### Step 1.4: Add `HotSnapshot` struct and `atomicMinLevel` to `config/config.go`

- [ ] Add after the `var (defaultConfig ...configMu sync.RWMutex)` block (around line 87):

```go
// atomicMinLevel is a lock-free copy of defaultConfig.minLevel.
// Updated by resetConfig and SetMinLevel under configMu write lock.
// Read in the hot path via GetAtomicMinLevel — zero allocations, zero locks.
var atomicMinLevel atomic.Int64
```

Add `"sync/atomic"` to imports.

- [ ] Add `HotSnapshot` struct and `GetHotSnapshot` function at the end of `config/config.go`:

```go
// HotSnapshot holds all config fields read on the hot log path.
// Obtained via GetHotSnapshot — one RLock for all fields.
type HotSnapshot struct {
	AddSource     bool
	TimeFormat    string
	DefaultFields map[enum.DefaultLogKey]string
	Rendered      map[enum.DefaultLogKey][]byte
	StaticFields  string
	ContextParser ContextFieldsParser
	Encoder       encoder.Encoder
	EncoderType   enum.LogEncodeType
}

// GetHotSnapshot snapshots all hot-path config fields under a single RLock.
func GetHotSnapshot() HotSnapshot {
	configMu.RLock()
	defer configMu.RUnlock()
	return HotSnapshot{
		AddSource:     defaultConfig.addSource,
		TimeFormat:    defaultConfig.timeFormat,
		DefaultFields: defaultConfig.defaultFields,
		Rendered:      defaultConfig.defaultFieldsRendered,
		StaticFields:  defaultConfig.parsedStaticFields,
		ContextParser: defaultConfig.contextParser,
		Encoder:       defaultConfig.encoderObj,
		EncoderType:   defaultConfig.encoderType,
	}
}

// GetAtomicMinLevel reads minLevel with zero locks (atomic load).
func GetAtomicMinLevel() enum.LogLevel {
	return enum.LogLevel(atomicMinLevel.Load())
}
```

- [ ] In `resetConfig`, after setting `defaultConfig`, add:

```go
atomicMinLevel.Store(int64(newCfg.minLevel))
```

(Place this inside the `configMu.Lock()` block, after `defaultConfig = newCfg`.)

- [ ] In `SetMinLevel`, after `defaultConfig.minLevel = level`, add:

```go
atomicMinLevel.Store(int64(level))
```

### Step 1.5: Run tests to verify snapshot tests pass

```bash
go test -tags testing -run "Test_GetHotSnapshot|Test_AtomicMinLevel" ./config/ -v
```
Expected: PASS.

### Step 1.6: Write failing test — verify level gate uses zero locks (alloc test)

The existing `Test_LogEntry_FilteredPath_ZeroAlloc` in `entry/level_gate_test.go` already covers this. Run it first to confirm it still passes after upcoming entry.go changes.

### Step 1.7: Rewrite `entry/entry.go` `logWithSkip` to use atomic gate + single snapshot

- [ ] Replace the existing `logWithSkip` body (`entry/entry.go` lines 113–212) with:

```go
func (e *LogEntry) logWithSkip(level enum.LogLevel, ctx context.Context, message string, err error, skip int, fields ...model.LogAttr) {
	// Lock-free level gate — atomic load, zero allocations.
	if config.GetAtomicMinLevel() > level {
		e.Put()
		return
	}
	if config.EventPreProcessors == nil {
		e.Put()
		return
	}

	// Single RLock: capture all hot-path config fields at once.
	snap := config.GetHotSnapshot()

	if snap.AddSource {
		if pc, _, _, ok := runtime.Caller(skip); ok {
			fn := runtime.FuncForPC(pc)
			frame := &runtime.Frame{}
			if fn != nil {
				frame.Function = fn.Name()
			}
			e.caller = frame
		} else {
			e.caller = &runtime.Frame{Function: "unknown"}
		}
	}

	e.buf = e.buf[:0]

	e.buf = append(e.buf, snap.Rendered[enum.DefaultLogKeyTime]...)
	e.buf = config.AppendQuotedString(e.buf, customTime.Format(customTime.TimeNow(), snap.TimeFormat))
	e.buf = append(e.buf, ',')
	e.buf = append(e.buf, snap.Rendered[enum.DefaultLogKeyLevel]...)
	e.buf = config.AppendQuotedString(e.buf, level.String())
	e.buf = append(e.buf, ',')
	e.buf = append(e.buf, snap.Rendered[enum.DefaultLogKeyMessage]...)
	e.buf = config.AppendQuotedString(e.buf, message)

	for _, field := range fields {
		key := string(field.Key)
		if _, ok := snap.DefaultFields[enum.DefaultLogKey(field.Key)]; ok {
			key = config.DefaultPrefix + key
		}
		e.buf = append(e.buf, ',')
		e.buf = config.AppendField(e.buf, key, field.Value)
	}

	if ctx != nil && snap.ContextParser != nil {
		for key, value := range snap.ContextParser(ctx) {
			e.buf = append(e.buf, ',')
			e.buf = config.AppendField(e.buf, config.ValidateandParseContextKey(string(key)), value)
		}
	}

	if err != nil {
		e.buf = append(e.buf, ',')
		e.buf = append(e.buf, snap.Rendered[enum.DefaultLogKeyError]...)
		e.buf = config.AppendQuotedString(e.buf, err.Error())
	}
	if e.caller != nil {
		e.buf = append(e.buf, ',')
		e.buf = append(e.buf, snap.Rendered[enum.DefaultLogKeyCaller]...)
		e.buf = config.AppendQuotedString(e.buf, e.caller.Function)
	}
	if snap.StaticFields != "" {
		e.buf = append(e.buf, ',')
		e.buf = append(e.buf, snap.StaticFields...)
	}

	byteData := snap.Encoder.Append(nil, e.buf)
	config.PublishLog(level, byteData)

	e.Put()
}
```

**Note:** `setLogContextFields` is now inlined. Delete the `setLogContextFields` method from `entry.go`.

Also add `ValidateandParseContextKey` to `config/config.go` (or reuse `ValidateandParseLogField`):

```go
// ValidateandParseContextKey is like ValidateandParseLogField but returns only the
// potentially-prefixed key (no value encoding). Used by entry hot path for context fields.
// Inlines the restricted-set check; uses the snapshot-provided key directly.
func ValidateandParseContextKey(key string) string {
	configMu.RLock()
	_, restricted := restrictedFieldsSet[key]
	configMu.RUnlock()
	if restricted {
		return DefaultPrefix + key
	}
	return key
}
```

Wait — this still takes a lock. Since context field key validation is on the hot path, we should pass the restricted set through the snapshot. Update `HotSnapshot` to include `RestrictedFields map[string]struct{}` and use that in entry. No extra lock needed.

Updated `HotSnapshot` struct:

```go
type HotSnapshot struct {
	AddSource       bool
	TimeFormat      string
	DefaultFields   map[enum.DefaultLogKey]string
	Rendered        map[enum.DefaultLogKey][]byte
	StaticFields    string
	ContextParser   ContextFieldsParser
	Encoder         encoder.Encoder
	EncoderType     enum.LogEncodeType
	RestrictedFields map[string]struct{}
}
```

Updated `GetHotSnapshot`:

```go
func GetHotSnapshot() HotSnapshot {
	configMu.RLock()
	defer configMu.RUnlock()
	return HotSnapshot{
		AddSource:        defaultConfig.addSource,
		TimeFormat:       defaultConfig.timeFormat,
		DefaultFields:    defaultConfig.defaultFields,
		Rendered:         defaultConfig.defaultFieldsRendered,
		StaticFields:     defaultConfig.parsedStaticFields,
		ContextParser:    defaultConfig.contextParser,
		Encoder:          defaultConfig.encoderObj,
		EncoderType:      defaultConfig.encoderType,
		RestrictedFields: restrictedFieldsSet,
	}
}
```

Updated context key validation in entry.go (no lock):

```go
if ctx != nil && snap.ContextParser != nil {
	for key, value := range snap.ContextParser(ctx) {
		k := string(key)
		if _, restricted := snap.RestrictedFields[k]; restricted {
			k = config.DefaultPrefix + k
		}
		e.buf = append(e.buf, ',')
		e.buf = config.AppendField(e.buf, k, value)
	}
}
```

### Step 1.8: Run all tests

```bash
go test -tags testing ./... 2>&1 | tail -20
```
Expected: all PASS.

### Step 1.9: Run bench to confirm improvement

```bash
go test -bench=Benchmark_Log -benchmem -count=5 -run=^$ ./test/benchmark/ 2>&1
```
Expected: ns/op lower (should drop ~150–250 ns from fewer RLocks).

### Step 1.10: Update GitHub issue #82

```bash
./.claude/scripts/github-api.sh update-issue 82 "open" "## P1 complete\n\nChanges:\n- \`atomicMinLevel atomic.Int64\` at package level, updated by SetMinLevel + resetConfig\n- \`HotSnapshot\` struct + \`GetHotSnapshot()\` — one RLock for all hot-path fields\n- entry.go logWithSkip: atomic gate (0 locks) + single snapshot (1 RLock)\n- setLogContextFields inlined; restricted-field check uses snapshot (0 locks)\n\nBench result: (paste ns/op before/after)"
```

### Step 1.11: Commit and push

```bash
git add config/config.go config/hot_snapshot_test.go entry/entry.go
git commit -m "perf(p1): atomic minLevel gate + single hot-path config snapshot refs #82"
git push
```

### Step 1.12: Open draft PR #82 → `feat/81-v2-performance`

```bash
./.claude/scripts/github-api.sh create-draft-pr "P1: atomic minLevel gate + single config snapshot" "Fixes #82\n\nEliminate 6+ configMu.RLocks per log call:\n- atomicMinLevel: lock-free level gate (filtered path stays 0-alloc)\n- GetHotSnapshot(): single RLock captures all hot-path fields" feat/82-p1-atomic-min-level feat/81-v2-performance
```

---

## Task 2 (P2): Typed field API — Str / Int / Bool / Float64

**Issue:** #83  
**Branch:** `feat/83-p2-typed-field-api`

**Root cause:** `model.LogAttr.Value = any`. When callers write `model.LogAttr{Key: "k", Value: int(n)}`, the `int` is boxed into `interface{}`, allocating ~1 heap object per field with non-pointer-sized values. Fix: add typed `LogAttr` constructors that store values in inline struct fields (no boxing), and `AppendAttr` that dispatches on kind without `interface{}`.

**Files:**
- Modify: `model/attr.go`
- Modify: `config/parse_field.go` (add `AppendAttr`)
- Create: `model/attr_typed_test.go`
- Create: `config/append_attr_test.go`
- Modify: `entry/entry.go` (use `AppendAttr` in field loop)

### Step 2.1: Checkout story branch

```bash
git checkout feat/83-p2-typed-field-api
# cherry-pick P1 changes so this branch has the snapshot:
git merge feat/82-p1-atomic-min-level --no-ff -m "merge P1 atomic snapshot into P2 branch"
```

### Step 2.2: Write failing tests — typed constructors

- [ ] Create `model/attr_typed_test.go`:

```go
package model_test

import (
	"testing"

	"github.com/architagr/lognugget/model"
)

func Test_Str_SetsKindStr(t *testing.T) {
	a := model.Str("msg", "hello")
	if a.Key != "msg" {
		t.Errorf("Key = %q, want %q", a.Key, "msg")
	}
	if a.StrVal() != "hello" {
		t.Errorf("StrVal() = %q, want %q", a.StrVal(), "hello")
	}
	if a.Kind() != model.KindStr {
		t.Errorf("Kind() = %v, want KindStr", a.Kind())
	}
}

func Test_Int_SetsKindInt(t *testing.T) {
	a := model.Int("count", 42)
	if a.IntVal() != 42 {
		t.Errorf("IntVal() = %d, want 42", a.IntVal())
	}
	if a.Kind() != model.KindInt {
		t.Errorf("Kind() = %v, want KindInt", a.Kind())
	}
}

func Test_Bool_SetsKindBool(t *testing.T) {
	a := model.Bool("ok", true)
	if !a.BoolVal() {
		t.Error("BoolVal() = false, want true")
	}
}

func Test_Float64_SetsKindFloat(t *testing.T) {
	a := model.Float64("ratio", 3.14)
	if a.FloatVal() != 3.14 {
		t.Errorf("FloatVal() = %v, want 3.14", a.FloatVal())
	}
}

func Test_LogAttr_BackwardCompat_AnyField(t *testing.T) {
	// Existing struct literal API must still work.
	a := model.LogAttr{Key: "legacy", Value: "val"}
	if a.Kind() != model.KindAny {
		t.Errorf("Kind() = %v, want KindAny for struct literal", a.Kind())
	}
}

func Test_Str_ZeroAlloc(t *testing.T) {
	allocs := testing.AllocsPerRun(100, func() {
		_ = model.Str("key", "value")
	})
	if allocs != 0 {
		t.Errorf("Str() allocated %.0f times, want 0", allocs)
	}
}

func Test_Int_ZeroAlloc(t *testing.T) {
	allocs := testing.AllocsPerRun(100, func() {
		_ = model.Int("key", 99)
	})
	if allocs != 0 {
		t.Errorf("Int() allocated %.0f times, want 0", allocs)
	}
}
```

### Step 2.3: Run tests to confirm they fail

```bash
go test -run "Test_Str|Test_Int|Test_Bool|Test_Float64|Test_LogAttr_BackwardCompat" ./model/ -v
```
Expected: FAIL — `model.Str`, `model.Int` etc. undefined.

### Step 2.4: Extend `model/attr.go` with kind + typed fields

- [ ] Replace the entire content of `model/attr.go` with:

```go
package model

// AttrKind identifies how the value of a LogAttr is stored.
// KindAny is the legacy path (Value field, interface boxing).
// Other kinds use the inline typed fields (no boxing).
type AttrKind uint8

const (
	KindAny   AttrKind = 0
	KindStr   AttrKind = 1
	KindInt   AttrKind = 2
	KindUint  AttrKind = 3
	KindFloat AttrKind = 4
	KindBool  AttrKind = 5
)

type LogAttrKey string
type LogAttrValue any

// LogAttr is a single structured log field. Use the typed constructors
// (Str, Int, Bool, Float64) on the hot path to avoid interface{} boxing.
// The struct-literal form (LogAttr{Key: k, Value: v}) still works for
// backward compatibility and uses KindAny.
type LogAttr struct {
	Key      LogAttrKey
	Value    LogAttrValue // used when kind == KindAny
	strVal   string
	intVal   int64
	uintVal  uint64
	floatVal float64
	boolVal  bool
	kind     AttrKind
}

func (a LogAttr) Kind() AttrKind  { return a.kind }
func (a LogAttr) StrVal() string  { return a.strVal }
func (a LogAttr) IntVal() int64   { return a.intVal }
func (a LogAttr) UintVal() uint64 { return a.uintVal }
func (a LogAttr) FloatVal() float64 { return a.floatVal }
func (a LogAttr) BoolVal() bool   { return a.boolVal }

// Str returns a zero-alloc LogAttr for a string value.
func Str(key, val string) LogAttr {
	return LogAttr{Key: LogAttrKey(key), strVal: val, kind: KindStr}
}

// Int returns a zero-alloc LogAttr for an int64 value.
func Int(key string, val int64) LogAttr {
	return LogAttr{Key: LogAttrKey(key), intVal: val, kind: KindInt}
}

// Uint returns a zero-alloc LogAttr for a uint64 value.
func Uint(key string, val uint64) LogAttr {
	return LogAttr{Key: LogAttrKey(key), uintVal: val, kind: KindUint}
}

// Bool returns a zero-alloc LogAttr for a bool value.
func Bool(key string, val bool) LogAttr {
	return LogAttr{Key: LogAttrKey(key), boolVal: val, kind: KindBool}
}

// Float64 returns a zero-alloc LogAttr for a float64 value.
func Float64(key string, val float64) LogAttr {
	return LogAttr{Key: LogAttrKey(key), floatVal: val, kind: KindFloat}
}
```

### Step 2.5: Run model tests

```bash
go test -run "Test_Str|Test_Int|Test_Bool|Test_Float64|Test_LogAttr_BackwardCompat" ./model/ -v
```
Expected: PASS.

### Step 2.6: Write failing test — AppendAttr in config

- [ ] Create `config/append_attr_test.go`:

```go
package config_test

import (
	"testing"

	"github.com/architagr/lognugget/config"
	"github.com/architagr/lognugget/model"
)

func Test_AppendAttr_Str(t *testing.T) {
	got := string(config.AppendAttr(nil, model.Str("k", "hello")))
	want := `"k":"hello"`
	if got != want {
		t.Errorf("AppendAttr Str = %q, want %q", got, want)
	}
}

func Test_AppendAttr_Int(t *testing.T) {
	got := string(config.AppendAttr(nil, model.Int("n", 42)))
	want := `"n":42`
	if got != want {
		t.Errorf("AppendAttr Int = %q, want %q", got, want)
	}
}

func Test_AppendAttr_Bool(t *testing.T) {
	got := string(config.AppendAttr(nil, model.Bool("ok", true)))
	want := `"ok":true`
	if got != want {
		t.Errorf("AppendAttr Bool = %q, want %q", got, want)
	}
}

func Test_AppendAttr_Float64(t *testing.T) {
	got := string(config.AppendAttr(nil, model.Float64("r", 1.5)))
	want := `"r":1.5`
	if got != want {
		t.Errorf("AppendAttr Float64 = %q, want %q", got, want)
	}
}

func Test_AppendAttr_AnyFallback(t *testing.T) {
	got := string(config.AppendAttr(nil, model.LogAttr{Key: "x", Value: "v"}))
	want := `"x":"v"`
	if got != want {
		t.Errorf("AppendAttr Any fallback = %q, want %q", got, want)
	}
}

func Test_AppendAttr_Str_ZeroAlloc(t *testing.T) {
	attr := model.Str("key", "value")
	dst := make([]byte, 0, 64)
	allocs := testing.AllocsPerRun(100, func() {
		dst = config.AppendAttr(dst[:0], attr)
	})
	if allocs != 0 {
		t.Errorf("AppendAttr Str allocated %.0f times, want 0", allocs)
	}
}

func Test_AppendAttr_Int_ZeroAlloc(t *testing.T) {
	attr := model.Int("n", 99)
	dst := make([]byte, 0, 32)
	allocs := testing.AllocsPerRun(100, func() {
		dst = config.AppendAttr(dst[:0], attr)
	})
	if allocs != 0 {
		t.Errorf("AppendAttr Int allocated %.0f times, want 0", allocs)
	}
}
```

### Step 2.7: Run to confirm AppendAttr tests fail

```bash
go test -run "Test_AppendAttr" ./config/ -v
```
Expected: FAIL — `config.AppendAttr` undefined.

### Step 2.8: Add `AppendAttr` to `config/parse_field.go`

- [ ] Append to the end of `config/parse_field.go`:

```go
// AppendAttr appends a JSON key-value fragment for attr to dst without
// interface{} boxing for the typed kinds (KindStr, KindInt, KindUint,
// KindFloat, KindBool). Falls back to AppendField for KindAny (legacy).
func AppendAttr(dst []byte, attr model.LogAttr) []byte {
	key := string(attr.Key)
	dst = appendJSONString(dst, []byte(key))
	dst = append(dst, ':')
	switch attr.Kind() {
	case model.KindStr:
		dst = appendJSONString(dst, []byte(attr.StrVal()))
	case model.KindInt:
		dst = strconv.AppendInt(dst, attr.IntVal(), 10)
	case model.KindUint:
		dst = strconv.AppendUint(dst, attr.UintVal(), 10)
	case model.KindFloat:
		f := attr.FloatVal()
		if math.IsNaN(f) || math.IsInf(f, 0) {
			dst = append(dst, "null"...)
		} else {
			dst = strconv.AppendFloat(dst, f, 'f', -1, 64)
		}
	case model.KindBool:
		dst = strconv.AppendBool(dst, attr.BoolVal())
	default:
		// KindAny: legacy interface{} dispatch
		switch v := attr.Value.(type) {
		case string:
			dst = appendJSONString(dst, []byte(v))
		case int:
			dst = strconv.AppendInt(dst, int64(v), 10)
		case int64:
			dst = strconv.AppendInt(dst, v, 10)
		case float64:
			if math.IsNaN(v) || math.IsInf(v, 0) {
				dst = append(dst, "null"...)
			} else {
				dst = strconv.AppendFloat(dst, v, 'f', -1, 64)
			}
		case bool:
			dst = strconv.AppendBool(dst, v)
		default:
			dst = fmt.Appendf(dst, "%+v", attr.Value)
		}
	}
	return dst
}
```

Add `"github.com/architagr/lognugget/model"` to imports in `config/parse_field.go`.

### Step 2.9: Run AppendAttr tests

```bash
go test -run "Test_AppendAttr" ./config/ -v
```
Expected: PASS.

### Step 2.10: Update `entry/entry.go` field loop to use `AppendAttr`

- [ ] In `logWithSkip`, replace the fields loop:

```go
for _, field := range fields {
    key := string(field.Key)
    if _, ok := snap.DefaultFields[enum.DefaultLogKey(field.Key)]; ok {
        key = config.DefaultPrefix + key
    }
    e.buf = append(e.buf, ',')
    e.buf = config.AppendField(e.buf, key, field.Value)
}
```

With:

```go
for _, field := range fields {
    key := string(field.Key)
    if _, ok := snap.DefaultFields[enum.DefaultLogKey(field.Key)]; ok {
        key = config.DefaultPrefix + key
    }
    e.buf = append(e.buf, ',')
    if field.Kind() == model.KindAny {
        e.buf = config.AppendField(e.buf, key, field.Value)
    } else {
        // Use key-override-aware append for typed fields.
        overridden := model.LogAttr{Key: model.LogAttrKey(key)}
        switch field.Kind() {
        case model.KindStr:
            overridden = model.Str(key, field.StrVal())
        case model.KindInt:
            overridden = model.Int(key, field.IntVal())
        case model.KindUint:
            overridden = model.Uint(key, field.UintVal())
        case model.KindFloat:
            overridden = model.Float64(key, field.FloatVal())
        case model.KindBool:
            overridden = model.Bool(key, field.BoolVal())
        }
        e.buf = config.AppendAttr(e.buf, overridden)
    }
}
```

**Note:** This is slightly verbose due to key-prefix override. Simplify later if needed.

### Step 2.11: Run all tests

```bash
go test -tags testing ./... 2>&1 | tail -20
```
Expected: all PASS.

### Step 2.12: Bench

```bash
go test -bench=Benchmark_Log -benchmem -count=5 -run=^$ ./test/benchmark/ 2>&1
```

### Step 2.13: Update GitHub issue #83, commit, push, open draft PR

```bash
# Update issue
./.claude/scripts/github-api.sh update-issue 83 "open" "## P2 complete\n\nChanges: typed LogAttr constructors (Str/Int/Bool/Float64/Uint) + AppendAttr in config. Zero boxing on typed hot path. Bench: (paste)"

# Commit
git add model/attr.go model/attr_typed_test.go config/parse_field.go config/append_attr_test.go entry/entry.go
git commit -m "perf(p2): typed LogAttr constructors + AppendAttr zero-boxing encoder refs #83"
git push

# Draft PR
./.claude/scripts/github-api.sh create-draft-pr "P2: typed field API — Str/Int/Bool/Float64" "Fixes #83\n\nEliminate interface{} boxing for typed log fields. New constructors: model.Str/Int/Bool/Float64/Uint. AppendAttr dispatches by kind without type assertion on interface{}." feat/83-p2-typed-field-api feat/81-v2-performance
```

---

## Task 3 (P3): Append-to-buf context API — eliminate map[string]any

**Issue:** #84  
**Branch:** `feat/84-p3-ctx-append-buf`

**Root cause:** `ContextFieldsParser = func(ctx context.Context) map[string]any` allocates a new `map` + `[]string` intermediate on every log call (~23 allocs, ~600 ns). Fix: add `ContextFieldsAppender = func(ctx context.Context, dst []byte) []byte` that writes fields directly into the entry buffer. Keep old `ContextFieldsParser` for backward compatibility.

**Files:**
- Modify: `config/config.go` (new type + field + setter + snapshot field)
- Modify: `entry/entry.go` (prefer appender over parser)
- Create: `config/ctx_appender_test.go`
- Modify: `test/benchmark/entry_benchmark_test.go` (update benchmark to use new API)

### Step 3.1: Checkout story branch

```bash
git checkout feat/84-p3-ctx-append-buf
git merge feat/83-p2-typed-field-api --no-ff -m "merge P2 typed fields into P3 branch"
```

### Step 3.2: Write failing test — new type exists and settter works

- [ ] Create `config/ctx_appender_test.go`:

```go
//go:build testing

package config_test

import (
	"context"
	"testing"

	"github.com/architagr/lognugget/config"
)

func Test_SetContextFieldsAppender_AppenderIsCalled(t *testing.T) {
	config.TestResetConfig()
	called := false
	config.SetContextFieldsAppender(func(ctx context.Context, dst []byte) []byte {
		called = true
		dst = append(dst, `,"svc":"test"`...)
		return dst
	})
	snap := config.GetHotSnapshot()
	if snap.ContextAppender == nil {
		t.Fatal("ContextAppender should not be nil after SetContextFieldsAppender")
	}
	dst := snap.ContextAppender(context.Background(), nil)
	if !called {
		t.Error("appender was not called")
	}
	if string(dst) != `,"svc":"test"` {
		t.Errorf("appender output = %q, want %q", string(dst), `,"svc":"test"`)
	}
}

func Test_ContextAppender_TakesPrecedenceOverParser(t *testing.T) {
	config.TestResetConfig()
	// Install both — appender should win.
	config.SetContextFieldsParser(func(ctx context.Context) map[string]any {
		return map[string]any{"from_parser": true}
	})
	config.SetContextFieldsAppender(func(ctx context.Context, dst []byte) []byte {
		return append(dst, `,"from_appender":true`...)
	})
	snap := config.GetHotSnapshot()
	if snap.ContextAppender == nil {
		t.Fatal("ContextAppender nil")
	}
}
```

### Step 3.3: Run to confirm failure

```bash
go test -tags testing -run "Test_SetContextFieldsAppender|Test_ContextAppender" ./config/ -v
```
Expected: FAIL — `config.SetContextFieldsAppender`, `snap.ContextAppender` undefined.

### Step 3.4: Add `ContextFieldsAppender` type and wiring to `config/config.go`

- [ ] After the existing `ContextFieldsParser` type alias, add:

```go
// ContextFieldsAppender writes context-derived log fields directly into dst,
// returning the extended slice. Each field should be appended as `,<key>:<value>`
// (leading comma, no opening/closing braces). This API eliminates the map[string]any
// allocation of ContextFieldsParser on the hot path.
type ContextFieldsAppender = func(ctx context.Context, dst []byte) []byte
```

- [ ] Add `contextAppender ContextFieldsAppender` field to the `Config` struct (after `contextParser`).

- [ ] Add `SetContextFieldsAppender` function:

```go
// SetContextFieldsAppender sets the zero-alloc context field writer.
// When set, it takes precedence over any ContextFieldsParser on the hot path.
// The appender receives the entry buffer and must append `,key:value` fragments
// for each context field, returning the extended buffer. Safe for concurrent use.
func SetContextFieldsAppender(appender ContextFieldsAppender) {
	configMu.Lock()
	defer configMu.Unlock()
	defaultConfig.contextAppender = appender
}
```

- [ ] Add `ContextAppender` to `HotSnapshot` struct:

```go
ContextAppender ContextFieldsAppender
```

- [ ] Add it to `GetHotSnapshot` return:

```go
ContextAppender: defaultConfig.contextAppender,
```

### Step 3.5: Run config tests

```bash
go test -tags testing -run "Test_SetContextFieldsAppender|Test_ContextAppender" ./config/ -v
```
Expected: PASS.

### Step 3.6: Update `entry/entry.go` to prefer appender over parser

- [ ] Replace the context-fields section in `logWithSkip`:

```go
if ctx != nil && snap.ContextParser != nil {
    for key, value := range snap.ContextParser(ctx) {
        k := string(key)
        if _, restricted := snap.RestrictedFields[k]; restricted {
            k = config.DefaultPrefix + k
        }
        e.buf = append(e.buf, ',')
        e.buf = config.AppendField(e.buf, k, value)
    }
}
```

With:

```go
if ctx != nil {
    if snap.ContextAppender != nil {
        // Zero-alloc path: appender writes directly into e.buf.
        e.buf = snap.ContextAppender(ctx, e.buf)
    } else if snap.ContextParser != nil {
        // Legacy path: map[string]any (allocates).
        for key, value := range snap.ContextParser(ctx) {
            k := string(key)
            if _, restricted := snap.RestrictedFields[k]; restricted {
                k = config.DefaultPrefix + k
            }
            e.buf = append(e.buf, ',')
            e.buf = config.AppendField(e.buf, k, value)
        }
    }
}
```

### Step 3.7: Update benchmark to use new API

- [ ] In `test/benchmark/entry_benchmark_test.go`, add a new benchmark `Benchmark_Log_Appender` that mirrors `Benchmark_Log` but uses `SetContextFieldsAppender`:

```go
func Benchmark_Log_Appender(b *testing.B) {
	b.StopTimer()
	out := &MockWriter{}
	config.SetMinLevel(enum.LevelDebug)
	config.SetEncoderType(enum.EncoderJSON)
	config.SetStaticEnvFieldsParser(func() map[string]any {
		return map[string]any{
			"app_name": "lognugget",
			"version":  "1.0.0",
		}
	})
	config.SetContextFieldsAppender(func(ctx context.Context, dst []byte) []byte {
		reqID := ctx.Value(ctxKeyRequestID)
		userID := ctx.Value(ctxKeyUserID)
		dst = append(dst, `,"request_id":`...)
		dst = config.AppendQuotedString(dst, fmt.Sprintf("%v", reqID))
		dst = append(dst, `,"user_id":`...)
		dst = config.AppendQuotedString(dst, fmt.Sprintf("%v", userID))
		return dst
	})

	unsetPostProcessor := pipelineStage.NewUnsetLogEventPostProcessor(2*time.Second, 500, out)
	defer unsetPostProcessor.Stop()
	pipelineStage.EventPreProcessorObj.RegisterHook(enum.LevelUnSet, unsetPostProcessor)
	config.InitPreProcessors(pipelineStage.EventPreProcessorObj)
	entry.GenerateInitialPool(1_000_000)

	ctxs := make([]context.Context, b.N)
	for i := range ctxs {
		ctxs[i] = context.WithValue(context.WithValue(context.Background(), ctxKeyRequestID, i), ctxKeyUserID, "User1234")
	}

	b.ReportAllocs()
	b.ResetTimer()
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		entryObj := entry.NewLogEntry()
		entryObj.Debug(ctxs[i], "debug message that has a log message, from lognugget", model.Int("itrr", int64(i)))
	}
}
```

Add `"fmt"` to imports if not already present.

### Step 3.8: Run all tests

```bash
go test -tags testing ./... 2>&1 | tail -20
```
Expected: PASS.

### Step 3.9: Bench both variants

```bash
go test -bench="Benchmark_Log$|Benchmark_Log_Appender" -benchmem -count=5 -run=^$ ./test/benchmark/ 2>&1
```
Expected: `Benchmark_Log_Appender` shows dramatically fewer allocs vs `Benchmark_Log`.

### Step 3.10: Update GitHub issue #84, commit, push, open draft PR

```bash
./.claude/scripts/github-api.sh update-issue 84 "open" "## P3 complete\n\nChanges: ContextFieldsAppender type + SetContextFieldsAppender + hot path appender takes precedence over parser. Bench (paste ns/op + allocs before/after)"

git add config/config.go config/ctx_appender_test.go entry/entry.go test/benchmark/entry_benchmark_test.go
git commit -m "perf(p3): ContextFieldsAppender — direct-append context API eliminates map[string]any refs #84"
git push

./.claude/scripts/github-api.sh create-draft-pr "P3: append-to-buf context API" "Fixes #84\n\nNew ContextFieldsAppender type writes context fields directly into the entry buffer. Eliminates map[string]any allocation (~600 ns / ~23 allocs per call). Old ContextFieldsParser preserved for backward compat." feat/84-p3-ctx-append-buf feat/81-v2-performance
```

---

## Task 4 (P4): Inline framing — eliminate en.Append double-buffer

**Issue:** #85  
**Branch:** `feat/85-p4-inline-framing`

**Root cause:** `en.Append(nil, e.buf)` creates a new `[]byte` (alloc #1) containing `{body}\n`. Then `PublishLog` copies it (alloc #2). Total: 2 allocs just for framing. Fix: pre-write the opening byte into `e.buf` at the start of `logWithSkip`; post-append the closing bytes at the end. `e.buf` then contains the fully-framed payload and `PublishLog` makes just 1 copy. The `encoder.Append` call is removed from the hot path; framing is encoder-type-aware.

**Files:**
- Modify: `encoder/encoder.go` (add `OpenBytes() []byte`, `CloseBytes() []byte` to interface)
- Modify: `encoder/json_encoder.go` (implement new methods)
- Modify: `encoder/text_encoder.go` (implement new methods)
- Modify: `config/config.go` (`HotSnapshot` adds `EncoderOpen/Close []byte`)
- Modify: `entry/entry.go` (pre-write open, post-write close, no encoder.Append)
- Modify: `encoder/factory_test.go` / encoder tests (add coverage for new methods)

### Step 4.1: Checkout story branch

```bash
git checkout feat/85-p4-inline-framing
git merge feat/84-p3-ctx-append-buf --no-ff -m "merge P3 context appender into P4 branch"
```

### Step 4.2: Write failing tests — new encoder interface methods

- [ ] Add to `encoder/json_encoder_test.go` (or a new file `encoder/inline_framing_test.go`):

```go
package encoder_test

import (
	"testing"

	"github.com/architagr/lognugget/encoder"
)

func Test_JSONEncoder_OpenCloseBytes(t *testing.T) {
	enc := encoder.NewJSONEncoder()
	if string(enc.OpenBytes()) != "{" {
		t.Errorf("OpenBytes = %q, want %q", enc.OpenBytes(), "{")
	}
	if string(enc.CloseBytes()) != "}\n" {
		t.Errorf("CloseBytes = %q, want %q", enc.CloseBytes(), "}\n")
	}
}

func Test_TextEncoder_OpenCloseBytes(t *testing.T) {
	enc := encoder.NewTextEncoder()
	if len(enc.OpenBytes()) != 0 {
		t.Errorf("TextEncoder.OpenBytes should be empty, got %q", enc.OpenBytes())
	}
	if string(enc.CloseBytes()) != "\n" {
		t.Errorf("TextEncoder.CloseBytes = %q, want %q", enc.CloseBytes(), "\n")
	}
}

func Test_InlineFraming_ProducesEquivalentOutput(t *testing.T) {
	enc := encoder.NewJSONEncoder()
	body := []byte(`"time":"t","level":"info"`)

	// Old path: Append(nil, body)
	oldOut := enc.Append(nil, body)

	// New inline path
	buf := make([]byte, 0, 64)
	buf = append(buf, enc.OpenBytes()...)
	buf = append(buf, body...)
	buf = append(buf, enc.CloseBytes()...)

	if string(buf) != string(oldOut) {
		t.Errorf("inline path = %q, want %q", buf, oldOut)
	}
}
```

### Step 4.3: Run to confirm failure

```bash
go test -run "Test_JSONEncoder_Open|Test_TextEncoder_Open|Test_InlineFraming" ./encoder/ -v
```
Expected: FAIL — `enc.OpenBytes()` undefined.

### Step 4.4: Add `OpenBytes`/`CloseBytes` to `encoder/encoder.go`

- [ ] Extend the `Encoder` interface:

```go
type Encoder interface {
	// Append wraps body as a complete log line and appends to dst.
	// Retained for backward compat; hot path uses inline framing instead.
	Append(dst, body []byte) []byte

	// OpenBytes returns the bytes prepended before the log body in inline framing.
	// These bytes are written into the entry buffer before field construction.
	OpenBytes() []byte

	// CloseBytes returns the bytes appended after the log body in inline framing.
	// These bytes are written into the entry buffer after all fields are built.
	CloseBytes() []byte

	Name() string
}
```

### Step 4.5: Implement in `encoder/json_encoder.go` and `encoder/text_encoder.go`

- [ ] Add to `json_encoder.go`:

```go
var jsonOpen  = []byte{'{'}
var jsonClose = []byte{'}', '\n'}

func (e *JSONEncoder) OpenBytes() []byte  { return jsonOpen }
func (e *JSONEncoder) CloseBytes() []byte { return jsonClose }
```

- [ ] Add to `text_encoder.go`:

```go
var textClose = []byte{'\n'}

func (e *TextEncoder) OpenBytes() []byte  { return nil }
func (e *TextEncoder) CloseBytes() []byte { return textClose }
```

### Step 4.6: Run encoder tests

```bash
go test -run "Test_JSONEncoder_Open|Test_TextEncoder_Open|Test_InlineFraming" ./encoder/ -v
```
Expected: PASS.

### Step 4.7: Add `EncoderOpen`/`EncoderClose` to `HotSnapshot` and `GetHotSnapshot`

- [ ] Add to `HotSnapshot`:

```go
EncoderOpen  []byte
EncoderClose []byte
```

- [ ] Add to `GetHotSnapshot`:

```go
EncoderOpen:  defaultConfig.encoderObj.OpenBytes(),
EncoderClose: defaultConfig.encoderObj.CloseBytes(),
```

### Step 4.8: Update `entry/entry.go` to use inline framing

- [ ] In `logWithSkip`, replace:

```go
e.buf = e.buf[:0]

e.buf = append(e.buf, snap.Rendered[enum.DefaultLogKeyTime]...)
```

With:

```go
e.buf = e.buf[:0]
e.buf = append(e.buf, snap.EncoderOpen...)  // pre-write open bytes (e.g. '{')

e.buf = append(e.buf, snap.Rendered[enum.DefaultLogKeyTime]...)
```

- [ ] Replace at the end of `logWithSkip`:

```go
byteData := snap.Encoder.Append(nil, e.buf)
config.PublishLog(level, byteData)
```

With:

```go
e.buf = append(e.buf, snap.EncoderClose...)  // post-write close bytes (e.g. "}\n")
config.PublishLog(level, e.buf)
```

### Step 4.9: Run all tests

```bash
go test -tags testing ./... 2>&1 | tail -20
```
Expected: PASS.

### Step 4.10: Bench — confirm 1 alloc saved

```bash
go test -bench="Benchmark_Log_Appender" -benchmem -count=5 -run=^$ ./test/benchmark/ 2>&1
```
Expected: allocs/op drops by 1 (the `en.Append(nil, body)` allocation is gone).

### Step 4.11: Update issue, commit, push, draft PR

```bash
./.claude/scripts/github-api.sh update-issue 85 "open" "## P4 complete\n\nChanges: Encoder.OpenBytes()/CloseBytes() interface methods. Entry pre-writes open, post-writes close into e.buf; no Append(nil,...) call. Saves 1 alloc. Bench: (paste)"

git add encoder/encoder.go encoder/json_encoder.go encoder/text_encoder.go config/config.go entry/entry.go
git commit -m "perf(p4): inline encoder framing — eliminate en.Append(nil,buf) double-buffer alloc refs #85"
git push

./.claude/scripts/github-api.sh create-draft-pr "P4: inline framing, eliminate Append double-buffer" "Fixes #85\n\nNew Encoder.OpenBytes()/CloseBytes() methods. Entry pre-writes open bytes, builds fields, post-writes close bytes. No Append(nil,body) call in hot path — saves 1 alloc per log event." feat/85-p4-inline-framing feat/81-v2-performance
```

---

## Task 5 (P5): Channel capacity ≥ 1000 + configurable

**Issue:** #86  
**Branch:** `feat/86-p5-channel-capacity`

**Root cause:** `make(chan LogEvent, 10)` in `resetConfig` is hardcoded to 10. Under 8-goroutine parallel load, callers block waiting for the dispatcher to drain. Fix: raise default to 1,000 and wire `resetConfig` to use `DafaultLogBuffer`.

**Files:**
- Modify: `config/config.go` (change `DafaultLogBuffer` default + fix hardcoded `10`)
- Modify: `config/config_race_test.go` or create `config/channel_cap_test.go`

### Step 5.1: Checkout story branch

```bash
git checkout feat/86-p5-channel-capacity
git merge feat/85-p4-inline-framing --no-ff -m "merge P4 inline framing into P5 branch"
```

### Step 5.2: Write failing test — channel is sized by DafaultLogBuffer

- [ ] Create `config/channel_cap_test.go`:

```go
//go:build testing

package config_test

import (
	"testing"

	"github.com/architagr/lognugget/config"
)

func Test_DefaultLogBuffer_IsAtLeast1000(t *testing.T) {
	if config.DafaultLogBuffer < 1000 {
		t.Errorf("DafaultLogBuffer = %d, want >= 1000 (must not block 8-goroutine parallel callers)", config.DafaultLogBuffer)
	}
}

// Test_ChannelNotBlockedUnder1000Sends verifies that sending 999 events into
// the dispatch channel does not block (i.e. channel capacity >= 1000).
func Test_ChannelNotBlockedUnder1000Sends(t *testing.T) {
	config.TestResetConfig()

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < config.DafaultLogBuffer-1; i++ {
			config.TestPublishDirect(i)
		}
	}()

	select {
	case <-done:
		// good — no blocking
	case <-timeoutChan(200):
		t.Error("channel blocked before reaching DafaultLogBuffer-1 sends")
	}
}
```

This test requires `config.TestPublishDirect` (test-only helper) and a `timeoutChan` helper. Add them.

- [ ] Add to `config/test_only_helpers.go`:

```go
//go:build testing

package config

import "time"

// TestPublishDirect bypasses the copy in PublishLog and sends a minimal event
// to verify channel capacity without allocating large byte slices.
func TestPublishDirect(_ int) {
	ch <- LogEvent{Level: 0, Data: []byte{}}
}

func TimeoutChan(ms int) <-chan struct{} {
	c := make(chan struct{})
	go func() {
		time.Sleep(time.Duration(ms) * time.Millisecond)
		close(c)
	}()
	return c
}
```

Adjust `channel_cap_test.go` to call `config.TimeoutChan(200)` instead of `timeoutChan(200)`.

### Step 5.3: Run to confirm failure

```bash
go test -tags testing -run "Test_DefaultLogBuffer|Test_ChannelNotBlocked" ./config/ -v
```
Expected: FAIL — `DafaultLogBuffer` is 20, not ≥ 1000; or `TestPublishDirect` undefined.

### Step 5.4: Fix `config/config.go`

- [ ] Change:

```go
DafaultLogBuffer  int       = 20          // Default buffer size for logs
```

To:

```go
DafaultLogBuffer int = 1000 // Default channel buffer; sized to absorb bursts from parallel callers
```

- [ ] In `resetConfig`, change:

```go
newCh := make(chan LogEvent, 10)
```

To:

```go
newCh := make(chan LogEvent, DafaultLogBuffer)
```

### Step 5.5: Run tests

```bash
go test -tags testing -run "Test_DefaultLogBuffer|Test_ChannelNotBlocked" ./config/ -v
```
Expected: PASS.

### Step 5.6: Run all tests

```bash
go test -tags testing ./... 2>&1 | tail -20
```
Expected: PASS.

### Step 5.7: Bench parallel path

```bash
go test -bench="Benchmark_Log_Parallel|Benchmark_Log_Appender" -benchmem -count=5 -run=^$ ./test/benchmark/ 2>&1
```
Expected: parallel benchmark shows fewer goroutine-blocking stalls.

### Step 5.8: Update STATUS.md for all 5 stories

- [ ] In `docs/STATUS.md`, update the Epic V2 stories table:

```markdown
| P1 | [#82] | Atomic minLevel + single config snapshot per call | ✅ DONE |
| P2 | [#83] | Typed field API — Str/Int/Bool/Float64 on LogEntry | ✅ DONE |
| P3 | [#84] | Append-to-buf context API — eliminate map[string]any | ✅ DONE |
| P4 | [#85] | Inline framing — eliminate en.Append double-buffer | ✅ DONE |
| P5 | [#86] | Channel capacity ≥ 1000 + configurable | ✅ DONE |
```

Update umbrella issue status to `🚧 IN PROGRESS` → `✅ DONE` (once bench gate is green).

### Step 5.9: Update GitHub issue #86, commit, push, draft PR

```bash
./.claude/scripts/github-api.sh update-issue 86 "open" "## P5 complete\n\nChanges: DafaultLogBuffer 20 → 1000; resetConfig uses DafaultLogBuffer not hardcoded 10. Parallel bench: (paste)"

git add config/config.go config/channel_cap_test.go config/test_only_helpers.go docs/STATUS.md
git commit -m "perf(p5): channel capacity 1000 + use DafaultLogBuffer in resetConfig refs #86"
git push

./.claude/scripts/github-api.sh create-draft-pr "P5: channel capacity >= 1000, configurable" "Fixes #86\n\nDafaultLogBuffer 20 → 1000. resetConfig uses DafaultLogBuffer (was hardcoded 10). Prevents blocking under 8-goroutine parallel callers." feat/86-p5-channel-capacity feat/81-v2-performance
```

---

## Final: Merge all stories → feat/81-v2-performance + run bench gate

```bash
git checkout feat/81-v2-performance
git merge feat/82-p1-atomic-min-level --no-ff -m "merge P1: atomic minLevel + hot snapshot"
git merge feat/83-p2-typed-field-api --no-ff -m "merge P2: typed field API"
git merge feat/84-p3-ctx-append-buf --no-ff -m "merge P3: context append-to-buf"
git merge feat/85-p4-inline-framing --no-ff -m "merge P4: inline encoder framing"
git merge feat/86-p5-channel-capacity --no-ff -m "merge P5: channel capacity 1000"

# Full bench gate
./scripts/bench-check.sh

git push origin feat/81-v2-performance
```

Once bench gate is green, open a PR from `feat/81-v2-performance` → `develop`.

---

## Self-review checklist

- [x] Every story has a failing test before implementation code
- [x] P1 preserves 0-alloc filtered path (atomic gate, no RLock on filtered)
- [x] P2 is backward compatible — `LogAttr{Key:...,Value:...}` still works
- [x] P3 keeps old `ContextFieldsParser` for existing callers; appender is opt-in
- [x] P4 encoder `Append(dst,body)` method kept — no interface breakage; `OpenBytes/CloseBytes` added
- [x] P5 changes only the default value and one `make()` call — zero logic change
- [x] Each story updates its GitHub issue and opens a draft PR to the feature branch (not develop)
- [x] Final merge runs `./scripts/bench-check.sh` before PR to develop
