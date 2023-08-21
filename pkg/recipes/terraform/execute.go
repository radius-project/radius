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
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	install "github.com/hashicorp/hc-install"
	tfjson "github.com/hashicorp/terraform-json"
	"github.com/radius-project/radius/pkg/metrics"
	"github.com/radius-project/radius/pkg/recipes/recipecontext"
	"github.com/radius-project/radius/pkg/recipes/terraform/config"
	"github.com/radius-project/radius/pkg/recipes/terraform/config/backends"
	"github.com/radius-project/radius/pkg/recipes/terraform/config/providers"
	"github.com/radius-project/radius/pkg/sdk"
	ucp_provider "github.com/radius-project/radius/pkg/ucp/secret/provider"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	executionSubDir                = "deploy"
	workingDirFileMode fs.FileMode = 0700
	// Default prefix string added to secret suffix by terraform while creating kubernetes secret.
	// Kubernetes secrets to store terraform state files are named in the format: tfstate-{workspace}-{secret_suffix}. And we always use "default" workspace for terraform operations.
	terraformStateKubernetesPrefix = "tfstate-default-"
)

var (
	// ErrRecipeNameEmpty is the error when the recipe name is empty.
	ErrRecipeNameEmpty = errors.New("recipe name cannot be empty")
)

var _ TerraformExecutor = (*executor)(nil)

// NewExecutor creates a new Executor with the given UCP connection and secret provider, to execute a Terraform recipe.
func NewExecutor(ucpConn sdk.Connection, secretProvider *ucp_provider.SecretProvider, k8sClientSet kubernetes.Interface) *executor {
	return &executor{ucpConn: ucpConn, secretProvider: secretProvider, k8sClientSet: k8sClientSet}
}

type executor struct {
	// ucpConn represents the configuration needed to connect to UCP, required to fetch cloud provider credentials.
	ucpConn sdk.Connection

	// secretProvider is the secret store provider used for managing credentials in UCP.
	secretProvider *ucp_provider.SecretProvider

	// k8sClientSet is the Kubernetes client.
	k8sClientSet kubernetes.Interface
}

// Deploy installs Terraform, creates a working directory, generates a config, and runs Terraform init and
// apply in the working directory, returning an error if any of these steps fail.
func (e *executor) Deploy(ctx context.Context, options Options) (*tfjson.State, error) {
	logger := ucplog.FromContextOrDiscard(ctx)

	// Install Terraform
	i := install.NewInstaller()
	execPath, err := Install(ctx, i, options.RootDir)
	// The terraform zip for installation is downloaded in a location outside of the install directory and is only accessible through the installer.Remove function -
	// stored in latestVersion.pathsToRemove. So this needs to be called for complete cleanup even if the root terraform directory is deleted.
	defer func() {
		if err := i.Remove(ctx); err != nil {
			logger.Info(fmt.Sprintf("Failed to cleanup Terraform installation: %s", err.Error()))
		}
	}()
	if err != nil {
		return nil, err
	}

	// Create Working Directory
	workingDir, err := createWorkingDir(ctx, options.RootDir)
	if err != nil {
		return nil, err
	}

	// Create Terraform config in the working directory
	secretSuffix, err := e.generateConfig(ctx, workingDir, execPath, options)
	if err != nil {
		return nil, err
	}

	// Run TF Init and Apply in the working directory
	state, err := initAndApply(ctx, workingDir, execPath)
	if err != nil {
		return nil, err
	}
	// verifying if kubernetes secret is created as part of terraform init
	// this is valid only for the backend of type kubernetes
	err = verifyKubernetesSecret(ctx, options, e.k8sClientSet, secretSuffix)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("expected kubernetes secret for terraform state is not found : %w", err)
		}
		return nil, fmt.Errorf("error retrieving Kubernetes secret for Terraform state : %w", err)
	}

	return state, nil
}

func createWorkingDir(ctx context.Context, tfDir string) (string, error) {
	logger := ucplog.FromContextOrDiscard(ctx)

	workingDir := filepath.Join(tfDir, executionSubDir)
	logger.Info(fmt.Sprintf("Creating Terraform working directory: %q", workingDir))
	if err := os.MkdirAll(workingDir, workingDirFileMode); err != nil {
		return "", fmt.Errorf("failed to create working directory for terraform execution: %w", err)
	}

	return workingDir, nil
}

