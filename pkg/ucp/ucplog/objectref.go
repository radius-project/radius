// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package ucplog

import (
	"errors"
	"os"

	"github.com/project-radius/radius/pkg/version"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var errNonStringKey = errors.New("non-string key type")

// NewResourceObject returns the resource object which includes the system info.
func NewResourceObject(serviceName string) map[string]any {
	host, _ := os.Hostname()
	return map[string]any{
		string(semconv.ServiceNameKey):    serviceName,
		string(semconv.ServiceVersionKey): version.Channel(),
		string(semconv.HostNameKey):       host,
	}
}

// Attributes creates attributes object including the additional properties and info for Radius.
// This leverages zapcore.ObjectMarshaler to define the custom attributes, so it works only for uber/zap.
func Attributes(keysAndValues ...any) zap.Field {
	l := len(keysAndValues)
	if l%2 != 0 {
		return zap.String(LogFieldAttributes, "invalid key and value pairs")
	}
	attr := &attributeMarshaller{keysAndValues: keysAndValues}
	return zap.Object(LogFieldAttributes, attr)
}

var _ zapcore.ObjectMarshaler = (*attributeMarshaller)(nil)

type attributeMarshaller struct {
	keysAndValues []any
}

func (r *attributeMarshaller) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	l := len(r.keysAndValues)
	for i := 0; i < l; i += 2 {
		key, ok := r.keysAndValues[i].(string)
		if !ok {
			return errNonStringKey
		}
		val := r.keysAndValues[i+1]
		zap.Any(key, val).AddTo(enc)
	}
	return nil
}
