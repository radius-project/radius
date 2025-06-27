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
	"os"
	"path/filepath"
	"reflect"
	"strings"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/components/kubernetesclient/kubernetesclientprovider"
	"github.com/radius-project/radius/pkg/components/secret/secretprovider"
	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/recipes/driver"
	"github.com/radius-project/radius/pkg/recipes/terraform"
	recipes_util "github.com/radius-project/radius/pkg/recipes/util"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	"github.com/radius-project/radius/pkg/sdk"
	resources "github.com/radius-project/radius/pkg/ucp/resources"
	awsresources "github.com/radius-project/radius/pkg/ucp/resources/aws"
	kubernetesresources "github.com/radius-project/radius/pkg/ucp/resources/kubernetes"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
	"github.com/radius-project/radius/pkg/ucp/util"

	"github.com/google/uuid"
	tfjson "github.com/hashicorp/terraform-json"
	"golang.org/x/exp/slices"
)

var _ driver.Driver = (*terraformDriver)(nil)

// NewTerraformDriver creates a new instance of driver to execute a Terraform recipe.
func NewTerraformDriver(ucpConn sdk.Connection, secretProvider *secretprovider.SecretProvider, options TerraformOptions, kubernetesClients kubernetesclientprovider.KubernetesClientProvider) driver.Driver {
	return &terraformDriver{
		terraformExecutor: terraform.NewExecutor(ucpConn, secretProvider, kubernetesClients),
		options:           options,
	}
}

// TerraformOptions represents the options required for execution of Terraform driver.
type TerraformOptions struct {
	// Path is the path to the directory mounted to the container where terraform can be installed and executed.
	Path string

	// EnvConfig is the configuration for the environment.
	EnvConfig recipes.Configuration
}

// terraformDriver represents a driver to interact with Terraform Recipe - deploy recipe, delete resources, etc.
type terraformDriver struct {
	// terraformExecutor is used to execute Terraform commands - deploy, destroy, etc.
	terraformExecutor terraform.TerraformExecutor

	// options contains options required to execute a Terraform recipe, such as the path to the directory mounted to the container where Terraform can be executed in sub directories.
	options TerraformOptions
}

