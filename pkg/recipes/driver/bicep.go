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
	"github.com/radius-project/radius/pkg/metrics"
	"github.com/radius-project/radius/pkg/portableresources/datamodel"
	"github.com/radius-project/radius/pkg/portableresources/processors"
	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/recipes/recipecontext"
	"github.com/radius-project/radius/pkg/rp/util"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	clients "github.com/radius-project/radius/pkg/sdk/clients"
	"github.com/radius-project/radius/pkg/ucp/resources"
	resources_radius "github.com/radius-project/radius/pkg/ucp/resources/radius"
	"github.com/radius-project/radius/pkg/ucp/ucplog"

	coredm "github.com/radius-project/radius/pkg/corerp/datamodel"
)

//go:generate mockgen -destination=./mock_driver.go -package=driver -self_package github.com/radius-project/radius/pkg/recipes/driver github.com/radius-project/radius/pkg/recipes/driver Driver
const (
	deploymentPrefix = "recipe"
	pollFrequency    = time.Second * 5
	recipeParameters = "parameters"
)

var _ Driver = (*bicepDriver)(nil)

// NewBicepDriver creates a new bicep driver instance with the given ARM client options, deployment client and resource client.
func NewBicepDriver(armOptions *arm.ClientOptions, deploymentClient *clients.ResourceDeploymentsClient, client processors.ResourceClient) Driver {
	return &bicepDriver{
		ArmClientOptions: armOptions,
		DeploymentClient: deploymentClient,
		ResourceClient:   client,
	}
}

type bicepDriver struct {
	ArmClientOptions *arm.ClientOptions
	DeploymentClient *clients.ResourceDeploymentsClient
	ResourceClient   processors.ResourceClient
}

// Execute fetches recipe contents from container registry, creates a deployment ID, a recipe context parameter, recipe parameters,
// a provider config, and deploys a bicep template for the recipe using UCP deployment client, then polls until the deployment
// is done and prepares the recipe response.
func (d *bicepDriver) Execute(ctx context.Context, opts ExecuteOptions) (*recipes.RecipeOutput, error) {
	logger := logr.FromContextOrDiscard(ctx)
	logger.Info(fmt.Sprintf("Deploying recipe: %q, template: %q", opts.Definition.Name, opts.Definition.TemplatePath))

	recipeData := make(map[string]any)
	downloadStartTime := time.Now()
	err := util.ReadFromRegistry(ctx, opts.Definition.TemplatePath, &recipeData)
	if err != nil {
		metrics.DefaultRecipeEngineMetrics.RecordRecipeDownloadDuration(ctx, downloadStartTime,
			metrics.NewRecipeAttributes(metrics.RecipeEngineOperationDownloadRecipe, opts.Recipe.Name, &opts.Definition, recipes.RecipeDownloadFailed))
		return nil, recipes.NewRecipeError(recipes.RecipeDownloadFailed, err.Error(), recipes.GetRecipeErrorDetails(err))
	}
	metrics.DefaultRecipeEngineMetrics.RecordRecipeDownloadDuration(ctx, downloadStartTime,
		metrics.NewRecipeAttributes(metrics.RecipeEngineOperationDownloadRecipe, opts.Recipe.Name, &opts.Definition, metrics.SuccessfulOperationState))

	// create the context object to be passed to the recipe deployment
	recipeContext, err := recipecontext.New(&opts.Recipe, &opts.Configuration)
	if err != nil {
		return nil, recipes.NewRecipeError(recipes.RecipeDeploymentFailed, err.Error(), recipes.GetRecipeErrorDetails(err))
	}

	// get the parameters after resolving the conflict between developer and operator parameters
	// if the recipe template also has the context parameter defined then add it to the parameter for deployment
	isContextParameterDefined := hasContextParameter(recipeData)
	parameters := createRecipeParameters(opts.Recipe.Parameters, opts.Definition.Parameters, isContextParameterDefined, recipeContext)

	deploymentName := deploymentPrefix + strconv.FormatInt(time.Now().UnixNano(), 10)
	deploymentID, err := createDeploymentID(recipeContext.Resource.ID, deploymentName)
	if err != nil {
		return nil, recipes.NewRecipeError(recipes.RecipeDeploymentFailed, err.Error(), recipes.GetRecipeErrorDetails(err))
	}

	// Provider config will specify the Azure and AWS scopes (if provided).
	providerConfig := newProviderConfig(deploymentID.FindScope(resources_radius.ScopeResourceGroups), opts.Configuration.Providers)

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

	// When a Radius portable resource consuming a recipe is redeployed, Garbage collection of the recipe resources that aren't included
	// in the currently deployed resources compared to the list of resources from the previous deployment needs to be deleted
	// as bicep does not take care of automatically deleting the unused resources.
	// Identify the output resources that are no longer relevant to the recipe.
	diff := d.getGCOutputResources(recipeResponse.Resources, opts.PrevState)

	// Deleting obsolete output resources.
	err = d.deleteGCOutputResources(ctx, diff)
	if err != nil {
		return nil, recipes.NewRecipeError(recipes.RecipeGarbageCollectionFailed, err.Error(), nil)
	}

	return recipeResponse, nil
}

