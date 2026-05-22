//go:build testing

// Package config_test verifies the atomic preprocessor gate introduced in V3-P1.
// These tests exercise HasEventPreProcessors, InitPreProcessors, AddPreProcessors,
// and RemovePreProcessor to verify the atomic.Bool mirrors map state correctly
// under concurrent access.
package config_test

import (
	"sync"
	"testing"

	"github.com/architagr/lognugget/config"
	"github.com/architagr/lognugget/test/support"
)

// Test_HasPreProcessors_FalseOnEmpty asserts that HasEventPreProcessors returns
// false when InitPreProcessors is called with no arguments.
func Test_HasPreProcessors_FalseOnEmpty(t *testing.T) {
	t.Cleanup(func() { config.TestResetConfig() })
	config.TestResetConfig()
	config.InitPreProcessors()
	if config.HasEventPreProcessors() {
		t.Fatal("expected false for empty processor set")
	}
}

// Test_HasPreProcessors_TrueAfterInit asserts that HasEventPreProcessors returns
// true after InitPreProcessors is called with at least one processor.
func Test_HasPreProcessors_TrueAfterInit(t *testing.T) {
	t.Cleanup(func() { config.TestResetConfig() })
	config.TestResetConfig()
	fp := support.NewFakePreProc("p1")
	config.InitPreProcessors(fp)
	if !config.HasEventPreProcessors() {
		t.Fatal("expected true after InitPreProcessors with one processor")
	}
}

// Test_HasPreProcessors_FalseAfterRemove asserts that HasEventPreProcessors returns
// false after the last processor is removed via RemovePreProcessor.
func Test_HasPreProcessors_FalseAfterRemove(t *testing.T) {
	t.Cleanup(func() { config.TestResetConfig() })
	config.TestResetConfig()
	fp := support.NewFakePreProc("p1")
	config.InitPreProcessors(fp)
	config.RemovePreProcessor(fp.Name())
	if config.HasEventPreProcessors() {
		t.Fatal("expected false after removing last processor")
	}
}

// Test_HasPreProcessors_TrueAfterAdd asserts that HasEventPreProcessors returns
// true after a processor is registered via AddPreProcessors.
func Test_HasPreProcessors_TrueAfterAdd(t *testing.T) {
	t.Cleanup(func() { config.TestResetConfig() })
	config.TestResetConfig()
	fp := support.NewFakePreProc("p1")
	config.AddPreProcessors(fp)
	if !config.HasEventPreProcessors() {
		t.Fatal("expected true after AddPreProcessors")
	}
}

// Test_HasPreProcessors_FalseAfterReset asserts that HasEventPreProcessors returns
// false immediately after TestResetConfig clears all state.
func Test_HasPreProcessors_FalseAfterReset(t *testing.T) {
	t.Cleanup(func() { config.TestResetConfig() })
	config.TestResetConfig()
	fp := support.NewFakePreProc("p1")
	config.InitPreProcessors(fp)
	config.TestResetConfig()
	if config.HasEventPreProcessors() {
		t.Fatal("expected false after TestResetConfig")
	}
}

// Test_HasPreProcessors_Race verifies that concurrent adds, removes, and reads
// of HasEventPreProcessors produce no data races under -race. Four goroutines
// repeatedly add and remove a processor while four readers poll HasEventPreProcessors.
func Test_HasPreProcessors_Race(t *testing.T) {
	t.Cleanup(func() { config.TestResetConfig() })
	config.TestResetConfig()
	fp := support.NewFakePreProc("p1")
	config.InitPreProcessors(fp)
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 1000; j++ {
				config.AddPreProcessors(support.NewFakePreProc("tmp"))
				config.RemovePreProcessor("tmp")
			}
		}()
	}
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 1000; j++ {
				_ = config.HasEventPreProcessors()
			}
		}()
	}
	wg.Wait()
}
