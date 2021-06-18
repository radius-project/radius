// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package radlogger

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Radius uses the Zapr: https://github.com/go-logr/zapr which implements a logr interface
// for a zap log sink

const (
	DefaultLoggerName = "radiusRP"
	RadLogLevel       = "RADIUS_LOG_LEVEL" // Env variable that determines the log level
	RadProfile        = "RADIUS_PROFILE"   // Env variable that determines the logger config presets
)

// Log levels
const (
	Verbose                 = 1
	Normal                  = 0
	DefaultLogLevel       = Normal
	VerbosityLevelNormal  = "normal"
	VerbosityLevelVerbose = "verbose"
)

// Logger Profiles which determines the logger configuration
const (
	LoggerProfileProd    = "production"
	LoggerProfileDev     = "development"
	DefaultLoggerProfile = LoggerProfileProd
)

func InitRadLoggerConfig() (*zap.Logger, error) {
	var cfg zap.Config

	// Define the logger configuration based on the logger profile specified by RADIUS_PROFILE env variable
	profile := os.Getenv(RadProfile)
	if profile == "" {
		profile = DefaultLoggerProfile
	}
	if strings.EqualFold(profile, LoggerProfileDev) {
		cfg = zap.NewDevelopmentConfig()
	} else if strings.EqualFold(profile, LoggerProfileProd) {
		cfg = zap.NewProductionConfig()
	} else {
		return nil, fmt.Errorf("Invalid Radius Logger Profile set. Valid options are: %s, %s", LoggerProfileDev, LoggerProfileProd)
	}

	// Modify the default log level intialized by the profile preset if a custom value
	// is specified by "RADIUS_LOG_LEVEL" env variable
	radLogLevel := os.Getenv(RadLogLevel)
	var logLevel int
	if radLogLevel != "" {
		if strings.EqualFold(VerbosityLevelVerbose, radLogLevel) {
			logLevel = Verbose
		} else if strings.EqualFold(VerbosityLevelNormal, radLogLevel) {
			logLevel = Normal
		}
		cfg.Level = zap.NewAtomicLevelAt(zapcore.Level(logLevel))
	}

	// Build the logger config based on profile and custom presets
	logger, err := cfg.Build()
	if err != nil {
		return nil, fmt.Errorf("Unable to initialize zap logger: %v", err)
	}

	return logger, nil
}

func NewLogger(name string) (logr.Logger, error) {
	if name == "" {
		name = DefaultLoggerName
	}

	logConfig, err := InitRadLoggerConfig()
	if err != nil {
		return nil, err
	}
	log := zapr.NewLogger(logConfig)
	log = log.WithName(name)
	return log, nil
}

func WrapLogContext(ctx context.Context, keyValues ...interface{}) context.Context {
	logger := logr.FromContext(ctx).WithValues(keyValues...)
	ctx = logr.NewContext(ctx, logger)
	return ctx
}

func GetLogger(ctx context.Context) logr.Logger {
	return logr.FromContext(ctx)
}

func SetLogLevel(level zapcore.Level) {
	zap.NewAtomicLevelAt(level)
}
