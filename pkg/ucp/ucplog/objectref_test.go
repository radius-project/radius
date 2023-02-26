// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package ucplog

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zapcore"
)

func TestAttributes_InvalidKeyValuePairs(t *testing.T) {
	field := Attributes("key", "value", "key2")
	require.Equal(t, LogFieldAttributes, field.Key)
	require.Equal(t, "invalid key and value pairs", field.String)
}

func TestAttributeMarshaller(t *testing.T) {
	t.Run("valid key-value pairs", func(t *testing.T) {
		field := Attributes("key", "value", "key2", "value2")
		require.Equal(t, LogFieldAttributes, field.Key)
		require.Equal(t, zapcore.ObjectMarshalerType, field.Type)

		marshaller, ok := field.Interface.(*attributeMarshaller)
		require.True(t, ok)
		enc := zapcore.NewMapObjectEncoder()

		err := marshaller.MarshalLogObject(enc)
		require.NoError(t, err)
		require.Equal(t, "value", enc.Fields["key"])
		require.Equal(t, "value2", enc.Fields["key2"])
	})

	t.Run("invalid key type", func(t *testing.T) {
		field := Attributes("key", "value", struct{}{}, "value2")
		require.Equal(t, LogFieldAttributes, field.Key)
		require.Equal(t, zapcore.ObjectMarshalerType, field.Type)

		marshaller, ok := field.Interface.(*attributeMarshaller)
		require.True(t, ok)
		enc := zapcore.NewMapObjectEncoder()

		err := marshaller.MarshalLogObject(enc)
		require.ErrorIs(t, err, errNonStringKey)
	})
}