// Execute creates a unique directory for each execution of terraform and deploys the recipe using the
// the Terraform CLI through terraform-exec. It returns a RecipeOutput or an error if the deployment fails.
func (d *terraformDriver) Execute(ctx context.Context, opts driver.ExecuteOptions) (*recipes.RecipeOutput, error) {
	logger := ucplog.FromContextOrDiscard(ctx)

	requestDirPath, err := d.createExecutionDirectory(ctx, opts.Recipe, opts.Definition)
	if err != nil {
		return nil, recipes.NewRecipeError(recipes.RecipeDeploymentFailed, err.Error(), recipes_util.RecipeSetupError, recipes.GetErrorDetails(err))
	}
	defer func() {
		if err := os.RemoveAll(requestDirPath); err != nil {
			logger.Info(fmt.Sprintf("Failed to cleanup Terraform execution directory %q. Err: %s", requestDirPath, err.Error()))
		}
	}()

	logger.Info("Configuring Terraform registry",
		"hasConfiguration", opts.Configuration.RecipeConfig.Terraform.Registry != nil,
		"secretsCount", len(opts.Secrets))

	regConfig, err := ConfigureTerraformRegistry(ctx, opts.Configuration, opts.Secrets, requestDirPath)
	if err != nil {
		logger.Error(err, "Failed to configure Terraform registry")
		return nil, fmt.Errorf("failed to configure terraform registry: %w", err)
	}

	if regConfig != nil {
		logger.Info("Terraform registry configured successfully",
			"configPath", regConfig.ConfigPath,
			"envVarsCount", len(regConfig.EnvVars),
			"tempFilesCount", len(regConfig.TempFiles))
	} else {
		logger.Info("No Terraform registry configuration needed")
	}

	defer func() {
		if err := CleanupTerraformRegistryConfig(ctx, regConfig); err != nil {
			// Log the error but don't fail the operation
			logger.Info(fmt.Sprintf("Failed to cleanup Terraform registry configuration: %s", err.Error()))
		}
	}()

	// Extract registry environment variables to pass to the executor
	var registryEnv map[string]string
	if regConfig != nil && regConfig.EnvVars != nil {
		registryEnv = regConfig.EnvVars
		logger.Info("Extracted registry environment variables",
			"count", len(registryEnv))

		for key, value := range registryEnv {
			logger.Info("Registry environment variable", "key", key, "value", value)
		}
	}

	// Get the secret store ID associated with the git private terraform repository source.
	secretStoreID, err := GetPrivateGitRepoSecretStoreID(opts.Configuration, opts.Definition.TemplatePath)
	if err != nil {
		return nil, err
	}

	// Add credential information to .gitconfig for module source of type git if applicable.
	err = addSecretsToGitConfigIfApplicable(secretStoreID, opts.Secrets, requestDirPath, opts.Definition.TemplatePath)
	if err != nil {
		return nil, err
	}

	tfState, err := d.terraformExecutor.Deploy(ctx, terraform.Options{
		RootDir:        requestDirPath,
		EnvConfig:      &opts.Configuration,
		ResourceRecipe: &opts.Recipe,
		EnvRecipe:      &opts.Definition,
		Secrets:        opts.Secrets,
		RegistryEnv:    registryEnv,
	})

	unsetError := unsetGitConfigForDirIfApplicable(secretStoreID, opts.Secrets, requestDirPath, opts.Definition.TemplatePath)
	if unsetError != nil {
		return nil, unsetError
	}

	if err != nil {
		return nil, recipes.NewRecipeError(recipes.RecipeDeploymentFailed, err.Error(), recipes_util.ExecutionError, recipes.GetErrorDetails(err))
	}

	recipeOutputs, err := d.prepareRecipeResponse(ctx, opts.Definition, tfState)
	if err != nil {
		return nil, recipes.NewRecipeError(recipes.InvalidRecipeOutputs, fmt.Sprintf("failed to read the recipe output %q: %s", recipes.ResultPropertyName, err.Error()), recipes_util.ExecutionError, recipes.GetErrorDetails(err))
	}

	return recipeOutputs, nil
}

