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

package manifest

import (
	"context"
	"net/http"

	armpolicy "github.com/Azure/azure-sdk-for-go/sdk/azcore/arm/policy"
	azfake "github.com/Azure/azure-sdk-for-go/sdk/azcore/fake"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	ucpfake "github.com/radius-project/radius/pkg/ucp/api/v20231001preview/fake"
)

// NewTestClientFactory creates a new client factory for testing purposes.
func NewTestClientFactory(resourceProvidersServer func() ucpfake.ResourceProvidersServer) (*v20231001preview.ClientFactory, error) {
	serverFactory := ucpfake.ServerFactory{
		ResourceProvidersServer: resourceProvidersServer(),
		ResourceTypesServer:     WithResourceTypeServerNoError(),
		APIVersionsServer:       WithAPIVersionServerNoError(),
		LocationsServer:         WithLocationServerNoError(),
	}

	serverFactoryTransport := ucpfake.NewServerFactoryTransport(&serverFactory)

	clientOptions := &armpolicy.ClientOptions{
		ClientOptions: policy.ClientOptions{
			Transport: serverFactoryTransport,
		},
	}

	clientFactory, err := v20231001preview.NewClientFactory(&azfake.TokenCredential{}, clientOptions)
	if err != nil {
		return nil, err
	}

	return clientFactory, err
}

func WithResourceProviderServerNoError() ucpfake.ResourceProvidersServer {
	resourceProvidersServer := ucpfake.ResourceProvidersServer{
		BeginCreateOrUpdate: func(
			ctx context.Context,
			planeName string,
			resourceProviderName string,
			resource v20231001preview.ResourceProviderResource,
			options *v20231001preview.ResourceProvidersClientBeginCreateOrUpdateOptions,
		) (resp azfake.PollerResponder[v20231001preview.ResourceProvidersClientCreateOrUpdateResponse], errResp azfake.ErrorResponder) {
			// Simulate successful creation
			result := v20231001preview.ResourceProvidersClientCreateOrUpdateResponse{
				ResourceProviderResource: resource,
			}
			resp.AddNonTerminalResponse(http.StatusCreated, nil)
			resp.SetTerminalResponse(http.StatusOK, result, nil)

			return
		},
		Get: func(
			ctx context.Context,
			planeName string,
			resourceProviderName string,
			options *v20231001preview.ResourceProvidersClientGetOptions, // Add this parameter
		) (resp azfake.Responder[v20231001preview.ResourceProvidersClientGetResponse], errResp azfake.ErrorResponder) {
			response := v20231001preview.ResourceProvidersClientGetResponse{
				ResourceProviderResource: v20231001preview.ResourceProviderResource{
					Name: to.Ptr(resourceProviderName),
				},
			}
			resp.SetResponse(http.StatusOK, response, nil)
			return
		},
		GetProviderSummary: func(
			ctx context.Context,
			planeName string,
			resourceProviderName string,
			options *v20231001preview.ResourceProvidersClientGetProviderSummaryOptions,
		) (resp azfake.Responder[v20231001preview.ResourceProvidersClientGetProviderSummaryResponse], errResp azfake.ErrorResponder) {
			response := v20231001preview.ResourceProvidersClientGetProviderSummaryResponse{
				ResourceProviderSummary: v20231001preview.ResourceProviderSummary{
					Name: to.Ptr(resourceProviderName),
					ResourceTypes: map[string]*v20231001preview.ResourceProviderSummaryResourceType{
						"testResources": {
							Description: to.Ptr("Resource type description"),
							APIVersions: map[string]*v20231001preview.ResourceTypeSummaryResultAPIVersion{
								"2023-10-01-preview": {
									Schema: map[string]any{
										"properties": map[string]any{
											"application": map[string]any{
												"type":        "string",
												"description": "The name of the application.",
											},
											"environment": map[string]any{
												"type":        "string",
												"description": "The name of the environment.",
											},
											"database": map[string]any{
												"type":        "string",
												"description": "The name of the database.",
												"readOnly":    true,
											},
										},
										"required": []any{
											"environment",
										},
									},
								},
							},
						},
					},
				},
			}
			resp.SetResponse(http.StatusOK, response, nil)
			return
		},
	}
	return resourceProvidersServer
}

