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
	"github.com/Azure/go-autorest/autorest/validation"
)

type UCPDeploymentsClient struct {
	resources.DeploymentsClient
}

// NewDeploymentsClientWithBaseURI creates an instance of the DeploymentsClient client using a custom endpoint.  Use
// this when interacting with an Azure cloud that uses a non-standard base URI (sovereign clouds, Azure stack).
func NewUCPDeploymentsClientWithBaseURI(baseURI string) UCPDeploymentsClient {
	return UCPDeploymentsClient{newDeploymentsClientWithBaseURI(baseURI, "")}
}

func (client UCPDeploymentsClient) GetAutorestClient() autorest.Client {
	return client.Client
}

func (client UCPDeploymentsClient) GetDeploymentClient() resources.DeploymentsClient {
	return client.DeploymentsClient
}

func (client UCPDeploymentsClient) CreateOrUpdate(ctx context.Context, resourceGroupName string, deploymentName string, parameters resources.Deployment) (result resources.DeploymentsCreateOrUpdateFuture, err error) {

	if err := validation.Validate([]validation.Validation{
		{TargetValue: resourceGroupName,
			Constraints: []validation.Constraint{{Target: "resourceGroupName", Name: validation.MaxLength, Rule: 90, Chain: nil},
				{Target: "resourceGroupName", Name: validation.MinLength, Rule: 1, Chain: nil}}},
		{TargetValue: deploymentName,
			Constraints: []validation.Constraint{{Target: "deploymentName", Name: validation.MaxLength, Rule: 64, Chain: nil},
				{Target: "deploymentName", Name: validation.MinLength, Rule: 1, Chain: nil}}},
		{TargetValue: parameters,
			Constraints: []validation.Constraint{{Target: "parameters.Properties", Name: validation.Null, Rule: true,
				Chain: []validation.Constraint{{Target: "parameters.Properties.ParametersLink", Name: validation.Null, Rule: false,
					Chain: []validation.Constraint{{Target: "parameters.Properties.ParametersLink.URI", Name: validation.Null, Rule: true, Chain: nil}}},
				}}}}}); err != nil {
		return result, validation.NewError("resources.DeploymentsClient", "CreateOrUpdate", err.Error())
	}

	req, err := client.UCPCreateOrUpdatePreparer(ctx, resourceGroupName, deploymentName, parameters)
	if err != nil {
		err = autorest.NewErrorWithError(err, "resources.DeploymentsClient", "CreateOrUpdate", nil, "Failure preparing request")
		return
	}

	result, err = client.CreateOrUpdateSender(req)
	if err != nil {
		err = autorest.NewErrorWithError(err, "resources.DeploymentsClient", "CreateOrUpdate", nil, "Failure sending request")
		return
	}
	return
}

// CreateOrUpdatePreparer prepares the CreateOrUpdate request.
func (client UCPDeploymentsClient) UCPCreateOrUpdatePreparer(ctx context.Context, resourceGroupName string, deploymentName string, parameters resources.Deployment) (*http.Request, error) {
	pathParameters := map[string]interface{}{
		"deploymentName":    autorest.Encode("path", deploymentName),
		"resourceGroupName": autorest.Encode("path", resourceGroupName),
	}

	const APIVersion = "2020-10-01"
	queryParameters := map[string]interface{}{
		"api-version": APIVersion,
	}

	preparer := autorest.CreatePreparer(
		autorest.AsContentType("application/json; charset=utf-8"),
		autorest.AsPut(),
		autorest.WithBaseURL(client.BaseURI),
		autorest.WithPathParameters("/resourcegroups/{resourceGroupName}/providers/Microsoft.Resources/deployments/{deploymentName}", pathParameters),
		autorest.WithJSON(parameters),
		autorest.WithQueryParameters(queryParameters))
	return preparer.Prepare((&http.Request{}).WithContext(ctx))
}
