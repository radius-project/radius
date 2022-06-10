// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package azure

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/project-radius/radius/pkg/cli/clients"
	"github.com/project-radius/radius/pkg/connectorrp/api/v20220315privatepreview"
	"github.com/project-radius/radius/pkg/ucp/resources"
)

//TODO: Change subId and ResourceId to scope
type ARMUCPManagementClient struct {
	Connection      *arm.Connection
	ResourceGroup   string
	SubscriptionID  string
	EnvironmentName string
}

var _ clients.FirstPartyServiceManagementClient = (*ARMUCPManagementClient)(nil)

// ListAllResourcesByApplication lists the resources of a particular application
func (um *ARMUCPManagementClient) ListAllResourcesByApplication(ctx context.Context, applicationName string) ([]v20220315privatepreview.Resource, error) {
	applicationList := []v20220315privatepreview.Resource{}
	mongoResourceList, err := getMongoResources(um.Connection, um.SubscriptionID, um.ResourceGroup, ctx)
	if err != nil {
		return nil, err
	}
	//filter by application name
	for _, mongoResource := range mongoResourceList {
		currAppParsedName, err := resources.Parse(*mongoResource.Properties.Application)
		if err != nil {
			return nil, err
		}
		if currAppParsedName.Name() == applicationName {
			applicationList = append(applicationList, mongoResource.Resource)
		}
	}
	return applicationList, nil
}

func getMongoResources(con *arm.Connection, subscriptionId string, resourceGroupName string, ctx context.Context) ([]v20220315privatepreview.MongoDatabaseResource, error) {
	// get all mongo resources
	mongoclient := v20220315privatepreview.NewMongoDatabasesClient(con, "00000000-0000-0000-0000-000000000000")
	mongoPager := mongoclient.List("radius-test-rg", nil)
	mongoResourceList := []v20220315privatepreview.MongoDatabaseResource{}
	for mongoPager.NextPage(ctx) {
		currResourceList := mongoPager.PageResponse().MongoDatabaseList.Value
		for _, resource := range currResourceList {
			mongoResourceList = append(mongoResourceList, *resource)
		}
	}
	return mongoResourceList, nil
}

func getResourceAppName(applicationId string) (string, error) {
	parsedId, err := resources.Parse(applicationId)
	if err != nil {
		return "", err
	}
	return parsedId.Name(), nil
}
