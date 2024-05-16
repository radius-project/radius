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
	"net/http"
	"reflect"
	"strconv"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/cli/clients_new/generated"
	corerp "github.com/radius-project/radius/pkg/corerp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/to"
	ucp "github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

const (
	testScope    = "/planes/radius/local/resourceGroups/my-default-rg"
	anotherScope = "/planes/radius/local/resourceGroups/my-other-rg"
)

func Test_Resource(t *testing.T) {
	createClient := func(wrapped genericResourceClient) *UCPApplicationsManagementClient {
		return &UCPApplicationsManagementClient{
			RootScope: testScope,
			genericResourceClientFactory: func(scope string, resourceType string) (genericResourceClient, error) {
				return wrapped, nil
			},
			capture: testCapture,
		}
	}

	testResourceType := "Applications.Test/testResource"
	testResourceName := "test-resource-name"
	testResourceID := testScope + "/providers/" + testResourceType + "/" + testResourceName

	expectedResource := generated.GenericResource{
		ID:       &testResourceID,
		Name:     &testResourceName,
		Type:     &testResourceType,
		Location: to.Ptr(v1.LocationGlobal),
	}

	listPages := []generated.GenericResourcesClientListByRootScopeResponse{
		{
			GenericResourcesList: generated.GenericResourcesList{
				Value: []*generated.GenericResource{
					{
						ID:       to.Ptr(testScope + "/providers/" + testResourceType + "/" + "test1"),
						Name:     to.Ptr("test1"),
						Type:     &testResourceType,
						Location: to.Ptr(v1.LocationGlobal),
						Properties: map[string]any{
							"application": testScope + "/providers/Applications.Core/applications/test-application",
							"environment": testScope + "/providers/Applications.Core/environments/test-environment",
						},
					},
					{
						ID:       to.Ptr(testScope + "/providers/" + testResourceType + "/" + "test2"),
						Name:     to.Ptr("test2"),
						Type:     &testResourceType,
						Location: to.Ptr(v1.LocationGlobal),
						Properties: map[string]any{
							"environment": testScope + "/providers/Applications.Core/environments/test-environment",
						},
					},
				},
				NextLink: to.Ptr("0"),
			},
		},
		{
			GenericResourcesList: generated.GenericResourcesList{
				Value: []*generated.GenericResource{
					{
						ID:       to.Ptr(testScope + "/providers/" + testResourceType + "/" + "test3"),
						Name:     to.Ptr("test3"),
						Type:     &testResourceType,
						Location: to.Ptr(v1.LocationGlobal),
						Properties: map[string]any{
							"application": anotherScope + "/providers/Applications.Core/applications/test-application",
							"environment": anotherScope + "/providers/Applications.Core/environments/test-environment",
						},
					},
					{
						ID:       to.Ptr(testScope + "/providers/" + testResourceType + "/" + "test4"),
						Name:     to.Ptr("test4"),
						Type:     &testResourceType,
						Location: to.Ptr(v1.LocationGlobal),
						Properties: map[string]any{
							"environment": anotherScope + "/providers/Applications.Core/environments/test-environment",
						},
					},
				},
				NextLink: to.Ptr("1"),
			},
		},
	}

	t.Run("ListResourcesOfType", func(t *testing.T) {
		mock := NewMockgenericResourceClient(gomock.NewController(t))
		client := createClient(mock)

		mock.EXPECT().
			NewListByRootScopePager(gomock.Any()).
			Return(pager(listPages))

		expectedResourceList := []generated.GenericResource{*listPages[0].Value[0], *listPages[0].Value[1], *listPages[1].Value[0], *listPages[1].Value[1]}

		resources, err := client.ListResourcesOfType(context.Background(), testResourceType)
		require.NoError(t, err)
		require.Equal(t, expectedResourceList, resources)
	})

	t.Run("ListResourcesOfTypeInApplication", func(t *testing.T) {
		mock := NewMockgenericResourceClient(gomock.NewController(t))
		client := createClient(mock)

		mock.EXPECT().
			NewListByRootScopePager(gomock.Any()).
			Return(pager(listPages))

		expectedResourceList := []generated.GenericResource{*listPages[0].Value[0]}

		resources, err := client.ListResourcesOfTypeInApplication(context.Background(), "test-application", testResourceType)
		require.NoError(t, err)
		require.Equal(t, expectedResourceList, resources)
	})

	t.Run("ListResourcesOfTypeInEnvironment", func(t *testing.T) {
		mock := NewMockgenericResourceClient(gomock.NewController(t))
		client := createClient(mock)

		mock.EXPECT().
			NewListByRootScopePager(gomock.Any()).
			Return(pager(listPages))

		expectedResourceList := []generated.GenericResource{*listPages[0].Value[0], *listPages[0].Value[1]}

		resources, err := client.ListResourcesOfTypeInEnvironment(context.Background(), "test-environment", testResourceType)
		require.NoError(t, err)
		require.Equal(t, expectedResourceList, resources)
	})

	t.Run("ListResourcesInApplication", func(t *testing.T) {
		mock := NewMockgenericResourceClient(gomock.NewController(t))
		client := createClient(mock)

		for i := range ResourceTypesList {
			if i == 0 {
				mock.EXPECT().
					NewListByRootScopePager(gomock.Any()).
					Return(pager(listPages))
			} else {
				mock.EXPECT().
					NewListByRootScopePager(gomock.Any()).
					Return(pager([]generated.GenericResourcesClientListByRootScopeResponse{{GenericResourcesList: generated.GenericResourcesList{NextLink: to.Ptr("0")}}}))
			}
		}

		expectedResourceList := []generated.GenericResource{*listPages[0].Value[0]}

		resources, err := client.ListResourcesInApplication(context.Background(), "test-application")
		require.NoError(t, err)
		require.Equal(t, expectedResourceList, resources)
	})

	t.Run("ListResourcesInEnvironment", func(t *testing.T) {
		mock := NewMockgenericResourceClient(gomock.NewController(t))
		client := createClient(mock)

		for i := range ResourceTypesList {
			if i == 0 {
				mock.EXPECT().
					NewListByRootScopePager(gomock.Any()).
					Return(pager(listPages))
			} else {
				mock.EXPECT().
					NewListByRootScopePager(gomock.Any()).
					Return(pager([]generated.GenericResourcesClientListByRootScopeResponse{{GenericResourcesList: generated.GenericResourcesList{NextLink: to.Ptr("0")}}}))
			}
		}

		expectedResourceList := []generated.GenericResource{*listPages[0].Value[0], *listPages[0].Value[1]}

		resources, err := client.ListResourcesInEnvironment(context.Background(), "test-environment")
		require.NoError(t, err)
		require.Equal(t, expectedResourceList, resources)
	})

	t.Run("GetResource", func(t *testing.T) {
		mock := NewMockgenericResourceClient(gomock.NewController(t))
		client := createClient(mock)

		mock.EXPECT().
			Get(gomock.Any(), testResourceName, gomock.Any()).
			Return(generated.GenericResourcesClientGetResponse{GenericResource: expectedResource}, nil)

		resource, err := client.GetResource(context.Background(), testResourceType, testResourceID)
		require.NoError(t, err)
		require.Equal(t, expectedResource, resource)
	})

	t.Run("DeleteResource", func(t *testing.T) {
		mock := NewMockgenericResourceClient(gomock.NewController(t))
		client := createClient(mock)

		mock.EXPECT().
			BeginDelete(gomock.Any(), testResourceName, gomock.Any()).
			Return(poller(&generated.GenericResourcesClientDeleteResponse{}), nil)

		deleted, err := client.DeleteResource(context.Background(), testResourceType, testResourceID)
		require.NoError(t, err)
		require.True(t, deleted)
	})
}

