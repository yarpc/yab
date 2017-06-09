package main

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// getLoggerVerbosity returns an appropriate logging level based on flags.
func getLoggerVerbosity(verbosity []bool) zapcore.Level {
	switch len(verbosity) {
	case 0:
		return zap.WarnLevel
	case 1:
		return zap.InfoLevel
	default:
		return zap.DebugLevel
	}
}

func configureLoggerConfig(opts *Options) zap.Config {
	loggerConfig := zap.NewDevelopmentConfig()
	loggerConfig.EncoderConfig = zapcore.EncoderConfig{
		// Keys can be anything except the empty string.
		TimeKey:        "T",
		LevelKey:       "L",
		NameKey:        "N",
		CallerKey:      "C",
		MessageKey:     "M",
		StacktraceKey:  "S",
		EncodeLevel:    zapcore.CapitalLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.StringDurationEncoder,
	}

	// Set the logger level based on the command line parsed options.
	loggerConfig.Level.SetLevel(getLoggerVerbosity(opts.Verbosity))

	return loggerConfig
}
