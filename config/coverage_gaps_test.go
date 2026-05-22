//go:build testing

package config_test

import (
	"bytes"
	"testing"
	"time"

	"github.com/architagr/lognugget/config"
	"github.com/architagr/lognugget/enum"
	"github.com/architagr/lognugget/model"
)

// --- Config accessor coverage ---

func Test_Config_StaticFields(t *testing.T) {
	t.Cleanup(config.TestResetConfig)
	config.SetStaticEnvFieldsParser(func() map[string]any { return map[string]any{"env": "test"} })
	if got := config.GetConfig().StaticFields(); got == "" {
		t.Error("StaticFields() should be non-empty after SetStaticEnvFieldsParser")
	}
}

func Test_Config_ContextParser(t *testing.T) {
	t.Cleanup(config.TestResetConfig)
	if got := config.GetConfig().ContextParser(); got != nil {
		t.Error("default ContextParser should be nil")
	}
}

func Test_Config_TimeFormat(t *testing.T) {
	t.Cleanup(config.TestResetConfig)
	if got := config.GetConfig().TimeFormat(); got == "" {
		t.Error("TimeFormat() should be non-empty by default")
	}
}

func Test_Config_Encoder(t *testing.T) {
	t.Cleanup(config.TestResetConfig)
	if got := config.GetConfig().Encoder(); got == nil {
		t.Error("Encoder() should be non-nil by default")
	}
}

// --- Setter missing-branch coverage ---

func Test_SetOutput_NilFallsBackToDefault(t *testing.T) {
	t.Cleanup(config.TestResetConfig)
	config.SetOutput(nil)
	if got := config.GetConfig().Output(); got == nil {
		t.Error("SetOutput(nil) should fall back to DefaultOutput, not nil")
	}
}

func Test_SetRate_ZeroFallsBackToDefault(t *testing.T) {
	t.Cleanup(config.TestResetConfig)
	config.SetRate(0)
	if got := config.GetConfig().Rate(); got != time.Second {
		t.Errorf("SetRate(0) = %v; want 1s", got)
	}
}

func Test_SetEncoderType_UnknownFallsBackToJSON(t *testing.T) {
	t.Cleanup(config.TestResetConfig)
	config.SetEncoderType(enum.LogEncodeType("bogus"))
	if got := config.GetConfig().Encoder(); got == nil {
		t.Error("SetEncoderType(unknown) should fall back to JSON encoder")
	}
}

// --- AppendAttr KindAny uncovered type paths ---

func Test_AppendAttr_KindAny_IntVariants(t *testing.T) {
	cases := []struct {
		name string
		attr model.LogAttr
		want string
	}{
		{"int8", model.LogAttr{Key: "k", Value: int8(1)}, `"k":1`},
		{"int16", model.LogAttr{Key: "k", Value: int16(2)}, `"k":2`},
		{"int32", model.LogAttr{Key: "k", Value: int32(3)}, `"k":3`},
		{"uint", model.LogAttr{Key: "k", Value: uint(4)}, `"k":4`},
		{"uint8", model.LogAttr{Key: "k", Value: uint8(5)}, `"k":5`},
		{"uint16", model.LogAttr{Key: "k", Value: uint16(6)}, `"k":6`},
		{"uint32", model.LogAttr{Key: "k", Value: uint32(7)}, `"k":7`},
		{"uint64", model.LogAttr{Key: "k", Value: uint64(8)}, `"k":8`},
		{"bool", model.LogAttr{Key: "k", Value: true}, `"k":true`},
	}
	for _, tc := range cases {
		got := string(config.AppendAttr(nil, "k", tc.attr))
		if got != tc.want {
			t.Errorf("%s: got %q want %q", tc.name, got, tc.want)
		}
	}
}

func Test_AppendAttr_KindAny_Float32(t *testing.T) {
	got := string(config.AppendAttr(nil, "k", model.LogAttr{Key: "k", Value: float32(1.5)}))
	if got != `"k":1.5` {
		t.Errorf("got %q want %q", got, `"k":1.5`)
	}
}

func Test_AppendAttr_KindAny_Float32_NaN(t *testing.T) {
	got := string(config.AppendAttr(nil, "k", model.LogAttr{Key: "k", Value: float32(float64(^uint32(0)>>1 + 1))}))
	// any non-panic result is acceptable; just verify it doesn't crash
	_ = got
}

// --- appendJSONString invalid UTF-8 path (via AppendAttr KindStr) ---

func Test_AppendAttr_Str_InvalidUTF8(t *testing.T) {
	// A lone 0xFF byte is invalid UTF-8; AppendAttr must not panic.
	got := config.AppendAttr(nil, "k", model.Str("k", "\xff"))
	if len(got) == 0 {
		t.Error("expected non-empty output for invalid UTF-8 input")
	}
}

func Test_AppendAttr_Str_MultibyteUTF8(t *testing.T) {
	got := string(config.AppendAttr(nil, "k", model.Str("k", "日本語")))
	if got != `"k":"日本語"` {
		t.Errorf("got %q want %q", got, `"k":"日本語"`)
	}
}

// --- SetDefaultFields missing branch ---

func Test_SetDefaultFields_WithOutput(t *testing.T) {
	t.Cleanup(config.TestResetConfig)
	var buf bytes.Buffer
	config.SetOutput(&buf)
	config.SetDefaultFields(map[enum.DefaultLogKey]string{
		enum.DefaultLogKeyLevel: "severity",
	})
	if got := config.GetConfig().DefaultFields(); got[enum.DefaultLogKeyLevel] != "severity" {
		t.Errorf("DefaultFields level key = %q; want %q", got[enum.DefaultLogKeyLevel], "severity")
	}
}
