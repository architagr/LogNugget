package lognugget

import (
	"bytes"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/architagr/lognugget/config"
	"github.com/architagr/lognugget/enum"
	pipelineStage "github.com/architagr/lognugget/pipeline_stage"
)

// safeBuffer is a thread-safe io.Writer / string reader backed by bytes.Buffer.
// bytes.Buffer is not goroutine-safe; the flush goroutine writes while the test
// polls Len/String, so we need a mutex wrapper.
type safeBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (s *safeBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}

func (s *safeBuffer) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Len()
}

func (s *safeBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}

// Test_ZeroConfig_DefaultPostProcIsWired verifies that init() sets defaultPostProc
// to a non-nil unsetLogEventPostProcessor (SC1 / ARCH-5).
func Test_ZeroConfig_DefaultPostProcIsWired(t *testing.T) {
	if defaultPostProc == nil {
		t.Fatal("defaultPostProc is nil after init; zero-config wiring failed")
	}
	if got := defaultPostProc.Name(); got != "unsetLogEventPostProcessor" {
		t.Errorf("defaultPostProc.Name() = %q; want %q", got, "unsetLogEventPostProcessor")
	}
}

// Test_ZeroConfig_NoCallerByDefault verifies that the default config does not
// add source/caller information to log entries (D-7: addSource defaults false).
func Test_ZeroConfig_NoCallerByDefault(t *testing.T) {
	cfg := config.GetConfig()
	if cfg.AddSource() {
		t.Error("AddSource() = true; want false — zero-config must not pay runtime.Callers overhead")
	}
}

// Test_ZeroConfig_EmitsJSONLine verifies that a log event published through
// the pipeline reaches a post-processor, producing at least one write.
// It replaces defaultPostProc with a buffer-backed processor for isolation.
func Test_ZeroConfig_EmitsJSONLine(t *testing.T) {
	var buf safeBuffer
	proc := pipelineStage.NewUnsetLogEventPostProcessor(50*time.Millisecond, 20, &buf)
	pipelineStage.EventPreProcessorObj.RegisterHook(enum.LevelUnSet, proc)
	t.Cleanup(func() {
		proc.Stop()
		// Re-register the real stdout processor so subsequent tests aren't broken.
		realProc := pipelineStage.NewUnsetLogEventPostProcessor(time.Second, 20, os.Stdout)
		defaultPostProc = realProc
		pipelineStage.EventPreProcessorObj.RegisterHook(enum.LevelUnSet, realProc)
	})

	config.PublishLog(enum.LevelInfo, []byte(`{"level":"info","msg":"zero-config smoke"}`))

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if buf.Len() > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if buf.Len() == 0 {
		t.Fatal("no output written to buffer; pipeline not wired")
	}
	if !strings.Contains(buf.String(), "zero-config smoke") {
		t.Errorf("output = %q; want substring %q", buf.String(), "zero-config smoke")
	}
}
