// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package clientv2

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	armruntime "github.com/Azure/azure-sdk-for-go/sdk/azcore/arm/runtime"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
	"github.com/project-radius/radius/pkg/ucp/resources"
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
	//ProviderConfig specifies the scope for resources
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

// ResourceDeploymentClient is a deployment client for Azure Resource Manager.
// It is used by both Azure and UCP clients.
type ResourceDeploymentClient struct {
	client   *armresources.DeploymentsClient
	pipeline *runtime.Pipeline
	baseURI  string
}

// NewResourceDeploymentClient creates an instance of the ResourceDeploymentClient.
func NewResourceDeploymentClient(subscriptionID string, options *Options) (*ResourceDeploymentClient, error) {
	baseURI := DefaultBaseURI
	if options.BaseURI != "" {
		baseURI = options.BaseURI
	}

	client, err := armresources.NewDeploymentsClient(subscriptionID, options.Cred, defaultClientOptions)
	if err != nil {
		return nil, err
	}

	pipeline, err := armruntime.NewPipeline(ModuleName, ModuleVersion, options.Cred, runtime.PipelineOptions{}, defaultClientOptions)
	if err != nil {
		return nil, err
	}

	return &ResourceDeploymentClient{
		client:   client,
		pipeline: &pipeline,
		baseURI:  baseURI,
	}, nil
}

// ClientCreateOrUpdateResponse contains the response from method Client.CreateOrUpdate.
type ClientCreateOrUpdateResponse struct {
	armresources.GenericResource
}

// CreateOrUpdate creates a deployment or updates the existing deployment.
func (client *ResourceDeploymentClient) CreateOrUpdate(ctx context.Context, parameters Deployment, resourceID, apiVersion string) (*runtime.Poller[ClientCreateOrUpdateResponse], error) {
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
func (client *ResourceDeploymentClient) createOrUpdateCreateRequest(ctx context.Context, resourceID, apiVersion string, parameters Deployment) (*policy.Request, error) {
	if resourceID == "" {
		return nil, errors.New("resourceID cannot be empty")
	}

	urlPath := runtime.JoinPaths(client.baseURI, url.PathEscape(resourceID))
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
