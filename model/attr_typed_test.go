// Package model_test tests the typed LogAttr constructors and zero-allocation
// guarantees introduced in Epic V2 / Story P2.
package model_test

import (
	"testing"

	"github.com/architagr/lognugget/model"
)

// Test_Str_KindAndValue verifies that Str sets KindStr and stores the string
// value without touching the legacy Value (interface{}) field.
func Test_Str_KindAndValue(t *testing.T) {
	a := model.Str("k", "hello")
	if a.Key != "k" {
		t.Errorf("Key=%q want %q", a.Key, "k")
	}
	if a.Kind() != model.KindStr {
		t.Errorf("Kind=%v want KindStr", a.Kind())
	}
	if a.StrVal() != "hello" {
		t.Errorf("StrVal=%q want %q", a.StrVal(), "hello")
	}
}

// Test_Int_KindAndValue verifies that Int sets KindInt and stores the int64
// value in the inline intVal field.
func Test_Int_KindAndValue(t *testing.T) {
	a := model.Int("n", 42)
	if a.Kind() != model.KindInt {
		t.Errorf("Kind=%v want KindInt", a.Kind())
	}
	if a.IntVal() != 42 {
		t.Errorf("IntVal=%d want 42", a.IntVal())
	}
}

// Test_Bool_KindAndValue verifies that Bool sets KindBool and stores the bool
// value in the inline boolVal field.
func Test_Bool_KindAndValue(t *testing.T) {
	a := model.Bool("ok", true)
	if a.Kind() != model.KindBool {
		t.Errorf("Kind=%v want KindBool", a.Kind())
	}
	if !a.BoolVal() {
		t.Error("BoolVal=false want true")
	}
}

// Test_Float64_KindAndValue verifies that Float64 sets KindFloat and stores
// the float64 value in the inline floatVal field.
func Test_Float64_KindAndValue(t *testing.T) {
	a := model.Float64("r", 3.14)
	if a.Kind() != model.KindFloat {
		t.Errorf("Kind=%v want KindFloat", a.Kind())
	}
	if a.FloatVal() != 3.14 {
		t.Errorf("FloatVal=%v want 3.14", a.FloatVal())
	}
}

// Test_Uint_KindAndValue verifies that Uint sets KindUint and stores the
// uint64 value in the inline uintVal field.
func Test_Uint_KindAndValue(t *testing.T) {
	a := model.Uint("u", 99)
	if a.Kind() != model.KindUint {
		t.Errorf("Kind=%v want KindUint", a.Kind())
	}
	if a.UintVal() != 99 {
		t.Errorf("UintVal=%d want 99", a.UintVal())
	}
}

// Test_LogAttr_BackwardCompat_KindAny verifies that the struct-literal form
// (LogAttr{Key:k, Value:v}) still produces KindAny so existing callers are
// unaffected by the P2 changes.
func Test_LogAttr_BackwardCompat_KindAny(t *testing.T) {
	a := model.LogAttr{Key: "legacy", Value: "val"}
	if a.Kind() != model.KindAny {
		t.Errorf("struct literal Kind=%v want KindAny", a.Kind())
	}
}

// Test_Str_ZeroAlloc confirms that Str performs zero heap allocations, which
// is the primary goal of Story P2 (eliminate interface{} boxing on the hot path).
func Test_Str_ZeroAlloc(t *testing.T) {
	allocs := testing.AllocsPerRun(100, func() { _ = model.Str("k", "v") })
	if allocs != 0 {
		t.Errorf("Str allocs=%.0f want 0", allocs)
	}
}

// Test_Int_ZeroAlloc confirms that Int performs zero heap allocations.
func Test_Int_ZeroAlloc(t *testing.T) {
	allocs := testing.AllocsPerRun(100, func() { _ = model.Int("k", 99) })
	if allocs != 0 {
		t.Errorf("Int allocs=%.0f want 0", allocs)
	}
}

// Test_Bool_ZeroAlloc confirms that Bool performs zero heap allocations.
func Test_Bool_ZeroAlloc(t *testing.T) {
	allocs := testing.AllocsPerRun(100, func() { _ = model.Bool("k", true) })
	if allocs != 0 {
		t.Errorf("Bool allocs=%.0f want 0", allocs)
	}
}
