package config

import (
	"runtime"
	"time"
)

// DefaultFlushTimeout bounds how long FlushDispatch waits for the dispatcher
// to drain. It is deliberately generous: the dispatcher only stalls this long
// if a hook's Write is blocked on slow IO.
const DefaultFlushTimeout = 5 * time.Second

// FlushDispatch blocks until every log event published before the call has
// been handed to all registered pre-processors, or until timeout elapses.
// It reports whether the dispatcher drained in time.
//
// why: PublishLog returns as soon as the event is in the MPSC ring, which is
// the whole point of the async pipeline. Stopping the output hooks without
// draining the ring first therefore discards whatever the consumer goroutine
// had not yet picked up, which is how a process that logged and exited
// immediately could lose its last lines. Shutdown paths must call this before
// stopping hooks.
//
// Flushing the dispatcher does not flush the hooks themselves: a buffering
// hook still holds its own bucket until its rate ticker fires or it is
// stopped. lognugget.Shutdown does both, in that order.
//
// Events published by other goroutines while FlushDispatch runs extend the
// work it waits for; the call returns once the dispatcher has caught up with
// the ring as it stands at that moment.
func FlushDispatch(timeout time.Duration) bool {
	if timeout <= 0 {
		timeout = DefaultFlushTimeout
	}
	ring := atomicRing.Load()
	if ring == nil {
		return true
	}

	deadline := time.Now().Add(timeout)
	for {
		// tail is the number of events ever published; dispatched is the number
		// the consumer has finished delivering. Equality means quiescent.
		if ring.dispatched.Load() >= ring.tail.Load() {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		runtime.Gosched()
	}
}
