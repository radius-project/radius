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

	"github.com/project-radius/radius/pkg/azure/clientv2"
	aztoken "github.com/project-radius/radius/pkg/azure/tokencredentials"
	"github.com/project-radius/radius/pkg/cli/clients_new/generated"
	corerpv20220315 "github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
	"github.com/project-radius/radius/pkg/linkrp"
	ucpv20220901 "github.com/project-radius/radius/pkg/ucp/api/v20220901privatepreview"
	"github.com/project-radius/radius/pkg/ucp/resources"
)

type UCPApplicationsManagementClient struct {
	RootScope     string
	ClientOptions *arm.ClientOptions
}

var _ ApplicationsManagementClient = (*UCPApplicationsManagementClient)(nil)

var (
	ResourceTypesList = []string{
		linkrp.MongoDatabasesResourceType,
		linkrp.RabbitMQMessageQueuesResourceType,
		linkrp.RedisCachesResourceType,
		linkrp.SqlDatabasesResourceType,
		linkrp.DaprStateStoresResourceType,
		linkrp.DaprSecretStoresResourceType,
		linkrp.DaprPubSubBrokersResourceType,
		linkrp.ExtendersResourceType,
		linkrp.N_RabbitMQQueuesResourceType,
		"Applications.Core/gateways",
		"Applications.Core/httpRoutes",
		"Applications.Core/containers",
		"Applications.Core/secretStores",
	}
)

// ListAllResourcesByType lists the all the resources within a scope
//
// # Function Explanation
//
// ListAllResourcesByType retrieves a list of all resources of a given type from the root
// scope, and returns them in a slice of GenericResource objects, or an error if one occurs.
func (amc *UCPApplicationsManagementClient) ListAllResourcesByType(ctx context.Context, resourceType string) ([]generated.GenericResource, error) {
	results := []generated.GenericResource{}

	client, err := generated.NewGenericResourcesClient(amc.RootScope, resourceType, &aztoken.AnonymousCredential{}, amc.ClientOptions)
	if err != nil {
		return results, err
	}

	pager := client.NewListByRootScopePager(&generated.GenericResourcesClientListByRootScopeOptions{})
	for pager.More() {
		nextPage, err := pager.NextPage(ctx)
		if err != nil {
			return results, err
		}
		applicationList := nextPage.GenericResourcesList.Value
		for _, application := range applicationList {
			results = append(results, *application)
		}
	}

	return results, nil
}

// ListAllResourceOfTypeInApplication lists the resources of a particular type in an application
//
// # Function Explanation
//
// ListAllResourcesOfTypeInApplication takes in a context, an application name and a
// resource type and returns a slice of GenericResources and an error if one occurs.
func (amc *UCPApplicationsManagementClient) ListAllResourcesOfTypeInApplication(ctx context.Context, applicationName string, resourceType string) ([]generated.GenericResource, error) {
	results := []generated.GenericResource{}
	resourceList, err := amc.ListAllResourcesByType(ctx, resourceType)
	if err != nil {
		return nil, err
	}
	for _, resource := range resourceList {
		isResourceWithApplication := isResourceInApplication(ctx, resource, applicationName)
		if isResourceWithApplication {
			results = append(results, resource)
		}
	}
	return results, nil
}

// ListAllResourcesByApplication lists the resources of a particular application
//
// # Function Explanation
//
// ListAllResourcesByApplication takes in a context and an application name and returns
// a slice of GenericResources and an error if one occurs.
func (amc *UCPApplicationsManagementClient) ListAllResourcesByApplication(ctx context.Context, applicationName string) ([]generated.GenericResource, error) {
	results := []generated.GenericResource{}
	for _, resourceType := range ResourceTypesList {
		resourceList, err := amc.ListAllResourcesOfTypeInApplication(ctx, applicationName, resourceType)
		if err != nil {
			return nil, err
		}
		results = append(results, resourceList...)
	}

	return results, nil
}

