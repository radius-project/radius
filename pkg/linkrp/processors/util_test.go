// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package processors

import (
	"testing"

	"github.com/project-radius/radius/pkg/recipes"
	"github.com/project-radius/radius/pkg/resourcemodel"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
	"github.com/project-radius/radius/pkg/to"
	"github.com/stretchr/testify/require"
)

func Test_GetOutputResourceFromResourceID(t *testing.T) {
	id := "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/rg/providers/Microsoft.Cache/redis/test-resource1"

	redisType := resourcemodel.ResourceType{
		Type:     "Microsoft.Cache/redis",
		Provider: resourcemodel.ProviderAzure,
	}

	expected := rpv1.OutputResource{
		LocalID:       "Resource0",
		ResourceType:  redisType,
		Identity:      resourcemodel.NewARMIdentity(&redisType, id, "unknown"),
		RadiusManaged: to.Ptr(false),
	}

	actual, err := GetOutputResourceFromResourceID(id)
	require.NoError(t, err)
	require.Equal(t, expected, actual)
}

func Test_GetOutputResourceFromResourceID_Invalid(t *testing.T) {
	id := "/////asdf////"

	actual, err := GetOutputResourceFromResourceID(id)
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
