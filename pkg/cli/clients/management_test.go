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
	"fmt"
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
	version      = "2025-01-01"
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
								APIVersions: map[string]*ucp.ResourceTypeSummaryResultAPIVersion{
									version: {},
								},
								DefaultAPIVersion: to.Ptr(version),
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
								APIVersions: map[string]*ucp.ResourceTypeSummaryResultAPIVersion{
									version: {},
								},
								DefaultAPIVersion: to.Ptr(version),
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
								APIVersions: map[string]*ucp.ResourceTypeSummaryResultAPIVersion{
									version: {},
								},
								DefaultAPIVersion: to.Ptr(version),
							},
						},
						Locations: map[string]map[string]any{
							"east": {},
						},
					},
					{
						Name: to.Ptr("Applications.Core"),
						ResourceTypes: map[string]*ucp.ResourceProviderSummaryResourceType{
							"environments": {
								APIVersions: map[string]*ucp.ResourceTypeSummaryResultAPIVersion{
									version: {},
								},
								DefaultAPIVersion: to.Ptr(version),
							},
						},
						Locations: map[string]map[string]any{
							"east": {},
						},
					},
					{
						Name: to.Ptr("Radius.Core"),
						ResourceTypes: map[string]*ucp.ResourceProviderSummaryResourceType{
							"environments": {
								APIVersions: map[string]*ucp.ResourceTypeSummaryResultAPIVersion{
									"2025-08-01-preview": {},
								},
								DefaultAPIVersion: to.Ptr("2025-08-01-preview"),
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

// Test helper functions to reduce repetition

// mockResourceGroupExists sets up expectation for resource group existence check
func mockResourceGroupExists(mock *MockresourceGroupClient, planeName, rgName string, times int) {
	mock.EXPECT().
		Get(gomock.Any(), planeName, rgName, gomock.Any()).
		Return(ucp.ResourceGroupsClientGetResponse{
			ResourceGroupResource: ucp.ResourceGroupResource{
				Name: to.Ptr(rgName),
			},
		}, nil).Times(times)
}

// mockListProviders sets up standard provider listing expectation
func mockListProviders(mock *MockresourceProviderClient, planeName string) {
	mock.EXPECT().
		NewListProviderSummariesPager(planeName, gomock.Any()).
		Return(pager(resourceProviderSummaryPages))
}

// mockProviderSummaries sets up provider summary expectations with findProviderSummary logic
func mockProviderSummaries(mock *MockresourceProviderClient, planeName string, times int) {
	mock.EXPECT().
		GetProviderSummary(gomock.Any(), planeName, gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, plane string, providerName string, opts *ucp.ResourceProvidersClientGetProviderSummaryOptions) (ucp.ResourceProvidersClientGetProviderSummaryResponse, error) {
			summary := findProviderSummary(providerName)
			if summary != nil {
				return ucp.ResourceProvidersClientGetProviderSummaryResponse{
					ResourceProviderSummary: *summary,
				}, nil
			}
			return ucp.ResourceProvidersClientGetProviderSummaryResponse{}, nil
		}).Times(times)
}

// mockProviderSummaryForDeletion mocks API version lookup for a specific provider during deletion
func mockProviderSummaryForDeletion(mock *MockresourceProviderClient, planeName, providerName string) {
	summary := findProviderSummary(providerName)
	if summary != nil {
		mock.EXPECT().
			GetProviderSummary(gomock.Any(), planeName, providerName, gomock.Any()).
			Return(ucp.ResourceProvidersClientGetProviderSummaryResponse{
				ResourceProviderSummary: *summary,
			}, nil).Times(1)
	} else {
		// If no test data found, create a minimal provider summary for Applications.Core
		if providerName == "Applications.Core" {
			mock.EXPECT().
				GetProviderSummary(gomock.Any(), planeName, providerName, gomock.Any()).
				Return(ucp.ResourceProvidersClientGetProviderSummaryResponse{
					ResourceProviderSummary: ucp.ResourceProviderSummary{
						Name: to.Ptr("Applications.Core"),
						ResourceTypes: map[string]*ucp.ResourceProviderSummaryResourceType{
							"environments": {
								APIVersions: map[string]*ucp.ResourceTypeSummaryResultAPIVersion{
									version: {},
								},
							},
						},
					},
				}, nil).Times(1)
		}
	}
}

// mockResourceDeletion mocks successful resource deletion
func mockResourceDeletion(mock *MockgenericResourceClient, resourceName string) {
	mock.EXPECT().
		BeginDelete(gomock.Any(), resourceName, gomock.Any()).
		Return(poller(&generated.GenericResourcesClientDeleteResponse{}), nil)
}

// mockResourceDeletionFailure mocks failed resource deletion
func mockResourceDeletionFailure(mock *MockgenericResourceClient, resourceName string, errorMsg string) {
	mock.EXPECT().
		BeginDelete(gomock.Any(), resourceName, gomock.Any()).
		Return(nil, fmt.Errorf("%s", errorMsg))
}

// mockResourceGroupDeletion mocks successful resource group deletion
func mockResourceGroupDeletion(mock *MockresourceGroupClient, planeName, rgName string) {
	mock.EXPECT().
		Delete(gomock.Any(), planeName, rgName, gomock.Any()).
		DoAndReturn(func(ctx context.Context, s1, s2 string, opts *ucp.ResourceGroupsClientDeleteOptions) (ucp.ResourceGroupsClientDeleteResponse, error) {
			setCapture(ctx, &http.Response{StatusCode: 200})
			return ucp.ResourceGroupsClientDeleteResponse{}, nil
		})
}

// setupResourceGroupMocks creates a client and all necessary mocks for resource group operations
func setupResourceGroupMocks(t *testing.T) (*UCPApplicationsManagementClient, *MockresourceGroupClient, *MockgenericResourceClient, *MockresourceProviderClient) {
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	rgClient := NewMockresourceGroupClient(ctrl)
	genericClient := NewMockgenericResourceClient(ctrl)
	rpClient := NewMockresourceProviderClient(ctrl)

	client := &UCPApplicationsManagementClient{
		RootScope: testScope,
		resourceGroupClientFactory: func() (resourceGroupClient, error) {
			return rgClient, nil
		},
		genericResourceClientFactory: func(scope string, resourceType string) (genericResourceClient, error) {
			return genericClient, nil
		},
		resourceProviderClientFactory: func() (resourceProviderClient, error) {
			return rpClient, nil
		},
		capture: testCapture,
	}

	return client, rgClient, genericClient, rpClient
}

// createResource creates a test resource with the given name and type
func createResource(name, resourceType string) *generated.GenericResource {
	id := fmt.Sprintf("/planes/radius/local/resourceGroups/test-rg/providers/%s/%s", resourceType, name)
	return &generated.GenericResource{
		ID:       to.Ptr(id),
		Type:     to.Ptr(resourceType),
		Name:     to.Ptr(name),
		Location: to.Ptr(v1.LocationGlobal),
	}
}

// createResourceList wraps resources in the response format expected by the pager
func createResourceList(resources ...*generated.GenericResource) []generated.GenericResourcesClientListByRootScopeResponse {
	return []generated.GenericResourcesClientListByRootScopeResponse{
		{
			GenericResourcesList: generated.GenericResourcesList{
				Value:    resources,
				NextLink: to.Ptr("0"),
			},
		},
	}
}

// withProperties adds properties to a resource
func withProperties(resource *generated.GenericResource, props map[string]any) *generated.GenericResource {
	resource.Properties = props
	return resource
}

func Test_Resource(t *testing.T) {
	t.Parallel()
	createClient := func(wrapped genericResourceClient) *UCPApplicationsManagementClient {
		return &UCPApplicationsManagementClient{
			RootScope: testScope,
			genericResourceClientFactory: func(scope string, resourceType string) (genericResourceClient, error) {
				return wrapped, nil
			},
			capture: testCapture,
		}
	}

	createResourceAndResourceProviderClient := func(wrapped genericResourceClient, wrappedRP resourceProviderClient) *UCPApplicationsManagementClient {
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
		ctrl := gomock.NewController(t)
		mock := NewMockgenericResourceClient(ctrl)
		resourceProviderMock := NewMockresourceProviderClient(ctrl)
		client := createClient(mock)
		client.resourceProviderClientFactory = func() (resourceProviderClient, error) {
			return resourceProviderMock, nil
		}
		expectedResource := ucp.ResourceProviderSummary{
			Name: to.Ptr("Applications.Test"),
			ResourceTypes: map[string]*ucp.ResourceProviderSummaryResourceType{
				"testResource": {
					APIVersions: map[string]*ucp.ResourceTypeSummaryResultAPIVersion{
						version: {},
					},
				},
			},
			Locations: map[string]map[string]any{
				"east": {},
			},
		}

		resourceProviderMock.EXPECT().
			GetProviderSummary(gomock.Any(), "local", "Applications.Test", gomock.Any()).
			Return(ucp.ResourceProvidersClientGetProviderSummaryResponse{ResourceProviderSummary: expectedResource}, nil)

		mock.EXPECT().
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
		require.Equal(t, []string{
			"Applications.Test1/resourceType1",
			"Applications.Test2/resourceType2",
			"Applications.Test3/resourceType3",
			"Applications.Core/environments",
		}, resourceTypes)
	})

	t.Run("ListResourcesOfTypeInApplication", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mock := NewMockgenericResourceClient(ctrl)
		resourceProviderMock := NewMockresourceProviderClient(ctrl)
		client := createClient(mock)
		client.resourceProviderClientFactory = func() (resourceProviderClient, error) {
			return resourceProviderMock, nil
		}
		expectedResource := ucp.ResourceProviderSummary{
			Name: to.Ptr("Applications.Test"),
			ResourceTypes: map[string]*ucp.ResourceProviderSummaryResourceType{
				"testResource": {
					APIVersions: map[string]*ucp.ResourceTypeSummaryResultAPIVersion{
						version: {},
					},
				},
			},
			Locations: map[string]map[string]any{
				"east": {},
			},
		}

		resourceProviderMock.EXPECT().
			GetProviderSummary(gomock.Any(), "local", "Applications.Test", gomock.Any()).
			Return(ucp.ResourceProvidersClientGetProviderSummaryResponse{ResourceProviderSummary: expectedResource}, nil)

		mock.EXPECT().
			NewListByRootScopePager(gomock.Any()).
			Return(pager(listPages))

		expectedResourceList := []generated.GenericResource{*listPages[0].Value[0]}

		resources, err := client.ListResourcesOfTypeInApplication(context.Background(), "test-application", testResourceType)
		require.NoError(t, err)
		require.Equal(t, expectedResourceList, resources)
	})

	t.Run("ListResourcesOfTypeInEnvironment", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mock := NewMockgenericResourceClient(ctrl)
		resourceProviderMock := NewMockresourceProviderClient(ctrl)
		client := createClient(mock)
		client.resourceProviderClientFactory = func() (resourceProviderClient, error) {
			return resourceProviderMock, nil
		}
		expectedResource := ucp.ResourceProviderSummary{
			Name: to.Ptr("Applications.Test"),
			ResourceTypes: map[string]*ucp.ResourceProviderSummaryResourceType{
				"testResource": {
					APIVersions: map[string]*ucp.ResourceTypeSummaryResultAPIVersion{
						version: {},
					},
				},
			},
			Locations: map[string]map[string]any{
				"east": {},
			},
		}
		resourceProviderMock.EXPECT().
			GetProviderSummary(gomock.Any(), "local", "Applications.Test", gomock.Any()).
			Return(ucp.ResourceProvidersClientGetProviderSummaryResponse{ResourceProviderSummary: expectedResource}, nil)

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
		client := createResourceAndResourceProviderClient(mockResourceClient, mockResourceProviderClient)
		mockResourceProviderClient.EXPECT().NewListProviderSummariesPager("local", gomock.Any()).Return(pager(resourceProviderSummaryPages))

		mockResourceClient.EXPECT().
			NewListByRootScopePager(gomock.Any()).
			Return(pager(listPages)).AnyTimes()

		mockResourceProviderClient.EXPECT().
			GetProviderSummary(gomock.Any(), "local", gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, plane string, providerName string, opts *ucp.ResourceProvidersClientGetProviderSummaryOptions) (ucp.ResourceProvidersClientGetProviderSummaryResponse, error) {
				summary := findProviderSummary(providerName)
				if summary != nil {
					return ucp.ResourceProvidersClientGetProviderSummaryResponse{
						ResourceProviderSummary: *summary,
					}, nil
				}

				// Fallback for providers not in test data
				return ucp.ResourceProvidersClientGetProviderSummaryResponse{
					ResourceProviderSummary: ucp.ResourceProviderSummary{
						Name: &providerName,
						ResourceTypes: map[string]*ucp.ResourceProviderSummaryResourceType{
							"resourceType" + string(providerName[len(providerName)-1]): {
								APIVersions: map[string]*ucp.ResourceTypeSummaryResultAPIVersion{
									version: {},
								},
							},
						},
					},
				}, nil
			}).AnyTimes()

		expectedResourceList := []generated.GenericResource{*listPages[0].Value[0]}

		resources, err := client.ListResourcesInApplication(context.Background(), "test-application")
		require.NoError(t, err)
		require.Equal(t, expectedResourceList, resources)
	})

	t.Run("ListResourcesInEnvironment", func(t *testing.T) {
		mockResourceClient := NewMockgenericResourceClient(gomock.NewController(t))
		mockResourceProviderClient := NewMockresourceProviderClient(gomock.NewController(t))

		client := createResourceAndResourceProviderClient(mockResourceClient, mockResourceProviderClient)

		mockResourceProviderClient.EXPECT().NewListProviderSummariesPager("local", gomock.Any()).Return(pager(resourceProviderSummaryPages))
		mockResourceClient.EXPECT().
			NewListByRootScopePager(gomock.Any()).
			Return(pager(listPages)).AnyTimes()
		mockResourceProviderClient.EXPECT().
			GetProviderSummary(gomock.Any(), "local", gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, plane string, providerName string, opts *ucp.ResourceProvidersClientGetProviderSummaryOptions) (ucp.ResourceProvidersClientGetProviderSummaryResponse, error) {
				summary := findProviderSummary(providerName)
				if summary != nil {
					return ucp.ResourceProvidersClientGetProviderSummaryResponse{
						ResourceProviderSummary: *summary,
					}, nil
				}

				// Fallback for providers not in test data
				return ucp.ResourceProvidersClientGetProviderSummaryResponse{
					ResourceProviderSummary: ucp.ResourceProviderSummary{
						Name: &providerName,
						ResourceTypes: map[string]*ucp.ResourceProviderSummaryResourceType{
							"resourceType" + string(providerName[len(providerName)-1]): {
								APIVersions: map[string]*ucp.ResourceTypeSummaryResultAPIVersion{
									version: {},
								},
							},
						},
					},
				}, nil
			}).AnyTimes()

		expectedResourceList := []generated.GenericResource{*listPages[0].Value[0], *listPages[0].Value[1]}

		resources, err := client.ListResourcesInEnvironment(context.Background(), "test-environment")
		require.NoError(t, err)
		require.Equal(t, expectedResourceList, resources)
	})

	t.Run("GetResource", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mock := NewMockgenericResourceClient(ctrl)
		resourceProviderMock := NewMockresourceProviderClient(ctrl)
		client := createClient(mock)
		client.resourceProviderClientFactory = func() (resourceProviderClient, error) {
			return resourceProviderMock, nil
		}
		expectedResourceSummary := ucp.ResourceProviderSummary{
			Name: to.Ptr("Applications.Test"),
			ResourceTypes: map[string]*ucp.ResourceProviderSummaryResourceType{
				"testResource": {
					APIVersions: map[string]*ucp.ResourceTypeSummaryResultAPIVersion{
						version: {},
					},
				},
			},
			Locations: map[string]map[string]any{
				"east": {},
			},
		}
		resourceProviderMock.EXPECT().
			GetProviderSummary(gomock.Any(), "local", "Applications.Test", gomock.Any()).
			Return(ucp.ResourceProvidersClientGetProviderSummaryResponse{ResourceProviderSummary: expectedResourceSummary}, nil)

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
		ctrl := gomock.NewController(t)
		mock := NewMockgenericResourceClient(ctrl)
		resourceProviderMock := NewMockresourceProviderClient(ctrl)
		client := createClient(mock)
		client.resourceProviderClientFactory = func() (resourceProviderClient, error) {
			return resourceProviderMock, nil
		}
		expectedResourceSummary := ucp.ResourceProviderSummary{
			Name: to.Ptr("Applications.Test"),
			ResourceTypes: map[string]*ucp.ResourceProviderSummaryResourceType{
				"testResource": {
					APIVersions: map[string]*ucp.ResourceTypeSummaryResultAPIVersion{
						version: {},
					},
				},
			},
			Locations: map[string]map[string]any{
				"east": {},
			},
		}
		resourceProviderMock.EXPECT().
			GetProviderSummary(gomock.Any(), "local", "Applications.Test", gomock.Any()).
			Return(ucp.ResourceProvidersClientGetProviderSummaryResponse{ResourceProviderSummary: expectedResourceSummary}, nil)

		mock.EXPECT().
			BeginDelete(gomock.Any(), testResourceName, gomock.Any()).
			Return(poller(&generated.GenericResourcesClientDeleteResponse{}), nil)

		deleted, err := client.DeleteResource(context.Background(), testResourceType, testResourceID)
		require.NoError(t, err)
		require.True(t, deleted)
	})

	t.Run("Radius.Core resources use specific API version", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mock := NewMockgenericResourceClient(ctrl)
		resourceProviderMock := NewMockresourceProviderClient(ctrl)

		client := &UCPApplicationsManagementClient{
			RootScope: testScope,
			genericResourceClientFactory: func(scope string, resourceType string) (genericResourceClient, error) {
				return mock, nil
			},
			resourceProviderClientFactory: func() (resourceProviderClient, error) {
				return resourceProviderMock, nil
			},
			capture: testCapture,
		}

		// Mock the resource provider summary for Radius.Core
		resourceProviderMock.EXPECT().
			GetProviderSummary(gomock.Any(), "local", "Radius.Core", gomock.Any()).
			Return(ucp.ResourceProvidersClientGetProviderSummaryResponse{
				ResourceProviderSummary: ucp.ResourceProviderSummary{
					Name: to.Ptr("Radius.Core"),
					ResourceTypes: map[string]*ucp.ResourceProviderSummaryResourceType{
						"environments": {
							APIVersions: map[string]*ucp.ResourceTypeSummaryResultAPIVersion{
								"2025-08-01-preview": {},
								"other-version":      {},
							},
						},
					},
				},
			}, nil)

		// Set up expectation for the resource call
		mock.EXPECT().
			Get(gomock.Any(), "test-env", gomock.Any()).
			Return(generated.GenericResourcesClientGetResponse{
				GenericResource: generated.GenericResource{
					ID:   to.Ptr("/test/id"),
					Name: to.Ptr("test-env"),
					Type: to.Ptr("Radius.Core/environments"),
				},
			}, nil)

		// Test via GetResource which calls getGenericClient internally
		// This indirectly tests that getGenericClient handles Radius.Core resources correctly
		_, err := client.GetResource(context.Background(), "Radius.Core/environments", "test-env")
		require.NoError(t, err)
	})
}

