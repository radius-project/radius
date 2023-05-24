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

package processors

import (
	"testing"

	"github.com/project-radius/radius/pkg/linkrp"
	"github.com/project-radius/radius/pkg/recipes"
	"github.com/project-radius/radius/pkg/resourcemodel"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
	"github.com/project-radius/radius/pkg/to"
	"github.com/stretchr/testify/require"
)

func Test_GetOutputResourcesFromResourcesField(t *testing.T) {
	resourcesField := []*linkrp.ResourceReference{
		{ID: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/rg/providers/Microsoft.Cache/redis/test-resource1"},
		{ID: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/rg/providers/Microsoft.Cache/redis/test-resource2"},
	}

	redisType := resourcemodel.ResourceType{
		Type:     "Microsoft.Cache/redis",
		Provider: resourcemodel.ProviderAzure,
	}

	expected := []rpv1.OutputResource{
		{
			LocalID:       "Resource0",
			ResourceType:  redisType,
			Identity:      resourcemodel.NewARMIdentity(&redisType, resourcesField[0].ID, "unknown"),
			RadiusManaged: to.Ptr(false),
		},
		{
			LocalID:       "Resource1",
			ResourceType:  redisType,
			Identity:      resourcemodel.NewARMIdentity(&redisType, resourcesField[1].ID, "unknown"),
			RadiusManaged: to.Ptr(false),
		},
	}

	actual, err := GetOutputResourcesFromResourcesField(resourcesField)
	require.NoError(t, err)
	require.Equal(t, expected, actual)
}

func Test_GetOutputResourceFromResourceID_Invalid(t *testing.T) {
	resourcesField := []*linkrp.ResourceReference{
		{ID: "/////asdf////"},
	}

	actual, err := GetOutputResourcesFromResourcesField(resourcesField)
	require.Error(t, err)
	require.Empty(t, actual)
	require.IsType(t, &ValidationError{}, err)
	require.Equal(t, "resource id \"/////asdf////\" is invalid", err.Error())
}

func Test_GetOutputResourcesFromRecipe(t *testing.T) {
	output := recipes.RecipeOutput{
		Resources: []string{
			"/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/rg/providers/Microsoft.Cache/redis/test-resource1",
			"/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/rg/providers/Microsoft.Cache/redis/test-resource2",
		},
	}

	redisType := resourcemodel.ResourceType{
		Type:     "Microsoft.Cache/redis",
		Provider: resourcemodel.ProviderAzure,
	}

	expected := []rpv1.OutputResource{
		{
			LocalID:       "RecipeResource0",
			ResourceType:  redisType,
			Identity:      resourcemodel.NewARMIdentity(&redisType, output.Resources[0], "unknown"),
			RadiusManaged: to.Ptr(true),
		},
		{
			LocalID:       "RecipeResource1",
			ResourceType:  redisType,
			Identity:      resourcemodel.NewARMIdentity(&redisType, output.Resources[1], "unknown"),
			RadiusManaged: to.Ptr(true),
		},
	}

	actual, err := GetOutputResourcesFromRecipe(&output)
	require.NoError(t, err)
	require.Equal(t, expected, actual)
}

func Test_GetOutputResourcesFromRecipe_Invalid(t *testing.T) {
	output := recipes.RecipeOutput{
		Resources: []string{
			"/////asdf////",
		},
	}

	actual, err := GetOutputResourcesFromRecipe(&output)
	require.Error(t, err)
	require.Empty(t, actual)
	require.IsType(t, &ValidationError{}, err)
	require.Equal(t, "resource id \"/////asdf////\" returned by recipe is invalid", err.Error())
}
