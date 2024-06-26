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

package embedded

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/armrpc/asyncoperation/controller"
	"github.com/radius-project/radius/pkg/portableresources"
	"github.com/radius-project/radius/pkg/portableresources/processors"
	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/recipes/configloader"
	"github.com/radius-project/radius/pkg/recipes/engine"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/pkg/ucp/datamodel"
	"github.com/radius-project/radius/pkg/ucp/frontend/controller/resourcegroups"
	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/radius-project/radius/pkg/ucp/store"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

const (
	apiVersion    = "2023-10-01-preview"
	applicationID = "/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/applications/test-app"
	environmentID = "/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/environments/test-env"
	resourceName  = "testResource"
	resourceType  = "Applications.Test/testResources"
	resourceID    = "/planes/radius/local/resourceGroups/test-group/providers/Applications.Test/testResources/testResource"
)

func createController(t *testing.T) (*Controller, *store.MockStorageClient, *engine.MockEngine, *processors.MockResourceClient, *configloader.MockConfigurationLoader) {
	ctrl := gomock.NewController(t)

	// Recipe types
	engine := engine.NewMockEngine(ctrl)
	client := processors.NewMockResourceClient(ctrl)
	configLoader := configloader.NewMockConfigurationLoader(ctrl)

	// Controller types
	storageClient := store.NewMockStorageClient(ctrl)
	opts := controller.Options{
		StorageClient: storageClient,
		ResourceType:  resourceType,
	}

	return &Controller{
		BaseController: controller.NewBaseAsyncController(opts),
		opts:           opts,
		engine:         engine,
		client:         client,
		configLoader:   configLoader,
	}, storageClient, engine, client, configLoader
}

func createRequest(operationMethod string) *controller.Request {
	return &controller.Request{
		APIVersion:    apiVersion,
		ResourceID:    resourceID,
		OperationID:   uuid.New(),
		OperationType: "APPLICATIONS.TEST/TESTRESOURCES|" + strings.ToUpper(operationMethod),
	}
}

func mockResourceProvider(storageClient *store.MockStorageClient) {
	providerID := resourcegroups.MakeResourceProviderID(resources.MustParse(resourceID))
	provider := &datamodel.ResourceProvider{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:   providerID.String(),
				Name: providerID.Name(),
				Type: providerID.Type(),
			},
		},
		Properties: datamodel.ResourceProviderProperties{
			Locations: map[string]datamodel.ResourceProviderLocation{
				v1.LocationGlobal: {
					Address: "internal",
				},
			},
			ResourceTypes: []datamodel.ResourceType{
				{
					ResourceType: "testResources",
					APIVersions: map[string]datamodel.ResourceTypeAPIVersion{
						apiVersion: {
							Schema: map[string]any{},
						},
					},
				},
			},
		},
	}
	storageClient.EXPECT().
		Get(gomock.Any(), providerID.String()).
		Return(&store.Object{Data: provider}, nil).
		AnyTimes()
}

func mockResource(storageClient *store.MockStorageClient, resource *datamodel.DynamicResource) {
	storageClient.EXPECT().
		Get(gomock.Any(), resourceID).
		Return(&store.Object{Data: resource}, nil).
		AnyTimes()
}

func mockResourceSave(storageClient *store.MockStorageClient, resource **datamodel.DynamicResource) {
	storageClient.EXPECT().
		Save(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, o *store.Object, so ...store.SaveOptions) error {
			*resource = o.Data.(*datamodel.DynamicResource)
			return nil
		}).
		AnyTimes()
}

func mockResourceDelete(storageClient *store.MockStorageClient) {
	storageClient.EXPECT().
		Delete(gomock.Any(), resourceID, gomock.Any()).
		Return(nil).
		AnyTimes()
}

func mockConfig(configLoader *configloader.MockConfigurationLoader, config *recipes.Configuration) {
	configLoader.EXPECT().
		LoadConfiguration(gomock.Any(), recipes.ResourceMetadata{
			Name:          "",
			ApplicationID: applicationID,
			EnvironmentID: environmentID,
			ResourceID:    resourceID,
			Parameters:    nil,
		}).
		Return(config, nil).
		AnyTimes()
}

func Test_Run_InvalidOperation(t *testing.T) {
	c, storageClient, _, _, _ := createController(t)

	mockResourceProvider(storageClient)

	req := createRequest("invalid")
	result, err := c.Run(context.Background(), req)
	require.NoError(t, err)

	expected := controller.NewFailedResult(v1.ErrorDetails{
		Code:    v1.CodeInvalid,
		Message: "Invalid operation type: \"APPLICATIONS.TEST/TESTRESOURCES|INVALID\"",
		Target:  resourceID,
	})
	require.Equal(t, expected, result)
}

