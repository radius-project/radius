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

package controller

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	armpolicy "github.com/Azure/azure-sdk-for-go/sdk/azcore/arm/policy"
	azfake "github.com/Azure/azure-sdk-for-go/sdk/azcore/fake"
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	ctrl "github.com/radius-project/radius/pkg/armrpc/asyncoperation/controller"
	aztoken "github.com/radius-project/radius/pkg/azure/tokencredentials"
	"github.com/radius-project/radius/pkg/components/database"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/ucp/api/v20231001preview/fake"
	"github.com/radius-project/radius/pkg/ucp/datamodel"
	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

const (
	inertResourceType  = "Applications.Test/testInertResources"
	recipeResourceType = "Applications.Test/testRecipeResources"
)

func Test_DynamicResourceController_selectController(t *testing.T) {
	setup := func() *DynamicResourceController {
		ucp, err := testUCPClientFactory()
		require.NoError(t, err)

		// The recipe engine and configuration loader are not used in this test.
		controller, err := NewDynamicResourceController(ctrl.Options{}, ucp, nil, nil, nil)
		require.NoError(t, err)
		return controller.(*DynamicResourceController)
	}

	selectController := func(t *testing.T, request *ctrl.Request) (ctrl.Controller, error) {
		controller := setup()
		operationType, resourceTypeDetails, err := controller.extractOperationAndResourceTypeDetails(t.Context(), request)
		require.NoError(t, err)
		return controller.selectController(operationType, resourceTypeDetails)
	}

	t.Run("inert PUT", func(t *testing.T) {
		request := &ctrl.Request{
			ResourceID:    "/planes/radius/local/resourceGroups/test-group/providers/" + inertResourceType + "/test-resource",
			OperationType: v1.OperationType{Type: inertResourceType, Method: v1.OperationPut}.String(),
		}

		selected, err := selectController(t, request)
		require.NoError(t, err)

		require.IsType(t, &InertPutController{}, selected)
	})

	t.Run("inert DELETE", func(t *testing.T) {
		request := &ctrl.Request{
			ResourceID:    "/planes/radius/local/resourceGroups/test-group/providers/" + inertResourceType + "/test-resource",
			OperationType: v1.OperationType{Type: inertResourceType, Method: v1.OperationDelete}.String(),
		}

		selected, err := selectController(t, request)
		require.NoError(t, err)

		require.IsType(t, &InertDeleteController{}, selected)
	})

	t.Run("recipe PUT", func(t *testing.T) {
		request := &ctrl.Request{
			ResourceID:    "/planes/radius/local/resourceGroups/test-group/providers/" + recipeResourceType + "/test-resource",
			OperationType: v1.OperationType{Type: recipeResourceType, Method: v1.OperationPut}.String(),
		}

		selected, err := selectController(t, request)
		require.NoError(t, err)

		require.IsType(t, &RecipePutController{}, selected)
	})

	t.Run("recipe DELETE", func(t *testing.T) {
		request := &ctrl.Request{
			ResourceID:    "/planes/radius/local/resourceGroups/test-group/providers/" + recipeResourceType + "/test-resource",
			OperationType: v1.OperationType{Type: recipeResourceType, Method: v1.OperationDelete}.String(),
		}

		selected, err := selectController(t, request)
		require.NoError(t, err)

		require.IsType(t, &RecipeDeleteController{}, selected)
	})

	t.Run("unknown operation", func(t *testing.T) {
		request := &ctrl.Request{
			ResourceID:    "/planes/radius/local/resourceGroups/test-group/providers/" + inertResourceType + "/test-resource",
			OperationType: v1.OperationType{Type: inertResourceType, Method: v1.OperationGet}.String(),
		}

		selected, err := selectController(t, request)
		require.Error(t, err)
		require.Equal(t, "unsupported operation type: \"APPLICATIONS.TEST/TESTINERTRESOURCES|GET\"", err.Error())
		require.Nil(t, selected)
	})
}

func testUCPClientFactory() (*v20231001preview.ClientFactory, error) {
	resourceTypesServer := fake.ResourceTypesServer{
		Get: func(ctx context.Context, planeName, resourceProviderName, resourceTypeName string, options *v20231001preview.ResourceTypesClientGetOptions) (resp azfake.Responder[v20231001preview.ResourceTypesClientGetResponse], errResp azfake.ErrorResponder) {
			resourceType := resourceProviderName + resources.SegmentSeparator + resourceTypeName
			response := v20231001preview.ResourceTypesClientGetResponse{
				Name: new(resourceTypeName),
			}

			switch resourceType {
			case inertResourceType:
				response.Properties = &v20231001preview.ResourceTypeProperties{
					Capabilities: []*string{to.Ptr(datamodel.CapabilityManualResourceProvisioning)},
				}
				resp.SetResponse(http.StatusOK, response, nil)
				return
			case recipeResourceType:
				response.Properties = &v20231001preview.ResourceTypeProperties{}
				resp.SetResponse(http.StatusOK, response, nil)
				return
			default:
				errResp.SetError(fmt.Errorf("resource type %s not recognized", resourceType))
				return
			}
		},
	}

	return v20231001preview.NewClientFactory(&aztoken.AnonymousCredential{}, &armpolicy.ClientOptions{
		Transport: fake.NewServerFactoryTransport(&fake.ServerFactory{
			ResourceTypesServer: resourceTypesServer,
		}),
	})
}

