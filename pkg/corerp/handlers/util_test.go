// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handlers

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetStringProperty(t *testing.T) {
	propertyTests := []struct {
		in        any
		errString string
		key       string
		val       string
	}{
		{
			in: map[string]string{
				"hello": "world",
			},
			key: "hello",
			val: "world",
		},
		{
			in: map[string]any{
				"hello": "world",
			},
			key: "hello",
			val: "world",
		},
		{
			in: map[string]any{
				"hello": "world",
			},
			errString: "key1 not found",
			key:       "key1",
		},
		{
			in:        "invalid_type",
			errString: "unsupported type",
		},
	}

	for _, tc := range propertyTests {
		val, err := GetStringProperty(tc.in, tc.key)
		if tc.errString != "" {
			require.ErrorContains(t, err, tc.errString)
		} else {
			require.Equal(t, tc.val, val)
		}
	}
}
