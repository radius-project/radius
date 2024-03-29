//go:build go1.18
// +build go1.18

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

// PlanesClient contains the methods for the Planes group.
// Don't use this type directly, use NewPlanesClient() instead.
type PlanesClient struct {
	internal *arm.Client
}

// NewPlanesClient creates a new instance of PlanesClient with the specified values.
//   - credential - used to authorize requests. Usually a credential from azidentity.
//   - options - pass nil to accept the default values.
func NewPlanesClient(credential azcore.TokenCredential, options *arm.ClientOptions) (*PlanesClient, error) {
	cl, err := arm.NewClient(moduleName+".PlanesClient", moduleVersion, credential, options)
	if err != nil {
		return nil, err
	}
	client := &PlanesClient{
	internal: cl,
	}
	return client, nil
}

// BeginCreateOrUpdate - Create or update a plane
// If the operation fails it returns an *azcore.ResponseError type.
//
// Generated from API version 2023-10-01-preview
//   - planeType - The plane type.
//   - planeName - The name of the plane
//   - resource - Resource create parameters.
//   - options - PlanesClientBeginCreateOrUpdateOptions contains the optional parameters for the PlanesClient.BeginCreateOrUpdate
//     method.
func (client *PlanesClient) BeginCreateOrUpdate(ctx context.Context, planeType string, planeName string, resource PlaneResource, options *PlanesClientBeginCreateOrUpdateOptions) (*runtime.Poller[PlanesClientCreateOrUpdateResponse], error) {
	if options == nil || options.ResumeToken == "" {
		resp, err := client.createOrUpdate(ctx, planeType, planeName, resource, options)
		if err != nil {
			return nil, err
		}
		poller, err := runtime.NewPoller(resp, client.internal.Pipeline(), &runtime.NewPollerOptions[PlanesClientCreateOrUpdateResponse]{
			FinalStateVia: runtime.FinalStateViaAzureAsyncOp,
		})
		return poller, err
	} else {
		return runtime.NewPollerFromResumeToken[PlanesClientCreateOrUpdateResponse](options.ResumeToken, client.internal.Pipeline(), nil)
	}
}