func Test_hasCapability(t *testing.T) {
	tests := []struct {
		name         string
		resourceType *v20231001preview.ResourceTypeResource
		capability   string
		want         bool
	}{
		{
			name: "has capability",
			resourceType: &v20231001preview.ResourceTypeResource{
				Properties: &v20231001preview.ResourceTypeProperties{
					Capabilities: []*string{new("capability1"), new("capability2")},
				},
			},
			capability: "capability1",
			want:       true,
		},
		{
			name: "does not have capability",
			resourceType: &v20231001preview.ResourceTypeResource{
				Properties: &v20231001preview.ResourceTypeProperties{
					Capabilities: []*string{new("capability1"), new("capability2")},
				},
			},
			capability: "capability3",
			want:       false,
		},
		{
			name: "nil capabilities",
			resourceType: &v20231001preview.ResourceTypeResource{
				Properties: &v20231001preview.ResourceTypeProperties{
					Capabilities: nil,
				},
			},
			capability: "capability1",
			want:       false,
		},
		{
			name: "empty capabilities",
			resourceType: &v20231001preview.ResourceTypeResource{
				Properties: &v20231001preview.ResourceTypeProperties{
					Capabilities: []*string{},
				},
			},
			capability: "capability1",
			want:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hasCapability(tt.resourceType, tt.capability)
			require.Equal(t, tt.want, got)
		})
	}
}

func Test_DynamicResourceController_fetchResourceTypeDetails(t *testing.T) {
	setup := func() *DynamicResourceController {
		ucp, err := testUCPClientFactory()
		require.NoError(t, err)
		controller, err := NewDynamicResourceController(ctrl.Options{}, ucp, nil, nil, nil)
		require.NoError(t, err)
		return controller.(*DynamicResourceController)
	}

	tests := []struct {
		name       string
		resourceID string
		wantErr    bool
		errMessage string
	}{
		{
			name:       "inert resource type found",
			resourceID: "/planes/radius/local/resourceGroups/test-group/providers/" + inertResourceType + "/test-resource",
			wantErr:    false,
		},
		{
			name:       "recipe resource type found",
			resourceID: "/planes/radius/local/resourceGroups/test-group/providers/" + recipeResourceType + "/test-resource",
			wantErr:    false,
		},
		{
			name:       "unknown resource type",
			resourceID: "/planes/radius/local/resourceGroups/test-group/providers/Applications.Test/unknownType/test-resource",
			wantErr:    true,
			errMessage: "resource type Applications.Test/unknownType not recognized",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			controller := setup()
			id, err := resources.ParseResource(tt.resourceID)
			require.NoError(t, err)

			resourceType, err := controller.fetchResourceTypeDetails(t.Context(), id)
			if tt.wantErr {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.errMessage)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, resourceType)
			require.NotNil(t, resourceType.Properties)
		})
	}
}

func TestDynamicResourceController_validateRequestSchema(t *testing.T) {
	// Note: The validation logic is tested in the schema package tests.

	setup := func() *DynamicResourceController {
		ucp, err := testUCPClientFactory()
		require.NoError(t, err)

		controller, err := NewDynamicResourceController(ctrl.Options{}, ucp, nil, nil, nil)
		require.NoError(t, err)
		return controller.(*DynamicResourceController)
	}

	t.Run("skip validation for non-PUT operations", func(t *testing.T) {
		controller := setup()
		request := &ctrl.Request{
			ResourceID:    "/planes/radius/local/resourceGroups/test-group/providers/" + inertResourceType + "/test-resource",
			OperationType: v1.OperationType{Type: inertResourceType, Method: v1.OperationDelete}.String(),
		}

		operationType, resourceTypeDetails, err := controller.extractOperationAndResourceTypeDetails(t.Context(), request)
		require.NoError(t, err)

		err = controller.validateRequestSchema(t.Context(), request, operationType, resourceTypeDetails)
		require.NoError(t, err) // Should skip validation for DELETE
	})
}

