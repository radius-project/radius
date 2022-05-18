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
)

// ResourceDeploymentClient is a deployment client which takes in a resourceID as the destination to deploy to.
// It is used by both Azure and UCP clients
type ResourceDeploymentClient struct {
	resources.DeploymentsClient
}

// NewDeploymentsClientWithBaseURI creates an instance of the UCPDeploymentsClient client using a custom endpoint.  Use
// this when interacting with UCP resources that uses a non-standard base URI
func NewResourceDeploymentClientWithBaseURI(baseURI string) ResourceDeploymentClient {
	return ResourceDeploymentClient{newResourceDeploymentClientWithBaseURI(baseURI, "")}
}

// CreateOrUpdate creates a deployment
// resourceId - the resourceId to deploy to. NOTE, must start with a '/'. Ex: "/resourcegroups/{resourceGroupName}/deployments/{deploymentName}/operations
func (client ResourceDeploymentClient) CreateOrUpdate(ctx context.Context, resourceId string, parameters resources.Deployment) (result resources.DeploymentsCreateOrUpdateFuture, err error) {
	req, err := client.ResourceCreateOrUpdatePreparer(ctx, resourceId, parameters)
	if err != nil {
		err = autorest.NewErrorWithError(err, "ResourceDeploymentClient", "CreateOrUpdate", nil, "Failure preparing request")
		return
	}

	result, err = client.CreateOrUpdateSender(req)
	if err != nil {
		err = autorest.NewErrorWithError(err, "ResourceDeploymentClient", "CreateOrUpdate", nil, "Failure sending request")
		return
	}
	return
}

// CreateOrUpdatePreparer prepares the CreateOrUpdate request.
func (client ResourceDeploymentClient) ResourceCreateOrUpdatePreparer(ctx context.Context, resourceId string, parameters resources.Deployment) (*http.Request, error) {
	const APIVersion = "2020-10-01"
	queryParameters := map[string]interface{}{
		"api-version": APIVersion,
	}

	preparer := autorest.CreatePreparer(
		autorest.AsContentType("application/json; charset=utf-8"),
		autorest.AsPut(),
		autorest.WithBaseURL(client.BaseURI),
		autorest.WithPath(resourceId),
		autorest.WithJSON(parameters),
		autorest.WithQueryParameters(queryParameters))
	return preparer.Prepare((&http.Request{}).WithContext(ctx))
}
