package main

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// VerbosityLevel maps to a level of logs to be enabled on the command
type VerbosityLevel uint8

const (
	// VerbosityLevelOff no log messages displayed
	VerbosityLevelOff VerbosityLevel = iota
	// VerbosityLevelInfo enables printing up to Info level command log statements
	VerbosityLevelInfo
	// VerbosityLevelDebug enables printing up to Debug level command log statements
	VerbosityLevelDebug
)

// GetLoggerVerbosity takes a VerbosityLevel and returns an appropriate logging level
func GetLoggerVerbosity(lvl VerbosityLevel) zapcore.Level {
	zlvl := zap.DebugLevel
	switch lvl {
	case VerbosityLevelOff:
		zlvl = zap.WarnLevel
	case VerbosityLevelInfo:
		zlvl = zap.InfoLevel
	}
	return zlvl
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
	loggerConfig.Level.SetLevel(GetLoggerVerbosity(VerbosityLevel(len(opts.Verbosity))))

	return loggerConfig
}
