// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
	"github.com/go-logr/logr"
	dockerParser "github.com/novln/docker-parser"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/azure/armauth"
	"github.com/project-radius/radius/pkg/azure/clientv2"
	coreDatamodel "github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/pkg/logging"
	"oras.land/oras-go/v2/content"
	"oras.land/oras-go/v2/registry/remote"
)

const deploymentPrefix = "recipe"

// RecipeHandler is an interface to deploy and delete recipe resources
//
//go:generate mockgen -destination=./mock_recipe_handler.go -package=handlers -self_package github.com/project-radius/radius/pkg/linkrp/handlers github.com/project-radius/radius/pkg/linkrp/handlers RecipeHandler
type RecipeHandler interface {
	DeployRecipe(ctx context.Context, recipe datamodel.RecipeProperties, envProviders coreDatamodel.Providers) ([]string, error)
}

func NewRecipeHandler(arm *armauth.ArmConfig) RecipeHandler {
	return &azureRecipeHandler{
		arm: arm,
	}
}

type azureRecipeHandler struct {
	arm *armauth.ArmConfig
}

// DeployRecipe deploys the recipe template fetched from the provided recipe TemplatePath using the providers scope.
// Currently the implementation assumes TemplatePath is location of an ARM JSON template in Azure Container Registry.
// Returns resource IDs of the resources deployed by the template
func (handler *azureRecipeHandler) DeployRecipe(ctx context.Context, recipe datamodel.RecipeProperties, envProviders coreDatamodel.Providers) (deployedResources []string, err error) {
	if recipe.TemplatePath == "" {
		return nil, fmt.Errorf("recipe template path cannot be empty")
	}
	if envProviders == (coreDatamodel.Providers{}) {
		return nil, v1.NewClientErrInvalidRequest(fmt.Sprintf("failed to deploy recipe %q. Environment provider scope is required to deploy link recipes.", recipe.Name))
	}
	subscriptionID, resourceGroup, err := parseAzureProvider(&envProviders)
	if err != nil {
		return nil, err
	}

	logger := logr.FromContextOrDiscard(ctx).WithValues(
		logging.LogFieldResourceGroup, resourceGroup,
		logging.LogFieldSubscriptionID, subscriptionID,
	)
	logger.Info(fmt.Sprintf("Deploying recipe: %q, template: %q", recipe.Name, recipe.TemplatePath))

	registryRepo, tag, err := parseTemplatePath(recipe.TemplatePath)
	if err != nil {
		return nil, v1.NewClientErrInvalidRequest(fmt.Sprintf("Invalid recipe templatePath %s", err.Error()))
	}

	// get the recipe from ACR
	// client to the ACR repository in the templatePath
	repo, err := remote.NewRepository(registryRepo)
	if err != nil {
		return nil, fmt.Errorf("failed to create client to registry %s", err.Error())
	}

	digest, err := getDigestFromManifest(ctx, repo, tag)
	if err != nil {
		return nil, v1.NewClientErrInvalidRequest(fmt.Sprintf("failed to fetch template from the path %q for recipe %q: %s", recipe.TemplatePath, recipe.Name, err.Error()))
	}

	recipeBytes, err := getRecipeBytes(ctx, repo, digest)
	if err != nil {
		return nil, v1.NewClientErrInvalidRequest(fmt.Sprintf("failed to fetch template from the path %q for recipe %q: %s", recipe.TemplatePath, recipe.Name, err.Error()))
	}

	recipeData := make(map[string]any)
	err = json.Unmarshal(recipeBytes, &recipeData)
	if err != nil {
		return nil, err
	}

	// get the parameters after resolving the conflict
	parameters := handleParameterConflict(recipe.Parameters, recipe.EnvParameters)

	// Using ARM deployment client to deploy ARM JSON template fetched from ACR
	client, err := clientv2.NewDeploymentsClient(subscriptionID, &handler.arm.ClientOptions)
	if err != nil {
		return nil, err
	}

	deploymentName := deploymentPrefix + strconv.FormatInt(time.Now().UnixNano(), 10)
	poller, err := client.BeginCreateOrUpdate(
		ctx,
		resourceGroup,
		deploymentName,
		armresources.Deployment{
			Properties: &armresources.DeploymentProperties{
				Template:   recipeData,
				Parameters: parameters,
				Mode:       to.Ptr(armresources.DeploymentModeIncremental),
			},
		},
		&armresources.DeploymentsClientBeginCreateOrUpdateOptions{},
	)
	if err != nil {
		return nil, err
	}

	resp, err := poller.PollUntilDone(ctx, nil)
	if err != nil {
		return nil, err
	}

	if *resp.Properties.ProvisioningState != armresources.ProvisioningStateSucceeded {
		return nil, fmt.Errorf("failed to deploy the recipe %q, template path: %q, deployment: %q", recipe.Name, recipe.TemplatePath, deploymentName)
	}

	for _, id := range resp.Properties.OutputResources {
		deployedResources = append(deployedResources, *id.ID)
	}

	return deployedResources, nil
}

