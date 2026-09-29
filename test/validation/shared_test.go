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
package validation

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_ResourcesInDeletionOrder(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		set      *RPResourceSet
		expected []string
	}{
		{
			name:     "nil set yields no resources",
			set:      nil,
			expected: nil,
		},
		{
			name:     "empty set yields no resources",
			set:      &RPResourceSet{},
			expected: nil,
		},
		{
			name: "dependencies are deleted after their dependents",
			set: &RPResourceSet{
				Resources: []RPResource{
					{Name: "pack", Type: CoreRecipePacksResource},
					{Name: "env", Type: CoreEnvironmentsResource},
					{Name: "app", Type: CoreApplicationsResource},
					{Name: "db", Type: DataMySQLDatabasesResource},
				},
			},
			expected: []string{"app", "db", "env", "pack"},
		},
		{
			name: "already ordered set is unchanged",
			set: &RPResourceSet{
				Resources: []RPResource{
					{Name: "app", Type: CoreApplicationsResource},
					{Name: "db", Type: DataMySQLDatabasesResource},
					{Name: "env", Type: CoreEnvironmentsResource},
					{Name: "pack", Type: CoreRecipePacksResource},
				},
			},
			expected: []string{"app", "db", "env", "pack"},
		},
		{
			name: "applications.core types are ordered like their radius.core equivalents",
			set: &RPResourceSet{
				Resources: []RPResource{
					{Name: "env", Type: EnvironmentsResource},
					{Name: "app", Type: ApplicationsResource},
					{Name: "container", Type: ContainersResource},
				},
			},
			expected: []string{"app", "container", "env"},
		},
		{
			name: "declaration order is preserved within a group",
			set: &RPResourceSet{
				Resources: []RPResource{
					{Name: "env", Type: CoreEnvironmentsResource},
					{Name: "second", Type: ComputeContainersResource},
					{Name: "first", Type: DataMySQLDatabasesResource},
				},
			},
			expected: []string{"second", "first", "env"},
		},
		{
			name: "resource types are matched case-insensitively",
			set: &RPResourceSet{
				Resources: []RPResource{
					{Name: "env", Type: "Radius.Core/environments"},
					{Name: "app", Type: "Radius.Core/applications"},
				},
			},
			expected: []string{"app", "env"},
		},
		{
			name: "unknown types are treated as application-scoped resources",
			set: &RPResourceSet{
				Resources: []RPResource{
					{Name: "env", Type: CoreEnvironmentsResource},
					{Name: "usertype", Type: "Test.Resources/userTypeAlphas"},
					{Name: "app", Type: CoreApplicationsResource},
				},
			},
			expected: []string{"app", "usertype", "env"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ordered := ResourcesInDeletionOrder(tt.set)

			names := make([]string, 0, len(ordered))
			for _, resource := range ordered {
				names = append(names, resource.Name)
			}

			if tt.expected == nil {
				require.Empty(t, names)
			} else {
				require.Equal(t, tt.expected, names)
			}
		})
	}
}

func Test_ResourcesInDeletionOrder_DoesNotMutateInput(t *testing.T) {
	t.Parallel()

	set := &RPResourceSet{
		Resources: []RPResource{
			{Name: "pack", Type: CoreRecipePacksResource},
			{Name: "app", Type: CoreApplicationsResource},
		},
	}

	_ = ResourcesInDeletionOrder(set)

	require.Equal(t, "pack", set.Resources[0].Name)
	require.Equal(t, "app", set.Resources[1].Name)
}
