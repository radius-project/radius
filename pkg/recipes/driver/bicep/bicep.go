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

package bicep

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	reflect "reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armdeployments/v3"
	awsarn "github.com/aws/aws-sdk-go-v2/aws/arn"
	"github.com/go-logr/logr"
	"oras.land/oras-go/v2/registry/remote"

	"github.com/radius-project/radius/pkg/aws/arnproperties"
	"github.com/radius-project/radius/pkg/components/metrics"
	coredm "github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/portableresources/datamodel"
	"github.com/radius-project/radius/pkg/portableresources/processors"
	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/recipes/driver"
	"github.com/radius-project/radius/pkg/recipes/kubernetes/clusteraccess"
	"github.com/radius-project/radius/pkg/recipes/paramresolver"
	"github.com/radius-project/radius/pkg/recipes/recipecontext"
	recipes_util "github.com/radius-project/radius/pkg/recipes/util"
	"github.com/radius-project/radius/pkg/resourcemodel"
	"github.com/radius-project/radius/pkg/rp/util"
	"github.com/radius-project/radius/pkg/rp/util/authclient"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	"github.com/radius-project/radius/pkg/sdk/clients"
	"github.com/radius-project/radius/pkg/ucp/resources"
	resources_aws "github.com/radius-project/radius/pkg/ucp/resources/aws"
	resources_radius "github.com/radius-project/radius/pkg/ucp/resources/radius"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/rest"
)

const (
	deploymentPrefix = "recipe"
	pollFrequency    = time.Second * 5
	recipeParameters = "parameters"
	recipeOutputs    = "outputs"
)

var _ driver.Driver = (*bicepDriver)(nil)

// NewBicepDriver creates a new bicep driver instance with the given ARM client options, deployment client, resource client factory, and options.
func NewBicepDriver(armOptions *arm.ClientOptions, deploymentClient clients.ResourceDeploymentsClient, resourceClientFactory processors.ResourceClientFactory, options BicepOptions) driver.Driver {
	return &bicepDriver{
		ArmClientOptions:      armOptions,
		DeploymentClient:      deploymentClient,
		resourceClientFactory: resourceClientFactory,
		options:               options,
		clusterAccessResolver: clusteraccess.NewResolver(),
	}
}

type BicepOptions struct {
	DeleteRetryCount        int
	DeleteRetryDelaySeconds int
}

type bicepDriver struct {
	ArmClientOptions      *arm.ClientOptions
	DeploymentClient      clients.ResourceDeploymentsClient
	options               BicepOptions
	clusterAccessResolver clusteraccess.ClusterAccessResolver
	resourceClientFactory processors.ResourceClientFactory

	// RegistryClient is the optional client used to interact with the container registry.
	RegistryClient remote.Client
}

