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

package reconciler

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_deleteAPIVersion(t *testing.T) {
	t.Parallel()

	const id = "/planes/radius/local/resourceGroups/g/providers/Radius.Compute/containers/vote"
	tests := []struct {
		name     string
		template string
		id       string
		expected string
	}{
		{
			name:     "symbolic names",
			template: `{"resources":{"vote":{"type":"Radius.Compute/containers@2025-08-01-preview"}}}`,
			id:       id,
			expected: "2025-08-01-preview",
		},
		{
			name:     "resource array, case-insensitive type",
			template: `{"resources":[{"type":"radius.compute/Containers@2025-08-01-preview"}]}`,
			id:       id,
			expected: "2025-08-01-preview",
		},
		{
			name:     "type not in template",
			template: `{"resources":{"db":{"type":"Radius.Data/postgreSqlDatabases@2025-08-01-preview"}}}`,
			id:       id,
			expected: defaultDeleteAPIVersion,
		},
		{
			name:     "empty template",
			id:       id,
			expected: defaultDeleteAPIVersion,
		},
		{
			name:     "unparseable id",
			template: `{"resources":{}}`,
			id:       "not-an-id",
			expected: defaultDeleteAPIVersion,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.expected, deleteAPIVersion(tt.template, tt.id))
		})
	}
}