// Delete deletes output resources in reverse dependency order, logging each resource deleted and skipping any
// resources that are not managed by Radius. It returns an error if any of the resources fail to delete.
func (d *bicepDriver) Delete(ctx context.Context, opts DeleteOptions) error {
	logger := ucplog.FromContextOrDiscard(ctx)

	orderedOutputResources, err := rpv1.OrderOutputResources(opts.OutputResources)
	if err != nil {
		return recipes.NewRecipeError(recipes.RecipeDeletionFailed, err.Error(), recipes.GetRecipeErrorDetails(err))
	}

	// Loop over each output resource and delete in reverse dependency order
	for i := len(orderedOutputResources) - 1; i >= 0; i-- {
		outputResource := orderedOutputResources[i]

		resourceType := outputResource.GetResourceType()
		logger.Info(fmt.Sprintf("Deleting output resource: %v, LocalID: %s, resource type: %s\n", outputResource.ID.String(), outputResource.LocalID, resourceType.Type))
		if outputResource.RadiusManaged == nil || !*outputResource.RadiusManaged {
			continue
		}

		err = d.ResourceClient.Delete(ctx, outputResource.ID.String())
		if err != nil {
			return recipes.NewRecipeError(recipes.RecipeDeletionFailed, err.Error(), recipes.GetRecipeErrorDetails(err))
		}
		logger.Info(fmt.Sprintf("Deleted output resource: %q", outputResource.ID.String()), ucplog.LogFieldTargetResourceID, outputResource.ID.String())

	}

	return nil
}

// GetRecipeMetadata gets the Bicep recipe parameters information from the container registry
func (d *bicepDriver) GetRecipeMetadata(ctx context.Context, opts BaseOptions) (map[string]any, error) {
	// Recipe parameters can be found in the recipe data pulled from the registry in the following format:
	//	{
	//		"parameters": {
	//			<parameter-name>: {
	//				<parameter-constraint-name> : <parameter-constraint-value>
	// 			}
	//		}
	//	}
	// For example:
	//	{
	//		"parameters": {
	//			"location": {
	//				"type": "string",
	//				"defaultValue" : "[resourceGroup().location]"
	//			}
	//		}
	//	}
	recipeData := make(map[string]any)
	err := util.ReadFromRegistry(ctx, opts.Definition.TemplatePath, &recipeData)
	if err != nil {
		return nil, err
	}

	return recipeData, nil
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

	resourceGroup := parsed.FindScope(resources_radius.ScopeResourceGroups)
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

// getGCOutputResources [GC stands for Garbage Collection] compares two slices of resource ids and
// returns a slice of resource ids that contains the elements that are in the "previous" slice but not in the "current".
func (d *bicepDriver) getGCOutputResources(current []string, previous []string) []string {
	// We can easily determine which resources have changed via a brute-force search comparing IDs.
	// The lists of resources we work with are small, so this is fine.
	diff := []string{}
	for _, prevResource := range previous {
		found := false
		for _, currentResource := range current {
			if prevResource == currentResource {
				found = true
				break
			}
		}

		if !found {
			diff = append(diff, prevResource)
		}
	}

	return diff
}

func (d *bicepDriver) deleteGCOutputResources(ctx context.Context, diff []string) error {
	logger := ucplog.FromContextOrDiscard(ctx)
	for _, resource := range diff {
		logger.Info(fmt.Sprintf("Deleting output resource: %s", resource), ucplog.LogFieldTargetResourceID, resource)
		err := d.ResourceClient.Delete(ctx, resource)
		if err != nil {
			return err
		}
		logger.Info(fmt.Sprintf("Deleted output resource: %s", resource), ucplog.LogFieldTargetResourceID, resource)
	}

	return nil
}