func Test_Application(t *testing.T) {
	createClient := func(wrapped applicationResourceClient) *UCPApplicationsManagementClient {
		return &UCPApplicationsManagementClient{
			RootScope: testScope,
			applicationResourceClientFactory: func(scope string) (applicationResourceClient, error) {
				return wrapped, nil
			},
			capture: testCapture,
		}
	}

	testResourceType := "Applications.Core/applications"
	testResourceName := "test-application"
	testResourceID := testScope + "/providers/" + testResourceType + "/" + testResourceName

	expectedResource := corerp.ApplicationResource{
		ID:       &testResourceID,
		Name:     &testResourceName,
		Type:     &testResourceType,
		Location: to.Ptr(v1.LocationGlobal),
	}

	listPages := []corerp.ApplicationsClientListByScopeResponse{
		{
			ApplicationResourceListResult: corerp.ApplicationResourceListResult{
				Value: []*corerp.ApplicationResource{
					{
						ID:       to.Ptr(testScope + "/providers/" + testResourceType + "/" + "test1"),
						Name:     to.Ptr("test1"),
						Type:     &testResourceType,
						Location: to.Ptr(v1.LocationGlobal),
						Properties: &corerp.ApplicationProperties{
							Environment: to.Ptr(testScope + "/providers/Applications.Core/environments/test-environment"),
						},
					},
					{
						ID:       to.Ptr(testScope + "/providers/" + testResourceType + "/" + "test2"),
						Name:     to.Ptr("test2"),
						Type:     &testResourceType,
						Location: to.Ptr(v1.LocationGlobal),
						Properties: &corerp.ApplicationProperties{
							Environment: to.Ptr(testScope + "/providers/Applications.Core/environments/test-environment"),
						},
					},
				},
				NextLink: to.Ptr("0"),
			},
		},
		{
			ApplicationResourceListResult: corerp.ApplicationResourceListResult{
				Value: []*corerp.ApplicationResource{
					{
						ID:       to.Ptr(testScope + "/providers/" + testResourceType + "/" + "test3"),
						Name:     to.Ptr("test3"),
						Type:     &testResourceType,
						Location: to.Ptr(v1.LocationGlobal),
						Properties: &corerp.ApplicationProperties{
							Environment: to.Ptr(anotherScope + "/providers/Applications.Core/environments/test-environment"),
						},
					},
					{
						ID:       to.Ptr(testScope + "/providers/" + testResourceType + "/" + "test4"),
						Name:     to.Ptr("test4"),
						Type:     &testResourceType,
						Location: to.Ptr(v1.LocationGlobal),
						Properties: &corerp.ApplicationProperties{
							Environment: to.Ptr(anotherScope + "/providers/Applications.Core/environments/test-environment"),
						},
					},
				},
				NextLink: to.Ptr("1"),
			},
		},
	}

	t.Run("ListApplications", func(t *testing.T) {
		mock := NewMockapplicationResourceClient(gomock.NewController(t))
		client := createClient(mock)

		mock.EXPECT().
			NewListByScopePager(gomock.Any()).
			Return(pager(listPages))

		expectedResourceList := []corerp.ApplicationResource{*listPages[0].Value[0], *listPages[0].Value[1], *listPages[1].Value[0], *listPages[1].Value[1]}

		resources, err := client.ListApplications(context.Background())
		require.NoError(t, err)
		require.Equal(t, expectedResourceList, resources)
	})

	t.Run("ListApplicationsInEnvironment", func(t *testing.T) {
		mock := NewMockapplicationResourceClient(gomock.NewController(t))
		client := createClient(mock)

		mock.EXPECT().
			NewListByScopePager(gomock.Any()).
			Return(pager(listPages))

		expectedResourceList := []corerp.ApplicationResource{*listPages[0].Value[0], *listPages[0].Value[1]}

		resources, err := client.ListApplicationsInEnvironment(context.Background(), "test-environment")
		require.NoError(t, err)
		require.Equal(t, expectedResourceList, resources)
	})

	t.Run("GetApplication", func(t *testing.T) {
		mock := NewMockapplicationResourceClient(gomock.NewController(t))
		client := createClient(mock)

		mock.EXPECT().
			Get(gomock.Any(), testResourceName, gomock.Any()).
			Return(corerp.ApplicationsClientGetResponse{ApplicationResource: expectedResource}, nil)

		application, err := client.GetApplication(context.Background(), testResourceID)
		require.NoError(t, err)
		require.Equal(t, expectedResource, application)
	})

	t.Run("GetApplicationGraph", func(t *testing.T) {
		mock := NewMockapplicationResourceClient(gomock.NewController(t))
		client := createClient(mock)

		expectedGraph := corerp.ApplicationGraphResponse{
			Resources: []*corerp.ApplicationGraphResource{
				{
					ID: &testResourceID,
				},
			},
		}

		mock.EXPECT().
			GetGraph(gomock.Any(), testResourceName, gomock.Any(), gomock.Any()).
			Return(corerp.ApplicationsClientGetGraphResponse{ApplicationGraphResponse: expectedGraph}, nil)

		graph, err := client.GetApplicationGraph(context.Background(), testResourceID)
		require.NoError(t, err)
		require.Equal(t, expectedGraph, graph)
	})

	t.Run("CreateOrUpdateApplication", func(t *testing.T) {
		mock := NewMockapplicationResourceClient(gomock.NewController(t))
		client := createClient(mock)

		mock.EXPECT().
			CreateOrUpdate(gomock.Any(), testResourceName, expectedResource, gomock.Any()).
			Return(corerp.ApplicationsClientCreateOrUpdateResponse{}, nil)

		err := client.CreateOrUpdateApplication(context.Background(), testResourceID, &expectedResource)
		require.NoError(t, err)
	})

	t.Run("CreateApplicationIfNotFound", func(t *testing.T) {
		mock := NewMockapplicationResourceClient(gomock.NewController(t))
		client := createClient(mock)

		mock.EXPECT().
			Get(gomock.Any(), testResourceName, gomock.Any()).
			Return(corerp.ApplicationsClientGetResponse{}, &azcore.ResponseError{StatusCode: http.StatusNotFound})

		mock.EXPECT().
			CreateOrUpdate(gomock.Any(), testResourceName, expectedResource, gomock.Any()).
			Return(corerp.ApplicationsClientCreateOrUpdateResponse{}, nil)

		err := client.CreateApplicationIfNotFound(context.Background(), testResourceID, &expectedResource)
		require.NoError(t, err)
	})

	t.Run("DeleteApplication", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mock := NewMockapplicationResourceClient(ctrl)
		genericResourceMock := NewMockgenericResourceClient(ctrl)
		client := createClient(mock)
		client.genericResourceClientFactory = func(scope string, resourceType string) (genericResourceClient, error) {
			return genericResourceMock, nil
		}

		resourceListPages := []generated.GenericResourcesClientListByRootScopeResponse{
			{
				GenericResourcesList: generated.GenericResourcesList{
					Value: []*generated.GenericResource{
						{
							ID:       to.Ptr(testScope + "/providers/Applications.Test/testResources/test1"),
							Name:     to.Ptr("test1"),
							Type:     to.Ptr("Applications.Test/testResources"),
							Location: to.Ptr(v1.LocationGlobal),
							Properties: map[string]any{
								"application": testScope + "/providers/Applications.Core/applications/test-application",
								"environment": testScope + "/providers/Applications.Core/environments/test-environment",
							},
						},
						{
							ID:       to.Ptr(testScope + "/providers/Applications.Test/testResources/test2"),
							Name:     to.Ptr("test2"),
							Type:     to.Ptr("Applications.Test/testResources"),
							Location: to.Ptr(v1.LocationGlobal),
							Properties: map[string]any{
								"environment": testScope + "/providers/Applications.Core/environments/test-environment",
							},
						},
					},
					NextLink: to.Ptr("0"),
				},
			},
		}

		// Handle deletion of resources in the application.
		for i := range ResourceTypesList {
			if i == 0 {
				genericResourceMock.EXPECT().
					NewListByRootScopePager(gomock.Any()).
					Return(pager(resourceListPages))
			} else {
				genericResourceMock.EXPECT().
					NewListByRootScopePager(gomock.Any()).
					Return(pager([]generated.GenericResourcesClientListByRootScopeResponse{{GenericResourcesList: generated.GenericResourcesList{NextLink: to.Ptr("0")}}}))
			}
		}

		genericResourceMock.EXPECT().
			BeginDelete(gomock.Any(), "test1", gomock.Any()).
			Return(poller(&generated.GenericResourcesClientDeleteResponse{}), nil)

		mock.EXPECT().
			Delete(gomock.Any(), testResourceName, gomock.Any()).
			DoAndReturn(func(ctx context.Context, s string, acdo *corerp.ApplicationsClientDeleteOptions) (corerp.ApplicationsClientDeleteResponse, error) {
				setCapture(ctx, &http.Response{StatusCode: 200})
				return corerp.ApplicationsClientDeleteResponse{}, nil
			})

		deleted, err := client.DeleteApplication(context.Background(), testResourceID)
		require.NoError(t, err)
		require.True(t, deleted)
	})
}

