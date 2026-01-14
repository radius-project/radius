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

func TestTerraformSettingsConvertVersionedToDataModel(t *testing.T) {
	versionedResource := &TerraformSettingsResource{
		ID:       to.Ptr("/planes/radius/local/resourceGroups/testGroup/providers/Radius.Core/terraformSettings/my-tf-settings"),
		Name:     to.Ptr("my-tf-settings"),
		Type:     to.Ptr("Radius.Core/terraformSettings"),
		Location: to.Ptr("global"),
		Tags: map[string]*string{
			"env": to.Ptr("test"),
		},
		Properties: &TerraformSettingsProperties{
			ProvisioningState: to.Ptr(ProvisioningStateSucceeded),
			Terraformrc: &TerraformCliConfiguration{
				ProviderInstallation: &TerraformProviderInstallationConfiguration{
					NetworkMirror: &TerraformNetworkMirrorConfiguration{
						URL:     to.Ptr("https://mirror.corp.example.com/terraform/providers"),
						Include: []*string{to.Ptr("*")},
						Exclude: []*string{to.Ptr("hashicorp/azurerm")},
					},
					Direct: &TerraformDirectConfiguration{
						Exclude: []*string{to.Ptr("hashicorp/azurerm")},
					},
				},
				Credentials: map[string]*TerraformCredentialConfiguration{
					"app.terraform.io": {
						Token: &SecretReference{
							SecretID: to.Ptr("/planes/radius/local/providers/Radius.Security/secrets/tfc-token"),
							Key:      to.Ptr("token"),
						},
					},
				},
			},
			Backend: &TerraformBackendConfiguration{
				Type: to.Ptr("kubernetes"),
				Config: map[string]*string{
					"secretSuffix": to.Ptr("prod-terraform-state"),
					"namespace":    to.Ptr("radius-system"),
				},
			},
			Env: map[string]*string{
				"TF_LOG": to.Ptr("TRACE"),
			},
			Logging: &TerraformLoggingConfiguration{
				Level: to.Ptr(TerraformLogLevelTrace),
				Path:  to.Ptr("/var/log/terraform.log"),
			},
		},
	}

	dm, err := versionedResource.ConvertTo()
	require.NoError(t, err)

	ts := dm.(*datamodel.TerraformSettings_v20250801preview)

	require.Equal(t, "/planes/radius/local/resourceGroups/testGroup/providers/Radius.Core/terraformSettings/my-tf-settings", ts.ID)
	require.Equal(t, "my-tf-settings", ts.Name)
	require.Equal(t, "Radius.Core/terraformSettings", ts.Type)
	require.Equal(t, "global", ts.Location)
	require.Equal(t, map[string]string{"env": "test"}, ts.Tags)

	// TerraformRC
	require.NotNil(t, ts.Properties.TerraformRC)
	require.NotNil(t, ts.Properties.TerraformRC.ProviderInstallation)
	require.NotNil(t, ts.Properties.TerraformRC.ProviderInstallation.NetworkMirror)
	require.Equal(t, "https://mirror.corp.example.com/terraform/providers", ts.Properties.TerraformRC.ProviderInstallation.NetworkMirror.URL)
	require.Equal(t, []string{"*"}, ts.Properties.TerraformRC.ProviderInstallation.NetworkMirror.Include)
	require.Equal(t, []string{"hashicorp/azurerm"}, ts.Properties.TerraformRC.ProviderInstallation.NetworkMirror.Exclude)
	require.NotNil(t, ts.Properties.TerraformRC.ProviderInstallation.Direct)
	require.Equal(t, []string{"hashicorp/azurerm"}, ts.Properties.TerraformRC.ProviderInstallation.Direct.Exclude)

	// Credentials
	require.NotNil(t, ts.Properties.TerraformRC.Credentials)
	require.Contains(t, ts.Properties.TerraformRC.Credentials, "app.terraform.io")
	require.NotNil(t, ts.Properties.TerraformRC.Credentials["app.terraform.io"].Token)
	require.Equal(t, "/planes/radius/local/providers/Radius.Security/secrets/tfc-token", ts.Properties.TerraformRC.Credentials["app.terraform.io"].Token.SecretID)
	require.Equal(t, "token", ts.Properties.TerraformRC.Credentials["app.terraform.io"].Token.Key)

	// Backend
	require.NotNil(t, ts.Properties.Backend)
	require.Equal(t, "kubernetes", ts.Properties.Backend.Type)
	require.Equal(t, "prod-terraform-state", ts.Properties.Backend.Config["secretSuffix"])
	require.Equal(t, "radius-system", ts.Properties.Backend.Config["namespace"])

	// Env
	require.Equal(t, map[string]string{"TF_LOG": "TRACE"}, ts.Properties.Env)

	// Logging
	require.NotNil(t, ts.Properties.Logging)
	require.Equal(t, datamodel.TerraformLogLevelTrace, ts.Properties.Logging.Level)
	require.Equal(t, "/var/log/terraform.log", ts.Properties.Logging.Path)
}

