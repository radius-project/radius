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
	"github.com/vmware/vmware-go-kcl/logger"
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
func (amc *ARMApplicationsManagementClient) ListAllResourcesByApplication(ctx context.Context, applicationName string) ([]generated.GenericResource, error) {
	results := []generated.GenericResource{}
	for _, resourceType := range resourceTypesList {
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

func isResourceWithApplication(ctx context.Context, resource generated.GenericResource, applicationName string) (bool, error) {
	log := logr.FromContextOrDiscard(ctx)
	obj, found := resource.Properties["application"]
	// A resource will always have an application associated.
	// This is a required field while creating a resource.
	// Additionally in case of connectors the resources are not required to have an application attached
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