func Test_Environment(t *testing.T) {
	createClient := func(wrapped environmentResourceClient) *UCPApplicationsManagementClient {
		return &UCPApplicationsManagementClient{
			RootScope: testScope,
			environmentResourceClientFactory: func(scope string) (environmentResourceClient, error) {
				return wrapped, nil
			},
			capture: testCapture,
		}
	}

	testResourceType := "Applications.Core/environments"
	testResourceName := "test-environment"
	testResourceID := testScope + "/providers/" + testResourceType + "/" + testResourceName

	expectedResource := corerp.EnvironmentResource{
		ID:       &testResourceID,
		Name:     &testResourceName,
		Type:     &testResourceType,
		Location: to.Ptr(v1.LocationGlobal),
	}

	listPages := []corerp.EnvironmentsClientListByScopeResponse{
		{
			EnvironmentResourceListResult: corerp.EnvironmentResourceListResult{
				Value: []*corerp.EnvironmentResource{
					{
						ID:       to.Ptr(testScope + "/providers/" + testResourceType + "/" + "test1"),
						Name:     to.Ptr("test1"),
						Type:     &testResourceType,
						Location: to.Ptr(v1.LocationGlobal),
					},
					{
						ID:       to.Ptr(testScope + "/providers/" + testResourceType + "/" + "test2"),
						Name:     to.Ptr("test2"),
						Type:     &testResourceType,
						Location: to.Ptr(v1.LocationGlobal),
					},
				},
				NextLink: to.Ptr("0"),
			},
		},
		{
			EnvironmentResourceListResult: corerp.EnvironmentResourceListResult{
				Value: []*corerp.EnvironmentResource{
					{
						ID:       to.Ptr(testScope + "/providers/" + testResourceType + "/" + "test3"),
						Name:     to.Ptr("test3"),
						Type:     &testResourceType,
						Location: to.Ptr(v1.LocationGlobal),
					},
					{
						ID:       to.Ptr(testScope + "/providers/" + testResourceType + "/" + "test4"),
						Name:     to.Ptr("test4"),
						Type:     &testResourceType,
						Location: to.Ptr(v1.LocationGlobal),
					},
				},
				NextLink: to.Ptr("1"),
			},
		},
	}

	t.Run("ListEnvironments", func(t *testing.T) {
		mock := NewMockenvironmentResourceClient(gomock.NewController(t))
		client := createClient(mock)

		mock.EXPECT().
			NewListByScopePager(gomock.Any()).
			Return(pager(listPages))

		expectedResourceList := []corerp.EnvironmentResource{*listPages[0].Value[0], *listPages[0].Value[1], *listPages[1].Value[0], *listPages[1].Value[1]}

		resources, err := client.ListEnvironments(context.Background())
		require.NoError(t, err)
		require.Equal(t, expectedResourceList, resources)
	})

	t.Run("ListEnvironmentsAll", func(t *testing.T) {
		mock := NewMockenvironmentResourceClient(gomock.NewController(t))
		client := createClient(mock)

		mock.EXPECT().
			NewListByScopePager(gomock.Any()).
			Return(pager(listPages))

		expectedResourceList := []corerp.EnvironmentResource{*listPages[0].Value[0], *listPages[0].Value[1], *listPages[1].Value[0], *listPages[1].Value[1]}

		resources, err := client.ListEnvironmentsAll(context.Background())
		require.NoError(t, err)
		require.Equal(t, expectedResourceList, resources)
	})

	t.Run("GetEnvironment", func(t *testing.T) {
		mock := NewMockenvironmentResourceClient(gomock.NewController(t))
		client := createClient(mock)

		mock.EXPECT().
			Get(gomock.Any(), testResourceName, gomock.Any()).
			Return(corerp.EnvironmentsClientGetResponse{EnvironmentResource: expectedResource}, nil)

		environment, err := client.GetEnvironment(context.Background(), testResourceID)
		require.NoError(t, err)
		require.Equal(t, expectedResource, environment)
	})

	t.Run("GetRecipeMetadata", func(t *testing.T) {
		mock := NewMockenvironmentResourceClient(gomock.NewController(t))
		client := createClient(mock)

		expectedMetadata := corerp.RecipeGetMetadata{
			Name:         to.Ptr("test-recipe"),
			ResourceType: to.Ptr("Applications.Core/gateways"),
		}

		expectedResult := corerp.RecipeGetMetadataResponse{
			Parameters: map[string]any{
				"a": "a-value",
			},
		}

		mock.EXPECT().
			GetMetadata(gomock.Any(), testResourceName, expectedMetadata, gomock.Any()).
			Return(corerp.EnvironmentsClientGetMetadataResponse{
				RecipeGetMetadataResponse: corerp.RecipeGetMetadataResponse{
					Parameters: map[string]any{
						"a": "a-value",
					},
				},
			}, nil)

		result, err := client.GetRecipeMetadata(context.Background(), testResourceID, expectedMetadata)
		require.NoError(t, err)
		require.Equal(t, expectedResult, result)
	})

	t.Run("CreateOrUpdateEnviroment", func(t *testing.T) {
		mock := NewMockenvironmentResourceClient(gomock.NewController(t))
		client := createClient(mock)

		mock.EXPECT().
			CreateOrUpdate(gomock.Any(), testResourceName, expectedResource, gomock.Any()).
			Return(corerp.EnvironmentsClientCreateOrUpdateResponse{EnvironmentResource: expectedResource}, nil)

		err := client.CreateOrUpdateEnvironment(context.Background(), testResourceID, &expectedResource)
		require.NoError(t, err)
	})

	t.Run("DeleteEnvironment", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mock := NewMockenvironmentResourceClient(ctrl)
		applicationResourceMock := NewMockapplicationResourceClient(ctrl)
		genericResourceMock := NewMockgenericResourceClient(ctrl)
		client := createClient(mock)
		client.applicationResourceClientFactory = func(scope string) (applicationResourceClient, error) {
			return applicationResourceMock, nil
		}
		client.genericResourceClientFactory = func(scope string, resourceType string) (genericResourceClient, error) {
			return genericResourceMock, nil
		}

		resourceListPages := []generated.GenericResourcesClientListByRootScopeResponse{
			{
				GenericResourcesList: generated.GenericResourcesList{
					Value: []*generated.GenericResource{
						{
							ID:       to.Ptr(testScope + "/providers/Applications.Test/testResources/test1"),
							Name:     to.Ptr("test1"),
							Type:     to.Ptr("Applications.Test/testResources"),
							Location: to.Ptr(v1.LocationGlobal),
							Properties: map[string]any{
								"application": testScope + "/providers/Applications.Core/applications/test-application",
								"environment": testScope + "/providers/Applications.Core/environments/test-environment",
							},
						},
					},
					NextLink: to.Ptr("0"),
				},
			},
		}

		// Handle deletion of resources in the application.
		for i := range ResourceTypesList {
			if i == 0 {
				genericResourceMock.EXPECT().
					NewListByRootScopePager(gomock.Any()).
					Return(pager(resourceListPages))
			} else {
				genericResourceMock.EXPECT().
					NewListByRootScopePager(gomock.Any()).
					Return(pager([]generated.GenericResourcesClientListByRootScopeResponse{{GenericResourcesList: generated.GenericResourcesList{NextLink: to.Ptr("0")}}}))
			}
		}

		genericResourceMock.EXPECT().
			BeginDelete(gomock.Any(), "test1", gomock.Any()).
			Return(poller(&generated.GenericResourcesClientDeleteResponse{}), nil)

		// Setup deletion of applications in the environment.
		applicationListPages := []corerp.ApplicationsClientListByScopeResponse{
			{
				ApplicationResourceListResult: corerp.ApplicationResourceListResult{
					Value: []*corerp.ApplicationResource{
						{
							ID:       to.Ptr(testScope + "/providers/Applications.Core/applications/test-application"),
							Name:     to.Ptr("test-application"),
							Type:     to.Ptr("Applications.Core/applications"),
							Location: to.Ptr(v1.LocationGlobal),
							Properties: &corerp.ApplicationProperties{
								Environment: to.Ptr(testScope + "/providers/Applications.Core/environments/test-environment"),
							},
						},
					},
					NextLink: to.Ptr("0"),
				},
			},
		}
		applicationResourceMock.EXPECT().
			NewListByScopePager(gomock.Any()).
			Return(pager(applicationListPages))

		applicationResourceMock.EXPECT().
			Delete(gomock.Any(), "test-application", gomock.Any()).
			DoAndReturn(func(ctx context.Context, s string, acdo *corerp.ApplicationsClientDeleteOptions) (corerp.ApplicationsClientDeleteResponse, error) {
				setCapture(ctx, &http.Response{StatusCode: 200})
				return corerp.ApplicationsClientDeleteResponse{}, nil
			})

		mock.EXPECT().
			Delete(gomock.Any(), testResourceName, gomock.Any()).
			DoAndReturn(func(ctx context.Context, s string, acdo *corerp.EnvironmentsClientDeleteOptions) (corerp.EnvironmentsClientDeleteResponse, error) {
				setCapture(ctx, &http.Response{StatusCode: 200})
				return corerp.EnvironmentsClientDeleteResponse{}, nil
			})

		deleted, err := client.DeleteEnvironment(context.Background(), testResourceID)
		require.NoError(t, err)
		require.True(t, deleted)
	})
}