func WithResourceTypeServerNoError() ucpfake.ResourceTypesServer {
	resourceTypesServer := ucpfake.ResourceTypesServer{
		BeginCreateOrUpdate: func(
			ctx context.Context,
			planeName string,
			resourceProviderName string,
			resourceTypeName string,
			resource v20231001preview.ResourceTypeResource,
			options *v20231001preview.ResourceTypesClientBeginCreateOrUpdateOptions,
		) (resp azfake.PollerResponder[v20231001preview.ResourceTypesClientCreateOrUpdateResponse], errResp azfake.ErrorResponder) {
			result := v20231001preview.ResourceTypesClientCreateOrUpdateResponse{
				ResourceTypeResource: resource,
			}

			resp.AddNonTerminalResponse(http.StatusCreated, nil)
			resp.SetTerminalResponse(http.StatusOK, result, nil)

			return
		},
		Get: func(
			ctx context.Context,
			planeName string,
			resourceProviderName string,
			resourceTypeName string,
			options *v20231001preview.ResourceTypesClientGetOptions,
		) (resp azfake.Responder[v20231001preview.ResourceTypesClientGetResponse], errResp azfake.ErrorResponder) {
			response := v20231001preview.ResourceTypesClientGetResponse{
				ResourceTypeResource: v20231001preview.ResourceTypeResource{
					Name: to.Ptr(resourceTypeName),
				},
			}
			resp.SetResponse(http.StatusOK, response, nil)
			return
		},
	}
	return resourceTypesServer
}

func WithAPIVersionServerNoError() ucpfake.APIVersionsServer {
	apiVersionsServer := ucpfake.APIVersionsServer{
		BeginCreateOrUpdate: func(
			ctx context.Context,
			planeName string,
			resourceProviderName string,
			resourceTypeName string,
			apiVersionName string, // Added missing parameter
			resource v20231001preview.APIVersionResource,
			options *v20231001preview.APIVersionsClientBeginCreateOrUpdateOptions,
		) (resp azfake.PollerResponder[v20231001preview.APIVersionsClientCreateOrUpdateResponse], errResp azfake.ErrorResponder) {
			// Simulate successful creation
			result := v20231001preview.APIVersionsClientCreateOrUpdateResponse{
				APIVersionResource: resource,
			}
			resp.AddNonTerminalResponse(http.StatusCreated, nil)
			resp.SetTerminalResponse(http.StatusOK, result, nil)
			return
		},
	}
	return apiVersionsServer
}

func WithLocationServerNoError() ucpfake.LocationsServer {
	locationsServer := ucpfake.LocationsServer{
		BeginCreateOrUpdate: func(
			ctx context.Context,
			planeName string,
			resourceProviderName string,
			locationName string,
			resource v20231001preview.LocationResource,
			options *v20231001preview.LocationsClientBeginCreateOrUpdateOptions,
		) (resp azfake.PollerResponder[v20231001preview.LocationsClientCreateOrUpdateResponse], errResp azfake.ErrorResponder) {
			// Simulate successful creation
			result := v20231001preview.LocationsClientCreateOrUpdateResponse{
				LocationResource: resource,
			}
			resp.AddNonTerminalResponse(http.StatusCreated, nil)
			resp.SetTerminalResponse(http.StatusOK, result, nil)

			return
		},
		Get: func(
			ctx context.Context,
			planeName string,
			resourceProviderName string,
			locationName string,
			options *v20231001preview.LocationsClientGetOptions,
		) (resp azfake.Responder[v20231001preview.LocationsClientGetResponse], errResp azfake.ErrorResponder) {
			response := v20231001preview.LocationsClientGetResponse{
				LocationResource: v20231001preview.LocationResource{
					Name: to.Ptr(locationName),
					ID:   to.Ptr("id"),
					Properties: &v20231001preview.LocationProperties{
						ResourceTypes: map[string]*v20231001preview.LocationResourceType{},
					},
				},
			}
			resp.SetResponse(http.StatusOK, response, nil)
			return
		},
	}
	return locationsServer
}

