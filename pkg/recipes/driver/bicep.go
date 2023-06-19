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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
	deployments "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
	"github.com/go-logr/logr"
	coreDatamodel "github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/pkg/recipes"
	"github.com/project-radius/radius/pkg/rp/util"
	clients "github.com/project-radius/radius/pkg/sdk/clients"
	"github.com/project-radius/radius/pkg/ucp/resources"
)

//go:generate mockgen -destination=./mock_driver.go -package=driver -self_package github.com/project-radius/radius/pkg/recipes/driver github.com/project-radius/radius/pkg/recipes/driver Driver
const (
	deploymentPrefix   = "recipe"
	pollFrequency      = time.Second * 5
	ResultPropertyName = "result"
	recipeParameters   = "parameters"
)

var _ Driver = (*bicepDriver)(nil)

// NewBicepDriver creates the new Driver for Bicep.
func NewBicepDriver(armOptions *arm.ClientOptions, deploymentClient *clients.ResourceDeploymentsClient) Driver {
	return &bicepDriver{ArmClientOptions: armOptions, DeploymentClient: deploymentClient}
}

type bicepDriver struct {
	ArmClientOptions *arm.ClientOptions
	DeploymentClient *clients.ResourceDeploymentsClient
}

// Execute fetches the recipe contents from acr and deploys the recipe by making a call to ucp and returns the recipe result.
func (d *bicepDriver) Execute(ctx context.Context, configuration recipes.Configuration, recipe recipes.ResourceMetadata, definition recipes.EnvironmentDefinition) (*recipes.RecipeOutput, error) {
	logger := logr.FromContextOrDiscard(ctx)
	logger.Info(fmt.Sprintf("Deploying recipe: %q, template: %q", definition.Name, definition.TemplatePath))

	recipeData := make(map[string]any)
	err := util.ReadFromRegistry(ctx, definition.TemplatePath, &recipeData)
	if err != nil {
		return nil, err
	}
	// create the context object to be passed to the recipe deployment
	recipeContext, err := createRecipeContextParameter(recipe.ResourceID, recipe.EnvironmentID, configuration.Runtime.Kubernetes.EnvironmentNamespace, recipe.ApplicationID, configuration.Runtime.Kubernetes.Namespace)
	if err != nil {
		return nil, err
	}

	// get the parameters after resolving the conflict between developer and operator parameters
	// if the recipe template also has the context parameter defined then add it to the parameter for deployment
	_, isContextParameterDefined := recipeData[recipeParameters].(map[string]any)[datamodel.RecipeContextParameter]
	parameters := createRecipeParameters(recipe.Parameters, definition.Parameters, isContextParameterDefined, recipeContext)

	deploymentName := deploymentPrefix + strconv.FormatInt(time.Now().UnixNano(), 10)
	deploymentID, err := createDeploymentID(recipeContext.Resource.ID, deploymentName)
	if err != nil {
		return nil, err
	}

	// Provider config will specify the Azure and AWS scopes (if provided).
	providerConfig := createProviderConfig(deploymentID.FindScope(resources.ResourceGroupsSegment), configuration.Providers)

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
		return nil, err
	}

	resp, err := poller.PollUntilDone(ctx, &runtime.PollUntilDoneOptions{Frequency: pollFrequency})
	if err != nil {
		return nil, err
	}

	recipeResponse, err := prepareRecipeResponse(resp.Properties.Outputs, resp.Properties.OutputResources)
	if err != nil {
		return nil, fmt.Errorf("failed to read the recipe output %q: %w", ResultPropertyName, err)
	}

	return &recipeResponse, nil
}

// createRecipeContextParameter creates the context parameter for the recipe with the link, environment and application info
func createRecipeContextParameter(resourceID, environmentID, environmentNamespace, applicationID, applicationNamespace string) (*RecipeContext, error) {
	parsedLink, err := resources.ParseResource(resourceID)
	if err != nil {
		return nil, fmt.Errorf("failed to parse resourceID: %q while building the recipe context parameter %w", resourceID, err)
	}
	parsedEnv, err := resources.ParseResource(environmentID)
	if err != nil {
		return nil, fmt.Errorf("failed to parse environmentID: %q while building the recipe context parameter %w", environmentID, err)
	}

	recipeContext := RecipeContext{
		Resource: Resource{
			ResourceInfo: ResourceInfo{
				Name: parsedLink.Name(),
				ID:   resourceID,
			},
			Type: parsedLink.Type(),
		},
		Environment: ResourceInfo{
			Name: parsedEnv.Name(),
			ID:   environmentID,
		},
		Runtime: recipes.RuntimeConfiguration{
			Kubernetes: &recipes.KubernetesRuntime{
				Namespace:            environmentNamespace,
				EnvironmentNamespace: environmentNamespace,
			},
		},
	}

	if applicationID != "" {
		parsedApp, err := resources.ParseResource(applicationID)
		if err != nil {
			return nil, fmt.Errorf("failed to parse applicationID :%q while building the recipe context parameter %w", applicationID, err)
		}
		recipeContext.Application.ID = applicationID
		recipeContext.Application.Name = parsedApp.Name()
		recipeContext.Runtime.Kubernetes.Namespace = applicationNamespace
	}

	return &recipeContext, nil
}

// createRecipeParameters creates the parameters to be passed for recipe deployment after handling conflicts in parameters set by operator and developer.
// In case of conflict the developer parameter takes precedence. If recipe has context parameter defined adds the context information to the parameters list
func createRecipeParameters(devParams, operatorParams map[string]any, isCxtSet bool, recipeContext *RecipeContext) map[string]any {
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
		parameters["context"] = map[string]any{
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
	return resources.ParseResource(fmt.Sprintf("/planes/deployments/local/resourceGroups/%s/providers/Microsoft.Resources/deployments/%s", resourceGroup, deploymentName))
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

// prepareRecipeResponse populates the recipe response from parsing the deployment output 'result' object and the
// resources created by the template.
func prepareRecipeResponse(outputs any, resources []*armresources.ResourceReference) (recipes.RecipeOutput, error) {
	// We populate the recipe response from the 'result' output (if set)
	// and the resources created by the template.
	//
	// Note that there are two ways a resource can be returned:
	// - Implicitly when it is created in the template (it will be in 'resources').
	// - Explicitly as part of the 'result' output.
	//
	// The latter is needed because non-ARM and non-UCP resources are not returned as part of the implicit 'resources'
	// collection. For us this mostly means Kubernetes resources - the user has to be explicit.
	recipeResponse := recipes.RecipeOutput{}

	out, ok := outputs.(map[string]any)
	if ok {
		recipeOutput, ok := out[ResultPropertyName].(map[string]any)
		if ok {
			output, ok := recipeOutput["value"].(map[string]any)
			if ok {
				b, err := json.Marshal(&output)
				if err != nil {
					return recipes.RecipeOutput{}, err
				}

				// Using a decoder to block unknown fields.
				decoder := json.NewDecoder(bytes.NewBuffer(b))
				decoder.DisallowUnknownFields()
				err = decoder.Decode(&recipeResponse)
				if err != nil {
					return recipes.RecipeOutput{}, err
				}
			}
		}
	}

	// process the 'resources' created by the template
	for _, id := range resources {
		recipeResponse.Resources = append(recipeResponse.Resources, *id.ID)
	}

	// Make sure our maps are non-nil (it's just friendly).
	if recipeResponse.Secrets == nil {
		recipeResponse.Secrets = map[string]any{}
	}
	if recipeResponse.Values == nil {
		recipeResponse.Values = map[string]any{}
	}

	return recipeResponse, nil
}
