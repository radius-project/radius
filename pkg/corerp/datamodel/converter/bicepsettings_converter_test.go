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

package converter

import (
	"encoding/json"
	"testing"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/corerp/api/v20250801preview"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/to"
	"github.com/stretchr/testify/require"
)

func TestBicepSettingsDataModelToVersioned(t *testing.T) {
	testCases := []struct {
		name        string
		dataModel   *datamodel.BicepSettings_v20250801preview
		version     string
		expectError bool
	}{
		{
			name: "valid conversion to 2025-08-01-preview",
			dataModel: &datamodel.BicepSettings_v20250801preview{
				BaseResource: v1.BaseResource{
					TrackedResource: v1.TrackedResource{
						ID:       "/planes/radius/local/resourceGroups/test-rg/providers/Radius.Core/bicepSettings/test-settings",
						Name:     "test-settings",
						Type:     "Radius.Core/bicepSettings",
						Location: "global",
						Tags: map[string]string{
							"env": "test",
						},
					},
					InternalMetadata: v1.InternalMetadata{
						CreatedAPIVersion:      v20250801preview.Version,
						UpdatedAPIVersion:      v20250801preview.Version,
						AsyncProvisioningState: v1.ProvisioningStateSucceeded,
					},
				},
				Properties: datamodel.BicepSettingsProperties_v20250801preview{
					Authentication: &datamodel.BicepAuthenticationConfiguration{
						Registries: map[string]*datamodel.BicepRegistryAuthentication{
							"myregistry.azurecr.io": {
								Basic: &datamodel.BicepBasicAuthentication{
									Username: "admin",
									Password: &datamodel.SecretRef{
										SecretID: "/planes/radius/local/providers/Radius.Security/secrets/acr-password",
										Key:      "password",
									},
								},
							},
						},
					},
				},
			},
			version:     v20250801preview.Version,
			expectError: false,
		},
		{
			name: "minimal settings",
			dataModel: &datamodel.BicepSettings_v20250801preview{
				BaseResource: v1.BaseResource{
					TrackedResource: v1.TrackedResource{
						ID:       "/planes/radius/local/resourceGroups/test-rg/providers/Radius.Core/bicepSettings/minimal",
						Name:     "minimal",
						Type:     "Radius.Core/bicepSettings",
						Location: "global",
					},
				},
				Properties: datamodel.BicepSettingsProperties_v20250801preview{},
			},
			version:     v20250801preview.Version,
			expectError: false,
		},
		{
			name:        "unsupported version",
			dataModel:   &datamodel.BicepSettings_v20250801preview{},
			version:     "unsupported-version",
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := BicepSettingsDataModelToVersioned(tc.dataModel, tc.version)

			if tc.expectError {
				require.Error(t, err)
				require.Equal(t, v1.ErrUnsupportedAPIVersion, err)
				require.Nil(t, result)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
				require.IsType(t, &v20250801preview.BicepSettingsResource{}, result)

				versionedResource := result.(*v20250801preview.BicepSettingsResource)
				require.Equal(t, tc.dataModel.ID, to.String(versionedResource.ID))
				require.Equal(t, tc.dataModel.Name, to.String(versionedResource.Name))
				require.Equal(t, tc.dataModel.Type, to.String(versionedResource.Type))
				require.Equal(t, tc.dataModel.Location, to.String(versionedResource.Location))
			}
		})
	}
}

func TestBicepSettingsDataModelFromVersioned(t *testing.T) {
	testCases := []struct {
		name        string
		content     []byte
		version     string
		expectError bool
		expected    *datamodel.BicepSettings_v20250801preview
	}{
		{
			name: "valid conversion from 2025-08-01-preview",
			content: []byte(`{
				"id": "/planes/radius/local/resourceGroups/test-rg/providers/Radius.Core/bicepSettings/test-settings",
				"name": "test-settings",
				"type": "Radius.Core/bicepSettings",
				"location": "global",
				"tags": {
					"env": "test"
				},
				"properties": {
					"authentication": {
						"registries": {
							"myregistry.azurecr.io": {
								"basic": {
									"username": "admin",
									"password": {
										"secretId": "/planes/radius/local/providers/Radius.Security/secrets/acr-password",
										"key": "password"
									}
								}
							}
						}
					}
				}
			}`),
			version:     v20250801preview.Version,
			expectError: false,
			expected: &datamodel.BicepSettings_v20250801preview{
				BaseResource: v1.BaseResource{
					TrackedResource: v1.TrackedResource{
						ID:       "/planes/radius/local/resourceGroups/test-rg/providers/Radius.Core/bicepSettings/test-settings",
						Name:     "test-settings",
						Type:     "Radius.Core/bicepSettings",
						Location: "global",
						Tags: map[string]string{
							"env": "test",
						},
					},
				},
			},
		},
		{
			name: "minimal settings",
			content: []byte(`{
				"id": "/planes/radius/local/resourceGroups/test-rg/providers/Radius.Core/bicepSettings/minimal",
				"name": "minimal",
				"type": "Radius.Core/bicepSettings",
				"location": "global",
				"properties": {}
			}`),
			version:     v20250801preview.Version,
			expectError: false,
			expected: &datamodel.BicepSettings_v20250801preview{
				BaseResource: v1.BaseResource{
					TrackedResource: v1.TrackedResource{
						ID:       "/planes/radius/local/resourceGroups/test-rg/providers/Radius.Core/bicepSettings/minimal",
						Name:     "minimal",
						Type:     "Radius.Core/bicepSettings",
						Location: "global",
					},
				},
			},
		},
		{
			name:        "invalid JSON",
			content:     []byte(`{invalid json}`),
			version:     v20250801preview.Version,
			expectError: true,
		},
		{
			name:        "unsupported version",
			content:     []byte(`{}`),
			version:     "unsupported-version",
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := BicepSettingsDataModelFromVersioned(tc.content, tc.version)

			if tc.expectError {
				require.Error(t, err)
				require.Nil(t, result)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
				require.Equal(t, tc.expected.ID, result.ID)
				require.Equal(t, tc.expected.Name, result.Name)
				require.Equal(t, tc.expected.Type, result.Type)
				require.Equal(t, tc.expected.Location, result.Location)
			}
		})
	}
}

