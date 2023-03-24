// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package driver

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
	deployments "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
	"github.com/go-logr/logr"
	"github.com/project-radius/radius/pkg/recipes"
	"github.com/project-radius/radius/pkg/recipes/configloader"
	"github.com/project-radius/radius/pkg/rp/util"
	clients "github.com/project-radius/radius/pkg/sdk/clients"
	"github.com/project-radius/radius/pkg/ucp/resources"
)

//go:generate mockgen -destination=./mock_driver.go -package=driver -self_package github.com/project-radius/radius/pkg/recipes/driver github.com/project-radius/radius/pkg/recipes/driver Driver
const (
	deploymentPrefix = "recipe"
	pollFrequency    = time.Second * 5
)

var _ Driver = (*bicepDriver)(nil)

func NewBicepDriver(ucpOptions *arm.ClientOptions, deploymentClient *clients.ResourceDeploymentsClient) Driver {
	return &bicepDriver{UCPClientOptions: ucpOptions, DeploymentClient: deploymentClient}
}

type bicepDriver struct {
	UCPClientOptions *arm.ClientOptions
	DeploymentClient *clients.ResourceDeploymentsClient
}

// Execute implements recipes.Driver
func (d *bicepDriver) Execute(ctx context.Context, configuration configloader.Configuration, recipe recipes.RecipeMetadata, definition configloader.RecipeDefinition) (*recipes.RecipeResult, error) {
	logger := logr.FromContextOrDiscard(ctx)
	logger.Info(fmt.Sprintf("Deploying recipe: %q, template: %q", recipe.Name, definition.TemplatePath))

	recipeData := make(map[string]any)
	err := util.ReadFromRegistry(ctx, definition.TemplatePath, &recipeData)
	if err != nil {
		return nil, err
	}
	subjectID, err := resources.ParseResource(recipe.ResourceID)
	if err != nil {
		return nil, err
	}

	deploymentName := deploymentPrefix + strconv.FormatInt(time.Now().UnixNano(), 10)
	scopes := []resources.ScopeSegment{
		{Type: "deployments", Name: "local"},
		{Type: "resourceGroups", Name: subjectID.FindScope(resources.ResourceGroupsSegment)},
	}
	types := []resources.TypeSegment{
		{Type: "Microsoft.Resources/deployments", Name: deploymentName},
	}
	resourceID := resources.MakeUCPID(scopes, types...)

	parameters := map[string]interface{}{}
	for key, value := range definition.Parameters {
		parameters[key] = map[string]interface{}{
			"value": value,
		}
	}
	for key, value := range recipe.Parameters {
		parameters[key] = map[string]interface{}{
			"value": value,
		}
	}

	_, contextParamterDefined := recipeData["parameters"].(map[string]interface{})["context"]
	if contextParamterDefined {
		resource := map[string]interface{}{
			"environmentId": recipe.EnvironmentID,
			"applicationId": recipe.ApplicationID,
			"resourceId":    recipe.ResourceID,
		}

		parsed, err := resources.ParseResource(recipe.EnvironmentID)
		if err != nil {
			return nil, err
		}
		resource["environmentName"] = parsed.Name()

		if recipe.ApplicationID != "" {
			parsed, err := resources.ParseResource(recipe.ApplicationID)
			if err != nil {
				return nil, err
			}
			resource["applicationName"] = parsed.Name()
		}

		parsed, err = resources.ParseResource(recipe.ResourceID)
		if err != nil {
			return nil, err
		}
		resource["resourceName"] = parsed.Name()

		parameters["context"] = map[string]interface{}{
			"value": map[string]interface{}{
				"runtime":  configuration.Runtime,
				"resource": resource,
			},
		}
	}

	providerConfig := d.formatProviderConfigs(configuration, subjectID)

	// Using ARM deployment client to deploy ARM JSON template fetched from ACR
	future, err := d.DeploymentClient.CreateOrUpdate(
		ctx,
		clients.Deployment{
			Properties: &clients.DeploymentProperties{
				Mode:           deployments.DeploymentModeIncremental,
				ProviderConfig: &providerConfig,
				Parameters:     parameters,
				Template:       recipeData,
			},
		},
		resourceID,
		clients.DeploymentsClientAPIVersion,
	)
	if err != nil {
		return nil, err
	}

	resp, err := future.PollUntilDone(ctx, &runtime.PollUntilDoneOptions{Frequency: pollFrequency})
	if err != nil {
		return nil, err
	}

	// return error if the Provisioning is not success
	if *resp.Properties.ProvisioningState != armresources.ProvisioningStateSucceeded {
		return nil, fmt.Errorf("failed to deploy the recipe %q, template path: %q, deployment: %q", recipe.Name, definition.TemplatePath, deploymentName)
	}

	result := recipes.RecipeResult{
		Secrets: map[string]interface{}{},
		Values:  map[string]interface{}{},
	}

	// Get list of output resources deployed
	for _, id := range resp.Properties.OutputResources {
		result.Resources = append(result.Resources, *id.ID)
	}

	output, outputFound := resp.Properties.Outputs.(map[string]interface{})["result"]
	if outputFound {
		obj, resourcesFound := output.(map[string]interface{})["value"].(map[string]interface{})["resources"]
		if resourcesFound {
			resources := obj.([]interface{})
			for _, resource := range resources {
				result.Resources = append(result.Resources, resource.(string))
			}

		}

		obj, secretsFound := output.(map[string]interface{})["value"].(map[string]interface{})["secrets"]
		if secretsFound {
			secrets := obj.(map[string]interface{})
			for key, value := range secrets {
				result.Secrets[key] = value
			}
		}

		obj, valuesFound := output.(map[string]interface{})["value"].(map[string]interface{})["values"]
		if valuesFound {
			values := obj.(map[string]interface{})
			for key, value := range values {
				result.Values[key] = value
			}
		}
	}

	return &result, nil
}

func (d *bicepDriver) formatProviderConfigs(configuration configloader.Configuration, subjectID resources.ID) clients.ProviderConfig {
	providerConfig := clients.ProviderConfig{}

	if &configuration.Providers != nil && &configuration.Providers.Azure != nil {
		providerConfig.Az = &clients.Az{
			Type: "AzureResourceManager",
			Value: clients.Value{
				Scope: configuration.Providers.Azure.Scope,
			},
		}
	}
	if &configuration.Providers != nil && &configuration.Providers.AWS != nil {
		providerConfig.AWS = &clients.AWS{
			Type: "AWS",
			Value: clients.Value{
				Scope: configuration.Providers.AWS.Scope,
			},
		}
	}

	// TODO: remove the deployment plane and use the Radius scope for everything.
	providerConfig.Deployments = &clients.Deployments{
		Type: "Microsoft.Resources",
		Value: clients.Value{
			Scope: "/planes/deployments/local/resourceGroups/" + subjectID.FindScope(resources.ResourceGroupsSegment),
		},
	}

	providerConfig.Radius = &clients.Radius{
		Type: "Radius",
		Value: clients.Value{
			Scope: subjectID.RootScope(),
		},
	}

	return providerConfig
}