// Delete creates a unique directory for each execution of terraform and deletes the resources deployed by the Terraform module
// using the Terraform CLI through terraform-exec. It returns an error if the deletion fails.
func (d *terraformDriver) Delete(ctx context.Context, opts driver.DeleteOptions) error {
	logger := ucplog.FromContextOrDiscard(ctx)

	requestDirPath, err := d.createExecutionDirectory(ctx, opts.Recipe, opts.Definition)
	if err != nil {
		return recipes.NewRecipeError(recipes.RecipeDeletionFailed, err.Error(), "", recipes.GetErrorDetails(err))
	}
	defer func() {
		if err := os.RemoveAll(requestDirPath); err != nil {
			logger.Info(fmt.Sprintf("Failed to cleanup Terraform execution directory %q. Err: %s", requestDirPath, err.Error()))
		}
	}()

	logger.Info("Configuring Terraform registry for delete operation",
		"hasConfiguration", opts.Configuration.RecipeConfig.Terraform.Registry != nil,
		"secretsCount", len(opts.Secrets))

	regConfig, err := ConfigureTerraformRegistry(ctx, opts.Configuration, opts.Secrets, requestDirPath)
	if err != nil {
		logger.Error(err, "Failed to configure Terraform registry for delete")
		return recipes.NewRecipeError(recipes.RecipeDeletionFailed, fmt.Sprintf("failed to configure terraform registry: %s", err.Error()), "", nil)
	}

	if regConfig != nil {
		logger.Info("Terraform registry configured successfully for delete",
			"configPath", regConfig.ConfigPath,
			"envVarsCount", len(regConfig.EnvVars),
			"tempFilesCount", len(regConfig.TempFiles))
	} else {
		logger.Info("No Terraform registry configuration needed for delete")
	}

	defer func() {
		if err := CleanupTerraformRegistryConfig(ctx, regConfig); err != nil {
			// Log the error but don't fail the operation
			logger.Info(fmt.Sprintf("Failed to cleanup Terraform registry configuration: %s", err.Error()))
		}
	}()

	// Extract registry environment variables to pass to the executor
	var registryEnv map[string]string
	if regConfig != nil && regConfig.EnvVars != nil {
		registryEnv = regConfig.EnvVars
		logger.Info("Extracted registry environment variables for delete",
			"count", len(registryEnv))

		for key, value := range registryEnv {
			logger.Info("Registry environment variable", "key", key, "value", value)
		}
	}

	// Get the secret store ID associated with the git private terraform repository source.
	secretStoreID, err := GetPrivateGitRepoSecretStoreID(opts.Configuration, opts.Definition.TemplatePath)
	if err != nil {
		return err
	}

	// Add credential information to .gitconfig for module source of type git if applicable.
	err = addSecretsToGitConfigIfApplicable(secretStoreID, opts.Secrets, requestDirPath, opts.Definition.TemplatePath)
	if err != nil {
		return err
	}

	err = d.terraformExecutor.Delete(ctx, terraform.Options{
		RootDir:        requestDirPath,
		EnvConfig:      &opts.Configuration,
		ResourceRecipe: &opts.Recipe,
		EnvRecipe:      &opts.Definition,
		Secrets:        opts.Secrets,
		RegistryEnv:    registryEnv,
	})

	unsetError := unsetGitConfigForDirIfApplicable(secretStoreID, opts.Secrets, requestDirPath, opts.Definition.TemplatePath)
	if unsetError != nil {
		return unsetError
	}

	if err != nil {
		return recipes.NewRecipeError(recipes.RecipeDeletionFailed, err.Error(), "", recipes.GetErrorDetails(err))
	}

	return nil
}

// prepareRecipeResponse populates the recipe response from the module output named "result" and the
// resources deployed by the Terraform module. The outputs and resources are retrieved from the input Terraform JSON state.
func (d *terraformDriver) prepareRecipeResponse(ctx context.Context, definition recipes.EnvironmentDefinition, tfState *tfjson.State) (*recipes.RecipeOutput, error) {
	// We need to use reflect.DeepEqual to compare the struct that has a slice with an empty struct.
	// The reason is that Go does not allow comparison of structs that contain slices.
	// Please see: https://go.dev/ref/spec#Comparison_operators.
	if tfState == nil || reflect.DeepEqual(*tfState, tfjson.State{}) {
		return &recipes.RecipeOutput{}, errors.New("terraform state is empty")
	}

	recipeResponse := &recipes.RecipeOutput{}
	if tfState.Values != nil && tfState.Values.Outputs != nil {
		// We populate the recipe response from the 'result' output (if set).
		moduleOutputs := tfState.Values.Outputs
		if result, ok := moduleOutputs[recipes.ResultPropertyName].Value.(map[string]any); ok {
			err := recipeResponse.PrepareRecipeResponse(result)
			if err != nil {
				return &recipes.RecipeOutput{}, err
			}
		}
	}

	recipeResponse.Status = &rpv1.RecipeStatus{
		TemplateKind:    recipes.TemplateKindTerraform,
		TemplatePath:    definition.TemplatePath,
		TemplateVersion: definition.TemplateVersion,
	}

	var deployedResources []string
	if tfState.Values != nil && tfState.Values.RootModule != nil {
		var err error
		deployedResources, err = d.getDeployedOutputResources(ctx, tfState.Values.RootModule)
		if err != nil {
			return &recipes.RecipeOutput{}, err
		}
	}

	uniqueResourceIDs := []string{}
	for _, val := range recipeResponse.Resources {
		uniqueResourceIDs = append(uniqueResourceIDs, strings.ToLower(val))
	}

	for _, val := range deployedResources {
		if !slices.Contains(uniqueResourceIDs, strings.ToLower(val)) {
			recipeResponse.Resources = append(recipeResponse.Resources, val)
		}
	}

	return recipeResponse, nil
}

