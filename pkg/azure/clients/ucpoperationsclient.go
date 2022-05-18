// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package clients

import (
	"context"
	"net/http"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/resources/mgmt/resources"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/Azure/go-autorest/autorest/validation"
)

type UCPOperationsClient struct {
	resources.DeploymentOperationsClient
}

// NewDeploymentsClientWithBaseURI creates an instance of the DeploymentsClient client using a custom endpoint.  Use
// this when interacting with an Azure cloud that uses a non-standard base URI (sovereign clouds, Azure stack).
func NewUCPOperationsClientWithBaseURI(baseURI string) UCPOperationsClient {
	return UCPOperationsClient{NewOperationsClientWithBaseUri(baseURI, "")}
}

// List gets all deployments operations for a deployment.
// Parameters:
// resourceGroupName - the name of the resource group. The name is case insensitive.
// deploymentName - the name of the deployment.
// top - the number of results to return.
func (client UCPOperationsClient) List(ctx context.Context, resourceGroupName string, deploymentName string, top *int32) (resources.DeploymentOperationsListResultPage, error) {

	if err := validation.Validate([]validation.Validation{
		{TargetValue: resourceGroupName,
			Constraints: []validation.Constraint{{Target: "resourceGroupName", Name: validation.MaxLength, Rule: 90, Chain: nil},
				{Target: "resourceGroupName", Name: validation.MinLength, Rule: 1, Chain: nil}}},
		{TargetValue: deploymentName,
			Constraints: []validation.Constraint{{Target: "deploymentName", Name: validation.MaxLength, Rule: 64, Chain: nil},
				{Target: "deploymentName", Name: validation.MinLength, Rule: 1, Chain: nil}}}}); err != nil {
		return resources.NewDeploymentOperationsListResultPage(resources.DeploymentOperationsListResult{}, nil), validation.NewError("resources.DeploymentOperationsClient", "List", err.Error())
	}

	fn := client.listNextResults

	req, err := client.ListUCPPreparer(ctx, resourceGroupName, deploymentName, top)
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
func (client UCPOperationsClient) ListUCPPreparer(ctx context.Context, resourceGroupName string, deploymentName string, top *int32) (*http.Request, error) {
	pathParameters := map[string]interface{}{
		"deploymentName":    autorest.Encode("path", deploymentName),
		"resourceGroupName": autorest.Encode("path", resourceGroupName),
	}

	const APIVersion = "2020-10-01"
	queryParameters := map[string]interface{}{
		"api-version": APIVersion,
	}
	if top != nil {
		queryParameters["$top"] = autorest.Encode("query", *top)
	}

	preparer := autorest.CreatePreparer(
		autorest.AsGet(),
		autorest.WithBaseURL(client.BaseURI),
		autorest.WithPathParameters("/resourcegroups/{resourceGroupName}/deployments/{deploymentName}/operations", pathParameters),
		autorest.WithQueryParameters(queryParameters))
	return preparer.Prepare((&http.Request{}).WithContext(ctx))
}

// listNextResults retrieves the next set of results, if any.
func (client UCPOperationsClient) listNextResults(ctx context.Context, lastResults resources.DeploymentOperationsListResult) (result resources.DeploymentOperationsListResult, err error) {
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
	if dolr.NextLink != nil && len(*dolr.NextLink) != 0 {
		return nil, nil
	}
	return autorest.Prepare((&http.Request{}).WithContext(ctx),
		autorest.AsJSON(),
		autorest.AsGet(),
		autorest.WithBaseURL(to.String(dolr.NextLink)))
}
