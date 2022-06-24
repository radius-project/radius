// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package ucp

import (
	"context"
	"errors"

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

// ListAllResourcesByApplication lists the resources of a particular application
func (um *ARMApplicationsManagementClient) ListAllResourcesByApplication(ctx context.Context, applicationName string) ([]generated.GenericResource, error) {
	results := []generated.GenericResource{}
	for _, resourceType := range resourceTypesList {
		client := generated.NewGenericResourcesClient(um.Connection, um.RootScope, resourceType)
		pager := client.ListByRootScope(nil)
		for pager.NextPage(ctx) {
			resourceList := pager.PageResponse().GenericResourcesList.Value
			for _, resource := range resourceList {
				isResourceWithApplication, err := isResourceWithApplication(*resource, applicationName)
				if err != nil {
					return nil, err
				}
				if isResourceWithApplication {
					results = append(results, *resource)
				}
			}
		}
	}
	return results, nil
}

func isResourceWithApplication(resource generated.GenericResource, applicationName string) (bool, error) {

	obj, found := resource.Properties["application"]
	if !found {
		return false, errors.New("Resource isn't associated to an application")
	}
	associatedAppName, ok := obj.(string)
	if !ok {
		return false, errors.New("Invalid app")
	}
	idParsed, err := resources.Parse(associatedAppName)
	if err != nil {
		return false, err
	}
	if idParsed.Name() == applicationName {
		return true, nil
	}
	return false, nil
}
