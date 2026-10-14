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

package terraform

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	install "github.com/hashicorp/hc-install"
	"github.com/radius-project/radius/pkg/components/kubernetesclient/kubernetesclientprovider"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/recipes/terraform/config/backends"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/kubernetes/fake"
)

const (
	// aliasedProviderModule declares an aliased provider with configuration_aliases.
	// Terraform rejects this module during "terraform get" if the module block does not pass azurerm.custom.
	aliasedProviderModule = `terraform {
  required_providers {
    azurerm = {
      source                = "hashicorp/azurerm"
      configuration_aliases = [azurerm.custom]
    }
  }
}

resource "azurerm_resource_group" "test" {
  provider = azurerm.custom
  name     = "test"
  location = "westus3"
}
`

	// defaultProviderModule uses only the default provider configuration.
	defaultProviderModule = `terraform {
  required_providers {
    azurerm = {
      source = "hashicorp/azurerm"
    }
  }
}

resource "azurerm_resource_group" "test" {
  name     = "test"
  location = "westus3"
}
`

	// partnerProviderModule declares an aliased provider that is not in the "hashicorp" namespace.
	// The alias placeholder must use the source from the module, or Terraform rejects this module
	// with a provider type mismatch.
	partnerProviderModule = `terraform {
  required_providers {
    postgresql = {
      source                = "cyrilgdn/postgresql"
      configuration_aliases = [postgresql.pgdb-test]
    }
  }
}

resource "postgresql_database" "test" {
  provider = postgresql.pgdb-test
  name     = "test"
}
`

	// partnerDefaultProviderModule uses only the default configuration of a provider that is not in the
	// "hashicorp" namespace. The environment has an aliased configuration for this provider. Radius must not
	// add an alias placeholder, because a placeholder without a source causes a provider type mismatch.
	partnerDefaultProviderModule = `terraform {
  required_providers {
    kubectl = {
      source = "alekc/kubectl"
    }
  }
}

resource "kubectl_manifest" "test" {
  yaml_body = "apiVersion: v1\nkind: Namespace\nmetadata:\n  name: test\n"
}
`

	// childOwnedAliasModule configures its own aliased provider. The environment has an aliased configuration
	// with the same name. Radius must not pass the alias to the module, because Terraform does not let a
	// module block override a provider configuration that the module defines.
	childOwnedAliasModule = `terraform {
  required_providers {
    aws = {
      source = "hashicorp/aws"
    }
  }
}

provider "aws" {
  alias  = "custom"
  region = "us-east-1"
}

resource "aws_s3_bucket" "test" {
  provider = aws.custom
  bucket   = "test"
}
`
)

