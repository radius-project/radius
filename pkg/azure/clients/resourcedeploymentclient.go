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
	"github.com/project-radius/radius/pkg/providers"
)

// ResourceDeploymentClient is a deployment client which takes in a resourceID as the destination to deploy to.
// It is used by both Azure and UCP clients
type ResourceDeploymentClient struct {
	resources.DeploymentsClient
}

// NewDeploymentsClientWithBaseURI creates an instance of the UCPDeploymentsClient client using a custom endpoint.  Use
// this when interacting with UCP or Azure resources that uses a non-standard base URI
func NewResourceDeploymentClientWithBaseURI(baseURI string) ResourceDeploymentClient {
	return ResourceDeploymentClient{NewDeploymentsClientWithBaseURI(baseURI, "")}
}

// CreateOrUpdate creates a deployment
// resourceId - the resourceId to deploy to. NOTE, must start with a '/'. Ex: "/resourcegroups/{resourceGroupName}/deployments/{deploymentName}/operations
func (client ResourceDeploymentClient) CreateOrUpdate(ctx context.Context, resourceID string, parameters providers.Deployment) (result resources.DeploymentsCreateOrUpdateFuture, err error) {
	if !strings.HasPrefix(resourceID, "/") {
		err = errors.New("resourceID must contain starting slash")
		return
	}

	req, err := client.ResourceCreateOrUpdatePreparer(ctx, resourceID, parameters)
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

// CreateOrUpdate creates a deployment
// resourceId - the resourceId to deploy to. NOTE, must start with a '/'. Ex: "/resourcegroups/{resourceGroupName}/deployments/{deploymentName}/operations
func (client ResourceDeploymentClient) CreateOrUpdateLegacy(ctx context.Context, resourceID string, parameters resources.Deployment) (result resources.DeploymentsCreateOrUpdateFuture, err error) {
	if !strings.HasPrefix(resourceID, "/") {
		err = errors.New("resourceID must contain starting slash")
		return
	}

	req, err := client.ResourceCreateOrUpdatePreparerLegacy(ctx, resourceID, parameters)
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
func (client ResourceDeploymentClient) ResourceCreateOrUpdatePreparer(ctx context.Context, resourceID string, parameters providers.Deployment) (*http.Request, error) {
	const APIVersion = "2020-10-01"
	queryParameters := map[string]interface{}{
		"api-version": APIVersion,
	}

	preparer := autorest.CreatePreparer(
		autorest.AsContentType("application/json; charset=utf-8"),
		autorest.AsPut(),
		autorest.WithBaseURL(client.BaseURI),
		autorest.WithPath(resourceID),
		autorest.WithJSON(parameters),
		autorest.WithQueryParameters(queryParameters))
	return preparer.Prepare((&http.Request{}).WithContext(ctx))
}

// CreateOrUpdatePreparer prepares the CreateOrUpdate request.
func (client ResourceDeploymentClient) ResourceCreateOrUpdatePreparerLegacy(ctx context.Context, resourceID string, parameters resources.Deployment) (*http.Request, error) {
	const APIVersion = "2020-10-01"
	queryParameters := map[string]interface{}{
		"api-version": APIVersion,
	}

	preparer := autorest.CreatePreparer(
		autorest.AsContentType("application/json; charset=utf-8"),
		autorest.AsPut(),
		autorest.WithBaseURL(client.BaseURI),
		autorest.WithPath(resourceID),
		autorest.WithJSON(parameters),
		autorest.WithQueryParameters(queryParameters))
	return preparer.Prepare((&http.Request{}).WithContext(ctx))
}
