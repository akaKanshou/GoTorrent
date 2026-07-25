package internal

import (
	"fmt"
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var logger *zap.Logger

func init() {
	err := os.MkdirAll("./temp/logs", os.ModeDir|os.ModePerm)
	if err != nil {
		panic(err)
	}

	config := zap.NewDevelopmentConfig()
	config.EncoderConfig.EncodeTime = zapcore.TimeEncoderOfLayout("2006-01-02 15:04:05")
	config.OutputPaths = []string{"./temp/logs/output"}
	config.ErrorOutputPaths = []string{"stderr", "./temp/logs/errorOutput"}

	logger, err = config.Build()
	if err != nil {
		panic(err)
	}

	logger.Info("Logger initialized")
}

func SyncLog() {
	if err := logger.Sync(); err != nil {
		fmt.Println(err)
	}
}
