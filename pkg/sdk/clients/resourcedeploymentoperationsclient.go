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
	"net/http"
	"strconv"

	"github.com/project-radius/radius/pkg/to"

	armruntime "github.com/Azure/azure-sdk-for-go/sdk/azcore/arm/runtime"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
)

// ResourceDeploymentOperationsClient is an operations client which takes in a resourceID as the destination to query.
// It is used by both Azure and UCP clients.
type ResourceDeploymentOperationsClient struct {
	client   *armresources.Client
	pipeline *runtime.Pipeline
	baseURI  string
}

// NewResourceDeploymentOperationsClient creates a new ResourceDeploymentOperationsClient with the provided options and
// returns it, or returns an error if the client creation fails.
func NewResourceDeploymentOperationsClient(options *Options) (*ResourceDeploymentOperationsClient, error) {
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

	return &ResourceDeploymentOperationsClient{
		client:   client,
		pipeline: &pipeline,
		baseURI:  options.BaseURI,
	}, nil
}

// List retrieves a list of deployment operations for a given resource ID and API version. It returns an error if the list retrieval fails.
// Parameters:
// resourceId - the resourceId to deploy to. NOTE, must start with a '/'. Ex: "/resourcegroups/{resourceGroupName}/deployments/{deploymentName}/operations
// top - the number of results to return.
func (client *ResourceDeploymentOperationsClient) List(ctx context.Context, resourceGroupName string, deploymentName string, resourceID string, apiVersion string, top *int32) (*armresources.DeploymentOperationsListResult, error) {
	result := &armresources.DeploymentOperationsListResult{
		Value:    make([]*armresources.DeploymentOperation, 0),
		NextLink: to.Ptr(""),
	}

	pager := client.NewListPager(resourceID, apiVersion, &armresources.DeploymentOperationsClientListOptions{
		Top: top,
	})

	for pager.More() {
		nextPage, err := pager.NextPage(ctx)
		if err != nil {
			return result, err
		}
		deploymentOperationsList := nextPage.Value
		result.Value = append(result.Value, deploymentOperationsList...)
	}

	return result, nil
}

// NewListPager creates a pager to iterate over the list of deployment operations for a given resource.
func (client *ResourceDeploymentOperationsClient) NewListPager(resourceID string, apiVersion string, options *armresources.DeploymentOperationsClientListOptions) *runtime.Pager[armresources.DeploymentOperationsClientListResponse] {
	return runtime.NewPager(runtime.PagingHandler[armresources.DeploymentOperationsClientListResponse]{
		More: func(page armresources.DeploymentOperationsClientListResponse) bool {
			return page.NextLink != nil && len(*page.NextLink) > 0
		},
		Fetcher: func(ctx context.Context, page *armresources.DeploymentOperationsClientListResponse) (armresources.DeploymentOperationsClientListResponse, error) {
			var req *policy.Request
			var err error
			if page == nil {
				req, err = client.listCreateRequest(ctx, resourceID, apiVersion, options)
			} else {
				req, err = runtime.NewRequest(ctx, http.MethodGet, *page.NextLink)
			}
			if err != nil {
				return armresources.DeploymentOperationsClientListResponse{}, err
			}
			resp, err := client.pipeline.Do(req)
			if err != nil {
				return armresources.DeploymentOperationsClientListResponse{}, err
			}
			if !runtime.HasStatusCode(resp, http.StatusOK) {
				return armresources.DeploymentOperationsClientListResponse{}, runtime.NewResponseError(resp)
			}
			return client.listHandleResponse(resp)
		},
	})
}

// listCreateRequest creates the List request.
func (client *ResourceDeploymentOperationsClient) listCreateRequest(ctx context.Context, resourceID string, apiVersion string, options *armresources.DeploymentOperationsClientListOptions) (*policy.Request, error) {
	if resourceID == "" {
		return nil, errors.New("resourceID cannot be empty")
	}

	urlPath := DeploymentEngineURL(client.baseURI, resourceID)
	req, err := runtime.NewRequest(ctx, http.MethodGet, urlPath+"/operations")
	if err != nil {
		return nil, err
	}
	reqQP := req.Raw().URL.Query()
	if options != nil && options.Top != nil {
		reqQP.Set("$top", strconv.FormatInt(int64(*options.Top), 10))
	}
	reqQP.Set("api-version", apiVersion)
	req.Raw().URL.RawQuery = reqQP.Encode()
	req.Raw().Header["Accept"] = []string{"application/json"}
	return req, nil
}

// listHandleResponse handles the List response.
func (client *ResourceDeploymentOperationsClient) listHandleResponse(resp *http.Response) (armresources.DeploymentOperationsClientListResponse, error) {
	result := armresources.DeploymentOperationsClientListResponse{}
	if err := runtime.UnmarshalAsJSON(resp, &result.DeploymentOperationsListResult); err != nil {
		return armresources.DeploymentOperationsClientListResponse{}, err
	}
	return result, nil
}
