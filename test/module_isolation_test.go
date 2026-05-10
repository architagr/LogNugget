// Package moduleisolation enforces NF12: the library go.mod is stdlib + testify only.
package moduleisolation

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// bannedDeps lists module paths that must not appear in the library go.mod.
// why: NF12 requires zero non-test runtime deps so library consumers don't
// inherit gin/zerolog into their dep graphs. Demos and comparison benches
// live in nested modules outside the library tree.
var bannedDeps = []string{
	"github.com/gin-gonic/gin",
	"github.com/rs/zerolog",
}

// TestLibraryGoMod_HasNoBannedDeps fails if the repo-root go.mod still names
// any dep listed in bannedDeps. It reads ../go.mod relative to the test file.
func TestLibraryGoMod_HasNoBannedDeps(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	goModPath := filepath.Join(wd, "..", "go.mod")
	data, err := os.ReadFile(goModPath)
	if err != nil {
		t.Fatalf("read %s: %v", goModPath, err)
	}
	content := string(data)
	for _, dep := range bannedDeps {
		if strings.Contains(content, dep) {
			t.Errorf("library go.mod must not reference %q (NF12 violation)", dep)
		}
	}
}

// TestLibraryRoot_HasNoDemoLogger fails if logger.go still sits at the repo
// root. The demo belongs under examples/gin-demo with its own module.
func TestLibraryRoot_HasNoDemoLogger(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	loggerPath := filepath.Join(wd, "..", "logger.go")
	if _, err := os.Stat(loggerPath); err == nil {
		t.Errorf("repo-root logger.go still present; demo must live under examples/gin-demo")
	}
}