// Execute fetches recipe contents from container registry, creates a deployment ID, a recipe context parameter, recipe parameters,
// a provider config, and deploys a bicep template for the recipe using UCP deployment client, then polls until the deployment
// is done and prepares the recipe response.
func (d *bicepDriver) Execute(ctx context.Context, opts driver.ExecuteOptions) (*recipes.RecipeOutput, error) {
	logger := logr.FromContextOrDiscard(ctx)
	logger.Info(fmt.Sprintf("Deploying recipe: %q, template: %q", opts.Definition.Name, opts.Definition.TemplatePath))

	recipeData := make(map[string]any)
	downloadStartTime := time.Now()
	secrets := util.GetRegistrySecrets(opts.Configuration, opts.Definition.TemplatePath, opts.Secrets)

	registryClient := d.RegistryClient
	// Get ORAS authentication client if secrets are found for the registry.
	if !reflect.DeepEqual(secrets, recipes.SecretData{}) {
		authClient, err := getRegistryAuthClient(ctx, secrets, opts.Definition.TemplatePath)
		if err != nil {
			return nil, err
		}

		registryClient = authClient
	}

	err := util.ReadFromRegistry(ctx, opts.Definition, &recipeData, registryClient)
	if err != nil {
		metrics.DefaultRecipeEngineMetrics.RecordRecipeDownloadDuration(ctx, downloadStartTime,
			metrics.NewRecipeAttributes(metrics.RecipeEngineOperationDownloadRecipe, opts.Recipe.Name, &opts.Definition, recipes.RecipeDownloadFailed))
		return nil, recipes.NewRecipeError(recipes.RecipeDownloadFailed, err.Error(), recipes_util.RecipeSetupError, recipes.GetErrorDetails(err))
	}
	metrics.DefaultRecipeEngineMetrics.RecordRecipeDownloadDuration(ctx, downloadStartTime,
		metrics.NewRecipeAttributes(metrics.RecipeEngineOperationDownloadRecipe, opts.Recipe.Name, &opts.Definition, metrics.SuccessfulOperationState))

	if err := validateOutputMappings(opts.Definition, recipeData); err != nil {
		return nil, recipes.NewRecipeError(recipes.InvalidRecipeOutputs, err.Error(), recipes_util.RecipeSetupError, recipes.GetErrorDetails(err))
	}

	// create the context object to be passed to the recipe deployment
	recipeContext, err := recipecontext.New(&opts.Recipe, &opts.Configuration)
	if err != nil {
		return nil, recipes.NewRecipeError(recipes.RecipeDeploymentFailed, err.Error(), recipes_util.RecipeSetupError, recipes.GetErrorDetails(err))
	}

	//update the recipe context with connected resources properties
	recipeContext.Resource.Connections = opts.Recipe.ConnectedResourcesProperties

	// get the parameters after resolving the conflict between developer and operator parameters
	// if the recipe template also has the context parameter defined then add it to the parameter for deployment
	isContextParameterDefined := hasContextParameter(recipeData)

	var parameters map[string]any
	if isContextParameterDefined {
		// Wrapped recipe — use the existing context injection flow.
		parameters = createRecipeParameters(opts.Recipe.Parameters, opts.Definition.Parameters, true, recipeContext)
	} else {
		// Direct module — resolve {{context.*}} expressions, merge parameters, and wrap as ARM parameters.
		// Resource (developer) parameters take precedence over environment (operator) parameters.
		mergedParams := recipes_util.ShallowMergeParameters(opts.Definition.Parameters, opts.Recipe.Parameters)
		resolvedParams, err := paramresolver.ResolveParameterExpressions(mergedParams, recipeContext)
		if err != nil {
			// Report an execution error rather than a setup error so a later delete still cleans up
			// resources from an earlier successful deployment.
			return nil, recipes.NewRecipeError(recipes.RecipeDeploymentFailed, err.Error(), recipes_util.ExecutionError, recipes.GetErrorDetails(err))
		}
		parameters = wrapARMParameters(resolvedParams)
	}

	deploymentName := deploymentPrefix + strconv.FormatInt(time.Now().UnixNano(), 10)
	deploymentID, err := createDeploymentID(recipeContext.Resource.ID, deploymentName)
	if err != nil {
		return nil, recipes.NewRecipeError(recipes.RecipeDeploymentFailed, err.Error(), recipes_util.RecipeSetupError, recipes.GetErrorDetails(err))
	}

	// Provider config will specify the Azure and AWS scopes (if provided).
	providerConfig := newProviderConfig(deploymentID.FindScope(resources_radius.ScopeResourceGroups), opts.Configuration.Providers, awsOwner(opts.Recipe))

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
				Mode:           armdeployments.DeploymentModeIncremental,
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

	resp, err := poller.PollUntilDone(ctx, &clients.PollUntilDoneOptions{Frequency: pollFrequency})
	if err != nil {
		return nil, recipes.NewRecipeError(recipes.RecipeDeploymentFailed, fmt.Sprintf("failed to deploy recipe %s of type %s", opts.BaseOptions.Recipe.Name, opts.BaseOptions.Definition.ResourceType), recipes_util.ExecutionError, recipes.GetErrorDetails(err))
	}

	// Log only IDs, not output values, which can contain credentials.
	outputResourceIDs := make([]string, 0, len(resp.Properties.OutputResources))
	for _, output := range resp.Properties.OutputResources {
		if output != nil && output.ID != nil {
			outputResourceIDs = append(outputResourceIDs, *output.ID)
		}
	}
	logger.Info("Deployment engine returned recipe output resource IDs",
		"deploymentID", deploymentID.String(),
		"recipeName", opts.Definition.Name,
		"outputResourceCount", len(resp.Properties.OutputResources),
		"outputResourceIDs", outputResourceIDs)

	recipeResponse, err := d.prepareRecipeResponse(opts.BaseOptions.Definition, resp.Properties.Outputs, resp.Properties.OutputResources)
	if err != nil {
		if _, ok := errors.AsType[*recipes_util.MissingOutputValuesError](err); ok {
			return nil, recipes.NewRecipeError(recipes.InvalidRecipeOutputs, err.Error(), recipes_util.ExecutionError, recipes.GetErrorDetails(err))
		}
		return nil, recipes.NewRecipeError(recipes.InvalidRecipeOutputs, fmt.Sprintf("failed to read the recipe output %q: %s", recipes.ResultPropertyName, err.Error()), recipes_util.ExecutionError, recipes.GetErrorDetails(err))
	}

	d.addAWSProviderResourceIDs(ctx, recipeResponse)

	// containerImages recipes may declare an imageBuild output. Check for it first, then run the
	// embedded build script and merge the resulting image reference into the recipe output.
	shouldExecuteImageBuildHook, err := d.hasImageBuildProperty(opts.BaseOptions.Definition.ResourceType, resp.Properties.Outputs)
	if err != nil {
		return nil, recipes.NewRecipeError(recipes.RecipeDeploymentFailed, err.Error(), recipes_util.ExecutionError, recipes.GetErrorDetails(err))
	}
	if shouldExecuteImageBuildHook {
		if err := d.executeImageBuildHook(ctx, recipeData, resp.Properties.Outputs, recipeResponse, opts); err != nil {
			return nil, recipes.NewRecipeError(recipes.RecipeDeploymentFailed, err.Error(), recipes_util.ExecutionError, recipes.GetErrorDetails(err))
		}
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
	err = d.Delete(ctx, driver.DeleteOptions{
		BaseOptions:     opts.BaseOptions,
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
func (d *bicepDriver) Delete(ctx context.Context, opts driver.DeleteOptions) error {
	logger := ucplog.FromContextOrDiscard(ctx)

	resourceClient, err := d.resourceClientForDelete(ctx, opts)
	if err != nil {
		return err
	}
	if resourceClient == nil {
		return nil
	}

	// UCP deletes an AWS resource only for the owner that deployed it.
	ctx = processors.WithAWSOwner(ctx, awsOwner(opts.Recipe))

	// Each output resource gets its own delete attempts, so one failure does not stop the others.
	var wg sync.WaitGroup
	var mu sync.Mutex
	failures := []error{}

	for i := range opts.OutputResources {
		outputResource := opts.OutputResources[i]

		if outputResource.IsRadiusManaged() && deletingKeyVaultParent(outputResource.ID, opts.OutputResources) {
			// ARM cannot delete secrets. The batch must still wait for the managed vault deletion to succeed.
			logger.Info("Skipping direct Key Vault secret deletion; the managed parent vault is in the deletion batch", "resourceID", outputResource.ID.String())
			continue
		}

		// Create a goroutine that handles the deletion of one resource
		wg.Go(func() {
			id := outputResource.ID.String()
			logger.V(ucplog.LevelInfo).Info(fmt.Sprintf("Deleting output resource: %v, LocalID: %s, resource type: %s\n", outputResource.ID, outputResource.LocalID, outputResource.GetResourceType()))

			// If the resource is not managed by Radius, skip the deletion
			if outputResource.RadiusManaged == nil || !*outputResource.RadiusManaged {
				logger.Info(fmt.Sprintf("Skipping deletion of output resource: %q, not managed by Radius", id))
				return
			}

			var err error
			attempts := 0
			for {
				if err = ctx.Err(); err != nil {
					break
				}
				attempts++
				attemptLogger := logger.WithValues("attempt", attempts, "resourceID", id)
				attemptCtx := logr.NewContext(ctx, attemptLogger)
				attemptLogger.V(ucplog.LevelDebug).Info("beginning attempt")

				err = resourceClient.Delete(attemptCtx, id)
				if err == nil {
					attemptLogger.V(ucplog.LevelInfo).Info(fmt.Sprintf("Deleted output resource: %q", id))
					return
				}
				if attempts > d.options.DeleteRetryCount || !isRetryableDeleteError(err) {
					break
				}

				attemptLogger.V(ucplog.LevelInfo).Error(err, "attempt failed", "delay", d.options.DeleteRetryDelaySeconds)
				timer := time.NewTimer(time.Duration(d.options.DeleteRetryDelaySeconds) * time.Second)
				select {
				case <-ctx.Done():
					err = ctx.Err()
				case <-timer.C:
				}
				timer.Stop()
				if ctx.Err() != nil {
					break
				}
			}

			mu.Lock()
			defer mu.Unlock()
			failures = append(failures, fmt.Errorf("failed to delete resource %q after %d attempt(s), last error: %w", id, attempts, err))
		})
	}
	wg.Wait()

	if len(failures) == 0 {
		return nil
	}

	// Sort so the error is the same no matter which delete finished first.
	slices.SortFunc(failures, func(a, b error) int { return strings.Compare(a.Error(), b.Error()) })
	deletionErr := errors.Join(failures...)
	return recipes.NewRecipeError(recipes.RecipeDeletionFailed, deletionErr.Error(), "", recipes.GetErrorDetails(deletionErr))
}

// permanentDeleteErrorCodes are ARM error codes that a later attempt cannot fix.
var permanentDeleteErrorCodes = map[string]struct{}{
	"authenticationfailed":             {},
	"authorizationfailed":              {},
	"invalidauthenticationtoken":       {},
	"invalidauthenticationtokentenant": {},
	"linkedauthorizationfailed":        {},
}

// isRetryableDeleteError reports whether another delete attempt can succeed.
// Deletes run in parallel without dependency ordering, and dependency failures
// arrive with many status codes (for example 400 for Azure "in use" errors, or 200
// for a failed long-running operation). Only errors that are known to be permanent stop retries.
func isRetryableDeleteError(err error) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}

	if responseError, ok := errors.AsType[*azcore.ResponseError](err); ok {
		if _, permanent := permanentDeleteErrorCodes[strings.ToLower(responseError.ErrorCode)]; permanent {
			return false
		}
		return !isAuthStatus(responseError.StatusCode)
	}

	var statusError apierrors.APIStatus
	if errors.As(err, &statusError) {
		return !isAuthStatus(int(statusError.Status().Code))
	}

	return true
}

func isAuthStatus(statusCode int) bool {
	return statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden
}

func deletingKeyVaultParent(id resources.ID, outputResources []rpv1.OutputResource) bool {
	if !strings.EqualFold(id.Type(), "Microsoft.KeyVault/vaults/secrets") || len(id.ExtensionSegments()) != 0 {
		return false
	}
	if id.IsUCPQualified() && !strings.HasPrefix(strings.ToLower(id.PlaneNamespace()), "azure/") {
		return false
	}

	parentID := azureResourcePath(id.Truncate())
	for _, output := range outputResources {
		if output.IsRadiusManaged() && strings.EqualFold(azureResourcePath(output.ID), parentID) {
			return true
		}
	}

	return false
}

func azureResourcePath(id resources.ID) string {
	if !id.IsUCPQualified() {
		return id.String()
	}
	if !strings.HasPrefix(strings.ToLower(id.PlaneNamespace()), "azure/") {
		return ""
	}

	// Match the ARM path used by resourceClient.deleteAzureResource, not an unrelated plane's path.
	return resources.MakeRelativeID(id.ScopeSegments()[1:], id.TypeSegments(), id.ExtensionSegments())
}

func (d *bicepDriver) resourceClientForDelete(ctx context.Context, opts driver.DeleteOptions) (processors.ResourceClient, error) {
	hasManagedOutput := false
	hasManagedKubernetesOutput := false
	for _, outputResource := range opts.OutputResources {
		if !outputResource.IsRadiusManaged() {
			continue
		}

		hasManagedOutput = true
		if strings.EqualFold(outputResource.GetResourceType().Provider, resourcemodel.ProviderKubernetes) {
			hasManagedKubernetesOutput = true
		}
	}

	if !hasManagedOutput {
		return nil, nil
	}

	if d.resourceClientFactory == nil {
		err := errors.New("bicep driver has no resource client factory configured")
		return nil, recipes.NewRecipeError(recipes.RecipeDeletionFailed, err.Error(), "", recipes.GetErrorDetails(err))
	}

	var kubernetesConfig *rest.Config
	if hasManagedKubernetesOutput {
		if d.clusterAccessResolver == nil {
			err := errors.New("bicep driver has no cluster access resolver configured")
			return nil, recipes.NewRecipeError(recipes.RecipeDeletionFailed, err.Error(), "", recipes.GetErrorDetails(err))
		}

		var err error
		kubernetesConfig, err = d.clusterAccessResolver.Resolve(ctx, &opts.Configuration)
		if err != nil {
			err = fmt.Errorf("failed to resolve target cluster for recipe output deletion: %w", err)
			return nil, recipes.NewRecipeError(recipes.RecipeDeletionFailed, err.Error(), "", recipes.GetErrorDetails(err))
		}
		if kubernetesConfig == nil {
			err = errors.New("cluster access resolver returned a nil Kubernetes configuration")
			return nil, recipes.NewRecipeError(recipes.RecipeDeletionFailed, err.Error(), "", recipes.GetErrorDetails(err))
		}

		ucplog.FromContextOrDiscard(ctx).Info("Resolved Kubernetes target for recipe output deletion", "host", kubernetesConfig.Host)
	}

	resourceClient := d.resourceClientFactory(kubernetesConfig)
	if resourceClient == nil {
		err := errors.New("bicep resource client factory returned nil")
		return nil, recipes.NewRecipeError(recipes.RecipeDeletionFailed, err.Error(), "", recipes.GetErrorDetails(err))
	}

	return resourceClient, nil
}

// GetRecipeMetadata gets the Bicep recipe parameters information from the container registry
func (d *bicepDriver) GetRecipeMetadata(ctx context.Context, opts driver.BaseOptions) (map[string]any, error) {
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
	secrets := util.GetRegistrySecrets(opts.Configuration, opts.Definition.TemplatePath, opts.Secrets)

	registryClient := d.RegistryClient
	// Get ORAS authentication client if secrets are found for the registry.
	if !reflect.DeepEqual(secrets, recipes.SecretData{}) {
		authClient, err := getRegistryAuthClient(ctx, secrets, opts.Definition.TemplatePath)
		if err != nil {
			return nil, err
		}

		registryClient = authClient
	}

	err := util.ReadFromRegistry(ctx, opts.Definition, &recipeData, registryClient)
	if err != nil {
		return nil, err
	}

	return recipeData, nil
}

func validateOutputMappings(definition recipes.EnvironmentDefinition, recipeData map[string]any) error {
	if len(definition.Outputs) == 0 && len(definition.SecretOutputs) == 0 {
		return nil
	}

	var declaredOutputs []string
	rawOutputs, ok := recipeData[recipeOutputs]
	if ok && rawOutputs != nil {
		outputs, ok := rawOutputs.(map[string]any)
		if !ok {
			return fmt.Errorf(
				"recipe %q for resource type %q: recipe outputs must be an object",
				definition.Name,
				definition.ResourceType)
		}

		declaredOutputs = make([]string, 0, len(outputs))
		for outputName := range outputs {
			declaredOutputs = append(declaredOutputs, outputName)
		}
	}

	return recipes_util.ValidateOutputsMapping(
		definition.Name,
		definition.ResourceType,
		declaredOutputs,
		definition.Outputs,
		definition.SecretOutputs)
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

// awsOwner returns the owner of the AWS resources a recipe deploys.
func awsOwner(recipe recipes.ResourceMetadata) string {
	return clients.AWSRecipeOwner(recipe.ApplicationID, recipe.EnvironmentID, recipe.ResourceID)
}

func newProviderConfig(resourceGroup string, envProviders coredm.Providers, owner string) clients.ProviderConfig {
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
				Owner: owner,
			},
		}
	}

	return config
}

