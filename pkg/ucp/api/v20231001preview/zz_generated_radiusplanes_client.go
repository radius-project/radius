// Licensed under the Apache License, Version 2.0 . See LICENSE in the repository root for license information.
// Code generated by Microsoft (R) AutoRest Code Generator. DO NOT EDIT.
// Changes may cause incorrect behavior and will be lost if the code is regenerated.

package v20231001preview

import (
	"context"
	"errors"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"net/http"
	"net/url"
	"strings"
)

// RadiusPlanesClient contains the methods for the RadiusPlanes group.
// Don't use this type directly, use NewRadiusPlanesClient() instead.
type RadiusPlanesClient struct {
	internal *arm.Client
}

// NewRadiusPlanesClient creates a new instance of RadiusPlanesClient with the specified values.
//   - credential - used to authorize requests. Usually a credential from azidentity.
//   - options - pass nil to accept the default values.
func NewRadiusPlanesClient(credential azcore.TokenCredential, options *arm.ClientOptions) (*RadiusPlanesClient, error) {
	cl, err := arm.NewClient(moduleName, moduleVersion, credential, options)
	if err != nil {
		return nil, err
	}
	client := &RadiusPlanesClient{
	internal: cl,
	}
	return client, nil
}

// BeginCreateOrUpdate - Create or update a plane
// If the operation fails it returns an *azcore.ResponseError type.
//
// Generated from API version 2023-10-01-preview
//   - planeName - The plane name.
//   - resource - Resource create parameters.
//   - options - RadiusPlanesClientBeginCreateOrUpdateOptions contains the optional parameters for the RadiusPlanesClient.BeginCreateOrUpdate
//     method.
func (client *RadiusPlanesClient) BeginCreateOrUpdate(ctx context.Context, planeName string, resource RadiusPlaneResource, options *RadiusPlanesClientBeginCreateOrUpdateOptions) (*runtime.Poller[RadiusPlanesClientCreateOrUpdateResponse], error) {
	if options == nil || options.ResumeToken == "" {
		resp, err := client.createOrUpdate(ctx, planeName, resource, options)
		if err != nil {
			return nil, err
		}
		poller, err := runtime.NewPoller(resp, client.internal.Pipeline(), &runtime.NewPollerOptions[RadiusPlanesClientCreateOrUpdateResponse]{
			FinalStateVia: runtime.FinalStateViaAzureAsyncOp,
			Tracer: client.internal.Tracer(),
		})
		return poller, err
	} else {
		return runtime.NewPollerFromResumeToken(options.ResumeToken, client.internal.Pipeline(), &runtime.NewPollerFromResumeTokenOptions[RadiusPlanesClientCreateOrUpdateResponse]{
			Tracer: client.internal.Tracer(),
		})
	}
}

// CreateOrUpdate - Create or update a plane
// If the operation fails it returns an *azcore.ResponseError type.
//
// Generated from API version 2023-10-01-preview
func (client *RadiusPlanesClient) createOrUpdate(ctx context.Context, planeName string, resource RadiusPlaneResource, options *RadiusPlanesClientBeginCreateOrUpdateOptions) (*http.Response, error) {
	var err error
	const operationName = "RadiusPlanesClient.BeginCreateOrUpdate"
	ctx = context.WithValue(ctx, runtime.CtxAPINameKey{}, operationName)
	ctx, endSpan := runtime.StartSpan(ctx, operationName, client.internal.Tracer(), nil)
	defer func() { endSpan(err) }()
	req, err := client.createOrUpdateCreateRequest(ctx, planeName, resource, options)
	if err != nil {
		return nil, err
	}
	httpResp, err := client.internal.Pipeline().Do(req)
	if err != nil {
		return nil, err
	}
	if !runtime.HasStatusCode(httpResp, http.StatusOK, http.StatusCreated) {
		err = runtime.NewResponseError(httpResp)
		return nil, err
	}
	return httpResp, nil
}

