// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package ucp

import (
	"context"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	azclient "github.com/project-radius/radius/pkg/azure/clients"
	"github.com/project-radius/radius/pkg/cli/clients"
	"github.com/project-radius/radius/pkg/cli/clients_new/generated"
	"github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
	corerp "github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"golang.org/x/sync/errgroup"
)

type ARMApplicationsManagementClient struct {
	Connection *arm.Connection
	RootScope  string
}

var _ clients.ApplicationsManagementClient = (*ARMApplicationsManagementClient)(nil)

var (
	ResourceTypesList = []string{
		"Applications.Connector/mongoDatabases",
		"Applications.Connector/rabbitMQMessageQueues",
		"Applications.Connector/redisCaches",
		"Applications.Connector/sqlDatabases",
		"Applications.Connector/daprStateStores",
		"Applications.Connector/daprSecretStores",
		"Applications.Connector/daprPubSubBrokers",
		"Applications.Connector/daprInvokeHttpRoutes",
		"Applications.Connector/extenders",
		"Applications.Core/gateways",
		"Applications.Core/httpRoutes",
		"Applications.Core/containers",
	}
)

// ListAllResourcesByType lists the all the resources within a scope
func (amc *ARMApplicationsManagementClient) ListAllResourcesByType(ctx context.Context, resourceType string) ([]generated.GenericResource, error) {
	results := []generated.GenericResource{}
	client := generated.NewGenericResourcesClient(amc.Connection, amc.RootScope, resourceType)
	pager := client.ListByRootScope(nil)
	if pager.Err() != nil {
		return nil, pager.Err()
	}
	for pager.NextPage(ctx) {
		resourceList := pager.PageResponse().GenericResourcesList.Value
		for _, resource := range resourceList {
			results = append(results, *resource)
		}
	}
	return results, nil
}

// ListAllResourceOfTypeInApplication lists the resources of a particular type in an application
func (amc *ARMApplicationsManagementClient) ListAllResourceOfTypeInApplication(ctx context.Context, applicationName string, resourceType string) ([]generated.GenericResource, error) {
	results := []generated.GenericResource{}
	resourceList, err := amc.ListAllResourcesByType(ctx, resourceType)
	if err != nil {
		return nil, err
	}
	for _, resource := range resourceList {
		isResourceWithApplication := isResourceWithApplication(ctx, resource, applicationName)
		if isResourceWithApplication {
			results = append(results, resource)
		}
	}
	return results, nil
}

// ListAllResourcesByApplication lists the resources of a particular application
func (amc *ARMApplicationsManagementClient) ListAllResourcesByApplication(ctx context.Context, applicationName string) ([]generated.GenericResource, error) {
	results := []generated.GenericResource{}
	for _, resourceType := range ResourceTypesList {
		resourceList, err := amc.ListAllResourceOfTypeInApplication(ctx, applicationName, resourceType)
		if err != nil {
			return nil, err
		}
		results = append(results, resourceList...)
	}

	return results, nil
}

func (amc *ARMApplicationsManagementClient) ShowResource(ctx context.Context, resourceType string, resourceName string) (generated.GenericResource, error) {
	client := generated.NewGenericResourcesClient(amc.Connection, amc.RootScope, resourceType)
	getResponse, err := client.Get(ctx, resourceName, &generated.GenericResourcesGetOptions{})
	if err != nil {
		return generated.GenericResource{}, err
	}
	return getResponse.GenericResource, nil
}

func (amc *ARMApplicationsManagementClient) DeleteResource(ctx context.Context, resourceType string, resourceName string) (generated.GenericResourcesDeleteResponse, error) {
	client := generated.NewGenericResourcesClient(amc.Connection, amc.RootScope, resourceType)
	return client.Delete(ctx, resourceName, nil)
}

func (amc *ARMApplicationsManagementClient) ListApplications(ctx context.Context) ([]v20220315privatepreview.ApplicationResource, error) {
	results := []v20220315privatepreview.ApplicationResource{}
	client := v20220315privatepreview.NewApplicationsClient(amc.Connection, amc.RootScope)
	pager := client.ListByScope(nil)
	for pager.NextPage(ctx) {
		if pager.Err() != nil {
			return nil, pager.Err()
		}
		applicationList := pager.PageResponse().ApplicationResourceList.Value
		for _, application := range applicationList {
			results = append(results, *application)
		}
	}
	return results, nil
}

