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

func TestBicepSettingsConvertVersionedToDataModel(t *testing.T) {
	versionedResource := &BicepSettingsResource{
		ID:       to.Ptr("/planes/radius/local/resourceGroups/testGroup/providers/Radius.Core/bicepSettings/my-bicep-settings"),
		Name:     to.Ptr("my-bicep-settings"),
		Type:     to.Ptr("Radius.Core/bicepSettings"),
		Location: to.Ptr("global"),
		Tags: map[string]*string{
			"env": to.Ptr("test"),
		},
		Properties: &BicepSettingsProperties{
			ProvisioningState: to.Ptr(ProvisioningStateSucceeded),
			Authentication: &BicepAuthenticationConfiguration{
				Registries: map[string]*BicepRegistryAuthentication{
					"myregistry.azurecr.io": {
						Basic: &BicepBasicAuthentication{
							Username: to.Ptr("admin"),
							Password: &SecretReference{
								SecretID: to.Ptr("/planes/radius/local/providers/Radius.Security/secrets/acr-password"),
								Key:      to.Ptr("password"),
							},
						},
					},
					"ghcr.io": {
						AzureWorkloadIdentity: &BicepAzureWorkloadIdentityAuthentication{
							ClientID: to.Ptr("00000000-0000-0000-0000-000000000001"),
							TenantID: to.Ptr("00000000-0000-0000-0000-000000000002"),
							Token: &SecretReference{
								SecretID: to.Ptr("/planes/radius/local/providers/Radius.Security/secrets/azure-token"),
								Key:      to.Ptr("token"),
							},
						},
					},
					"ecr.aws": {
						AwsIrsa: &BicepAwsIrsaAuthentication{
							RoleArn: to.Ptr("arn:aws:iam::123456789012:role/my-role"),
							Token: &SecretReference{
								SecretID: to.Ptr("/planes/radius/local/providers/Radius.Security/secrets/aws-token"),
								Key:      to.Ptr("token"),
							},
						},
					},
				},
			},
		},
	}

	dm, err := versionedResource.ConvertTo()
	require.NoError(t, err)

	bs := dm.(*datamodel.BicepSettings_v20250801preview)

	require.Equal(t, "/planes/radius/local/resourceGroups/testGroup/providers/Radius.Core/bicepSettings/my-bicep-settings", bs.ID)
	require.Equal(t, "my-bicep-settings", bs.Name)
	require.Equal(t, "Radius.Core/bicepSettings", bs.Type)
	require.Equal(t, "global", bs.Location)
	require.Equal(t, map[string]string{"env": "test"}, bs.Tags)

	// Authentication
	require.NotNil(t, bs.Properties.Authentication)
	require.NotNil(t, bs.Properties.Authentication.Registries)

	// Basic auth
	require.Contains(t, bs.Properties.Authentication.Registries, "myregistry.azurecr.io")
	basicAuth := bs.Properties.Authentication.Registries["myregistry.azurecr.io"].Basic
	require.NotNil(t, basicAuth)
	require.Equal(t, "admin", basicAuth.Username)
	require.NotNil(t, basicAuth.Password)
	require.Equal(t, "/planes/radius/local/providers/Radius.Security/secrets/acr-password", basicAuth.Password.SecretID)
	require.Equal(t, "password", basicAuth.Password.Key)

	// Azure Workload Identity auth
	require.Contains(t, bs.Properties.Authentication.Registries, "ghcr.io")
	azureAuth := bs.Properties.Authentication.Registries["ghcr.io"].AzureWorkloadIdentity
	require.NotNil(t, azureAuth)
	require.Equal(t, "00000000-0000-0000-0000-000000000001", azureAuth.ClientID)
	require.Equal(t, "00000000-0000-0000-0000-000000000002", azureAuth.TenantID)
	require.NotNil(t, azureAuth.Token)
	require.Equal(t, "/planes/radius/local/providers/Radius.Security/secrets/azure-token", azureAuth.Token.SecretID)
	require.Equal(t, "token", azureAuth.Token.Key)

	// AWS IRSA auth
	require.Contains(t, bs.Properties.Authentication.Registries, "ecr.aws")
	awsAuth := bs.Properties.Authentication.Registries["ecr.aws"].AwsIrsa
	require.NotNil(t, awsAuth)
	require.Equal(t, "arn:aws:iam::123456789012:role/my-role", awsAuth.RoleArn)
	require.NotNil(t, awsAuth.Token)
	require.Equal(t, "/planes/radius/local/providers/Radius.Security/secrets/aws-token", awsAuth.Token.SecretID)
	require.Equal(t, "token", awsAuth.Token.Key)
}

