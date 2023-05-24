/*
Copyright 2023 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package handlers

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetMapValue(t *testing.T) {
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
			in: map[string]any{
				"hello": false,
			},
			errString: "value is not string type",
			key:       "hello",
		},
		{
			in:        "invalid_type",
			errString: "unsupported type",
		},
	}

	for _, tc := range propertyTests {
		val, err := GetMapValue[string](tc.in, tc.key)
		if tc.errString != "" {
			require.ErrorContains(t, err, tc.errString)
		} else {
			require.Equal(t, tc.val, val)
		}
	}
}
