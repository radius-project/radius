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
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
	"github.com/go-logr/logr"
	"golang.org/x/sync/errgroup"
	"oras.land/oras-go/v2/registry/remote"

	coredm "github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/metrics"
	"github.com/radius-project/radius/pkg/portableresources/datamodel"
	"github.com/radius-project/radius/pkg/portableresources/processors"
	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/recipes/recipecontext"
	recipes_util "github.com/radius-project/radius/pkg/recipes/util"
	"github.com/radius-project/radius/pkg/rp/util"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	"github.com/radius-project/radius/pkg/sdk/clients"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/pkg/ucp/resources"
	resources_radius "github.com/radius-project/radius/pkg/ucp/resources/radius"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
)

//go:generate mockgen -destination=./mock_driver.go -package=driver -self_package github.com/radius-project/radius/pkg/recipes/driver github.com/radius-project/radius/pkg/recipes/driver Driver
const (
	deploymentPrefix = "recipe"
	pollFrequency    = time.Second * 5
	recipeParameters = "parameters"
)

var _ Driver = (*bicepDriver)(nil)

// NewBicepDriver creates a new bicep driver instance with the given ARM client options, deployment client, resource client, and options.
func NewBicepDriver(armOptions *arm.ClientOptions, deploymentClient *clients.ResourceDeploymentsClient, client processors.ResourceClient, options BicepOptions) Driver {
	return &bicepDriver{
		ArmClientOptions: armOptions,
		DeploymentClient: deploymentClient,
		ResourceClient:   client,
		options:          options,
	}
}

type BicepOptions struct {
	DeleteRetryCount        int
	DeleteRetryDelaySeconds int
}

type bicepDriver struct {
	ArmClientOptions *arm.ClientOptions
	DeploymentClient *clients.ResourceDeploymentsClient
	ResourceClient   processors.ResourceClient
	options          BicepOptions

	// RegistryClient is the optional client used to interact with the container registry.
	RegistryClient remote.Client
}

