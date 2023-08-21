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
	"fmt"
	"strconv"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"

	deployments "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
	"github.com/go-logr/logr"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/pkg/linkrp/processors"
	"github.com/project-radius/radius/pkg/metrics"
	"github.com/project-radius/radius/pkg/recipes"
	"github.com/project-radius/radius/pkg/recipes/recipecontext"
	"github.com/project-radius/radius/pkg/resourcemodel"
	"github.com/project-radius/radius/pkg/rp/util"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
	clients "github.com/project-radius/radius/pkg/sdk/clients"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/pkg/ucp/ucplog"

	coredm "github.com/project-radius/radius/pkg/corerp/datamodel"
)

//go:generate mockgen -destination=./mock_driver.go -package=driver -self_package github.com/project-radius/radius/pkg/recipes/driver github.com/project-radius/radius/pkg/recipes/driver Driver
const (
	deploymentPrefix = "recipe"
	pollFrequency    = time.Second * 5
	recipeParameters = "parameters"
)

var _ Driver = (*bicepDriver)(nil)

// NewBicepDriver creates a new bicep driver instance with the given ARM client options, deployment client and resource client.
func NewBicepDriver(armOptions *arm.ClientOptions, deploymentClient *clients.ResourceDeploymentsClient, client processors.ResourceClient) Driver {
	return &bicepDriver{ArmClientOptions: armOptions, DeploymentClient: deploymentClient, ResourceClient: client}
}

type bicepDriver struct {
	ArmClientOptions *arm.ClientOptions
	DeploymentClient *clients.ResourceDeploymentsClient
	ResourceClient   processors.ResourceClient
}

// Execute fetches recipe contents from container registry, creates a deployment ID, a recipe context parameter, recipe parameters,
// a provider config, and deploys a bicep template for the recipe using UCP deployment client, then polls until the deployment
// is done and prepares the recipe response.
func (d *bicepDriver) Execute(ctx context.Context, configuration recipes.Configuration, recipe recipes.ResourceMetadata, definition recipes.EnvironmentDefinition) (*recipes.RecipeOutput, error) {
	logger := logr.FromContextOrDiscard(ctx)
	logger.Info(fmt.Sprintf("Deploying recipe: %q, template: %q", definition.Name, definition.TemplatePath))

	recipeData := make(map[string]any)
	downloadStartTime := time.Now()
	err := util.ReadFromRegistry(ctx, definition.TemplatePath, &recipeData)
	if err != nil {
		return nil, recipes.NewRecipeError(recipes.RecipeDownloadFailed, err.Error(), recipes.GetRecipeErrorDetails(err))
	}
	metrics.DefaultRecipeEngineMetrics.RecordRecipeDownloadDuration(ctx, downloadStartTime,
		metrics.NewRecipeAttributes(metrics.RecipeEngineOperationDownloadRecipe, recipe.Name, &definition, metrics.SuccessfulOperationState))

	// create the context object to be passed to the recipe deployment
	recipeContext, err := recipecontext.New(&recipe, &configuration)
	if err != nil {
		return nil, recipes.NewRecipeError(recipes.RecipeDeploymentFailed, err.Error(), recipes.GetRecipeErrorDetails(err))
	}

	// get the parameters after resolving the conflict between developer and operator parameters
	// if the recipe template also has the context parameter defined then add it to the parameter for deployment
	isContextParameterDefined := hasContextParameter(recipeData)
	parameters := createRecipeParameters(recipe.Parameters, definition.Parameters, isContextParameterDefined, recipeContext)

	deploymentName := deploymentPrefix + strconv.FormatInt(time.Now().UnixNano(), 10)
	deploymentID, err := createDeploymentID(recipeContext.Resource.ID, deploymentName)
	if err != nil {
		return nil, recipes.NewRecipeError(recipes.RecipeDeploymentFailed, err.Error(), recipes.GetRecipeErrorDetails(err))
	}

	// Provider config will specify the Azure and AWS scopes (if provided).
	providerConfig := newProviderConfig(deploymentID.FindScope(resources.ResourceGroupsSegment), configuration.Providers)

	logger.Info("deploying bicep template for recipe", "deploymentID", deploymentID)
	if providerConfig.AWS != nil {
		logger.Info("using AWS provider", "deploymentID", deploymentID, "scope", providerConfig.AWS.Value.Scope)
	}
	if providerConfig.Az != nil {
		logger.Info("using Azure provider", "deploymentID", deploymentID, "scope", providerConfig.Az.Value.Scope)
	}

	poller, err := d.DeploymentClient.CreateOrUpdate(
		ctx,
		clients.Deployment{
			Properties: &clients.DeploymentProperties{
				Mode:           deployments.DeploymentModeIncremental,
				ProviderConfig: &providerConfig,
				Parameters:     parameters,
				Template:       recipeData,
			},
		},
		deploymentID.String(),
		clients.DeploymentsClientAPIVersion,
	)
	if err != nil {
		return nil, recipes.NewRecipeError(recipes.RecipeDeploymentFailed, err.Error(), recipes.GetRecipeErrorDetails(err))
	}

	resp, err := poller.PollUntilDone(ctx, &runtime.PollUntilDoneOptions{Frequency: pollFrequency})
	if err != nil {
		return nil, recipes.NewRecipeError(recipes.RecipeDeploymentFailed, err.Error(), recipes.GetRecipeErrorDetails(err))
	}

	recipeResponse, err := d.prepareRecipeResponse(resp.Properties.Outputs, resp.Properties.OutputResources)
	if err != nil {
		return nil, recipes.NewRecipeError(recipes.InvalidRecipeOutputs, fmt.Sprintf("failed to read the recipe output %q: %s", recipes.ResultPropertyName, err.Error()), recipes.GetRecipeErrorDetails(err))
	}

	return recipeResponse, nil
}

