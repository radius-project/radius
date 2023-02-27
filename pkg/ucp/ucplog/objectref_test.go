// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package ucplog

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zapcore"
)

func TestAttributes_InvalidKeyValuePairs(t *testing.T) {
	keyPairTests := []struct {
		name string
		in   []any
	}{
		{
			name: "odd number of key value item",
			in:   []any{"key", "value", "key2"},
		},
		{
			name: "invalid key type",
			in:   []any{"key", "value", struct{}{}, "value2"},
		},
	}

	for _, tc := range keyPairTests {
		t.Run(tc.name, func(t *testing.T) {
			field := Attributes(context.TODO(), tc.in...)
			require.Equal(t, LogFieldAttributes, field.Key)
			require.Equal(t, "invalid key and value pairs", field.String)
		})
	}
}

func TestAttributeMarshaller(t *testing.T) {
	marshallerTests := []struct {
		name      string
		ctx       context.Context
		contextKV []any
		kv        []any
		out       map[string]any
	}{
		{
			name:      "valid key-value pairs with empty context",
			ctx:       context.TODO(),
			contextKV: []any{},
			kv:        []any{"key", "value", "key2", "value2"},
			out: map[string]any{
				"key":  "value",
				"key2": "value2",
			},
		},
		{
			name:      "valid key-value pairs with context attributes",
			ctx:       context.TODO(),
			contextKV: []any{"ctxkey", "ctxvalue", "ctxkey2", "ctxvalue2"},
			kv:        []any{"key", "value", "key2", "value2"},
			out: map[string]any{
				"ctxkey":  "ctxvalue",
				"ctxkey2": "ctxvalue2",
				"key":     "value",
				"key2":    "value2",
			},
		},
		{
			name:      "only context attributes",
			ctx:       context.TODO(),
			contextKV: []any{"ctxkey", "ctxvalue", "ctxkey2", "ctxvalue2"},
			kv:        []any{},
			out: map[string]any{
				"ctxkey":  "ctxvalue",
				"ctxkey2": "ctxvalue2",
			},
		},
	}

	for _, tc := range marshallerTests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := WithAttribute(tc.ctx, tc.contextKV...)
			field := Attributes(ctx, tc.kv...)
			require.Equal(t, LogFieldAttributes, field.Key)
			require.Equal(t, zapcore.ObjectMarshalerType, field.Type)

			marshaller, ok := field.Interface.(*attributeMarshaller)
			require.True(t, ok)
			enc := zapcore.NewMapObjectEncoder()

			err := marshaller.MarshalLogObject(enc)
			require.NoError(t, err)
			require.Equal(t, tc.out, enc.Fields)
		})
	}
}
