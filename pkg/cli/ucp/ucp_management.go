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
)

type ARMUCPManagementClient struct {
	EnvironmentName string
	Connection      *arm.Connection
	RootScope       string
}

var _ clients.AppManagementClient = (*ARMUCPManagementClient)(nil)

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
func (um *ARMUCPManagementClient) ListAllResourcesByApplication(ctx context.Context, applicationName string) ([]v20220315privatepreview.Resource, error) {
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
