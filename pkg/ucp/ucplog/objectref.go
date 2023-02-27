// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package ucplog

import (
	"context"
	"errors"
	"os"

	"github.com/project-radius/radius/pkg/version"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type contextKey struct {
	keyType string
}

var (
	errNonStringKey                      = errors.New("non-string key type")
	radiusAttributeContextKey contextKey = contextKey{"radius-attribute"}
)

// NewResourceObject returns the resource object which includes the system info.
func NewResourceObject(serviceName string) map[string]any {
	host, _ := os.Hostname()
	return map[string]any{
		string(semconv.ServiceNameKey):    serviceName,
		string(semconv.ServiceVersionKey): version.Channel(),
		string(semconv.HostNameKey):       host,
	}
}

func validateKeyValues(keysAndValues ...any) bool {
	l := len(keysAndValues)
	if l%2 != 0 {
		return false
	}

	for i := 0; i < l; i += 2 {
		_, ok := keysAndValues[i].(string)
		if !ok {
			return false
		}
	}

	return true
}

// WithAttribute returns a copy of parent in which radiusAttributeContextKey are added.
// To use attributes in context, ucplog.Attribute(ctx) must be included in logger as a key value.
// For instance, logger.Info("hello radius", ucplog.Attribute(ctx))
func WithAttribute(ctx context.Context, keysAndValues ...any) context.Context {
	if ctx == nil {
		ctx = context.TODO()
	}
	if !validateKeyValues(keysAndValues...) {
		return ctx
	}
	return context.WithValue(ctx, radiusAttributeContextKey, keysAndValues)
}

// Attributes creates attributes object including the additional properties and info for Radius log.
// This leverages zapcore.ObjectMarshaler to define the custom attributes, so it works only for uber/zap.
func Attributes(ctx context.Context, keysAndValues ...any) zap.Field {
	attr, ok := ctx.Value(radiusAttributeContextKey).([]any)
	if !ok {
		attr = nil
	}
	if !validateKeyValues(keysAndValues...) {
		return zap.String(LogFieldAttributes, "invalid key and value pairs")
	}
	marshaller := &attributeMarshaller{contextAttributes: attr, keysAndValues: keysAndValues}
	return zap.Object(LogFieldAttributes, marshaller)
}

var _ zapcore.ObjectMarshaler = (*attributeMarshaller)(nil)

type attributeMarshaller struct {
	contextAttributes []any
	keysAndValues     []any
}

func (r *attributeMarshaller) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	fn := func(kv []any) error {
		if kv == nil {
			return nil
		}
		for i := 0; i < len(kv); i += 2 {
			key, ok := kv[i].(string)
			if !ok {
				return errNonStringKey
			}
			val := kv[i+1]
			zap.Any(key, val).AddTo(enc)
		}
		return nil
	}

	if err := fn(r.contextAttributes); err != nil {
		return err
	}

	return fn(r.keysAndValues)
}