func Test_Application(t *testing.T) {
	t.Parallel()
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
							Type:     to.Ptr("Applications.Test1/resourceType1"),
							Location: to.Ptr(v1.LocationGlobal),
							Properties: map[string]any{
								"application": testScope + "/providers/Applications.Core/applications/test-application",
								"environment": testScope + "/providers/Applications.Core/environments/test-environment",
							},
						},
						{
							ID:       to.Ptr(testScope + "/providers/Applications.Test/testResources/test2"),
							Name:     to.Ptr("test2"),
							Type:     to.Ptr("Applications.Test1/resourceType1"),
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

		mockResourceProviderClient.EXPECT().
			GetProviderSummary(gomock.Any(), "local", gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, plane string, providerName string, opts *ucp.ResourceProvidersClientGetProviderSummaryOptions) (ucp.ResourceProvidersClientGetProviderSummaryResponse, error) {
				summary := findProviderSummary(providerName)
				if summary != nil {
					return ucp.ResourceProvidersClientGetProviderSummaryResponse{
						ResourceProviderSummary: *summary,
					}, nil
				}

				// Fallback for providers not in test data
				return ucp.ResourceProvidersClientGetProviderSummaryResponse{
					ResourceProviderSummary: ucp.ResourceProviderSummary{
						Name: &providerName,
						ResourceTypes: map[string]*ucp.ResourceProviderSummaryResourceType{
							"resourceType" + string(providerName[len(providerName)-1]): {
								APIVersions: map[string]*ucp.ResourceTypeSummaryResultAPIVersion{
									version: {},
								},
							},
						},
					},
				}, nil
			}).AnyTimes()

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
	t.Parallel()
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
							Type:     to.Ptr("Applications.Test1/resourceType1"),
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

		resourceProviderMock.EXPECT().
			GetProviderSummary(gomock.Any(), "local", gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, plane string, providerName string, opts *ucp.ResourceProvidersClientGetProviderSummaryOptions) (ucp.ResourceProvidersClientGetProviderSummaryResponse, error) {
				summary := findProviderSummary(providerName)
				if summary != nil {
					return ucp.ResourceProvidersClientGetProviderSummaryResponse{
						ResourceProviderSummary: *summary,
					}, nil
				}

				// Fallback for providers not in test data
				return ucp.ResourceProvidersClientGetProviderSummaryResponse{
					ResourceProviderSummary: ucp.ResourceProviderSummary{
						Name: &providerName,
						ResourceTypes: map[string]*ucp.ResourceProviderSummaryResourceType{
							"resourceType" + string(providerName[len(providerName)-1]): {
								APIVersions: map[string]*ucp.ResourceTypeSummaryResultAPIVersion{
									version: {},
								},
							},
						},
					},
				}, nil
			}).AnyTimes()

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
	t.Parallel()
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
		ctrl := gomock.NewController(t)
		mockResourceGroupClient := NewMockresourceGroupClient(ctrl)
		mockGenericClient := NewMockgenericResourceClient(ctrl)
		mockResourceProviderClient := NewMockresourceProviderClient(ctrl)

		client := &UCPApplicationsManagementClient{
			RootScope: testScope,
			resourceGroupClientFactory: func() (resourceGroupClient, error) {
				return mockResourceGroupClient, nil
			},
			genericResourceClientFactory: func(scope string, resourceType string) (genericResourceClient, error) {
				return mockGenericClient, nil
			},
			resourceProviderClientFactory: func() (resourceProviderClient, error) {
				return mockResourceProviderClient, nil
			},
			capture: testCapture,
		}

		// Expect resource group existence check (called twice: once in DeleteResourceGroup, once in ListResourcesInResourceGroup)
		mockResourceGroupExists(mockResourceGroupClient, "local", testResourceName, 2)

		// Expect listing all resource types
		mockListProviders(mockResourceProviderClient, "local")

		// Expect provider summaries for listing resources
		mockProviderSummaries(mockResourceProviderClient, "local", 4)

		// Expect listing resources for each type (empty results)
		emptyResources := []generated.GenericResourcesClientListByRootScopeResponse{
			{
				GenericResourcesList: generated.GenericResourcesList{
					Value:    []*generated.GenericResource{},
					NextLink: to.Ptr("0"),
				},
			},
		}
		mockGenericClient.EXPECT().
			NewListByRootScopePager(gomock.Any()).
			Return(pager(emptyResources)).Times(4)

		mockResourceGroupClient.EXPECT().
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