// prepareRecipeResponse populates the recipe response from the deployment outputs and the
// resources created by the template.
//
// Output resolution precedence (direct module support):
//   - If an `outputs` mapping is configured on the definition, it takes precedence: all ARM
//     deployment outputs are collected flat and then renamed via the mapping.
//   - Otherwise, if the deployment produced a `result` output, the module is treated as a
//     wrapped recipe (existing behavior).
//   - Otherwise, all ARM deployment outputs pass through unchanged.
//
// Note that there are two ways a resource can be returned:
//   - Implicitly when it is created in the template (it will be in 'resources').
//   - Explicitly as part of the 'result' output.
//
// The latter is needed because non-ARM and non-UCP resources are not returned as part of the implicit 'resources'
// collection. For us this mostly means Kubernetes resources - the user has to be explicit.
func (d *bicepDriver) prepareRecipeResponse(definition recipes.EnvironmentDefinition, outputs any, resources []*armdeployments.ResourceReference) (*recipes.RecipeOutput, error) {
	recipeResponse := &recipes.RecipeOutput{}
	out, _ := outputs.(map[string]any)
	hasOutputsMapping := len(definition.Outputs) > 0 || len(definition.SecretOutputs) > 0
	_, hasResultOutput := out[recipes.ResultPropertyName]

	switch {
	case hasOutputsMapping:
		// Direct module with an outputs mapping — collect all ARM outputs flat (splitting
		// secure-typed outputs into secrets), then apply the mapping.
		values, secrets := collectARMOutputs(out)
		mappedValues, mappedSecrets, err := recipes_util.ApplyOutputsMapping(values, secrets, definition.Outputs, definition.SecretOutputs)
		if err != nil {
			return &recipes.RecipeOutput{}, fmt.Errorf("recipe %q for resource type %q: %w", definition.Name, definition.ResourceType, err)
		}
		recipeResponse.Values = mappedValues
		recipeResponse.Secrets = mappedSecrets
	case len(out) > 0 && hasResultOutput:
		// Wrapped recipe — use the existing 'result' output parsing.
		if result, ok := out[recipes.ResultPropertyName].(map[string]any); ok {
			if resultValue, ok := result["value"].(map[string]any); ok {
				err := recipeResponse.PrepareRecipeResponse(resultValue)
				if err != nil {
					return &recipes.RecipeOutput{}, err
				}
			}
		}
	case len(out) > 0:
		// Direct module without a mapping — pass through all ARM outputs unchanged, routing
		// secure-typed outputs to Secrets so they are not exposed as plain values.
		recipeResponse.Values, recipeResponse.Secrets = collectARMOutputs(out)
	}

	recipeResponse.Status = &rpv1.RecipeStatus{
		TemplateKind: recipes.TemplateKindBicep,
		TemplatePath: definition.TemplatePath,
	}

	// process the 'resources' created by the template
	for _, id := range resources {
		recipeResponse.Resources = append(recipeResponse.Resources, *id.ID)
	}

	return recipeResponse, nil
}