// ListAllResourcesByEnvironment lists the all the resources of a particular environment
//
// # Function Explanation
//
// ListAllResourcesByEnvironment iterates through a list of resource types and calls ListAllResourcesOfTypeInEnvironment
// for each one, appending the results to a slice of GenericResources and returning it. If an error is encountered, it is returned.
func (amc *UCPApplicationsManagementClient) ListAllResourcesByEnvironment(ctx context.Context, environmentName string) ([]generated.GenericResource, error) {
	results := []generated.GenericResource{}
	for _, resourceType := range ResourceTypesList {
		resourceList, err := amc.ListAllResourcesOfTypeInEnvironment(ctx, environmentName, resourceType)
		if err != nil {
			return nil, err
		}
		results = append(results, resourceList...)
	}

	return results, nil
}

// ListAllResourcesByTypeInEnvironment lists the all the resources of a particular type in an environment
//
// # Function Explanation
//
// ListAllResourcesOfTypeInEnvironment takes in a context, an environment name and a
// resource type and returns a slice of GenericResources and an error if one occurs.
func (amc *UCPApplicationsManagementClient) ListAllResourcesOfTypeInEnvironment(ctx context.Context, environmentName string, resourceType string) ([]generated.GenericResource, error) {
	results := []generated.GenericResource{}
	resourceList, err := amc.ListAllResourcesByType(ctx, resourceType)
	if err != nil {
		return nil, err
	}
	for _, resource := range resourceList {
		isResourceWithApplication := isResourceInEnvironment(ctx, resource, environmentName)
		if isResourceWithApplication {
			results = append(results, resource)
		}
	}
	return results, nil
}

// # Function Explanation
//
// ShowResource creates a new client for a given resource type and attempts to retrieve the resource with the given name,
// returning the resource or an error if one occurs.
func (amc *UCPApplicationsManagementClient) ShowResource(ctx context.Context, resourceType string, resourceName string) (generated.GenericResource, error) {
	client, err := generated.NewGenericResourcesClient(amc.RootScope, resourceType, &aztoken.AnonymousCredential{}, amc.ClientOptions)
	if err != nil {
		return generated.GenericResource{}, err
	}

	getResponse, err := client.Get(ctx, resourceName, &generated.GenericResourcesClientGetOptions{})
	if err != nil {
		return generated.GenericResource{}, err
	}

	return getResponse.GenericResource, nil
}

// # Function Explanation
//
// DeleteResource creates a new client, sends a delete request to the resource, polls until the request is completed,
// and returns a boolean indicating whether the resource was successfully deleted or not, and an error if one occurred.
func (amc *UCPApplicationsManagementClient) DeleteResource(ctx context.Context, resourceType string, resourceName string) (bool, error) {
	client, err := generated.NewGenericResourcesClient(amc.RootScope, resourceType, &aztoken.AnonymousCredential{}, amc.ClientOptions)
	if err != nil {
		return false, err
	}

	var respFromCtx *http.Response
	ctxWithResp := runtime.WithCaptureResponse(ctx, &respFromCtx)

	poller, err := client.BeginDelete(ctxWithResp, resourceName, nil)
	if err != nil {
		return false, err
	}

	_, err = poller.PollUntilDone(ctx, nil)
	if err != nil {
		return false, err
	}

	return respFromCtx.StatusCode != 204, nil
}

// # Function Explanation
//
// ListApplications() retrieves a list of ApplicationResource objects from the Azure API
// and returns them in a slice, or an error if one occurs.
func (amc *UCPApplicationsManagementClient) ListApplications(ctx context.Context) ([]corerpv20220315.ApplicationResource, error) {
	results := []corerpv20220315.ApplicationResource{}

	client, err := corerpv20220315.NewApplicationsClient(amc.RootScope, &aztoken.AnonymousCredential{}, amc.ClientOptions)
	if err != nil {
		return results, err
	}

	pager := client.NewListByScopePager(&corerpv20220315.ApplicationsClientListByScopeOptions{})
	for pager.More() {
		nextPage, err := pager.NextPage(ctx)
		if err != nil {
			return results, err
		}
		applicationList := nextPage.ApplicationResourceList.Value
		for _, application := range applicationList {
			results = append(results, *application)
		}
	}

	return results, nil
}

