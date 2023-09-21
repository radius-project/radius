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
	reflect "reflect"
	"testing"

	corerp "github.com/radius-project/radius/pkg/corerp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/to"
	"github.com/stretchr/testify/require"
)

func Test_getResourceTypeFromPath(t *testing.T) {
	t.Run("Successfully returns metadata", func(t *testing.T) {
		resourceType := getResourceTypeFromPath("recipes/local-dev/rediscaches")
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
			"recipes/local-dev/rediscaches",
			"rediscaches",
		},
		{
			"Invalid Path #1",
			"recipes////local-dev/rediscaches",
			"",
		},
		{
			"Invalid Path #2",
			"recipes/local-dev////rediscaches",
			"",
		},
		{
			"Path With Extra Path Argument",
			"recipes/local-dev/rediscaches/testing",
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

func Test_getPortableResourceType(t *testing.T) {
	tests := []struct {
		name         string
		resourceType string
		want         string
	}{
		{
			"Dapr PubSub Portable Resource",
			"pubsubbrokers",
			"Applications.Dapr/pubSubBrokers",
		},
		{
			"Dapr Secret Store Portable Resource",
			"secretstores",
			"Applications.Dapr/secretStores",
		},
		{
			"Dapr State Store Portable Resource",
			"statestores",
			"Applications.Dapr/stateStores",
		},
		{
			"Rabbit MQ Portable Resource",
			"rabbitmqqueues",
			"Applications.Messaging/rabbitMQQueues",
		},
		{
			"Redis Cache Portable Resource",
			"rediscaches",
			"Applications.Datastores/redisCaches",
		},
		{
			"Mongo Database Portable Resource",
			"mongodatabases",
			"Applications.Datastores/mongoDatabases",
		},
		{
			"SQL Database Portable Resource",
			"sqldatabases",
			"Applications.Datastores/sqlDatabases",
		},
		{
			"Invalid Portable Resource",
			"unsupported",
			"",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := getPortableResourceType(tt.resourceType); got != tt.want {
				t.Errorf("getPortableResourceType() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_processRepositories(t *testing.T) {
	tests := []struct {
		name  string
		repos []string
		tag   string
		want  map[string]map[string]corerp.RecipePropertiesClassification
	}{
		{
			"Valid Repository with Redis Cache",
			[]string{
				"recipes/local-dev/rediscaches",
			},
			"0.20",
			map[string]map[string]corerp.RecipePropertiesClassification{
				"Applications.Datastores/redisCaches": {
					"default": &corerp.BicepRecipeProperties{
						TemplateKind: to.Ptr(recipes.TemplateKindBicep),
						TemplatePath: to.Ptr(fmt.Sprintf("%s/recipes/local-dev/rediscaches:0.20", DevRecipesRegistry)),
					},
				},
			},
		},
		{
			"Valid Repository with Redis Cache and Mongo Database",
			[]string{
				"recipes/local-dev/rediscaches",
				"recipes/local-dev/mongodatabases",
			},
			"0.20",
			map[string]map[string]corerp.RecipePropertiesClassification{
				"Applications.Datastores/redisCaches": {
					"default": &corerp.BicepRecipeProperties{
						TemplateKind: to.Ptr(recipes.TemplateKindBicep),
						TemplatePath: to.Ptr(fmt.Sprintf("%s/recipes/local-dev/rediscaches:0.20", DevRecipesRegistry)),
					},
				},
				"Applications.Datastores/mongoDatabases": {
					"default": &corerp.BicepRecipeProperties{
						TemplateKind: to.Ptr(recipes.TemplateKindBicep),
						TemplatePath: to.Ptr(fmt.Sprintf("%s/recipes/local-dev/mongodatabases:0.20", DevRecipesRegistry)),
					},
				},
			},
		},
		{
			"Valid Repository with Redis Cache, Mongo Database, and an unsupported type",
			[]string{
				"recipes/local-dev/rediscaches",
				"recipes/local-dev/mongodatabases",
				"recipes/local-dev/unsupported",
				"recipes/unsupported/rediscaches",
				"recipes/unsupported/unsupported",
			},
			"latest",
			map[string]map[string]corerp.RecipePropertiesClassification{
				"Applications.Datastores/redisCaches": {
					"default": &corerp.BicepRecipeProperties{
						TemplateKind: to.Ptr(recipes.TemplateKindBicep),
						TemplatePath: to.Ptr(fmt.Sprintf("%s/recipes/local-dev/rediscaches:latest", DevRecipesRegistry)),
					},
				},
				"Applications.Datastores/mongoDatabases": {
					"default": &corerp.BicepRecipeProperties{
						TemplateKind: to.Ptr(recipes.TemplateKindBicep),
						TemplatePath: to.Ptr(fmt.Sprintf("%s/recipes/local-dev/mongodatabases:latest", DevRecipesRegistry)),
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
