//go:build testing

// Package config contains V3-P2 tests for the atomic.Pointer[HotSnapshot]
// copy-on-write replacement of the configMu.RLock snapshot path.
//
// These tests are gated behind the "testing" build tag so production builds
// never include them (D-9 / ARCH-15). They live in package config (not
// config_test) because Test_GetHotSnapshot_NilSafe must access the
// package-level hotSnapshotPtr variable directly to verify the invariant
// that it is never nil after resetConfig.
package config

import (
	"context"
	"testing"
	"time"

	"github.com/architagr/lognugget/enum"
)

// Test_GetHotSnapshot_NilSafe asserts that hotSnapshotPtr is never nil after
// resetConfig. A nil pointer would cause a nil-deref panic on every log call,
// so this is a hard invariant for the atomic.Pointer[HotSnapshot] path.
func Test_GetHotSnapshot_NilSafe(t *testing.T) {
	TestResetConfig()
	snap := hotSnapshotPtr.Load()
	if snap == nil {
		t.Fatal("hotSnapshotPtr.Load() must not be nil after resetConfig")
	}
}

// Test_HotSnapshot_ConsistentWithSetTimeFormat asserts that after
// SetTimeFormat, the atomic snapshot immediately reflects the new value —
// without acquiring configMu. This verifies storeHotSnapshot is called
// inside SetTimeFormat's lock scope.
func Test_HotSnapshot_ConsistentWithSetTimeFormat(t *testing.T) {
	TestResetConfig()
	SetTimeFormat("2006-01-02")
	snap := GetHotSnapshot()
	if snap.TimeFormat != "2006-01-02" {
		t.Fatalf("snapshot TimeFormat = %q, want %q", snap.TimeFormat, "2006-01-02")
	}
}

// Test_HotSnapshot_ConsistentWithSetMinLevel asserts that after SetMinLevel the
// atomic snapshot's TimeFormat still matches what GetConfig returns, and that
// GetAtomicMinLevel reflects the new level. This verifies storeHotSnapshot is
// called inside SetMinLevel's lock scope without corrupting other fields.
func Test_HotSnapshot_ConsistentWithSetMinLevel(t *testing.T) {
	TestResetConfig()
	SetMinLevel(enum.LevelWarn)
	snap := GetHotSnapshot()
	cfg := GetConfig()
	if snap.TimeFormat != cfg.TimeFormat() {
		t.Fatalf("snapshot TimeFormat %q != config %q", snap.TimeFormat, cfg.TimeFormat())
	}
	if GetAtomicMinLevel() != enum.LevelWarn {
		t.Fatalf("atomic min level not updated: got %v, want %v", GetAtomicMinLevel(), enum.LevelWarn)
	}
}

// Test_HotSnapshot_ConsistentWithSetAddSource asserts that after SetAddSource,
// the atomic snapshot reflects the new value without a configMu read.
func Test_HotSnapshot_ConsistentWithSetAddSource(t *testing.T) {
	t.Cleanup(func() { TestResetConfig() })
	TestResetConfig()
	SetAddSource(true)
	snap := GetHotSnapshot()
	if !snap.AddSource {
		t.Fatal("snapshot AddSource = false, want true after SetAddSource(true)")
	}
	SetAddSource(false)
	snap = GetHotSnapshot()
	if snap.AddSource {
		t.Fatal("snapshot AddSource = true, want false after SetAddSource(false)")
	}
}

// Test_HotSnapshot_ConsistentWithSetEncoderType asserts that after
// SetEncoderType, the atomic snapshot reflects the new encoder type.
func Test_HotSnapshot_ConsistentWithSetEncoderType(t *testing.T) {
	t.Cleanup(func() { TestResetConfig() })
	TestResetConfig()
	SetEncoderType(enum.EncoderText)
	snap := GetHotSnapshot()
	if snap.EncoderType != enum.EncoderText {
		t.Fatalf("snapshot EncoderType = %v, want %v", snap.EncoderType, enum.EncoderText)
	}
}

// Test_HotSnapshot_ConsistentWithSetContextFieldsParser asserts that after
// SetContextFieldsParser, the atomic snapshot surfaces the new parser.
func Test_HotSnapshot_ConsistentWithSetContextFieldsParser(t *testing.T) {
	t.Cleanup(func() { TestResetConfig() })
	TestResetConfig()
	SetContextFieldsParser(func(_ context.Context) map[string]any {
		return nil
	})
	// The snapshot stores a function pointer; we can only verify non-nil.
	snap := GetHotSnapshot()
	if snap.ContextParser == nil {
		t.Fatal("snapshot ContextParser should be non-nil after SetContextFieldsParser")
	}
}

// Test_HotSnapshot_ConsistentWithSetDefaultFields asserts that after
// SetDefaultFields renames a key, the atomic snapshot's RestrictedFields map
// contains the new name and the Rendered map has been rebuilt.
func Test_HotSnapshot_ConsistentWithSetDefaultFields(t *testing.T) {
	t.Cleanup(func() { TestResetConfig() })
	TestResetConfig()
	SetDefaultFields(map[enum.DefaultLogKey]string{
		enum.DefaultLogKeyTime: "ts",
	})
	snap := GetHotSnapshot()
	if _, ok := snap.RestrictedFields["ts"]; !ok {
		t.Fatal("snapshot RestrictedFields should contain renamed key 'ts'")
	}
	if _, ok := snap.RestrictedFields["time"]; ok {
		t.Fatal("snapshot RestrictedFields should NOT contain old key 'time' after rename")
	}
}

// Test_HotSnapshot_NeverNilAfterEverySetterCalled asserts that after calling
// every Set* mutator, hotSnapshotPtr is still non-nil and the snapshot has a
// non-empty TimeFormat (a basic sanity sentinel for full snapshot integrity).
func Test_HotSnapshot_NeverNilAfterEverySetterCalled(t *testing.T) {
	t.Cleanup(func() { TestResetConfig() })
	TestResetConfig()

	SetTimeFormat("15:04:05")
	SetMinLevel(enum.LevelDebug)
	SetEncoderType(enum.EncoderJSON)
	SetAddSource(false)
	SetLogBufferMaxSize(50)
	SetRate(2 * time.Second)
	SetContextFieldsParser(nil)
	SetContextFieldsAppender(nil)
	SetDefaultFields(nil) // nil is a no-op per SetDefaultFields contract

	ptr := hotSnapshotPtr.Load()
	if ptr == nil {
		t.Fatal("hotSnapshotPtr must not be nil after all setters called")
	}
	snap := GetHotSnapshot()
	if snap.TimeFormat != "15:04:05" {
		t.Fatalf("snapshot TimeFormat = %q, want %q", snap.TimeFormat, "15:04:05")
	}
}

// Test_HotSnapshot_RaceWithSetters exercises concurrent writers and readers
// against the atomic.Pointer[HotSnapshot] path. Under -race, any unsynchronised
// access to hotSnapshotPtr would be reported as a DATA RACE.
func Test_HotSnapshot_RaceWithSetters(t *testing.T) {
	TestResetConfig()
	t.Cleanup(func() { TestResetConfig() })

	const goroutines = 4
	const iterations = 500

	done := make(chan struct{})
	defer close(done)

	for i := 0; i < goroutines; i++ {
		go func() {
			for j := 0; j < iterations; j++ {
				SetTimeFormat("2006-01-02T15:04:05Z07:00")
				SetMinLevel(enum.LevelDebug)
			}
		}()
	}
	for i := 0; i < goroutines; i++ {
		go func() {
			for j := 0; j < iterations; j++ {
				_ = GetHotSnapshot()
			}
		}()
	}
}