// generateConfig generates Terraform configuration with required inputs for the module, providers and backend to be initialized and applied.
func (e *executor) generateConfig(ctx context.Context, workingDir, execPath string, options Options) (string, error) {
	logger := ucplog.FromContextOrDiscard(ctx)

	// Generate Terraform json config in the working directory
	// Use recipe name as a local reference to the module.
	// Modules are downloaded in a subdirectory in the working directory. Name of the module specified in the
	// configuration is used as subdirectory name under .terraform/modules directory.
	// https://developer.hashicorp.com/terraform/tutorials/modules/module-use#understand-how-modules-work
	localModuleName := options.EnvRecipe.Name
	if localModuleName == "" {
		return "", ErrRecipeNameEmpty
	}

	// Create Terraform configuration containing module information with the given recipe parameters.
	tfConfig := config.New(localModuleName, options.EnvRecipe, options.ResourceRecipe)

	// Before downloading the module, Teraform configuration needs to be persisted in the working directory.
	// Terraform Get command uses this config file to download module from the source specified in the config.
	if err := tfConfig.Save(ctx, workingDir); err != nil {
		return "", err
	}

	// Download the Terraform module to the working directory.
	logger.Info(fmt.Sprintf("Downloading Terraform module: %s", options.EnvRecipe.TemplatePath))
	downloadStartTime := time.Now()
	if err := downloadModule(ctx, workingDir, execPath); err != nil {
		return "", err
	}
	metrics.DefaultRecipeEngineMetrics.RecordRecipeDownloadDuration(ctx, downloadStartTime,
		metrics.NewRecipeAttributes(metrics.RecipeEngineOperationDownloadRecipe, options.EnvRecipe.Name,
			options.EnvRecipe, metrics.SuccessfulOperationState))

	// Load the downloaded module to retrieve providers and variables required by the module.
	// This is needed to add the appropriate providers config and populate the value of recipe context variable.
	logger.Info(fmt.Sprintf("Inspecting the downloaded Terraform module: %s", options.EnvRecipe.TemplatePath))
	loadedModule, err := inspectModule(workingDir, localModuleName)
	if err != nil {
		return "", err
	}

	// Generate Terraform providers configuration for required providers and add it to the Terraform configuration.
	logger.Info(fmt.Sprintf("Adding provider config for required providers %+v", loadedModule.RequiredProviders))
	if err := tfConfig.AddProviders(ctx, loadedModule.RequiredProviders, providers.GetSupportedTerraformProviders(e.ucpConn, e.secretProvider),
		options.EnvConfig); err != nil {
		return "", err
	}

	backendConfig, err := tfConfig.AddTerraformBackend(options.ResourceRecipe, backends.NewKubernetesBackend())
	if err != nil {
		return "", err
	}
	// Retrieving the secret_suffix property from backend config to use it to verify secret creation during terraform init.
	// This is only used for the backend of type kubernetes and should be moved inside an if block when we add more backends.
	var secretSuffix string
	if backendDetails, ok := backendConfig[backends.BackendKubernetes]; ok {
		backendMap := backendDetails.(map[string]any)
		if secret, ok := backendMap["secret_suffix"]; ok {
			secretSuffix = secret.(string)
		}
	}

	// Add recipe context parameter to the generated Terraform config's module parameters.
	// This should only be added if the recipe context variable is declared in the downloaded module.
	if loadedModule.ContextVarExists {
		logger.Info("Adding recipe context module result")

		// Create the recipe context object to be passed to the recipe deployment
		recipectx, err := recipecontext.New(options.ResourceRecipe, options.EnvConfig)
		if err != nil {
			return "", err
		}

		if err = tfConfig.AddRecipeContext(ctx, localModuleName, recipectx); err != nil {
			return "", err
		}
	}
	if loadedModule.ResultOutputExists {
		if err = tfConfig.AddOutputs(localModuleName); err != nil {
			return "", err
		}
	}

	// Add more configurations here.

	// Ensure that we need to save the configuration after adding providers and recipecontext.
	if err := tfConfig.Save(ctx, workingDir); err != nil {
		return "", err
	}

	return secretSuffix, nil
}

// initAndApply runs Terraform init and apply in the provided working directory.
func initAndApply(ctx context.Context, workingDir, execPath string) (*tfjson.State, error) {
	logger := ucplog.FromContextOrDiscard(ctx)

	tf, err := NewTerraform(ctx, workingDir, execPath)
	if err != nil {
		return nil, err
	}
	// Initialize Terraform
	logger.Info("Initializing Terraform")

	terraformInitStartTime := time.Now()
	if err := tf.Init(ctx); err != nil {
		return nil, fmt.Errorf("terraform init failure: %w", err)
	}
	metrics.DefaultRecipeEngineMetrics.RecordTerraformInitializationDuration(ctx, terraformInitStartTime, nil)

	// Apply Terraform configuration
	logger.Info("Running Terraform apply")
	if err := tf.Apply(ctx); err != nil {
		return nil, fmt.Errorf("terraform apply failure: %w", err)
	}

	// Load Terraform state to retrieve the outputs
	logger.Info("Fetching Terraform state")
	return tf.Show(ctx)
}

// verifyKubernetesSecret is used to verify if the secret is created as part of terraform backend initialization.
// It takes secret_suffix created during AddBackend as input and looks for the secret in radius-system namespace
func verifyKubernetesSecret(ctx context.Context, options Options, k8s kubernetes.Interface, secretSuffix string) error {
	_, err := k8s.CoreV1().Secrets(backends.RadiusNamespace).Get(ctx, terraformStateKubernetesPrefix+secretSuffix, metav1.GetOptions{})
	if err != nil {
		return err
	}

	return nil
}
