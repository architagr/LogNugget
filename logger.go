package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"
	"time"

	"github.com/architagr/lognugget/config"
	"github.com/architagr/lognugget/entry"
	"github.com/architagr/lognugget/enum"
	"github.com/architagr/lognugget/model"
	pipelineStage "github.com/architagr/lognugget/pipeline_stage"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
)

func init() {
	config.SetOutput(os.Stdout)
	config.SetMinLevel(enum.LevelDebug)
	config.SetEncoderType(enum.EncoderJSON)
	config.SetStaticEnvFieldsParser(func() map[string]any {
		return map[string]any{
			"app_name": "lognugget",
			"version":  "1.0.0",
		}
	})
	config.SetContextFieldsParser(func(ctx context.Context) map[string]any {
		requestId := ctx.Value("request_id")
		userId := ctx.Value("user_id")
		m := make(map[string]any, 2)
		if requestId != nil {
			m["request_id"] = requestId
		}
		if userId != nil {
			m["user_id"] = userId
		}
		return m
	})

}

type traceHook struct {
}

func (h traceHook) Run(e *zerolog.Event, level zerolog.Level, msg string) {
	ctx := e.GetCtx()
	if ctx != nil {
		requestId := ctx.Value("request_id")
		userId := ctx.Value("user_id")
		if requestId != nil {
			e.Str("request_id", requestId.(string))
		}
		if userId != nil {
			e.Str("user_id", userId.(string))
		}
	}
}

func main() {
	go func() {
		log.Println(http.ListenAndServe("localhost:6060", nil))
	}()
	unsetPostProcessor := pipelineStage.NewUnsetLogEventPostProcessor(5*time.Second, 10, config.GetConfig().Output())

	pipelineStage.EventPreProcessorObj.RegisterHook(enum.LevelUnSet, unsetPostProcessor)
	config.InitPreProcessors(pipelineStage.EventPreProcessorObj)
	entry.GenerateInitialPool(10_000)

	engine := gin.New()
	// obj := zerolog.New(os.Stdout).With().Timestamp().Logger().Hook(traceHook{})

	engine.GET("/v1/user", func(ctx *gin.Context) {
		a := []model.LogAttr{
			{Key: model.LogAttrKey("itrr"), Value: model.LogAttrValue(ctx.RemoteIP())},
			{Key: model.LogAttrKey("time"), Value: model.LogAttrValue(time.Now())},
		}
		ctxObj := context.WithValue(context.WithValue(ctx.Request.Context(), "requestID", 1), "userID", "User1234")
		// z := obj.With().Ctx(ctx).Logger()
		// z.Debug().Fields(map[string]any{"itrr": ctx.RemoteIP(), "time": time.Now()}).Msg("debug message that has a log message from zero log")

		entryObj := entry.NewLogEntry()

		entryObj.Debug(ctxObj, "debug message that has a log message", a...)
		ctx.JSON(http.StatusOK, gin.H{
			"message": "user retrieved",
		})
	})
	if err := engine.Run(":8081"); err != nil {
		log.Fatalf("[server] Failed to start server: %v", err)
	}
	fmt.Println("server stoped")
	unsetPostProcessor.Stop()
}