func Test_DeleteResourceGroup(t *testing.T) {
	t.Parallel()

	t.Run("empty group", func(t *testing.T) {
		client, rgClient, genericClient, rpClient := setupResourceGroupMocks(t)

		// Setup: group exists but is empty
		mockResourceGroupExists(rgClient, "local", "test-rg", 2)
		mockListProviders(rpClient, "local")
		mockProviderSummaries(rpClient, "local", 4)

		// No resources in group
		emptyList := createResourceList()
		genericClient.EXPECT().NewListByRootScopePager(gomock.Any()).Return(pager(emptyList)).Times(4)

		// Expect group deletion
		mockResourceGroupDeletion(rgClient, "local", "test-rg")

		deleted, err := client.DeleteResourceGroup(context.Background(), "local", "test-rg")
		require.NoError(t, err)
		require.True(t, deleted)
	})

	t.Run("group with resources", func(t *testing.T) {
		client, rgClient, genericClient, rpClient := setupResourceGroupMocks(t)

		// Setup standard expectations
		mockResourceGroupExists(rgClient, "local", "test-rg", 2)
		mockListProviders(rpClient, "local")
		mockProviderSummaries(rpClient, "local", 4)

		// Create test resources
		resources := createResourceList(
			createResource("resource1", "Applications.Test1/resourceType1"),
			createResource("test-env", "Applications.Core/environments"),
		)
		genericClient.EXPECT().NewListByRootScopePager(gomock.Any()).Return(pager(resources)).Times(4)

		// Expect deletion of each resource
		mockProviderSummaryForDeletion(rpClient, "local", "Applications.Test1")
		mockProviderSummaryForDeletion(rpClient, "local", "Applications.Core")
		mockResourceDeletion(genericClient, "resource1")
		mockResourceDeletion(genericClient, "test-env")

		// Expect group deletion
		mockResourceGroupDeletion(rgClient, "local", "test-rg")

		deleted, err := client.DeleteResourceGroup(context.Background(), "local", "test-rg")
		require.NoError(t, err)
		require.True(t, deleted)
	})

	t.Run("resource deletion fails", func(t *testing.T) {
		client, rgClient, genericClient, rpClient := setupResourceGroupMocks(t)

		mockResourceGroupExists(rgClient, "local", "test-rg", 2)
		mockListProviders(rpClient, "local")
		mockProviderSummaries(rpClient, "local", 4)

		resources := createResourceList(
			createResource("test-env", "Applications.Core/environments"),
		)
		genericClient.EXPECT().NewListByRootScopePager(gomock.Any()).Return(pager(resources)).Times(4)

		mockProviderSummaryForDeletion(rpClient, "local", "Applications.Core")
		mockResourceDeletionFailure(genericClient, "test-env", "deletion failed")

		deleted, err := client.DeleteResourceGroup(context.Background(), "local", "test-rg")
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to delete resources in group")
		require.False(t, deleted)
	})
}

