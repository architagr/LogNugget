// Package config_test tests AppendAttr, the zero-boxing typed-field serialiser
// introduced in Epic V2 / Story P2.
package config_test

import (
	"math"
	"testing"

	"github.com/architagr/lognugget/config"
	"github.com/architagr/lognugget/model"
)

// Test_AppendAttr_Str verifies that a KindStr attr is serialised to a quoted
// JSON string fragment.
func Test_AppendAttr_Str(t *testing.T) {
	got := string(config.AppendAttr(nil, "k", model.Str("k", "hello")))
	if got != `"k":"hello"` {
		t.Errorf("got %q want %q", got, `"k":"hello"`)
	}
}

// Test_AppendAttr_Int verifies that a KindInt attr is serialised to an
// unquoted integer JSON fragment.
func Test_AppendAttr_Int(t *testing.T) {
	got := string(config.AppendAttr(nil, "n", model.Int("n", 42)))
	if got != `"n":42` {
		t.Errorf("got %q want %q", got, `"n":42`)
	}
}

// Test_AppendAttr_Bool verifies that a KindBool attr is serialised to an
// unquoted JSON boolean fragment.
func Test_AppendAttr_Bool(t *testing.T) {
	got := string(config.AppendAttr(nil, "ok", model.Bool("ok", true)))
	if got != `"ok":true` {
		t.Errorf("got %q want %q", got, `"ok":true`)
	}
}

// Test_AppendAttr_Float64 verifies that a KindFloat attr is serialised to an
// unquoted JSON number fragment.
func Test_AppendAttr_Float64(t *testing.T) {
	got := string(config.AppendAttr(nil, "r", model.Float64("r", 1.5)))
	if got != `"r":1.5` {
		t.Errorf("got %q want %q", got, `"r":1.5`)
	}
}

// Test_AppendAttr_AnyFallback verifies that KindAny (struct-literal LogAttr)
// falls back to the legacy interface{} dispatch and still produces correct JSON.
func Test_AppendAttr_AnyFallback(t *testing.T) {
	got := string(config.AppendAttr(nil, "x", model.LogAttr{Key: "x", Value: "v"}))
	if got != `"x":"v"` {
		t.Errorf("got %q want %q", got, `"x":"v"`)
	}
}

// Test_AppendAttr_Str_ZeroAlloc verifies that AppendAttr for a KindStr attr
// performs zero heap allocations when the destination slice has sufficient capacity.
func Test_AppendAttr_Str_ZeroAlloc(t *testing.T) {
	attr := model.Str("key", "value")
	dst := make([]byte, 0, 64)
	allocs := testing.AllocsPerRun(100, func() {
		dst = config.AppendAttr(dst[:0], "key", attr)
	})
	if allocs != 0 {
		t.Errorf("AppendAttr Str allocs=%.0f want 0", allocs)
	}
}

// Test_AppendAttr_Int_ZeroAlloc verifies that AppendAttr for a KindInt attr
// performs zero heap allocations when the destination slice has sufficient capacity.
func Test_AppendAttr_Int_ZeroAlloc(t *testing.T) {
	attr := model.Int("n", 99)
	dst := make([]byte, 0, 32)
	allocs := testing.AllocsPerRun(100, func() {
		dst = config.AppendAttr(dst[:0], "n", attr)
	})
	if allocs != 0 {
		t.Errorf("AppendAttr Int allocs=%.0f want 0", allocs)
	}
}

// Test_AppendAttr_Float64_NaN verifies that NaN is serialised as JSON null
// (JSON does not allow NaN as a literal value).
func Test_AppendAttr_Float64_NaN(t *testing.T) {
	got := string(config.AppendAttr(nil, "f", model.Float64("f", math.NaN())))
	if got != `"f":null` {
		t.Errorf("got %q want %q", got, `"f":null`)
	}
}

// Test_AppendAttr_Float64_Inf verifies that ±Inf is serialised as JSON null.
func Test_AppendAttr_Float64_Inf(t *testing.T) {
	got := string(config.AppendAttr(nil, "f", model.Float64("f", math.Inf(1))))
	if got != `"f":null` {
		t.Errorf("got %q want %q", got, `"f":null`)
	}
}
