package support

import (
	"sync"

	"github.com/architagr/lognugget/config"
	"github.com/architagr/lognugget/enum"
)

// FakeWriter is a thread-safe io.Writer that records every byte
// written and the number of Write calls.
type FakeWriter struct {
	mu    sync.Mutex
	buf   []byte
	count int
}

// NewFakeWriter returns a zero-state FakeWriter.
func NewFakeWriter() *FakeWriter { return &FakeWriter{} }

// Write appends p, bumps the call counter, returns (len(p), nil).
func (w *FakeWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buf = append(w.buf, p...)
	w.count++
	return len(p), nil
}

// Bytes returns a copy of every byte ever written.
// why: copy prevents callers corrupting fake state via slice aliasing.
func (w *FakeWriter) Bytes() []byte {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := make([]byte, len(w.buf))
	copy(out, w.buf)
	return out
}

// Count returns the number of Write calls observed.
func (w *FakeWriter) Count() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.count
}

// SpyHook is a chan-backed config.PublishLogMessageHookContract that
// records published payloads in FIFO order. The contract surface is
// PublishLogMessage([]byte) only — level is not propagated by the
// upstream hook contract.
//
// Buffering is bounded: once the channel is full, further publishes
// are dropped silently (never block the producer). Callers MUST size
// the capacity at construction time to be at least the number of
// events the test expects to observe; oversize is harmless, undersize
// causes lost records.
type SpyHook struct {
	name    string
	records chan config.LogEvent
}

// NewSpyHook returns a SpyHook with the given name and buffer
// capacity. Any capacity < 1 is silently raised to 1 so the zero
// value is usable; tests that need "always block" semantics are not
// supported by this fake — pick a capacity sized to the expected
// event count instead.
func NewSpyHook(name string, capacity int) *SpyHook {
	if capacity < 1 {
		capacity = 1
	}
	return &SpyHook{name: name, records: make(chan config.LogEvent, capacity)}
}

// Name reports the hook name.
func (h *SpyHook) Name() string { return h.name }

// PublishLogMessage records entry; drops silently if buffer is full.
// why: copy so a producer reusing its buffer cannot mutate what tests
// later assert on.
func (h *SpyHook) PublishLogMessage(entry []byte) {
	cp := make([]byte, len(entry))
	copy(cp, entry)
	select {
	case h.records <- config.LogEvent{Data: cp}:
	default:
	}
}

// Next blocks until a record is available or the channel closes.
func (h *SpyHook) Next() (config.LogEvent, bool) {
	ev, ok := <-h.records
	return ev, ok
}

// NextNonBlocking returns the next record if immediately available.
func (h *SpyHook) NextNonBlocking() (config.LogEvent, bool) {
	select {
	case ev, ok := <-h.records:
		return ev, ok
	default:
		return config.LogEvent{}, false
	}
}

// FakePreProcRecord captures a single PreProcess invocation.
type FakePreProcRecord struct {
	Level enum.LogLevel
	Data  []byte
}

// FakePreProc records every PreProcess invocation in order. Satisfies
// the (unexported) preProcessingObserverContract in package config via
// structural typing.
//
// LIMITATION: upstream contract is unexported; update this fake in
// lock step if its method set changes.
type FakePreProc struct {
	mu   sync.Mutex
	name string
	recs []FakePreProcRecord
}

// NewFakePreProc returns a FakePreProc with the given name.
func NewFakePreProc(name string) *FakePreProc { return &FakePreProc{name: name} }

// Name reports the processor name.
func (p *FakePreProc) Name() string { return p.name }

// PreProcess records (level, data); data is defensively copied.
func (p *FakePreProc) PreProcess(level enum.LogLevel, logMsg []byte) {
	cp := make([]byte, len(logMsg))
	copy(cp, logMsg)
	p.mu.Lock()
	defer p.mu.Unlock()
	p.recs = append(p.recs, FakePreProcRecord{Level: level, Data: cp})
}

// Records returns a snapshot copy of every invocation in call order.
func (p *FakePreProc) Records() []FakePreProcRecord {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]FakePreProcRecord, len(p.recs))
	copy(out, p.recs)
	return out
}
