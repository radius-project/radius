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

package clients

import (
	"context"
	"net/http"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"golang.org/x/sync/errgroup"

	"github.com/radius-project/radius/pkg/azure/clientv2"
	aztoken "github.com/radius-project/radius/pkg/azure/tokencredentials"
	"github.com/radius-project/radius/pkg/cli/clients_new/generated"
	corerpv20231001 "github.com/radius-project/radius/pkg/corerp/api/v20231001preview"
	cntr_ctrl "github.com/radius-project/radius/pkg/corerp/frontend/controller/containers"
	ext_ctrl "github.com/radius-project/radius/pkg/corerp/frontend/controller/extenders"
	gtwy_ctrl "github.com/radius-project/radius/pkg/corerp/frontend/controller/gateways"
	sstr_ctrl "github.com/radius-project/radius/pkg/corerp/frontend/controller/secretstores"
	dapr_ctrl "github.com/radius-project/radius/pkg/daprrp/frontend/controller"
	ds_ctrl "github.com/radius-project/radius/pkg/datastoresrp/frontend/controller"
	msg_ctrl "github.com/radius-project/radius/pkg/messagingrp/frontend/controller"
	ucpv20231001 "github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/ucp/resources"
	resources_radius "github.com/radius-project/radius/pkg/ucp/resources/radius"
)

type UCPApplicationsManagementClient struct {
	RootScope                        string
	ClientOptions                    *arm.ClientOptions
	genericResourceClientFactory     func(scope string, resourceType string) (genericResourceClient, error)
	applicationResourceClientFactory func(scope string) (applicationResourceClient, error)
	environmentResourceClientFactory func(scope string) (environmentResourceClient, error)
	resourceGroupClientFactory       func() (resourceGroupClient, error)
	resourceProviderClientFactory    func() (resourceProviderClient, error)
	resourceTypeClientFactory        func() (resourceTypeClient, error)
	apiVersionClientFactory          func() (apiVersionClient, error)
	locationClientFactory            func() (locationClient, error)
	capture                          func(ctx context.Context, capture **http.Response) context.Context
}

var _ ApplicationsManagementClient = (*UCPApplicationsManagementClient)(nil)

var (
	ResourceTypesList = []string{
		ds_ctrl.MongoDatabasesResourceType,
		msg_ctrl.RabbitMQQueuesResourceType,
		ds_ctrl.RedisCachesResourceType,
		ds_ctrl.SqlDatabasesResourceType,
		dapr_ctrl.DaprStateStoresResourceType,
		dapr_ctrl.DaprSecretStoresResourceType,
		dapr_ctrl.DaprPubSubBrokersResourceType,
		dapr_ctrl.DaprConfigurationStoresResourceType,
		dapr_ctrl.DaprBindingsResourceType,
		ext_ctrl.ResourceTypeName,
		gtwy_ctrl.ResourceTypeName,
		cntr_ctrl.ResourceTypeName,
		sstr_ctrl.ResourceTypeName,
	}
)

// ListResourcesOfType lists all resources of a given type in the configured scope.
func (amc *UCPApplicationsManagementClient) ListResourcesOfType(ctx context.Context, resourceType string) ([]generated.GenericResource, error) {
	client, err := amc.createGenericClient(amc.RootScope, resourceType)
	if err != nil {
		return nil, err
	}

	results := []generated.GenericResource{}
	pager := client.NewListByRootScopePager(&generated.GenericResourcesClientListByRootScopeOptions{})
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, err
		}

		for _, resource := range page.Value {
			results = append(results, *resource)
		}
	}

	return results, nil
}

// ListResourcesOfTypeInApplication lists all resources of a given type in a given application in the configured scope.
func (amc *UCPApplicationsManagementClient) ListResourcesOfTypeInApplication(ctx context.Context, applicationNameOrID string, resourceType string) ([]generated.GenericResource, error) {
	applicationID, err := amc.fullyQualifyID(applicationNameOrID, "Applications.Core/applications")
	if err != nil {
		return nil, err
	}

	resources, err := amc.ListResourcesOfType(ctx, resourceType)
	if err != nil {
		return nil, err
	}

	results := []generated.GenericResource{}
	for _, resource := range resources {
		if isResourceInApplication(resource, applicationID) {
			results = append(results, resource)
		}
	}

	return results, nil
}