// createOrUpdateCreateRequest creates the CreateOrUpdate request.
func (client *RadiusPlanesClient) createOrUpdateCreateRequest(ctx context.Context, planeName string, resource RadiusPlaneResource, _ *RadiusPlanesClientBeginCreateOrUpdateOptions) (*policy.Request, error) {
	urlPath := "/planes/radius/{planeName}"
	if planeName == "" {
		return nil, errors.New("parameter planeName cannot be empty")
	}
	urlPath = strings.ReplaceAll(urlPath, "{planeName}", url.PathEscape(planeName))
	req, err := runtime.NewRequest(ctx, http.MethodPut, runtime.JoinPaths(client.internal.Endpoint(), urlPath))
	if err != nil {
		return nil, err
	}
	reqQP := req.Raw().URL.Query()
	reqQP.Set("api-version", "2023-10-01-preview")
	req.Raw().URL.RawQuery = reqQP.Encode()
	req.Raw().Header["Accept"] = []string{"application/json"}
	if err := runtime.MarshalAsJSON(req, resource); err != nil {
	return nil, err
}
;	return req, nil
}

// BeginDelete - Delete a plane
// If the operation fails it returns an *azcore.ResponseError type.
//
// Generated from API version 2023-10-01-preview
//   - planeName - The plane name.
//   - options - RadiusPlanesClientBeginDeleteOptions contains the optional parameters for the RadiusPlanesClient.BeginDelete
//     method.
func (client *RadiusPlanesClient) BeginDelete(ctx context.Context, planeName string, options *RadiusPlanesClientBeginDeleteOptions) (*runtime.Poller[RadiusPlanesClientDeleteResponse], error) {
	if options == nil || options.ResumeToken == "" {
		resp, err := client.deleteOperation(ctx, planeName, options)
		if err != nil {
			return nil, err
		}
		poller, err := runtime.NewPoller(resp, client.internal.Pipeline(), &runtime.NewPollerOptions[RadiusPlanesClientDeleteResponse]{
			FinalStateVia: runtime.FinalStateViaLocation,
			Tracer: client.internal.Tracer(),
		})
		return poller, err
	} else {
		return runtime.NewPollerFromResumeToken(options.ResumeToken, client.internal.Pipeline(), &runtime.NewPollerFromResumeTokenOptions[RadiusPlanesClientDeleteResponse]{
			Tracer: client.internal.Tracer(),
		})
	}
}

// Delete - Delete a plane
// If the operation fails it returns an *azcore.ResponseError type.
//
// Generated from API version 2023-10-01-preview
func (client *RadiusPlanesClient) deleteOperation(ctx context.Context, planeName string, options *RadiusPlanesClientBeginDeleteOptions) (*http.Response, error) {
	var err error
	const operationName = "RadiusPlanesClient.BeginDelete"
	ctx = context.WithValue(ctx, runtime.CtxAPINameKey{}, operationName)
	ctx, endSpan := runtime.StartSpan(ctx, operationName, client.internal.Tracer(), nil)
	defer func() { endSpan(err) }()
	req, err := client.deleteCreateRequest(ctx, planeName, options)
	if err != nil {
		return nil, err
	}
	httpResp, err := client.internal.Pipeline().Do(req)
	if err != nil {
		return nil, err
	}
	if !runtime.HasStatusCode(httpResp, http.StatusOK, http.StatusAccepted, http.StatusNoContent) {
		err = runtime.NewResponseError(httpResp)
		return nil, err
	}
	return httpResp, nil
}

// deleteCreateRequest creates the Delete request.
func (client *RadiusPlanesClient) deleteCreateRequest(ctx context.Context, planeName string, _ *RadiusPlanesClientBeginDeleteOptions) (*policy.Request, error) {
	urlPath := "/planes/radius/{planeName}"
	if planeName == "" {
		return nil, errors.New("parameter planeName cannot be empty")
	}
	urlPath = strings.ReplaceAll(urlPath, "{planeName}", url.PathEscape(planeName))
	req, err := runtime.NewRequest(ctx, http.MethodDelete, runtime.JoinPaths(client.internal.Endpoint(), urlPath))
	if err != nil {
		return nil, err
	}
	reqQP := req.Raw().URL.Query()
	reqQP.Set("api-version", "2023-10-01-preview")
	req.Raw().URL.RawQuery = reqQP.Encode()
	req.Raw().Header["Accept"] = []string{"application/json"}
	return req, nil
}

