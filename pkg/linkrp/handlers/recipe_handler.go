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
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
	"github.com/go-logr/logr"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	aztoken "github.com/project-radius/radius/pkg/azure/tokencredentials"
	coreDatamodel "github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/linkrp"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/pkg/sdk"
	"github.com/project-radius/radius/pkg/sdk/clients"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"oras.land/oras-go/v2/content"
	"oras.land/oras-go/v2/registry/remote"
)

const (
	deploymentPrefix = "recipe"

	// pollFrequency is the polling frequency of the deployment client.
	// This is set to a relatively low number because we're using the UCP deployment engine
	// inside the cluster. This is a good balance to feel responsible for quick operations
	// like deploying Kubernetes resources without generating a wasteful amount of traffic.
	// The default would be 30 seconds.
	pollFrequency = time.Second * 5
)

// RecipeHandler is an interface to deploy and delete recipe resources
//
//go:generate mockgen -destination=./mock_recipe_handler.go -package=handlers -self_package github.com/project-radius/radius/pkg/linkrp/handlers github.com/project-radius/radius/pkg/linkrp/handlers RecipeHandler
type RecipeHandler interface {
	DeployRecipe(ctx context.Context, recipe linkrp.RecipeProperties, envProviders coreDatamodel.Providers, recipeContext linkrp.RecipeContext) (*RecipeResponse, error)
}

func NewRecipeHandler(connection sdk.Connection) RecipeHandler {
	return &recipeHandler{
		connection: connection,
	}
}

type recipeHandler struct {
	connection sdk.Connection
}

// DeployRecipe deploys the recipe template fetched from the provided recipe TemplatePath using the providers scope.
// Currently the implementation assumes TemplatePath is location of an ARM JSON template in Azure Container Registry.
// Returns resource IDs of the resources deployed by the template
func (handler *recipeHandler) DeployRecipe(ctx context.Context, recipe linkrp.RecipeProperties, envProviders coreDatamodel.Providers, recipeContext linkrp.RecipeContext) (*RecipeResponse, error) {
	if recipe.TemplatePath == "" {
		return nil, fmt.Errorf("recipe template path cannot be empty")
	}

	logger := logr.FromContextOrDiscard(ctx)
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
	recipeParam, ok := recipeData["parameters"].(map[string]any)
	// get the parameters after resolving the conflict between developer and operator parameters
	// if the recipe template also has the context parameter defined then add it to the parameter for deployment
	_, isContextParameterDefined := recipeData["parameters"].(map[string]interface{})[datamodel.RecipeContextParameter]
	parameters := createRecipeParameters(recipe.Parameters, recipe.EnvParameters, isContextParameterDefined, &recipeContext)

	// Using ARM deployment client to deploy ARM JSON template fetched from ACR
	client, err := clients.NewResourceDeploymentsClient(&clients.Options{
		ARMClientOptions: sdk.NewClientOptions(handler.connection),
		BaseURI:          handler.connection.Endpoint(),
		Cred:             &aztoken.AnonymousCredential{},
	})
	if err != nil {
		return nil, err
	}

	deploymentName := deploymentPrefix + strconv.FormatInt(time.Now().UnixNano(), 10)
	deploymentID, err := createDeploymentID(recipeContext.Resource.ID, deploymentName)
	if err != nil {
		return nil, err
	}

	// Provider config will specify the Azure and AWS scopes (if provided).
	providerConfig := createProviderConfig(deploymentID.FindScope(resources.ResourceGroupsSegment), envProviders)

	logger.Info("deploying bicep template for recipe", "deploymentID", deploymentID)
	if providerConfig.AWS != nil {
		logger.Info("using AWS provider", "deploymentID", deploymentID, "scope", providerConfig.AWS.Value.Scope)
	}
	if providerConfig.Az != nil {
		logger.Info("using Azure provider", "deploymentID", deploymentID, "scope", providerConfig.Az.Value.Scope)
	}

	poller, err := client.CreateOrUpdate(
		ctx,
		clients.Deployment{
			Properties: &clients.DeploymentProperties{
				Template:       recipeData,
				Parameters:     parameters,
				ProviderConfig: providerConfig,
				Mode:           armresources.DeploymentModeIncremental,
			},
		},
		deploymentID.String(),
		clients.DeploymentsClientAPIVersion)
	if err != nil {
		return nil, err
	}

	resp, err := poller.PollUntilDone(ctx, &runtime.PollUntilDoneOptions{Frequency: pollFrequency})
	if err != nil {
		return nil, err
	}

	if *resp.Properties.ProvisioningState != armresources.ProvisioningStateSucceeded {
		return nil, fmt.Errorf("failed to deploy the recipe %q, template path: %q, deployment: %q", recipe.Name, recipe.TemplatePath, deploymentID.Name())
	}

	recipeResponse, err := prepareRecipeResponse(resp.Properties.Outputs, resp.Properties.OutputResources)
	if err != nil {
		return nil, fmt.Errorf("failed to read the recipe output %q: %w", ResultPropertyName, err)
	}

	return &recipeResponse, nil
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

func createDeploymentID(resourceID string, deploymentName string) (resources.ID, error) {
	parsed, err := resources.ParseResource(resourceID)
	if err != nil {
		return resources.ID{}, err
	}

	resourceGroup := parsed.FindScope(resources.ResourceGroupsSegment)
	raw := fmt.Sprintf("/planes/deployments/local/resourceGroups/%s/providers/Microsoft.Resources/deployments/%s", resourceGroup, deploymentName)
	return resources.ParseResource(raw)
}

func createProviderConfig(resourceGroup string, envProviders coreDatamodel.Providers) clients.ProviderConfig {
	config := clients.NewDefaultProviderConfig(resourceGroup)

	if envProviders.Azure != (coreDatamodel.ProvidersAzure{}) {
		config.Az = &clients.Az{
			Type: clients.ProviderTypeAzure,
			Value: clients.Value{
				Scope: envProviders.Azure.Scope,
			},
		}
	}

	if envProviders.AWS != (coreDatamodel.ProvidersAWS{}) {
		config.AWS = &clients.AWS{
			Type: clients.ProviderTypeAWS,
			Value: clients.Value{
				Scope: envProviders.AWS.Scope,
			},
		}
	}

	return config
}