// ListResourcesOfTypeInEnvironment lists all resources of a given type in a given environment in the configured scope.
func (amc *UCPApplicationsManagementClient) ListResourcesOfTypeInEnvironment(ctx context.Context, environmentNameOrID string, resourceType string) ([]generated.GenericResource, error) {
	environmentID, err := amc.fullyQualifyID(environmentNameOrID, "Applications.Core/environments")
	if err != nil {
		return nil, err
	}

	resources, err := amc.ListResourcesOfType(ctx, resourceType)
	if err != nil {
		return nil, err
	}

	results := []generated.GenericResource{}
	for _, resource := range resources {
		if isResourceInEnvironment(resource, environmentID) {
			results = append(results, resource)
		}
	}

	return results, nil
}

// ListResourcesInApplication lists all resources in a given application in the configured scope.
func (amc *UCPApplicationsManagementClient) ListResourcesInApplication(ctx context.Context, applicationNameOrID string) ([]generated.GenericResource, error) {
	applicationID, err := amc.fullyQualifyID(applicationNameOrID, "Applications.Core/applications")
	if err != nil {
		return nil, err
	}

	results := []generated.GenericResource{}
	for _, resourceType := range ResourceTypesList {
		resources, err := amc.ListResourcesOfTypeInApplication(ctx, applicationID, resourceType)
		if err != nil {
			return nil, err
		}

		results = append(results, resources...)
	}

	return results, nil
}

// ListResourcesInEnvironment lists all resources in a given environment in the configured scope.
func (amc *UCPApplicationsManagementClient) ListResourcesInEnvironment(ctx context.Context, environmentNameOrID string) ([]generated.GenericResource, error) {
	environmentID, err := amc.fullyQualifyID(environmentNameOrID, "Applications.Core/environments")
	if err != nil {
		return nil, err
	}

	results := []generated.GenericResource{}
	for _, resourceType := range ResourceTypesList {
		resources, err := amc.ListResourcesOfTypeInEnvironment(ctx, environmentID, resourceType)
		if err != nil {
			return nil, err
		}
		results = append(results, resources...)
	}

	return results, nil
}

// GetResource retrieves a resource by its type and name (or id).
func (amc *UCPApplicationsManagementClient) GetResource(ctx context.Context, resourceType string, resourceNameOrID string) (generated.GenericResource, error) {
	scope, name, err := amc.extractScopeAndName(resourceNameOrID)
	if err != nil {
		return generated.GenericResource{}, err
	}

	client, err := amc.createGenericClient(scope, resourceType)
	if err != nil {
		return generated.GenericResource{}, err
	}

	getResponse, err := client.Get(ctx, name, &generated.GenericResourcesClientGetOptions{})
	if err != nil {
		return generated.GenericResource{}, err
	}

	return getResponse.GenericResource, nil
}

// CreateOrUpdateResource creates or updates a resource using its type name (or id).
func (amc *UCPApplicationsManagementClient) CreateOrUpdateResource(ctx context.Context, resourceType string, resourceNameOrID string, resource *generated.GenericResource) (generated.GenericResource, error) {
	scope, name, err := amc.extractScopeAndName(resourceNameOrID)
	if err != nil {
		return generated.GenericResource{}, err
	}

	client, err := amc.createGenericClient(scope, resourceType)
	if err != nil {
		return generated.GenericResource{}, err
	}

	poller, err := client.BeginCreateOrUpdate(ctx, name, *resource, &generated.GenericResourcesClientBeginCreateOrUpdateOptions{})
	if err != nil {
		return generated.GenericResource{}, err
	}

	response, err := poller.PollUntilDone(ctx, nil)
	if err != nil {
		return generated.GenericResource{}, err
	}

	return response.GenericResource, nil
}

// DeleteResource deletes a resource by its type and name (or id).
func (amc *UCPApplicationsManagementClient) DeleteResource(ctx context.Context, resourceType string, resourceNameOrID string) (bool, error) {
	scope, name, err := amc.extractScopeAndName(resourceNameOrID)
	if err != nil {
		return false, err
	}

	client, err := amc.createGenericClient(scope, resourceType)
	if err != nil {
		return false, err
	}

	var response *http.Response
	ctx = amc.captureResponse(ctx, &response)

	poller, err := client.BeginDelete(ctx, name, nil)
	if err != nil {
		return false, err
	}

	_, err = poller.PollUntilDone(ctx, nil)
	if err != nil {
		return false, err
	}

	return response.StatusCode != 204, nil
}