// Get - Get a plane by name
// If the operation fails it returns an *azcore.ResponseError type.
//
// Generated from API version 2023-10-01-preview
//   - planeName - The plane name.
//   - options - RadiusPlanesClientGetOptions contains the optional parameters for the RadiusPlanesClient.Get method.
func (client *RadiusPlanesClient) Get(ctx context.Context, planeName string, options *RadiusPlanesClientGetOptions) (RadiusPlanesClientGetResponse, error) {
	var err error
	const operationName = "RadiusPlanesClient.Get"
	ctx = context.WithValue(ctx, runtime.CtxAPINameKey{}, operationName)
	ctx, endSpan := runtime.StartSpan(ctx, operationName, client.internal.Tracer(), nil)
	defer func() { endSpan(err) }()
	req, err := client.getCreateRequest(ctx, planeName, options)
	if err != nil {
		return RadiusPlanesClientGetResponse{}, err
	}
	httpResp, err := client.internal.Pipeline().Do(req)
	if err != nil {
		return RadiusPlanesClientGetResponse{}, err
	}
	if !runtime.HasStatusCode(httpResp, http.StatusOK) {
		err = runtime.NewResponseError(httpResp)
		return RadiusPlanesClientGetResponse{}, err
	}
	resp, err := client.getHandleResponse(httpResp)
	return resp, err
}

// getCreateRequest creates the Get request.
func (client *RadiusPlanesClient) getCreateRequest(ctx context.Context, planeName string, _ *RadiusPlanesClientGetOptions) (*policy.Request, error) {
	urlPath := "/planes/radius/{planeName}"
	if planeName == "" {
		return nil, errors.New("parameter planeName cannot be empty")
	}
	urlPath = strings.ReplaceAll(urlPath, "{planeName}", url.PathEscape(planeName))
	req, err := runtime.NewRequest(ctx, http.MethodGet, runtime.JoinPaths(client.internal.Endpoint(), urlPath))
	if err != nil {
		return nil, err
	}
	reqQP := req.Raw().URL.Query()
	reqQP.Set("api-version", "2023-10-01-preview")
	req.Raw().URL.RawQuery = reqQP.Encode()
	req.Raw().Header["Accept"] = []string{"application/json"}
	return req, nil
}

// getHandleResponse handles the Get response.
func (client *RadiusPlanesClient) getHandleResponse(resp *http.Response) (RadiusPlanesClientGetResponse, error) {
	result := RadiusPlanesClientGetResponse{}
	if err := runtime.UnmarshalAsJSON(resp, &result.RadiusPlaneResource); err != nil {
		return RadiusPlanesClientGetResponse{}, err
	}
	return result, nil
}

// NewListPager - List Radius planes
//
// Generated from API version 2023-10-01-preview
//   - options - RadiusPlanesClientListOptions contains the optional parameters for the RadiusPlanesClient.NewListPager method.
func (client *RadiusPlanesClient) NewListPager(options *RadiusPlanesClientListOptions) (*runtime.Pager[RadiusPlanesClientListResponse]) {
	return runtime.NewPager(runtime.PagingHandler[RadiusPlanesClientListResponse]{
		More: func(page RadiusPlanesClientListResponse) bool {
			return page.NextLink != nil && len(*page.NextLink) > 0
		},
		Fetcher: func(ctx context.Context, page *RadiusPlanesClientListResponse) (RadiusPlanesClientListResponse, error) {
		ctx = context.WithValue(ctx, runtime.CtxAPINameKey{}, "RadiusPlanesClient.NewListPager")
			nextLink := ""
			if page != nil {
				nextLink = *page.NextLink
			}
			resp, err := runtime.FetcherForNextLink(ctx, client.internal.Pipeline(), nextLink, func(ctx context.Context) (*policy.Request, error) {
				return client.listCreateRequest(ctx, options)
			}, nil)
			if err != nil {
				return RadiusPlanesClientListResponse{}, err
			}
			return client.listHandleResponse(resp)
			},
		Tracer: client.internal.Tracer(),
	})
}

// listCreateRequest creates the List request.
func (client *RadiusPlanesClient) listCreateRequest(ctx context.Context, _ *RadiusPlanesClientListOptions) (*policy.Request, error) {
	urlPath := "/planes/radius"
	req, err := runtime.NewRequest(ctx, http.MethodGet, runtime.JoinPaths(client.internal.Endpoint(), urlPath))
	if err != nil {
		return nil, err
	}
	reqQP := req.Raw().URL.Query()
	reqQP.Set("api-version", "2023-10-01-preview")
	req.Raw().URL.RawQuery = reqQP.Encode()
	req.Raw().Header["Accept"] = []string{"application/json"}
	return req, nil
}

