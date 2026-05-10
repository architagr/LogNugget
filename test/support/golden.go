//go:build testing

package support

import (
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// update is registered once at package init so any test binary built with
// -tags testing can pass -update on the command line to regenerate fixtures.
// why: flag.Bool panics on duplicate registration; a package-level var
// guarantees exactly one registration per binary regardless of how many
// tests call Golden.
var update = flag.Bool("update", false, "rewrite golden fixture files")

// Golden compares got against testdata/golden/<name>.json relative to the
// current working directory. When the process-wide -update flag is set the
// fixture is overwritten with got instead of compared.
//
// tb.Fatal is called on IO error or mismatch; treat this call as terminal.
// For parallel-safe usage without os.Chdir, prefer GoldenFromDir.
func Golden(tb testing.TB, name string, got []byte) {
	tb.Helper()
	GoldenFromDir(tb, ".", name, got)
}

// GoldenFromDir is the parallel-safe variant of Golden that resolves the
// fixture path relative to baseDir instead of the process working directory.
// Prefer this in tests that run with t.Parallel() to avoid chdir races.
func GoldenFromDir(tb testing.TB, baseDir, name string, got []byte) {
	tb.Helper()
	path := filepath.Join(baseDir, "testdata", "golden", name+".json")
	if *update {
		writeGolden(tb, path, got)
		return
	}
	want, err := os.ReadFile(path)
	require.NoError(tb, err)
	require.Equal(tb, string(want), string(got))
}

// GoldenUpdate unconditionally writes got to testdata/golden/<name>.json
// relative to the current working directory, creating the directory tree as
// needed. Use this in tests that verify the update path without toggling the
// process-wide -update flag.
func GoldenUpdate(tb testing.TB, name string, got []byte) {
	tb.Helper()
	GoldenUpdateFromDir(tb, ".", name, got)
}

// GoldenUpdateFromDir is the parallel-safe variant of GoldenUpdate that
// resolves the fixture path relative to baseDir.
func GoldenUpdateFromDir(tb testing.TB, baseDir, name string, got []byte) {
	tb.Helper()
	path := filepath.Join(baseDir, "testdata", "golden", name+".json")
	writeGolden(tb, path, got)
}

// writeGolden creates parent directories and writes data to path.
func writeGolden(tb testing.TB, path string, data []byte) {
	tb.Helper()
	require.NoError(tb, os.MkdirAll(filepath.Dir(path), 0755))
	require.NoError(tb, os.WriteFile(path, data, 0644))
}
