// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package ucplog

import (
	"context"

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func NewLogger() logr.Logger {
	config := zap.NewDevelopmentConfig()
	config.EncoderConfig.CallerKey = zapcore.OmitKey
	config.EncoderConfig.EncodeTime = zapcore.TimeEncoderOfLayout("2006-01-02 15:04:05")
	zapLog, err := config.Build()
	if err != nil {
		panic("failed to create logger")
	}

	return zapr.NewLogger(zapLog)
}

func GetLogger(ctx context.Context) logr.Logger {
	return logr.FromContextOrDiscard(ctx)
}

func Unwrap(logger logr.Logger) *zap.Logger {
	underlier, ok := logger.GetSink().(zapr.Underlier)
	if ok {
		return underlier.GetUnderlying()
	}

	return nil
}
