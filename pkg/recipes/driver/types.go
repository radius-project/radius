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

package driver

import (
	"context"
	"errors"
	"fmt"
	"os/exec"

	"github.com/radius-project/radius/pkg/corerp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/recipes"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
)

const (
	TerraformAzureProvider      = "registry.terraform.io/hashicorp/azurerm"
	TerraformAWSProvider        = "registry.terraform.io/hashicorp/aws"
	TerraformKubernetesProvider = "registry.terraform.io/hashicorp/kubernetes"
)

// Driver is an interface to implement recipe deployment and recipe resources deletion.
type Driver interface {
	// Execute fetches the recipe contents and deploys the recipe and returns deployed resources, secrets and values.
	Execute(ctx context.Context, opts ExecuteOptions) (*recipes.RecipeOutput, error)

	// Delete handles deletion of output resources for the recipe deployment.
	Delete(ctx context.Context, opts DeleteOptions) error

	// Gets the Recipe metadata and parameters from Recipe's template path
	GetRecipeMetadata(ctx context.Context, opts BaseOptions) (map[string]any, error)
}

// BaseOptions is the base options for the driver operations.
type BaseOptions struct {
	// Configuration is the configuration for the recipe.
	Configuration recipes.Configuration

	// Recipe is the recipe metadata.
	Recipe recipes.ResourceMetadata

	// Definition is the environment definition for the recipe.
	Definition recipes.EnvironmentDefinition

	Secrets v20231001preview.SecretStoresClientListSecretsResponse
}

// ExecuteOptions is the options for the Execute method.
type ExecuteOptions struct {
	BaseOptions
	// Previously deployed state of output resource IDs.
	PrevState []string
}

// DeleteOptions is the options for the Delete method.
type DeleteOptions struct {
	BaseOptions

	// OutputResources is the list of output resources for the recipe.
	OutputResources []rpv1.OutputResource
}

func getURLConfigKeyValue(secrets v20231001preview.SecretStoresClientListSecretsResponse, templatePath string) (string, string, error) {
	url, err := recipes.GetGitURL(templatePath)
	if err != nil {
		return "", "", err
	}

	var username, pat *string
	path := "https://"
	user, ok := secrets.Data["username"]
	if ok {
		username = user.Value
		path += fmt.Sprintf("%s:", *username)
	}

	token, ok := secrets.Data["pat"]
	if ok {
		pat = token.Value
		path += *pat
	}

	path += fmt.Sprintf("@%s", url.Hostname())
	return fmt.Sprintf("url.%s.insteadOf", path), url.Hostname(), nil
}
func addSecretsToGitConfig(secrets v20231001preview.SecretStoresClientListSecretsResponse, recipeMetadata *recipes.ResourceMetadata, templatePath string) error {
	urlConfigKey, urlConfigValue, err := getURLConfigKeyValue(secrets, templatePath)
	if err != nil {
		return err
	}
	env, app, resource, err := recipes.GetEnvAppResourceNames(recipeMetadata)
	if err != nil {
		return err
	}
	urlConfigValue = fmt.Sprintf("https://%s-%s-%s-%s", env, app, resource, urlConfigValue)
	cmd := exec.Command("git", "config", "--global", urlConfigKey, urlConfigValue)
	_, err = cmd.Output()
	if err != nil {
		return errors.New("failed to add git config")
	}

	return err
}

func unsetSecretsFromGitConfig(secrets v20231001preview.SecretStoresClientListSecretsResponse, templatePath string) error {
	urlConfigKey, _, err := getURLConfigKeyValue(secrets, templatePath)
	if err != nil {
		return err
	}

	cmd := exec.Command("git", "config", "--global", "--unset", urlConfigKey)
	_, err = cmd.Output()
	if err != nil {
		return errors.New("failed to unset git config")
	}

	return err
}
