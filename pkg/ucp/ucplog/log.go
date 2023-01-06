// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package ucplog

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest"
)

// Radius uses the Zapr: https://github.com/go-logr/zapr which implements a logr interface
// for a zap log sink
const (
	DefaultLoggerName string = "radius"
	LogLevel          string = "RADIUS_LOG_LEVEL"   // Env variable that determines the log level
	LogProfile        string = "RADIUS_LOG_PROFILE" // Env variable that determines the logger config presets
)

// Log levels
const (
	// More details on verbosity levels can be found here: https://pkg.go.dev/go.uber.org/zap@v1.20.0/zapcore#DebugLevel
	// We do not want to support levels that introduce a new control flow
	Error int = 1
	Warn  int = 2
	Info  int = 3
	//Verbose = 4
	Debug int = 9
	//Trace   = 10
	DefaultLogLevel     int    = Info
	VerbosityLevelInfo  string = "info"
	VerbosityLevelDebug string = "debug"
	VerbosityLevelError string = "error"
	VerbosityLevelWarn  string = "warn"
)

// Logger Profiles which determines the logger configuration
const (
	LoggerProfileProd    string = "production"
	LoggerProfileDev     string = "development"
	DefaultLoggerProfile        = LoggerProfileDev
)

func initRadLoggerConfig() (*zap.Logger, error) {
	var cfg zap.Config

	// Define the logger configuration based on the logger profile specified by RADIUS_PROFILE env variable
	profile := os.Getenv(LogProfile)
	if profile == "" {
		profile = DefaultLoggerProfile
	}
	if strings.EqualFold(profile, LoggerProfileDev) {
		cfg = zap.NewDevelopmentConfig()
	} else if strings.EqualFold(profile, LoggerProfileProd) {
		cfg = zap.NewProductionConfig()
	} else {
		return nil, fmt.Errorf("invalid Radius Logger Profile set. Valid options are: %s, %s", LoggerProfileDev, LoggerProfileProd)
	}

	// Modify the default log level intialized by the profile preset if a custom value
	// is specified by "RADIUS_LOG_LEVEL" env variable
	radLogLevel := os.Getenv(LogLevel)
	var logLevel int
	if radLogLevel != "" {
		if strings.EqualFold(VerbosityLevelDebug, radLogLevel) {
			logLevel = int(zapcore.DebugLevel)
		} else if strings.EqualFold(VerbosityLevelInfo, radLogLevel) {
			logLevel = int(zapcore.InfoLevel)
		} else if strings.EqualFold(VerbosityLevelWarn, radLogLevel) {
			logLevel = int(zapcore.WarnLevel)
		} else if strings.EqualFold(VerbosityLevelError, radLogLevel) {
			logLevel = int(zapcore.ErrorLevel)
		} else {
			return nil, fmt.Errorf("invalid Radius Logger Level set. Valid options are: %s, %s, %s, %s", VerbosityLevelError, VerbosityLevelWarn, VerbosityLevelInfo, VerbosityLevelDebug)
		}
		cfg.Level = zap.NewAtomicLevelAt(zapcore.Level(logLevel))
	}
	cfg.EncoderConfig.CallerKey = zapcore.OmitKey
	cfg.EncoderConfig.EncodeTime = zapcore.TimeEncoderOfLayout("2006-01-02 15:04:05")
	// Build the logger config based on profile and custom presets
	logger, err := cfg.Build()
	if err != nil {
		return nil, fmt.Errorf("unable to initialize zap logger: %v", err)
	}

	return logger, nil
}

// NewLogger creates a new logr.Logger with zap logger implementation
func NewLogger(name string) (logr.Logger, func(), error) {
	if name == "" {
		name = DefaultLoggerName
	}

	zapLogger, err := initRadLoggerConfig()
	if err != nil {
		return logr.Discard(), nil, err
	}
	logger := zapr.NewLogger(zapLogger).WithName(name)

	// The underlying zap logger needs to be flushed before server exits
	flushLogs := func() {
		err := zapLogger.Sync()
		if err != nil {
			logger.Error(err, "Unable to flush logs...")
		}
	}
	return logger, flushLogs, nil
}

// NewTestLogger creates a new logr.Logger with zaptest logger implementation
func NewTestLogger(t *testing.T) (logr.Logger, error) {
	zapLogger := zaptest.NewLogger(t)
	log := zapr.NewLogger(zapLogger)
	return log, nil
}

// GetLogger gets the logr.Logger from supplied context
func GetLogger(ctx context.Context) logr.Logger {
	return logr.FromContextOrDiscard(ctx)
}

// WrapLogContext modifies the log context in provided context to include the keyValues provided, and returns this modified context
func WrapLogContext(ctx context.Context, keyValues ...any) context.Context {
	logger := logr.FromContextOrDiscard(ctx)

	ctx = logr.NewContext(ctx, logger.WithValues(keyValues...))
	return ctx
}

// Unwrap returns the underlying zap logger of logr.Logger
func Unwrap(logger logr.Logger) *zap.Logger {
	underlier, ok := logger.GetSink().(zapr.Underlier)
	if ok {
		return underlier.GetUnderlying()
	}

	return nil
}
