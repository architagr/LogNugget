package encoder

import "testing"

// Benchmark_JSONEncoder_Append measures the hot-path cost of wrapping a
// pre-rendered log body in JSON braces and appending a newline. Input shape:
// a 64-byte body representative of a single-line structured log event. Budget:
// the encoder step must stay well under the 5 ms gate; expected ~10–20 ns/op
// with zero allocations when dst is pre-allocated.
func Benchmark_JSONEncoder_Append(b *testing.B) {
	b.ReportAllocs()
	enc := NewJSONEncoder()
	body := []byte(`"time":"2026-01-02T03:04:05Z","level":"info","msg":"ok"`)
	buf := make([]byte, 0, 128)
	for i := 0; i < b.N; i++ {
		buf = enc.Append(buf[:0], body)
	}
	_ = buf
}
