package logger

import (
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
)

type LogLevel string

const (
	Debug LogLevel = "debug"
	Info  LogLevel = "info"
	Warn  LogLevel = "warn"
	Error LogLevel = "error"
)

// LogLevelFromString determines log level to string, defaults to all,
func LogLevelFromString(l string) level.Option {
	switch l {
	case "debug":
		return level.AllowDebug()
	case "info":
		return level.AllowInfo()
	case "warn":
		return level.AllowWarn()
	case "error":
		return level.AllowError()
	default:
		return level.AllowAll()
	}
}

// Log is used to handle the error of logger.Log globally
func Log(log log.Logger, l LogLevel, keyvals ...interface{}) {
	switch l {
	// ignore errors during logging
	case Debug:
		_ = level.Debug(log).Log(keyvals...)
	case Info:
		_ = level.Info(log).Log(keyvals...)
	case Warn:
		_ = level.Warn(log).Log(keyvals...)
	case Error:
		_ = level.Error(log).Log(keyvals...)
	}
}
