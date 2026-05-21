//go:build testing

// Package config_test verifies the P1 atomic level gate and HotSnapshot.
// Tests in this file exercise the new GetAtomicMinLevel and GetHotSnapshot
// functions added for Epic V2 story P1.
package config_test

import (
	"context"
	"testing"

	"github.com/architagr/lognugget/config"
	"github.com/architagr/lognugget/enum"
)

// Test_GetAtomicMinLevel_AfterSetMinLevel asserts that SetMinLevel stores the
// level in the atomic so GetAtomicMinLevel returns it lock-free.
func Test_GetAtomicMinLevel_AfterSetMinLevel(t *testing.T) {
	config.TestResetConfig()
	config.SetMinLevel(enum.LevelWarn)
	if got := config.GetAtomicMinLevel(); got != enum.LevelWarn {
		t.Errorf("GetAtomicMinLevel() = %v, want %v", got, enum.LevelWarn)
	}
}

// Test_GetAtomicMinLevel_AfterReset asserts that TestResetConfig restores the
// atomic to DafaultLevel (LevelInfo).
func Test_GetAtomicMinLevel_AfterReset(t *testing.T) {
	config.TestResetConfig()
	if got := config.GetAtomicMinLevel(); got != config.DafaultLevel {
		t.Errorf("after reset: GetAtomicMinLevel() = %v, want %v", got, config.DafaultLevel)
	}
}

// Test_GetHotSnapshot_ReflectsConfig asserts that GetHotSnapshot returns a
// snapshot that mirrors the current config state, including the Encoder,
// RestrictedFields, Rendered prefix map, and EncoderOpen/EncoderClose bytes.
func Test_GetHotSnapshot_ReflectsConfig(t *testing.T) {
	config.TestResetConfig()
	config.SetAddSource(true)
	snap := config.GetHotSnapshot()
	if !snap.AddSource {
		t.Error("AddSource should be true")
	}
	if snap.Encoder == nil {
		t.Error("Encoder should not be nil")
	}
	if snap.RestrictedFields == nil {
		t.Error("RestrictedFields should not be nil")
	}
	if snap.Rendered == nil {
		t.Error("Rendered should not be nil")
	}
	// JSON encoder must produce at least one of Open/Close bytes.
	if len(snap.EncoderOpen) == 0 && len(snap.EncoderClose) == 0 {
		t.Error("EncoderOpen or EncoderClose should be non-empty for JSON encoder")
	}
}

// Test_GetHotSnapshot_RestrictedFieldsIncludesDefaults asserts that the five
// core log-field names are always present in RestrictedFields so that user
// fields with the same names are correctly prefixed on the hot path.
func Test_GetHotSnapshot_RestrictedFieldsIncludesDefaults(t *testing.T) {
	config.TestResetConfig()
	snap := config.GetHotSnapshot()
	// "time", "level", "message", "error", "caller" must always be restricted.
	for _, key := range []string{"time", "level", "message", "error", "caller"} {
		if _, ok := snap.RestrictedFields[key]; !ok {
			t.Errorf("RestrictedFields missing %q", key)
		}
	}
}

// Test_SetContextFieldsAppender_Stored asserts that SetContextFieldsAppender
// stores the appender so GetHotSnapshot surfaces it as a non-nil ContextAppender.
func Test_SetContextFieldsAppender_Stored(t *testing.T) {
	config.TestResetConfig()
	var called bool
	config.SetContextFieldsAppender(func(ctx context.Context, dst []byte) []byte {
		called = true
		return append(dst, `,"svc":"test"`...)
	})
	snap := config.GetHotSnapshot()
	if snap.ContextAppender == nil {
		t.Fatal("ContextAppender should not be nil after SetContextFieldsAppender")
	}
	_ = snap.ContextAppender(context.Background(), nil)
	if !called {
		t.Error("appender was not called")
	}
}

// Test_AtomicMinLevel_ConsistentWithConfig asserts that GetAtomicMinLevel
// returns the exact level set via SetMinLevel for all defined levels.
func Test_AtomicMinLevel_ConsistentWithConfig(t *testing.T) {
	for _, level := range []enum.LogLevel{enum.LevelDebug, enum.LevelInfo, enum.LevelWarn, enum.LevelError} {
		config.TestResetConfig()
		config.SetMinLevel(level)
		if got := config.GetAtomicMinLevel(); got != level {
			t.Errorf("level=%v: GetAtomicMinLevel()=%v, want %v", level, got, level)
		}
	}
}

// Test_HotSnapshot_Fields asserts that a fresh GetHotSnapshot after reset
// contains non-nil/non-empty values for every mandatory hot-path field.
func Test_HotSnapshot_Fields(t *testing.T) {
	config.TestResetConfig()
	snap := config.GetHotSnapshot()
	if snap.DefaultFields == nil {
		t.Error("DefaultFields should not be nil")
	}
	if snap.Rendered == nil {
		t.Error("Rendered should not be nil")
	}
	if snap.RestrictedFields == nil {
		t.Error("RestrictedFields should not be nil")
	}
	if snap.Encoder == nil {
		t.Error("Encoder should not be nil")
	}
	if snap.EncoderOpen == nil && snap.EncoderClose == nil {
		t.Error("EncoderOpen and EncoderClose both nil — JSON encoder should return non-nil")
	}
	if snap.TimeFormat == "" {
		t.Error("TimeFormat should not be empty")
	}
}