func TestBicepSettingsConvertDataModelToVersioned(t *testing.T) {
	dataModelResource := &datamodel.BicepSettings_v20250801preview{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:       "/planes/radius/local/resourceGroups/testGroup/providers/Radius.Core/bicepSettings/test-settings",
				Name:     "test-settings",
				Type:     "Radius.Core/bicepSettings",
				Location: "global",
				Tags: map[string]string{
					"env": "prod",
				},
			},
			InternalMetadata: v1.InternalMetadata{
				CreatedAPIVersion:      Version,
				UpdatedAPIVersion:      Version,
				AsyncProvisioningState: v1.ProvisioningStateSucceeded,
			},
		},
		Properties: datamodel.BicepSettingsProperties_v20250801preview{
			Authentication: &datamodel.BicepAuthenticationConfiguration{
				Registries: map[string]*datamodel.BicepRegistryAuthentication{
					"docker.io": {
						Basic: &datamodel.BicepBasicAuthentication{
							Username: "docker-user",
							Password: &datamodel.SecretRef{
								SecretID: "/planes/radius/local/providers/Radius.Security/secrets/docker-pass",
								Key:      "password",
							},
						},
					},
					"quay.io": {
						AzureWorkloadIdentity: &datamodel.BicepAzureWorkloadIdentityAuthentication{
							ClientID: "client-id-123",
							TenantID: "tenant-id-456",
							Token: &datamodel.SecretRef{
								SecretID: "/planes/radius/local/providers/Radius.Security/secrets/quay-token",
								Key:      "access-token",
							},
						},
					},
				},
			},
		},
	}

	versionedResource := &BicepSettingsResource{}
	err := versionedResource.ConvertFrom(dataModelResource)
	require.NoError(t, err)

	require.Equal(t, to.Ptr("test-settings"), versionedResource.Name)
	require.Equal(t, to.Ptr("Radius.Core/bicepSettings"), versionedResource.Type)
	require.Equal(t, to.Ptr("global"), versionedResource.Location)
	require.Equal(t, map[string]*string{"env": to.Ptr("prod")}, versionedResource.Tags)

	// Authentication
	require.NotNil(t, versionedResource.Properties.Authentication)
	require.NotNil(t, versionedResource.Properties.Authentication.Registries)

	// Basic auth
	require.Contains(t, versionedResource.Properties.Authentication.Registries, "docker.io")
	basicAuth := versionedResource.Properties.Authentication.Registries["docker.io"].Basic
	require.NotNil(t, basicAuth)
	require.Equal(t, to.Ptr("docker-user"), basicAuth.Username)
	require.NotNil(t, basicAuth.Password)
	require.Equal(t, to.Ptr("/planes/radius/local/providers/Radius.Security/secrets/docker-pass"), basicAuth.Password.SecretID)
	require.Equal(t, to.Ptr("password"), basicAuth.Password.Key)

	// Azure Workload Identity auth
	require.Contains(t, versionedResource.Properties.Authentication.Registries, "quay.io")
	azureAuth := versionedResource.Properties.Authentication.Registries["quay.io"].AzureWorkloadIdentity
	require.NotNil(t, azureAuth)
	require.Equal(t, to.Ptr("client-id-123"), azureAuth.ClientID)
	require.Equal(t, to.Ptr("tenant-id-456"), azureAuth.TenantID)
	require.NotNil(t, azureAuth.Token)
	require.Equal(t, to.Ptr("/planes/radius/local/providers/Radius.Security/secrets/quay-token"), azureAuth.Token.SecretID)
	require.Equal(t, to.Ptr("access-token"), azureAuth.Token.Key)
}

func TestBicepSettingsConvertFromInvalidType(t *testing.T) {
	versionedResource := &BicepSettingsResource{}
	err := versionedResource.ConvertFrom(&datamodel.Environment_v20250801preview{})
	require.Error(t, err)
	require.Equal(t, v1.ErrInvalidModelConversion, err)
}