// Delete deletes output resources in reverse dependency order, logging each resource deleted and skipping any
// resources that are not managed by Radius. It returns an error if any of the resources fail to delete.
func (d *bicepDriver) Delete(ctx context.Context, outputResources []rpv1.OutputResource) error {
	logger := ucplog.FromContextOrDiscard(ctx)

	orderedOutputResources, err := rpv1.OrderOutputResources(outputResources)
	if err != nil {
		return recipes.NewRecipeError(recipes.RecipeDeletionFailed, err.Error(), recipes.GetRecipeErrorDetails(err))
	}

	// Loop over each output resource and delete in reverse dependency order
	for i := len(orderedOutputResources) - 1; i >= 0; i-- {
		outputResource := orderedOutputResources[i]
		id := outputResource.Identity.GetID()
		if err != nil {
			return recipes.NewRecipeError(recipes.RecipeDeletionFailed, err.Error(), recipes.GetRecipeErrorDetails(err))
		}
		logger.Info(fmt.Sprintf("Deleting output resource: %v, LocalID: %s, resource type: %s\n", outputResource.Identity, outputResource.LocalID, outputResource.ResourceType.Type))
		if outputResource.RadiusManaged == nil || !*outputResource.RadiusManaged {
			continue
		}

		err = d.ResourceClient.Delete(ctx, id, resourcemodel.APIVersionUnknown)
		if err != nil {
			return recipes.NewRecipeError(recipes.RecipeDeletionFailed, err.Error(), recipes.GetRecipeErrorDetails(err))
		}
		logger.Info(fmt.Sprintf("Deleted output resource: %q", id), ucplog.LogFieldTargetResourceID, id)

	}

	return nil
}

func hasContextParameter(recipeData map[string]any) bool {
	parametersAny, ok := recipeData[recipeParameters]
	if !ok {
		return false
	}

	parameters, ok := parametersAny.(map[string]any)
	if !ok {
		return false
	}

	_, ok = parameters[datamodel.RecipeContextParameter]
	return ok
}

// createRecipeParameters creates the parameters to be passed for recipe deployment after handling conflicts in parameters set by operator and developer.
// In case of conflict the developer parameter takes precedence. If recipe has context parameter defined adds the context information to the parameters list
func createRecipeParameters(devParams, operatorParams map[string]any, isCxtSet bool, recipeContext *recipecontext.Context) map[string]any {
	parameters := map[string]any{}
	for k, v := range operatorParams {
		parameters[k] = map[string]any{
			"value": v,
		}
	}
	for k, v := range devParams {
		parameters[k] = map[string]any{
			"value": v,
		}
	}
	if isCxtSet {
		parameters[recipecontext.RecipeContextParamKey] = map[string]any{
			"value": *recipeContext,
		}
	}
	return parameters
}

func createDeploymentID(resourceID string, deploymentName string) (resources.ID, error) {
	parsed, err := resources.ParseResource(resourceID)
	if err != nil {
		return resources.ID{}, err
	}

	resourceGroup := parsed.FindScope(resources.ResourceGroupsSegment)
	return resources.ParseResource(fmt.Sprintf("/planes/radius/local/resourceGroups/%s/providers/Microsoft.Resources/deployments/%s", resourceGroup, deploymentName))
}

func newProviderConfig(resourceGroup string, envProviders coredm.Providers) clients.ProviderConfig {
	config := clients.NewDefaultProviderConfig(resourceGroup)

	if envProviders.Azure != (coredm.ProvidersAzure{}) {
		config.Az = &clients.Az{
			Type: clients.ProviderTypeAzure,
			Value: clients.Value{
				Scope: envProviders.Azure.Scope,
			},
		}
	}

	if envProviders.AWS != (coredm.ProvidersAWS{}) {
		config.AWS = &clients.AWS{
			Type: clients.ProviderTypeAWS,
			Value: clients.Value{
				Scope: envProviders.AWS.Scope,
			},
		}
	}

	return config
}

// prepareRecipeResponse populates the recipe response from parsing the deployment output 'result' object and the
// resources created by the template.
func (d *bicepDriver) prepareRecipeResponse(outputs any, resources []*deployments.ResourceReference) (*recipes.RecipeOutput, error) {
	// We populate the recipe response from the 'result' output (if set)
	// and the resources created by the template.
	//
	// Note that there are two ways a resource can be returned:
	// - Implicitly when it is created in the template (it will be in 'resources').
	// - Explicitly as part of the 'result' output.
	//
	// The latter is needed because non-ARM and non-UCP resources are not returned as part of the implicit 'resources'
	// collection. For us this mostly means Kubernetes resources - the user has to be explicit.
	recipeResponse := &recipes.RecipeOutput{}
	out, ok := outputs.(map[string]any)
	if ok {
		if result, ok := out[recipes.ResultPropertyName].(map[string]any); ok {
			if resultValue, ok := result["value"].(map[string]any); ok {
				err := recipeResponse.PrepareRecipeResponse(resultValue)
				if err != nil {
					return &recipes.RecipeOutput{}, err
				}
			}
		}
	}

	// process the 'resources' created by the template
	for _, id := range resources {
		recipeResponse.Resources = append(recipeResponse.Resources, *id.ID)
	}

	return recipeResponse, nil
}
