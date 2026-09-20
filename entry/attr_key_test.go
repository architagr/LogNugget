//go:build testing

// V4-P4 regression guard: attaching a field must not allocate for its key.
package entry_test

import (
	"testing"

	"github.com/architagr/lognugget/v4/config"
	"github.com/architagr/lognugget/v4/model"
)

// Test_LogAttr_KeyCostsNoAllocation pins the property V4-P4 (#135) asked for.
//
// why this is a guard rather than a change: the issue assumed
// model.LogAttr.Key was []byte and that entry.go paid a string([]byte)
// conversion per field. LogAttrKey has been declared `string` since the first
// configuration commit, so the conversion is string→string — the compiler
// emits no copy and no allocation. This test fails if anyone widens the key
// type back to []byte.
func Test_LogAttr_KeyCostsNoAllocation(t *testing.T) {
	attr := model.Str("request_id", "req-abc")
	buf := make([]byte, 0, 256)

	allocs := testing.AllocsPerRun(100, func() {
		buf = config.AppendAttr(buf[:0], string(attr.Key), attr)
	})

	if allocs != 0 {
		t.Errorf("AppendAttr with a string key = %.0f allocs; want 0", allocs)
	}
}

// Test_LogAttrKey_IsString documents the type the guard above depends on.
func Test_LogAttrKey_IsString(t *testing.T) {
	var key model.LogAttrKey = "k"
	if string(key) != "k" {
		t.Fatalf("LogAttrKey must be a string type; got %v", key)
	}
}
