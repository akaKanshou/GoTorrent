package internal

import "go.uber.org/zap"

// LogDebug is a wrapper function for zap.Logger::Debug
// Used in case logger is moved to a separate package
func LogDebug(msg string, fields ...zap.Field) {
	logger.Debug(msg, fields...)
}

// LogError is a wrapper function for zap.Logger::Error
// Used in case logger is moved to a separate package
func LogError(msg string, fields ...zap.Field) {
	logger.Error(msg, fields...)
}

// LogInfo is a wrapper function for zap.Logger::Info
// Used in case logger is moved to a separate package
func LogInfo(msg string, fields ...zap.Field) {
	logger.Info(msg, fields...)
}

// LogPanic is a wrapper function for zap.Logger::Panic
// Used in case logger is moved to a separate package
func LogPanic(msg string, fields ...zap.Field) {
	logger.Panic(msg, fields...)
}

// LogWarn is a wrapper function for zap.Logger::Warn
// Used in case logger is moved to a separate package
func LogWarn(msg string, fields ...zap.Field) {
	logger.Warn(msg, fields...)
}