func TestTerraformSettingsConvertDataModelToVersioned(t *testing.T) {
	dataModelResource := &datamodel.TerraformSettings_v20250801preview{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:       "/planes/radius/local/resourceGroups/testGroup/providers/Radius.Core/terraformSettings/test-settings",
				Name:     "test-settings",
				Type:     "Radius.Core/terraformSettings",
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
		Properties: datamodel.TerraformSettingsProperties_v20250801preview{
			TerraformRC: &datamodel.TerraformCliConfiguration{
				ProviderInstallation: &datamodel.TerraformProviderInstallationConfiguration{
					NetworkMirror: &datamodel.TerraformNetworkMirrorConfiguration{
						URL:     "https://mirror.example.com/",
						Include: []string{"hashicorp/*"},
					},
					Direct: &datamodel.TerraformDirectConfiguration{
						Include: []string{"*"},
					},
				},
				Credentials: map[string]*datamodel.TerraformCredentialConfiguration{
					"registry.terraform.io": {
						Token: &datamodel.SecretRef{
							SecretID: "/planes/radius/local/providers/Radius.Security/secrets/registry-token",
							Key:      "api-token",
						},
					},
				},
			},
			Backend: &datamodel.TerraformBackendConfiguration{
				Type: "kubernetes",
				Config: map[string]string{
					"namespace": "terraform",
				},
			},
			Env: map[string]string{
				"TF_LOG":                     "DEBUG",
				"TF_REGISTRY_CLIENT_TIMEOUT": "30",
			},
			Logging: &datamodel.TerraformLoggingConfiguration{
				Level: datamodel.TerraformLogLevelDebug,
				Path:  "/tmp/tf.log",
			},
		},
	}

	versionedResource := &TerraformSettingsResource{}
	err := versionedResource.ConvertFrom(dataModelResource)
	require.NoError(t, err)

	require.Equal(t, to.Ptr("test-settings"), versionedResource.Name)
	require.Equal(t, to.Ptr("Radius.Core/terraformSettings"), versionedResource.Type)
	require.Equal(t, to.Ptr("global"), versionedResource.Location)
	require.Equal(t, map[string]*string{"env": to.Ptr("prod")}, versionedResource.Tags)

	// TerraformRC
	require.NotNil(t, versionedResource.Properties.Terraformrc)
	require.NotNil(t, versionedResource.Properties.Terraformrc.ProviderInstallation)
	require.NotNil(t, versionedResource.Properties.Terraformrc.ProviderInstallation.NetworkMirror)
	require.Equal(t, to.Ptr("https://mirror.example.com/"), versionedResource.Properties.Terraformrc.ProviderInstallation.NetworkMirror.URL)
	require.Equal(t, []*string{to.Ptr("hashicorp/*")}, versionedResource.Properties.Terraformrc.ProviderInstallation.NetworkMirror.Include)

	// Credentials
	require.NotNil(t, versionedResource.Properties.Terraformrc.Credentials)
	require.Contains(t, versionedResource.Properties.Terraformrc.Credentials, "registry.terraform.io")
	require.NotNil(t, versionedResource.Properties.Terraformrc.Credentials["registry.terraform.io"].Token)
	require.Equal(t, to.Ptr("/planes/radius/local/providers/Radius.Security/secrets/registry-token"), versionedResource.Properties.Terraformrc.Credentials["registry.terraform.io"].Token.SecretID)
	require.Equal(t, to.Ptr("api-token"), versionedResource.Properties.Terraformrc.Credentials["registry.terraform.io"].Token.Key)

	// Backend
	require.NotNil(t, versionedResource.Properties.Backend)
	require.Equal(t, to.Ptr("kubernetes"), versionedResource.Properties.Backend.Type)
	require.Equal(t, to.Ptr("terraform"), versionedResource.Properties.Backend.Config["namespace"])

	// Env
	require.Equal(t, map[string]*string{
		"TF_LOG":                     to.Ptr("DEBUG"),
		"TF_REGISTRY_CLIENT_TIMEOUT": to.Ptr("30"),
	}, versionedResource.Properties.Env)

	// Logging
	require.NotNil(t, versionedResource.Properties.Logging)
	require.Equal(t, to.Ptr(TerraformLogLevelDebug), versionedResource.Properties.Logging.Level)
	require.Equal(t, to.Ptr("/tmp/tf.log"), versionedResource.Properties.Logging.Path)
}

func TestTerraformSettingsConvertFromInvalidType(t *testing.T) {
	versionedResource := &TerraformSettingsResource{}
	err := versionedResource.ConvertFrom(&datamodel.Environment_v20250801preview{})
	require.Error(t, err)
	require.Equal(t, v1.ErrInvalidModelConversion, err)
}
