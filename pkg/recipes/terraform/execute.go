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
	"time"

	install "github.com/hashicorp/hc-install"
	"github.com/hashicorp/terraform-exec/tfexec"
	tfjson "github.com/hashicorp/terraform-json"
	"github.com/radius-project/radius/pkg/metrics"
	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/recipes/recipecontext"
	"github.com/radius-project/radius/pkg/recipes/terraform/config"
	"github.com/radius-project/radius/pkg/recipes/terraform/config/backends"
	"github.com/radius-project/radius/pkg/recipes/terraform/config/providers"
	"github.com/radius-project/radius/pkg/recipes/util"
	"github.com/radius-project/radius/pkg/sdk"
	ucp_provider "github.com/radius-project/radius/pkg/ucp/secret/provider"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
	"go.opentelemetry.io/otel/attribute"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
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
	tf, err := Install(ctx, i, options.RootDir)
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

	// Create Terraform config in the working directory
	kubernetesBackendSuffix, err := e.generateConfig(ctx, tf, options)
	if err != nil {
		return nil, err
	}

	// Set environment variables for the Terraform process.
	err = e.setEnvironmentVariables(ctx, tf, options.EnvConfig)
	if err != nil {
		return nil, err
	}

	// Run TF Init and Apply in the working directory
	state, err := initAndApply(ctx, tf)
	if err != nil {
		return nil, err
	}

	// Validate that the terraform state file backend source exists.
	// Currently only Kubernetes secret backend is supported, which is created by Terraform as a part of Terraform apply.
	backendExists, err := backends.NewKubernetesBackend(e.k8sClientSet).ValidateBackendExists(ctx, backends.KubernetesBackendNamePrefix+kubernetesBackendSuffix)
	if err != nil {
		return nil, fmt.Errorf("error retrieving kubernetes secret for terraform state: %w", err)
	} else if !backendExists {
		return nil, errors.New("expected kubernetes secret for terraform state is not found")
	}

	return state, nil
}

// Delete installs Terraform, creates a working directory, generates a config, and runs Terraform destroy
// in the working directory, returning an error if any of these steps fail.
func (e *executor) Delete(ctx context.Context, options Options) error {
	logger := ucplog.FromContextOrDiscard(ctx)

	// Install Terraform
	i := install.NewInstaller()
	tf, err := Install(ctx, i, options.RootDir)
	// The terraform zip for installation is downloaded in a location outside of the install directory and is only accessible through the installer.Remove function -
	// stored in latestVersion.pathsToRemove. So this needs to be called for complete cleanup even if the root terraform directory is deleted.
	defer func() {
		if err := i.Remove(ctx); err != nil {
			logger.Info(fmt.Sprintf("Failed to cleanup Terraform installation: %s", err.Error()))
		}
	}()
	if err != nil {
		return err
	}

	// Create Terraform config in the working directory
	kubernetesBackendSuffix, err := e.generateConfig(ctx, tf, options)
	if err != nil {
		return err
	}

	// Before running terraform init and destroy, ensure that the Terraform state file storage source exists.
	// If the state file source has been deleted or wasn't created due to a failure during apply then
	// terraform initialization will fail due to missing backend source.
	backendExists, err := backends.NewKubernetesBackend(e.k8sClientSet).ValidateBackendExists(ctx, backends.KubernetesBackendNamePrefix+kubernetesBackendSuffix)
	if err != nil {
		// Continue with the delete flow for all errors other than backend not found.
		// If it is an intermittent error then the delete flow will fail and should be retried from the client.
		logger.Info(fmt.Sprintf("Error retrieving Terraform state file backend: %s", err.Error()))
	} else if !backendExists {
		// Skip deletion if the backend does not exist. Delete can't be performed without Terraform state file.
		logger.Info("Skipping deletion of recipe resources: Terraform state file backend does not exist.")
		return nil
	}

	// Run TF Destroy in the working directory to delete the resources deployed by the recipe
	err = initAndDestroy(ctx, tf)
	if err != nil {
		return err
	}

	// Delete the kubernetes secret created for terraform state file.
	err = e.k8sClientSet.CoreV1().
		Secrets(backends.RadiusNamespace).
		Delete(ctx, backends.KubernetesBackendNamePrefix+kubernetesBackendSuffix, metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("error deleting kubernetes secret for terraform state: %w", err)
	}

	return nil
}

