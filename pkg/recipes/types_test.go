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

package recipes

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRecipeOutput_PrepareRecipeResponse(t *testing.T) {
	tests := []struct {
		desc        string
		result      map[string]any
		expectedErr bool
	}{
		{
			desc: "all valid result values",
			result: map[string]any{
				"values": map[string]any{
					"host": "testhost",
					"port": float64(6379),
				},
				"secrets": map[string]any{
					"connectionString": "testConnectionString",
				},
				"resources": []string{"outputResourceId1"},
			},
		},
		{
			desc:   "empty result",
			result: map[string]any{},
		},
		{
			desc: "invalid field",
			result: map[string]any{
				"invalid": "invalid-field",
			},
			expectedErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			ro := &RecipeOutput{}
			if !tt.expectedErr {
				err := ro.PrepareRecipeResponse(tt.result)
				require.NoError(t, err)

				if tt.result["values"] != nil {
					require.Equal(t, tt.result["values"], ro.Values)
					require.Equal(t, tt.result["secrets"], ro.Secrets)
					require.Equal(t, tt.result["resources"], ro.OutputResources)
				} else {
					require.Equal(t, map[string]any{}, ro.Values)
					require.Equal(t, map[string]any{}, ro.Secrets)
					require.Equal(t, []string{}, ro.OutputResources)
				}
			} else {
				err := ro.PrepareRecipeResponse(tt.result)
				require.Error(t, err)
				require.Equal(t, "json: unknown field \"invalid\"", err.Error())
			}
		})
	}
}