// listHandleResponse handles the List response.
func (client *RadiusPlanesClient) listHandleResponse(resp *http.Response) (RadiusPlanesClientListResponse, error) {
	result := RadiusPlanesClientListResponse{}
	if err := runtime.UnmarshalAsJSON(resp, &result.RadiusPlaneResourceListResult); err != nil {
		return RadiusPlanesClientListResponse{}, err
	}
	return result, nil
}

// BeginUpdate - Update a plane
// If the operation fails it returns an *azcore.ResponseError type.
//
// Generated from API version 2023-10-01-preview
//   - planeName - The plane name.
//   - properties - The resource properties to be updated.
//   - options - RadiusPlanesClientBeginUpdateOptions contains the optional parameters for the RadiusPlanesClient.BeginUpdate
//     method.
func (client *RadiusPlanesClient) BeginUpdate(ctx context.Context, planeName string, properties RadiusPlaneResourceTagsUpdate, options *RadiusPlanesClientBeginUpdateOptions) (*runtime.Poller[RadiusPlanesClientUpdateResponse], error) {
	if options == nil || options.ResumeToken == "" {
		resp, err := client.update(ctx, planeName, properties, options)
		if err != nil {
			return nil, err
		}
		poller, err := runtime.NewPoller(resp, client.internal.Pipeline(), &runtime.NewPollerOptions[RadiusPlanesClientUpdateResponse]{
			FinalStateVia: runtime.FinalStateViaLocation,
			Tracer: client.internal.Tracer(),
		})
		return poller, err
	} else {
		return runtime.NewPollerFromResumeToken(options.ResumeToken, client.internal.Pipeline(), &runtime.NewPollerFromResumeTokenOptions[RadiusPlanesClientUpdateResponse]{
			Tracer: client.internal.Tracer(),
		})
	}
}

// Update - Update a plane
// If the operation fails it returns an *azcore.ResponseError type.
//
// Generated from API version 2023-10-01-preview
func (client *RadiusPlanesClient) update(ctx context.Context, planeName string, properties RadiusPlaneResourceTagsUpdate, options *RadiusPlanesClientBeginUpdateOptions) (*http.Response, error) {
	var err error
	const operationName = "RadiusPlanesClient.BeginUpdate"
	ctx = context.WithValue(ctx, runtime.CtxAPINameKey{}, operationName)
	ctx, endSpan := runtime.StartSpan(ctx, operationName, client.internal.Tracer(), nil)
	defer func() { endSpan(err) }()
	req, err := client.updateCreateRequest(ctx, planeName, properties, options)
	if err != nil {
		return nil, err
	}
	httpResp, err := client.internal.Pipeline().Do(req)
	if err != nil {
		return nil, err
	}
	if !runtime.HasStatusCode(httpResp, http.StatusOK, http.StatusAccepted) {
		err = runtime.NewResponseError(httpResp)
		return nil, err
	}
	return httpResp, nil
}

// updateCreateRequest creates the Update request.
func (client *RadiusPlanesClient) updateCreateRequest(ctx context.Context, planeName string, properties RadiusPlaneResourceTagsUpdate, _ *RadiusPlanesClientBeginUpdateOptions) (*policy.Request, error) {
	urlPath := "/planes/radius/{planeName}"
	if planeName == "" {
		return nil, errors.New("parameter planeName cannot be empty")
	}
	urlPath = strings.ReplaceAll(urlPath, "{planeName}", url.PathEscape(planeName))
	req, err := runtime.NewRequest(ctx, http.MethodPatch, runtime.JoinPaths(client.internal.Endpoint(), urlPath))
	if err != nil {
		return nil, err
	}
	reqQP := req.Raw().URL.Query()
	reqQP.Set("api-version", "2023-10-01-preview")
	req.Raw().URL.RawQuery = reqQP.Encode()
	req.Raw().Header["Accept"] = []string{"application/json"}
	if err := runtime.MarshalAsJSON(req, properties); err != nil {
	return nil, err
}
;	return req, nil
}

