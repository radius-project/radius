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
)

type Deployment struct {
	// Location - The location to store the deployment data.
	Location *string `json:"location,omitempty"`
	// Properties - The deployment properties.
	Properties *DeploymentProperties `json:"properties,omitempty"`
	// Tags - Deployment tags
	Tags map[string]*string `json:"tags"`
}

// DeploymentProperties deployment properties.
type DeploymentProperties struct {
	// Template - The template content. You use this element when you want to pass the template syntax directly in the request rather than link to an existing template. It can be a JObject or well-formed JSON string. Use either the templateLink property or the template property, but not both.
	Template interface{} `json:"template,omitempty"`
	// TemplateLink - The URI of the template. Use either the templateLink property or the template property, but not both.
	TemplateLink *resources.TemplateLink `json:"templateLink,omitempty"`
	//ProviderConfig specifies the scope for resources
	ProviderConfig interface{} `json:"providerconfig,omitempty"`
	// Parameters - Name and value pairs that define the deployment parameters for the template. You use this element when you want to provide the parameter values directly in the request rather than link to an existing parameter file. Use either the parametersLink property or the parameters property, but not both. It can be a JObject or a well formed JSON string.
	Parameters interface{} `json:"parameters,omitempty"`
	// ParametersLink - The URI of parameters file. You use this element to link to an existing parameters file. Use either the parametersLink property or the parameters property, but not both.
	ParametersLink *resources.ParametersLink `json:"parametersLink,omitempty"`
	// Mode - The mode that is used to deploy resources. This value can be either Incremental or Complete. In Incremental mode, resources are deployed without deleting existing resources that are not included in the template. In Complete mode, resources are deployed and existing resources in the resource group that are not included in the template are deleted. Be careful when using Complete mode as you may unintentionally delete resources. Possible values include: 'DeploymentModeIncremental', 'DeploymentModeComplete'
	Mode resources.DeploymentMode `json:"mode,omitempty"`
	// DebugSetting - The debug setting of the deployment.
	DebugSetting *resources.DebugSetting `json:"debugSetting,omitempty"`
	// OnErrorDeployment - The deployment on error behavior.
	OnErrorDeployment *resources.OnErrorDeployment `json:"onErrorDeployment,omitempty"`
	// ExpressionEvaluationOptions - Specifies whether template expressions are evaluated within the scope of the parent template or nested template. Only applicable to nested templates. If not specified, default value is outer.
	ExpressionEvaluationOptions *resources.ExpressionEvaluationOptions `json:"expressionEvaluationOptions,omitempty"`
}

type Value struct {
	Scope string `json:"scope,omitempty"`
}

type Radius struct {
	Type  string `json:"type,omitempty"`
	Value Value  `json:"value,omitempty"`
}

type Az struct {
	Type  string `json:"type,omitempty"`
	Value Value  `json:"value,omitempty"`
}

type AWS struct {
	Type  string `json:"type,omitempty"`
	Value Value  `json:"value,omitempty"`
}

type Deployments struct {
	Type  string `json:"type,omitempty"`
	Value Value  `json:"value,omitempty"`
}

type ProviderConfig struct {
	Radius      *Radius      `json:"radius,omitempty"`
	Az          *Az          `json:"az,omitempty"`
	AWS         *AWS         `json:"aws,omitempty"`
	Deployments *Deployments `json:"deployments,omitempty"`
}

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
func (client ResourceDeploymentClient) CreateOrUpdate(ctx context.Context, resourceID string, parameters Deployment) (result resources.DeploymentsCreateOrUpdateFuture, err error) {
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

// CreateOrUpdatePreparer prepares the CreateOrUpdate request.
func (client ResourceDeploymentClient) ResourceCreateOrUpdatePreparer(ctx context.Context, resourceID string, parameters Deployment) (*http.Request, error) {
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
