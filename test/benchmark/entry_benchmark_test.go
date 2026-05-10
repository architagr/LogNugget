// Package benchmark hosts library hot-path benchmarks.
//
// why: comparison benches against external loggers (e.g. zerolog) live under
// examples/ in nested modules so the library go.mod stays stdlib + testify
// only (NF12). T-9: Benchmark_ZeroLog removed from this file; full bench
// rewrite tracked by story 010.
package benchmark

import (
	"context"
	"testing"
	"time"

	"github.com/architagr/lognugget/config"
	"github.com/architagr/lognugget/entry"
	"github.com/architagr/lognugget/enum"
	"github.com/architagr/lognugget/model"
	pipelineStage "github.com/architagr/lognugget/pipeline_stage"
)

type ctxKey string

const (
	ctxKeyRequestID ctxKey = "requestID"
	ctxKeyUserID    ctxKey = "userID"
)

// MockWriter is a discard io.Writer used by Benchmark_Log to avoid I/O cost.
type MockWriter struct {
}

// Write discards p and returns its length to satisfy io.Writer.
func (e *MockWriter) Write(p []byte) (n int, err error) {
	return len(p), nil
}

// 279054	      4163 ns/op	    2094 B/op	      36 allocs/op
// 403225         3093 ns/op        1848 B/op         35 allocs/op
// 416030	      2629 ns/op	    1683 B/op	      32 allocs/op
// 421268	      3786 ns/op	    1667 B/op	      32 allocs/op
// 439795	      2933 ns/op	    1916 B/op	      45 allocs/op
// 442698	      2533 ns/op	    1825 B/op	      29 allocs/op
// 510938	      2330 ns/op	    1742 B/op	      25 allocs/op
// 874008	      1246 ns/op	    1989 B/op	      25 allocs/op
func Benchmark_Log(b *testing.B) {
	b.StopTimer()
	out := &MockWriter{}
	config.SetMinLevel(enum.LevelDebug)
	config.SetEncoderType(enum.EncoderJSON)
	config.SetStaticEnvFieldsParser(func() map[string]any {
		return map[string]any{
			"app_name": "lognugget",
			"version":  "1.0.0",
		}
	})
	config.SetContextFieldsParser(func(ctx context.Context) map[string]any {
		requestId := ctx.Value(ctxKeyRequestID)
		userId := ctx.Value(ctxKeyUserID)
		return map[string]any{
			"request_id": requestId,
			"user_id":    userId,
		}
	})

	unsetPostProcessor := pipelineStage.NewUnsetLogEventPostProcessor(2*time.Second, 500, out)
	defer unsetPostProcessor.Stop()

	pipelineStage.EventPreProcessorObj.RegisterHook(enum.LevelUnSet, unsetPostProcessor)
	config.InitPreProcessors(pipelineStage.EventPreProcessorObj)
	entry.GenerateInitialPool(1_000_000)

	ctxs := make([]context.Context, b.N)
	for i := range ctxs {
		ctxs[i] = context.WithValue(context.WithValue(context.Background(), ctxKeyRequestID, i), ctxKeyUserID, "User1234")
	}

	b.ReportAllocs()
	b.ResetTimer()
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		entryObj := entry.NewLogEntry()
		entryObj.Debug(ctxs[i], "debug message that has a log message, from lognugget", model.LogAttr{Key: model.LogAttrKey("itrr"), Value: model.LogAttrValue(i)})
	}
}