// createExecutionDirectory creates a unique directory for each execution of terraform.
func (d *terraformDriver) createExecutionDirectory(ctx context.Context, recipe recipes.ResourceMetadata, definition recipes.EnvironmentDefinition) (string, error) {
	logger := ucplog.FromContextOrDiscard(ctx)

	if d.options.Path == "" {
		return "", fmt.Errorf("path is a required option for Terraform driver")
	}

	// We need a unique directory per execution of terraform. We generate this using the unique operation id of the async request so that names are always unique,
	// but we can also trace them to the resource we were working on through operationID. UUID is added to the path to prevent overwrite across retries for same ARM request.
	dirID := ""
	armCtx := v1.ARMRequestContextFromContext(ctx)
	if armCtx.OperationID != uuid.Nil {
		dirID = armCtx.OperationID.String() + "-" + uuid.NewString()
	} else {
		// If the operationID is nil, we generate a new UUID for unique directory name combined with resource id so that we can trace it to the resource.
		// Ideally operationID should not be nil.
		logger.Info("Empty operation ID provided in the request context, using uuid to generate a unique directory name")
		dirID = util.NormalizeStringToLower(recipe.ResourceID) + "-" + uuid.NewString()
	}
	requestDirPath := filepath.Join(d.options.Path, dirID)

	logger.Info(fmt.Sprintf("Deploying terraform recipe: %q, template: %q, execution directory: %q", recipe.Name, definition.TemplatePath, requestDirPath))
	if err := os.MkdirAll(requestDirPath, 0755); err != nil {
		return "", fmt.Errorf("failed to create directory %q to execute terraform: %s", requestDirPath, err.Error())
	}

	return requestDirPath, nil
}

// GetRecipeMetadata returns the Terraform Recipe parameters by downloading the module and retrieving variable information
func (d *terraformDriver) GetRecipeMetadata(ctx context.Context, opts driver.BaseOptions) (map[string]any, error) {
	logger := ucplog.FromContextOrDiscard(ctx)

	requestDirPath, err := d.createExecutionDirectory(ctx, opts.Recipe, opts.Definition)
	if err != nil {
		return nil, recipes.NewRecipeError(recipes.RecipeGetMetadataFailed, err.Error(), "", recipes.GetErrorDetails(err))
	}
	defer func() {
		if err := os.RemoveAll(requestDirPath); err != nil {
			logger.Info(fmt.Sprintf("Failed to cleanup Terraform execution directory %q. Err: %s", requestDirPath, err.Error()))
		}
	}()

	logger.Info("Configuring Terraform registry for metadata operation",
		"hasConfiguration", opts.Configuration.RecipeConfig.Terraform.Registry != nil,
		"secretsCount", len(opts.Secrets))

	regConfig, err := ConfigureTerraformRegistry(ctx, opts.Configuration, opts.Secrets, requestDirPath)
	if err != nil {
		logger.Error(err, "Failed to configure Terraform registry for metadata")
		return nil, recipes.NewRecipeError(recipes.RecipeGetMetadataFailed, fmt.Sprintf("failed to configure terraform registry: %s", err.Error()), "", nil)
	}

	if regConfig != nil {
		logger.Info("Terraform registry configured successfully for metadata",
			"configPath", regConfig.ConfigPath,
			"envVarsCount", len(regConfig.EnvVars),
			"tempFilesCount", len(regConfig.TempFiles))
	} else {
		logger.Info("No Terraform registry configuration needed for metadata")
	}

	defer func() {
		if err := CleanupTerraformRegistryConfig(ctx, regConfig); err != nil {
			// Log the error but don't fail the operation
			logger.Info(fmt.Sprintf("Failed to cleanup Terraform registry configuration: %s", err.Error()))
		}
	}()

	// Extract registry environment variables to pass to the executor
	var registryEnv map[string]string
	if regConfig != nil && regConfig.EnvVars != nil {
		registryEnv = regConfig.EnvVars
		logger.Info("Extracted registry environment variables for metadata",
			"count", len(registryEnv))

		for key, value := range registryEnv {
			logger.Info("Registry environment variable", "key", key, "value", value)
		}
	}

	// Get the secret store ID associated with the git private terraform repository source.
	secretStoreID, err := GetPrivateGitRepoSecretStoreID(opts.Configuration, opts.Definition.TemplatePath)
	if err != nil {
		return nil, err
	}

	// Add credential information to .gitconfig for module source of type git if applicable.
	err = addSecretsToGitConfigIfApplicable(secretStoreID, opts.Secrets, requestDirPath, opts.Definition.TemplatePath)
	if err != nil {
		return nil, err
	}

	recipeData, err := d.terraformExecutor.GetRecipeMetadata(ctx, terraform.Options{
		RootDir:        requestDirPath,
		ResourceRecipe: &opts.Recipe,
		EnvRecipe:      &opts.Definition,
		RegistryEnv:    registryEnv,
	})

	unsetError := unsetGitConfigForDirIfApplicable(secretStoreID, opts.Secrets, requestDirPath, opts.Definition.TemplatePath)
	if unsetError != nil {
		return nil, unsetError
	}

	if err != nil {
		return nil, recipes.NewRecipeError(recipes.RecipeGetMetadataFailed, err.Error(), "", recipes.GetErrorDetails(err))
	}

	return recipeData, nil
}

