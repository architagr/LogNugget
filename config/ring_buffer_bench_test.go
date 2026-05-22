//go:build testing

package config

import (
	"runtime"
	"testing"

	"github.com/architagr/lognugget/enum"
)

// BenchmarkRingBuffer_Push measures producer-side push throughput under
// GOMAXPROCS=8 contention. Target: ≤ 100 ns/op per the P9 acceptance criteria.
func BenchmarkRingBuffer_Push(b *testing.B) {
	prev := runtime.GOMAXPROCS(8)
	b.Cleanup(func() { runtime.GOMAXPROCS(prev) })

	r := newMpscRingBuffer()

	// Consumer goroutine drains continuously to prevent the ring from filling.
	stop := make(chan struct{})
	go func() {
		for {
			select {
			case <-stop:
				return
			default:
				r.Pop()
			}
		}
	}()
	b.Cleanup(func() { close(stop) })

	e := LogEvent{Level: enum.LevelInfo, Data: []byte(`{"level":"INFO"}`)}
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			r.Push(e)
		}
	})
}