func (amc *ARMApplicationsManagementClient) ListApplicationsByEnv(ctx context.Context, envName string) ([]v20220315privatepreview.ApplicationResource, error) {
	results := []v20220315privatepreview.ApplicationResource{}
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

func (amc *ARMApplicationsManagementClient) ShowApplication(ctx context.Context, applicationName string) (v20220315privatepreview.ApplicationResource, error) {
	client := v20220315privatepreview.NewApplicationsClient(amc.Connection, amc.RootScope)
	getResponse, err := client.Get(ctx, applicationName, &corerp.ApplicationsGetOptions{})
	var result v20220315privatepreview.ApplicationResource
	if err != nil {
		return result, err
	}
	result = getResponse.ApplicationResource
	return result, nil
}

func (amc *ARMApplicationsManagementClient) DeleteApplication(ctx context.Context, applicationName string) (v20220315privatepreview.ApplicationsDeleteResponse, error) {
	resourcesWithApplication, err := amc.ListAllResourcesByApplication(ctx, applicationName)

	//This handles errors received from server and ignores 404 related to scope
	if err != nil && !azclient.Is404Error(err) {
		return v20220315privatepreview.ApplicationsDeleteResponse{}, err
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
		return v20220315privatepreview.ApplicationsDeleteResponse{}, err
	}
	client := v20220315privatepreview.NewApplicationsClient(amc.Connection, amc.RootScope)
	response, err := client.Delete(ctx, applicationName, nil)
	if err != nil && !azclient.Is404Error(err) {
		return v20220315privatepreview.ApplicationsDeleteResponse{}, err
	}
	return response, nil
}

func isResourceWithApplication(ctx context.Context, resource generated.GenericResource, applicationName string) bool {
	obj, found := resource.Properties["application"]
	// A resource may not have an application associated with it.
	if !found {
		return false
	}
	associatedAppId, ok := obj.(string)
	if !ok || associatedAppId == "" {
		return false
	}
	idParsed, err := resources.Parse(associatedAppId)
	if err != nil {
		return false
	}

	if strings.EqualFold(idParsed.Name(), applicationName) {
		return true
	}
	return false
}

func (amc *ARMApplicationsManagementClient) ListEnv(ctx context.Context) ([]corerp.EnvironmentResource, error) {

	envClient := corerp.NewEnvironmentsClient(amc.Connection, amc.RootScope)
	envListPager := envClient.ListByScope(&corerp.EnvironmentsListByScopeOptions{})
	envResourceList := []corerp.EnvironmentResource{}
	for envListPager.NextPage(ctx) {
		currEnvPage := envListPager.PageResponse().EnvironmentResourceList.Value
		for _, env := range currEnvPage {
			envResourceList = append(envResourceList, *env)
		}
	}

	return envResourceList, nil

}

func (amc *ARMApplicationsManagementClient) GetEnvDetails(ctx context.Context, envName string) (corerp.EnvironmentResource, error) {

	envClient := corerp.NewEnvironmentsClient(amc.Connection, amc.RootScope)
	envGetResp, err := envClient.Get(ctx, envName, &corerp.EnvironmentsGetOptions{})
	if err == nil {
		return envGetResp.EnvironmentsGetResult.EnvironmentResource, nil
	}

	return corerp.EnvironmentResource{}, err

}

func (amc *ARMApplicationsManagementClient) DeleteEnv(ctx context.Context, envName string) error {
	applicationsWithEnv, err := amc.ListApplicationsByEnv(ctx, envName)
	if err != nil {
		return err
	}
	for _, application := range applicationsWithEnv {
		_, err := amc.DeleteApplication(ctx, *application.Name)
		if err != nil {
			return err
		}
	}
	envClient := corerp.NewEnvironmentsClient(amc.Connection, amc.RootScope)

	_, err = envClient.Delete(ctx, envName, &corerp.EnvironmentsDeleteOptions{})
	if err != nil {
		return err
	}

	return nil

}