func Test_ResourceGroup(t *testing.T) {
	createClient := func(wrapped resourceGroupClient) *UCPApplicationsManagementClient {
		return &UCPApplicationsManagementClient{
			RootScope: testScope,
			resourceGroupClientFactory: func() (resourceGroupClient, error) {
				return wrapped, nil
			},
			capture: testCapture,
		}
	}

	testResourceName := "test-resource-group"

	expectedResource := ucp.ResourceGroupResource{
		ID:       to.Ptr("/planes/radius/local/resourcegroups/" + testResourceName),
		Name:     &testResourceName,
		Type:     to.Ptr("System.Resources/resourceGroups"),
		Location: to.Ptr(v1.LocationGlobal),
	}

	t.Run("ListResourceGroups", func(t *testing.T) {
		mock := NewMockresourceGroupClient(gomock.NewController(t))
		client := createClient(mock)

		resourceGroupPages := []ucp.ResourceGroupsClientListResponse{
			{
				ResourceGroupResourceListResult: ucp.ResourceGroupResourceListResult{
					Value: []*ucp.ResourceGroupResource{
						{
							ID:       to.Ptr("/planes/radius/local/resourcegroups/test1"),
							Name:     to.Ptr("test1"),
							Type:     to.Ptr("System.Resources/resourceGroups"),
							Location: to.Ptr(v1.LocationGlobal),
						},
						{
							ID:       to.Ptr("/planes/radius/local/resourcegroups/test2"),
							Name:     to.Ptr("test2"),
							Type:     to.Ptr("System.Resources/resourceGroups"),
							Location: to.Ptr(v1.LocationGlobal),
						},
					},
					NextLink: to.Ptr("0"),
				},
			},
			{
				ResourceGroupResourceListResult: ucp.ResourceGroupResourceListResult{
					Value: []*ucp.ResourceGroupResource{
						{
							ID:       to.Ptr("/planes/radius/local/resourcegroups/test3"),
							Name:     to.Ptr("test3"),
							Type:     to.Ptr("System.Resources/resourceGroups"),
							Location: to.Ptr(v1.LocationGlobal),
						},
						{
							ID:       to.Ptr("/planes/radius/local/resourcegroups/test4"),
							Name:     to.Ptr("test4"),
							Type:     to.Ptr("System.Resources/resourceGroups"),
							Location: to.Ptr(v1.LocationGlobal),
						},
					},
					NextLink: to.Ptr("1"),
				},
			},
		}

		mock.EXPECT().
			NewListPager(gomock.Any(), gomock.Any()).
			Return(pager(resourceGroupPages))

		expected := []ucp.ResourceGroupResource{*resourceGroupPages[0].Value[0], *resourceGroupPages[0].Value[1], *resourceGroupPages[1].Value[0], *resourceGroupPages[1].Value[1]}

		groups, err := client.ListResourceGroups(context.Background(), "local")
		require.NoError(t, err)
		require.Equal(t, expected, groups)
	})

	t.Run("GetResourceGroup", func(t *testing.T) {
		mock := NewMockresourceGroupClient(gomock.NewController(t))
		client := createClient(mock)

		mock.EXPECT().
			Get(gomock.Any(), "local", testResourceName, gomock.Any()).
			Return(ucp.ResourceGroupsClientGetResponse{ResourceGroupResource: expectedResource}, nil)

		group, err := client.GetResourceGroup(context.Background(), "local", testResourceName)
		require.NoError(t, err)
		require.Equal(t, expectedResource, group)
	})

	t.Run("CreateOrUpdateResourceGroup", func(t *testing.T) {
		mock := NewMockresourceGroupClient(gomock.NewController(t))
		client := createClient(mock)

		mock.EXPECT().
			CreateOrUpdate(gomock.Any(), "local", testResourceName, expectedResource, gomock.Any()).
			Return(ucp.ResourceGroupsClientCreateOrUpdateResponse{}, nil)

		err := client.CreateOrUpdateResourceGroup(context.Background(), "local", testResourceName, &expectedResource)
		require.NoError(t, err)
	})

	t.Run("DeleteResourceGroup", func(t *testing.T) {
		mock := NewMockresourceGroupClient(gomock.NewController(t))
		client := createClient(mock)

		mock.EXPECT().
			Delete(gomock.Any(), "local", testResourceName, gomock.Any()).
			DoAndReturn(func(ctx context.Context, s1, s2 string, rgcdo *ucp.ResourceGroupsClientDeleteOptions) (ucp.ResourceGroupsClientDeleteResponse, error) {
				setCapture(ctx, &http.Response{StatusCode: 200})
				return ucp.ResourceGroupsClientDeleteResponse{}, nil
			})

		deleted, err := client.DeleteResourceGroup(context.Background(), "local", testResourceName)
		require.NoError(t, err)
		require.True(t, deleted)
	})
}

