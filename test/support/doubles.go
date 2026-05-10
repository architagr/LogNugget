package support

import (
	"context"
	"sync"
	"time"

	"github.com/architagr/lognugget/config"
	"github.com/architagr/lognugget/encoder"
)

// SlowHook implements config.PublishLogMessageHookContract and blocks
// PublishLogMessage until Release is invoked. Drives F22/F23
// backpressure tests; T-1 mitigation removes time.Sleep waits.
type SlowHook struct {
	name    string
	gate    chan struct{}
	mu      sync.Mutex
	records [][]byte
}

// NewSlowHook returns a SlowHook with a closed-on-Release gate.
func NewSlowHook(name string) *SlowHook {
	return &SlowHook{name: name, gate: make(chan struct{})}
}

// Name reports the hook name.
func (h *SlowHook) Name() string { return h.name }

// PublishLogMessage records entry then blocks until Release is called.
// why: defensive copy so the producer can reuse its buffer.
func (h *SlowHook) PublishLogMessage(entry []byte) {
	cp := append([]byte(nil), entry...)
	h.mu.Lock()
	h.records = append(h.records, cp)
	h.mu.Unlock()
	<-h.gate
}

// Release unblocks every current and future PublishLogMessage call.
func (h *SlowHook) Release() { close(h.gate) }

// Records returns a snapshot of every payload observed.
func (h *SlowHook) Records() [][]byte {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([][]byte(nil), h.records...)
}

// FakePostProcessor implements the hook contract with no IO and
// exposes recorded payloads via Got.
type FakePostProcessor struct {
	name string
	mu   sync.Mutex
	recs [][]byte
}

// NewFakePostProcessor returns a FakePostProcessor with the given name.
func NewFakePostProcessor(name string) *FakePostProcessor {
	return &FakePostProcessor{name: name}
}

// Name reports the processor name.
func (p *FakePostProcessor) Name() string { return p.name }

// PublishLogMessage records a defensive copy of entry.
func (p *FakePostProcessor) PublishLogMessage(entry []byte) {
	cp := append([]byte(nil), entry...)
	p.mu.Lock()
	p.recs = append(p.recs, cp)
	p.mu.Unlock()
}

// Got returns deep copies of every recorded payload so callers cannot
// mutate fake state via slice aliasing.
func (p *FakePostProcessor) Got() [][]byte {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([][]byte, len(p.recs))
	for i, b := range p.recs {
		out[i] = append([]byte(nil), b...)
	}
	return out
}

// StubContextParser produces a config.ContextFieldsParser that returns
// a fixed map and counts parser invocations.
type StubContextParser struct {
	fixed map[string]any
	mu    sync.Mutex
	calls int
}

// NewStubContextParser returns a stub bound to fixed.
func NewStubContextParser(fixed map[string]any) *StubContextParser {
	return &StubContextParser{fixed: fixed}
}

// Parser returns a closure that records calls and returns fixed.
func (s *StubContextParser) Parser() config.ContextFieldsParser {
	return func(_ context.Context) map[string]any {
		s.mu.Lock()
		s.calls++
		s.mu.Unlock()
		return s.fixed
	}
}

// Calls returns the number of parser invocations.
func (s *StubContextParser) Calls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

// StubStaticParser produces a config.StaticEnvFieldsParser that returns
// a fixed map and asserts the source is sampled exactly once across
// any number of parser invocations (F15).
type StubStaticParser struct {
	fixed       map[string]any
	mu          sync.Mutex
	cached      map[string]any
	evaluations int
}

// NewStubStaticParser returns a stub bound to fixed.
func NewStubStaticParser(fixed map[string]any) *StubStaticParser {
	return &StubStaticParser{fixed: fixed}
}

// Parser returns a memoising config.StaticEnvFieldsParser closure.
// why: F15 requires once-only evaluation regardless of call count.
func (s *StubStaticParser) Parser() config.StaticEnvFieldsParser {
	return func() map[string]any {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.cached == nil {
			s.evaluations++
			s.cached = s.fixed
		}
		return s.cached
	}
}

// Evaluations returns the count of underlying-source evaluations.
func (s *StubStaticParser) Evaluations() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.evaluations
}

// StubEncoder implements encoder.Encoder with a passthrough Write that
// returns the body verbatim and records every call. Isolates pipeline
// tests from real-encoder churn (F10).
type StubEncoder struct {
	mu    sync.Mutex
	calls []string
}

// NewStubEncoder returns a zero-state StubEncoder.
func NewStubEncoder() *StubEncoder { return &StubEncoder{} }

// Write records body and returns it as raw bytes; never errors.
func (s *StubEncoder) Write(body string) ([]byte, error) {
	s.mu.Lock()
	s.calls = append(s.calls, body)
	s.mu.Unlock()
	return []byte(body), nil
}

// Calls returns the recorded Write inputs in call order.
func (s *StubEncoder) Calls() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.calls...)
}

var _ encoder.Encoder = (*StubEncoder)(nil)

// FakeClock returns a deterministic time pinned to
// 2026-01-02T03:04:05Z, required for stable golden-master output.
type FakeClock struct {
	mu  sync.Mutex
	now time.Time
}

// NewFakeClock returns a FakeClock pinned to 2026-01-02T03:04:05Z.
func NewFakeClock() *FakeClock {
	return &FakeClock{now: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)}
}

// Now returns the pinned instant.
func (c *FakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

// Advance shifts the pinned instant forward by d.
func (c *FakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}
