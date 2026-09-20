package config

import (
	"io"
	"sync/atomic"
	"time"
)

// DefaultSink is the narrow view of the library's built-in collector that the
// configuration setters need in order to take effect at runtime.
//
// The zero-config pipeline creates one buffering collector (see package
// lognugget) and registers it here. why: SetOutput, SetRate and
// SetLogBufferMaxSize describe that collector's behaviour, but the collector
// lives in package pipelineStage, which package config must not import: the
// dependency runs the other way. An interface registered at init keeps the
// documented setters working without inverting the package graph.
//
// Implementations must be safe for concurrent use.
type DefaultSink interface {
	// SetOutput redirects subsequent writes.
	SetOutput(io.Writer)
	// SetRate changes the flush interval.
	SetRate(time.Duration)
	// SetMaxBucketSize changes the record count that triggers a flush.
	SetMaxBucketSize(int)
}

// defaultSink holds the registered collector, or nil when the caller has built
// their own pipeline instead of importing the zero-config facade.
var defaultSink atomic.Pointer[DefaultSink]

// RegisterDefaultSink installs the collector that SetOutput, SetRate and
// SetLogBufferMaxSize retune. Passing nil detaches the current sink, after
// which those setters only record the values.
//
// Called by package lognugget during init; application code does not normally
// need it. A caller that builds its own post-processor can register it here to
// make the same setters apply to their collector.
func RegisterDefaultSink(sink DefaultSink) {
	if sink == nil {
		defaultSink.Store(nil)
		return
	}
	defaultSink.Store(&sink)
}

// loadDefaultSink returns the registered sink, or nil.
func loadDefaultSink() DefaultSink {
	if p := defaultSink.Load(); p != nil {
		return *p
	}
	return nil
}
