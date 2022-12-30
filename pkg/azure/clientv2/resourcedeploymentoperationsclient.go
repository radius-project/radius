// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package clientv2

import (
	"context"

	armruntime "github.com/Azure/azure-sdk-for-go/sdk/azcore/arm/runtime"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
)

// DeploymentOperationsClient is an operations client which takes in a resourceID as the destination to query.
// It is used by both Azure and UCP clients.
type DeploymentOperationsClient struct {
	client   *armresources.DeploymentOperationsClient
	pipeline *runtime.Pipeline
	baseURI  string
}

// NewDeploymentsClient creates an instance of the DeploymentsClient.
func NewDeploymentOperationsClient(subscriptionID string, options *Options) (*DeploymentOperationsClient, error) {
	baseURI := DefaultBaseURI
	if options.BaseURI != "" {
		baseURI = options.BaseURI
	}

	client, err := armresources.NewDeploymentOperationsClient(subscriptionID, options.Cred, defaultClientOptions)
	if err != nil {
		return nil, err
	}

	pipeline, err := armruntime.NewPipeline(ModuleName, ModuleVersion, options.Cred, runtime.PipelineOptions{}, defaultClientOptions)
	if err != nil {
		return nil, err
	}

	return &DeploymentOperationsClient{
		client:   client,
		pipeline: &pipeline,
		baseURI:  baseURI,
	}, nil
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
	pager := client.client.NewListPager(resourceGroupName, deploymentName,
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