func Test_extractScopeAndName(t *testing.T) {
	client := UCPApplicationsManagementClient{
		RootScope: testScope,
	}

	t.Run("valid resource id", func(t *testing.T) {
		// Different from test scope
		scope, name, err := client.extractScopeAndName("/planes/radius/local/resourceGroups/my-rg/providers/Applications.Core/environments/my-env")
		require.NoError(t, err)
		require.Equal(t, "/planes/radius/local/resourceGroups/my-rg", scope)
		require.Equal(t, "my-env", name)
	})

	t.Run("valid name", func(t *testing.T) {
		scope, name, err := client.extractScopeAndName("my-env")
		require.NoError(t, err)
		require.Equal(t, testScope, scope)
		require.Equal(t, "my-env", name)
	})

	t.Run("invalid resource id", func(t *testing.T) {
		// Missing `/planes` makes it invalid.
		scope, name, err := client.extractScopeAndName("/local/resourceGroups/my-rg/providers/Applications.Core/environments/my-env")
		require.Error(t, err)
		require.Equal(t, "'/local/resourceGroups/my-rg/providers/Applications.Core/environments/my-env' is not a valid resource id", err.Error())
		require.Empty(t, scope)
		require.Empty(t, name)
	})
}

func Test_fullyQualifyID(t *testing.T) {
	client := UCPApplicationsManagementClient{
		RootScope: testScope,
	}

	t.Run("valid resource id", func(t *testing.T) {
		// Different from test scope
		id, err := client.fullyQualifyID("/planes/radius/local/resourceGroups/my-rg/providers/Applications.Core/environments/my-env", "Applications.Core/environments")
		require.NoError(t, err)
		require.Equal(t, "/planes/radius/local/resourceGroups/my-rg/providers/Applications.Core/environments/my-env", id)
	})

	t.Run("valid name", func(t *testing.T) {
		id, err := client.fullyQualifyID("my-env", "Applications.Core/environments")
		require.NoError(t, err)
		require.Equal(t, "/planes/radius/local/resourceGroups/my-default-rg/providers/Applications.Core/environments/my-env", id)
	})

	t.Run("invalid resource id", func(t *testing.T) {
		// Missing `/planes` makes it invalid.
		id, err := client.fullyQualifyID("/local/resourceGroups/my-rg/providers/Applications.Core/environments/my-env", "Applications.Core/environments")
		require.Error(t, err)
		require.Equal(t, "'/local/resourceGroups/my-rg/providers/Applications.Core/environments/my-env' is not a valid resource id", err.Error())
		require.Empty(t, id)
	})
}