// # Function Explanation
//
// ListApplicationsByEnv takes in a context and an environment name and returns a slice of ApplicationResource objects
// and an error if one occurs.
func (amc *UCPApplicationsManagementClient) ListApplicationsByEnv(ctx context.Context, envName string) ([]corerpv20220315.ApplicationResource, error) {
	results := []corerpv20220315.ApplicationResource{}
	applicationsList, err := amc.ListApplications(ctx)
	if err != nil {
		return nil, err
	}
	envID := "/" + amc.RootScope + "/providers/applications.core/environments/" + envName
	for _, application := range applicationsList {
		if strings.EqualFold(envID, *application.Properties.Environment) {
			results = append(results, application)
		}
	}
	return results, nil
}

// # Function Explanation
//
// ShowApplication creates a new ApplicationsClient, attempts to get an application
// resource from the Azure Cognitive Search service, and returns the resource or an error if one occurs.
func (amc *UCPApplicationsManagementClient) ShowApplication(ctx context.Context, applicationName string) (corerpv20220315.ApplicationResource, error) {
	client, err := corerpv20220315.NewApplicationsClient(amc.RootScope, &aztoken.AnonymousCredential{}, amc.ClientOptions)
	if err != nil {
		return corerpv20220315.ApplicationResource{}, err
	}

	getResponse, err := client.Get(ctx, applicationName, &corerpv20220315.ApplicationsClientGetOptions{})
	var result corerpv20220315.ApplicationResource
	if err != nil {
		return result, err
	}
	result = getResponse.ApplicationResource
	return result, nil
}

// # Function Explanation
//
// DeleteApplication deletes an application and all its associated resources, and returns an error if any of the operations fail.
func (amc *UCPApplicationsManagementClient) DeleteApplication(ctx context.Context, applicationName string) (bool, error) {
	// This handles the case where the application doesn't exist.
	resourcesWithApplication, err := amc.ListAllResourcesByApplication(ctx, applicationName)
	if err != nil && !clientv2.Is404Error(err) {
		return false, err
	}

	g, groupCtx := errgroup.WithContext(ctx)
	for _, resource := range resourcesWithApplication {
		resource := resource
		g.Go(func() error {
			_, err := amc.DeleteResource(groupCtx, *resource.Type, *resource.Name)
			if err != nil {
				return err
			}
			return nil
		})
	}

	err = g.Wait()
	if err != nil {
		return false, err
	}

	client, err := corerpv20220315.NewApplicationsClient(amc.RootScope, &aztoken.AnonymousCredential{}, amc.ClientOptions)
	if err != nil {
		return false, err
	}

	var respFromCtx *http.Response
	ctxWithResp := runtime.WithCaptureResponse(ctx, &respFromCtx)

	_, err = client.Delete(ctxWithResp, applicationName, nil)
	if err != nil {
		return false, err
	}

	return respFromCtx.StatusCode != 204, nil
}

// CreateOrUpdateApplication creates or updates an application.
//
// # Function Explanation
//
// CreateOrUpdateApplication creates or updates an application resource in Azure using the
// given application name and resource. It returns an error if the creation or update fails.
func (amc *UCPApplicationsManagementClient) CreateOrUpdateApplication(ctx context.Context, applicationName string, resource corerpv20220315.ApplicationResource) error {
	client, err := corerpv20220315.NewApplicationsClient(amc.RootScope, &aztoken.AnonymousCredential{}, amc.ClientOptions)
	if err != nil {
		return err
	}

	_, err = client.CreateOrUpdate(ctx, applicationName, resource, nil)
	if err != nil {
		return err
	}

	return nil
}