func TestBicepSettingsRoundTripConversion(t *testing.T) {
	originalDataModel := &datamodel.BicepSettings_v20250801preview{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:       "/planes/radius/local/resourceGroups/test-rg/providers/Radius.Core/bicepSettings/round-trip",
				Name:     "round-trip",
				Type:     "Radius.Core/bicepSettings",
				Location: "global",
				Tags: map[string]string{
					"purpose": "testing",
				},
			},
			InternalMetadata: v1.InternalMetadata{
				CreatedAPIVersion:      v20250801preview.Version,
				UpdatedAPIVersion:      v20250801preview.Version,
				AsyncProvisioningState: v1.ProvisioningStateSucceeded,
			},
		},
		Properties: datamodel.BicepSettingsProperties_v20250801preview{
			Authentication: &datamodel.BicepAuthenticationConfiguration{
				Registries: map[string]*datamodel.BicepRegistryAuthentication{
					"myregistry.azurecr.io": {
						Basic: &datamodel.BicepBasicAuthentication{
							Username: "admin",
							Password: &datamodel.SecretRef{
								SecretID: "/planes/radius/local/providers/Radius.Security/secrets/acr-password",
								Key:      "password",
							},
						},
					},
					"ghcr.io": {
						AzureWorkloadIdentity: &datamodel.BicepAzureWorkloadIdentityAuthentication{
							ClientID: "00000000-0000-0000-0000-000000000001",
							TenantID: "00000000-0000-0000-0000-000000000002",
							Token: &datamodel.SecretRef{
								SecretID: "/planes/radius/local/providers/Radius.Security/secrets/azure-token",
								Key:      "token",
							},
						},
					},
					"ecr.aws": {
						AwsIrsa: &datamodel.BicepAwsIrsaAuthentication{
							RoleArn: "arn:aws:iam::123456789012:role/my-role",
							Token: &datamodel.SecretRef{
								SecretID: "/planes/radius/local/providers/Radius.Security/secrets/aws-token",
								Key:      "token",
							},
						},
					},
				},
			},
		},
	}

	// Convert to versioned model
	versionedModel, err := BicepSettingsDataModelToVersioned(originalDataModel, v20250801preview.Version)
	require.NoError(t, err)
	require.NotNil(t, versionedModel)

	// Serialize to JSON
	jsonBytes, err := json.Marshal(versionedModel)
	require.NoError(t, err)

	// Convert back to datamodel
	resultDataModel, err := BicepSettingsDataModelFromVersioned(jsonBytes, v20250801preview.Version)
	require.NoError(t, err)
	require.NotNil(t, resultDataModel)

	// Validate round-trip preserved data
	require.Equal(t, originalDataModel.ID, resultDataModel.ID)
	require.Equal(t, originalDataModel.Name, resultDataModel.Name)
	require.Equal(t, originalDataModel.Type, resultDataModel.Type)
	require.Equal(t, originalDataModel.Location, resultDataModel.Location)
	require.Equal(t, originalDataModel.Tags, resultDataModel.Tags)

	// Validate Authentication
	require.NotNil(t, resultDataModel.Properties.Authentication)
	require.NotNil(t, resultDataModel.Properties.Authentication.Registries)
	require.Len(t, resultDataModel.Properties.Authentication.Registries, 3)

	// Validate basic auth
	basicAuth := resultDataModel.Properties.Authentication.Registries["myregistry.azurecr.io"]
	require.NotNil(t, basicAuth)
	require.NotNil(t, basicAuth.Basic)
	require.Equal(t, "admin", basicAuth.Basic.Username)
	require.NotNil(t, basicAuth.Basic.Password)
	require.Equal(t, "/planes/radius/local/providers/Radius.Security/secrets/acr-password", basicAuth.Basic.Password.SecretID)

	// Validate Azure Workload Identity auth
	azureAuth := resultDataModel.Properties.Authentication.Registries["ghcr.io"]
	require.NotNil(t, azureAuth)
	require.NotNil(t, azureAuth.AzureWorkloadIdentity)
	require.Equal(t, "00000000-0000-0000-0000-000000000001", azureAuth.AzureWorkloadIdentity.ClientID)
	require.Equal(t, "00000000-0000-0000-0000-000000000002", azureAuth.AzureWorkloadIdentity.TenantID)

	// Validate AWS IRSA auth
	awsAuth := resultDataModel.Properties.Authentication.Registries["ecr.aws"]
	require.NotNil(t, awsAuth)
	require.NotNil(t, awsAuth.AwsIrsa)
	require.Equal(t, "arn:aws:iam::123456789012:role/my-role", awsAuth.AwsIrsa.RoleArn)
}
