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

// UCP Logging context fields
const (
	LogFieldPlaneID       string = "PlaneID"
	LogFieldPlaneKind     string = "PlaneKind"
	LogFieldRequestPath   string = "Path"
	LogFieldHTTPScheme    string = "HTTPScheme"
	LogFieldPlaneURL      string = "ProxyURL"
	LogFieldProvider      string = "Provider"
	LogFieldResourceGroup string = "ResourceGroup"
	LogFieldHTTPMethod    string = "HttpMethod"
	LogFieldRequestURL    string = "RequestURL"
	LogFieldContentLength string = "ContentLength"
	LogFieldUCPHost       string = "UCPHost"
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

func WrapLogContext(ctx context.Context, keyValues ...any) context.Context {
	logger := logr.FromContextOrDiscard(ctx)

	ctx = logr.NewContext(ctx, logger.WithValues(keyValues...))
	return ctx
}

func Unwrap(logger logr.Logger) *zap.Logger {
	underlier, ok := logger.GetSink().(zapr.Underlier)
	if ok {
		return underlier.GetUnderlying()
	}

	return nil
}