func Test_Run_Put(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		c, storageClient, e, _, configLoader := createController(t)

		mockResourceProvider(storageClient)
		mockResource(storageClient, &datamodel.DynamicResource{
			BaseResource: v1.BaseResource{
				TrackedResource: v1.TrackedResource{
					ID:   resourceID,
					Name: resourceName,
					Type: resourceType,
				},
			},
			Properties: map[string]any{
				"application": applicationID,
				"environment": environmentID,
			},
		})

		var saved *datamodel.DynamicResource
		mockResourceSave(storageClient, &saved)

		mockConfig(configLoader, &recipes.Configuration{})

		e.EXPECT().
			Execute(gomock.Any(), engine.ExecuteOptions{
				BaseOptions: engine.BaseOptions{
					Recipe: recipes.ResourceMetadata{
						Name:          "",
						ApplicationID: applicationID,
						EnvironmentID: environmentID,
						ResourceID:    resourceID,
					},
				},
				PreviousState: []string{},
				Simulated:     false,
			}).
			Return(&recipes.RecipeOutput{
				Resources: []string{
					"/planes/azure/azurecloud/subscriptions/abcb/resourceGroups/my-group/providers/Microsoft.Storage/storageAccounts/my-storage",
				},
				Secrets: map[string]any{
					"password": "abcd",
				},
				Values: map[string]any{
					"uri":      "http://example.com",
					"username": "cooluser",
				},
				Status: &rpv1.RecipeStatus{
					TemplateKind:    "example",
					TemplatePath:    "example/template",
					TemplateVersion: "1.0.0",
				},
			}, nil).
			Times(1)

		req := createRequest(http.MethodPut)
		result, err := c.Run(context.Background(), req)
		require.NoError(t, err)
		require.Equal(t, controller.Result{}, result)

		expected := &datamodel.DynamicResource{
			BaseResource: v1.BaseResource{
				TrackedResource: v1.TrackedResource{
					ID:   resourceID,
					Name: resourceName,
					Type: resourceType,
				},
			},
			Properties: map[string]any{
				"application": applicationID,
				"environment": environmentID,
				"recipe": &portableresources.ResourceRecipe{
					DeploymentStatus: "success",
				},
				"status": &datamodel.DynamicResourceStatus{
					Binding: map[string]any{
						"password": "abcd", // TODO: no secret support yet.
						"uri":      to.Ptr[any]("http://example.com"),
						"username": to.Ptr[any]("cooluser"),
					},
					ResourceStatus: rpv1.ResourceStatus{
						Recipe: &rpv1.RecipeStatus{
							TemplateKind:    "example",
							TemplatePath:    "example/template",
							TemplateVersion: "1.0.0",
						},
						OutputResources: []rpv1.OutputResource{
							{
								LocalID:       "",
								RadiusManaged: to.Ptr(true),
								ID:            resources.MustParse("/planes/azure/azurecloud/subscriptions/abcb/resourceGroups/my-group/providers/Microsoft.Storage/storageAccounts/my-storage"),
							},
						},
					},
				},
			},
		}

		require.Equal(t, expected.Properties["status"], saved.Properties["status"])
		require.Equal(t, expected, saved)
	})
}

func Test_Run_Delete(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		c, storageClient, e, _, configLoader := createController(t)

		mockResourceProvider(storageClient)
		mockResource(storageClient, &datamodel.DynamicResource{
			BaseResource: v1.BaseResource{
				TrackedResource: v1.TrackedResource{
					ID:   resourceID,
					Name: resourceName,
					Type: resourceType,
				},
			},
			Properties: map[string]any{
				"application": applicationID,
				"environment": environmentID,
				"recipe": &portableresources.ResourceRecipe{
					DeploymentStatus: "success",
				},
				"status": &datamodel.DynamicResourceStatus{
					Binding: map[string]any{
						"password": "abcd", // TODO: no secret support yet.
						"uri":      to.Ptr[any]("http://example.com"),
						"username": to.Ptr[any]("cooluser"),
					},
					ResourceStatus: rpv1.ResourceStatus{
						Recipe: &rpv1.RecipeStatus{
							TemplateKind:    "example",
							TemplatePath:    "example/template",
							TemplateVersion: "1.0.0",
						},
						OutputResources: []rpv1.OutputResource{
							{
								LocalID:       "",
								RadiusManaged: to.Ptr(true),
								ID:            resources.MustParse("/planes/azure/azurecloud/subscriptions/abcb/resourceGroups/my-group/providers/Microsoft.Storage/storageAccounts/my-storage"),
							},
						},
					},
				},
			},
		})

		mockResourceDelete(storageClient)

		mockConfig(configLoader, &recipes.Configuration{})

		e.EXPECT().
			Delete(gomock.Any(), engine.DeleteOptions{
				BaseOptions: engine.BaseOptions{
					Recipe: recipes.ResourceMetadata{
						Name:          "",
						ApplicationID: applicationID,
						EnvironmentID: environmentID,
						ResourceID:    resourceID,
					},
				},
				OutputResources: []rpv1.OutputResource{
					{
						LocalID:       "",
						RadiusManaged: to.Ptr(true),
						ID:            resources.MustParse("/planes/azure/azurecloud/subscriptions/abcb/resourceGroups/my-group/providers/Microsoft.Storage/storageAccounts/my-storage"),
					},
				},
			}).
			Return(nil).
			Times(1)

		req := createRequest(http.MethodDelete)
		result, err := c.Run(context.Background(), req)
		require.NoError(t, err)
		require.Equal(t, controller.Result{}, result)
	})
}
