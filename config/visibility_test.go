//go:build testing

// Package config_test verifies the test-only access shim for resetting
// the package-level config singleton (story 006, fixes D-9). The
// production identifier `resetConfig` is unexported; cross-package
// tests must reach it through the build-tagged TestResetConfig helper
// declared in test_only_helpers.go.
package config_test

import (
	"testing"

	"github.com/architagr/lognugget/config"
	"github.com/architagr/lognugget/enum"
)

// Test_TestResetConfig_RestoresDefaults asserts that the test-only
// helper resets the singleton back to package defaults so cleanup
// callers (notably test/support.ConfigBuilder.Build) get deterministic
// state between tests.
func Test_TestResetConfig_RestoresDefaults(t *testing.T) {
	// Mutate state away from defaults.
	config.SetMinLevel(enum.LevelError)
	config.SetEncoderType(enum.EncoderText)
	config.SetAddSource(false)

	if got := config.GetConfig().MinLevel(); got != enum.LevelError {
		t.Fatalf("precondition: SetMinLevel did not stick; got %v", got)
	}

	config.TestResetConfig()

	if got := config.GetConfig().MinLevel(); got != config.DafaultLevel {
		t.Errorf("MinLevel after TestResetConfig = %v, want %v", got, config.DafaultLevel)
	}
	if got := config.GetConfig().EncoderType(); got != config.DafaultEncoderType {
		t.Errorf("EncoderType after TestResetConfig = %v, want %v", got, config.DafaultEncoderType)
	}
	if got := config.GetConfig().AddSource(); got != config.DafaultAddSource {
		t.Errorf("AddSource after TestResetConfig = %v, want %v", got, config.DafaultAddSource)
	}
}

// Test_TestResetConfig_RebuildsConfigStruct guards that callers receive
// a non-nil *Config after the reset — protects against a future change
// that nils the singleton mid-reset and breaks every test using the
// support builder cleanup.
func Test_TestResetConfig_RebuildsConfigStruct(t *testing.T) {
	config.TestResetConfig()
	if config.GetConfig() == nil {
		t.Fatal("GetConfig() returned nil after TestResetConfig")
	}
}