// FindSecretIDs is used to retrieve a map of secretStoreIDs and corresponding secret keys.
// associated with the given environment configuration and environment definition.
func (d *terraformDriver) FindSecretIDs(ctx context.Context, envConfig recipes.Configuration, definition recipes.EnvironmentDefinition) (secretStoreIDResourceKeys map[string][]string, err error) {
	secretStoreIDResourceKeys = make(map[string][]string)

	// Get the secret store ID associated with the git private terraform repository source.
	secretStoreID, err := GetPrivateGitRepoSecretStoreID(envConfig, definition.TemplatePath)
	if err != nil {
		return nil, err
	}

	if secretStoreID != "" {
		// For Git authentication, we request both pat and username keys.
		// The username is optional and will be handled gracefully if not present.
		secretStoreIDResourceKeys[secretStoreID] = []string{PrivateRegistrySecretKey_Pat, PrivateRegistrySecretKey_Username}
	}

	// Get the secret IDs and associated keys in provider configuration and environment variables
	providerSecretIDs := terraform.GetProviderEnvSecretIDs(envConfig)

	// Merge secretStoreIDResourceKeys with providerSecretIDs
	for secretStoreID, keys := range providerSecretIDs {
		if _, ok := secretStoreIDResourceKeys[secretStoreID]; !ok {
			secretStoreIDResourceKeys[secretStoreID] = keys
		} else {
			secretStoreIDResourceKeys[secretStoreID] = append(secretStoreIDResourceKeys[secretStoreID], keys...)
		}
	}

	terraformRegistrySecrets := terraform.GetTerraformRegistrySecretIDs(envConfig)
	for secretStoreID, keys := range terraformRegistrySecrets {
		if _, ok := secretStoreIDResourceKeys[secretStoreID]; !ok {
			secretStoreIDResourceKeys[secretStoreID] = keys
		} else {
			secretStoreIDResourceKeys[secretStoreID] = append(secretStoreIDResourceKeys[secretStoreID], keys...)
		}
	}

	return secretStoreIDResourceKeys, nil
}

