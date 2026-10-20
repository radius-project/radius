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

package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_ShallowMergeParameters(t *testing.T) {
	tests := []struct {
		name     string
		base     map[string]any
		override map[string]any
		expected map[string]any
	}{
		{
			name:     "disjoint keys merged",
			base:     map[string]any{"a": "1"},
			override: map[string]any{"b": "2"},
			expected: map[string]any{"a": "1", "b": "2"},
		},
		{
			name:     "overlapping keys - override wins",
			base:     map[string]any{"a": "base", "b": "base"},
			override: map[string]any{"a": "override"},
			expected: map[string]any{"a": "override", "b": "base"},
		},
		{
			name: "nested object replaced not merged",
			base: map[string]any{
				"config": map[string]any{"x": 1, "y": 2},
			},
			override: map[string]any{
				"config": map[string]any{"z": 3},
			},
			expected: map[string]any{
				"config": map[string]any{"z": 3},
			},
		},
		{
			name:     "nil base",
			base:     nil,
			override: map[string]any{"a": "1"},
			expected: map[string]any{"a": "1"},
		},
		{
			name:     "nil override",
			base:     map[string]any{"a": "1"},
			override: nil,
			expected: map[string]any{"a": "1"},
		},
		{
			name:     "both nil",
			base:     nil,
			override: nil,
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ShallowMergeParameters(tt.base, tt.override)
			if tt.expected == nil {
				assert.Nil(t, result)
			} else {
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}
