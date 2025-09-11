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

package v20231001preview

import (
	"testing"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/to"
	"github.com/stretchr/testify/require"
)

func TestRecipePackConvertVersionedToDataModel(t *testing.T) {
	src := &RecipePackResource{
		ID:       to.Ptr("/subscriptions/test/resourceGroups/testgroup/providers/Applications.Core/recipePacks/testpack"),
		Name:     to.Ptr("testpack"),
		Type:     to.Ptr("Applications.Core/recipePacks"),
		Location: to.Ptr("eastus"),
		Tags: map[string]*string{
			"env": to.Ptr("test"),
		},
		Properties: &RecipePackProperties{
			Description:       to.Ptr("Test recipe pack"),
			ProvisioningState: to.Ptr(ProvisioningStateSucceeded),
			Recipes: map[string]*RecipeDefinition{
				"Applications.Datastores/sqlDatabases": {
					RecipeKind:     to.Ptr(RecipeKindTerraform),
					RecipeLocation: to.Ptr("https://example.com/terraform/sql"),
					Parameters: map[string]any{
						"version": "latest",
					},
				},
				"Applications.Messaging/rabbitMQQueues": {
					RecipeKind:     to.Ptr(RecipeKindBicep),
					RecipeLocation: to.Ptr("br:myregistry.azurecr.io/bicep/rabbitmq:v1"),
				},
			},
		},
	}

	expected := &datamodel.RecipePack{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:   "/subscriptions/test/resourceGroups/testgroup/providers/Applications.Core/recipePacks/testpack",
				Name: "testpack",
				Type: "Applications.Core/recipePacks",
				Tags: map[string]string{
					"env": "test",
				},
				Location: "eastus",
			},
			InternalMetadata: v1.InternalMetadata{
				CreatedAPIVersion:      Version,
				UpdatedAPIVersion:      Version,
				AsyncProvisioningState: v1.ProvisioningStateSucceeded,
			},
		},
		Properties: datamodel.RecipePackProperties{
			Description: to.Ptr("Test recipe pack"),
			Recipes: map[string]datamodel.RecipePackDefinition{
				"Applications.Datastores/sqlDatabases": {
					RecipeKind:     "terraform",
					RecipeLocation: "https://example.com/terraform/sql",
					Parameters: map[string]any{
						"version": "latest",
					},
				},
				"Applications.Messaging/rabbitMQQueues": {
					RecipeKind:     "bicep",
					RecipeLocation: "br:myregistry.azurecr.io/bicep/rabbitmq:v1",
				},
			},
		},
	}

	result, err := src.ConvertTo()
	require.NoError(t, err)
	require.Equal(t, expected, result)
}

func TestRecipePackConvertDataModelToVersioned(t *testing.T) {
	src := &datamodel.RecipePack{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:       "/subscriptions/test/resourceGroups/testgroup/providers/Applications.Core/recipePacks/testpack",
				Name:     "testpack",
				Type:     "Applications.Core/recipePacks",
				Location: "eastus",
				Tags: map[string]string{
					"env": "test",
				},
			},
			InternalMetadata: v1.InternalMetadata{
				AsyncProvisioningState: v1.ProvisioningStateSucceeded,
			},
		},
		Properties: datamodel.RecipePackProperties{
			Description: to.Ptr("Test recipe pack"),
			Recipes: map[string]datamodel.RecipePackDefinition{
				"Applications.Datastores/sqlDatabases": {
					RecipeKind:     "terraform",
					RecipeLocation: "https://example.com/terraform/sql",
					Parameters: map[string]any{
						"version": "latest",
					},
				},
			},
		},
	}

	expected := &RecipePackResource{
		ID:       to.Ptr("/subscriptions/test/resourceGroups/testgroup/providers/Applications.Core/recipePacks/testpack"),
		Name:     to.Ptr("testpack"),
		Type:     to.Ptr("Applications.Core/recipePacks"),
		Location: to.Ptr("eastus"),
		Tags: map[string]*string{
			"env": to.Ptr("test"),
		},
		Properties: &RecipePackProperties{
			Description:       to.Ptr("Test recipe pack"),
			ProvisioningState: to.Ptr(ProvisioningStateSucceeded),
			Recipes: map[string]*RecipeDefinition{
				"Applications.Datastores/sqlDatabases": {
					RecipeKind:     to.Ptr(RecipeKindTerraform),
					RecipeLocation: to.Ptr("https://example.com/terraform/sql"),
					Parameters: map[string]any{
						"version": "latest",
					},
				},
			},
		},
	}

	dst := &RecipePackResource{}
	err := dst.ConvertFrom(src)
	require.NoError(t, err)
	require.Equal(t, expected, dst)
}