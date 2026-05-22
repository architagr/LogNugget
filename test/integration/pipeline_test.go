//go:build testing

// Package integration tests end-to-end async event delivery through the MPSC
// ring buffer. P9 replaced the buffered chan LogEvent (TS-25 F21/F22/F23) with
// a lock-free ring buffer; blocking-on-full semantics no longer apply.
package integration

import (
	"testing"
	"time"

	"github.com/architagr/lognugget/config"
	"github.com/architagr/lognugget/enum"
	"github.com/architagr/lognugget/test/support"
)

// Test_Integration_ChannelDrainsAsyncAfterRelease verifies that once
// SlowHook is released, ProcessLogEvent delivers all buffered events.
func Test_Integration_ChannelDrainsAsyncAfterRelease(t *testing.T) {
	support.NewConfigBuilder(t).
		MinLevel(enum.LevelDebug).
		Build()

	fake := support.NewFakePostProcessor("drain-spy")
	spy := &fakePreProc{hook: fake}
	config.InitPreProcessors(spy)

	const n = 5
	payload := []byte(`{"level":"info","msg":"drain"}`)
	for i := 0; i < n; i++ {
		config.PublishLog(enum.LevelInfo, payload)
	}

	// Wait for all n events to be delivered asynchronously.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(fake.Got()) >= n {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if got := len(fake.Got()); got < n {
		t.Errorf("delivered %d events; want %d", got, n)
	}
}

type fakePreProc struct {
	hook *support.FakePostProcessor
}

func (f *fakePreProc) Name() string { return f.hook.Name() }
func (f *fakePreProc) PreProcess(_ enum.LogLevel, logMsg []byte) {
	f.hook.PublishLogMessage(logMsg)
}
