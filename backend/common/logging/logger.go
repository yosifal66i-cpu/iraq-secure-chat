package logging

import (
	"os"
	"sync"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	instance *zap.Logger
	once     sync.Once
)

func Init(serviceName, environment, logLevel string) *zap.Logger {
	once.Do(func() {
		var level zapcore.Level
		switch logLevel {
		case "debug":
			level = zapcore.DebugLevel
		case "info":
			level = zapcore.InfoLevel
		case "warn":
			level = zapcore.WarnLevel
		case "error":
			level = zapcore.ErrorLevel
		default:
			level = zapcore.InfoLevel
		}

		encoderConfig := zapcore.EncoderConfig{
			TimeKey:        "@timestamp",
			LevelKey:       "level",
			NameKey:        "logger",
			CallerKey:      "caller",
			FunctionKey:    zapcore.OmitKey,
			MessageKey:     "message",
			StacktraceKey:  "stacktrace",
			LineEnding:     zapcore.DefaultLineEnding,
			EncodeLevel:    zapcore.CapitalLevelEncoder,
			EncodeTime:     zapcore.ISO8601TimeEncoder,
			EncodeDuration: zapcore.MillisDurationEncoder,
			EncodeCaller:   zapcore.ShortCallerEncoder,
		}

		var cores []zapcore.Core

		// Always log to stdout
		consoleEncoder := zapcore.NewJSONEncoder(encoderConfig)
		consoleCore := zapcore.NewCore(consoleEncoder, zapcore.AddSync(os.Stdout), level)
		cores = append(cores, consoleCore)

		// In production, add file logging
		if environment == "production" {
			logFile, err := os.OpenFile("/var/log/iraqchat/"+serviceName+".log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
			if err == nil {
				fileCore := zapcore.NewCore(
					zapcore.NewJSONEncoder(encoderConfig),
					zapcore.AddSync(logFile),
					level,
				)
				cores = append(cores, fileCore)
			}
		}

		tee := zapcore.NewTee(cores...)
		logger := zap.New(tee, zap.AddCaller(), zap.AddStacktrace(zapcore.ErrorLevel))
		logger = logger.With(
			zap.String("service", serviceName),
			zap.String("environment", environment),
		)

		instance = logger
	})

	return instance
}

func Get() *zap.Logger {
	if instance == nil {
		return Init("unknown", "development", "info")
	}
	return instance
}

type Fields map[string]interface{}

func With(fields Fields) *zap.Logger {
	var zapFields []zap.Field
	for k, v := range fields {
		zapFields = append(zapFields, zap.Any(k, v))
	}
	return Get().With(zapFields...)
}
