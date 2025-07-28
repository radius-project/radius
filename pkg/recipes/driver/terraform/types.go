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
	"fmt"
	"strings"

	"github.com/radius-project/radius/pkg/recipes"
)

const (
	DefaultTerraformRegistry           = "registry.terraform.io"
	DefaultTerraformAzureProvider      = "hashicorp/azurerm"
	DefaultTerraformAWSProvider        = "hashicorp/aws"
	DefaultTerraformKubernetesProvider = "hashicorp/kubernetes"

	PrivateRegistrySecretKey_Pat      = "pat"
	PrivateRegistrySecretKey_Username = "username"
)

// GetTerraformProviderFullName returns the full provider name including registry
func GetTerraformProviderFullName(registry, provider string) string {
	return fmt.Sprintf("%s/%s", registry, provider)
}

// GetTerraformRegistry returns the configured Terraform provider mirror URL, or the default registry if none is configured.
func GetTerraformRegistry(config recipes.Configuration) string {
	if config.RecipeConfig.Terraform.ProviderMirror != nil && config.RecipeConfig.Terraform.ProviderMirror.Mirror != "" {
		return config.RecipeConfig.Terraform.ProviderMirror.Mirror
	}
	return DefaultTerraformRegistry
}

// GetTerraformProviderName returns the provider name to use, applying any configured provider mappings.
func GetTerraformProviderName(config recipes.Configuration, providerName string) string {
	if config.RecipeConfig.Terraform.ProviderMirror != nil && len(config.RecipeConfig.Terraform.ProviderMirror.ProviderMappings) > 0 {
		if mappedName, exists := config.RecipeConfig.Terraform.ProviderMirror.ProviderMappings[providerName]; exists {
			return mappedName
		}
	}
	return providerName
}

// GetPrivateGitRepoSecretStoreID returns secretstore resource ID associated with git private terraform repository source.
func GetPrivateGitRepoSecretStoreID(envConfig recipes.Configuration, templatePath string) (string, error) {
	if strings.HasPrefix(templatePath, "git::") {
		url, err := GetGitURL(templatePath)
		if err != nil {
			return "", err
		}

		// get the secret store id associated with the git domain of the template path.
		hostname := strings.TrimPrefix(url.Hostname(), "www.")

		// Check for PAT authentication
		if envConfig.RecipeConfig.Terraform.Authentication.Git.PAT != nil {
			if patConfig, exists := envConfig.RecipeConfig.Terraform.Authentication.Git.PAT[hostname]; exists {
				return patConfig.Secret, nil
			}
		}

		// No authentication configured for this hostname
		return "", nil
	}

	return "", nil
}
