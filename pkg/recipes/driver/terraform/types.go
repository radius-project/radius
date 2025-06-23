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

	// SSH authentication secret keys
	SSHSecretKey_PrivateKey            = "privateKey"
	SSHSecretKey_Passphrase            = "passphrase"
	SSHSecretKey_StrictHostKeyChecking = "strictHostKeyChecking"
)

// GetTerraformProviderFullName returns the full provider name including registry
func GetTerraformProviderFullName(registry, provider string) string {
	return fmt.Sprintf("%s/%s", registry, provider)
}

// GetTerraformRegistry returns the registry to use based on configuration
func GetTerraformRegistry(config recipes.Configuration) string {
	if config.RecipeConfig.Terraform.Registry != nil && config.RecipeConfig.Terraform.Registry.Mirror != "" {
		return config.RecipeConfig.Terraform.Registry.Mirror
	}
	return DefaultTerraformRegistry
}

// GetTerraformProviderName returns the provider name to use based on configuration
func GetTerraformProviderName(config recipes.Configuration, defaultProvider, providerName string) string {
	if config.RecipeConfig.Terraform.Registry != nil && config.RecipeConfig.Terraform.Registry.ProviderMappings != nil {
		if mapping, exists := config.RecipeConfig.Terraform.Registry.ProviderMappings[defaultProvider]; exists {
			return mapping
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

		// Check for SSH authentication first
		if envConfig.RecipeConfig.Terraform.Authentication.Git.SSH != nil {
			if sshConfig, exists := envConfig.RecipeConfig.Terraform.Authentication.Git.SSH[hostname]; exists {
				return sshConfig.Secret, nil
			}
		}

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

// GetGitAuthType returns the authentication type for a given hostname
func GetGitAuthType(envConfig recipes.Configuration, hostname string) string {
	// Normalize hostname
	hostname = strings.TrimPrefix(hostname, "www.")

	// Check for SSH authentication first
	if envConfig.RecipeConfig.Terraform.Authentication.Git.SSH != nil {
		if _, exists := envConfig.RecipeConfig.Terraform.Authentication.Git.SSH[hostname]; exists {
			return "ssh"
		}
	}

	// Check for PAT authentication
	if envConfig.RecipeConfig.Terraform.Authentication.Git.PAT != nil {
		if _, exists := envConfig.RecipeConfig.Terraform.Authentication.Git.PAT[hostname]; exists {
			return "pat"
		}
	}

	return "none"
}