func (e *executor) GetRecipeMetadata(ctx context.Context, options Options) (map[string]any, error) {
	logger := ucplog.FromContextOrDiscard(ctx)

	// Install Terraform
	i := install.NewInstaller()
	tf, err := Install(ctx, i, options.RootDir)
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

	_, err = getTerraformConfig(ctx, tf.WorkingDir(), options)
	if err != nil {
		return nil, err
	}

	result, err := downloadAndInspect(ctx, tf, options)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"parameters": result.Parameters,
	}, nil
}

// setEnvironmentVariables sets environment variables for the Terraform process by reading values from the environment configuration.
// Terraform process will use environment variables as input for the recipe deployment.
func (e executor) setEnvironmentVariables(ctx context.Context, tf *tfexec.Terraform, envConfig *recipes.Configuration) error {
	if envConfig != nil && envConfig.RecipeConfig.Env.AdditionalProperties != nil {
		envVars := map[string]string{}

		for key, value := range envConfig.RecipeConfig.Env.AdditionalProperties {
			envVars[key] = value
		}

		if err := tf.SetEnv(envVars); err != nil {
			return fmt.Errorf("failed to set environment variables: %w", err)
		}
	}

	return nil
}

// generateConfig generates Terraform configuration with required inputs for the module, providers and backend to be initialized and applied.
func (e *executor) generateConfig(ctx context.Context, tf *tfexec.Terraform, options Options) (string, error) {
	logger := ucplog.FromContextOrDiscard(ctx)
	workingDir := tf.WorkingDir()

	tfConfig, err := getTerraformConfig(ctx, workingDir, options)
	if err != nil {
		return "", err
	}

	loadedModule, err := downloadAndInspect(ctx, tf, options)
	if err != nil {
		return "", err
	}

	// Generate Terraform providers configuration for required providers and add it to the Terraform configuration.
	logger.Info(fmt.Sprintf("Adding provider config for required providers %+v", loadedModule.RequiredProviders))
	if err := tfConfig.AddProviders(ctx, loadedModule.RequiredProviders, providers.GetUCPConfiguredTerraformProviders(e.ucpConn, e.secretProvider),
		options.EnvConfig); err != nil {
		return "", err
	}

	backendConfig, err := tfConfig.AddTerraformBackend(options.ResourceRecipe, backends.NewKubernetesBackend(e.k8sClientSet))
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

		if err = tfConfig.AddRecipeContext(ctx, options.EnvRecipe.Name, recipectx); err != nil {
			return "", err
		}
	}
	if loadedModule.ResultOutputExists {
		if err = tfConfig.AddOutputs(options.EnvRecipe.Name); err != nil {
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

// downloadAndInspect handles downloading the TF module and retrieving the necessary information
func downloadAndInspect(ctx context.Context, tf *tfexec.Terraform, options Options) (*moduleInspectResult, error) {
	logger := ucplog.FromContextOrDiscard(ctx)

	// Download the Terraform module to the working directory.
	logger.Info(fmt.Sprintf("Downloading Terraform module: %s", options.EnvRecipe.TemplatePath))
	downloadStartTime := time.Now()

	if err := downloadModule(ctx, tf, options.EnvRecipe.TemplatePath); err != nil {
		metrics.DefaultRecipeEngineMetrics.RecordRecipeDownloadDuration(ctx, downloadStartTime,
			metrics.NewRecipeAttributes(metrics.RecipeEngineOperationDownloadRecipe, options.EnvRecipe.Name,
				options.EnvRecipe, recipes.RecipeDownloadFailed))
		return nil, recipes.NewRecipeError(recipes.RecipeDownloadFailed, err.Error(), util.RecipeSetupError, recipes.GetErrorDetails(err))
	}
	metrics.DefaultRecipeEngineMetrics.RecordRecipeDownloadDuration(ctx, downloadStartTime,
		metrics.NewRecipeAttributes(metrics.RecipeEngineOperationDownloadRecipe, options.EnvRecipe.Name,
			options.EnvRecipe, metrics.SuccessfulOperationState))

	// Load the downloaded module to retrieve providers and variables required by the module.
	// This is needed to add the appropriate providers config and populate the value of recipe context variable.
	logger.Info(fmt.Sprintf("Inspecting the downloaded Terraform module: %s", options.EnvRecipe.TemplatePath))
	loadedModule, err := inspectModule(tf.WorkingDir(), options.EnvRecipe)
	if err != nil {
		return nil, err
	}

	return loadedModule, nil
}

// getTerraformConfig initializes the Terraform json config with provided module source and saves it
func getTerraformConfig(ctx context.Context, workingDir string, options Options) (*config.TerraformConfig, error) {
	// Generate Terraform json config in the working directory
	// Use recipe name as a local reference to the module.
	// Modules are downloaded in a subdirectory in the working directory. Name of the module specified in the
	// configuration is used as subdirectory name under .terraform/modules directory.
	// https://developer.hashicorp.com/terraform/tutorials/modules/module-use#understand-how-modules-work
	localModuleName := options.EnvRecipe.Name
	if localModuleName == "" {
		return nil, ErrRecipeNameEmpty
	}

	// Create Terraform configuration containing module information with the given recipe parameters.
	tfConfig, err := config.New(ctx, localModuleName, options.EnvRecipe, options.ResourceRecipe, options.EnvConfig)
	if err != nil {
		return nil, err
	}

	// Before downloading the module, Teraform configuration needs to be persisted in the working directory.
	// Terraform Get command uses this config file to download module from the source specified in the config.
	if err := tfConfig.Save(ctx, workingDir); err != nil {
		return nil, err
	}

	return tfConfig, nil
}

// initAndApply runs Terraform init and apply in the provided working directory.
func initAndApply(ctx context.Context, tf *tfexec.Terraform) (*tfjson.State, error) {
	logger := ucplog.FromContextOrDiscard(ctx)

	// Initialize Terraform
	logger.Info("Initializing Terraform")
	terraformInitStartTime := time.Now()
	if err := tf.Init(ctx); err != nil {
		metrics.DefaultRecipeEngineMetrics.RecordTerraformInitializationDuration(ctx, terraformInitStartTime,
			[]attribute.KeyValue{metrics.OperationStateAttrKey.String(metrics.FailedOperationState)})

		return nil, fmt.Errorf("terraform init failure: %w", err)
	}
	metrics.DefaultRecipeEngineMetrics.RecordTerraformInitializationDuration(ctx, terraformInitStartTime,
		[]attribute.KeyValue{metrics.OperationStateAttrKey.String(metrics.SuccessfulOperationState)})

	// Apply Terraform configuration
	logger.Info("Running Terraform apply")
	if err := tf.Apply(ctx); err != nil {
		return nil, fmt.Errorf("terraform apply failure: %w", err)
	}

	// Load Terraform state to retrieve the outputs
	logger.Info("Fetching Terraform state")
	return tf.Show(ctx)
}

// initAndDestroy runs Terraform init and destroy in the provided working directory.
func initAndDestroy(ctx context.Context, tf *tfexec.Terraform) error {
	logger := ucplog.FromContextOrDiscard(ctx)

	// Initialize Terraform
	logger.Info("Initializing Terraform")
	terraformInitStartTime := time.Now()
	if err := tf.Init(ctx); err != nil {
		metrics.DefaultRecipeEngineMetrics.RecordTerraformInitializationDuration(ctx, terraformInitStartTime,
			[]attribute.KeyValue{metrics.OperationStateAttrKey.String(metrics.FailedOperationState)})

		return fmt.Errorf("terraform init failure: %w", err)
	}
	metrics.DefaultRecipeEngineMetrics.RecordTerraformInitializationDuration(ctx, terraformInitStartTime, nil)

	// Destroy Terraform configuration
	logger.Info("Running Terraform destroy")
	if err := tf.Destroy(ctx); err != nil {
		return fmt.Errorf("terraform destroy failure: %w", err)
	}

	return nil
}
