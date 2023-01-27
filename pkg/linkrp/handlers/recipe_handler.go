// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handlers

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
	"github.com/go-logr/logr"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/azure/armauth"
	"github.com/project-radius/radius/pkg/azure/clientv2"
	coreDatamodel "github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/pkg/logging"
	"github.com/project-radius/radius/pkg/rp/util"
)

const deploymentPrefix = "recipe"

// RecipeHandler is an interface to deploy and delete recipe resources
//
//go:generate mockgen -destination=./mock_recipe_handler.go -package=handlers -self_package github.com/project-radius/radius/pkg/linkrp/handlers github.com/project-radius/radius/pkg/linkrp/handlers RecipeHandler
type RecipeHandler interface {
	DeployRecipe(ctx context.Context, recipe datamodel.RecipeProperties, envProviders coreDatamodel.Providers, recipeContext datamodel.RecipeContext) ([]string, error)
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
func (handler *azureRecipeHandler) DeployRecipe(ctx context.Context, recipe datamodel.RecipeProperties, envProviders coreDatamodel.Providers, recipeContext datamodel.RecipeContext) (deployedResources []string, err error) {
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
	recipeData := make(map[string]any)
	err = util.ReadFromRegistry(ctx, recipe.TemplatePath, &recipeData)
	if err != nil {
		return nil, v1.NewClientErrInvalidRequest(fmt.Sprintf("failed to fetch template from the path %q for recipe %q: %s", recipe.TemplatePath, recipe.Name, err.Error()))
	}

	// get the parameters after resolving the conflict between developer and operator parameters
	// if the recipe template also has the context parameter defined then add it to the parameter for deployment
	_, isContextParameterDefined := recipeData["parameters"].(map[string]interface{})[datamodel.RecipeContextParameter]
	parameters := createRecipeParameters(recipe.Parameters, recipe.EnvParameters, isContextParameterDefined, &recipeContext)

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
