package logger

import (
	"fmt"
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var Logger *zap.Logger

func init() {
	err := os.MkdirAll("./temp/logs", os.ModeDir|os.ModePerm)
	if err != nil {
		panic(err)
	}

	config := zap.NewDevelopmentConfig()
	config.EncoderConfig.EncodeTime = zapcore.TimeEncoderOfLayout("2006-01-02 15:04:05")
	config.OutputPaths = []string{"./temp/logs/output"}
	config.ErrorOutputPaths = []string{"stderr", "./temp/logs/errorOutput"}

	Logger, err = config.Build()
	if err != nil {
		panic(err)
	}

	Logger.Info("Logger initialized")
}

func SyncLog() {
	if err := Logger.Sync(); err != nil {
		fmt.Println(err)
	}
}
