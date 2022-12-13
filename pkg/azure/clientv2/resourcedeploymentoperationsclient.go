// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package clientv2

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
)

// DeploymentOperationsClient is an operations client which takes in a resourceID as the destination to query.
// It is used by both Azure and UCP clients.
type DeploymentOperationsClient struct {
	armresources.DeploymentOperationsClient
}

// NewDeploymentOperationsClient creates an instance of the DeploymentOperations client using the default endpoint.
func NewDeploymentOperationsClient(cred azcore.TokenCredential, subscriptionID string) (*DeploymentOperationsClient, error) {
	client, err := NewDeploymentOperationsClientWithBaseURI(cred, subscriptionID, DefaultBaseURI)
	if err != nil {
		return nil, err
	}

	return client, err
}

// NewDeploymentOperationsClientWithBaseURI creates an instance of the DeploymentOperations client using a custom endpoint.
// Use this when interacting with UCP resources that uses a non-standard base URI.
func NewDeploymentOperationsClientWithBaseURI(cred azcore.TokenCredential, subscriptionID string, baseURI string) (*DeploymentOperationsClient, error) {
	options := &arm.ClientOptions{
		ClientOptions: azcore.ClientOptions{
			Cloud: cloud.Configuration{
				Services: map[cloud.ServiceName]cloud.ServiceConfiguration{
					cloud.ResourceManager: {
						Endpoint: baseURI,
					},
				},
			},
		},
	}
	client, err := armresources.NewDeploymentOperationsClient(subscriptionID, cred, options)
	if err != nil {
		return nil, err
	}

	return &DeploymentOperationsClient{*client}, nil
}

// List gets all deployments operations for a deployment.
// Parameters:
// resourceId - the resourceId to deploy to. NOTE, must start with a '/'. Ex: "/resourcegroups/{resourceGroupName}/deployments/{deploymentName}/operations
// top - the number of results to return.
func (client *DeploymentOperationsClient) List(ctx context.Context, resourceGroupName string, deploymentName string, resourceID string, top *int32) (*armresources.DeploymentOperationsListResult, error) {
	result := &armresources.DeploymentOperationsListResult{
		Value:    make([]*armresources.DeploymentOperation, 0),
		NextLink: to.Ptr(""),
	}
	// TODO: Validate resourceID
	pager := client.NewListPager(resourceGroupName, deploymentName,
		&armresources.DeploymentOperationsClientListOptions{
			Top: top,
		})

	for pager.More() {
		nextPage, err := pager.NextPage(ctx)
		if err != nil {
			return result, err
		}
		deploymentOperationsList := nextPage.Value
		result.Value = append(result.Value, deploymentOperationsList...)
	}

	return result, nil
}
