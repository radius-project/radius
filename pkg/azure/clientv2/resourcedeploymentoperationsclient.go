// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package clientv2

import (
	"context"
	"net/http"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/resources/mgmt/resources"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/to"
)

// ResourceDeploymentOperationsClient is an operations client which takes in a resourceID as the destination to query.
// It is used by both Azure and UCP clients.
type ResourceDeploymentOperationsClient struct {
	armresources.DeploymentOperationsClient
}

// NewResourceDeploymentOperationsClientWithBaseURI creates an instance of the ResourceOperations client using a custom endpoint.  Use
// this when interacting with UCP resources that uses a non-standard base URI.
func NewResourceDeploymentOperationsClientWithBaseURI(subscriptionID string, baseURI string) (*ResourceDeploymentOperationsClient, error) {
	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, err
	}

	client, err := armresources.NewDeploymentOperationsClient(subscriptionID, cred, nil)
	if err != nil {
		return nil, err
	}

	return &ResourceDeploymentOperationsClient{*client}, nil
}

// List gets all deployments operations for a deployment.
// Parameters:
// resourceId - the resourceId to deploy to. NOTE, must start with a '/'. Ex: "/resourcegroups/{resourceGroupName}/deployments/{deploymentName}/operations
// top - the number of results to return.
func (client *ResourceDeploymentOperationsClient) List(ctx context.Context, resourceGroupName string, deploymentName string, resourceID string, top *int32) (*armresources.DeploymentOperationsListResult, error) {
	result := &armresources.DeploymentOperationsListResult{
		Value:    make([]*armresources.DeploymentOperation, 0),
		NextLink: to.StringPtr(""),
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
		for _, deploymentOperation := range deploymentOperationsList {
			result.Value = append(result.Value, *&deploymentOperation)
		}
	}
	return nil, nil
}

// ListPreparer prepares the List request.
func (client ResourceDeploymentOperationsClient) ListPreparer(ctx context.Context, resourceId string, top *int32) (*http.Request, error) {
	const APIVersion = "2020-10-01"
	queryParameters := map[string]interface{}{
		"api-version": APIVersion,
	}
	if top != nil {
		queryParameters["$top"] = autorest.Encode("query", *top)
	}

	preparer := autorest.CreatePreparer(
		autorest.AsGet(),
		// autorest.WithBaseURL(client.BaseURI),
		autorest.WithPath(resourceId+"/operations"),
		autorest.WithQueryParameters(queryParameters))
	return preparer.Prepare((&http.Request{}).WithContext(ctx))
}

// listNextResults retrieves the next set of results, if any.
func (client ResourceDeploymentOperationsClient) listNextResults(ctx context.Context, lastResults resources.DeploymentOperationsListResult) (result resources.DeploymentOperationsListResult, err error) {
	req, err := deploymentOperationsListResultPreparer(lastResults, ctx)
	if err != nil {
		return result, autorest.NewErrorWithError(err, "resources.DeploymentOperationsClient", "listNextResults", nil, "Failure preparing next results request")
	}
	if req == nil {
		return
	}

	// TODO: Find out what to use here.
	// resp, err := client.ListSender(req)
	// if err != nil {
	// 	result.Response = autorest.Response{Response: resp}
	// 	return result, autorest.NewErrorWithError(err, "resources.DeploymentOperationsClient", "listNextResults", resp, "Failure sending next results request")
	// }
	// result, err = client.ListResponder(resp)
	// if err != nil {
	// 	err = autorest.NewErrorWithError(err, "resources.DeploymentOperationsClient", "listNextResults", resp, "Failure responding to next results request")
	// }
	return
}

// deploymentOperationsListResultPreparer prepares a request to retrieve the next set of results.
// It returns nil if no more results exist.
func deploymentOperationsListResultPreparer(dolr resources.DeploymentOperationsListResult, ctx context.Context) (*http.Request, error) {
	if dolr.NextLink == nil || len(*dolr.NextLink) == 0 {
		return nil, nil
	}
	return autorest.Prepare((&http.Request{}).WithContext(ctx),
		autorest.AsJSON(),
		autorest.AsGet(),
		autorest.WithBaseURL(to.String(dolr.NextLink)))
}