// Execute fetches recipe contents from container registry, creates a deployment ID, a recipe context parameter, recipe parameters,
// a provider config, and deploys a bicep template for the recipe using UCP deployment client, then polls until the deployment
// is done and prepares the recipe response.
func (d *bicepDriver) Execute(ctx context.Context, opts ExecuteOptions) (*recipes.RecipeOutput, error) {
	logger := logr.FromContextOrDiscard(ctx)
	logger.Info(fmt.Sprintf("Deploying recipe: %q, template: %q", opts.Definition.Name, opts.Definition.TemplatePath))

	recipeData := make(map[string]any)
	downloadStartTime := time.Now()

	err := util.ReadFromRegistry(ctx, opts.Definition, &recipeData, d.RegistryClient)
	if err != nil {
		metrics.DefaultRecipeEngineMetrics.RecordRecipeDownloadDuration(ctx, downloadStartTime,
			metrics.NewRecipeAttributes(metrics.RecipeEngineOperationDownloadRecipe, opts.Recipe.Name, &opts.Definition, recipes.RecipeDownloadFailed))
		return nil, recipes.NewRecipeError(recipes.RecipeDownloadFailed, err.Error(), recipes_util.RecipeSetupError, recipes.GetErrorDetails(err))
	}
	metrics.DefaultRecipeEngineMetrics.RecordRecipeDownloadDuration(ctx, downloadStartTime,
		metrics.NewRecipeAttributes(metrics.RecipeEngineOperationDownloadRecipe, opts.Recipe.Name, &opts.Definition, metrics.SuccessfulOperationState))

	// create the context object to be passed to the recipe deployment
	recipeContext, err := recipecontext.New(&opts.Recipe, &opts.Configuration)
	if err != nil {
		return nil, recipes.NewRecipeError(recipes.RecipeDeploymentFailed, err.Error(), recipes_util.RecipeSetupError, recipes.GetErrorDetails(err))
	}

	// get the parameters after resolving the conflict between developer and operator parameters
	// if the recipe template also has the context parameter defined then add it to the parameter for deployment
	isContextParameterDefined := hasContextParameter(recipeData)
	parameters := createRecipeParameters(opts.Recipe.Parameters, opts.Definition.Parameters, isContextParameterDefined, recipeContext)

	deploymentName := deploymentPrefix + strconv.FormatInt(time.Now().UnixNano(), 10)
	deploymentID, err := createDeploymentID(recipeContext.Resource.ID, deploymentName)
	if err != nil {
		return nil, recipes.NewRecipeError(recipes.RecipeDeploymentFailed, err.Error(), recipes_util.RecipeSetupError, recipes.GetErrorDetails(err))
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
				Mode:           armresources.DeploymentModeIncremental,
				ProviderConfig: &providerConfig,
				Parameters:     parameters,
				Template:       recipeData,
			},
		},
		deploymentID.String(),
		clients.DeploymentsClientAPIVersion,
	)

	if err != nil {
		return nil, recipes.NewRecipeError(recipes.RecipeDeploymentFailed, fmt.Sprintf("failed to deploy recipe %s of type %s", opts.BaseOptions.Recipe.Name, opts.BaseOptions.Definition.ResourceType), recipes_util.ExecutionError, recipes.GetErrorDetails(err))
	}

	resp, err := poller.PollUntilDone(ctx, &runtime.PollUntilDoneOptions{Frequency: pollFrequency})
	if err != nil {
		return nil, recipes.NewRecipeError(recipes.RecipeDeploymentFailed, fmt.Sprintf("failed to deploy recipe %s of type %s", opts.BaseOptions.Recipe.Name, opts.BaseOptions.Definition.ResourceType), recipes_util.ExecutionError, recipes.GetErrorDetails(err))
	}

	recipeResponse, err := d.prepareRecipeResponse(opts.BaseOptions.Definition.TemplatePath, resp.Properties.Outputs, resp.Properties.OutputResources)
	if err != nil {
		return nil, recipes.NewRecipeError(recipes.InvalidRecipeOutputs, fmt.Sprintf("failed to read the recipe output %q: %s", recipes.ResultPropertyName, err.Error()), recipes_util.ExecutionError, recipes.GetErrorDetails(err))
	}

	// When a Radius portable resource consuming a recipe is redeployed, Garbage collection of the recipe resources that aren't included
	// in the currently deployed resources compared to the list of resources from the previous deployment needs to be deleted
	// as bicep does not take care of automatically deleting the unused resources.
	// Identify the output resources that are no longer relevant to the recipe.
	garbageCollectionStartTime := time.Now()
	diff, err := d.getGCOutputResources(recipeResponse.Resources, opts.PrevState)
	if err != nil {
		return nil, err
	}

	// Deleting obsolete output resources.
	err = d.Delete(ctx, DeleteOptions{
		OutputResources: diff,
	})
	if err != nil {
		metrics.DefaultRecipeEngineMetrics.RecordRecipeGarbageCollectionDuration(ctx, garbageCollectionStartTime,
			metrics.NewRecipeAttributes(metrics.RecipeEngineOperationGC, opts.Recipe.Name, &opts.Definition, metrics.FailedOperationState))
		return nil, recipes.NewRecipeError(recipes.RecipeGarbageCollectionFailed, err.Error(), recipes_util.ExecutionError, nil)
	}
	metrics.DefaultRecipeEngineMetrics.RecordRecipeGarbageCollectionDuration(ctx, garbageCollectionStartTime,
		metrics.NewRecipeAttributes(metrics.RecipeEngineOperationGC, opts.Recipe.Name, &opts.Definition, metrics.SuccessfulOperationState))
	return recipeResponse, nil
}

