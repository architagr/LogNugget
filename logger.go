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
		return map[string]any{
			"request_id": requestId,
			"user_id":    userId,
		}
	})

}

func main() {
	go func() {
		log.Println(http.ListenAndServe("localhost:6060", nil))
	}()
	unsetPostProcessor := pipelineStage.NewUnsetLogEventPostProcessor(5*time.Second, 10, os.Stdout)

	pipelineStage.EventPreProcessorObj.RegisterHook(enum.LevelUnSet, unsetPostProcessor)
	config.InitPreProcessors(pipelineStage.EventPreProcessorObj)
	entry.GenerateInitialPool(10_000)

	engine := gin.New()

	engine.GET("/v1/user", func(ctx *gin.Context) {
		ctxObj := context.WithValue(context.WithValue(ctx.Request.Context(), "requestID", 1), "userID", "User1234")
		entryObj := entry.NewLogEntry()
		a := []model.LogAttr{
			{Key: model.LogAttrKey("itrr"), Value: model.LogAttrValue(ctx.RemoteIP)},
			{Key: model.LogAttrKey("time"), Value: model.LogAttrValue(time.Now)},
		}
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
