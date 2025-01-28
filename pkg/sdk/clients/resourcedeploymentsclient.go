/*
Copyright 2023 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package clients

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	armruntime "github.com/Azure/azure-sdk-for-go/sdk/azcore/arm/runtime"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
	"github.com/radius-project/radius/pkg/ucp/resources"
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
	Template any `json:"template,omitempty"`
	// TemplateLink - The URI of the template. Use either the templateLink property or the template property, but not both.
	TemplateLink *armresources.TemplateLink `json:"templateLink,omitempty"`
	// ProviderConfig specifies the scope for resources
	ProviderConfig any `json:"providerconfig,omitempty"`
	// Parameters - Name and value pairs that define the deployment parameters for the template. You use this element when you want to provide the parameter values directly in the request rather than link to an existing parameter file. Use either the parametersLink property or the parameters property, but not both. It can be a JObject or a well formed JSON string.
	Parameters any `json:"parameters,omitempty"`
	// ParametersLink - The URI of parameters file. You use this element to link to an existing parameters file. Use either the parametersLink property or the parameters property, but not both.
	ParametersLink *armresources.ParametersLink `json:"parametersLink,omitempty"`
	// Mode - The mode that is used to deploy resources. This value can be either Incremental or Complete. In Incremental mode, resources are deployed without deleting existing resources that are not included in the template. In Complete mode, resources are deployed and existing resources in the resource group that are not included in the template are deleted. Be careful when using Complete mode as you may unintentionally delete resources. Possible values include: 'DeploymentModeIncremental', 'DeploymentModeComplete'
	Mode armresources.DeploymentMode `json:"mode,omitempty"`
	// DebugSetting - The debug setting of the deployment.
	DebugSetting *armresources.DebugSetting `json:"debugSetting,omitempty"`
	// OnErrorDeployment - The deployment on error behavior.
	OnErrorDeployment *armresources.OnErrorDeployment `json:"onErrorDeployment,omitempty"`
	// ExpressionEvaluationOptions - Specifies whether template expressions are evaluated within the scope of the parent template or nested template. Only applicable to nested templates. If not specified, default value is outer.
	ExpressionEvaluationOptions *armresources.ExpressionEvaluationOptions `json:"expressionEvaluationOptions,omitempty"`
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

// ResourceDeploymentsClient is a deployments client for Azure Resource Manager.
// It is used by both Azure and UCP clients.
type ResourceDeploymentsClient interface {
	CreateOrUpdate(ctx context.Context, parameters Deployment, resourceID, apiVersion string) (Poller[ClientCreateOrUpdateResponse], error)
	ContinueCreateOperation(ctx context.Context, resumeToken string) (Poller[ClientCreateOrUpdateResponse], error)
	Delete(ctx context.Context, resourceID, apiVersion string) (Poller[ClientDeleteResponse], error)
	ContinueDeleteOperation(ctx context.Context, resumeToken string) (Poller[ClientDeleteResponse], error)
}

type ResourceDeploymentsClientImpl struct {
	client   *armresources.Client
	pipeline *runtime.Pipeline
	baseURI  string
}

var _ ResourceDeploymentsClient = (*ResourceDeploymentsClientImpl)(nil)

// NewResourceDeploymentsClient creates a new ResourceDeploymentsClient with the provided options and returns an error if
// the options are invalid.
func NewResourceDeploymentsClient(options *Options) (ResourceDeploymentsClient, error) {
	if options.BaseURI == "" {
		return nil, errors.New("baseURI cannot be empty")
	}

	// SubscriptionID will be empty for this type of client.
	client, err := armresources.NewClient("", options.Cred, options.ARMClientOptions)
	if err != nil {
		return nil, err
	}

	pipeline, err := armruntime.NewPipeline(ModuleName, ModuleVersion, options.Cred, runtime.PipelineOptions{}, options.ARMClientOptions)
	if err != nil {
		return nil, err
	}

	return &ResourceDeploymentsClientImpl{
		client:   client,
		pipeline: &pipeline,
		baseURI:  options.BaseURI,
	}, nil
}

// ClientCreateOrUpdateResponse contains the response from method Client.CreateOrUpdate.
type ClientCreateOrUpdateResponse struct {
	armresources.DeploymentExtended
}

// ClientDeleteResponse contains the response from method Client.Delete.
type ClientDeleteResponse struct {
	armresources.DeploymentExtended
}

// CreateOrUpdate creates a request to create or update a deployment and returns a poller to
// track the progress of the operation.
func (client *ResourceDeploymentsClientImpl) CreateOrUpdate(ctx context.Context, parameters Deployment, resourceID, apiVersion string) (Poller[ClientCreateOrUpdateResponse], error) {
	if !strings.HasPrefix(resourceID, "/") {
		return nil, fmt.Errorf("error creating or updating a deployment: resourceID must start with a slash")
	}

	_, err := resources.ParseResource(resourceID)
	if err != nil {
		return nil, fmt.Errorf("invalid resourceID: %v", resourceID)
	}

	req, err := client.createOrUpdateCreateRequest(ctx, resourceID, apiVersion, parameters)
	if err != nil {
		return nil, err
	}

	resp, err := client.pipeline.Do(req)
	if err != nil {
		return nil, err
	}
	if !runtime.HasStatusCode(resp, http.StatusOK, http.StatusCreated, http.StatusAccepted) {
		return nil, runtime.NewResponseError(resp)
	}

	return runtime.NewPoller[ClientCreateOrUpdateResponse](resp, *client.pipeline, nil)
}

// createOrUpdateCreateRequest creates the CreateOrUpdate request.
func (client *ResourceDeploymentsClientImpl) createOrUpdateCreateRequest(ctx context.Context, resourceID, apiVersion string, parameters Deployment) (*policy.Request, error) {
	if resourceID == "" {
		return nil, errors.New("resourceID cannot be empty")
	}

	urlPath := DeploymentEngineURL(client.baseURI, resourceID)
	req, err := runtime.NewRequest(ctx, http.MethodPut, urlPath)
	if err != nil {
		return nil, err
	}
	reqQP := req.Raw().URL.Query()
	reqQP.Set("api-version", apiVersion)
	req.Raw().URL.RawQuery = reqQP.Encode()
	req.Raw().Header["Accept"] = []string{"application/json"}
	return req, runtime.MarshalAsJSON(req, parameters)
}

// ContinueCreateOperation continues a create operation given a resume token.
func (client *ResourceDeploymentsClientImpl) ContinueCreateOperation(ctx context.Context, resumeToken string) (Poller[ClientCreateOrUpdateResponse], error) {
	return runtime.NewPollerFromResumeToken[ClientCreateOrUpdateResponse](resumeToken, *client.pipeline, nil)
}

// Delete creates a request to delete a resource and returns a poller to
// track the progress of the operation.
func (client *ResourceDeploymentsClientImpl) Delete(ctx context.Context, resourceID, apiVersion string) (Poller[ClientDeleteResponse], error) {
	if !strings.HasPrefix(resourceID, "/") {
		return nil, fmt.Errorf("error creating or updating a deployment: resourceID must start with a slash")
	}

	_, err := resources.ParseResource(resourceID)
	if err != nil {
		return nil, fmt.Errorf("invalid resourceID: %v", resourceID)
	}

	req, err := client.deleteCreateRequest(ctx, resourceID, apiVersion)
	if err != nil {
		return nil, err
	}

	resp, err := client.pipeline.Do(req)
	if err != nil {
		return nil, err
	}
	if !runtime.HasStatusCode(resp, http.StatusOK, http.StatusNoContent, http.StatusAccepted, http.StatusNotFound) {
		return nil, runtime.NewResponseError(resp)
	}

	return runtime.NewPoller[ClientDeleteResponse](resp, *client.pipeline, nil)
}

// deleteCreateRequest creates the Delete request.
func (client *ResourceDeploymentsClientImpl) deleteCreateRequest(ctx context.Context, resourceID, apiVersion string) (*policy.Request, error) {
	if resourceID == "" {
		return nil, errors.New("resourceID cannot be empty")
	}

	urlPath := DeploymentEngineURL(client.baseURI, resourceID)
	req, err := runtime.NewRequest(ctx, http.MethodDelete, urlPath)
	if err != nil {
		return nil, err
	}
	reqQP := req.Raw().URL.Query()
	reqQP.Set("api-version", apiVersion)
	req.Raw().URL.RawQuery = reqQP.Encode()
	req.Raw().Header["Accept"] = []string{"application/json"}
	return req, nil
}

// ContinueCreateOperation continues a create operation given a resume token.
func (client *ResourceDeploymentsClientImpl) ContinueDeleteOperation(ctx context.Context, resumeToken string) (Poller[ClientDeleteResponse], error) {
	return runtime.NewPollerFromResumeToken[ClientDeleteResponse](resumeToken, *client.pipeline, nil)
}