// addAWSProviderResourceIDs records the ARN of each AWS output resource as its provider resource ID. This covers
// resources the recipe deploys and resources it returns in result.resources, including existing resources.
//
// The ARN is read through UCP from the property that holds the resource's own ARN for its type. Types that have no
// such property are skipped. For types that aren't in the catalog, the Arn, ARN, or arn property is used.
//
// It is best effort: when the ARN can't be determined, the output resource is stored without it.
func (d *bicepDriver) addAWSProviderResourceIDs(ctx context.Context, output *recipes.RecipeOutput) {
	logger := ucplog.FromContextOrDiscard(ctx)

	var resourceClient processors.ResourceClient
	for _, resource := range output.Resources {
		id, err := resources.ParseResource(resource)
		if _, ok := output.ProviderResourceIDs[resource]; err != nil || ok || !isAWSResource(id) {
			continue
		}

		property, known := arnproperties.Lookup(resources_aws.ToAWSResourceType(id))
		if known && property == "" {
			continue
		}

		if resourceClient == nil {
			if d.resourceClientFactory == nil {
				return
			}
			resourceClient = d.resourceClientFactory(nil)
			if resourceClient == nil {
				return
			}
		}

		properties, err := resourceClient.GetProperties(ctx, resource)
		if err != nil {
			logger.Info("Could not read the AWS output resource to find its ARN", "resourceID", resource, "error", err.Error())
			continue
		}

		if arn := arnFromProperties(properties, property); arn != "" {
			output.SetProviderResourceID(resource, arnproperties.ProviderResourceID(arn))
		}
	}
}