// CreateOrUpdate - Create or update a plane
// If the operation fails it returns an *azcore.ResponseError type.
//
// Generated from API version 2023-10-01-preview
func (client *PlanesClient) createOrUpdate(ctx context.Context, planeType string, planeName string, resource PlaneResource, options *PlanesClientBeginCreateOrUpdateOptions) (*http.Response, error) {
	var err error
	req, err := client.createOrUpdateCreateRequest(ctx, planeType, planeName, resource, options)
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
func (client *PlanesClient) createOrUpdateCreateRequest(ctx context.Context, planeType string, planeName string, resource PlaneResource, options *PlanesClientBeginCreateOrUpdateOptions) (*policy.Request, error) {
	urlPath := "/planes/{planeType}/{planeName}"
	if planeType == "" {
		return nil, errors.New("parameter planeType cannot be empty")
	}
	urlPath = strings.ReplaceAll(urlPath, "{planeType}", url.PathEscape(planeType))
	urlPath = strings.ReplaceAll(urlPath, "{planeName}", planeName)
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
	return req, nil
}

// BeginDelete - Delete a plane
// If the operation fails it returns an *azcore.ResponseError type.
//
// Generated from API version 2023-10-01-preview
//   - planeType - The plane type.
//   - planeName - The name of the plane
//   - options - PlanesClientBeginDeleteOptions contains the optional parameters for the PlanesClient.BeginDelete method.
func (client *PlanesClient) BeginDelete(ctx context.Context, planeType string, planeName string, options *PlanesClientBeginDeleteOptions) (*runtime.Poller[PlanesClientDeleteResponse], error) {
	if options == nil || options.ResumeToken == "" {
		resp, err := client.deleteOperation(ctx, planeType, planeName, options)
		if err != nil {
			return nil, err
		}
		poller, err := runtime.NewPoller(resp, client.internal.Pipeline(), &runtime.NewPollerOptions[PlanesClientDeleteResponse]{
			FinalStateVia: runtime.FinalStateViaLocation,
		})
		return poller, err
	} else {
		return runtime.NewPollerFromResumeToken[PlanesClientDeleteResponse](options.ResumeToken, client.internal.Pipeline(), nil)
	}
}

// Delete - Delete a plane
// If the operation fails it returns an *azcore.ResponseError type.
//
// Generated from API version 2023-10-01-preview
func (client *PlanesClient) deleteOperation(ctx context.Context, planeType string, planeName string, options *PlanesClientBeginDeleteOptions) (*http.Response, error) {
	var err error
	req, err := client.deleteCreateRequest(ctx, planeType, planeName, options)
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
func (client *PlanesClient) deleteCreateRequest(ctx context.Context, planeType string, planeName string, options *PlanesClientBeginDeleteOptions) (*policy.Request, error) {
	urlPath := "/planes/{planeType}/{planeName}"
	if planeType == "" {
		return nil, errors.New("parameter planeType cannot be empty")
	}
	urlPath = strings.ReplaceAll(urlPath, "{planeType}", url.PathEscape(planeType))
	urlPath = strings.ReplaceAll(urlPath, "{planeName}", planeName)
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
//   - planeType - The plane type.
//   - planeName - The name of the plane
//   - options - PlanesClientGetOptions contains the optional parameters for the PlanesClient.Get method.
func (client *PlanesClient) Get(ctx context.Context, planeType string, planeName string, options *PlanesClientGetOptions) (PlanesClientGetResponse, error) {
	var err error
	req, err := client.getCreateRequest(ctx, planeType, planeName, options)
	if err != nil {
		return PlanesClientGetResponse{}, err
	}
	httpResp, err := client.internal.Pipeline().Do(req)
	if err != nil {
		return PlanesClientGetResponse{}, err
	}
	if !runtime.HasStatusCode(httpResp, http.StatusOK) {
		err = runtime.NewResponseError(httpResp)
		return PlanesClientGetResponse{}, err
	}
	resp, err := client.getHandleResponse(httpResp)
	return resp, err
}

// getCreateRequest creates the Get request.
func (client *PlanesClient) getCreateRequest(ctx context.Context, planeType string, planeName string, options *PlanesClientGetOptions) (*policy.Request, error) {
	urlPath := "/planes/{planeType}/{planeName}"
	if planeType == "" {
		return nil, errors.New("parameter planeType cannot be empty")
	}
	urlPath = strings.ReplaceAll(urlPath, "{planeType}", url.PathEscape(planeType))
	urlPath = strings.ReplaceAll(urlPath, "{planeName}", planeName)
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
func (client *PlanesClient) getHandleResponse(resp *http.Response) (PlanesClientGetResponse, error) {
	result := PlanesClientGetResponse{}
	if err := runtime.UnmarshalAsJSON(resp, &result.PlaneResource); err != nil {
		return PlanesClientGetResponse{}, err
	}
	return result, nil
}

// NewListByTypePager - List planes by type
//
// Generated from API version 2023-10-01-preview
//   - planeType - The plane type.
//   - options - PlanesClientListByTypeOptions contains the optional parameters for the PlanesClient.NewListByTypePager method.
func (client *PlanesClient) NewListByTypePager(planeType string, options *PlanesClientListByTypeOptions) (*runtime.Pager[PlanesClientListByTypeResponse]) {
	return runtime.NewPager(runtime.PagingHandler[PlanesClientListByTypeResponse]{
		More: func(page PlanesClientListByTypeResponse) bool {
			return page.NextLink != nil && len(*page.NextLink) > 0
		},
		Fetcher: func(ctx context.Context, page *PlanesClientListByTypeResponse) (PlanesClientListByTypeResponse, error) {
			var req *policy.Request
			var err error
			if page == nil {
				req, err = client.listByTypeCreateRequest(ctx, planeType, options)
			} else {
				req, err = runtime.NewRequest(ctx, http.MethodGet, *page.NextLink)
			}
			if err != nil {
				return PlanesClientListByTypeResponse{}, err
			}
			resp, err := client.internal.Pipeline().Do(req)
			if err != nil {
				return PlanesClientListByTypeResponse{}, err
			}
			if !runtime.HasStatusCode(resp, http.StatusOK) {
				return PlanesClientListByTypeResponse{}, runtime.NewResponseError(resp)
			}
			return client.listByTypeHandleResponse(resp)
		},
	})
}

// listByTypeCreateRequest creates the ListByType request.
func (client *PlanesClient) listByTypeCreateRequest(ctx context.Context, planeType string, options *PlanesClientListByTypeOptions) (*policy.Request, error) {
	urlPath := "/planes/{planeType}"
	if planeType == "" {
		return nil, errors.New("parameter planeType cannot be empty")
	}
	urlPath = strings.ReplaceAll(urlPath, "{planeType}", url.PathEscape(planeType))
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

// listByTypeHandleResponse handles the ListByType response.
func (client *PlanesClient) listByTypeHandleResponse(resp *http.Response) (PlanesClientListByTypeResponse, error) {
	result := PlanesClientListByTypeResponse{}
	if err := runtime.UnmarshalAsJSON(resp, &result.PlaneResourceListResult); err != nil {
		return PlanesClientListByTypeResponse{}, err
	}
	return result, nil
}

// NewListPlanesPager - List all planes
//
// Generated from API version 2023-10-01-preview
//   - options - PlanesClientListPlanesOptions contains the optional parameters for the PlanesClient.NewListPlanesPager method.
func (client *PlanesClient) NewListPlanesPager(options *PlanesClientListPlanesOptions) (*runtime.Pager[PlanesClientListPlanesResponse]) {
	return runtime.NewPager(runtime.PagingHandler[PlanesClientListPlanesResponse]{
		More: func(page PlanesClientListPlanesResponse) bool {
			return page.NextLink != nil && len(*page.NextLink) > 0
		},
		Fetcher: func(ctx context.Context, page *PlanesClientListPlanesResponse) (PlanesClientListPlanesResponse, error) {
			var req *policy.Request
			var err error
			if page == nil {
				req, err = client.listPlanesCreateRequest(ctx, options)
			} else {
				req, err = runtime.NewRequest(ctx, http.MethodGet, *page.NextLink)
			}
			if err != nil {
				return PlanesClientListPlanesResponse{}, err
			}
			resp, err := client.internal.Pipeline().Do(req)
			if err != nil {
				return PlanesClientListPlanesResponse{}, err
			}
			if !runtime.HasStatusCode(resp, http.StatusOK) {
				return PlanesClientListPlanesResponse{}, runtime.NewResponseError(resp)
			}
			return client.listPlanesHandleResponse(resp)
		},
	})
}

// listPlanesCreateRequest creates the ListPlanes request.
func (client *PlanesClient) listPlanesCreateRequest(ctx context.Context, options *PlanesClientListPlanesOptions) (*policy.Request, error) {
	urlPath := "/planes"
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

// listPlanesHandleResponse handles the ListPlanes response.
func (client *PlanesClient) listPlanesHandleResponse(resp *http.Response) (PlanesClientListPlanesResponse, error) {
	result := PlanesClientListPlanesResponse{}
	if err := runtime.UnmarshalAsJSON(resp, &result.PlaneResourceListResult); err != nil {
		return PlanesClientListPlanesResponse{}, err
	}
	return result, nil
}

// BeginUpdate - Update a plane
// If the operation fails it returns an *azcore.ResponseError type.
//
// Generated from API version 2023-10-01-preview
//   - planeType - The plane type.
//   - planeName - The name of the plane
//   - properties - The resource properties to be updated.
//   - options - PlanesClientBeginUpdateOptions contains the optional parameters for the PlanesClient.BeginUpdate method.
func (client *PlanesClient) BeginUpdate(ctx context.Context, planeType string, planeName string, properties PlaneResourceTagsUpdate, options *PlanesClientBeginUpdateOptions) (*runtime.Poller[PlanesClientUpdateResponse], error) {
	if options == nil || options.ResumeToken == "" {
		resp, err := client.update(ctx, planeType, planeName, properties, options)
		if err != nil {
			return nil, err
		}
		poller, err := runtime.NewPoller(resp, client.internal.Pipeline(), &runtime.NewPollerOptions[PlanesClientUpdateResponse]{
			FinalStateVia: runtime.FinalStateViaLocation,
		})
		return poller, err
	} else {
		return runtime.NewPollerFromResumeToken[PlanesClientUpdateResponse](options.ResumeToken, client.internal.Pipeline(), nil)
	}
}

// Update - Update a plane
// If the operation fails it returns an *azcore.ResponseError type.
//
// Generated from API version 2023-10-01-preview
func (client *PlanesClient) update(ctx context.Context, planeType string, planeName string, properties PlaneResourceTagsUpdate, options *PlanesClientBeginUpdateOptions) (*http.Response, error) {
	var err error
	req, err := client.updateCreateRequest(ctx, planeType, planeName, properties, options)
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
func (client *PlanesClient) updateCreateRequest(ctx context.Context, planeType string, planeName string, properties PlaneResourceTagsUpdate, options *PlanesClientBeginUpdateOptions) (*policy.Request, error) {
	urlPath := "/planes/{planeType}/{planeName}"
	if planeType == "" {
		return nil, errors.New("parameter planeType cannot be empty")
	}
	urlPath = strings.ReplaceAll(urlPath, "{planeType}", url.PathEscape(planeType))
	urlPath = strings.ReplaceAll(urlPath, "{planeName}", planeName)
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
	return req, nil
}

