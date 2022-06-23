// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package ucp

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/project-radius/radius/pkg/cli/clients"
	"github.com/project-radius/radius/pkg/cli/clients_new/generated"
	"github.com/project-radius/radius/pkg/ucp/resources"
)

type ARMApplicationsManagementClient struct {
	EnvironmentName string
	Connection      *arm.Connection
	RootScope       string
}

var _ clients.ApplicationsManagementClient = (*ARMApplicationsManagementClient)(nil)

var (
	resourceTypesList = []string{
		"Applications.Connector/mongoDatabases",
		"Applications.Connector/mongoDatabases",
		"Applications.Connector/rabbitMQMessageQueues",
		"Applications.Connector/redisCaches",
		"Applications.Connector/sqlDatabases",
		"Applications.Connector/daprStateStores",
		"Applications.Connector/daprSecretStores",
		"Applications.Connector/daprPubSubBrokers",
		"Applications.Connector/daprInvokeHttpRoutes",
		"Applications.Core/gateways",
		"Applications.Core/httpRoutes",
	}
)

///{rootScope}/providers/Applications.Connector/mongoDatabases/{mongoDatabaseName}
// ListAllResourcesByApplication lists the resources of a particular application
func (um *ARMApplicationsManagementClient) ListAllResourcesByApplication(ctx context.Context, applicationName string) ([]generated.GenericResource, error) {
	resourceListByApplication := []generated.GenericResource{}
	for _, resourceType := range resourceTypesList {
		client := generated.NewGenericResourcesClient(um.Connection, um.RootScope, resourceType)
		pager := client.ListByRootScope(nil)
		for pager.NextPage(ctx) {
			resourceList := pager.PageResponse().GenericResourcesList.Value
			for _, resource := range resourceList {
				resourceListByApplication = append(resourceListByApplication, *resource)
			}
		}
	}
	return resourceListByApplication, nil
}

func filterByApplicationName(resourceList []generated.GenericResource, applicationName string) ([]generated.GenericResource, error) {
	filteredResourceList := []generated.GenericResource{}
	for _, resource := range resourceList {
		IdParsed, err := resources.Parse(*resource.ID)
		if err != nil {
			return nil, err
		}
		if IdParsed.Name() == applicationName {
			filteredResourceList = append(filteredResourceList, resource)
		}
	}
	return filteredResourceList, nil
}