// ListApplications lists all applications in the configured scope.
func (amc *UCPApplicationsManagementClient) ListApplications(ctx context.Context) ([]corerpv20231001.ApplicationResource, error) {
	client, err := amc.createApplicationClient(amc.RootScope)
	if err != nil {
		return nil, err
	}

	results := []corerpv20231001.ApplicationResource{}
	pager := client.NewListByScopePager(&corerpv20231001.ApplicationsClientListByScopeOptions{})
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, err
		}

		for _, application := range page.ApplicationResourceListResult.Value {
			results = append(results, *application)
		}
	}

	return results, nil
}

// ListApplicationsInEnvironment lists the applications that are part of the specified environment and in the configured scope.
func (amc *UCPApplicationsManagementClient) ListApplicationsInEnvironment(ctx context.Context, environmentNameOrID string) ([]corerpv20231001.ApplicationResource, error) {
	applications, err := amc.ListApplications(ctx)
	if err != nil {
		return nil, err
	}

	environmentID, err := amc.fullyQualifyID(environmentNameOrID, "Applications.Core/environments")
	if err != nil {
		return nil, err
	}

	results := []corerpv20231001.ApplicationResource{}
	for _, application := range applications {
		if strings.EqualFold(*application.Properties.Environment, environmentID) {
			results = append(results, application)
		}
	}
	return results, nil
}

// GetApplication retrieves an application by its name (or id).
func (amc *UCPApplicationsManagementClient) GetApplication(ctx context.Context, applicationNameOrID string) (corerpv20231001.ApplicationResource, error) {
	scope, name, err := amc.extractScopeAndName(applicationNameOrID)
	if err != nil {
		return corerpv20231001.ApplicationResource{}, err
	}

	client, err := amc.createApplicationClient(scope)
	if err != nil {
		return corerpv20231001.ApplicationResource{}, err
	}

	response, err := client.Get(ctx, name, &corerpv20231001.ApplicationsClientGetOptions{})
	if err != nil {
		return corerpv20231001.ApplicationResource{}, err
	}

	return response.ApplicationResource, nil
}

// GetApplicationGraph retrieves the application graph of an application by its name (or id).
func (amc *UCPApplicationsManagementClient) GetApplicationGraph(ctx context.Context, applicationNameOrID string) (corerpv20231001.ApplicationGraphResponse, error) {
	scope, name, err := amc.extractScopeAndName(applicationNameOrID)
	if err != nil {
		return corerpv20231001.ApplicationGraphResponse{}, err
	}

	client, err := amc.createApplicationClient(scope)
	if err != nil {
		return corerpv20231001.ApplicationGraphResponse{}, err
	}

	getResponse, err := client.GetGraph(ctx, name, map[string]any{}, &corerpv20231001.ApplicationsClientGetGraphOptions{})
	if err != nil {
		return corerpv20231001.ApplicationGraphResponse{}, err
	}

	return getResponse.ApplicationGraphResponse, nil
}

// CreateOrUpdateApplication creates or updates an application by its name (or id).
func (amc *UCPApplicationsManagementClient) CreateOrUpdateApplication(ctx context.Context, applicationNameOrID string, resource *corerpv20231001.ApplicationResource) error {
	scope, name, err := amc.extractScopeAndName(applicationNameOrID)
	if err != nil {
		return err
	}

	client, err := amc.createApplicationClient(scope)
	if err != nil {
		return err
	}

	// See: https://github.com/radius-project/radius/issues/7597
	//
	// This is a workaround because the server can return invalid system data, which
	// fails to roundtrip when the client does a "GET -> modify -> PUT".
	resource.SystemData = nil

	_, err = client.CreateOrUpdate(ctx, name, *resource, nil)
	if err != nil {
		return err
	}

	return nil
}

// CreateApplicationIfNotFound creates an application if it does not exist.
func (amc *UCPApplicationsManagementClient) CreateApplicationIfNotFound(ctx context.Context, applicationNameOrID string, resource *corerpv20231001.ApplicationResource) error {
	scope, name, err := amc.extractScopeAndName(applicationNameOrID)
	if err != nil {
		return err
	}

	client, err := amc.createApplicationClient(scope)
	if err != nil {
		return err
	}

	_, err = client.Get(ctx, name, nil)
	if Is404Error(err) {
		// continue
	} else if err != nil {
		return err
	} else {
		// Application already exists, nothing to do.
		return nil
	}

	// See: https://github.com/radius-project/radius/issues/7597
	//
	// This is a workaround because the server can return invalid system data, which
	// fails to roundtrip when the client does a "GET -> modify -> PUT".
	resource.SystemData = nil

	_, err = client.CreateOrUpdate(ctx, name, *resource, nil)
	if err != nil {
		return err
	}

	return nil
}

