// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package clientv2

import (
	"context"
	"errors"
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

type CustomActionClient struct {
	*BaseClient
}

// NewCustomActionClient creates an instance of the CustomActionClient with the default Base URI.
func NewCustomActionClient(subscriptionID string, credential azcore.TokenCredential) (*BaseClient, error) {
	client, err := NewCustomActionClientWithBaseURI(DefaultBaseURI, subscriptionID, credential)
	if err != nil {
		return nil, err
	}

	return client, err
}

// NewCustomActionClientWithBaseURI creates an instance of the CustomActionClient with a Base URI.
func NewCustomActionClientWithBaseURI(baseURI string, subscriptionID string, credential azcore.TokenCredential) (*BaseClient, error) {
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

	client, err := armresources.NewClient(subscriptionID, credential, options)
	if err != nil {
		return nil, err
	}

	pipeline, err := armruntime.NewPipeline(moduleName, moduleVersion, credential, runtime.PipelineOptions{}, options)
	if err != nil {
		return nil, err
	}

	return &BaseClient{
		Client:   client,
		Pipeline: &pipeline,
		BaseURI:  baseURI,
	}, nil
}

type ClientCustomActionResponse struct {
	armresources.GenericResource
}

type ClientBeginCustomActionOptions struct {
	resourceID string
	action     string
	apiVersion string
}

// New creates an instance of the CustomActionClient with the default Base URI.
func NewCustomActionRequestOptions(resourceID, action, apiVersion string) *ClientBeginCustomActionOptions {
	// FIXME: This is to validate the resourceID.
	_, err := resources.ParseResource(resourceID)
	if err != nil {
		return nil
	}

	return &ClientBeginCustomActionOptions{
		resourceID: resourceID,
		action:     action,
		apiVersion: apiVersion,
	}
}

func (client *CustomActionClient) BeginCustomAction(ctx context.Context, opts *ClientBeginCustomActionOptions) (*runtime.Poller[ClientCustomActionResponse], error) {
	resp, err := client.customAction(ctx, opts)
	if err != nil {
		return nil, err
	}

	// FIXME: Is this the right way?
	return runtime.NewPoller[ClientCustomActionResponse](resp, *client.Pipeline, nil)
}

func (client *CustomActionClient) customAction(ctx context.Context, opts *ClientBeginCustomActionOptions) (*http.Response, error) {
	req, err := client.customActionCreateRequest(ctx, opts)
	if err != nil {
		return nil, err
	}

	resp, err := client.Pipeline.Do(req)
	if err != nil {
		return nil, err
	}
	if !runtime.HasStatusCode(resp, http.StatusAccepted, http.StatusNoContent) {
		return nil, runtime.NewResponseError(resp)
	}
	return resp, nil
}

func (client *CustomActionClient) customActionCreateRequest(ctx context.Context, opts *ClientBeginCustomActionOptions) (*policy.Request, error) {
	urlPath := "/{resourceID}/{action}"
	if opts.resourceID == "" {
		return nil, errors.New("resourceID cannot be empty")
	}
	urlPath = strings.ReplaceAll(urlPath, "{resourceID}", url.PathEscape(opts.resourceID))

	if opts.action == "" {
		return nil, errors.New("action cannot be empty")
	}
	urlPath = strings.ReplaceAll(urlPath, "{action}", url.PathEscape(opts.action))

	// FIXME: Is joining BaseURI and URLPath going to give us a wrong URL?
	req, err := runtime.NewRequest(ctx, http.MethodPost, runtime.JoinPaths(client.BaseURI, urlPath))
	if err != nil {
		return nil, err
	}
	reqQP := req.Raw().URL.Query()
	reqQP.Set("api-version", opts.apiVersion)
	req.Raw().URL.RawQuery = reqQP.Encode()
	req.Raw().Header["Accept"] = []string{"application/json"}
	return req, runtime.MarshalAsJSON(req, nil)
}