// arnFromProperties returns the ARN in the given property of an AWS resource, or in its Arn, ARN, or arn property when
// property is empty. It returns an empty string when the value is missing or isn't an ARN.
func arnFromProperties(properties map[string]any, property string) string {
	candidates := []string{property}
	if property == "" {
		candidates = []string{"Arn", "ARN", "arn"}
	}
	for _, key := range candidates {
		if arn, ok := properties[key].(string); ok && awsarn.IsARN(arn) {
			return arn
		}
	}

	return ""
}

func isAWSResource(id resources.ID) bool {
	return id.IsUCPQualified() && strings.HasPrefix(strings.ToLower(id.PlaneNamespace()), resources_aws.PlaneTypeAWS+"/")
}

// collectARMOutputs splits ARM deployment outputs into non-sensitive values and sensitive secrets
// based on each output's declared ARM type. Outputs typed securestring/secureobject are routed to
// secrets so that secure Bicep module outputs are not exposed as plain values, mirroring the
// Terraform driver which preserves per-output sensitivity.
// ARM outputs have the shape: {"outputName": {"type": "...", "value": ...}, ...}.
func collectARMOutputs(outputs map[string]any) (map[string]any, map[string]any) {
	values := make(map[string]any)
	secrets := make(map[string]any)
	for name, raw := range outputs {
		outputMap, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		val, ok := outputMap["value"]
		if !ok {
			continue
		}
		if isSecureARMOutputType(outputMap["type"]) {
			secrets[name] = val
		} else {
			values[name] = val
		}
	}
	return values, secrets
}

