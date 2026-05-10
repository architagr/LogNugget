package support_test

import (
	"sync"
	"testing"
)

// testWriter is a minimal io.Writer used as a sentinel in
// ConfigBuilder tests — only its identity matters, not its contents.
type testWriter struct{ buf []byte }

func (w *testWriter) Write(p []byte) (int, error) {
	w.buf = append(w.buf, p...)
	return len(p), nil
}

// recordingTB wraps testing.TB and captures Cleanup callbacks instead
// of registering them with the underlying *testing.T. This lets us
// assert that the ConfigBuilder registers exactly one cleanup, and
// invoke that cleanup deterministically inside the test body rather
// than waiting for the test scope to unwind.
type recordingTB struct {
	testing.TB
	mu       sync.Mutex
	cleanups []func()
}

func newRecordingTB(t *testing.T) *recordingTB {
	return &recordingTB{TB: t}
}

// Cleanup overrides testing.TB.Cleanup so the registered fn lands in
// the recordingTB's slice instead of the real t's stack.
func (r *recordingTB) Cleanup(fn func()) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cleanups = append(r.cleanups, fn)
}

// runCleanups invokes every captured cleanup in LIFO order, mirroring
// testing.T's documented semantics.
func (r *recordingTB) runCleanups() {
	r.mu.Lock()
	fns := r.cleanups
	r.cleanups = nil
	r.mu.Unlock()
	for i := len(fns) - 1; i >= 0; i-- {
		fns[i]()
	}
}
