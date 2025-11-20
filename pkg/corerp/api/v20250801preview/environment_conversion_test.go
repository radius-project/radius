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

package v20250801preview

import (
	"testing"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/to"
	"github.com/stretchr/testify/require"
)

func TestEnvironmentConvertVersionedToDataModel(t *testing.T) {
	versionedResource := &EnvironmentResource{
		ID:       to.Ptr("/planes/radius/local/resourceGroups/testGroup/providers/Radius.Core/environments/my-aci-env"),
		Name:     to.Ptr("my-aci-env"),
		Type:     to.Ptr("Radius.Core/environments"),
		Location: to.Ptr("West US"),
		Tags: map[string]*string{
			"env": to.Ptr("test"),
		},
		Properties: &EnvironmentProperties{
			ProvisioningState: to.Ptr(ProvisioningStateSucceeded),
			RecipePacks: []*string{
				to.Ptr("/planes/radius/local/providers/Radius.Core/recipePacks/azure-aci-pack"),
			},
			RecipeParameters: map[string]map[string]any{
				"Radius.Compute/containers": {
					"allowPlatformOptions": false,
				},
			},
			Providers: &Providers{
				Azure: &ProvidersAzure{
					SubscriptionID:    to.Ptr("00000000-0000-0000-0000-000000000000"),
					ResourceGroupName: to.Ptr("my-resource-group"),
					Identity: &IdentitySettings{
						Kind: to.Ptr(IdentitySettingKindUserAssigned),
						ManagedIdentity: []*string{
							to.Ptr("/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/my-rg/providers/Microsoft.ManagedIdentity/userAssignedIdentities/my-identity"),
						},
					},
				},
			},
		},
	}

	dm, err := versionedResource.ConvertTo()
	require.NoError(t, err)

	env := dm.(*datamodel.Environment_v20250801preview)

	require.Equal(t, "/planes/radius/local/resourceGroups/testGroup/providers/Radius.Core/environments/my-aci-env", env.ID)
	require.Equal(t, "my-aci-env", env.Name)
	require.Equal(t, "Radius.Core/environments", env.Type)
	require.Equal(t, "West US", env.Location)
	require.Equal(t, map[string]string{"env": "test"}, env.Tags)
	require.Equal(t, []string{"/planes/radius/local/providers/Radius.Core/recipePacks/azure-aci-pack"}, env.Properties.RecipePacks)
	require.Equal(t, false, env.Properties.Simulated)
	require.NotNil(t, env.Properties.Providers)
	require.NotNil(t, env.Properties.Providers.Azure)
	require.Equal(t, "00000000-0000-0000-0000-000000000000", env.Properties.Providers.Azure.SubscriptionId)
	require.Equal(t, "my-resource-group", env.Properties.Providers.Azure.ResourceGroupName)
	require.NotNil(t, env.Properties.RecipeParameters)
	require.Len(t, env.Properties.RecipeParameters, 1)
	containerParams, ok := env.Properties.RecipeParameters["Radius.Compute/containers"]
	require.True(t, ok)
	require.Equal(t, false, containerParams["allowPlatformOptions"])
}

func TestEnvironmentConvertDataModelToVersioned(t *testing.T) {
	dataModelResource := &datamodel.Environment_v20250801preview{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:       "/planes/radius/local/resourceGroups/testGroup/providers/Radius.Core/environments/test-env",
				Name:     "test-env",
				Type:     "Radius.Core/environments",
				Location: "West US",
				Tags: map[string]string{
					"env": "test",
				},
			},
			InternalMetadata: v1.InternalMetadata{
				CreatedAPIVersion:      Version,
				UpdatedAPIVersion:      Version,
				AsyncProvisioningState: v1.ProvisioningStateSucceeded,
			},
		},
		Properties: datamodel.EnvironmentProperties_v20250801preview{
			RecipePacks: []string{"/planes/radius/local/providers/Radius.Core/recipePacks/test-pack"},
			RecipeParameters: map[string]map[string]any{
				"Radius.Compute/containers": {
					"allowPlatformOptions": true,
				},
			},
			Providers: &datamodel.Providers_v20250801preview{
				Kubernetes: &datamodel.ProvidersKubernetes_v20250801preview{
					Namespace: "default",
				},
			},
			Simulated: false,
		},
	}

	versionedResource := &EnvironmentResource{}
	err := versionedResource.ConvertFrom(dataModelResource)
	require.NoError(t, err)

	require.Equal(t, to.Ptr("test-env"), versionedResource.Name)
	require.Equal(t, to.Ptr("Radius.Core/environments"), versionedResource.Type)
	require.Equal(t, to.Ptr("West US"), versionedResource.Location)
	require.Equal(t, map[string]*string{"env": to.Ptr("test")}, versionedResource.Tags)
	require.Equal(t, []*string{to.Ptr("/planes/radius/local/providers/Radius.Core/recipePacks/test-pack")}, versionedResource.Properties.RecipePacks)
	require.NotNil(t, versionedResource.Properties.Providers)
	require.NotNil(t, versionedResource.Properties.Providers.Kubernetes)
	require.Equal(t, to.Ptr("default"), versionedResource.Properties.Providers.Kubernetes.Namespace)
	require.NotNil(t, versionedResource.Properties.RecipeParameters)
	require.Len(t, versionedResource.Properties.RecipeParameters, 1)
	containerParams, ok := versionedResource.Properties.RecipeParameters["Radius.Compute/containers"]
	require.True(t, ok)
	require.Equal(t, true, containerParams["allowPlatformOptions"])
}
