// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package clients

import (
	"context"
	"errors"

	armruntime "github.com/Azure/azure-sdk-for-go/sdk/azcore/arm/runtime"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
)

// ResourceDeploymentOperationsClient is an operations client which takes in a resourceID as the destination to query.
// It is used by both Azure and UCP clients.
type ResourceDeploymentOperationsClient struct {
	client   *armresources.DeploymentOperationsClient
	pipeline *runtime.Pipeline
	baseURI  string
}

// NewResourceDeploymentOperationsClient creates an instance of the DeploymentsClient.
func NewResourceDeploymentOperationsClient(subscriptionID string, options *Options) (*ResourceDeploymentOperationsClient, error) {
	if options.BaseURI == "" {
		return nil, errors.New("baseURI cannot be empty")
	}

	client, err := armresources.NewDeploymentOperationsClient(subscriptionID, options.Cred, options.ARMClientOptions)
	if err != nil {
		return nil, err
	}

	pipeline, err := armruntime.NewPipeline(ModuleName, ModuleVersion, options.Cred, runtime.PipelineOptions{}, options.ARMClientOptions)
	if err != nil {
		return nil, err
	}

	return &ResourceDeploymentOperationsClient{
		client:   client,
		pipeline: &pipeline,
		baseURI:  options.BaseURI,
	}, nil
}

// List gets all deployments operations for a deployment.
// Parameters:
// resourceId - the resourceId to deploy to. NOTE, must start with a '/'. Ex: "/resourcegroups/{resourceGroupName}/deployments/{deploymentName}/operations
// top - the number of results to return.
func (client *ResourceDeploymentOperationsClient) List(ctx context.Context, resourceGroupName string, deploymentName string, resourceID string, top *int32) (*armresources.DeploymentOperationsListResult, error) {
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
