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
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"

	"github.com/hashicorp/terraform-exec/tfexec"
	tfjson "github.com/hashicorp/terraform-json"
	dm "github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
)

const (
	executionSubDir                = "deploy"
	workingDirFileMode fs.FileMode = 0700
)

//go:generate mockgen -typed -destination=./mock_executor.go -package=terraform -self_package github.com/radius-project/radius/pkg/recipes/terraform github.com/radius-project/radius/pkg/recipes/terraform TerraformExecutor
type TerraformExecutor interface {
	// Deploy installs terraform and runs terraform init and apply on the terraform module referenced by the recipe using terraform-exec.
	Deploy(ctx context.Context, options Options) (*tfjson.State, error)

	// Delete installs terraform and runs terraform destroy on the terraform module referenced by the recipe using terraform-exec,
	// and deletes the Kubernetes secret created for terraform state store.
	Delete(ctx context.Context, options Options) error

	// GetRecipeMetadata installs terraform and runs terraform get to retrieve information on the terraform module
	GetRecipeMetadata(ctx context.Context, options Options) (map[string]any, error)
}

// Options represents the options required to build inputs to interact with Terraform.
type Options struct {
	// RootDir is the root directory of where Terraform is installed and executed for a specific recipe deployment/deletion request.
	RootDir string

	// EnvConfig is the kubernetes runtime and cloud provider configuration for the Radius Environment in which the application consuming the terraform recipe will be deployed.
	EnvConfig *recipes.Configuration

	// EnvRecipe is the recipe metadata associated with the Radius Environment in which the application consuming the terraform recipe will be deployed.
	EnvRecipe *recipes.EnvironmentDefinition

	// ResourceRecipe is recipe metadata associated with the Radius resource deploying the Terraform recipe.
	ResourceRecipe *recipes.ResourceMetadata

	// Secrets represents a map of secrets required for recipe execution.
	// The outer map's key represents the secretStoreIDs while
	// while the inner map's key-value pairs represent the [secretKey]secretValue.
	Secrets map[string]map[string]string
}

// NewTerraform creates a working directory for Terraform execution and new Terraform executor with Terraform logs enabled.
func NewTerraform(ctx context.Context, tfRootDir, execPath string) (*tfexec.Terraform, error) {
	workingDir, err := createWorkingDir(ctx, tfRootDir)
	if err != nil {
		return nil, err
	}

	tf, err := tfexec.NewTerraform(workingDir, execPath)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize Terraform: %w", err)
	}

	return tf, nil
}

// createWorkingDir creates a working directory for Terraform execution.
func createWorkingDir(ctx context.Context, tfDir string) (string, error) {
	logger := ucplog.FromContextOrDiscard(ctx)

	workingDir := filepath.Join(tfDir, executionSubDir)
	logger.Info(fmt.Sprintf("Creating Terraform working directory: %q", workingDir))
	if err := os.MkdirAll(workingDir, workingDirFileMode); err != nil {
		return "", fmt.Errorf("failed to create working directory for terraform execution: %w", err)
	}

	return workingDir, nil
}

// GetProviderEnvSecretIDs parses the envConfig to extract secret IDs configured in providers configuration and environment variables
// and returns a map of secret store IDs and corresponding slice of keys.
func GetProviderEnvSecretIDs(envConfig recipes.Configuration) map[string][]string {
	providerSecretIDs := make(map[string][]string)
	var mu sync.Mutex

	// Extract secrets from Terraform providers configuration
	extractProviderSecretIDs(envConfig.RecipeConfig.Terraform.Providers, providerSecretIDs, &mu)

	// Extract secrets from environment variables
	extractEnvSecretIDs(envConfig.RecipeConfig.EnvSecrets, providerSecretIDs, &mu)

	return providerSecretIDs
}

// extractProviderSecrets extracts secrets from Terraform provider configurations
func extractProviderSecretIDs(providers map[string][]dm.ProviderConfigProperties, secrets map[string][]string, mu *sync.Mutex) {
	for _, config := range providers {
		for _, providerConfig := range config {
			if providerConfig.Secrets != nil {
				for _, secret := range providerConfig.Secrets {
					addSecretKeys(secrets, secret.Source, secret.Key, mu)
				}
			}
		}
	}
}

// extractEnvSecrets extracts secrets from environment variable configurations
func extractEnvSecretIDs(envSecrets map[string]dm.SecretReference, secrets map[string][]string, mu *sync.Mutex) {
	for _, config := range envSecrets {
		addSecretKeys(secrets, config.Source, config.Key, mu)
	}
}

// addSecretKeys updates the secrets map with secretStoreID and key, ensuring thread safety with a mutex.
func addSecretKeys(secrets map[string][]string, secretStoreID, key string, mu *sync.Mutex) {
	if secretStoreID == "" || key == "" {
		return
	}

	mu.Lock()
	defer mu.Unlock()

	if _, ok := secrets[secretStoreID]; !ok {
		secrets[secretStoreID] = []string{key}
	} else {
		secrets[secretStoreID] = append(secrets[secretStoreID], key)
	}
}
