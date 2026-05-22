//go:build testing

// Package config exposes a test-only escape hatch for resetting the
// singleton state used by *_test.go in this module. The file is gated
// behind the `testing` build tag so production builds never see this
// surface — addressing D-9 / ARCH-15 (NF5: config setters are
// startup-only).
//
// Callers: package config tests, plus the test/support builder, both
// of which compile only with `go test -tags testing` (see makefile and
// .github/workflows/test.yml).
package config

// TestResetConfig restores the singleton to its package defaults.
//
// why: cross-package tests need to reach the unexported resetConfig.
// A `_test.go` shim cannot satisfy this because test/support is a
// regular importable package, not a `_test` package. The build-tagged
// helper keeps the production API surface free of this foot-gun while
// still letting test/support and config_test reach the reset path.
func TestResetConfig() { resetConfig() }

// TestResetChannelCapacity resets channelCapacity to DafaultLogBuffer.
// Call before TestResetConfig in capacity-specific tests that need a known state.
func TestResetChannelCapacity() { channelCapacity.Store(int64(DafaultLogBuffer)) }