// TestProviderConfigurationAliasesWithTerraform runs a real Terraform binary to download modules
// with and without provider configuration_aliases. The other unit tests use a fake Terraform binary,
// which does not validate the provider requirements of the module.
// See https://github.com/radius-project/radius/issues/13299.
func TestProviderConfigurationAliasesWithTerraform(t *testing.T) {
	// Skip this test in short mode as it requires downloading Terraform
	if testing.Short() {
		t.Skip("Skipping test that downloads Terraform in short mode")
	}

	t.Setenv("TERRAFORM_TEST_GLOBAL_DIR", t.TempDir())
	resetGlobalStateForTesting()
	t.Cleanup(resetGlobalStateForTesting)

	modules := map[string][]byte{
		"/aliased.zip":         zipModule(t, aliasedProviderModule),
		"/default.zip":         zipModule(t, defaultProviderModule),
		"/partner.zip":         zipModule(t, partnerProviderModule),
		"/partner-default.zip": zipModule(t, partnerDefaultProviderModule),
		"/child-owned.zip":     zipModule(t, childOwnedAliasModule),
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		module, ok := modules[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/zip")
		_, _ = w.Write(module)
	}))
	t.Cleanup(server.Close)

	envConfig := &recipes.Configuration{
		RecipeConfig: datamodel.RecipeConfigProperties{
			Terraform: datamodel.TerraformConfigProperties{
				Providers: map[string][]datamodel.ProviderConfigProperties{
					"azurerm": {
						{AdditionalProperties: map[string]any{"features": map[string]any{}, "subscription_id": "00000000-0000-0000-0000-000000000000"}},
						{AdditionalProperties: map[string]any{"alias": "custom", "features": map[string]any{}, "subscription_id": "11111111-1111-1111-1111-111111111111"}},
					},
					"postgresql": {
						{AdditionalProperties: map[string]any{"alias": "pgdb-test", "host": "localhost"}},
					},
					"kubectl": {
						{AdditionalProperties: map[string]any{"alias": "custom", "load_config_file": false}},
					},
					"aws": {
						{AdditionalProperties: map[string]any{"alias": "custom", "region": "us-west-2"}},
					},
				},
			},
		},
	}

	// Radius adds the provider configurations from the environment for all providers in the environment.
	expectedProviderConfigs := map[string][]map[string]any{
		"azurerm": {
			{"features": map[string]any{}, "subscription_id": "00000000-0000-0000-0000-000000000000"},
			{"alias": "custom", "features": map[string]any{}, "subscription_id": "11111111-1111-1111-1111-111111111111"},
		},
		"postgresql": {{"alias": "pgdb-test", "host": "localhost"}},
		"kubectl":    {{"alias": "custom", "load_config_file": false}},
		"aws":        {{"alias": "custom", "region": "us-west-2"}},
	}

	tests := []struct {
		name              string
		recipeName        string
		templatePath      string
		envConfig         *recipes.Configuration
		expectedErr       string
		expectedSources   map[string]string
		expectedProviders map[string]any
	}{
		{
			name:              "module declares configuration_aliases",
			recipeName:        "test-recipe",
			templatePath:      server.URL + "/aliased.zip",
			envConfig:         envConfig,
			expectedSources:   map[string]string{"azurerm": "hashicorp/azurerm"},
			expectedProviders: map[string]any{"azurerm.custom": "azurerm.custom"},
		},
		{
			name:            "module uses only the default provider",
			recipeName:      "test-recipe",
			templatePath:    server.URL + "/default.zip",
			envConfig:       envConfig,
			expectedSources: map[string]string{"azurerm": "hashicorp/azurerm"},
		},
		{
			name:              "module declares configuration_aliases for a provider outside the hashicorp namespace",
			recipeName:        "test-recipe",
			templatePath:      server.URL + "/partner.zip",
			envConfig:         envConfig,
			expectedSources:   map[string]string{"postgresql": "cyrilgdn/postgresql"},
			expectedProviders: map[string]any{"postgresql.pgdb-test": "postgresql.pgdb-test"},
		},
		{
			name:            "module uses only the default provider outside the hashicorp namespace",
			recipeName:      "test-recipe",
			templatePath:    server.URL + "/partner-default.zip",
			envConfig:       envConfig,
			expectedSources: map[string]string{"kubectl": "alekc/kubectl"},
		},
		{
			name:            "module configures its own aliased provider",
			recipeName:      "test-recipe",
			templatePath:    server.URL + "/child-owned.zip",
			envConfig:       envConfig,
			expectedSources: map[string]string{"aws": "hashicorp/aws"},
		},
		{
			// This case shows that the test detects the bug: without a matching alias in the
			// environment configuration, Terraform rejects the module during "terraform get".
			name:         "module declares configuration_aliases without environment alias",
			recipeName:   "test-recipe",
			templatePath: server.URL + "/aliased.zip",
			envConfig:    &recipes.Configuration{},
			expectedErr:  "Missing required provider configuration",
		},
		{
			name:         "empty recipe name",
			templatePath: server.URL + "/default.zip",
			envConfig:    envConfig,
			expectedErr:  ErrRecipeNameEmpty.Error(),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			options := Options{
				RootDir:   t.TempDir(),
				EnvConfig: tc.envConfig,
				EnvRecipe: &recipes.EnvironmentDefinition{
					Name:         tc.recipeName,
					TemplatePath: tc.templatePath,
					ResourceType: "Test.Resources/widgets",
				},
				ResourceRecipe: &recipes.ResourceMetadata{
					Name:          "widget",
					ResourceID:    "/planes/radius/local/resourceGroups/test-rg/providers/Test.Resources/widgets/widget",
					EnvironmentID: "/planes/radius/local/resourceGroups/test-rg/providers/Applications.Core/environments/test-env",
				},
			}

			kubernetesClients := kubernetesclientprovider.KubernetesClientProvider{}
			kubernetesClients.SetClientGoClient(fake.NewSimpleClientset())
			e := NewExecutor(nil, nil, kubernetesClients)

			// GetRecipeMetadata downloads the module with "terraform get".
			metadata, err := e.GetRecipeMetadata(t.Context(), options)
			if tc.expectedErr != "" {
				require.ErrorContains(t, err, tc.expectedErr)
				return
			}
			require.NoError(t, err)
			require.Contains(t, metadata, "parameters")

			// generateConfig is used by deploy and delete. It downloads the module with "terraform get"
			// and then adds the full provider configuration.
			tf, err := Install(t.Context(), install.NewInstaller(), InstallOptions{RootDir: options.RootDir})
			require.NoError(t, err)
			_, err = e.generateConfig(t.Context(), tf, options, backends.CloudBackendAuth{}, requireValidOutputMappings)
			require.NoError(t, err)

			content, err := os.ReadFile(filepath.Join(tf.WorkingDir(), "main.tf.json"))
			require.NoError(t, err)
			var generated struct {
				Terraform struct {
					RequiredProviders map[string]struct {
						Source string `json:"source"`
					} `json:"required_providers"`
				} `json:"terraform"`
				Provider map[string][]map[string]any `json:"provider"`
				Module   map[string]map[string]any   `json:"module"`
			}
			require.NoError(t, json.Unmarshal(content, &generated))

			// The placeholders are replaced with the full provider configuration from the environment.
			require.Equal(t, expectedProviderConfigs, generated.Provider)

			sources := map[string]string{}
			for name, requiredProvider := range generated.Terraform.RequiredProviders {
				sources[name] = requiredProvider.Source
			}
			require.Equal(t, tc.expectedSources, sources)

			moduleProviders, ok := generated.Module["test-recipe"]["providers"]
			if tc.expectedProviders == nil {
				require.False(t, ok, "module providers map must be removed when the module does not declare the alias")
				return
			}
			require.Equal(t, tc.expectedProviders, moduleProviders)
		})
	}
}

// zipModule returns a zip archive that contains main.tf with the given content.
func zipModule(t *testing.T, content string) []byte {
	buf := &bytes.Buffer{}
	writer := zip.NewWriter(buf)
	file, err := writer.Create("main.tf")
	require.NoError(t, err)
	_, err = file.Write([]byte(content))
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	return buf.Bytes()
}
