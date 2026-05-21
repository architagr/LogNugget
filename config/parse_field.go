// Package config holds the singleton logger configuration for LogNugget.
package config

import (
	"fmt"
	"math"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/architagr/lognugget/model"
)

// AppendQuotedString appends s to dst as an RFC 8259 JSON string (including the
// surrounding double-quote characters) and returns the extended slice. dst may
// be nil; a new slice is allocated in that case.
//
// This is the exported companion of the package-internal appendJSONString.  It
// lets packages outside config (specifically entry) append a quoted value onto
// a pre-rendered key prefix without having to duplicate the escape logic or
// import a cycle.
//
// Safe for concurrent use; reads no shared state.
func AppendQuotedString(dst []byte, s string) []byte {
	return appendJSONString(dst, []byte(s))
}

// appendJSONString appends src to dst as an RFC 8259 JSON string (including the
// surrounding double-quote characters). Invalid UTF-8 bytes are replaced with
// the Unicode replacement character (U+FFFD).
//
// This duplicates the escape logic from encoder.escapeJSON intentionally:
// importing encoder from config would create an import cycle (config → encoder
// already exists in the opposite direction). The escape table is initialised
// once in init() to avoid repeated branch evaluation on the hot path.
//
// why: copying the table pattern rather than the logic keeps the two sites
// in sync structurally while avoiding the cycle. If the escape rules ever
// diverge, the encoder tests (TestJSONEncoder_*) and the config tests
// (Test_AppendField_StringEscape) will both catch the drift.
func appendJSONString(dst, src []byte) []byte {
	dst = append(dst, '"')
	for i := 0; i < len(src); {
		b := src[i]
		if b >= utf8.RuneSelf {
			r, size := utf8.DecodeRune(src[i:])
			if r == utf8.RuneError && size == 1 {
				dst = append(dst, '\xef', '\xbf', '\xbd') // U+FFFD replacement
				i++
				continue
			}
			dst = append(dst, src[i:i+size]...)
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

// cfgEscapeTable maps each byte value to an RFC 8259 escape class.
// 0 = no escape; values 1-8 map to specific sequences.
// why: table lookup is faster than a chain of conditionals on the hot path.
var cfgEscapeTable [256]uint8

const (
	cfgEscHex  uint8 = 1 // \uXXXX (control chars U+0000–U+001F)
	cfgEscQuot uint8 = 2 // \"
	cfgEscBksl uint8 = 3 // \\
	cfgEscB    uint8 = 4 // \b
	cfgEscF    uint8 = 5 // \f
	cfgEscN    uint8 = 6 // \n
	cfgEscR    uint8 = 7 // \r
	cfgEscT    uint8 = 8 // \t
)

var cfgHexDigits = "0123456789abcdef"

func init() {
	for i := 0; i <= 0x1f; i++ {
		cfgEscapeTable[i] = cfgEscHex
	}
	cfgEscapeTable['"'] = cfgEscQuot
	cfgEscapeTable['\\'] = cfgEscBksl
	cfgEscapeTable['\b'] = cfgEscB
	cfgEscapeTable['\f'] = cfgEscF
	cfgEscapeTable['\n'] = cfgEscN
	cfgEscapeTable['\r'] = cfgEscR
	cfgEscapeTable['\t'] = cfgEscT
}

// AppendField appends a JSON key-value fragment of the form `"key":value` to
// dst and returns the extended slice. dst may be nil; a new slice is allocated
// in that case.
//
// Key encoding: RFC 8259 escaped JSON string (same rules as string values).
//
// Value encoding by type:
//   - string: quoted, RFC 8259 escaped
//   - int8/16/32/64/int: unquoted integer via strconv.AppendInt (0 allocs when dst has capacity)
//   - uint8/16/32/64/uint: unquoted via strconv.AppendUint
//   - float32/float64: unquoted via strconv.AppendFloat; NaN and ±Inf → unquoted null
//   - bool: unquoted true/false via strconv.AppendBool
//   - error: quoted string of err.Error()
//   - time.Time: quoted RFC3339 string
//   - []any, map, or any other type: slow path via fmt.Append (may allocate)
//
// The slow path is intentional and documented: unknown types are formatted with
// %+v and the result is validated to be JSON-safe only to the extent that the
// caller ensures the value's String() output is safe. For structured logging,
// callers should use one of the strongly-typed paths.
//
// AppendField is safe for concurrent use; it reads no shared state.
func AppendField(dst []byte, key string, value any) []byte {
	// Write the key as a quoted, RFC 8259 escaped JSON string.
	dst = appendJSONString(dst, []byte(key))
	dst = append(dst, ':')

	switch v := value.(type) {
	case string:
		dst = appendJSONString(dst, []byte(v))

	case int:
		dst = strconv.AppendInt(dst, int64(v), 10)
	case int8:
		dst = strconv.AppendInt(dst, int64(v), 10)
	case int16:
		dst = strconv.AppendInt(dst, int64(v), 10)
	case int32:
		dst = strconv.AppendInt(dst, int64(v), 10)
	case int64:
		dst = strconv.AppendInt(dst, v, 10)

	case uint:
		dst = strconv.AppendUint(dst, uint64(v), 10)
	case uint8:
		dst = strconv.AppendUint(dst, uint64(v), 10)
	case uint16:
		dst = strconv.AppendUint(dst, uint64(v), 10)
	case uint32:
		dst = strconv.AppendUint(dst, uint64(v), 10)
	case uint64:
		dst = strconv.AppendUint(dst, v, 10)

	case float32:
		f64 := float64(v)
		if math.IsNaN(f64) || math.IsInf(f64, 0) {
			// why: JSON does not allow NaN or Inf; null is the idiomatic sentinel.
			dst = append(dst, "null"...)
		} else {
			dst = strconv.AppendFloat(dst, f64, 'f', -1, 32)
		}
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) {
			// why: JSON does not allow NaN or Inf; null is the idiomatic sentinel.
			dst = append(dst, "null"...)
		} else {
			dst = strconv.AppendFloat(dst, v, 'f', -1, 64)
		}

	case bool:
		dst = strconv.AppendBool(dst, v)

	case error:
		dst = appendJSONString(dst, []byte(v.Error()))

	case time.Time:
		dst = appendJSONString(dst, []byte(v.Format(time.RFC3339)))

	default:
		// why: slow path for unknown/composite types. fmt.Append avoids an
		// intermediate string allocation compared to fmt.Sprintf. Callers on
		// the hot path should use one of the typed cases above.
		dst = fmt.Appendf(dst, "%+v", value)
	}

	return dst
}

// AppendAttr appends a JSON key-value fragment of the form `"key":value` to
// dst for attr and returns the extended slice. dst may be nil.
//
// For typed kinds (KindStr, KindInt, KindUint, KindFloat, KindBool) the value
// is read from the inline struct field — no interface{} boxing occurs and no
// heap allocation is required when dst has sufficient capacity.
//
// KindAny falls back to interface{} type-switch dispatch (backward-compatible
// with the struct-literal LogAttr{Key:k, Value:v} form).
//
// NaN and ±Inf float values are serialised as JSON null.
//
// AppendAttr is safe for concurrent use; it reads no shared state.
func AppendAttr(dst []byte, key string, attr model.LogAttr) []byte {
	// Write the key as a quoted, RFC 8259 escaped JSON string.
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
			// why: JSON does not allow NaN or Inf; null is the idiomatic sentinel.
			dst = append(dst, "null"...)
		} else {
			dst = strconv.AppendFloat(dst, f, 'f', -1, 64)
		}

	case model.KindBool:
		dst = strconv.AppendBool(dst, attr.BoolVal())

	default:
		// KindAny: legacy interface{} dispatch. Key has already been written above.
		// why: struct-literal callers (LogAttr{Key:k, Value:v}) produced KindAny
		// before P2 existed; this branch keeps them working without any change at
		// call sites. New callers should use the typed constructors.
		switch v := attr.Value.(type) {
		case string:
			dst = appendJSONString(dst, []byte(v))
		case int:
			dst = strconv.AppendInt(dst, int64(v), 10)
		case int8:
			dst = strconv.AppendInt(dst, int64(v), 10)
		case int16:
			dst = strconv.AppendInt(dst, int64(v), 10)
		case int32:
			dst = strconv.AppendInt(dst, int64(v), 10)
		case int64:
			dst = strconv.AppendInt(dst, v, 10)
		case uint:
			dst = strconv.AppendUint(dst, uint64(v), 10)
		case uint64:
			dst = strconv.AppendUint(dst, v, 10)
		case float32:
			f64 := float64(v)
			if math.IsNaN(f64) || math.IsInf(f64, 0) {
				dst = append(dst, "null"...)
			} else {
				dst = strconv.AppendFloat(dst, f64, 'f', -1, 32)
			}
		case float64:
			if math.IsNaN(v) || math.IsInf(v, 0) {
				dst = append(dst, "null"...)
			} else {
				dst = strconv.AppendFloat(dst, v, 'f', -1, 64)
			}
		case bool:
			dst = strconv.AppendBool(dst, v)
		default:
			// why: ultimate slow path for unknown/composite types. fmt.Appendf
			// avoids an intermediate string allocation vs fmt.Sprintf.
			dst = fmt.Appendf(dst, "%+v", attr.Value)
		}
	}

	return dst
}