// DeleteApplication deletes an application and all of its resources by its name (or id).
func (amc *UCPApplicationsManagementClient) DeleteApplication(ctx context.Context, applicationNameOrID string) (bool, error) {
	scope, name, err := amc.extractScopeAndName(applicationNameOrID)
	if err != nil {
		return false, err
	}

	// This *also* handles the case where the resource group doesn't exist.
	resources, err := amc.ListResourcesInApplication(ctx, applicationNameOrID)
	if err != nil && !clientv2.Is404Error(err) {
		return false, err
	}

	// Delete resources in parallel
	g, groupCtx := errgroup.WithContext(ctx)
	for _, resource := range resources {
		resource := resource
		g.Go(func() error {
			_, err := amc.DeleteResource(groupCtx, *resource.Type, *resource.Name)
			if err != nil {
				return err
			}
			return nil
		})
	}

	// Wait for dependent resources to be deleted.
	err = g.Wait()
	if err != nil {
		return false, err
	}

	client, err := amc.createApplicationClient(scope)
	if err != nil {
		return false, err
	}

	var response *http.Response
	ctx = amc.captureResponse(ctx, &response)

	_, err = client.Delete(ctx, name, nil)
	if err != nil {
		return false, err
	}

	return response.StatusCode != 204, nil
}

// ListEnvironments lists all environments in the configured scope (assumes configured scope is a resource group).
func (amc *UCPApplicationsManagementClient) ListEnvironments(ctx context.Context) ([]corerpv20231001.EnvironmentResource, error) {
	client, err := amc.createEnvironmentClient(amc.RootScope)
	if err != nil {
		return []corerpv20231001.EnvironmentResource{}, err
	}

	environments := []corerpv20231001.EnvironmentResource{}
	pager := client.NewListByScopePager(&corerpv20231001.EnvironmentsClientListByScopeOptions{})
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return []corerpv20231001.EnvironmentResource{}, err
		}

		for _, environment := range page.EnvironmentResourceListResult.Value {
			environments = append(environments, *environment)
		}
	}

	return environments, nil
}

// ListEnvironmentsAll queries the scope for all environment resources and returns a slice of environment resources or an error if one occurs.
func (amc *UCPApplicationsManagementClient) ListEnvironmentsAll(ctx context.Context) ([]corerpv20231001.EnvironmentResource, error) {
	scope, err := resources.ParseScope(amc.RootScope)
	if err != nil {
		return []corerpv20231001.EnvironmentResource{}, err
	}

	// Query at plane scope, not resource group scope. We don't enforce the exact structure of the scope, so handle both cases.
	//
	// - /planes/radius/local
	// - /planes/radius/local/resourceGroups/my-group
	if scope.FindScope(resources_radius.ScopeResourceGroups) != "" {
		scope = scope.Truncate()
	}

	client, err := amc.createEnvironmentClient(scope.String())
	if err != nil {
		return []corerpv20231001.EnvironmentResource{}, err
	}

	environments := []corerpv20231001.EnvironmentResource{}
	pager := client.NewListByScopePager(&corerpv20231001.EnvironmentsClientListByScopeOptions{})
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return []corerpv20231001.EnvironmentResource{}, err
		}

		for _, environment := range page.EnvironmentResourceListResult.Value {
			environments = append(environments, *environment)
		}
	}

	return environments, nil
}

// GetEnvironment retrieves an environment by its name (in the configured scope) or resource ID.
func (amc *UCPApplicationsManagementClient) GetEnvironment(ctx context.Context, environmentNameOrID string) (corerpv20231001.EnvironmentResource, error) {
	scope, name, err := amc.extractScopeAndName(environmentNameOrID)
	if err != nil {
		return corerpv20231001.EnvironmentResource{}, err
	}

	client, err := amc.createEnvironmentClient(scope)
	if err != nil {
		return corerpv20231001.EnvironmentResource{}, err
	}

	response, err := client.Get(ctx, name, &corerpv20231001.EnvironmentsClientGetOptions{})
	if err != nil {
		return corerpv20231001.EnvironmentResource{}, err
	}

	return response.EnvironmentResource, nil
}

// GetRecipeMetadata shows recipe details including list of all parameters for a given recipe registered to an environment.
func (amc *UCPApplicationsManagementClient) GetRecipeMetadata(ctx context.Context, environmentNameOrID string, recipeMetadata corerpv20231001.RecipeGetMetadata) (corerpv20231001.RecipeGetMetadataResponse, error) {
	scope, name, err := amc.extractScopeAndName(environmentNameOrID)
	if err != nil {
		return corerpv20231001.RecipeGetMetadataResponse{}, err
	}
	client, err := amc.createEnvironmentClient(scope)
	if err != nil {
		return corerpv20231001.RecipeGetMetadataResponse{}, err
	}

	resp, err := client.GetMetadata(ctx, name, recipeMetadata, &corerpv20231001.EnvironmentsClientGetMetadataOptions{})
	if err != nil {
		return corerpv20231001.RecipeGetMetadataResponse{}, err
	}

	return resp.RecipeGetMetadataResponse, nil
}

