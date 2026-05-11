//go:build testing

// Package config_test exercises static-env and context parser contracts
// (F15, F16, TS-13, TS-14). All sub-tests share the process-wide config
// singleton and run sequentially under one parallel parent — same strategy
// as entry_test.Test_LogEntry_Methods — to prevent races.
package config_test

import (
	"context"
	"strings"
	"testing"

	"github.com/architagr/lognugget/config"
	"github.com/architagr/lognugget/entry"
	"github.com/architagr/lognugget/enum"
	"github.com/architagr/lognugget/test/support"
)

// drainN spins until spy has at least n records, then returns them.
func drainN(t *testing.T, spy *support.FakePreProc, n int) []support.FakePreProcRecord {
	t.Helper()
	for i := 0; i < 2000; i++ {
		if recs := spy.Records(); len(recs) >= n {
			return recs[:n]
		}
		done := make(chan struct{})
		go func() { close(done) }()
		<-done
	}
	t.Fatalf("timed out waiting for %d log records", n)
	return nil
}

// resetParser resets the singleton to debug/JSON defaults and registers spy
// as the sole pre-processor.
func resetParser(t *testing.T, name string) *support.FakePreProc {
	t.Helper()
	support.NewConfigBuilder(t).MinLevel(enum.LevelDebug).Encoder(enum.EncoderJSON).Build()
	spy := support.NewFakePreProc(name)
	config.InitPreProcessors(spy)
	return spy
}

// Test_Parsers groups all static + context parser sub-tests sequentially
// under one parallel parent to prevent singleton races (TS-13, TS-14).
func Test_Parsers(t *testing.T) {
	t.Parallel()

	// TS-13: static env parser

	t.Run("StaticEnvParser_EvaluatesOnce", func(t *testing.T) {
		spy := resetParser(t, "static-eval-spy")
		stub := support.NewStubStaticParser(map[string]any{"env": "prod"})
		config.SetStaticEnvFieldsParser(stub.Parser())

		const N = 5
		for i := 0; i < N; i++ {
			entry.NewLogEntry().Debug(context.Background(), "msg")
		}
		drainN(t, spy, N)

		if got := stub.Evaluations(); got != 1 {
			t.Fatalf("Evaluations() = %d after %d log calls; want 1 (F15)", got, N)
		}
	})

	t.Run("StaticEnvParser_RenderedAppendedToEveryLine", func(t *testing.T) {
		spy := resetParser(t, "static-rendered-spy")
		stub := support.NewStubStaticParser(map[string]any{"env": "staging"})
		config.SetStaticEnvFieldsParser(stub.Parser())

		const N = 3
		for i := 0; i < N; i++ {
			entry.NewLogEntry().Info(context.Background(), "check")
		}
		recs := drainN(t, spy, N)

		for i, rec := range recs {
			if !strings.Contains(string(rec.Data), "staging") {
				t.Errorf("record %d missing static field value %q: %s", i, "staging", rec.Data)
			}
		}
	})

	// TS-14: context fields parser

	t.Run("ContextParser_InvokedPerCall", func(t *testing.T) {
		spy := resetParser(t, "ctx-per-call-spy")
		stub := support.NewStubContextParser(map[string]any{"req": "abc"})
		config.SetContextFieldsParser(stub.Parser())

		const N = 4
		ctx := context.Background()
		for i := 0; i < N; i++ {
			entry.NewLogEntry().Debug(ctx, "msg")
		}
		drainN(t, spy, N)

		if got := stub.Calls(); got != N {
			t.Fatalf("Calls() = %d after %d log calls with non-nil ctx; want %d (F16)", got, N, N)
		}
	})

	t.Run("ContextParser_SkippedWhenCtxNil", func(t *testing.T) {
		spy := resetParser(t, "ctx-nil-spy")
		stub := support.NewStubContextParser(map[string]any{"req": "abc"})
		config.SetContextFieldsParser(stub.Parser())

		const N = 3
		for i := 0; i < N; i++ {
			entry.NewLogEntry().Debug(nil, "msg") //nolint:staticcheck // intentional nil ctx
		}
		drainN(t, spy, N)

		if got := stub.Calls(); got != 0 {
			t.Fatalf("Calls() = %d with nil ctx; want 0", got)
		}
	})

	t.Run("ContextParser_F14Collision", func(t *testing.T) {
		spy := resetParser(t, "ctx-collision-spy")
		stub := support.NewStubContextParser(map[string]any{"time": "12:00"})
		config.SetContextFieldsParser(stub.Parser())

		entry.NewLogEntry().Debug(context.Background(), "collision-check")
		recs := drainN(t, spy, 1)

		line := string(recs[0].Data)
		if strings.Contains(line, `"time": "12:00"`) {
			t.Fatalf("reserved key 'time' must be prefixed; found raw key in: %s", line)
		}
		if !strings.Contains(line, "custom.time") {
			t.Fatalf("expected 'custom.time' in log line: %s", line)
		}
	})
}
