package config

import (
	"context"
	"math"
	"strconv"
	"sync"
)

// CtxFields is passed to a ContextFieldsFunc callback. Callers add per-request
// context fields with typed methods (Str, Int, Bool, Float64, Uint);
// LogNugget handles all JSON encoding and RFC 8259 escaping internally.
//
// The *CtxFields pointer is only valid for the duration of the callback.
// Do not store or use it outside the function.
type CtxFields struct {
	buf []byte
}

// Str appends a JSON string field: ,"key":"value" (both key and value are RFC 8259 escaped).
func (f *CtxFields) Str(key, value string) {
	f.buf = append(f.buf, ',')
	f.buf = appendJSONStringStr(f.buf, key)
	f.buf = append(f.buf, ':')
	f.buf = appendJSONStringStr(f.buf, value)
}

// Int appends a JSON integer field: ,"key":value.
func (f *CtxFields) Int(key string, value int64) {
	f.buf = append(f.buf, ',')
	f.buf = appendJSONStringStr(f.buf, key)
	f.buf = append(f.buf, ':')
	f.buf = strconv.AppendInt(f.buf, value, 10)
}

// Uint appends a JSON unsigned integer field: ,"key":value.
func (f *CtxFields) Uint(key string, value uint64) {
	f.buf = append(f.buf, ',')
	f.buf = appendJSONStringStr(f.buf, key)
	f.buf = append(f.buf, ':')
	f.buf = strconv.AppendUint(f.buf, value, 10)
}

// Bool appends a JSON boolean field: ,"key":true|false.
func (f *CtxFields) Bool(key string, value bool) {
	f.buf = append(f.buf, ',')
	f.buf = appendJSONStringStr(f.buf, key)
	f.buf = append(f.buf, ':')
	f.buf = strconv.AppendBool(f.buf, value)
}

// Float64 appends a JSON float field: ,"key":value.
// NaN and ±Inf are serialised as JSON null.
func (f *CtxFields) Float64(key string, value float64) {
	f.buf = append(f.buf, ',')
	f.buf = appendJSONStringStr(f.buf, key)
	f.buf = append(f.buf, ':')
	if math.IsNaN(value) || math.IsInf(value, 0) {
		f.buf = append(f.buf, "null"...)
	} else {
		f.buf = strconv.AppendFloat(f.buf, value, 'f', -1, 64)
	}
}

// ContextFieldsFunc is the simplified per-request context field callback.
// Callers add fields using typed methods on *CtxFields; LogNugget handles
// JSON encoding. See SetContextFields for usage.
type ContextFieldsFunc = func(ctx context.Context, f *CtxFields)

var ctxFieldsPool = sync.Pool{New: func() any { return &CtxFields{} }}

// SetContextFields registers a simplified per-request context field callback.
// Use this instead of SetContextFieldsAppender — callers provide field names
// and values using typed methods and LogNugget handles JSON encoding internally.
//
// Example (OTel trace/span IDs):
//
//	config.SetContextFields(func(ctx context.Context, f *config.CtxFields) {
//	    sc := trace.SpanFromContext(ctx).SpanContext()
//	    if sc.IsValid() {
//	        f.Str("trace_id", sc.TraceID().String())
//	        f.Str("span_id", sc.SpanID().String())
//	    }
//	})
//
// Passing nil clears any previously registered context field callback.
func SetContextFields(fn ContextFieldsFunc) {
	if fn == nil {
		SetContextFieldsAppender(nil)
		return
	}
	SetContextFieldsAppender(func(ctx context.Context, dst []byte) []byte {
		f := ctxFieldsPool.Get().(*CtxFields)
		f.buf = dst
		fn(ctx, f)
		dst = f.buf
		f.buf = nil
		ctxFieldsPool.Put(f)
		return dst
	})
}