// CreateOrUpdateEnvironment creates an environment by its name (or id).
func (amc *UCPApplicationsManagementClient) CreateOrUpdateEnvironment(ctx context.Context, environmentNameOrID string, resource *corerpv20231001.EnvironmentResource) error {
	scope, name, err := amc.extractScopeAndName(environmentNameOrID)
	if err != nil {
		return err
	}

	client, err := amc.createEnvironmentClient(scope)
	if err != nil {
		return err
	}

	// See: https://github.com/radius-project/radius/issues/7597
	//
	// This is a workaround because the server can return invalid system data, which
	// fails to roundtrip when the client does a "GET -> modify -> PUT".
	resource.SystemData = nil

	_, err = client.CreateOrUpdate(ctx, name, *resource, &corerpv20231001.EnvironmentsClientCreateOrUpdateOptions{})
	if err != nil {
		return err
	}

	return nil

}

// DeleteEnvironment deletes an environment and all of its resources by its name (in the configured scope) or resource ID.
func (amc *UCPApplicationsManagementClient) DeleteEnvironment(ctx context.Context, environmentNameOrID string) (bool, error) {
	scope, name, err := amc.extractScopeAndName(environmentNameOrID)
	if err != nil {
		return false, err
	}

	applications, err := amc.ListApplicationsInEnvironment(ctx, name)
	if err != nil {
		return false, err
	}

	for _, application := range applications {
		_, err := amc.DeleteApplication(ctx, *application.ID)
		if err != nil {
			return false, err
		}
	}

	client, err := amc.createEnvironmentClient(scope)
	if err != nil {
		return false, err
	}

	// Capture the raw HTTP response so we can check the status code.
	var response *http.Response
	ctx = amc.captureResponse(ctx, &response)

	_, err = client.Delete(ctx, name, nil)
	if err != nil {
		return false, err
	}

	return response.StatusCode != 204, nil
}

// ListResourceGroups lists all resource groups in the configured scope.
func (amc *UCPApplicationsManagementClient) ListResourceGroups(ctx context.Context, planeName string) ([]ucpv20231001.ResourceGroupResource, error) {
	client, err := amc.createResourceGroupClient()
	if err != nil {
		return nil, err
	}

	results := []ucpv20231001.ResourceGroupResource{}
	pager := client.NewListPager(planeName, &ucpv20231001.ResourceGroupsClientListOptions{})
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, err
		}

		for _, resourceGroup := range page.Value {
			results = append(results, *resourceGroup)
		}
	}

	return results, nil
}

// GetResourceGroup retrieves a resource group by its name.
func (amc *UCPApplicationsManagementClient) GetResourceGroup(ctx context.Context, planeName string, resourceGroupName string) (ucpv20231001.ResourceGroupResource, error) {
	client, err := amc.createResourceGroupClient()
	if err != nil {
		return ucpv20231001.ResourceGroupResource{}, err
	}

	response, err := client.Get(ctx, planeName, resourceGroupName, &ucpv20231001.ResourceGroupsClientGetOptions{})
	if err != nil {
		return ucpv20231001.ResourceGroupResource{}, err
	}

	return response.ResourceGroupResource, nil
}

// CreateOrUpdateResourceGroup creates a resource group by its name.
func (amc *UCPApplicationsManagementClient) CreateOrUpdateResourceGroup(ctx context.Context, planeName string, resourceGroupName string, resourceGroup *ucpv20231001.ResourceGroupResource) error {
	client, err := amc.createResourceGroupClient()
	if err != nil {
		return err
	}

	// See: https://github.com/radius-project/radius/issues/7597
	//
	// This is a workaround because the server can return invalid system data, which
	// fails to roundtrip when the client does a "GET -> modify -> PUT".
	resourceGroup.SystemData = nil

	_, err = client.CreateOrUpdate(ctx, planeName, resourceGroupName, *resourceGroup, &ucpv20231001.ResourceGroupsClientCreateOrUpdateOptions{})
	if err != nil {
		return err
	}

	return nil
}