// Delete deletes all of the output resources that are marked as managed by Radius.
// It will create a goroutine for each resource to be deleted and wait for them to finish,
// retrying if necessary.
// We don't have context on the dependency ordering here, so we need to try to delete them
// all in parallel. Since some resources may depend on others, we may need to retry.
func (d *bicepDriver) Delete(ctx context.Context, opts DeleteOptions) error {
	logger := ucplog.FromContextOrDiscard(ctx)

	// Create a waitgroup to track the deletion of each output resource
	g, groupCtx := errgroup.WithContext(ctx)

	for i := range opts.OutputResources {
		outputResource := opts.OutputResources[i]

		// Create a goroutine that handles the deletion of one resource
		g.Go(func() error {
			id := outputResource.ID.String()
			logger.V(ucplog.LevelInfo).Info(fmt.Sprintf("Deleting output resource: %v, LocalID: %s, resource type: %s\n", outputResource.ID, outputResource.LocalID, outputResource.GetResourceType()))

			// If the resource is not managed by Radius, skip the deletion
			if outputResource.RadiusManaged == nil || !*outputResource.RadiusManaged {
				logger.Info(fmt.Sprintf("Skipping deletion of output resource: %q, not managed by Radius", id))
				return nil
			}

			var err error
			for attempt := 0; attempt <= d.options.DeleteRetryCount; attempt++ {
				logger.WithValues("attempt", attempt)
				ctx := logr.NewContext(groupCtx, logger)
				logger.V(ucplog.LevelDebug).Info("beginning attempt")

				err = d.ResourceClient.Delete(ctx, id)
				if err != nil {
					if attempt <= d.options.DeleteRetryCount {
						logger.V(ucplog.LevelInfo).Error(err, "attempt failed", "delay", d.options.DeleteRetryDelaySeconds)
						time.Sleep(time.Duration(d.options.DeleteRetryDelaySeconds) * time.Second)
						continue
					}

					return recipes.NewRecipeError(recipes.RecipeDeletionFailed, err.Error(), "", recipes.GetErrorDetails(err))
				}

				// If the err is nil, then the resource is deleted successfully
				logger.V(ucplog.LevelInfo).Info(fmt.Sprintf("Deleted output resource: %q", id))
				return nil
			}

			deletionErr := fmt.Errorf("failed to delete resource after %d attempt(s), last error: %s", d.options.DeleteRetryCount+1, err.Error())
			return recipes.NewRecipeError(recipes.RecipeDeletionFailed, deletionErr.Error(), "", recipes.GetErrorDetails(deletionErr))
		})
	}

	if err := g.Wait(); err != nil {
		return err
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
	err := util.ReadFromRegistry(ctx, opts.Definition, &recipeData, d.RegistryClient)
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
func (d *bicepDriver) prepareRecipeResponse(templatePath string, outputs any, resources []*armresources.ResourceReference) (*recipes.RecipeOutput, error) {
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

	recipeResponse.Status = &rpv1.RecipeStatus{
		TemplateKind: recipes.TemplateKindBicep,
		TemplatePath: templatePath,
	}

	// process the 'resources' created by the template
	for _, id := range resources {
		recipeResponse.Resources = append(recipeResponse.Resources, *id.ID)
	}

	return recipeResponse, nil
}

// getGCOutputResources [GC stands for Garbage Collection] compares two slices of resource ids and
// returns a slice of OutputResources that contains the elements that are in the "previous" slice but not in the "current".
func (d *bicepDriver) getGCOutputResources(current []string, previous []string) ([]rpv1.OutputResource, error) {
	// We can easily determine which resources have changed via a brute-force search comparing IDs.
	// The lists of resources we work with are small, so this is fine.
	diff := []rpv1.OutputResource{}
	for _, prevResourceId := range previous {
		found := false
		for _, currentResourceId := range current {
			if prevResourceId == currentResourceId {
				found = true
				break
			}
		}

		if !found {
			id, err := resources.Parse(prevResourceId)
			if err != nil {
				return nil, recipes.NewRecipeError(recipes.RecipeGarbageCollectionFailed, err.Error(), recipes_util.ExecutionError, nil)
			}

			diff = append(diff, rpv1.OutputResource{
				ID:            id,
				RadiusManaged: to.Ptr(true),
			})
		}
	}

	return diff, nil
}