func pager[S ~[]E, E any](pages S) *runtime.Pager[E] {
	// Generated autorest types don't implement comparable, so we use
	// the next link to encode the index of each page.
	if len(pages) == 0 {
		panic("At least one page is required (it can be empty)")
	}

	find := func(page E) int {
		v := reflect.ValueOf(page)
		next := v.FieldByName("NextLink").Elem()
		index, err := strconv.ParseInt(next.String(), 10, 64)
		if err != nil {
			panic(err)
		}

		return int(index)
	}

	handler := runtime.PagingHandler[E]{
		More: func(page E) bool {
			index := find(page)
			return index < len(pages)-1
		},
		Fetcher: func(ctx context.Context, page *E) (E, error) {
			if page == nil {
				return pages[0], nil
			}

			index := find(*page)
			return pages[index+1], nil
		},
	}

	return runtime.NewPager[E](handler)
}

func poller[T any](response *T) *runtime.Poller[T] {

	p, err := runtime.NewPoller[T](nil, runtime.Pipeline{}, &runtime.NewPollerOptions[T]{
		Response: response,
		Handler:  &pollingHandler[T]{Response: response},
	})
	if err != nil {
		panic(err)
	}

	return p
}

type pollingHandler[T any] struct {
	Response *T
}

func (ph *pollingHandler[T]) Done() bool {
	return true
}

func (ph *pollingHandler[T]) Poll(context.Context) (*http.Response, error) {
	panic("should not be called!")
}

func (ph *pollingHandler[T]) Result(ctx context.Context, out *T) error {
	*out = *ph.Response

	setCapture(ctx, &http.Response{StatusCode: 200})

	return nil
}

type holder struct {
	capture **http.Response
}

func setCapture(ctx context.Context, response *http.Response) {
	holder := ctx.Value(holder{}).(*holder)
	if holder != nil {
		*holder.capture = response
	}
}

func testCapture(ctx context.Context, capture **http.Response) context.Context {
	return context.WithValue(ctx, holder{}, &holder{capture})
}