// DeleteResourceGroup deletes a resource group by its name.
func (amc *UCPApplicationsManagementClient) DeleteResourceGroup(ctx context.Context, planeName string, resourceGroupName string) (bool, error) {
	client, err := amc.createResourceGroupClient()
	if err != nil {
		return false, err
	}

	var response *http.Response
	ctx = amc.captureResponse(ctx, &response)

	_, err = client.Delete(ctx, planeName, resourceGroupName, &ucpv20231001.ResourceGroupsClientDeleteOptions{})
	if err != nil {
		return false, err
	}

	return response.StatusCode != 204, nil
}

// ListResourceProviders lists all resource providers in the configured scope.
func (amc *UCPApplicationsManagementClient) ListResourceProviders(ctx context.Context, planeName string) ([]ucpv20231001.ResourceProviderResource, error) {
	client, err := amc.createResourceProviderClient()
	if err != nil {
		return nil, err
	}

	results := []ucpv20231001.ResourceProviderResource{}
	pager := client.NewListPager(planeName, &ucpv20231001.ResourceProvidersClientListOptions{})
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, err
		}

		for _, resourceProvider := range page.Value {
			results = append(results, *resourceProvider)
		}
	}

	return results, nil
}

// GetResourceProvider gets the resource provider with the specified name in the configured scope.
func (amc *UCPApplicationsManagementClient) GetResourceProvider(ctx context.Context, planeName string, resourceProviderName string) (ucpv20231001.ResourceProviderResource, error) {
	client, err := amc.createResourceProviderClient()
	if err != nil {
		return ucpv20231001.ResourceProviderResource{}, err
	}

	response, err := client.Get(ctx, planeName, resourceProviderName, &ucpv20231001.ResourceProvidersClientGetOptions{})
	if err != nil {
		return ucpv20231001.ResourceProviderResource{}, err
	}

	return response.ResourceProviderResource, nil
}

// CreateOrUpdateResourceProvider creates or updates a resource provider in the configured scope.
func (amc *UCPApplicationsManagementClient) CreateOrUpdateResourceProvider(ctx context.Context, planeName string, resourceProviderName string, resource *ucpv20231001.ResourceProviderResource) (ucpv20231001.ResourceProviderResource, error) {
	client, err := amc.createResourceProviderClient()
	if err != nil {
		return ucpv20231001.ResourceProviderResource{}, err
	}

	poller, err := client.BeginCreateOrUpdate(ctx, planeName, resourceProviderName, *resource, &ucpv20231001.ResourceProvidersClientBeginCreateOrUpdateOptions{})
	if err != nil {
		return ucpv20231001.ResourceProviderResource{}, err
	}

	response, err := poller.PollUntilDone(ctx, nil)
	if err != nil {
		return ucpv20231001.ResourceProviderResource{}, err
	}

	return response.ResourceProviderResource, nil
}

// DeleteResourceProvider deletes a resource provider in the configured scope.
func (amc *UCPApplicationsManagementClient) DeleteResourceProvider(ctx context.Context, planeName string, resourceProviderName string) (bool, error) {
	client, err := amc.createResourceProviderClient()
	if err != nil {
		return false, err
	}

	var response *http.Response
	ctx = amc.captureResponse(ctx, &response)

	poller, err := client.BeginDelete(ctx, planeName, resourceProviderName, &ucpv20231001.ResourceProvidersClientBeginDeleteOptions{})
	if err != nil {
		return false, err
	}

	_, err = poller.PollUntilDone(ctx, nil)
	if err != nil {
		return false, err
	}

	return response.StatusCode != 204, nil
}

// ListResourceProviderSummaries lists all resource provider summaries in the configured scope.
func (amc *UCPApplicationsManagementClient) ListResourceProviderSummaries(ctx context.Context, planeName string) ([]ucpv20231001.ResourceProviderSummary, error) {
	client, err := amc.createResourceProviderClient()
	if err != nil {
		return nil, err
	}

	results := []ucpv20231001.ResourceProviderSummary{}
	pager := client.NewListProviderSummariesPager(planeName, &ucpv20231001.ResourceProvidersClientListProviderSummariesOptions{})
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, err
		}

		for _, summary := range page.Value {
			results = append(results, *summary)
		}
	}

	return results, nil
}

// GetResourceProvider gets the resource provider summary with the specified name in the configured scope.
func (amc *UCPApplicationsManagementClient) GetResourceProviderSummary(ctx context.Context, planeName string, resourceProviderName string) (ucpv20231001.ResourceProviderSummary, error) {
	client, err := amc.createResourceProviderClient()
	if err != nil {
		return ucpv20231001.ResourceProviderSummary{}, err
	}

	response, err := client.GetProviderSummary(ctx, planeName, resourceProviderName, &ucpv20231001.ResourceProvidersClientGetProviderSummaryOptions{})
	if err != nil {
		return ucpv20231001.ResourceProviderSummary{}, err
	}

	return response.ResourceProviderSummary, nil
}

