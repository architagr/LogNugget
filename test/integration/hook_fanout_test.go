//go:build testing

// Package integration: TS-27 — SC8 hook fan-out end-to-end.
//
// SC8: a hook registered at enum.LevelUnSet receives every log event
// regardless of level; a hook at a specific level receives only events
// at that level. Both hooks may co-exist on EventPreProcessorObj.
package integration

import (
	"testing"
	"time"

	"github.com/architagr/lognugget/config"
	"github.com/architagr/lognugget/enum"
	pipelineStage "github.com/architagr/lognugget/pipeline_stage"
	"github.com/architagr/lognugget/test/support"
)

// waitForCount polls fake.Got() until at least want records arrive or deadline.
func waitForCount(t *testing.T, fake *support.FakePostProcessor, want int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if len(fake.Got()) >= want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Errorf("timed out: got %d records, want %d", len(fake.Got()), want)
}

// Test_Integration_HookFanout_UnsetReceivesAll verifies SC8a:
// a hook at LevelUnSet receives every published event.
func Test_Integration_HookFanout_UnsetReceivesAll(t *testing.T) {
	support.NewConfigBuilder(t).MinLevel(enum.LevelDebug).Build()

	allHook := support.NewFakePostProcessor("unset-all")
	pipelineStage.EventPreProcessorObj.RegisterHook(enum.LevelUnSet, allHook)
	t.Cleanup(func() {
		pipelineStage.EventPreProcessorObj.DeRegisterHook(enum.LevelUnSet, allHook.Name())
	})
	config.InitPreProcessors(pipelineStage.EventPreProcessorObj)

	payload := []byte(`{"msg":"fanout-all"}`)
	levels := []enum.LogLevel{enum.LevelDebug, enum.LevelInfo, enum.LevelWarn, enum.LevelError}
	for _, lvl := range levels {
		config.PublishLog(lvl, payload)
	}

	waitForCount(t, allHook, len(levels), 2*time.Second)
	if got := len(allHook.Got()); got != len(levels) {
		t.Errorf("LevelUnSet hook got %d records; want %d (all levels)", got, len(levels))
	}
}

// Test_Integration_HookFanout_LevelSpecificReceivesOnly verifies SC8b:
// a hook at LevelInfo receives only LevelInfo events.
func Test_Integration_HookFanout_LevelSpecificReceivesOnly(t *testing.T) {
	support.NewConfigBuilder(t).MinLevel(enum.LevelDebug).Build()

	infoHook := support.NewFakePostProcessor("info-only")
	pipelineStage.EventPreProcessorObj.RegisterHook(enum.LevelInfo, infoHook)
	t.Cleanup(func() {
		pipelineStage.EventPreProcessorObj.DeRegisterHook(enum.LevelInfo, infoHook.Name())
	})
	config.InitPreProcessors(pipelineStage.EventPreProcessorObj)

	infoPayload := []byte(`{"level":"info","msg":"info-event"}`)
	debugPayload := []byte(`{"level":"debug","msg":"debug-event"}`)

	config.PublishLog(enum.LevelInfo, infoPayload)
	config.PublishLog(enum.LevelInfo, infoPayload)
	config.PublishLog(enum.LevelDebug, debugPayload) // must NOT reach infoHook

	waitForCount(t, infoHook, 2, 2*time.Second)
	time.Sleep(50 * time.Millisecond) // let any spurious debug event arrive

	if got := len(infoHook.Got()); got != 2 {
		t.Errorf("LevelInfo hook got %d records; want exactly 2 (only Info events)", got)
	}
}

// Test_Integration_HookFanout_BothHooksCoexist verifies SC8c:
// LevelUnSet hook + LevelInfo hook both receive Info events;
// only LevelUnSet hook receives Debug events.
func Test_Integration_HookFanout_BothHooksCoexist(t *testing.T) {
	support.NewConfigBuilder(t).MinLevel(enum.LevelDebug).Build()

	allHook := support.NewFakePostProcessor("coexist-all")
	infoHook := support.NewFakePostProcessor("coexist-info")

	pipelineStage.EventPreProcessorObj.RegisterHook(enum.LevelUnSet, allHook)
	pipelineStage.EventPreProcessorObj.RegisterHook(enum.LevelInfo, infoHook)
	t.Cleanup(func() {
		pipelineStage.EventPreProcessorObj.DeRegisterHook(enum.LevelUnSet, allHook.Name())
		pipelineStage.EventPreProcessorObj.DeRegisterHook(enum.LevelInfo, infoHook.Name())
	})
	config.InitPreProcessors(pipelineStage.EventPreProcessorObj)

	infoPayload := []byte(`{"level":"info","msg":"coexist"}`)
	debugPayload := []byte(`{"level":"debug","msg":"coexist"}`)

	config.PublishLog(enum.LevelInfo, infoPayload)   // → allHook + infoHook
	config.PublishLog(enum.LevelDebug, debugPayload) // → allHook only

	// allHook must receive 2 events (info + debug).
	waitForCount(t, allHook, 2, 2*time.Second)
	// infoHook must receive exactly 1 event (info only).
	waitForCount(t, infoHook, 1, 2*time.Second)
	time.Sleep(50 * time.Millisecond) // let any spurious event arrive

	if got := len(allHook.Got()); got != 2 {
		t.Errorf("LevelUnSet hook got %d; want 2 (info+debug)", got)
	}
	if got := len(infoHook.Got()); got != 1 {
		t.Errorf("LevelInfo hook got %d; want 1 (info only)", got)
	}
}
