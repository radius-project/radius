// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package clients

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/resources/mgmt/resources"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/to"
)

// ResourceDeploymentOperationsClient is an operations client which takes in a resourceID as the destination to query.
// It is used by both Azure and UCP clients.
type ResourceDeploymentOperationsClient struct {
	resources.DeploymentOperationsClient
}

// NewResourceDeploymentOperationsClientWithBaseURI creates an instance of the ResourceOperations client using a custom endpoint.  Use
// this when interacting with UCP resources that uses a non-standard base URI.
func NewResourceDeploymentOperationsClientWithBaseURI(baseURI string) ResourceDeploymentOperationsClient {
	return ResourceDeploymentOperationsClient{NewOperationsClientWithBaseUri(baseURI, "")}
}

// List gets all deployments operations for a deployment.
// Parameters:
// resourceId - the resourceId to deploy to. NOTE, must start with a '/'. Ex: "/resourcegroups/{resourceGroupName}/deployments/{deploymentName}/operations
// top - the number of results to return.
func (client ResourceDeploymentOperationsClient) List(ctx context.Context, resourceID string, top *int32) (resources.DeploymentOperationsListResultPage, error) {
	fn := client.listNextResults

	if !strings.HasPrefix(resourceID, "/") {
		return resources.NewDeploymentOperationsListResultPage(resources.DeploymentOperationsListResult{}, fn), errors.New("resourceID must contain starting slash")
	}

	req, err := client.ListPreparer(ctx, resourceID, top)
	if err != nil {
		err = autorest.NewErrorWithError(err, "resources.DeploymentOperationsClient", "List", nil, "Failure preparing request")
		return resources.NewDeploymentOperationsListResultPage(resources.DeploymentOperationsListResult{}, fn), err
	}

	resp, err := client.ListSender(req)
	if err != nil {
		dolr := resources.DeploymentOperationsListResult{
			Response: autorest.Response{Response: resp},
		}
		err = autorest.NewErrorWithError(err, "resources.DeploymentOperationsClient", "List", resp, "Failure sending request")
		return resources.NewDeploymentOperationsListResultPage(dolr, fn), err
	}

	dolr, err := client.ListResponder(resp)
	if err != nil {
		err = autorest.NewErrorWithError(err, "resources.DeploymentOperationsClient", "List", resp, "Failure responding to request")
		return resources.NewDeploymentOperationsListResultPage(dolr, fn), err
	}

	res := resources.NewDeploymentOperationsListResultPage(dolr, fn)

	if dolr.NextLink != nil && len(*dolr.NextLink) != 0 && dolr.IsEmpty() {
		err = res.NextWithContext(ctx)
		return res, err
	}

	return res, nil
}

// ListPreparer prepares the List request.
func (client ResourceDeploymentOperationsClient) ListPreparer(ctx context.Context, resourceId string, top *int32) (*http.Request, error) {
	const APIVersion = "2020-10-01"
	queryParameters := map[string]any{
		"api-version": APIVersion,
	}
	if top != nil {
		queryParameters["$top"] = autorest.Encode("query", *top)
	}

	preparer := autorest.CreatePreparer(
		autorest.AsGet(),
		autorest.WithBaseURL(client.BaseURI),
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
	resp, err := client.ListSender(req)
	if err != nil {
		result.Response = autorest.Response{Response: resp}
		return result, autorest.NewErrorWithError(err, "resources.DeploymentOperationsClient", "listNextResults", resp, "Failure sending next results request")
	}
	result, err = client.ListResponder(resp)
	if err != nil {
		err = autorest.NewErrorWithError(err, "resources.DeploymentOperationsClient", "listNextResults", resp, "Failure responding to next results request")
	}
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