// CreateOrUpdateResourceType creates or updates a resource type in the configured scope.
func (amc *UCPApplicationsManagementClient) CreateOrUpdateResourceType(ctx context.Context, planeName string, resourceProviderName string, resourceTypeName string, resource *ucpv20231001.ResourceTypeResource) (ucpv20231001.ResourceTypeResource, error) {
	client, err := amc.createResourceTypeClient()
	if err != nil {
		return ucpv20231001.ResourceTypeResource{}, err
	}

	poller, err := client.BeginCreateOrUpdate(ctx, planeName, resourceProviderName, resourceTypeName, *resource, &ucpv20231001.ResourceTypesClientBeginCreateOrUpdateOptions{})
	if err != nil {
		return ucpv20231001.ResourceTypeResource{}, err
	}

	response, err := poller.PollUntilDone(ctx, nil)
	if err != nil {
		return ucpv20231001.ResourceTypeResource{}, err
	}

	return response.ResourceTypeResource, nil
}

// DeleteResourceType deletes a resource type in the configured scope.
func (amc *UCPApplicationsManagementClient) DeleteResourceType(ctx context.Context, planeName string, resourceProviderName string, resourceTypeName string) (bool, error) {
	client, err := amc.createResourceTypeClient()
	if err != nil {
		return false, err
	}

	var response *http.Response
	ctx = amc.captureResponse(ctx, &response)

	poller, err := client.BeginDelete(ctx, planeName, resourceProviderName, resourceTypeName, &ucpv20231001.ResourceTypesClientBeginDeleteOptions{})
	if err != nil {
		return false, err
	}

	_, err = poller.PollUntilDone(ctx, nil)
	if err != nil {
		return false, err
	}

	return response.StatusCode != 204, nil
}

// CreateOrUpdateAPIVersion creates or updates an API version in the configured scope.
func (amc *UCPApplicationsManagementClient) CreateOrUpdateAPIVersion(ctx context.Context, planeName string, resourceProviderName string, resourceTypeName string, apiVersionName string, resource *ucpv20231001.APIVersionResource) (ucpv20231001.APIVersionResource, error) {
	client, err := amc.createAPIVersionClient()
	if err != nil {
		return ucpv20231001.APIVersionResource{}, err
	}

	poller, err := client.BeginCreateOrUpdate(ctx, planeName, resourceProviderName, resourceTypeName, apiVersionName, *resource, &ucpv20231001.APIVersionsClientBeginCreateOrUpdateOptions{})
	if err != nil {
		return ucpv20231001.APIVersionResource{}, err
	}

	response, err := poller.PollUntilDone(ctx, nil)
	if err != nil {
		return ucpv20231001.APIVersionResource{}, err
	}

	return response.APIVersionResource, nil
}

// CreateOrUpdateLocation creates or updates a resource provider location in the configured scope.
func (amc *UCPApplicationsManagementClient) CreateOrUpdateLocation(ctx context.Context, planeName string, resourceProviderName string, locationName string, resource *ucpv20231001.LocationResource) (ucpv20231001.LocationResource, error) {
	client, err := amc.createLocationClient()
	if err != nil {
		return ucpv20231001.LocationResource{}, err
	}

	poller, err := client.BeginCreateOrUpdate(ctx, planeName, resourceProviderName, locationName, *resource, &ucpv20231001.LocationsClientBeginCreateOrUpdateOptions{})
	if err != nil {
		return ucpv20231001.LocationResource{}, err
	}

	response, err := poller.PollUntilDone(ctx, nil)
	if err != nil {
		return ucpv20231001.LocationResource{}, err
	}

	return response.LocationResource, nil
}

func (amc *UCPApplicationsManagementClient) createApplicationClient(scope string) (applicationResourceClient, error) {
	if amc.applicationResourceClientFactory == nil {
		// Generated client doesn't like the leading '/' in the scope.
		return corerpv20231001.NewApplicationsClient(strings.TrimPrefix(scope, resources.SegmentSeparator), &aztoken.AnonymousCredential{}, amc.ClientOptions)
	}

	return amc.applicationResourceClientFactory(scope)
}