// CreateApplicationIfNotFound creates an application if it does not exist.
//
// # Function Explanation
//
// CreateApplicationIfNotFound checks if an application exists and creates it if it does
// not exist, returning an error if any occurs.
func (amc *UCPApplicationsManagementClient) CreateApplicationIfNotFound(ctx context.Context, applicationName string, resource corerpv20220315.ApplicationResource) error {
	client, err := corerpv20220315.NewApplicationsClient(amc.RootScope, &aztoken.AnonymousCredential{}, amc.ClientOptions)
	if err != nil {
		return err
	}

	_, err = client.Get(ctx, applicationName, nil)
	if Is404Error(err) {
		// continue
	} else if err != nil {
		return err
	} else {
		// Application already exists, nothing to do.
		return nil
	}

	_, err = client.CreateOrUpdate(ctx, applicationName, resource, nil)
	if err != nil {
		return err
	}

	return nil
}

// Creates a radius environment resource
//
// # Function Explanation
//
// CreateEnvironment creates or updates an environment with the given name, location and
// properties, and returns an error if one occurs.
func (amc *UCPApplicationsManagementClient) CreateEnvironment(ctx context.Context, envName string, location string, envProperties *corerpv20220315.EnvironmentProperties) error {
	client, err := corerpv20220315.NewEnvironmentsClient(amc.RootScope, &aztoken.AnonymousCredential{}, amc.ClientOptions)
	if err != nil {
		return err
	}

	_, err = client.CreateOrUpdate(ctx, envName, corerpv20220315.EnvironmentResource{Location: &location, Properties: envProperties}, &corerpv20220315.EnvironmentsClientCreateOrUpdateOptions{})
	if err != nil {
		return err
	}

	return nil

}

func isResourceInApplication(ctx context.Context, resource generated.GenericResource, applicationName string) bool {
	obj, found := resource.Properties["application"]
	// A resource may not have an application associated with it.
	if !found {
		return false
	}

	associatedAppId, ok := obj.(string)
	if !ok || associatedAppId == "" {
		return false
	}

	idParsed, err := resources.ParseResource(associatedAppId)
	if err != nil {
		return false
	}

	if strings.EqualFold(idParsed.Name(), applicationName) {
		return true
	}

	return false
}

func isResourceInEnvironment(ctx context.Context, resource generated.GenericResource, environmentName string) bool {
	obj, found := resource.Properties["environment"]
	// A resource may not have an environment associated with it.
	if !found {
		return false
	}

	associatedEnvId, ok := obj.(string)
	if !ok || associatedEnvId == "" {
		return false
	}

	idParsed, err := resources.ParseResource(associatedEnvId)
	if err != nil {
		return false
	}

	if strings.EqualFold(idParsed.Name(), environmentName) {
		return true
	}

	return false
}

// # Function Explanation
//
// ListEnvironmentsInResourceGroup creates a list of environment resources by paging through the list of environments in
// the resource group and appending each environment to the list. It returns the list of environment resources or an error
// if one occurs.
func (amc *UCPApplicationsManagementClient) ListEnvironmentsInResourceGroup(ctx context.Context) ([]corerpv20220315.EnvironmentResource, error) {
	envResourceList := []corerpv20220315.EnvironmentResource{}

	envClient, err := corerpv20220315.NewEnvironmentsClient(amc.RootScope, &aztoken.AnonymousCredential{}, amc.ClientOptions)
	if err != nil {
		return envResourceList, err
	}

	pager := envClient.NewListByScopePager(&corerpv20220315.EnvironmentsClientListByScopeOptions{})
	for pager.More() {
		nextPage, err := pager.NextPage(ctx)
		if err != nil {
			return envResourceList, err
		}
		applicationList := nextPage.EnvironmentResourceList.Value
		for _, application := range applicationList {
			envResourceList = append(envResourceList, *application)
		}
	}

	return envResourceList, nil
}

