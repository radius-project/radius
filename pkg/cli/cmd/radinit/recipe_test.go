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

package radinit

import (
	"fmt"
	"reflect"
	"testing"

	corerp "github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
	"github.com/project-radius/radius/pkg/to"
	"github.com/stretchr/testify/require"
)

func Test_getResourceTypeFromPath(t *testing.T) {
	t.Run("Successfully returns metadata", func(t *testing.T) {
		resourceType := getResourceTypeFromPath("recipes/dev/rediscaches")
		require.Equal(t, "rediscaches", resourceType)
	})

	tests := []struct {
		name     string
		repo     string
		expected string
	}{
		{
			"Path With No Resource Type",
			"randomRepo",
			"",
		},
		{
			"Valid Path",
			"recipes/dev/rediscaches",
			"rediscaches",
		},
		{
			"Invalid Path #1",
			"recipes////dev/rediscaches",
			"",
		},
		{
			"Invalid Path #2",
			"recipes/dev////rediscaches",
			"",
		},
		{
			"Path With Extra Path Argument",
			"recipes/dev/rediscaches/testing",
			"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resourceType := getResourceTypeFromPath(tt.repo)
			require.Equal(t, tt.expected, resourceType)
		})
	}
}

func Test_getLinkType(t *testing.T) {
	tests := []struct {
		name         string
		resourceType string
		want         string
	}{
		{
			"Redis Cache Link Type",
			"rediscaches",
			"Applications.Link/redisCaches",
		},
		{
			"Mongo Database Link Type",
			"mongodatabases",
			"Applications.Link/mongoDatabases",
		},
		{
			"Invalid Link Type",
			"daprstatestores",
			"",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := getLinkType(tt.resourceType); got != tt.want {
				t.Errorf("getLinkType() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_processRepositories(t *testing.T) {
	tests := []struct {
		name  string
		repos []string
		tag   string
		want  map[string]map[string]*corerp.EnvironmentRecipeProperties
	}{
		{
			"Valid Repository with Redis Cache",
			[]string{
				"recipes/dev/rediscaches",
			},
			"0.20",
			map[string]map[string]*corerp.EnvironmentRecipeProperties{
				"Applications.Link/redisCaches": {
					"default": {
						TemplatePath: to.Ptr(fmt.Sprintf("%s/recipes/dev/rediscaches:0.20", DevRecipesRegistry)),
					},
				},
			},
		},
		{
			"Valid Repository with Redis Cache and Mongo Database",
			[]string{
				"recipes/dev/rediscaches",
				"recipes/dev/mongodatabases",
			},
			"0.20",
			map[string]map[string]*corerp.EnvironmentRecipeProperties{
				"Applications.Link/redisCaches": {
					"default": {
						TemplatePath: to.Ptr(fmt.Sprintf("%s/recipes/dev/rediscaches:0.20", DevRecipesRegistry)),
					},
				},
				"Applications.Link/mongoDatabases": {
					"default": {
						TemplatePath: to.Ptr(fmt.Sprintf("%s/recipes/dev/mongodatabases:0.20", DevRecipesRegistry)),
					},
				},
			},
		},
		{
			"Valid Repository with Redis Cache, Mongo Database, and an unsupported type",
			[]string{
				"recipes/dev/rediscaches",
				"recipes/dev/mongodatabases",
				"recipes/dev/unsupported",
			},
			"latest",
			map[string]map[string]*corerp.EnvironmentRecipeProperties{
				"Applications.Link/redisCaches": {
					"default": {
						TemplatePath: to.Ptr(fmt.Sprintf("%s/recipes/dev/rediscaches:latest", DevRecipesRegistry)),
					},
				},
				"Applications.Link/mongoDatabases": {
					"default": {
						TemplatePath: to.Ptr(fmt.Sprintf("%s/recipes/dev/mongodatabases:latest", DevRecipesRegistry)),
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := processRepositories(tt.repos, tt.tag); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("processRepositories() = %v, want %v", got, tt.want)
			}
		})
	}
}
