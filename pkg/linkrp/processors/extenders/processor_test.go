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

package extenders

import (
	"context"
	"testing"

	"github.com/radius-project/radius/pkg/linkrp/datamodel"
	"github.com/radius-project/radius/pkg/linkrp/processors"
	"github.com/radius-project/radius/pkg/recipes"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	"github.com/stretchr/testify/require"
)

const (
	extenderResourceID1 = "/planes/aws/aws/accounts/123341234/regions/us-west-2/providers/AWS.S3/Bucket/myBucket"
	extenderResourceID2 = "/planes/aws/aws/accounts/123341234/regions/us-west-2/providers/AWS.S3/Bucket/myBucket2"
	password            = "testpassword"
)

func Test_Process(t *testing.T) {
	processor := Processor{}
	t.Run("success - recipe", func(t *testing.T) {
		resource := &datamodel.Extender{}
		options := processors.Options{
			RecipeOutput: &recipes.RecipeOutput{
				Resources: []string{
					extenderResourceID1,
				},
				Values: map[string]any{
					"bucketName": "myBucket",
					"region":     "westus",
				},
				Secrets: map[string]any{
					"databaseSecret": password,
					"adminSecret":    password,
				},
			},
		}

		err := processor.Process(context.Background(), resource, options)
		require.NoError(t, err)

		require.Equal(t, "myBucket", resource.Properties.AdditionalProperties["bucketName"])
		require.Equal(t, "westus", resource.Properties.AdditionalProperties["region"])
		require.Equal(t, password, resource.Properties.Secrets["databaseSecret"])
		require.Equal(t, password, resource.Properties.Secrets["adminSecret"])

		expectedValues := map[string]any{
			"bucketName": "myBucket",
			"region":     "westus",
		}
		expectedSecrets := map[string]rpv1.SecretValueReference{
			"databaseSecret": {
				Value: password,
			},
			"adminSecret": {
				Value: password,
			},
		}

		expectedOutputResources, err := processors.GetOutputResourcesFromRecipe(options.RecipeOutput)
		require.NoError(t, err)

		require.Equal(t, expectedValues, resource.ComputedValues)
		require.Equal(t, expectedSecrets, resource.SecretValues)
		require.Equal(t, expectedOutputResources, resource.Properties.Status.OutputResources)
	})

	t.Run("success - manual", func(t *testing.T) {
		resource := &datamodel.Extender{
			Properties: datamodel.ExtenderProperties{
				AdditionalProperties: map[string]any{"bucketName": "myBucket"},
				Secrets: map[string]any{
					"databaseSecret": password,
				},
			},
		}
		err := processor.Process(context.Background(), resource, processors.Options{})
		require.NoError(t, err)

		require.Equal(t, "myBucket", resource.Properties.AdditionalProperties["bucketName"])
		require.Equal(t, password, resource.Properties.Secrets["databaseSecret"])

		expectedValues := map[string]any{
			"bucketName": "myBucket",
		}

		expectedSecrets := map[string]rpv1.SecretValueReference{
			"databaseSecret": {
				Value: password,
			},
		}
		require.NoError(t, err)

		require.Equal(t, expectedValues, resource.ComputedValues)
		require.Equal(t, expectedSecrets, resource.SecretValues)
	})

	t.Run("success - recipe with value overrides", func(t *testing.T) {
		resource := &datamodel.Extender{
			Properties: datamodel.ExtenderProperties{
				AdditionalProperties: map[string]any{
					"bucketName": "myBucket",
				},
				Secrets: map[string]any{
					"databaseSecret": password,
				},
			},
		}
		options := processors.Options{
			RecipeOutput: &recipes.RecipeOutput{
				Resources: []string{
					extenderResourceID2,
				},
				// Values and secrets will be overridden by the resource.
				Values: map[string]any{
					"bucketName": "myBucket2",
				},
				Secrets: map[string]any{
					"databaseSecret": "overridepassword",
				},
			},
		}

		err := processor.Process(context.Background(), resource, options)
		require.NoError(t, err)

		require.Equal(t, "myBucket2", resource.Properties.AdditionalProperties["bucketName"])
		require.Equal(t, "overridepassword", resource.Properties.Secrets["databaseSecret"])

		expectedValues := map[string]any{
			"bucketName": "myBucket2",
		}
		expectedSecrets := map[string]rpv1.SecretValueReference{
			"databaseSecret": {
				Value: "overridepassword",
			},
		}

		expectedOutputResources := []rpv1.OutputResource{}

		recipeOutputResources, err := processors.GetOutputResourcesFromRecipe(options.RecipeOutput)
		require.NoError(t, err)
		expectedOutputResources = append(expectedOutputResources, recipeOutputResources...)

		require.Equal(t, expectedValues, resource.ComputedValues)
		require.Equal(t, expectedSecrets, resource.SecretValues)
		require.Equal(t, expectedOutputResources, resource.Properties.Status.OutputResources)
	})

	t.Run("failure - missing required values", func(t *testing.T) {
		resource := &datamodel.Extender{
			Properties: datamodel.ExtenderProperties{
				Secrets: map[string]any{
					"databaseSecret": 24,
				},
			},
		}
		options := processors.Options{RecipeOutput: &recipes.RecipeOutput{}}

		err := processor.Process(context.Background(), resource, options)
		require.Error(t, err)
		require.IsType(t, &processors.ValidationError{}, err)
		require.Equal(t, `secret 'databaseSecret' must be of type string`, err.Error())

	})
}

func Test_MergeOutputValues(t *testing.T) {
	resource := &datamodel.Extender{
		Properties: datamodel.ExtenderProperties{
			AdditionalProperties: map[string]any{"bucketName": "myBucket"},
			Secrets: map[string]any{
				"databaseSecret1": password,
			},
		},
	}
	propertiesMap := mergeOutputValues(resource.Properties.AdditionalProperties, nil, false)
	require.Equal(t, resource.Properties.AdditionalProperties, propertiesMap)
	secretsMap := mergeOutputValues(resource.Properties.Secrets, nil, true)
	require.Equal(t, resource.Properties.Secrets, secretsMap)

	options := processors.Options{
		RecipeOutput: &recipes.RecipeOutput{
			Resources: []string{
				extenderResourceID1,
			},
			Values: map[string]any{
				"region": "westus",
			},
			Secrets: map[string]any{
				"databaseSecret2": password,
				"adminSecret":     password,
			},
		},
	}

	expectedAdditionalProperties := map[string]any{
		"bucketName": "myBucket",
		"region":     "westus",
	}
	expectedSecrets := map[string]any{
		"databaseSecret1": password,
		"databaseSecret2": password,
		"adminSecret":     password,
	}

	propertiesMap = mergeOutputValues(resource.Properties.AdditionalProperties, options.RecipeOutput, false)
	require.Equal(t, expectedAdditionalProperties, propertiesMap)
	secretsMap = mergeOutputValues(resource.Properties.Secrets, options.RecipeOutput, true)
	require.Equal(t, expectedSecrets, secretsMap)
}
