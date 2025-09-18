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
	"strings"

	"github.com/radius-project/radius/pkg/recipes"
)

const (
	TerraformAzureProvider            = "registry.terraform.io/hashicorp/azurerm"
	TerraformAWSProvider              = "registry.terraform.io/hashicorp/aws"
	TerraformKubernetesProvider       = "registry.terraform.io/hashicorp/kubernetes"
	PrivateRegistrySecretKey_Pat      = "pat"
	PrivateRegistrySecretKey_Username = "username"
)

// GetPrivateGitRepoSecretStoreID returns secretstore resource ID associated with git private terraform repository source.
func GetPrivateGitRepoSecretStoreID(envConfig recipes.Configuration, templatePath string) (string, error) {
	if strings.HasPrefix(templatePath, "git::") {
		url, err := GetGitURL(templatePath)
		if err != nil {
			return "", err
		}

		// get the secret store id associated with the git domain of the template path.
		return envConfig.RecipeConfig.Terraform.Authentication.Git.PAT[strings.TrimPrefix(url.Hostname(), "www.")].Secret, nil
	}
	return "", nil
}
