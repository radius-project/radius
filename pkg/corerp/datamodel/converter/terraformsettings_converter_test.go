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

func TestTerraformSettingsDataModelToVersioned(t *testing.T) {
	testCases := []struct {
		name        string
		dataModel   *datamodel.TerraformSettings_v20250801preview
		version     string
		expectError bool
	}{
		{
			name: "valid conversion to 2025-08-01-preview",
			dataModel: &datamodel.TerraformSettings_v20250801preview{
				BaseResource: v1.BaseResource{
					TrackedResource: v1.TrackedResource{
						ID:       "/planes/radius/local/resourceGroups/test-rg/providers/Radius.Core/terraformSettings/test-settings",
						Name:     "test-settings",
						Type:     "Radius.Core/terraformSettings",
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
				Properties: datamodel.TerraformSettingsProperties_v20250801preview{
					TerraformRC: &datamodel.TerraformCliConfiguration{
						ProviderInstallation: &datamodel.TerraformProviderInstallationConfiguration{
							NetworkMirror: &datamodel.TerraformNetworkMirrorConfiguration{
								URL:     "https://mirror.example.com/",
								Include: []string{"*"},
							},
						},
					},
					Backend: &datamodel.TerraformBackendConfiguration{
						Type: "kubernetes",
						Config: map[string]any{
							"namespace": "radius-system",
						},
					},
					Env: map[string]string{
						"TF_LOG": "DEBUG",
					},
				},
			},
			version:     v20250801preview.Version,
			expectError: false,
		},
		{
			name: "minimal settings",
			dataModel: &datamodel.TerraformSettings_v20250801preview{
				BaseResource: v1.BaseResource{
					TrackedResource: v1.TrackedResource{
						ID:       "/planes/radius/local/resourceGroups/test-rg/providers/Radius.Core/terraformSettings/minimal",
						Name:     "minimal",
						Type:     "Radius.Core/terraformSettings",
						Location: "global",
					},
				},
				Properties: datamodel.TerraformSettingsProperties_v20250801preview{},
			},
			version:     v20250801preview.Version,
			expectError: false,
		},
		{
			name:        "unsupported version",
			dataModel:   &datamodel.TerraformSettings_v20250801preview{},
			version:     "unsupported-version",
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := TerraformSettingsDataModelToVersioned(tc.dataModel, tc.version)

			if tc.expectError {
				require.Error(t, err)
				require.Equal(t, v1.ErrUnsupportedAPIVersion, err)
				require.Nil(t, result)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
				require.IsType(t, &v20250801preview.TerraformSettingsResource{}, result)

				versionedResource := result.(*v20250801preview.TerraformSettingsResource)
				require.Equal(t, tc.dataModel.ID, to.String(versionedResource.ID))
				require.Equal(t, tc.dataModel.Name, to.String(versionedResource.Name))
				require.Equal(t, tc.dataModel.Type, to.String(versionedResource.Type))
				require.Equal(t, tc.dataModel.Location, to.String(versionedResource.Location))
			}
		})
	}
}

func TestTerraformSettingsDataModelFromVersioned(t *testing.T) {
	testCases := []struct {
		name        string
		content     []byte
		version     string
		expectError bool
		expected    *datamodel.TerraformSettings_v20250801preview
	}{
		{
			name: "valid conversion from 2025-08-01-preview",
			content: []byte(`{
				"id": "/planes/radius/local/resourceGroups/test-rg/providers/Radius.Core/terraformSettings/test-settings",
				"name": "test-settings",
				"type": "Radius.Core/terraformSettings",
				"location": "global",
				"tags": {
					"env": "test"
				},
				"properties": {
					"terraformrc": {
						"providerInstallation": {
							"networkMirror": {
								"url": "https://mirror.example.com/",
								"include": ["*"]
							}
						}
					},
					"backend": {
						"type": "kubernetes",
						"config": {
							"namespace": "radius-system"
						}
					},
					"env": {
						"TF_LOG": "DEBUG"
					}
				}
			}`),
			version:     v20250801preview.Version,
			expectError: false,
			expected: &datamodel.TerraformSettings_v20250801preview{
				BaseResource: v1.BaseResource{
					TrackedResource: v1.TrackedResource{
						ID:       "/planes/radius/local/resourceGroups/test-rg/providers/Radius.Core/terraformSettings/test-settings",
						Name:     "test-settings",
						Type:     "Radius.Core/terraformSettings",
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
				"id": "/planes/radius/local/resourceGroups/test-rg/providers/Radius.Core/terraformSettings/minimal",
				"name": "minimal",
				"type": "Radius.Core/terraformSettings",
				"location": "global",
				"properties": {}
			}`),
			version:     v20250801preview.Version,
			expectError: false,
			expected: &datamodel.TerraformSettings_v20250801preview{
				BaseResource: v1.BaseResource{
					TrackedResource: v1.TrackedResource{
						ID:       "/planes/radius/local/resourceGroups/test-rg/providers/Radius.Core/terraformSettings/minimal",
						Name:     "minimal",
						Type:     "Radius.Core/terraformSettings",
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
			result, err := TerraformSettingsDataModelFromVersioned(tc.content, tc.version)

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

func TestTerraformSettingsRoundTripConversion(t *testing.T) {
	originalDataModel := &datamodel.TerraformSettings_v20250801preview{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:       "/planes/radius/local/resourceGroups/test-rg/providers/Radius.Core/terraformSettings/round-trip",
				Name:     "round-trip",
				Type:     "Radius.Core/terraformSettings",
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
		Properties: datamodel.TerraformSettingsProperties_v20250801preview{
			TerraformRC: &datamodel.TerraformCliConfiguration{
				ProviderInstallation: &datamodel.TerraformProviderInstallationConfiguration{
					NetworkMirror: &datamodel.TerraformNetworkMirrorConfiguration{
						URL:     "https://mirror.example.com/",
						Include: []string{"hashicorp/*"},
						Exclude: []string{"hashicorp/azurerm"},
					},
					Direct: &datamodel.TerraformDirectConfiguration{
						Include: []string{"*"},
					},
				},
				Credentials: map[string]*datamodel.TerraformCredentialConfiguration{
					"app.terraform.io": {
						Token: &datamodel.SecretRef{
							SecretID: "/planes/radius/local/providers/Radius.Security/secrets/tfc-token",
							Key:      "token",
						},
					},
				},
			},
			Backend: &datamodel.TerraformBackendConfiguration{
				Type: "kubernetes",
				Config: map[string]any{
					"namespace":    "radius-system",
					"secretSuffix": "prod-state",
				},
			},
			Env: map[string]string{
				"TF_LOG":                     "DEBUG",
				"TF_REGISTRY_CLIENT_TIMEOUT": "30",
			},
			Logging: &datamodel.TerraformLoggingConfiguration{
				Level: datamodel.TerraformLogLevelDebug,
				Path:  "/var/log/terraform.log",
			},
		},
	}

	// Convert to versioned model
	versionedModel, err := TerraformSettingsDataModelToVersioned(originalDataModel, v20250801preview.Version)
	require.NoError(t, err)
	require.NotNil(t, versionedModel)

	// Serialize to JSON
	jsonBytes, err := json.Marshal(versionedModel)
	require.NoError(t, err)

	// Convert back to datamodel
	resultDataModel, err := TerraformSettingsDataModelFromVersioned(jsonBytes, v20250801preview.Version)
	require.NoError(t, err)
	require.NotNil(t, resultDataModel)

	// Validate round-trip preserved data
	require.Equal(t, originalDataModel.ID, resultDataModel.ID)
	require.Equal(t, originalDataModel.Name, resultDataModel.Name)
	require.Equal(t, originalDataModel.Type, resultDataModel.Type)
	require.Equal(t, originalDataModel.Location, resultDataModel.Location)
	require.Equal(t, originalDataModel.Tags, resultDataModel.Tags)

	// Validate TerraformRC
	require.NotNil(t, resultDataModel.Properties.TerraformRC)
	require.NotNil(t, resultDataModel.Properties.TerraformRC.ProviderInstallation)
	require.NotNil(t, resultDataModel.Properties.TerraformRC.ProviderInstallation.NetworkMirror)
	require.Equal(t, originalDataModel.Properties.TerraformRC.ProviderInstallation.NetworkMirror.URL,
		resultDataModel.Properties.TerraformRC.ProviderInstallation.NetworkMirror.URL)

	// Validate Backend
	require.NotNil(t, resultDataModel.Properties.Backend)
	require.Equal(t, originalDataModel.Properties.Backend.Type, resultDataModel.Properties.Backend.Type)

	// Validate Env
	require.Equal(t, originalDataModel.Properties.Env, resultDataModel.Properties.Env)

	// Validate Logging
	require.NotNil(t, resultDataModel.Properties.Logging)
	require.Equal(t, originalDataModel.Properties.Logging.Level, resultDataModel.Properties.Logging.Level)
	require.Equal(t, originalDataModel.Properties.Logging.Path, resultDataModel.Properties.Logging.Path)
}
