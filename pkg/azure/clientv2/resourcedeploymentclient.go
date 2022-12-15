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

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	armruntime "github.com/Azure/azure-sdk-for-go/sdk/azcore/arm/runtime"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
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
	Template interface{} `json:"template,omitempty"`
	// TemplateLink - The URI of the template. Use either the templateLink property or the template property, but not both.
	TemplateLink *armresources.TemplateLink `json:"templateLink,omitempty"`
	//ProviderConfig specifies the scope for resources
	ProviderConfig interface{} `json:"providerconfig,omitempty"`
	// Parameters - Name and value pairs that define the deployment parameters for the template. You use this element when you want to provide the parameter values directly in the request rather than link to an existing parameter file. Use either the parametersLink property or the parameters property, but not both. It can be a JObject or a well formed JSON string.
	Parameters interface{} `json:"parameters,omitempty"`
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

// DeploymentsClient is a deployments client for Azure Resource Manager.
// It is used by both Azure and UCP clients.
type DeploymentsClient struct {
	*armresources.DeploymentsClient
	Pipeline *runtime.Pipeline
	BaseURI  string
}

// NewDeploymentsClient creates an instance of the DeploymentsClient using the default endpoint.
func NewDeploymentsClient(cred azcore.TokenCredential, subscriptionID string) (*DeploymentsClient, error) {
	client, err := NewDeploymentsClientWithBaseURI(cred, subscriptionID, DefaultBaseURI)
	if err != nil {
		return nil, err
	}

	return client, err
}

// NewDeploymentsClientWithBaseURI creates an instance of the DeploymentsClient using a custom endpoint.
// Use this when interacting with UCP or Azure resources that uses a non-standard base URI.
func NewDeploymentsClientWithBaseURI(credential azcore.TokenCredential, subscriptionID string, baseURI string) (*DeploymentsClient, error) {
	options := &arm.ClientOptions{
		ClientOptions: azcore.ClientOptions{
			Cloud: cloud.Configuration{
				Services: map[cloud.ServiceName]cloud.ServiceConfiguration{
					cloud.ResourceManager: {
						Endpoint: baseURI,
					},
				},
			},
		},
	}
	client, err := armresources.NewDeploymentsClient(subscriptionID, credential, options)
	if err != nil {
		return nil, err
	}

	pipeline, err := armruntime.NewPipeline(moduleName, moduleVersion, credential, runtime.PipelineOptions{}, options)
	if err != nil {
		return nil, err
	}

	return &DeploymentsClient{
		DeploymentsClient: client,
		Pipeline:          &pipeline,
		BaseURI:           baseURI,
	}, nil
}

type ClientBeginCreateOrUpdateOptions struct {
	resourceID  string
	resumeToken string
	apiVersion  string
}

func NewClientBeginCreateOrUpdateOptions(resourceID, resumeToken, apiVersion string) *ClientBeginCreateOrUpdateOptions {
	// FIXME: This is to validate the resourceID.
	_, err := resources.ParseResource(resourceID)
	if err != nil {
		return nil
	}

	return &ClientBeginCreateOrUpdateOptions{
		resourceID:  resourceID,
		resumeToken: resumeToken,
		apiVersion:  apiVersion,
	}
}

// ClientCreateOrUpdateResponse contains the response from method Client.CreateOrUpdate.
type ClientCreateOrUpdateResponse struct {
	armresources.GenericResource
}

// CreateOrUpdate creates a deployment or updates the existing deployment.
func (client *DeploymentsClient) BeginCreateOrUpdate(ctx context.Context, parameters Deployment, options *ClientBeginCreateOrUpdateOptions) (*runtime.Poller[ClientCreateOrUpdateResponse], error) {
	// TODO: resourceID needs to be parsed to see if it is valid
	if !strings.HasPrefix(options.resourceID, "/") {
		return nil, fmt.Errorf("error creating or updating a deployment: resourceID must start with a slash")
	}

	_, err := resources.ParseResource(options.resourceID)
	if err != nil {
		return nil, fmt.Errorf("invalid resourceID: %v", options.resourceID)
	}

	resp, err := client.createOrUpdate(ctx, parameters, options)
	if err != nil {
		return nil, err
	}
	return runtime.NewPoller[ClientCreateOrUpdateResponse](resp, *client.Pipeline, nil)
}

// CreateOrUpdate - Creates a resource.
// If the operation fails it returns an *azcore.ResponseError type.
// Generated from API version 2021-04-01
func (client *DeploymentsClient) createOrUpdate(ctx context.Context, parameters Deployment, options *ClientBeginCreateOrUpdateOptions) (*http.Response, error) {
	req, err := client.createOrUpdateCreateRequest(ctx, parameters, options)
	if err != nil {
		return nil, err
	}
	resp, err := client.Pipeline.Do(req)
	if err != nil {
		return nil, err
	}
	if !runtime.HasStatusCode(resp, http.StatusOK, http.StatusCreated, http.StatusAccepted) {
		return nil, runtime.NewResponseError(resp)
	}
	return resp, nil
}

// createOrUpdateCreateRequest creates the CreateOrUpdate request.
func (client *DeploymentsClient) createOrUpdateCreateRequest(ctx context.Context, parameters Deployment, options *ClientBeginCreateOrUpdateOptions) (*policy.Request, error) {
	urlPath := "/{resourceID}"
	if options.resourceID == "" {
		return nil, errors.New("resourceID cannot be empty")
	}
	urlPath = strings.ReplaceAll(urlPath, "{resourceID}", url.PathEscape(options.resourceID))

	req, err := runtime.NewRequest(ctx, http.MethodPut, runtime.JoinPaths(client.BaseURI, urlPath))
	if err != nil {
		return nil, err
	}
	reqQP := req.Raw().URL.Query()
	reqQP.Set("api-version", options.apiVersion)
	req.Raw().URL.RawQuery = reqQP.Encode()
	req.Raw().Header["Accept"] = []string{"application/json"}
	return req, runtime.MarshalAsJSON(req, parameters)
}