func (amc *UCPApplicationsManagementClient) createEnvironmentClient(scope string) (environmentResourceClient, error) {
	if amc.environmentResourceClientFactory == nil {
		// Generated client doesn't like the leading '/' in the scope.
		return corerpv20231001.NewEnvironmentsClient(strings.TrimPrefix(scope, resources.SegmentSeparator), &aztoken.AnonymousCredential{}, amc.ClientOptions)
	}

	return amc.environmentResourceClientFactory(scope)
}

func (amc *UCPApplicationsManagementClient) createGenericClient(scope string, resourceType string) (genericResourceClient, error) {
	if amc.genericResourceClientFactory == nil {
		// Generated client doesn't like the leading '/' in the scope.
		return generated.NewGenericResourcesClient(strings.TrimPrefix(scope, resources.SegmentSeparator), resourceType, &aztoken.AnonymousCredential{}, amc.ClientOptions)
	}

	return amc.genericResourceClientFactory(scope, resourceType)
}

func (amc *UCPApplicationsManagementClient) createResourceGroupClient() (resourceGroupClient, error) {
	if amc.resourceGroupClientFactory == nil {
		return ucpv20231001.NewResourceGroupsClient(&aztoken.AnonymousCredential{}, amc.ClientOptions)
	}

	return amc.resourceGroupClientFactory()
}

func (amc *UCPApplicationsManagementClient) createResourceProviderClient() (resourceProviderClient, error) {
	if amc.resourceProviderClientFactory == nil {
		return ucpv20231001.NewResourceProvidersClient(&aztoken.AnonymousCredential{}, amc.ClientOptions)
	}

	return amc.resourceProviderClientFactory()
}

func (amc *UCPApplicationsManagementClient) createResourceTypeClient() (resourceTypeClient, error) {
	if amc.resourceTypeClientFactory == nil {
		return ucpv20231001.NewResourceTypesClient(&aztoken.AnonymousCredential{}, amc.ClientOptions)
	}

	return amc.resourceTypeClientFactory()
}

func (amc *UCPApplicationsManagementClient) createAPIVersionClient() (apiVersionClient, error) {
	if amc.apiVersionClientFactory == nil {
		return ucpv20231001.NewAPIVersionsClient(&aztoken.AnonymousCredential{}, amc.ClientOptions)
	}

	return amc.apiVersionClientFactory()
}

func (amc *UCPApplicationsManagementClient) createLocationClient() (locationClient, error) {
	if amc.locationClientFactory == nil {
		return ucpv20231001.NewLocationsClient(&aztoken.AnonymousCredential{}, amc.ClientOptions)
	}

	return amc.locationClientFactory()
}

func (amc *UCPApplicationsManagementClient) extractScopeAndName(nameOrID string) (string, string, error) {
	if strings.HasPrefix(nameOrID, resources.SegmentSeparator) {
		// Treat this as a resource id.
		id, err := resources.ParseResource(nameOrID)
		if err != nil {
			return "", "", err
		}

		return id.RootScope(), id.Name(), nil
	}

	// Treat this as a resource name
	return amc.RootScope, nameOrID, nil
}

func (amc *UCPApplicationsManagementClient) fullyQualifyID(nameOrID string, resourceType string) (string, error) {
	if strings.HasPrefix(nameOrID, resources.SegmentSeparator) {
		// Treat this as a resource id.
		id, err := resources.ParseResource(nameOrID)
		if err != nil {
			return "", err
		}

		return id.String(), nil
	}

	// Treat this as a resource name
	return amc.RootScope + "/providers/" + resourceType + "/" + nameOrID, nil
}

func isResourceInApplication(resource generated.GenericResource, applicationID string) bool {
	obj, found := resource.Properties["application"]
	// A resource may not have an application associated with it.
	if !found {
		return false
	}

	associatedAppId, ok := obj.(string)
	if !ok || associatedAppId == "" {
		return false
	}

	if strings.EqualFold(associatedAppId, applicationID) {
		return true
	}

	return false
}

func isResourceInEnvironment(resource generated.GenericResource, environmentID string) bool {
	obj, found := resource.Properties["environment"]
	// A resource may not have an environment associated with it.
	if !found {
		return false
	}

	associatedEnvId, ok := obj.(string)
	if !ok || associatedEnvId == "" {
		return false
	}

	if strings.EqualFold(associatedEnvId, environmentID) {
		return true
	}

	return false
}

func (amc *UCPApplicationsManagementClient) captureResponse(ctx context.Context, response **http.Response) context.Context {
	if amc.capture == nil {
		return runtime.WithCaptureResponse(ctx, response)
	}

	return amc.capture(ctx, response)
}
