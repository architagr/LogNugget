//go:build testing

// Package config_test contains race-safety tests for the config globals.
// These tests specifically target the data races identified in issue #54:
// concurrent writes via Set* functions and reads/writes via TestResetConfig
// against the ProcessLogEvent goroutine.
package config_test

import (
	"sync"
	"testing"

	"github.com/architagr/lognugget/config"
	"github.com/architagr/lognugget/enum"
)

// Test_Race_ResetConfig_UnderConcurrentSet spawns writer goroutines calling
// SetMinLevel and SetEncoderType concurrently with goroutines calling
// TestResetConfig. The test must produce zero DATA RACE reports when run
// with `go test -race -tags testing -count=10 ./config/...`.
//
// why: the three package-level globals (defaultConfig, ch,
// EventPreProcessors) were unprotected, causing the race detector to
// fire on every concurrent Set* / TestResetConfig combination. This
// test is the canonical proof that issue #54 is resolved.
func Test_Race_ResetConfig_UnderConcurrentSet(t *testing.T) {
	const (
		numWriters  = 10
		numResetters = 5
		iterations  = 20
	)

	var wg sync.WaitGroup

	// Writer goroutines: concurrently mutate config fields.
	for i := 0; i < numWriters; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				if (id+j)%2 == 0 {
					config.SetMinLevel(enum.LevelDebug)
				} else {
					config.SetMinLevel(enum.LevelError)
				}
				if (id+j)%3 == 0 {
					config.SetEncoderType(enum.EncoderText)
				} else {
					config.SetEncoderType(enum.EncoderJSON)
				}
			}
		}(i)
	}

	// Resetter goroutines: concurrently reset config to defaults.
	for i := 0; i < numResetters; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				config.TestResetConfig()
			}
		}()
	}

	wg.Wait()

	// Restore known state for subsequent tests in the same run.
	config.TestResetConfig()
}

// Test_Race_ProcessLogEvent_UnderConcurrentReset verifies that the
// package-level ch pointer is accessed without a data race when
// PublishLog and TestResetConfig run concurrently.
//
// why: resetConfig creates a new channel and updates the ch pointer
// under a write lock; PublishLog snapshots ch under a read lock. The
// race detector must observe no unsynchronised access to the ch
// variable itself. Events sent to a stale channel reference are
// silently dropped (old ProcessLogEvent goroutine drains them), which
// is the accepted trade-off for this test-only reset path.
//
// Note: numPublishers * iterations must stay well below the channel
// buffer (10) per reset cycle, or the old goroutine must drain fast
// enough. We keep the publish load low and rely on buffering.
func Test_Race_ProcessLogEvent_UnderConcurrentReset(t *testing.T) {
	const (
		numPublishers = 3
		numResetters  = 3
		iterations    = 3
	)

	var wg sync.WaitGroup

	// Resetter goroutines: swap ch + defaultConfig concurrently.
	for i := 0; i < numResetters; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				config.TestResetConfig()
			}
		}()
	}

	// Publisher goroutines: send events to whatever ch currently points at.
	// Runs after resetters have started to maximise interleaving.
	for i := 0; i < numPublishers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				config.PublishLog(enum.LevelInfo, []byte("race-test-payload"))
			}
		}()
	}

	wg.Wait()
	config.TestResetConfig()
}

// Test_Race_GetConfig_UnderConcurrentSet verifies that GetConfig reads
// a consistent pointer while Set* functions write fields on the same
// *Config.
//
// why: GetConfig returns the raw defaultConfig pointer. If resetConfig
// assigns a new *Config while a caller is mid-read of a field, the
// result is a data race on the pointer itself.
func Test_Race_GetConfig_UnderConcurrentSet(t *testing.T) {
	const (
		numReaders  = 8
		numWriters  = 8
		iterations  = 25
	)

	var wg sync.WaitGroup

	for i := 0; i < numReaders; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				cfg := config.GetConfig()
				if cfg == nil {
					t.Errorf("GetConfig returned nil during concurrent writes")
					return
				}
				_ = cfg.MinLevel()
			}
		}()
	}

	for i := 0; i < numWriters; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				if (id+j)%2 == 0 {
					config.SetMinLevel(enum.LevelWarn)
				} else {
					config.SetTimeFormat("2006-01-02")
				}
			}
		}(i)
	}

	wg.Wait()
	config.TestResetConfig()
}
