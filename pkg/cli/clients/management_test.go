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

var (
	resourceProviderSummaryPages = []ucp.ResourceProvidersClientListProviderSummariesResponse{
		{
			PagedResourceProviderSummary: ucp.PagedResourceProviderSummary{
				Value: []*ucp.ResourceProviderSummary{
					{
						Name: to.Ptr("Applications.Test1"),
						ResourceTypes: map[string]*ucp.ResourceProviderSummaryResourceType{
							"resourceType1": {
								APIVersions: map[string]map[string]any{
									"2025-01-01": {},
								},
								DefaultAPIVersion: to.Ptr("2025-01-01"),
							},
						},
						Locations: map[string]map[string]any{
							"east": {},
						},
					},
					{
						Name: to.Ptr("Applications.Test2"),
						ResourceTypes: map[string]*ucp.ResourceProviderSummaryResourceType{
							"resourceType2": {
								APIVersions: map[string]map[string]any{
									"2025-01-01": {},
								},
								DefaultAPIVersion: to.Ptr("2025-01-01"),
							},
						},
						Locations: map[string]map[string]any{
							"east": {},
						},
					},
				},
				NextLink: to.Ptr("0"),
			},
		},
		{
			PagedResourceProviderSummary: ucp.PagedResourceProviderSummary{
				Value: []*ucp.ResourceProviderSummary{
					{
						Name: to.Ptr("Applications.Test3"),
						ResourceTypes: map[string]*ucp.ResourceProviderSummaryResourceType{
							"resourceType3": {
								APIVersions: map[string]map[string]any{
									"2025-01-01": {},
								},
								DefaultAPIVersion: to.Ptr("2025-01-01"),
							},
						},
						Locations: map[string]map[string]any{
							"east": {},
						},
					},
				},
				NextLink: to.Ptr("1"),
			},
		},
	}
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

	createResourceAndRPClient := func(wrapped genericResourceClient, wrappedRP resourceProviderClient) *UCPApplicationsManagementClient {
		return &UCPApplicationsManagementClient{
			RootScope: testScope,
			genericResourceClientFactory: func(scope string, resourceType string) (genericResourceClient, error) {
				return wrapped, nil
			},
			resourceProviderClientFactory: func() (resourceProviderClient, error) {
				return wrappedRP, nil
			},
			capture: testCapture,
		}
	}

	createResourceProviderClient := func(wrapped resourceProviderClient) *UCPApplicationsManagementClient {
		return &UCPApplicationsManagementClient{
			RootScope: testScope,
			resourceProviderClientFactory: func() (resourceProviderClient, error) {
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

	t.Run("ListAllResourceTypesNames", func(t *testing.T) {
		mockResourceProviderClient := NewMockresourceProviderClient(gomock.NewController(t))

		mockResourceProviderClient.EXPECT().NewListProviderSummariesPager("local", gomock.Any()).Return(pager(resourceProviderSummaryPages)).AnyTimes()
		client := createResourceProviderClient(mockResourceProviderClient)

		resourceTypes, err := client.ListAllResourceTypesNames(context.Background(), "local")
		require.NoError(t, err)
		require.Equal(t, []string{"Applications.Test1/resourceType1", "Applications.Test2/resourceType2", "Applications.Test3/resourceType3"}, resourceTypes)
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
		mockResourceClient := NewMockgenericResourceClient(gomock.NewController(t))
		mockResourceProviderClient := NewMockresourceProviderClient(gomock.NewController(t))

		client := createResourceAndRPClient(mockResourceClient, mockResourceProviderClient)
		mockResourceProviderClient.EXPECT().NewListProviderSummariesPager("local", gomock.Any()).Return(pager(resourceProviderSummaryPages))
		mockResourceClient.EXPECT().
			NewListByRootScopePager(gomock.Any()).
			Return(pager(listPages)).AnyTimes()

		expectedResourceList := []generated.GenericResource{*listPages[0].Value[0]}

		resources, err := client.ListResourcesInApplication(context.Background(), "test-application")
		require.NoError(t, err)
		require.Equal(t, expectedResourceList, resources)
	})

	t.Run("ListResourcesInEnvironment", func(t *testing.T) {
		mockResourceClient := NewMockgenericResourceClient(gomock.NewController(t))
		mockResourceProviderClient := NewMockresourceProviderClient(gomock.NewController(t))

		client := createResourceAndRPClient(mockResourceClient, mockResourceProviderClient)

		mockResourceProviderClient.EXPECT().NewListProviderSummariesPager("local", gomock.Any()).Return(pager(resourceProviderSummaryPages))
		mockResourceClient.EXPECT().
			NewListByRootScopePager(gomock.Any()).
			Return(pager(listPages)).AnyTimes()

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

	t.Run("CreateOrUpdateResource", func(t *testing.T) {
		mock := NewMockgenericResourceClient(gomock.NewController(t))
		client := createClient(mock)

		mock.EXPECT().
			BeginCreateOrUpdate(gomock.Any(), testResourceName, expectedResource, gomock.Any()).
			Return(poller(&generated.GenericResourcesClientCreateOrUpdateResponse{GenericResource: expectedResource}), nil)

		response, err := client.CreateOrUpdateResource(context.Background(), testResourceType, testResourceID, &expectedResource)
		require.NoError(t, err)
		require.Equal(t, expectedResource, response)
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
		mockResourceProviderClient := NewMockresourceProviderClient(ctrl)
		genericResourceMock := NewMockgenericResourceClient(ctrl)
		client := createClient(mock)
		client.genericResourceClientFactory = func(scope string, resourceType string) (genericResourceClient, error) {
			return genericResourceMock, nil
		}
		client.resourceProviderClientFactory = func() (resourceProviderClient, error) {
			return mockResourceProviderClient, nil
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

		mockResourceProviderClient.EXPECT().NewListProviderSummariesPager("local", gomock.Any()).Return(pager(resourceProviderSummaryPages))
		genericResourceMock.EXPECT().
			NewListByRootScopePager(gomock.Any()).
			Return(pager(resourceListPages)).AnyTimes()

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
		resourceProviderMock := NewMockresourceProviderClient(ctrl)
		client := createClient(mock)
		client.applicationResourceClientFactory = func(scope string) (applicationResourceClient, error) {
			return applicationResourceMock, nil
		}
		client.resourceProviderClientFactory = func() (resourceProviderClient, error) {
			return resourceProviderMock, nil
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
		genericResourceMock.EXPECT().
			NewListByRootScopePager(gomock.Any()).
			Return(pager(resourceListPages)).AnyTimes()

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
		resourceProviderMock.EXPECT().
			NewListProviderSummariesPager("local", gomock.Any()).
			Return(pager(resourceProviderSummaryPages))

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

func Test_ResourceProvider(t *testing.T) {
	createClient := func(wrapped resourceProviderClient) *UCPApplicationsManagementClient {
		return &UCPApplicationsManagementClient{
			RootScope: testScope,
			resourceProviderClientFactory: func() (resourceProviderClient, error) {
				return wrapped, nil
			},
			capture: testCapture,
		}
	}

	testResourceProviderName := "Applications.Test"

	expectedResource := ucp.ResourceProviderResource{
		ID:       to.Ptr("/planes/radius/local/providers/System.Resources/resourceProviders/" + testResourceProviderName),
		Name:     &testResourceProviderName,
		Type:     to.Ptr("System.Resources/resourceProviders"),
		Location: to.Ptr(v1.LocationGlobal),
	}

	t.Run("ListResourceProviders", func(t *testing.T) {
		mock := NewMockresourceProviderClient(gomock.NewController(t))
		client := createClient(mock)

		resourceProviderPages := []ucp.ResourceProvidersClientListResponse{
			{
				ResourceProviderResourceListResult: ucp.ResourceProviderResourceListResult{
					Value: []*ucp.ResourceProviderResource{
						{
							ID:       to.Ptr("/planes/radius/local/providers/System.Resources/resourceProviders/Applications.Test1"),
							Name:     to.Ptr("Applications.Test1"),
							Type:     to.Ptr("System.Resources/resourceProviders"),
							Location: to.Ptr(v1.LocationGlobal),
						},
						{
							ID:       to.Ptr("/planes/radius/local/providers/System.Resources/resourceProviders/Applications.Test2"),
							Name:     to.Ptr("Applications.Test2"),
							Type:     to.Ptr("System.Resources/resourceProviders"),
							Location: to.Ptr(v1.LocationGlobal),
						},
					},
					NextLink: to.Ptr("0"),
				},
			},
			{
				ResourceProviderResourceListResult: ucp.ResourceProviderResourceListResult{
					Value: []*ucp.ResourceProviderResource{
						{
							ID:       to.Ptr("/planes/radius/local/providers/System.Resources/resourceProviders/Applications.Test3"),
							Name:     to.Ptr("Applications.Test3"),
							Type:     to.Ptr("System.Resources/resourceProviders"),
							Location: to.Ptr(v1.LocationGlobal),
						},
						{
							ID:       to.Ptr("/planes/radius/local/providers/System.Resources/resourceProviders/Applications.Test4"),
							Name:     to.Ptr("Applications.Test4"),
							Type:     to.Ptr("System.Resources/resourceProviders"),
							Location: to.Ptr(v1.LocationGlobal),
						},
					},
					NextLink: to.Ptr("1"),
				},
			},
		}

		mock.EXPECT().
			NewListPager(gomock.Any(), gomock.Any()).
			Return(pager(resourceProviderPages))

		expected := []ucp.ResourceProviderResource{*resourceProviderPages[0].Value[0], *resourceProviderPages[0].Value[1], *resourceProviderPages[1].Value[0], *resourceProviderPages[1].Value[1]}

		groups, err := client.ListResourceProviders(context.Background(), "local")
		require.NoError(t, err)
		require.Equal(t, expected, groups)
	})

	t.Run("GetResourceProvider", func(t *testing.T) {
		mock := NewMockresourceProviderClient(gomock.NewController(t))
		client := createClient(mock)

		mock.EXPECT().
			Get(gomock.Any(), "local", testResourceProviderName, gomock.Any()).
			Return(ucp.ResourceProvidersClientGetResponse{ResourceProviderResource: expectedResource}, nil)

		group, err := client.GetResourceProvider(context.Background(), "local", testResourceProviderName)
		require.NoError(t, err)
		require.Equal(t, expectedResource, group)
	})

	t.Run("CreateOrUpdateResourceProvider", func(t *testing.T) {
		mock := NewMockresourceProviderClient(gomock.NewController(t))
		client := createClient(mock)

		mock.EXPECT().
			BeginCreateOrUpdate(gomock.Any(), "local", testResourceProviderName, expectedResource, gomock.Any()).
			Return(poller(&ucp.ResourceProvidersClientCreateOrUpdateResponse{ResourceProviderResource: expectedResource}), nil)

		result, err := client.CreateOrUpdateResourceProvider(context.Background(), "local", testResourceProviderName, &expectedResource)
		require.NoError(t, err)
		require.Equal(t, result, expectedResource)
	})

	t.Run("DeleteResourceProvider", func(t *testing.T) {
		mock := NewMockresourceProviderClient(gomock.NewController(t))
		client := createClient(mock)

		mock.EXPECT().
			BeginDelete(gomock.Any(), "local", testResourceProviderName, gomock.Any()).
			DoAndReturn(func(ctx context.Context, s1, s2 string, rgcdo *ucp.ResourceProvidersClientBeginDeleteOptions) (*runtime.Poller[ucp.ResourceProvidersClientDeleteResponse], error) {
				setCapture(ctx, &http.Response{StatusCode: 200})
				return poller(&ucp.ResourceProvidersClientDeleteResponse{}), nil
			})

		deleted, err := client.DeleteResourceProvider(context.Background(), "local", testResourceProviderName)
		require.NoError(t, err)
		require.True(t, deleted)
	})

	t.Run("ListResourceProviderSummaries", func(t *testing.T) {
		mock := NewMockresourceProviderClient(gomock.NewController(t))
		client := createClient(mock)

		mock.EXPECT().
			NewListProviderSummariesPager(gomock.Any(), gomock.Any()).
			Return(pager(resourceProviderSummaryPages))
		expected := []ucp.ResourceProviderSummary{*resourceProviderSummaryPages[0].Value[0], *resourceProviderSummaryPages[0].Value[1], *resourceProviderSummaryPages[1].Value[0]}

		resourceProviderSummaries, err := client.ListResourceProviderSummaries(context.Background(), "local")
		require.NoError(t, err)
		require.Equal(t, expected, resourceProviderSummaries)
	})

	t.Run("GetResourceProviderSummary", func(t *testing.T) {
		mock := NewMockresourceProviderClient(gomock.NewController(t))
		client := createClient(mock)

		expectedResource := ucp.ResourceProviderSummary{
			Name: to.Ptr("Applications.Test1"),
			ResourceTypes: map[string]*ucp.ResourceProviderSummaryResourceType{
				"resourceType1": {
					APIVersions: map[string]map[string]any{
						"2025-01-01": {},
					},
					DefaultAPIVersion: to.Ptr("2025-01-01"),
				},
			},
			Locations: map[string]map[string]any{
				"east": {},
			},
		}

		mock.EXPECT().
			GetProviderSummary(gomock.Any(), "local", testResourceProviderName, gomock.Any()).
			Return(ucp.ResourceProvidersClientGetProviderSummaryResponse{ResourceProviderSummary: expectedResource}, nil)

		summary, err := client.GetResourceProviderSummary(context.Background(), "local", testResourceProviderName)
		require.NoError(t, err)
		require.Equal(t, expectedResource, summary)
	})
}

func Test_ResourceType(t *testing.T) {
	createClient := func(wrapped resourceTypeClient) *UCPApplicationsManagementClient {
		return &UCPApplicationsManagementClient{
			RootScope: testScope,
			resourceTypeClientFactory: func() (resourceTypeClient, error) {
				return wrapped, nil
			},
			capture: testCapture,
		}
	}

	testResourceProviderName := "Applications.Test"
	testResourceTypeName := "testResources"

	expectedResource := ucp.ResourceTypeResource{
		ID:   to.Ptr("/planes/radius/local/providers/System.Resources/resourceProviders/" + testResourceProviderName + "/resourceTypes/" + testResourceTypeName),
		Name: &testResourceTypeName,
		Type: to.Ptr("System.Resources/resourceProviders/resourceTypes"),
	}

	t.Run("CreateOrUpdateResourceType", func(t *testing.T) {
		mock := NewMockresourceTypeClient(gomock.NewController(t))
		client := createClient(mock)

		mock.EXPECT().
			BeginCreateOrUpdate(gomock.Any(), "local", testResourceProviderName, testResourceTypeName, expectedResource, gomock.Any()).
			Return(poller(&ucp.ResourceTypesClientCreateOrUpdateResponse{ResourceTypeResource: expectedResource}), nil)

		result, err := client.CreateOrUpdateResourceType(context.Background(), "local", testResourceProviderName, testResourceTypeName, &expectedResource)
		require.NoError(t, err)
		require.Equal(t, expectedResource, result)
	})

	t.Run("DeleteResourceType", func(t *testing.T) {
		mock := NewMockresourceTypeClient(gomock.NewController(t))
		client := createClient(mock)

		mock.EXPECT().
			BeginDelete(gomock.Any(), "local", testResourceProviderName, testResourceTypeName, gomock.Any()).
			DoAndReturn(func(ctx context.Context, s1, s2, s3 string, options *ucp.ResourceTypesClientBeginDeleteOptions) (*runtime.Poller[ucp.ResourceTypesClientDeleteResponse], error) {
				setCapture(ctx, &http.Response{StatusCode: 200})
				return poller(&ucp.ResourceTypesClientDeleteResponse{}), nil
			})

		deleted, err := client.DeleteResourceType(context.Background(), "local", testResourceProviderName, testResourceTypeName)
		require.NoError(t, err)
		require.True(t, deleted)
	})
}

func Test_APIVersion(t *testing.T) {
	createClient := func(wrapped apiVersionClient) *UCPApplicationsManagementClient {
		return &UCPApplicationsManagementClient{
			RootScope: testScope,
			apiVersionClientFactory: func() (apiVersionClient, error) {
				return wrapped, nil
			},
			capture: testCapture,
		}
	}

	testResourceProviderName := "Applications.Test"
	testResourceTypeName := "testResources"
	testAPIVersionResourceName := "2025-01-01"

	expectedResource := ucp.APIVersionResource{
		ID:   to.Ptr("/planes/radius/local/providers/System.Resources/resourceProviders/" + testResourceProviderName + "/resourceTypes/" + testResourceTypeName + "/apiVersions/" + testAPIVersionResourceName),
		Name: &testAPIVersionResourceName,
		Type: to.Ptr("System.Resources/resourceProviders/resourceTypes/apiVersions"),
	}

	t.Run("CreateOrUpdateAPIVersion", func(t *testing.T) {
		mock := NewMockapiVersionClient(gomock.NewController(t))
		client := createClient(mock)

		mock.EXPECT().
			BeginCreateOrUpdate(gomock.Any(), "local", testResourceProviderName, testResourceTypeName, testAPIVersionResourceName, expectedResource, gomock.Any()).
			Return(poller(&ucp.APIVersionsClientCreateOrUpdateResponse{APIVersionResource: expectedResource}), nil)

		result, err := client.CreateOrUpdateAPIVersion(context.Background(), "local", testResourceProviderName, testResourceTypeName, testAPIVersionResourceName, &expectedResource)
		require.NoError(t, err)
		require.Equal(t, expectedResource, result)
	})
}

func Test_Location(t *testing.T) {
	createClient := func(wrapped locationClient) *UCPApplicationsManagementClient {
		return &UCPApplicationsManagementClient{
			RootScope: testScope,
			locationClientFactory: func() (locationClient, error) {
				return wrapped, nil
			},
			capture: testCapture,
		}
	}

	testResourceProviderName := "Applications.Test"
	testLocationName := "east"

	expectedResource := ucp.LocationResource{
		ID:   to.Ptr("/planes/radius/local/providers/System.Resources/resourceProviders/" + testResourceProviderName + "/locations/" + testLocationName),
		Name: &testLocationName,
		Type: to.Ptr("System.Resources/resourceProviders/locations"),
	}

	t.Run("CreateOrUpdateLocation", func(t *testing.T) {
		mock := NewMocklocationClient(gomock.NewController(t))
		client := createClient(mock)

		mock.EXPECT().
			BeginCreateOrUpdate(gomock.Any(), "local", testResourceProviderName, testLocationName, expectedResource, gomock.Any()).
			Return(poller(&ucp.LocationsClientCreateOrUpdateResponse{LocationResource: expectedResource}), nil)

		result, err := client.CreateOrUpdateLocation(context.Background(), "local", testResourceProviderName, testLocationName, &expectedResource)
		require.NoError(t, err)
		require.Equal(t, expectedResource, result)
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
	obj := ctx.Value(holder{})
	if obj != nil {
		holder := obj.(*holder)
		*holder.capture = response
	}
}

func testCapture(ctx context.Context, capture **http.Response) context.Context {
	return context.WithValue(ctx, holder{}, &holder{capture})
}