// isSecureARMOutputType reports whether an ARM output's declared type marks it as sensitive
// (securestring or secureobject). The comparison is case-insensitive because ARM may return the
// type with different casing than the compiled template.
func isSecureARMOutputType(outputType any) bool {
	typeStr, ok := outputType.(string)
	if !ok {
		return false
	}
	switch strings.ToLower(typeStr) {
	case "securestring", "secureobject":
		return true
	default:
		return false
	}
}

// wrapARMParameters wraps raw parameter values into the ARM parameter format: {"key": {"value": val}}.
func wrapARMParameters(params map[string]any) map[string]any {
	wrapped := make(map[string]any, len(params))
	for k, v := range params {
		// Skip nil values so we don't emit an explicit ARM "value": null, which can
		// override a module parameter's default or fail ARM type validation. This
		// mirrors the Terraform driver's SetParams behavior.
		if v == nil {
			continue
		}
		wrapped[k] = map[string]any{
			"value": v,
		}
	}
	return wrapped
}

// getGCOutputResources [GC stands for Garbage Collection] compares two slices of resource ids and
// returns a slice of OutputResources that contains the elements that are in the "previous" slice but not in the "current".
func (d *bicepDriver) getGCOutputResources(current []string, previous []string) ([]rpv1.OutputResource, error) {
	// We can easily determine which resources have changed via a brute-force search comparing IDs.
	// The lists of resources we work with are small, so this is fine.
	diff := []rpv1.OutputResource{}
	for _, prevResourceId := range previous {
		found := slices.Contains(current, prevResourceId)

		if !found {
			id, err := resources.Parse(prevResourceId)
			if err != nil {
				return nil, recipes.NewRecipeError(recipes.RecipeGarbageCollectionFailed, err.Error(), recipes_util.ExecutionError, nil)
			}

			diff = append(diff, rpv1.OutputResource{
				ID:            id,
				RadiusManaged: new(!resources_aws.IsReadOnlyResource(id)),
			})
		}
	}

	return diff, nil
}

func (d *bicepDriver) FindSecretIDs(ctx context.Context, envConfig recipes.Configuration, definition recipes.EnvironmentDefinition) (secretStoreIDResourceKeys map[string][]string, err error) {
	secretStoreIDResourceKeys = make(map[string][]string)
	if envConfig.RecipeConfig.Bicep.Authentication != nil {
		for _, v := range envConfig.RecipeConfig.Bicep.Authentication {
			secretStoreIDResourceKeys[v.Secret] = []string{}
		}
	}

	return secretStoreIDResourceKeys, err
}

func getRegistryAuthClient(ctx context.Context, secrets recipes.SecretData, templatePath string) (remote.Client, error) {
	newRegistryClient, err := authclient.GetNewRegistryAuthClient(secrets)
	if err != nil {
		return nil, err
	}

	return newRegistryClient.GetAuthClient(ctx, templatePath)
}
