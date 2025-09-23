package benchmark

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/architagr/lognugget/config"
	"github.com/architagr/lognugget/entry"
	"github.com/architagr/lognugget/enum"
	"github.com/architagr/lognugget/model"
	pipelineStage "github.com/architagr/lognugget/pipeline_stage"
	"github.com/rs/zerolog"
)

type traceHook struct {
}

func (h traceHook) Run(e *zerolog.Event, level zerolog.Level, msg string) {
	ctx := e.GetCtx()
	if ctx != nil {
		requestId := ctx.Value("requestID")
		userId := ctx.Value("userID")
		e.Str("request_id", requestId.(string))
		e.Str("user_id", userId.(string))
	}
}

// 1481232           805.6 ns/op      1279 B/op         10 allocs/op
// 1704498	       641.8 ns/op	     680 B/op	       9 allocs/op
func Benchmark_ZeroLog(b *testing.B) {
	b.StopTimer()
	obj := zerolog.New(&MockWriter{}).With().Timestamp().Logger().Hook(traceHook{})
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		ctx := context.WithValue(context.WithValue(context.Background(), "requestID", fmt.Sprint(i)), "userID", "User1234")
		z := obj.With().Ctx(ctx).Logger()
		z.Debug().Fields(map[string]any{"itrr": i}).Msg("debug message that has a log message")
	}

}

type MockWriter struct {
}

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
	config.SetOutput(&MockWriter{})
	config.SetMinLevel(enum.LevelDebug)
	config.SetEncoderType(enum.EncoderJSON)
	config.SetStaticEnvFieldsParser(func() map[string]any {
		return map[string]any{
			"app_name": "lognugget",
			"version":  "1.0.0",
		}
	})
	config.SetContextFieldsParser(func(ctx context.Context) map[string]any {
		requestId := ctx.Value("requestID")
		userId := ctx.Value("userID")
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
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		ctx := context.WithValue(context.WithValue(context.Background(), "requestID", i), "userID", "User1234")
		entryObj := entry.NewLogEntry()
		entryObj.Debug(ctx, "debug message that has a log message, from lognugget", model.LogAttr{Key: model.LogAttrKey("itrr"), Value: model.LogAttrValue(i)})
	}
}