// getDigestFromManifest gets the layers digest from the manifest
func getDigestFromManifest(ctx context.Context, repo *remote.Repository, tag string) (string, error) {
	// resolves a manifest descriptor with a Tag reference
	descriptor, err := repo.Resolve(ctx, tag)
	if err != nil {
		return "", err
	}
	// get the manifest data
	rc, err := repo.Fetch(ctx, descriptor)
	if err != nil {
		return "", err
	}
	defer rc.Close()
	manifestBlob, err := content.ReadAll(rc, descriptor)
	if err != nil {
		return "", err
	}
	// create the manifest map to get the digest of the layer
	var manifest map[string]any
	err = json.Unmarshal(manifestBlob, &manifest)
	if err != nil {
		return "", err
	}
	// get the layers digest to fetch the blob
	layer, ok := manifest["layers"].([]any)[0].(map[string]any)
	if !ok {
		return "", fmt.Errorf("failed to decode the layers from manifest")
	}
	layerDigest, ok := layer["digest"].(string)
	if !ok {
		return "", fmt.Errorf("failed to decode the layers digest from manifest")
	}
	return layerDigest, nil
}

// getRecipeBytes fetches the recipe ARM JSON using the layers digest
func getRecipeBytes(ctx context.Context, repo *remote.Repository, layerDigest string) ([]byte, error) {
	// resolves a layer blob descriptor with a digest reference
	descriptor, err := repo.Blobs().Resolve(ctx, layerDigest)
	if err != nil {
		return nil, err
	}
	// get the layer data
	rc, err := repo.Fetch(ctx, descriptor)
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	pulledBlob, err := content.ReadAll(rc, descriptor)
	if err != nil {
		return nil, err
	}
	return pulledBlob, nil
}

// parseAzureProvider parses the scope to get the subscriptionID and resourceGroup
func parseAzureProvider(providers *coreDatamodel.Providers) (subscriptionID string, resourceGroup string, err error) {
	if providers.Azure == (coreDatamodel.ProvidersAzure{}) {
		return "", "", v1.NewClientErrInvalidRequest("environment does not contain Azure provider scope required to deploy recipes on Azure")
	}
	// valid scope: "/subscriptions/test-sub/resourceGroups/test-group"
	scope := strings.Split(providers.Azure.Scope, "/")
	if len(scope) != 5 {
		return "", "", v1.NewClientErrInvalidRequest(fmt.Sprintf("invalid azure scope. Valid scope eg: %q", "/subscriptions/<subscriptionID>/resourceGroups/<resourceGroup>"))
	}
	subscriptionID = scope[2]
	resourceGroup = scope[4]
	if subscriptionID == "" || resourceGroup == "" {
		return "", "", v1.NewClientErrInvalidRequest("subscriptionID and resourceGroup must be provided to deploy link recipes to Azure")
	}
	return
}

// handleParameterConflict handles conflicts in parameters set by operator and developer
// In case of conflict the developer parameter takes precedence
func handleParameterConflict(devParams, operatorParams map[string]any) map[string]any {
	for k, v := range operatorParams {
		if _, ok := devParams[k]; !ok {
			devParams[k] = v
		}
	}
	parameters := map[string]any{}
	for k, v := range devParams {
		parameters[k] = map[string]any{
			"value": v,
		}
	}
	return parameters
}

func parseTemplatePath(templatePath string) (repository string, tag string, err error) {
	reference, err := dockerParser.Parse(templatePath)
	if err != nil {
		return "", "", err
	}
	repository = reference.Repository()
	tag = reference.Tag()
	return
}
