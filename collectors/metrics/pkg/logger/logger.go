package logger

import (
	"fmt"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
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
	errkey := "failover_err_%d"
	switch l {
	case Debug:
		err := level.Debug(log).Log(keyvals...)
		if err != nil {
			fmt.Sprintf(errkey, err)
		}
	case Info:
		err := level.Info(log).Log(keyvals...)
		if err != nil {
			fmt.Sprintf(errkey, err)
		}
	case Warn:
		err := level.Warn(log).Log(keyvals...)
		if err != nil {
			fmt.Sprintf(errkey, err)
		}
	case Error:
		err := level.Error(log).Log(keyvals...)
		if err != nil {
			fmt.Sprintf(errkey, err)
		}
	}
}
