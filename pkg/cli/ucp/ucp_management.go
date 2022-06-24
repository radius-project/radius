// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package ucp

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/go-logr/logr"
	"github.com/project-radius/radius/pkg/cli/clients"
	"github.com/project-radius/radius/pkg/cli/clients_new/generated"
	"github.com/project-radius/radius/pkg/radlogger"
	"github.com/project-radius/radius/pkg/ucp/resources"
)

type ARMApplicationsManagementClient struct {
	EnvironmentName string
	Connection      *arm.Connection
	RootScope       string
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

// ListAllResourcesByApplication lists the resources of a particular application
func (amc *ARMApplicationsManagementClient) ListAllResourcesByApplication(ctx context.Context, applicationName string) ([]generated.GenericResource, error) {
	results := []generated.GenericResource{}
	for _, resourceType := range ResourceTypesList {
		client := generated.NewGenericResourcesClient(amc.Connection, amc.RootScope, resourceType)
		pager := client.ListByRootScope(nil)
		for pager.NextPage(ctx) {
			resourceList := pager.PageResponse().GenericResourcesList.Value
			for _, resource := range resourceList {
				isResourceWithApplication, err := isResourceWithApplication(ctx, *resource, applicationName)
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

func (amc *ARMApplicationsManagementClient) ShowResourceByApplication(ctx context.Context, applicationName string, resourceType string) ([]generated.GenericResource, error) {
	results := []generated.GenericResource{}
	client := generated.NewGenericResourcesClient(amc.Connection, amc.RootScope, resourceType)
	pager := client.ListByRootScope(nil)
	for pager.NextPage(ctx) {
		resourceList := pager.PageResponse().GenericResourcesList.Value
		for _, resource := range resourceList {
			isResourceWithApplication, err := isResourceWithApplication(ctx, *resource, applicationName)
			if err != nil {
				return nil, err
			}
			if isResourceWithApplication {
				results = append(results, *resource)
			}
		}
	}
	return results, nil
}

func (um *ARMApplicationsManagementClient) DeleteResource(ctx context.Context, resourceType string, resourceName string) (generated.GenericResourcesDeleteResponse, error) {
	client := generated.NewGenericResourcesClient(um.Connection, um.RootScope, resourceType)
	return client.Delete(ctx, resourceName, nil)
}

func isResourceWithApplication(ctx context.Context, resource generated.GenericResource, applicationName string) (bool, error) {
	log := logr.FromContextOrDiscard(ctx)

	obj, found := resource.Properties["application"]
	// A resource may not have an application associated with it.
	if !found {
		return false, nil
	}
	associatedAppId, ok := obj.(string)
	if !ok {
		log.V(radlogger.Warn).Info("Failed to list resources in the application. Resource with invalid application id found.")
		return false, nil
	}
	idParsed, err := resources.Parse(associatedAppId)

	if err != nil {
		return false, err
	}
	if idParsed.Name() == applicationName {
		return true, nil
	}
	return false, nil
}
