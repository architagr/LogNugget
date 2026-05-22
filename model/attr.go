// Package model defines the core data types shared across LogNugget packages.
// It is a leaf package: it imports nothing from the rest of the codebase.
// Key entry points: LogAttr (structured log field), the typed constructors
// Str/Int/Uint/Bool/Float64, and AttrKind (discriminates storage path).
package model

// AttrKind identifies how the value of a LogAttr is stored.
// KindAny is the legacy path (Value field, interface{} boxing).
// Other kinds use inline typed fields (no boxing, no heap alloc).
type AttrKind uint8

const (
	// KindAny is the zero value; used by struct-literal LogAttr{Key:k, Value:v}
	// for backward compatibility. AppendAttr falls back to interface{} dispatch.
	KindAny AttrKind = 0
	// KindStr signals that StrVal() holds the field's string value.
	KindStr AttrKind = 1
	// KindInt signals that IntVal() holds the field's int64 value.
	KindInt AttrKind = 2
	// KindUint signals that UintVal() holds the field's uint64 value.
	KindUint AttrKind = 3
	// KindFloat signals that FloatVal() holds the field's float64 value.
	KindFloat AttrKind = 4
	// KindBool signals that BoolVal() holds the field's bool value.
	KindBool AttrKind = 5
)

// LogAttrKey is the key type for a structured log field.
type LogAttrKey string

// LogAttrValue is the legacy untyped value type. It is only consulted when
// Kind() == KindAny. New callers should use the typed constructors.
type LogAttrValue any

// LogAttr is a single structured log field. Use the typed constructors
// (Str, Int, Bool, Float64, Uint) on the hot path to avoid interface{} boxing.
// The struct-literal form (LogAttr{Key: k, Value: v}) still works (KindAny).
//
// LogAttr is intended to be passed and returned by value; do not take a pointer
// to one — it will escape to the heap and defeat the zero-alloc guarantee.
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

// Kind returns the storage kind for this attr. KindAny means the Value field
// is populated; other kinds mean the corresponding inline typed field is used.
func (a LogAttr) Kind() AttrKind { return a.kind }

// StrVal returns the inline string value. Only meaningful when Kind() == KindStr.
func (a LogAttr) StrVal() string { return a.strVal }

// IntVal returns the inline int64 value. Only meaningful when Kind() == KindInt.
func (a LogAttr) IntVal() int64 { return a.intVal }

// UintVal returns the inline uint64 value. Only meaningful when Kind() == KindUint.
func (a LogAttr) UintVal() uint64 { return a.uintVal }

// FloatVal returns the inline float64 value. Only meaningful when Kind() == KindFloat.
func (a LogAttr) FloatVal() float64 { return a.floatVal }

// BoolVal returns the inline bool value. Only meaningful when Kind() == KindBool.
func (a LogAttr) BoolVal() bool { return a.boolVal }

// Str returns a zero-alloc LogAttr for a string value. The caller's string is
// stored inline (no interface{} boxing). Using a constant key is recommended
// to keep the key string on the stack.
func Str(key, val string) LogAttr {
	return LogAttr{Key: LogAttrKey(key), strVal: val, kind: KindStr}
}

// Int returns a zero-alloc LogAttr for an int64 value. The value is stored
// inline in intVal; no interface{} boxing occurs.
func Int(key string, val int64) LogAttr {
	return LogAttr{Key: LogAttrKey(key), intVal: val, kind: KindInt}
}

// Uint returns a zero-alloc LogAttr for a uint64 value. The value is stored
// inline in uintVal; no interface{} boxing occurs.
func Uint(key string, val uint64) LogAttr {
	return LogAttr{Key: LogAttrKey(key), uintVal: val, kind: KindUint}
}

// Bool returns a zero-alloc LogAttr for a bool value. The value is stored
// inline in boolVal; no interface{} boxing occurs.
func Bool(key string, val bool) LogAttr {
	return LogAttr{Key: LogAttrKey(key), boolVal: val, kind: KindBool}
}

// Float64 returns a zero-alloc LogAttr for a float64 value. The value is
// stored inline in floatVal; no interface{} boxing occurs.
// NaN and ±Inf are legal inputs; AppendAttr renders them as JSON null.
func Float64(key string, val float64) LogAttr {
	return LogAttr{Key: LogAttrKey(key), floatVal: val, kind: KindFloat}
}