func WithResourceProviderServerNotFoundError() ucpfake.ResourceProvidersServer {
	resourceProvidersNotFoundServer := ucpfake.ResourceProvidersServer{
		BeginCreateOrUpdate: func(
			ctx context.Context,
			planeName string,
			resourceProviderName string,
			resource v20231001preview.ResourceProviderResource,
			options *v20231001preview.ResourceProvidersClientBeginCreateOrUpdateOptions,
		) (resp azfake.PollerResponder[v20231001preview.ResourceProvidersClientCreateOrUpdateResponse], errResp azfake.ErrorResponder) {
			// Simulate successful creation
			result := v20231001preview.ResourceProvidersClientCreateOrUpdateResponse{
				ResourceProviderResource: resource,
			}
			resp.AddNonTerminalResponse(http.StatusCreated, nil)
			resp.SetTerminalResponse(http.StatusOK, result, nil)

			return
		},
		Get: func(
			ctx context.Context,
			planeName string,
			resourceProviderName string,
			options *v20231001preview.ResourceProvidersClientGetOptions, // Add this parameter
		) (resp azfake.Responder[v20231001preview.ResourceProvidersClientGetResponse], errResp azfake.ErrorResponder) {
			response := v20231001preview.ResourceProvidersClientGetResponse{
				ResourceProviderResource: v20231001preview.ResourceProviderResource{
					Name: to.Ptr(resourceProviderName),
				},
			}
			resp.SetResponse(http.StatusNotFound, response, nil)
			return
		},
		GetProviderSummary: func(
			ctx context.Context,
			planeName string,
			resourceProviderName string,
			options *v20231001preview.ResourceProvidersClientGetProviderSummaryOptions,
		) (resp azfake.Responder[v20231001preview.ResourceProvidersClientGetProviderSummaryResponse], errResp azfake.ErrorResponder) {
			resp.SetResponse(http.StatusNotFound, v20231001preview.ResourceProvidersClientGetProviderSummaryResponse{}, nil)
			return
		},
	}
	return resourceProvidersNotFoundServer
}

func WithResourceProviderServerInternalError() ucpfake.ResourceProvidersServer {
	resourceProvidersServerInternalError := ucpfake.ResourceProvidersServer{
		BeginCreateOrUpdate: func(
			ctx context.Context,
			planeName string,
			resourceProviderName string,
			resource v20231001preview.ResourceProviderResource,
			options *v20231001preview.ResourceProvidersClientBeginCreateOrUpdateOptions,
		) (resp azfake.PollerResponder[v20231001preview.ResourceProvidersClientCreateOrUpdateResponse], errResp azfake.ErrorResponder) {
			// Simulate successful creation
			result := v20231001preview.ResourceProvidersClientCreateOrUpdateResponse{
				ResourceProviderResource: resource,
			}
			resp.AddNonTerminalResponse(http.StatusCreated, nil)
			resp.SetTerminalResponse(http.StatusOK, result, nil)

			return
		},
		Get: func(
			ctx context.Context,
			planeName string,
			resourceProviderName string,
			options *v20231001preview.ResourceProvidersClientGetOptions, // Add this parameter
		) (resp azfake.Responder[v20231001preview.ResourceProvidersClientGetResponse], errResp azfake.ErrorResponder) {
			response := v20231001preview.ResourceProvidersClientGetResponse{
				ResourceProviderResource: v20231001preview.ResourceProviderResource{
					Name: to.Ptr(resourceProviderName),
				},
			}
			resp.SetResponse(http.StatusInternalServerError, response, nil)
			return
		},
		GetProviderSummary: func(
			ctx context.Context,
			planeName string,
			resourceProviderName string,
			options *v20231001preview.ResourceProvidersClientGetProviderSummaryOptions,
		) (resp azfake.Responder[v20231001preview.ResourceProvidersClientGetProviderSummaryResponse], errResp azfake.ErrorResponder) {
			resp.SetResponse(http.StatusInternalServerError, v20231001preview.ResourceProvidersClientGetProviderSummaryResponse{}, nil)
			return
		},
	}
	return resourceProvidersServerInternalError
}
