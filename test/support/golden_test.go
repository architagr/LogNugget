//go:build testing

package support_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/architagr/lognugget/test/support"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGolden_MatchPassesSilently verifies that Golden does not fail the test
// when got matches the fixture on disk exactly.
func TestGolden_MatchPassesSilently(t *testing.T) {
	t.Parallel()

	// Write fixture to a real temp dir and resolve a stable relative path via
	// a known-good fixture name. We use GoldenUpdate to create it, then
	// Golden to verify. This avoids os.Chdir (process-global, unsafe with
	// t.Parallel).
	dir := t.TempDir()
	name := "match_silent"
	content := []byte(`{"ok":true}`)

	// Plant the fixture.
	goldenDir := filepath.Join(dir, "testdata", "golden")
	require.NoError(t, os.MkdirAll(goldenDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(goldenDir, name+".json"), content, 0644))

	// Golden reads relative to cwd; use the abs-path variant so we don't
	// need to chdir. We call the exported path-based helper directly.
	support.GoldenFromDir(t, dir, name, content)
}

// TestGolden_MismatchFailsTest verifies that Golden marks the test as failed
// when got differs from the fixture content.
func TestGolden_MismatchFailsTest(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	name := "mismatch"
	goldenDir := filepath.Join(dir, "testdata", "golden")
	require.NoError(t, os.MkdirAll(goldenDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(goldenDir, name+".json"), []byte(`{"want":"this"}`), 0644))

	spy := &spyTB{TB: t}
	support.GoldenFromDir(spy, dir, name, []byte(`{"got":"something-else"}`))
	assert.True(t, spy.failed, "Golden must mark the test failed on content mismatch")
}

// TestGolden_MissingFileFailsTest verifies that Golden fails the test when
// the fixture file does not exist (absence is treated as a test failure).
func TestGolden_MissingFileFailsTest(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	spy := &spyTB{TB: t}
	support.GoldenFromDir(spy, dir, "nonexistent", []byte(`{}`))
	assert.True(t, spy.failed, "Golden must mark the test failed when fixture is missing")
}

// TestGolden_UpdateFlagRewritesFile verifies that GoldenUpdate overwrites
// the fixture file with new content unconditionally, creating dirs as needed.
func TestGolden_UpdateFlagRewritesFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	name := "update_target"
	newContent := []byte(`{"updated":true}`)

	support.GoldenUpdateFromDir(t, dir, name, newContent)

	got, err := os.ReadFile(filepath.Join(dir, "testdata", "golden", name+".json"))
	require.NoError(t, err)
	assert.Equal(t, string(newContent), string(got))
}

// spyTB wraps testing.TB and captures whether any failure method was called,
// so golden-mismatch tests can assert failure without killing the outer test.
type spyTB struct {
	testing.TB
	failed bool
}

// Fail records that the test was marked failed.
func (s *spyTB) Fail() { s.failed = true }

// FailNow records failure; does not call runtime.Goexit so the caller
// continues executing — safe within a test goroutine.
func (s *spyTB) FailNow() { s.failed = true }

// Fatal records failure (variadic form used by require).
func (s *spyTB) Fatal(args ...any) { s.failed = true }

// Fatalf records failure (formatted form used by require).
func (s *spyTB) Fatalf(format string, args ...any) { s.failed = true }

// Error records failure (non-fatal, used by assert).
func (s *spyTB) Error(args ...any) { s.failed = true }

// Errorf records failure (non-fatal formatted, used by assert).
func (s *spyTB) Errorf(format string, args ...any) { s.failed = true }

// Helper is a no-op so require/assert helpers don't panic on nil frame.
func (s *spyTB) Helper() {}
