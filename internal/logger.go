package internal

import (
	"fmt"
	"os"

	"go.uber.org/zap"
)

var logger *zap.Logger

func init() {
	err := os.MkdirAll("./temp/logs", os.ModeDir|os.ModePerm)
	if err != nil {
		panic(err)
	}

	config := zap.NewDevelopmentConfig()
	config.OutputPaths = []string{"stderr", "./temp/logs/output"}
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
