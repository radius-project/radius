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
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/radius-project/radius/pkg/recipes"

	"github.com/radius-project/radius/pkg/recipes/terraform"
	recipes_util "github.com/radius-project/radius/pkg/recipes/util"
	"github.com/radius-project/radius/pkg/sdk"
	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/radius-project/radius/pkg/ucp/resources/aws"
	kubernetes_resource "github.com/radius-project/radius/pkg/ucp/resources/kubernetes"
	ucp_provider "github.com/radius-project/radius/pkg/ucp/secret/provider"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
	"github.com/radius-project/radius/pkg/ucp/util"

	tfjson "github.com/hashicorp/terraform-json"
)

var _ Driver = (*terraformDriver)(nil)

// NewTerraformDriver creates a new instance of driver to execute a Terraform recipe.
func NewTerraformDriver(ucpConn sdk.Connection, secretProvider *ucp_provider.SecretProvider, options TerraformOptions, k8sClientSet kubernetes.Interface) Driver {
	return &terraformDriver{
		terraformExecutor: terraform.NewExecutor(ucpConn, secretProvider, k8sClientSet),
		options:           options,
	}
}

// Options represents the options required for execution of Terraform driver.
type TerraformOptions struct {
	// Path is the path to the directory mounted to the container where terraform can be installed and executed.
	Path string
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
func (d *terraformDriver) Execute(ctx context.Context, opts ExecuteOptions) (*recipes.RecipeOutput, error) {
	logger := ucplog.FromContextOrDiscard(ctx)

	requestDirPath, err := d.createExecutionDirectory(ctx, opts.Recipe, opts.Definition)
	if err != nil {
		return nil, recipes.NewRecipeError(recipes.RecipeDeploymentFailed, err.Error(), recipes_util.RecipeSetupError, recipes.GetRecipeErrorDetails(err))
	}
	defer func() {
		if err := os.RemoveAll(requestDirPath); err != nil {
			logger.Info(fmt.Sprintf("Failed to cleanup Terraform execution directory %q. Err: %s", requestDirPath, err.Error()))
		}
	}()

	tfState, err := d.terraformExecutor.Deploy(ctx, terraform.Options{
		RootDir:        requestDirPath,
		EnvConfig:      &opts.Configuration,
		ResourceRecipe: &opts.Recipe,
		EnvRecipe:      &opts.Definition,
	})
	if err != nil {
		return nil, recipes.NewRecipeError(recipes.RecipeDeploymentFailed, err.Error(), recipes_util.ExecutionError, recipes.GetRecipeErrorDetails(err))
	}

	recipeOutputs, err := d.prepareRecipeResponse(tfState)
	if err != nil {
		return nil, recipes.NewRecipeError(recipes.InvalidRecipeOutputs, fmt.Sprintf("failed to read the recipe output %q: %s", recipes.ResultPropertyName, err.Error()), recipes_util.ExecutionError, recipes.GetRecipeErrorDetails(err))
	}

	return recipeOutputs, nil
}

// Delete returns an error if called as it is not yet implemented.
func (d *terraformDriver) Delete(ctx context.Context, opts DeleteOptions) error {
	logger := ucplog.FromContextOrDiscard(ctx)

	requestDirPath, err := d.createExecutionDirectory(ctx, opts.Recipe, opts.Definition)
	if err != nil {
		return recipes.NewRecipeError(recipes.RecipeDeletionFailed, err.Error(), "", recipes.GetRecipeErrorDetails(err))
	}
	defer func() {
		if err := os.RemoveAll(requestDirPath); err != nil {
			logger.Info(fmt.Sprintf("Failed to cleanup Terraform execution directory %q. Err: %s", requestDirPath, err.Error()))
		}
	}()

	err = d.terraformExecutor.Delete(ctx, terraform.Options{
		RootDir:        requestDirPath,
		EnvConfig:      &opts.Configuration,
		ResourceRecipe: &opts.Recipe,
		EnvRecipe:      &opts.Definition,
	})
	if err != nil {
		return recipes.NewRecipeError(recipes.RecipeDeletionFailed, err.Error(), "", recipes.GetRecipeErrorDetails(err))
	}

	return nil
}

// prepareRecipeResponse populates the recipe response from the module output named "result" and the
// resources deployed by the Terraform module. The outputs and resources are retrieved from the input Terraform JSON state.
func (d *terraformDriver) prepareRecipeResponse(tfState *tfjson.State) (*recipes.RecipeOutput, error) {
	if tfState == nil || (*tfState == tfjson.State{}) {
		return &recipes.RecipeOutput{}, errors.New("terraform state is empty")
	}

	recipeResponse := &recipes.RecipeOutput{}
	moduleOutputs := tfState.Values.Outputs
	if moduleOutputs != nil {
		// We populate the recipe response from the 'result' output (if set).
		if result, ok := moduleOutputs[recipes.ResultPropertyName].Value.(map[string]any); ok {
			err := recipeResponse.PrepareRecipeResponse(result)
			if err != nil {
				return &recipes.RecipeOutput{}, err
			}
		}
	}
	deployedResources, err := d.getDeployedOutputResources(tfState.Values.RootModule)
	if err != nil {
		return &recipes.RecipeOutput{}, err
	}
	uniqueResources := []string{}
	recipeResponse.Resources = append(recipeResponse.Resources, deployedResources...)
	uniqueResourcesMap := map[string]bool{}
	for _, val := range recipeResponse.Resources {
		if ok, _ := uniqueResourcesMap[val]; !ok {
			uniqueResourcesMap[val] = true
			uniqueResources = append(uniqueResources, val)
		}
	}
	recipeResponse.Resources = uniqueResources
	return recipeResponse, err
}

// createExecutionDirectory creates a unique directory for each execution of terraform.
func (d *terraformDriver) createExecutionDirectory(ctx context.Context, recipe recipes.ResourceMetadata, definition recipes.EnvironmentDefinition) (string, error) {
	logger := ucplog.FromContextOrDiscard(ctx)

	if d.options.Path == "" {
		return "", fmt.Errorf("path is a required option for Terraform driver")
	}

	// We need a unique directory per execution of terraform. We generate this using the unique operation id of the async request so that names are always unique,
	// but we can also trace them to the resource we were working on through operationID.
	dirID := ""
	armCtx := v1.ARMRequestContextFromContext(ctx)
	if armCtx.OperationID != uuid.Nil {
		dirID = armCtx.OperationID.String()
	} else {
		// If the operationID is nil, we generate a new UUID for unique directory name combined with resource id so that we can trace it to the resource.
		// Ideally operationID should not be nil.
		logger.Info("Empty operation ID provided in the request context, using uuid to generate a unique directory name")
		dirID = util.NormalizeStringToLower(recipe.ResourceID) + "/" + uuid.NewString()
	}
	requestDirPath := filepath.Join(d.options.Path, dirID)

	logger.Info(fmt.Sprintf("Deploying terraform recipe: %q, template: %q, execution directory: %q", recipe.Name, definition.TemplatePath, requestDirPath))
	if err := os.MkdirAll(requestDirPath, 0755); err != nil {
		return "", fmt.Errorf("failed to create directory %q to execute terraform: %s", requestDirPath, err.Error())
	}

	return requestDirPath, nil
}

// GetRecipeMetadata returns the Terraform Recipe parameters by downloading the module and retrieving variable information
func (d *terraformDriver) GetRecipeMetadata(ctx context.Context, opts BaseOptions) (map[string]any, error) {
	logger := ucplog.FromContextOrDiscard(ctx)

	requestDirPath, err := d.createExecutionDirectory(ctx, opts.Recipe, opts.Definition)
	if err != nil {
		return nil, recipes.NewRecipeError(recipes.RecipeGetMetadataFailed, err.Error(), "", recipes.GetRecipeErrorDetails(err))
	}
	defer func() {
		if err := os.RemoveAll(requestDirPath); err != nil {
			logger.Info(fmt.Sprintf("Failed to cleanup Terraform execution directory %q. Err: %s", requestDirPath, err.Error()))
		}
	}()

	recipeData, err := d.terraformExecutor.GetRecipeMetadata(ctx, terraform.Options{
		RootDir:        requestDirPath,
		ResourceRecipe: &opts.Recipe,
		EnvRecipe:      &opts.Definition,
	})
	if err != nil {
		return nil, recipes.NewRecipeError(recipes.RecipeGetMetadataFailed, err.Error(), "", recipes.GetRecipeErrorDetails(err))
	}

	return recipeData, nil
}

func (d *terraformDriver) getDeployedOutputResources(module *tfjson.StateModule) ([]string, error) {
	recipeResources := []string{}
	if module == nil {
		return recipeResources, nil
	}
	for _, resource := range module.Resources {
		switch resource.ProviderName {
		case "registry.terraform.io/hashicorp/kubernetes":
			var resourceType, resourceName, namespace, provider string
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
					if apiVersion, ok := resource.AttributeValues["apiVersion"].(string); ok {
						provider = strings.Split(apiVersion, "/")[0]
					}
					if kind, ok := resource.AttributeValues["kind"].(string); ok {
						resourceType = kind
					}

				}
			} else {
				resourceType = strings.SplitN(resource.Type, "_", 2)[1]
				if resource.AttributeValues != nil {
					if id, ok := resource.AttributeValues["id"].(string); ok {
						namespace = strings.Split(id, "/")[0]
						resourceName = strings.Split(id, "/")[1]
					}
				}
			}
			recipeResources = append(recipeResources, kubernetes_resource.ToUCPResourceID(namespace, resourceType, resourceName, provider))
		case "registry.terraform.io/hashicorp/azurerm":
			if resource.AttributeValues != nil {
				if id, ok := resource.AttributeValues["id"].(string); ok {
					_, err := resources.ParseResource(id)
					if err != nil {
						return []string{}, nil
					}
					recipeResources = append(recipeResources, id)
				}
			}
		case "registry.terraform.io/hashicorp/aws":
			if resource.AttributeValues != nil {
				if arn, ok := resource.AttributeValues["arn"].(string); ok {
					recipeResources = append(recipeResources, aws.ToUCPResourceID(arn))
				}
			}
		default:
			continue
		}
		if resource.AttributeValues != nil {
			if id, ok := resource.AttributeValues["id"].(string); ok {
				recipeResources = append(recipeResources, id)
			}
		}
	}
	for _, childModule := range module.ChildModules {
		modResources, err := d.getDeployedOutputResources(childModule)
		if err != nil {
			return []string{}, nil
		}
		recipeResources = append(recipeResources, modResources...)
	}
	return recipeResources, nil
}
