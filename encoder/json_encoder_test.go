package encoder

import (
	"reflect"
	"testing"
)

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

func Test_JSONEncoder_Append_WrapsInBraces(t *testing.T) {
	t.Parallel()

	enc := NewJSONEncoder()
	got := enc.Append(nil, []byte("key:value"))
	if string(got) != "{key:value}\n" {
		t.Fatalf("got %q, want %q", string(got), "{key:value}\n")
	}
}

// Test_JSONEncoder_Append_EscapedBody verifies Append passes body bytes through
// verbatim — escaping is the caller's responsibility (done by config.AppendQuotedString).
func Test_JSONEncoder_Append_EscapedBody(t *testing.T) {
	t.Parallel()

	enc := NewJSONEncoder()
	// Body already escaped by the caller; Append must not re-escape.
	body := []byte(`"key":"say \"hi\"\n"`)
	got := enc.Append(nil, body)
	want := "{" + string(body) + "}\n"
	if string(got) != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