// runListTest is a helper for testing list operations with filters
func runListTest(t *testing.T, client *UCPApplicationsManagementClient, resourceGroupName, environmentID, applicationID string, expectedNames []string) {
	resources, err := client.ListResourcesInResourceGroupFiltered(
		context.Background(), "local", resourceGroupName, environmentID, applicationID)
	require.NoError(t, err)
	require.Len(t, resources, len(expectedNames))
	for i, expectedName := range expectedNames {
		require.Equal(t, expectedName, *resources[i].Name)
	}
}

func Test_ListResourcesInResourceGroup(t *testing.T) {
	t.Parallel()

	envID := "/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/environments/test-env"
	appID := "/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/applications/test-app"

	// Test data
	resource1 := withProperties(
		createResource("resource1", "Applications.Test1/resourceType1"),
		map[string]any{"environment": envID, "application": appID})
	resource2 := withProperties(
		createResource("resource2", "Applications.Core/environments"),
		map[string]any{"environment": envID})
	resource3 := withProperties(
		createResource("resource3", "Applications.Test1/resourceType1"),
		map[string]any{"application": appID})
	resource4 := createResource("resource4", "Applications.Test2/resourceType2")

	allResources := createResourceList(resource1, resource2, resource3, resource4)
	emptyResources := createResourceList()

	t.Run("list all resources", func(t *testing.T) {
		client, mockRG, mockGeneric, mockRP := setupResourceGroupMocks(t)

		mockResourceGroupExists(mockRG, "local", "test-group", 1)
		mockListProviders(mockRP, "local")
		mockProviderSummaries(mockRP, "local", 4)
		mockGeneric.EXPECT().
			NewListByRootScopePager(gomock.Any()).
			Return(pager(allResources)).Times(4)

		resources, err := client.ListResourcesInResourceGroup(context.Background(), "local", "test-group")
		require.NoError(t, err)
		require.Len(t, resources, 4)
		require.Equal(t, "resource1", *resources[0].Name)
		require.Equal(t, "resource2", *resources[1].Name)
		require.Equal(t, "resource3", *resources[2].Name)
		require.Equal(t, "resource4", *resources[3].Name)
	})

	t.Run("empty resource group", func(t *testing.T) {
		client, mockRG, mockGeneric, mockRP := setupResourceGroupMocks(t)

		mockResourceGroupExists(mockRG, "local", "test-group", 1)
		mockListProviders(mockRP, "local")
		mockProviderSummaries(mockRP, "local", 4)
		mockGeneric.EXPECT().
			NewListByRootScopePager(gomock.Any()).
			Return(pager(emptyResources)).Times(4)

		resources, err := client.ListResourcesInResourceGroup(context.Background(), "local", "test-group")
		require.NoError(t, err)
		require.Empty(t, resources)
	})

	t.Run("API version errors", func(t *testing.T) {
		client, mockRG, mockGeneric, mockRP := setupResourceGroupMocks(t)

		// Provider summaries with partial API versions
		summariesWithErrors := []ucp.ResourceProvidersClientListProviderSummariesResponse{{
			PagedResourceProviderSummary: ucp.PagedResourceProviderSummary{
				Value: []*ucp.ResourceProviderSummary{
					{
						Name: to.Ptr("Applications.Test"),
						ResourceTypes: map[string]*ucp.ResourceProviderSummaryResourceType{
							"resources": {APIVersions: map[string]*ucp.ResourceTypeSummaryResultAPIVersion{version: {}}},
						},
					},
					{
						Name: to.Ptr("Applications.TestNoVersion"),
						ResourceTypes: map[string]*ucp.ResourceProviderSummaryResourceType{
							"resources": {}, // Empty API versions
						},
					},
				},
				NextLink: to.Ptr("0"),
			},
		}}

		mockResourceGroupExists(mockRG, "local", "test-group", 1)
		mockRP.EXPECT().
			NewListProviderSummariesPager("local", gomock.Any()).
			Return(pager(summariesWithErrors))

		// First provider succeeds
		mockRP.EXPECT().
			GetProviderSummary(gomock.Any(), "local", "Applications.Test", gomock.Any()).
			Return(ucp.ResourceProvidersClientGetProviderSummaryResponse{
				ResourceProviderSummary: *summariesWithErrors[0].Value[0],
			}, nil)

		// Second provider has empty API versions
		mockRP.EXPECT().
			GetProviderSummary(gomock.Any(), "local", "Applications.TestNoVersion", gomock.Any()).
			Return(ucp.ResourceProvidersClientGetProviderSummaryResponse{
				ResourceProviderSummary: *summariesWithErrors[0].Value[1],
			}, nil)

		testResource := createResourceList(createResource("resource1", "Applications.Test/resources"))

		mockGeneric.EXPECT().
			NewListByRootScopePager(gomock.Any()).
			Return(pager(testResource)).Times(1)
		mockGeneric.EXPECT().
			NewListByRootScopePager(gomock.Any()).
			Return(pager(emptyResources)).Times(1)

		resources, err := client.ListResourcesInResourceGroup(context.Background(), "local", "test-group")
		require.NoError(t, err)
		require.Len(t, resources, 1)
		require.Equal(t, "resource1", *resources[0].Name)
	})

	t.Run("filter by environment", func(t *testing.T) {
		client, mockRG, mockGeneric, mockRP := setupResourceGroupMocks(t)

		mockResourceGroupExists(mockRG, "local", "test-group", 1)
		mockListProviders(mockRP, "local")
		mockProviderSummaries(mockRP, "local", 4)
		mockGeneric.EXPECT().
			NewListByRootScopePager(gomock.Any()).
			Return(pager(allResources)).Times(4)

		runListTest(t, client, "test-group", envID, "", []string{"resource1", "resource2"})
	})

	t.Run("filter by application", func(t *testing.T) {
		client, mockRG, mockGeneric, mockRP := setupResourceGroupMocks(t)

		mockResourceGroupExists(mockRG, "local", "test-group", 1)
		mockListProviders(mockRP, "local")
		mockProviderSummaries(mockRP, "local", 4)
		mockGeneric.EXPECT().
			NewListByRootScopePager(gomock.Any()).
			Return(pager(allResources)).Times(4)

		runListTest(t, client, "test-group", "", appID, []string{"resource1", "resource3"})
	})

	t.Run("filter by both", func(t *testing.T) {
		client, mockRG, mockGeneric, mockRP := setupResourceGroupMocks(t)

		mockResourceGroupExists(mockRG, "local", "test-group", 1)
		mockListProviders(mockRP, "local")
		mockProviderSummaries(mockRP, "local", 4)
		mockGeneric.EXPECT().
			NewListByRootScopePager(gomock.Any()).
			Return(pager(allResources)).Times(4)

		runListTest(t, client, "test-group", envID, appID, []string{"resource1"})
	})

	t.Run("no filters", func(t *testing.T) {
		client, mockRG, mockGeneric, mockRP := setupResourceGroupMocks(t)

		mockResourceGroupExists(mockRG, "local", "test-group", 1)
		mockListProviders(mockRP, "local")
		mockProviderSummaries(mockRP, "local", 4)
		mockGeneric.EXPECT().
			NewListByRootScopePager(gomock.Any()).
			Return(pager(allResources)).Times(4)

		runListTest(t, client, "test-group", "", "", []string{"resource1", "resource2", "resource3", "resource4"})
	})
}

