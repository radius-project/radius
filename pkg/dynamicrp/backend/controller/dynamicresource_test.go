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
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	ctrl "github.com/radius-project/radius/pkg/armrpc/asyncoperation/controller"
	aztoken "github.com/radius-project/radius/pkg/azure/tokencredentials"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/ucp/api/v20231001preview/fake"
	"github.com/radius-project/radius/pkg/ucp/datamodel"
	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/stretchr/testify/require"
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
		controller, err := NewDynamicResourceController(ctrl.Options{}, ucp, nil, nil)
		require.NoError(t, err)
		return controller.(*DynamicResourceController)
	}

	t.Run("inert PUT", func(t *testing.T) {
		controller := setup()
		request := &ctrl.Request{
			ResourceID:    "/planes/radius/local/resourceGroups/test-group/providers/" + inertResourceType + "/test-resource",
			OperationType: v1.OperationType{Type: inertResourceType, Method: v1.OperationPut}.String(),
		}

		selected, err := controller.selectController(context.Background(), request)
		require.NoError(t, err)

		require.IsType(t, &InertPutController{}, selected)
	})

	t.Run("inert DELETE", func(t *testing.T) {
		controller := setup()
		request := &ctrl.Request{
			ResourceID:    "/planes/radius/local/resourceGroups/test-group/providers/" + inertResourceType + "/test-resource",
			OperationType: v1.OperationType{Type: inertResourceType, Method: v1.OperationDelete}.String(),
		}

		selected, err := controller.selectController(context.Background(), request)
		require.NoError(t, err)

		require.IsType(t, &InertDeleteController{}, selected)
	})

	t.Run("recipe PUT", func(t *testing.T) {
		controller := setup()
		request := &ctrl.Request{
			ResourceID:    "/planes/radius/local/resourceGroups/test-group/providers/" + recipeResourceType + "/test-resource",
			OperationType: v1.OperationType{Type: recipeResourceType, Method: v1.OperationPut}.String(),
		}

		selected, err := controller.selectController(context.Background(), request)
		require.NoError(t, err)

		require.IsType(t, &RecipePutController{}, selected)
	})

	t.Run("recipe DELETE", func(t *testing.T) {
		controller := setup()
		request := &ctrl.Request{
			ResourceID:    "/planes/radius/local/resourceGroups/test-group/providers/" + recipeResourceType + "/test-resource",
			OperationType: v1.OperationType{Type: recipeResourceType, Method: v1.OperationDelete}.String(),
		}

		selected, err := controller.selectController(context.Background(), request)
		require.NoError(t, err)

		require.IsType(t, &RecipeDeleteController{}, selected)
	})

	t.Run("unknown operation", func(t *testing.T) {
		controller := setup()
		request := &ctrl.Request{
			ResourceID:    "/planes/radius/local/resourceGroups/test-group/providers/" + inertResourceType + "/test-resource",
			OperationType: v1.OperationType{Type: inertResourceType, Method: v1.OperationGet}.String(),
		}

		selected, err := controller.selectController(context.Background(), request)
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
				ResourceTypeResource: v20231001preview.ResourceTypeResource{
					Name: to.Ptr(resourceTypeName),
				},
			}

			switch resourceType {
			case inertResourceType:
				response.Properties = &v20231001preview.ResourceTypeProperties{}
				resp.SetResponse(http.StatusOK, response, nil)
				return
			case recipeResourceType:
				response.Properties = &v20231001preview.ResourceTypeProperties{
					Capabilities: []*string{to.Ptr(datamodel.CapabilitySupportsRecipes)},
				}
				resp.SetResponse(http.StatusOK, response, nil)
				return
			default:
				errResp.SetError(fmt.Errorf("resource type %s not recognized", resourceType))
				return
			}
		},
	}

	return v20231001preview.NewClientFactory(&aztoken.AnonymousCredential{}, &armpolicy.ClientOptions{
		ClientOptions: policy.ClientOptions{
			Transport: fake.NewServerFactoryTransport(&fake.ServerFactory{
				ResourceTypesServer: resourceTypesServer,
			}),
		},
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
					Capabilities: []*string{to.Ptr("capability1"), to.Ptr("capability2")},
				},
			},
			capability: "capability1",
			want:       true,
		},
		{
			name: "does not have capability",
			resourceType: &v20231001preview.ResourceTypeResource{
				Properties: &v20231001preview.ResourceTypeProperties{
					Capabilities: []*string{to.Ptr("capability1"), to.Ptr("capability2")},
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
		controller, err := NewDynamicResourceController(ctrl.Options{}, ucp, nil, nil)
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

			resourceType, err := controller.fetchResourceTypeDetails(context.Background(), id)
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
