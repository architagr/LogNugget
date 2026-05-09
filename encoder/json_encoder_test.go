package encoder

import (
	"reflect"
	"testing"
)

// Test_JSONEncoder_NoEmbeddedNilEncoder asserts JSONEncoder carries no
// surviving fields — the dead *json.Encoder slot from D-17 is gone.
func Test_JSONEncoder_NoEmbeddedNilEncoder(t *testing.T) {
	t.Parallel()

	enc := NewJSONEncoder()
	v := reflect.ValueOf(enc).Elem()
	if v.NumField() != 0 {
		t.Fatalf("JSONEncoder must have zero fields after D-17; got %d", v.NumField())
	}
}

// Test_JSONEncoder_Write_WrapsInBraces locks the externally observable
// behavior: F7 contract Write(string) ([]byte, error) returns "{<in>}".
func Test_JSONEncoder_Write_WrapsInBraces(t *testing.T) {
	t.Parallel()

	enc := NewJSONEncoder()
	got, err := enc.Write("key:value")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != "{key:value}" {
		t.Fatalf("got %q, want %q", string(got), "{key:value}")
	}
}
