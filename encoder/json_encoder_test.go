package encoder

import (
	"reflect"
	"testing"
)

// Test_JSONEncoder_NoEmbeddedNilEncoder asserts JSONEncoder carries no
// surviving fields — the dead *json.Encoder slot from D-17 is gone.
// why: defensive against future refactors that may switch NewJSONEncoder
// to return by value; reflect.Elem() only unwraps pointers, so we check
// the kind before dereferencing (QA-security P3, PR #50 cycle 1).
func Test_JSONEncoder_NoEmbeddedNilEncoder(t *testing.T) {
	t.Parallel()

	enc := NewJSONEncoder()
	v := reflect.ValueOf(enc)
	if v.Kind() == reflect.Pointer {
		v = v.Elem()
	}
	if v.NumField() != 0 {
		t.Fatalf("JSONEncoder must have zero fields after D-17; got %d", v.NumField())
	}
}

// Test_JSONEncoder_Append_WrapsInBraces locks the externally observable
// behavior: Append returns `{<in>}\n`.
func Test_JSONEncoder_Append_WrapsInBraces(t *testing.T) {
	t.Parallel()

	enc := NewJSONEncoder()
	got := enc.Append(nil, []byte("key:value"))
	if string(got) != "{key:value}\n" {
		t.Fatalf("got %q, want %q", string(got), "{key:value}\n")
	}
}
