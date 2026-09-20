// Command hooks shows how to send records to more than one destination, and
// how to send some levels somewhere different from the rest.
//
// A hook is anything with PublishLogMessage([]byte) and Name() string. The
// built-in collector (NewUnsetLogEventPostProcessor) is one; a custom type is
// another. Register hooks against a level — or against LevelUnSet, which
// receives every record.
//
// Run: go run ./05-hooks
package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/architagr/lognugget/config"
	"github.com/architagr/lognugget/entry"
	"github.com/architagr/lognugget/enum"
	"github.com/architagr/lognugget/examples/cookbook/internal/demo"
	pipelineStage "github.com/architagr/lognugget/pipeline_stage"
)

// countingHook is a minimal custom hook: it implements the two methods the
// fan-out stage needs and nothing else.
//
// The []byte it receives is borrowed — the dispatcher recycles that buffer as
// soon as PublishLogMessage returns, so a hook that keeps the record must copy
// it. This one copies into its own buffer.
type countingHook struct {
	name string
	mu   sync.Mutex
	buf  bytes.Buffer
	n    int
}

func (h *countingHook) PublishLogMessage(record []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.n++
	h.buf.Write(record) // copies; never retain the slice itself
}

func (h *countingHook) Name() string { return h.name }

func (h *countingHook) Report() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return fmt.Sprintf("%s received %d record(s)", h.name, h.n)
}

func main() {
	ctx := context.Background()

	// ── 1. One collector for everything ─────────────────────────────────────
	// This is what importing lognugget gives you: a collector at LevelUnSet
	// writing to stdout. Registering another hook with the same Name replaces
	// it, so this example builds its pipeline explicitly instead.
	stdoutHook := pipelineStage.NewUnsetLogEventPostProcessor(
		50*time.Millisecond, // flush interval
		1,                   // flush after this many records
		os.Stdout,
	)
	pipelineStage.EventPreProcessorObj.RegisterHook(enum.LevelUnSet, stdoutHook)

	// ── 2. A second destination for errors only ─────────────────────────────
	// Hooks registered at a specific level receive only that level. Records
	// still go to every LevelUnSet hook as well, so an error lands in both.
	errFile, err := os.CreateTemp("", "lognugget-errors-*.log")
	if err != nil {
		panic(err)
	}
	defer os.Remove(errFile.Name())

	errorHook := pipelineStage.NewUnsetLogEventPostProcessor(50*time.Millisecond, 1, errFile)
	pipelineStage.EventPreProcessorObj.RegisterHook(enum.LevelError, errorHook)

	// ── 3. A custom hook ────────────────────────────────────────────────────
	// Anything implementing the two-method contract works: ship to Loki, count
	// records for metrics, mirror into a test buffer.
	counter := &countingHook{name: "counting-hook"}
	pipelineStage.EventPreProcessorObj.RegisterHook(enum.LevelUnSet, counter)

	config.InitPreProcessors(pipelineStage.EventPreProcessorObj)
	config.SetMinLevel(enum.LevelDebug)

	// Shut the hooks down in reverse order of importance: drain the dispatcher
	// first, then stop each collector so its buffer is written.
	defer func() {
		config.FlushDispatch(2 * time.Second)
		stdoutHook.Stop()
		errorHook.Stop()

		fmt.Println("\n── results ──")
		fmt.Println(counter.Report())
		written, _ := os.ReadFile(errFile.Name())
		fmt.Printf("error-only file holds %d record(s):\n%s", bytes.Count(written, []byte{'\n'}), written)
	}()

	demo.Section("records fan out to every matching hook")
	entry.NewLogEntry().Str("step", "one").Info(ctx, "info goes to stdout + counter")
	entry.NewLogEntry().Str("step", "two").Warn(ctx, "warn goes to stdout + counter")
	entry.NewLogEntry().
		Str("step", "three").
		Error(ctx, fmt.Errorf("disk full"), "error goes to stdout + counter + error file")

	// ── 4. Removing a hook ──────────────────────────────────────────────────
	// DeRegisterHook takes the level and the hook's Name.
	demo.Section("after de-registering the counting hook")
	pipelineStage.EventPreProcessorObj.DeRegisterHook(enum.LevelUnSet, counter.Name())
	entry.NewLogEntry().Str("step", "four").Info(ctx, "counter no longer sees this")

	demo.Flush()
}