// runListResourcesOfTypeTest is a helper for testing list resources of type operations with filters
func runListResourcesOfTypeTest(t *testing.T, client *UCPApplicationsManagementClient, resourceGroupName, resourceType, environmentID, applicationID string, expectedNames []string) {
	resources, err := client.ListResourcesOfTypeInResourceGroupFiltered(
		context.Background(), "local", resourceGroupName, resourceType, environmentID, applicationID)
	require.NoError(t, err)
	require.Len(t, resources, len(expectedNames))
	for i, expectedName := range expectedNames {
		require.Equal(t, expectedName, *resources[i].Name)
	}
}

func Test_ListResourcesOfTypeInResourceGroup(t *testing.T) {
	t.Parallel()

	testResourceType := "Applications.Test1/resourceType1"
	envID := "/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/environments/test-env"
	appID := "/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/applications/test-app"

	// Test data
	resource1 := withProperties(
		createResource("resource1", testResourceType),
		map[string]any{"environment": envID, "application": appID})
	resource2 := withProperties(
		createResource("resource2", testResourceType),
		map[string]any{"environment": envID})
	resource3 := withProperties(
		createResource("resource3", testResourceType),
		map[string]any{"application": appID})

	allResourcesOfType := createResourceList(resource1, resource2, resource3)
	emptyResources := createResourceList()

	t.Run("list specific type", func(t *testing.T) {
		client, mockRG, mockGeneric, mockRP := setupResourceGroupMocks(t)

		mockResourceGroupExists(mockRG, "local", "test-group", 1)
		mockRP.EXPECT().
			GetProviderSummary(gomock.Any(), "local", "Applications.Test1", gomock.Any()).
			Return(ucp.ResourceProvidersClientGetProviderSummaryResponse{
				ResourceProviderSummary: *resourceProviderSummaryPages[0].Value[0],
			}, nil)
		mockGeneric.EXPECT().
			NewListByRootScopePager(gomock.Any()).
			Return(pager(allResourcesOfType))

		resources, err := client.ListResourcesOfTypeInResourceGroup(
			context.Background(), "local", "test-group", testResourceType)
		require.NoError(t, err)
		require.Len(t, resources, 3)
		require.Equal(t, "resource1", *resources[0].Name)
		require.Equal(t, "resource2", *resources[1].Name)
		require.Equal(t, "resource3", *resources[2].Name)
	})

	t.Run("empty results", func(t *testing.T) {
		client, mockRG, mockGeneric, mockRP := setupResourceGroupMocks(t)

		mockResourceGroupExists(mockRG, "local", "test-group", 1)
		mockRP.EXPECT().
			GetProviderSummary(gomock.Any(), "local", "Applications.Test1", gomock.Any()).
			Return(ucp.ResourceProvidersClientGetProviderSummaryResponse{
				ResourceProviderSummary: *resourceProviderSummaryPages[0].Value[0],
			}, nil)
		mockGeneric.EXPECT().
			NewListByRootScopePager(gomock.Any()).
			Return(pager(emptyResources))

		resources, err := client.ListResourcesOfTypeInResourceGroup(
			context.Background(), "local", "test-group", testResourceType)
		require.NoError(t, err)
		require.Empty(t, resources)
	})

	t.Run("API version error", func(t *testing.T) {
		client, mockRG, _, mockRP := setupResourceGroupMocks(t)

		mockResourceGroupExists(mockRG, "local", "test-group", 1)
		mockRP.EXPECT().
			GetProviderSummary(gomock.Any(), "local", "Unknown.Provider", gomock.Any()).
			Return(ucp.ResourceProvidersClientGetProviderSummaryResponse{}, fmt.Errorf("provider not found"))

		_, err := client.ListResourcesOfTypeInResourceGroup(
			context.Background(), "local", "test-group", "Unknown.Provider/unknownType")
		require.Error(t, err)
		require.Contains(t, err.Error(), "provider not found")
	})

	t.Run("filter by environment", func(t *testing.T) {
		client, mockRG, mockGeneric, mockRP := setupResourceGroupMocks(t)

		mockResourceGroupExists(mockRG, "local", "test-group", 1)
		mockRP.EXPECT().
			GetProviderSummary(gomock.Any(), "local", "Applications.Test1", gomock.Any()).
			Return(ucp.ResourceProvidersClientGetProviderSummaryResponse{
				ResourceProviderSummary: *resourceProviderSummaryPages[0].Value[0],
			}, nil)
		mockGeneric.EXPECT().
			NewListByRootScopePager(gomock.Any()).
			Return(pager(allResourcesOfType))

		runListResourcesOfTypeTest(t, client, "test-group", testResourceType, envID, "", []string{"resource1", "resource2"})
	})

	t.Run("filter by application", func(t *testing.T) {
		client, mockRG, mockGeneric, mockRP := setupResourceGroupMocks(t)

		mockResourceGroupExists(mockRG, "local", "test-group", 1)
		mockRP.EXPECT().
			GetProviderSummary(gomock.Any(), "local", "Applications.Test1", gomock.Any()).
			Return(ucp.ResourceProvidersClientGetProviderSummaryResponse{
				ResourceProviderSummary: *resourceProviderSummaryPages[0].Value[0],
			}, nil)
		mockGeneric.EXPECT().
			NewListByRootScopePager(gomock.Any()).
			Return(pager(allResourcesOfType))

		runListResourcesOfTypeTest(t, client, "test-group", testResourceType, "", appID, []string{"resource1", "resource3"})
	})

	t.Run("filter by both", func(t *testing.T) {
		client, mockRG, mockGeneric, mockRP := setupResourceGroupMocks(t)

		mockResourceGroupExists(mockRG, "local", "test-group", 1)
		mockRP.EXPECT().
			GetProviderSummary(gomock.Any(), "local", "Applications.Test1", gomock.Any()).
			Return(ucp.ResourceProvidersClientGetProviderSummaryResponse{
				ResourceProviderSummary: *resourceProviderSummaryPages[0].Value[0],
			}, nil)
		mockGeneric.EXPECT().
			NewListByRootScopePager(gomock.Any()).
			Return(pager(allResourcesOfType))

		runListResourcesOfTypeTest(t, client, "test-group", testResourceType, envID, appID, []string{"resource1"})
	})

	t.Run("no matches", func(t *testing.T) {
		client, mockRG, mockGeneric, mockRP := setupResourceGroupMocks(t)

		mockResourceGroupExists(mockRG, "local", "test-group", 1)
		mockRP.EXPECT().
			GetProviderSummary(gomock.Any(), "local", "Applications.Test1", gomock.Any()).
			Return(ucp.ResourceProvidersClientGetProviderSummaryResponse{
				ResourceProviderSummary: *resourceProviderSummaryPages[0].Value[0],
			}, nil)
		mockGeneric.EXPECT().
			NewListByRootScopePager(gomock.Any()).
			Return(pager(allResourcesOfType))

		otherEnvID := "/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/environments/other-env"
		runListResourcesOfTypeTest(t, client, "test-group", testResourceType, otherEnvID, "", []string{})
	})
}

func Test_ResourceProvider(t *testing.T) {
	t.Parallel()
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
		expected := []ucp.ResourceProviderSummary{*resourceProviderSummaryPages[0].Value[0], *resourceProviderSummaryPages[0].Value[1], *resourceProviderSummaryPages[1].Value[0], *resourceProviderSummaryPages[1].Value[1], *resourceProviderSummaryPages[1].Value[2]}

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
					APIVersions: map[string]*ucp.ResourceTypeSummaryResultAPIVersion{
						version: {},
					},
					DefaultAPIVersion: to.Ptr(version),
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
	t.Parallel()
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
	t.Parallel()
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
	testAPIVersionResourceName := version

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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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

// findProviderSummary is a helper function to find a provider summary from test data
func findProviderSummary(providerName string) *ucp.ResourceProviderSummary {
	for _, page := range resourceProviderSummaryPages {
		for _, provider := range page.Value {
			if *provider.Name == providerName {
				return provider
			}
		}
	}

	return nil
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

	return runtime.NewPager(handler)
}

func poller[T any](response *T) *runtime.Poller[T] {

	p, err := runtime.NewPoller(nil, runtime.Pipeline{}, &runtime.NewPollerOptions[T]{
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
