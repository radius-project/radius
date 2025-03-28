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

// ApplicationsClient contains the methods for the Applications group.
// Don't use this type directly, use NewApplicationsClient() instead.
type ApplicationsClient struct {
	internal *arm.Client
	rootScope string
}

// NewApplicationsClient creates a new instance of ApplicationsClient with the specified values.
//   - rootScope - The scope in which the resource is present. UCP Scope is /planes/{planeType}/{planeName}/resourceGroup/{resourcegroupID}
//     and Azure resource scope is
//     /subscriptions/{subscriptionID}/resourceGroup/{resourcegroupID}
//   - credential - used to authorize requests. Usually a credential from azidentity.
//   - options - pass nil to accept the default values.
func NewApplicationsClient(rootScope string, credential azcore.TokenCredential, options *arm.ClientOptions) (*ApplicationsClient, error) {
	cl, err := arm.NewClient(moduleName, moduleVersion, credential, options)
	if err != nil {
		return nil, err
	}
	client := &ApplicationsClient{
		rootScope: rootScope,
	internal: cl,
	}
	return client, nil
}

// CreateOrUpdate - Create a ApplicationResource
// If the operation fails it returns an *azcore.ResponseError type.
//
// Generated from API version 2023-10-01-preview
//   - applicationName - The application name
//   - resource - Resource create parameters.
//   - options - ApplicationsClientCreateOrUpdateOptions contains the optional parameters for the ApplicationsClient.CreateOrUpdate
//     method.
func (client *ApplicationsClient) CreateOrUpdate(ctx context.Context, applicationName string, resource ApplicationResource, options *ApplicationsClientCreateOrUpdateOptions) (ApplicationsClientCreateOrUpdateResponse, error) {
	var err error
	ctx, endSpan := runtime.StartSpan(ctx, "ApplicationsClient.CreateOrUpdate", client.internal.Tracer(), nil)
	defer func() { endSpan(err) }()
	req, err := client.createOrUpdateCreateRequest(ctx, applicationName, resource, options)
	if err != nil {
		return ApplicationsClientCreateOrUpdateResponse{}, err
	}
	httpResp, err := client.internal.Pipeline().Do(req)
	if err != nil {
		return ApplicationsClientCreateOrUpdateResponse{}, err
	}
	if !runtime.HasStatusCode(httpResp, http.StatusOK, http.StatusCreated) {
		err = runtime.NewResponseError(httpResp)
		return ApplicationsClientCreateOrUpdateResponse{}, err
	}
	resp, err := client.createOrUpdateHandleResponse(httpResp)
	return resp, err
}

