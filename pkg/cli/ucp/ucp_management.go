// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package ucp

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/project-radius/radius/pkg/cli/azureresources"
	"github.com/project-radius/radius/pkg/cli/clients"
	"github.com/project-radius/radius/pkg/connectorrp/api/v20220315privatepreview"
	corerp "github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
)

type ARMApplicationsManagementClient struct {
	EnvironmentName string
	Connection      *arm.Connection
	RootScope       string
}

var _ clients.ApplicationsManagementClient = (*ARMApplicationsManagementClient)(nil)

var (
	resourceOperationList = []azureresources.AzureResourceOperationsModel{
		{
			ResourceType:       azureresources.MongoResource,
			ResourceOperations: &azureresources.MongoResourceOperations{},
		},
		{
			ResourceType:       azureresources.RabbitMQResource,
			ResourceOperations: &azureresources.RabbitResourceOperations{},
		},
		{
			ResourceType:       azureresources.RedisResource,
			ResourceOperations: &azureresources.RedisResourceOperations{},
		},
		{
			ResourceType:       azureresources.SQLResource,
			ResourceOperations: &azureresources.SQLResourceOperations{},
		},
	}
)

// ListAllResourcesByApplication lists the resources of a particular application
func (um *ARMApplicationsManagementClient) ListAllResourcesByApplication(ctx context.Context, applicationName string) ([]v20220315privatepreview.Resource, error) {
	rootScope := um.RootScope
	resourceListByApplication := make([]v20220315privatepreview.Resource, 0)
	for _, resourceOperation := range resourceOperationList {
		resourceList, err := resourceOperation.ResourceOperations.GetResourcesByApplication(um.Connection, ctx, rootScope, applicationName)
		if err != nil {
			return nil, err
		}
		resourceListByApplication = append(resourceListByApplication, resourceList...)
	}
	return resourceListByApplication, nil
}

func (um *ARMApplicationsManagementClient) ListEnv(ctx context.Context) ([]corerp.EnvironmentResource, error) {

	envClient := corerp.NewEnvironmentsClient(um.Connection, um.RootScope)
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

func (um *ARMApplicationsManagementClient) GetEnvDetails(ctx context.Context, envName string) (corerp.EnvironmentResource, error) {

	envClient := corerp.NewEnvironmentsClient(um.Connection, um.RootScope)
	envGetResp, err := envClient.Get(ctx, envName, &corerp.EnvironmentsGetOptions{})
	if err == nil {
		return envGetResp.EnvironmentsGetResult.EnvironmentResource, nil
	}

	return corerp.EnvironmentResource{}, err

}
