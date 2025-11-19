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

package cmd

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_HandlePanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatal("handlePanic should recover and not propagate panic")
		}
	}()

	func() {
		defer handlePanic()
		panic("test panic")
	}()
}

func Test_prettyPrintRPError(t *testing.T) {
	err := fmt.Errorf("test error message")
	result := prettyPrintRPError(err)
	require.Contains(t, result, "test error")
}

func Test_prettyPrintJSON(t *testing.T) {
	t.Run("formats JSON correctly", func(t *testing.T) {
		obj := map[string]string{"key": "value"}
		result, err := prettyPrintJSON(obj)
		require.NoError(t, err)
		require.Contains(t, result, "key")
		require.Contains(t, result, "value")
		require.Contains(t, result, "\n")
	})

	t.Run("handles invalid JSON", func(t *testing.T) {
		invalidObj := make(chan int)
		_, err := prettyPrintJSON(invalidObj)
		require.Error(t, err)
	})

	t.Run("formats complex objects", func(t *testing.T) {
		obj := map[string]any{
			"nested": map[string]string{"inner": "value"},
			"array":  []string{"a", "b", "c"},
		}
		result, err := prettyPrintJSON(obj)
		require.NoError(t, err)
		require.Contains(t, result, "nested")
		require.Contains(t, result, "inner")
		require.Contains(t, result, "array")
	})
}
