//go:build testing

// Package config_test verifies the P1 atomic level gate and HotSnapshot.
// Tests in this file exercise the new GetAtomicMinLevel and GetHotSnapshot
// functions added for Epic V2 story P1.
package config_test

import (
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
