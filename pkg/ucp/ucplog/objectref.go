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

// WithAttributes returns a copy of parent context with additional attribute properties for logger.
// To emit the additional properties, ucplog.Attribute(ctx) should be used in logger's arguments
// along with message.
func WithAttributes(ctx context.Context, keysAndValues ...any) context.Context {
	if ctx == nil {
		panic("ctx is nil")
	}
	return context.WithValue(ctx, radiusAttributeContextKey, keysAndValues)
}

// Attributes creates attributes object including the additional properties and info for Radius log.
func Attributes(ctx context.Context, keysAndValues ...any) zap.Field {
	attr, ok := ctx.Value(radiusAttributeContextKey).([]any)
	if !ok {
		attr = nil
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