func Test_DynamicResourceController_Run(t *testing.T) {
	resourceWithSize := func(size any) *database.Object {
		return &database.Object{Data: map[string]any{"properties": map[string]any{"size": size}}}
	}

	tests := []struct {
		name         string
		resourceType string
		method       v1.OperationMethod
		setupDB      func(db *database.MockClient, resourceID string)
		want         ctrl.Result
	}{
		{
			name:         "inert DELETE",
			resourceType: inertResourceType,
			method:       v1.OperationDelete,
			setupDB: func(db *database.MockClient, resourceID string) {
				db.EXPECT().Delete(gomock.Any(), resourceID).Return(nil)
			},
			want: ctrl.Result{},
		},
		{
			name:         "inert PUT with valid properties",
			resourceType: inertResourceType,
			method:       v1.OperationPut,
			setupDB: func(db *database.MockClient, resourceID string) {
				db.EXPECT().Get(gomock.Any(), resourceID).Return(resourceWithSize("small"), nil)
			},
			want: ctrl.Result{},
		},
		{
			// The recipe PUT controller reads the resource again, so the mock fails if it runs.
			name:         "schema validation failure",
			resourceType: recipeResourceType,
			method:       v1.OperationPut,
			setupDB: func(db *database.MockClient, resourceID string) {
				db.EXPECT().Get(gomock.Any(), resourceID).Return(resourceWithSize(3), nil)
			},
			want: ctrl.NewFailedResult(v1.ErrorDetails{
				Code:    v1.CodeInvalidRequestContent,
				Message: `code InvalidRequestContent: err Schema validation failed: resource data validation failed: Error at "size": value must be a string`,
			}),
		},
		{
			name:         "resource type lookup failure",
			resourceType: "Applications.Test/unknownType",
			method:       v1.OperationPut,
			want: ctrl.NewFailedResult(v1.ErrorDetails{
				Code:    v1.CodeInvalidRequestContent,
				Message: "failed to fetch resource type details: resource type Applications.Test/unknownType not recognized",
			}),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gets := 0
			resourceTypesServer := fake.ResourceTypesServer{
				Get: func(ctx context.Context, planeName, resourceProviderName, resourceTypeName string, options *v20231001preview.ResourceTypesClientGetOptions) (resp azfake.Responder[v20231001preview.ResourceTypesClientGetResponse], errResp azfake.ErrorResponder) {
					gets++
					properties := &v20231001preview.ResourceTypeProperties{}
					switch resourceProviderName + resources.SegmentSeparator + resourceTypeName {
					case inertResourceType:
						properties.Capabilities = []*string{to.Ptr(datamodel.CapabilityManualResourceProvisioning)}
					case recipeResourceType:
					default:
						errResp.SetError(fmt.Errorf("resource type %s/%s not recognized", resourceProviderName, resourceTypeName))
						return
					}
					resp.SetResponse(http.StatusOK, v20231001preview.ResourceTypesClientGetResponse{
						ResourceTypeResource: v20231001preview.ResourceTypeResource{
							Name:       new(resourceTypeName),
							Properties: properties,
						},
					}, nil)
					return
				},
			}
			apiVersionsServer := fake.APIVersionsServer{
				Get: func(ctx context.Context, planeName, resourceProviderName, resourceTypeName, apiVersionName string, options *v20231001preview.APIVersionsClientGetOptions) (resp azfake.Responder[v20231001preview.APIVersionsClientGetResponse], errResp azfake.ErrorResponder) {
					resp.SetResponse(http.StatusOK, v20231001preview.APIVersionsClientGetResponse{
						APIVersionResource: v20231001preview.APIVersionResource{
							Properties: &v20231001preview.APIVersionProperties{
								Schema: map[string]any{
									"type": "object",
									"properties": map[string]any{
										"size": map[string]any{"type": "string"},
									},
								},
							},
						},
					}, nil)
					return
				},
			}
			ucp, err := v20231001preview.NewClientFactory(&aztoken.AnonymousCredential{}, &armpolicy.ClientOptions{
				Transport: fake.NewServerFactoryTransport(&fake.ServerFactory{
					APIVersionsServer:   apiVersionsServer,
					ResourceTypesServer: resourceTypesServer,
				}),
			})
			require.NoError(t, err)

			databaseClient := database.NewMockClient(gomock.NewController(t))
			controller, err := NewDynamicResourceController(ctrl.Options{DatabaseClient: databaseClient}, ucp, nil, nil, nil)
			require.NoError(t, err)

			request := &ctrl.Request{
				ResourceID:    "/planes/radius/local/resourceGroups/test-group/providers/" + tt.resourceType + "/test-resource",
				OperationType: v1.OperationType{Type: tt.resourceType, Method: tt.method}.String(),
				APIVersion:    "2025-01-01-preview",
			}
			if tt.setupDB != nil {
				tt.setupDB(databaseClient, request.ResourceID)
			}

			result, err := controller.Run(t.Context(), request)
			require.NoError(t, err)
			require.Equal(t, tt.want, result)
			require.Equal(t, 1, gets, "Run() resource type GETs")
		})
	}

	t.Run("invalid resource ID", func(t *testing.T) {
		ucp, err := testUCPClientFactory()
		require.NoError(t, err)

		controller, err := NewDynamicResourceController(ctrl.Options{}, ucp, nil, nil, nil)
		require.NoError(t, err)

		request := &ctrl.Request{
			ResourceID:    "invalid-resource-id",
			OperationType: v1.OperationType{Type: inertResourceType, Method: v1.OperationPut}.String(),
		}

		result, err := controller.Run(t.Context(), request)
		require.NoError(t, err)
		require.Equal(t, ctrl.NewFailedResult(v1.ErrorDetails{
			Code:    v1.CodeInvalidRequestContent,
			Message: `invalid resource ID: "invalid-resource-id"`,
		}), result)
	})
}
