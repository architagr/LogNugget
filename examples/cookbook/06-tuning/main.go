// Command tuning covers the three knobs that decide how much work the caller
// does and how long a record waits before it is written:
//
//   - entry.GenerateInitialPool(n) — how many LogEntry objects are pre-allocated
//   - config.SetLogBufferMaxSize(n) — flush after n records
//   - config.SetRate(d)             — flush at least every d
//
// A record is written when either trigger fires first. Bigger batches and
// longer intervals mean fewer writes and less IO overhead, at the cost of
// holding records in memory longer.
//
// Run: go run ./06-tuning
package main

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/architagr/lognugget/v4/config"
	"github.com/architagr/lognugget/v4/entry"
	"github.com/architagr/lognugget/v4/enum"
	pipelineStage "github.com/architagr/lognugget/v4/pipeline_stage"
)

// writeCounter counts Write calls so the effect of batching is visible.
type writeCounter struct {
	mu     sync.Mutex
	writes int
	bytes  int
}

func (w *writeCounter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.writes++
	w.bytes += len(p)
	return len(p), nil
}

func (w *writeCounter) stats() (int, int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.writes, w.bytes
}

func main() {
	ctx := context.Background()

	// ── Pre-allocating the entry pool ───────────────────────────────────────
	// Every log call takes a LogEntry from a sync.Pool and returns it when the
	// record has been dispatched. A cold pool allocates on the first calls; a
	// pre-warmed one does not.
	//
	// GOMAXPROCS × 64 is a sensible production shape: enough that every P has
	// entries to hand, small enough that the pool is not a hidden megabyte.
	// Do not size it by expected request count — sync.Pool is cleared at every
	// GC, so an enormous pool buys nothing and hides real allocation cost.
	entry.GenerateInitialPool(runtime.GOMAXPROCS(0) * 64)

	// ── Batching: records per write vs latency to disk ──────────────────────
	fmt.Println("500 records, three batching configurations:")
	fmt.Printf("%-34s %8s %10s\n", "configuration", "writes", "bytes")

	for _, tc := range []struct {
		name   string
		bucket int
		rate   time.Duration
	}{
		// One write per record: lowest latency to the sink, most syscalls.
		// Right for low-volume services, or when a record must be durable
		// immediately.
		{"bucket=1   rate=1s   (no batching)", 1, time.Second},
		// The library default. A record waits for 19 companions or one second,
		// whichever comes first.
		{"bucket=20  rate=1s   (default)", 20, time.Second},
		// High-throughput shape: fewer, larger writes. A quiet service can
		// hold a record for up to five seconds, so pair it with Shutdown.
		{"bucket=500 rate=5s   (throughput)", 500, 5 * time.Second},
	} {
		out := &writeCounter{}
		proc := pipelineStage.NewUnsetLogEventPostProcessor(tc.rate, tc.bucket, out)
		pipelineStage.EventPreProcessorObj.RegisterHook(enum.LevelUnSet, proc)
		config.InitPreProcessors(pipelineStage.EventPreProcessorObj)
		config.SetMinLevel(enum.LevelDebug)

		for i := 0; i < 500; i++ {
			entry.NewLogEntry().Int("seq", int64(i)).Info(ctx, "batching demo")
		}

		// Drain the dispatch ring, then stop the collector so its pending
		// bucket is written. Stop blocks until the last byte is out.
		config.FlushDispatch(2 * time.Second)
		proc.Stop()
		pipelineStage.EventPreProcessorObj.DeRegisterHook(enum.LevelUnSet, proc.Name())

		writes, bytes := out.stats()
		fmt.Printf("%-34s %8d %10d\n", tc.name, writes, bytes)
	}

	// ── Retuning the built-in collector at runtime ──────────────────────────
	// When you use the zero-config pipeline (import lognugget), these package
	// setters retune that collector — no need to build your own.
	fmt.Println("\nRetuning the default collector:")
	stdoutProc := pipelineStage.NewUnsetLogEventPostProcessor(time.Second, 20, os.Stdout)
	pipelineStage.EventPreProcessorObj.RegisterHook(enum.LevelUnSet, stdoutProc)
	config.InitPreProcessors(pipelineStage.EventPreProcessorObj)
	config.RegisterDefaultSink(stdoutProc)

	config.SetLogBufferMaxSize(1)         // flush every record
	config.SetRate(20 * time.Millisecond) // and at least every 20ms
	config.SetOutput(os.Stdout)           // to this writer

	entry.NewLogEntry().Str("knob", "SetLogBufferMaxSize").Info(ctx, "written immediately")

	config.FlushDispatch(2 * time.Second)
	stdoutProc.Stop()

	// ── Choosing values ─────────────────────────────────────────────────────
	fmt.Println(`
Rules of thumb:
  bucket  ≈ records/sec ÷ writes/sec you are willing to pay for
  rate    = the longest you can tolerate a record sitting unwritten
  pool    = GOMAXPROCS × 64 unless profiling says otherwise

Always call lognugget.Shutdown() (or FlushDispatch + Stop) before exit:
with a 5s rate, a crash-free exit can still drop five seconds of logs.`)
}