// # Function Explanation
//
// ListEnvironmentsAll queries the scope for all environment resources and returns a slice of environment resources or an error if one occurs.
func (amc *UCPApplicationsManagementClient) ListEnvironmentsAll(ctx context.Context) ([]corerpv20220315.EnvironmentResource, error) {
	scope, err := resources.ParseScope("/" + amc.RootScope)
	if err != nil {
		return []corerpv20220315.EnvironmentResource{}, err
	}

	// Query at plane scope, not resource group scope. We don't enforce the exact structure of the scope, so handle both cases.
	//
	// - /planes/radius/local
	// - /planes/radius/local/resourceGroups/my-group
	if scope.FindScope(resources.ResourceGroupsSegment) != "" {
		scope = scope.Truncate()
	}

	environments := []corerpv20220315.EnvironmentResource{}
	client, err := corerpv20220315.NewEnvironmentsClient(scope.String(), &aztoken.AnonymousCredential{}, amc.ClientOptions)
	if err != nil {
		return []corerpv20220315.EnvironmentResource{}, err
	}

	pager := client.NewListByScopePager(&corerpv20220315.EnvironmentsClientListByScopeOptions{})
	for pager.More() {
		nextPage, err := pager.NextPage(ctx)
		if err != nil {
			return []corerpv20220315.EnvironmentResource{}, err
		}

		for _, environment := range nextPage.EnvironmentResourceList.Value {
			environments = append(environments, *environment)
		}
	}

	return environments, nil
}

// # Function Explanation
//
// GetEnvDetails attempts to retrieve an environment resource from an environment client, and returns the environment
// resource or an error if unsuccessful.
func (amc *UCPApplicationsManagementClient) GetEnvDetails(ctx context.Context, envName string) (corerpv20220315.EnvironmentResource, error) {
	envClient, err := corerpv20220315.NewEnvironmentsClient(amc.RootScope, &aztoken.AnonymousCredential{}, amc.ClientOptions)
	if err != nil {
		return corerpv20220315.EnvironmentResource{}, err
	}

	envGetResp, err := envClient.Get(ctx, envName, &corerpv20220315.EnvironmentsClientGetOptions{})
	if err == nil {
		return envGetResp.EnvironmentResource, nil
	}

	return corerpv20220315.EnvironmentResource{}, err

}

// # Function Explanation
//
// DeleteEnv function checks if there are any applications associated with the given environment, deletes them if found,
// and then deletes the environment itself. It returns a boolean and an error if one occurs.
func (amc *UCPApplicationsManagementClient) DeleteEnv(ctx context.Context, envName string) (bool, error) {
	applicationsWithEnv, err := amc.ListApplicationsByEnv(ctx, envName)
	if err != nil {
		return false, err
	}

	for _, application := range applicationsWithEnv {
		_, err := amc.DeleteApplication(ctx, *application.Name)
		if err != nil {
			return false, err
		}
	}

	envClient, err := corerpv20220315.NewEnvironmentsClient(amc.RootScope, &aztoken.AnonymousCredential{}, amc.ClientOptions)
	if err != nil {
		return false, err
	}

	var respFromCtx *http.Response
	ctxWithResp := runtime.WithCaptureResponse(ctx, &respFromCtx)

	_, err = envClient.Delete(ctxWithResp, envName, nil)
	if err != nil {
		return false, err
	}

	return respFromCtx.StatusCode != 204, nil
}

// # Function Explanation
//
// CreateUCPGroup creates a new resource group in the specified plane type and plane name using the provided resource
// group resource and returns an error if one occurs.
func (amc *UCPApplicationsManagementClient) CreateUCPGroup(ctx context.Context, planeType string, planeName string, resourceGroupName string, resourceGroup ucpv20220901.ResourceGroupResource) error {
	var resourceGroupOptions *ucpv20220901.ResourceGroupsClientCreateOrUpdateOptions
	resourcegroupClient, err := ucpv20220901.NewResourceGroupsClient(&aztoken.AnonymousCredential{}, amc.ClientOptions)
	if err != nil {
		return err
	}

	_, err = resourcegroupClient.CreateOrUpdate(ctx, planeType, planeName, resourceGroupName, resourceGroup, resourceGroupOptions)
	if err != nil {
		return err
	}

	return nil
}