// getDeployedOutputResources is used to the get the resource IDs by parsing the terraform state for resource information and using it to create UCP qualified IDs.
// Currently only Azure, AWS and Kubernetes providers are supported by output resources.
func (d *terraformDriver) getDeployedOutputResources(ctx context.Context, module *tfjson.StateModule) ([]string, error) {
	logger := ucplog.FromContextOrDiscard(ctx)
	recipeResources := []string{}
	if module == nil {
		return recipeResources, nil
	}

	registry := GetTerraformRegistry(d.options.EnvConfig)
	azureProvider := GetTerraformProviderFullName(registry,
		GetTerraformProviderName(d.options.EnvConfig, DefaultTerraformAzureProvider, DefaultTerraformAzureProvider))
	awsProvider := GetTerraformProviderFullName(registry,
		GetTerraformProviderName(d.options.EnvConfig, DefaultTerraformAWSProvider, DefaultTerraformAWSProvider))
	kubernetesProvider := GetTerraformProviderFullName(registry,
		GetTerraformProviderName(d.options.EnvConfig, DefaultTerraformKubernetesProvider, DefaultTerraformKubernetesProvider))

	for _, resource := range module.Resources {
		switch resource.ProviderName {
		case kubernetesProvider:
			var resourceType, resourceName, namespace, provider string
			// For resource type "kubernetes_manifest" get the required details from the manifest property.
			// https://registry.terraform.io/providers/hashicorp/kubernetes/latest/docs/resources/manifest
			if resource.Type == "kubernetes_manifest" {
				if manifest, ok := resource.AttributeValues["manifest"].(map[string]interface{}); ok {
					if metadata, ok := manifest["metadata"].(map[string]interface{}); ok {
						if name, ok := metadata["name"].(string); ok {
							resourceName = name
						}
						if ns, ok := metadata["namespace"].(string); ok {
							namespace = ns
						}
					}

					if apiVersion, ok := manifest["apiVersion"].(string); ok {
						providerVersion := strings.Split(apiVersion, "/")
						if len(providerVersion) == 0 {
							return []string{}, errors.New("apiVersion is empty")
						}
						provider = providerVersion[0]
					} else {
						return []string{}, errors.New("unable to get apiVersion information from the resource")
					}

					if kind, ok := manifest["kind"].(string); ok {
						resourceType = kind
					}
				}
			} else {
				// Kubernetes resource types are prefixed with "kubernetes_" keyword, remove the prefix
				// Removing the "_" separator from the resource type. Ex: kubernetes_service_account -> serviceaccount
				resourceType = strings.Join(strings.Split(resource.Type, "_")[1:], "")

				if resource.AttributeValues != nil {
					if metadataList, ok := resource.AttributeValues["metadata"].([]interface{}); ok {
						if len(metadataList) == 0 {
							return []string{}, errors.New("")
						}
						metadata := metadataList[0].(map[string]interface{})
						if name, ok := metadata["name"].(string); ok {
							resourceName = name
						}
						if ns, ok := metadata["namespace"].(string); ok {
							namespace = ns
						}
					}

				}
			}
			kubernetesResourceID, err := kubernetesresources.ToUCPResourceID(namespace, resourceType, resourceName, provider)
			if err != nil {
				return []string{}, err
			}
			recipeResources = append(recipeResources, kubernetesResourceID)
		case azureProvider:
			if resource.AttributeValues != nil {
				if id, ok := resource.AttributeValues["id"].(string); ok {
					_, err := resources.ParseResource(id)
					if err != nil {
						// The Azure resources Ids that doesnt not follow relative ID format are mostly Non ARM resources and not added to the recipe output.
						logger.Info("Resource ID does not represent ARM resource and is not added to recipe output", "ResourceID", id)
					} else {
						recipeResources = append(recipeResources, id)
					}
				}
			}
		case awsProvider:
			if resource.AttributeValues != nil {
				if arn, ok := resource.AttributeValues["arn"].(string); ok {
					awsResourceID, err := awsresources.ToUCPResourceID(arn)
					if err != nil {
						return []string{}, err
					}
					recipeResources = append(recipeResources, awsResourceID)
				}
			}
		default:
			continue
		}
	}

	for _, childModule := range module.ChildModules {
		modResources, err := d.getDeployedOutputResources(ctx, childModule)
		if err != nil {
			return []string{}, err
		}
		recipeResources = append(recipeResources, modResources...)
	}

	return recipeResources, nil
}
