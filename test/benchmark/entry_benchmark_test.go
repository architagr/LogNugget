package benchmark

import (
	"bytes"
	"context"
	"fmt"
	"os"
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

func Benchmark_ZeroLog(b *testing.B) {
	b.StopTimer()
	var buf bytes.Buffer
	obj := zerolog.New(&buf).With().Timestamp().Logger().Hook(traceHook{})
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		ctx := context.WithValue(context.WithValue(context.Background(), "requestID", fmt.Sprint(i)), "userID", "User1234")
		z := obj.With().Ctx(ctx).Logger()
		z.Debug().Fields(map[string]any{"itrr": i, "time": time.Now()}).Msg("debug message that has a log message")
	}

}

// 279054	      4163 ns/op	    2094 B/op	      36 allocs/op

func Benchmark_Log(b *testing.B) {
	b.StopTimer()
	var buf bytes.Buffer
	config.SetOutput(&buf)
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

	unsetPostProcessor := pipelineStage.NewUnsetLogEventPostProcessor(5*time.Second, 10, os.Stdout)
	defer unsetPostProcessor.Stop()

	pipelineStage.EventPreProcessorObj.RegisterHook(enum.LevelUnSet, unsetPostProcessor)
	config.InitPreProcessors(pipelineStage.EventPreProcessorObj)
	entry.GenerateInitialPool(100_000)
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		ctx := context.WithValue(context.WithValue(context.Background(), "requestID", i), "userID", "User1234")
		entryObj := entry.NewLogEntry()
		entryObj.Debug(ctx, "debug message that has a log message", model.LogAttr{Key: model.LogAttrKey("itrr"), Value: model.LogAttrValue(i)}, model.LogAttr{Key: model.LogAttrKey("time"), Value: model.LogAttrValue(time.Now)})
	}
}