// # Function Explanation
//
// DeleteUCPGroup attempts to delete a UCP resource group using the provided plane type, plane name and resource group
// name, and returns a boolean indicating success or failure and an error if one occurs.
func (amc *UCPApplicationsManagementClient) DeleteUCPGroup(ctx context.Context, planeType string, planeName string, resourceGroupName string) (bool, error) {
	var resourceGroupOptions *ucpv20220901.ResourceGroupsClientDeleteOptions
	resourcegroupClient, err := ucpv20220901.NewResourceGroupsClient(&aztoken.AnonymousCredential{}, amc.ClientOptions)

	var respFromCtx *http.Response
	ctxWithResp := runtime.WithCaptureResponse(ctx, &respFromCtx)
	if err != nil {
		return false, err
	}

	_, err = resourcegroupClient.Delete(ctxWithResp, planeType, planeName, resourceGroupName, resourceGroupOptions)
	if err != nil {
		return false, err
	}

	return respFromCtx.StatusCode == 204, nil

}

// # Function Explanation
//
// ShowUCPGroup is a function that retrieves a resource group from the Azure Resource Manager using the given plane type,
// plane name and resource group name, and returns the resource group resource or an error if one occurs.
func (amc *UCPApplicationsManagementClient) ShowUCPGroup(ctx context.Context, planeType string, planeName string, resourceGroupName string) (ucpv20220901.ResourceGroupResource, error) {
	var resourceGroupOptions *ucpv20220901.ResourceGroupsClientGetOptions
	resourcegroupClient, err := ucpv20220901.NewResourceGroupsClient(&aztoken.AnonymousCredential{}, amc.ClientOptions)
	if err != nil {
		return ucpv20220901.ResourceGroupResource{}, err
	}

	resp, err := resourcegroupClient.Get(ctx, planeType, planeName, resourceGroupName, resourceGroupOptions)
	if err != nil {
		return ucpv20220901.ResourceGroupResource{}, err
	}

	return resp.ResourceGroupResource, nil
}

// # Function Explanation
//
// ListUCPGroup is a function that retrieves a list of resource groups from the UCP API and returns them as a slice of
// ResourceGroupResource objects. It may return an error if there is an issue with the API request.
func (amc *UCPApplicationsManagementClient) ListUCPGroup(ctx context.Context, planeType string, planeName string) ([]ucpv20220901.ResourceGroupResource, error) {
	var resourceGroupOptions *ucpv20220901.ResourceGroupsClientListByRootScopeOptions
	resourceGroupResources := []ucpv20220901.ResourceGroupResource{}
	resourcegroupClient, err := ucpv20220901.NewResourceGroupsClient(&aztoken.AnonymousCredential{}, amc.ClientOptions)
	if err != nil {
		return resourceGroupResources, err
	}

	pager := resourcegroupClient.NewListByRootScopePager(planeType, planeName, resourceGroupOptions)

	for pager.More() {
		resp, err := pager.NextPage(ctx)
		if err != nil {
			return resourceGroupResources, err
		}

		resourceGroupList := resp.Value
		for _, resourceGroup := range resourceGroupList {
			resourceGroupResources = append(resourceGroupResources, *resourceGroup)

		}
	}

	return resourceGroupResources, nil
}

// # Function Explanation
//
// ShowRecipe creates a new EnvironmentsClient, gets the recipe metadata from the
// environment, and returns the EnvironmentRecipeProperties or an error if one occurs.
func (amc *UCPApplicationsManagementClient) ShowRecipe(ctx context.Context, environmentName string, recipeName corerpv20220315.Recipe) (corerpv20220315.EnvironmentRecipeProperties, error) {
	client, err := corerpv20220315.NewEnvironmentsClient(amc.RootScope, &aztoken.AnonymousCredential{}, amc.ClientOptions)
	if err != nil {
		return corerpv20220315.EnvironmentRecipeProperties{}, err
	}

	resp, err := client.GetRecipeMetadata(ctx, environmentName, recipeName, &corerpv20220315.EnvironmentsClientGetRecipeMetadataOptions{})
	if err != nil {
		return corerpv20220315.EnvironmentRecipeProperties{}, err
	}

	return corerpv20220315.EnvironmentRecipeProperties(resp.EnvironmentRecipeProperties), nil
}
