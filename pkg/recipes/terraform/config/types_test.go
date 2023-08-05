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

package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSetParams(t *testing.T) {
	c := TFModuleConfig{}

	c.SetParams(RecipeParams{
		"foo": map[string]any{
			"bar": "baz",
		},
		"bar": map[string]any{
			"baz": "foo",
		},
	})

	require.Equal(t, 2, len(c))
	require.Equal(t, c["foo"].(map[string]any), map[string]any{"bar": "baz"})
	require.Equal(t, c["bar"].(map[string]any), map[string]any{"baz": "foo"})
}
