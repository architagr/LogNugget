//go:build testing

// Package entry_test provides black-box tests for the entry package.
// These tests cover TS-04 (table-driven level methods with json.Unmarshal
// assertions, T-2 fix) and parallel-safety requirements (T-4 fix).
//
// Parallel-safety strategy: all tests that mutate the process-wide config
// singleton are grouped as sequential sub-tests under a single parallel parent
// (Test_LogEntry_Methods). This ensures:
//   - The parent calls t.Parallel() satisfying T-4.
//   - Sub-tests run sequentially so they do not race on the singleton.
//   - Each sub-test resets the singleton via ConfigBuilder before configuring
//     its own spy.
//
// The root-cause race on config.EventPreProcessors is tracked as issue #54
// / story 039. Once that is resolved, sub-tests may be made parallel.
package entry_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/architagr/lognugget/v4/config"
	"github.com/architagr/lognugget/v4/entry"
	"github.com/architagr/lognugget/v4/enum"
	"github.com/architagr/lognugget/v4/test/support"
)

// drainSpy waits for the asynchronous pipeline to deliver, then returns the
// first record the spy received. It fails the test if nothing arrives.
func drainSpy(t *testing.T, spy *support.FakePreProc) []byte {
	t.Helper()

	// why: the dispatcher hands events to pre-processors on its own goroutine,
	// so the record is not there the instant the log call returns. This used
	// to spin a fixed 500 scheduler yields, which under -race occasionally ran
	// out before delivery and failed the test. FlushDispatch waits for the
	// dispatcher to actually catch up.
	if !config.FlushDispatch(5 * time.Second) {
		t.Fatal("dispatcher did not drain within 5s")
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if recs := spy.Records(); len(recs) > 0 {
			return recs[0].Data
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("timed out waiting for log record; spy was never invoked — check config.InitPreProcessors")
	return nil
}

// parseLogJSON unmarshals raw JSON produced by the JSON encoder into a
// string-keyed map. All values in the log output are quoted strings
// (see config.ParseLogField), so map[string]string is the correct target type.
func parseLogJSON(t *testing.T, data []byte) map[string]string {
	t.Helper()
	var m map[string]string
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("json.Unmarshal failed on log payload: %v\npayload: %s", err, data)
	}
	return m
}

// resetAndSpy resets the singleton to JSON/Debug defaults and installs a new
// spy via config.InitPreProcessors, then returns the spy.
func resetAndSpy(t *testing.T, name string) *support.FakePreProc {
	t.Helper()
	support.NewConfigBuilder(t).
		MinLevel(enum.LevelDebug).
		Encoder(enum.EncoderJSON).
		Build()
	spy := support.NewFakePreProc("spy-" + name)
	config.InitPreProcessors(spy)
	return spy
}

// Test_LogEntry_Methods is the top-level parallel test that groups all
// entry-level-method tests. The parent calls t.Parallel() (T-4 fix) to
// allow other non-singleton tests to run concurrently. Sub-tests are
// sequential because each mutates the global config singleton via
// config.InitPreProcessors; running them concurrently races on the singleton
// (issue #54 / story 039).
//
// Sub-tests covered:
//   - Table-driven level tests (Debug, Info, Warn, Error) — TS-04 / T-2
//   - ErrorAcceptsErr — interface-type check on Error()
//   - FatalLogsBeforeExiting — Fatal() emits a record before runtime.Goexit
//   - PanicLogsBeforeExiting — Panic() emits a record before panicking
func Test_LogEntry_Methods(t *testing.T) {
	t.Parallel()

	// Level-method table: covers TS-04 and T-2 (json.Unmarshal assertions).
	levelCases := []struct {
		name        string
		wantLevel   string
		wantMessage string
		doLog       func(e *entry.LogEntry, msg string)
	}{
		{
			name:        "Debug_emits_DEBUG_level",
			wantLevel:   "DEBUG",
			wantMessage: "hello-debug",
			doLog: func(e *entry.LogEntry, msg string) {
				e.Debug(context.Background(), msg)
			},
		},
		{
			name:        "Info_emits_INFO_level",
			wantLevel:   "INFO",
			wantMessage: "hello-info",
			doLog: func(e *entry.LogEntry, msg string) {
				e.Info(context.Background(), msg)
			},
		},
		{
			name:        "Warn_emits_WARN_level",
			wantLevel:   "WARN",
			wantMessage: "hello-warn",
			doLog: func(e *entry.LogEntry, msg string) {
				e.Warn(context.Background(), msg)
			},
		},
		{
			name:        "Error_emits_ERROR_level",
			wantLevel:   "ERROR",
			wantMessage: "hello-error",
			doLog: func(e *entry.LogEntry, msg string) {
				e.Error(context.Background(), errors.New("boom"), msg)
			},
		},
	}

	for _, tc := range levelCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			spy := resetAndSpy(t, tc.name)

			tc.doLog(entry.NewLogEntry(), tc.wantMessage)

			raw := drainSpy(t, spy)
			got := parseLogJSON(t, raw)

			if got["level"] != tc.wantLevel {
				t.Errorf("level = %q, want %q", got["level"], tc.wantLevel)
			}
			if got["message"] != tc.wantMessage {
				t.Errorf("message = %q, want %q", got["message"], tc.wantMessage)
			}
		})
	}

	// Test_LogEntry_ErrorAcceptsErr verifies that Error() accepts an error
	// interface argument and propagates its message to the "error" JSON field.
	// Any non-nil error implementation must be accepted (interface-type check).
	t.Run("ErrorAcceptsErr", func(t *testing.T) {
		spy := resetAndSpy(t, "error-accepts-err")

		sentinel := errors.New("sentinel error value")
		// Error's second parameter is of type `error` (interface). Passing a
		// concrete *errors.errorString verifies the interface contract is met.
		entry.NewLogEntry().Error(context.Background(), sentinel, "an error occurred")

		raw := drainSpy(t, spy)
		got := parseLogJSON(t, raw)

		if got["error"] != sentinel.Error() {
			t.Errorf("error field = %q, want %q", got["error"], sentinel.Error())
		}
	})

	// Test_LogEntry_FatalLogsBeforeExiting verifies that Fatal() emits a log
	// record before terminating the calling goroutine via runtime.Goexit. The
	// goroutine is isolated in a child goroutine so the test process survives.
	//
	// why: Fatal calls runtime.Goexit which terminates only the calling goroutine.
	// Wrapping in a dedicated goroutine + channel drain lets the test observe the
	// emitted record after the goroutine exits cleanly.
	t.Run("FatalLogsBeforeExiting", func(t *testing.T) {
		spy := resetAndSpy(t, "fatal")

		done := make(chan struct{})
		go func() {
			defer close(done)
			entry.NewLogEntry().Fatal(context.Background(), errors.New("fatal err"), "fatal message")
		}()
		<-done

		raw := drainSpy(t, spy)
		got := parseLogJSON(t, raw)

		if got["level"] != "ERROR" {
			t.Errorf("Fatal level field = %q, want ERROR", got["level"])
		}
		if got["message"] != "fatal message" {
			t.Errorf("Fatal message field = %q, want \"fatal message\"", got["message"])
		}
	})

	// Test_LogEntry_PanicLogsBeforeExiting verifies that Panic() emits a log
	// record before panicking. The panic is caught with recover() in a deferred
	// function so the test process survives.
	t.Run("PanicLogsBeforeExiting", func(t *testing.T) {
		spy := resetAndSpy(t, "panic")

		panicked := false
		func() {
			defer func() {
				if r := recover(); r != nil {
					panicked = true
				}
			}()
			entry.NewLogEntry().Panic(context.Background(), errors.New("panic err"), "panic message")
		}()

		if !panicked {
			t.Error("Panic() did not panic; expected a panic to be raised")
		}

		raw := drainSpy(t, spy)
		got := parseLogJSON(t, raw)

		if got["level"] != "ERROR" {
			t.Errorf("Panic level field = %q, want ERROR", got["level"])
		}
		if got["message"] != "panic message" {
			t.Errorf("Panic message field = %q, want \"panic message\"", got["message"])
		}
	})

	// SC1_ZeroConfig_NoCallerField verifies that zero-config Log() does not
	// emit a "caller" field in the JSON output (SC1 / D-7 regression guard).
	// why: caller is only appended when e.caller != nil; addSource defaults to
	// false. resetAndSpy resets the singleton without calling SetAddSource so
	// addSource remains false — the zero-config state.
	t.Run("SC1_ZeroConfig_NoCallerField", func(t *testing.T) {
		spy := resetAndSpy(t, "sc1")

		entry.NewLogEntry().Info(context.Background(), "sc1-zero-config")

		raw := drainSpy(t, spy)
		got := parseLogJSON(t, raw)

		if _, hasCaller := got["caller"]; hasCaller {
			t.Errorf("SC1: zero-config JSON must not contain 'caller' field (addSource=false); got: %s", raw)
		}
	})
}