// createOrUpdateCreateRequest creates the CreateOrUpdate request.
func (client *ApplicationsClient) createOrUpdateCreateRequest(ctx context.Context, applicationName string, resource ApplicationResource, _ *ApplicationsClientCreateOrUpdateOptions) (*policy.Request, error) {
	urlPath := "/{rootScope}/providers/Applications.Core/applications/{applicationName}"
	urlPath = strings.ReplaceAll(urlPath, "{rootScope}", client.rootScope)
	if applicationName == "" {
		return nil, errors.New("parameter applicationName cannot be empty")
	}
	urlPath = strings.ReplaceAll(urlPath, "{applicationName}", url.PathEscape(applicationName))
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

// createOrUpdateHandleResponse handles the CreateOrUpdate response.
func (client *ApplicationsClient) createOrUpdateHandleResponse(resp *http.Response) (ApplicationsClientCreateOrUpdateResponse, error) {
	result := ApplicationsClientCreateOrUpdateResponse{}
	if err := runtime.UnmarshalAsJSON(resp, &result.ApplicationResource); err != nil {
		return ApplicationsClientCreateOrUpdateResponse{}, err
	}
	return result, nil
}

// Delete - Delete a ApplicationResource
// If the operation fails it returns an *azcore.ResponseError type.
//
// Generated from API version 2023-10-01-preview
//   - applicationName - The application name
//   - options - ApplicationsClientDeleteOptions contains the optional parameters for the ApplicationsClient.Delete method.
func (client *ApplicationsClient) Delete(ctx context.Context, applicationName string, options *ApplicationsClientDeleteOptions) (ApplicationsClientDeleteResponse, error) {
	var err error
	ctx, endSpan := runtime.StartSpan(ctx, "ApplicationsClient.Delete", client.internal.Tracer(), nil)
	defer func() { endSpan(err) }()
	req, err := client.deleteCreateRequest(ctx, applicationName, options)
	if err != nil {
		return ApplicationsClientDeleteResponse{}, err
	}
	httpResp, err := client.internal.Pipeline().Do(req)
	if err != nil {
		return ApplicationsClientDeleteResponse{}, err
	}
	if !runtime.HasStatusCode(httpResp, http.StatusOK, http.StatusNoContent) {
		err = runtime.NewResponseError(httpResp)
		return ApplicationsClientDeleteResponse{}, err
	}
	return ApplicationsClientDeleteResponse{}, nil
}

// deleteCreateRequest creates the Delete request.
func (client *ApplicationsClient) deleteCreateRequest(ctx context.Context, applicationName string, _ *ApplicationsClientDeleteOptions) (*policy.Request, error) {
	urlPath := "/{rootScope}/providers/Applications.Core/applications/{applicationName}"
	urlPath = strings.ReplaceAll(urlPath, "{rootScope}", client.rootScope)
	if applicationName == "" {
		return nil, errors.New("parameter applicationName cannot be empty")
	}
	urlPath = strings.ReplaceAll(urlPath, "{applicationName}", url.PathEscape(applicationName))
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

// Get - Get a ApplicationResource
// If the operation fails it returns an *azcore.ResponseError type.
//
// Generated from API version 2023-10-01-preview
//   - applicationName - The application name
//   - options - ApplicationsClientGetOptions contains the optional parameters for the ApplicationsClient.Get method.
func (client *ApplicationsClient) Get(ctx context.Context, applicationName string, options *ApplicationsClientGetOptions) (ApplicationsClientGetResponse, error) {
	var err error
	ctx, endSpan := runtime.StartSpan(ctx, "ApplicationsClient.Get", client.internal.Tracer(), nil)
	defer func() { endSpan(err) }()
	req, err := client.getCreateRequest(ctx, applicationName, options)
	if err != nil {
		return ApplicationsClientGetResponse{}, err
	}
	httpResp, err := client.internal.Pipeline().Do(req)
	if err != nil {
		return ApplicationsClientGetResponse{}, err
	}
	if !runtime.HasStatusCode(httpResp, http.StatusOK) {
		err = runtime.NewResponseError(httpResp)
		return ApplicationsClientGetResponse{}, err
	}
	resp, err := client.getHandleResponse(httpResp)
	return resp, err
}

// getCreateRequest creates the Get request.
func (client *ApplicationsClient) getCreateRequest(ctx context.Context, applicationName string, _ *ApplicationsClientGetOptions) (*policy.Request, error) {
	urlPath := "/{rootScope}/providers/Applications.Core/applications/{applicationName}"
	urlPath = strings.ReplaceAll(urlPath, "{rootScope}", client.rootScope)
	if applicationName == "" {
		return nil, errors.New("parameter applicationName cannot be empty")
	}
	urlPath = strings.ReplaceAll(urlPath, "{applicationName}", url.PathEscape(applicationName))
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
func (client *ApplicationsClient) getHandleResponse(resp *http.Response) (ApplicationsClientGetResponse, error) {
	result := ApplicationsClientGetResponse{}
	if err := runtime.UnmarshalAsJSON(resp, &result.ApplicationResource); err != nil {
		return ApplicationsClientGetResponse{}, err
	}
	return result, nil
}

// GetGraph - Gets the application graph and resources.
// If the operation fails it returns an *azcore.ResponseError type.
//
// Generated from API version 2023-10-01-preview
//   - applicationName - The application name
//   - body - The content of the action request
//   - options - ApplicationsClientGetGraphOptions contains the optional parameters for the ApplicationsClient.GetGraph method.
func (client *ApplicationsClient) GetGraph(ctx context.Context, applicationName string, body map[string]any, options *ApplicationsClientGetGraphOptions) (ApplicationsClientGetGraphResponse, error) {
	var err error
	ctx, endSpan := runtime.StartSpan(ctx, "ApplicationsClient.GetGraph", client.internal.Tracer(), nil)
	defer func() { endSpan(err) }()
	req, err := client.getGraphCreateRequest(ctx, applicationName, body, options)
	if err != nil {
		return ApplicationsClientGetGraphResponse{}, err
	}
	httpResp, err := client.internal.Pipeline().Do(req)
	if err != nil {
		return ApplicationsClientGetGraphResponse{}, err
	}
	if !runtime.HasStatusCode(httpResp, http.StatusOK) {
		err = runtime.NewResponseError(httpResp)
		return ApplicationsClientGetGraphResponse{}, err
	}
	resp, err := client.getGraphHandleResponse(httpResp)
	return resp, err
}

// getGraphCreateRequest creates the GetGraph request.
func (client *ApplicationsClient) getGraphCreateRequest(ctx context.Context, applicationName string, body map[string]any, _ *ApplicationsClientGetGraphOptions) (*policy.Request, error) {
	urlPath := "/{rootScope}/providers/Applications.Core/applications/{applicationName}/getGraph"
	urlPath = strings.ReplaceAll(urlPath, "{rootScope}", client.rootScope)
	if applicationName == "" {
		return nil, errors.New("parameter applicationName cannot be empty")
	}
	urlPath = strings.ReplaceAll(urlPath, "{applicationName}", url.PathEscape(applicationName))
	req, err := runtime.NewRequest(ctx, http.MethodPost, runtime.JoinPaths(client.internal.Endpoint(), urlPath))
	if err != nil {
		return nil, err
	}
	reqQP := req.Raw().URL.Query()
	reqQP.Set("api-version", "2023-10-01-preview")
	req.Raw().URL.RawQuery = reqQP.Encode()
	req.Raw().Header["Accept"] = []string{"application/json"}
	if err := runtime.MarshalAsJSON(req, body); err != nil {
	return nil, err
}
;	return req, nil
}

// getGraphHandleResponse handles the GetGraph response.
func (client *ApplicationsClient) getGraphHandleResponse(resp *http.Response) (ApplicationsClientGetGraphResponse, error) {
	result := ApplicationsClientGetGraphResponse{}
	if err := runtime.UnmarshalAsJSON(resp, &result.ApplicationGraphResponse); err != nil {
		return ApplicationsClientGetGraphResponse{}, err
	}
	return result, nil
}

// NewListByScopePager - List ApplicationResource resources by Scope
//
// Generated from API version 2023-10-01-preview
//   - options - ApplicationsClientListByScopeOptions contains the optional parameters for the ApplicationsClient.NewListByScopePager
//     method.
func (client *ApplicationsClient) NewListByScopePager(options *ApplicationsClientListByScopeOptions) (*runtime.Pager[ApplicationsClientListByScopeResponse]) {
	return runtime.NewPager(runtime.PagingHandler[ApplicationsClientListByScopeResponse]{
		More: func(page ApplicationsClientListByScopeResponse) bool {
			return page.NextLink != nil && len(*page.NextLink) > 0
		},
		Fetcher: func(ctx context.Context, page *ApplicationsClientListByScopeResponse) (ApplicationsClientListByScopeResponse, error) {
			nextLink := ""
			if page != nil {
				nextLink = *page.NextLink
			}
			resp, err := runtime.FetcherForNextLink(ctx, client.internal.Pipeline(), nextLink, func(ctx context.Context) (*policy.Request, error) {
				return client.listByScopeCreateRequest(ctx, options)
			}, nil)
			if err != nil {
				return ApplicationsClientListByScopeResponse{}, err
			}
			return client.listByScopeHandleResponse(resp)
			},
		Tracer: client.internal.Tracer(),
	})
}

// listByScopeCreateRequest creates the ListByScope request.
func (client *ApplicationsClient) listByScopeCreateRequest(ctx context.Context, _ *ApplicationsClientListByScopeOptions) (*policy.Request, error) {
	urlPath := "/{rootScope}/providers/Applications.Core/applications"
	urlPath = strings.ReplaceAll(urlPath, "{rootScope}", client.rootScope)
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

// listByScopeHandleResponse handles the ListByScope response.
func (client *ApplicationsClient) listByScopeHandleResponse(resp *http.Response) (ApplicationsClientListByScopeResponse, error) {
	result := ApplicationsClientListByScopeResponse{}
	if err := runtime.UnmarshalAsJSON(resp, &result.ApplicationResourceListResult); err != nil {
		return ApplicationsClientListByScopeResponse{}, err
	}
	return result, nil
}

// Update - Update a ApplicationResource
// If the operation fails it returns an *azcore.ResponseError type.
//
// Generated from API version 2023-10-01-preview
//   - applicationName - The application name
//   - properties - The resource properties to be updated.
//   - options - ApplicationsClientUpdateOptions contains the optional parameters for the ApplicationsClient.Update method.
func (client *ApplicationsClient) Update(ctx context.Context, applicationName string, properties ApplicationResourceUpdate, options *ApplicationsClientUpdateOptions) (ApplicationsClientUpdateResponse, error) {
	var err error
	ctx, endSpan := runtime.StartSpan(ctx, "ApplicationsClient.Update", client.internal.Tracer(), nil)
	defer func() { endSpan(err) }()
	req, err := client.updateCreateRequest(ctx, applicationName, properties, options)
	if err != nil {
		return ApplicationsClientUpdateResponse{}, err
	}
	httpResp, err := client.internal.Pipeline().Do(req)
	if err != nil {
		return ApplicationsClientUpdateResponse{}, err
	}
	if !runtime.HasStatusCode(httpResp, http.StatusOK) {
		err = runtime.NewResponseError(httpResp)
		return ApplicationsClientUpdateResponse{}, err
	}
	resp, err := client.updateHandleResponse(httpResp)
	return resp, err
}

// updateCreateRequest creates the Update request.
func (client *ApplicationsClient) updateCreateRequest(ctx context.Context, applicationName string, properties ApplicationResourceUpdate, _ *ApplicationsClientUpdateOptions) (*policy.Request, error) {
	urlPath := "/{rootScope}/providers/Applications.Core/applications/{applicationName}"
	urlPath = strings.ReplaceAll(urlPath, "{rootScope}", client.rootScope)
	if applicationName == "" {
		return nil, errors.New("parameter applicationName cannot be empty")
	}
	urlPath = strings.ReplaceAll(urlPath, "{applicationName}", url.PathEscape(applicationName))
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

// updateHandleResponse handles the Update response.
func (client *ApplicationsClient) updateHandleResponse(resp *http.Response) (ApplicationsClientUpdateResponse, error) {
	result := ApplicationsClientUpdateResponse{}
	if err := runtime.UnmarshalAsJSON(resp, &result.ApplicationResource); err != nil {
		return ApplicationsClientUpdateResponse{}, err
	}
	return result, nil
}

