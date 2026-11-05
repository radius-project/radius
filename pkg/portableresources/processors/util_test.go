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

	"github.com/radius-project/radius/pkg/portableresources"
	"github.com/radius-project/radius/pkg/recipes"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/stretchr/testify/require"
)

func Test_GetOutputResourcesFromResourcesField(t *testing.T) {
	resourcesField := []*portableresources.ResourceReference{
		{ID: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/rg/providers/Microsoft.Cache/redis/test-resource1"},
		{ID: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/rg/providers/Microsoft.Cache/redis/test-resource2"},
	}

	expected := []rpv1.OutputResource{
		{
			LocalID:       "",
			ID:            resources.MustParse(resourcesField[0].ID),
			RadiusManaged: new(false),
		},
		{
			LocalID:       "",
			ID:            resources.MustParse(resourcesField[1].ID),
			RadiusManaged: new(false),
		},
	}

	actual, err := GetOutputResourcesFromResourcesField(resourcesField)
	require.NoError(t, err)
	require.Equal(t, expected, actual)
}

func Test_GetOutputResourceFromResourceID_Invalid(t *testing.T) {
	resourcesField := []*portableresources.ResourceReference{
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

	expected := []rpv1.OutputResource{
		{
			LocalID:       "",
			ID:            resources.MustParse(output.Resources[0]),
			RadiusManaged: new(true),
		},
		{
			LocalID:       "",
			ID:            resources.MustParse(output.Resources[1]),
			RadiusManaged: new(true),
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

func Test_GetOutputResourcesFromRecipe_AWS(t *testing.T) {
	tests := []struct {
		name    string
		id      string
		managed bool
	}{
		{
			name:    "read-only AWS type",
			id:      "/planes/aws/aws/accounts/000000000000/regions/us-west-2/providers/AWS.EC2/Image/ami-0123456789abcdef0",
			managed: false,
		},
		{
			name:    "read-only AWS type with different casing",
			id:      "/planes/aws/aws/accounts/000000000000/regions/us-west-2/providers/aws.bedrock/foundationmodel/anthropic.claude-v2",
			managed: false,
		},
		{
			name:    "regular AWS type",
			id:      "/planes/aws/aws/accounts/000000000000/regions/us-west-2/providers/AWS.S3/Bucket/my-bucket",
			managed: true,
		},
		{
			name:    "Kubernetes resource",
			id:      "/planes/kubernetes/local/namespaces/default/providers/core/Service/my-service",
			managed: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual, err := GetOutputResourcesFromRecipe(&recipes.RecipeOutput{Resources: []string{tt.id}})
			require.NoError(t, err)
			require.Equal(t, []rpv1.OutputResource{{ID: resources.MustParse(tt.id), RadiusManaged: new(tt.managed)}}, actual)
		})
	}
}

func Test_GetOutputResourcesFromRecipe_ProviderResourceID(t *testing.T) {
	awsID := "/planes/aws/aws/accounts/123456789012/regions/global/providers/AWS.S3/Bucket/my-bucket"
	azureID := "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/rg/providers/Microsoft.Cache/redis/test-resource1"
	output := recipes.RecipeOutput{Resources: []string{awsID, azureID}}
	output.SetProviderResourceID(awsID, "arn:aws:s3:::my-bucket")

	expected := []rpv1.OutputResource{
		{ID: resources.MustParse(awsID), RadiusManaged: new(true), ProviderResourceID: "arn:aws:s3:::my-bucket"},
		{ID: resources.MustParse(azureID), RadiusManaged: new(true)},
	}

	actual, err := GetOutputResourcesFromRecipe(&output)
	require.NoError(t, err)
	require.Equal(t, expected, actual)
}

func Test_GetOutputResourcesFromRecipe_ProviderResourceIDMatchesExactly(t *testing.T) {
	lowerID := "/planes/aws/aws/accounts/123456789012/regions/us-west-2/providers/AWS.SNS/Topic/orders"
	upperID := "/planes/aws/aws/accounts/123456789012/regions/us-west-2/providers/AWS.SNS/Topic/Orders"
	output := recipes.RecipeOutput{Resources: []string{lowerID, upperID}}
	output.SetProviderResourceID(lowerID, "arn:aws:sns:us-west-2:123456789012:orders")

	actual, err := GetOutputResourcesFromRecipe(&output)
	require.NoError(t, err)
	require.Equal(t, "arn:aws:sns:us-west-2:123456789012:orders", actual[0].ProviderResourceID)
	require.Empty(t, actual[1].ProviderResourceID, "a different resource must not take the ARN of an ID that differs only by case")
}
