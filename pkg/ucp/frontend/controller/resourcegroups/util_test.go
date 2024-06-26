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
package resourcegroups

import (
	"errors"
	"fmt"
	"net/url"
	"testing"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/ucp/datamodel"
	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/radius-project/radius/pkg/ucp/store"
	"github.com/radius-project/radius/test/testcontext"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func Test_ValidateDownstream(t *testing.T) {
	id, err := resources.ParseResource("/planes/radius/local/resourceGroups/test-group/providers/System.TestRP/testResources/name")
	require.NoError(t, err)

	providerID := MakeResourceProviderID(id)

	idWithoutResourceGroup, err := resources.Parse("/planes/radius/local/providers/System.TestRP/testResources")
	require.NoError(t, err)

	downstream := "http://localhost:7443"

	plane := &datamodel.RadiusPlane{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID: id.PlaneScope(),
			},
		},
		Properties: datamodel.RadiusPlaneProperties{
			ResourceProviders: map[string]string{
				"System.TestRP": downstream,
			},
		},
	}

	resourceGroup := &datamodel.ResourceGroup{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID: id.RootScope(),
			},
		},
	}

	setup := func(t *testing.T) *store.MockStorageClient {
		ctrl := gomock.NewController(t)
		return store.NewMockStorageClient(ctrl)
	}

	t.Run("success (resource group)", func(t *testing.T) {
		mock := setup(t)
		mock.EXPECT().Get(gomock.Any(), providerID.String()).Return(nil, &store.ErrNotFound{}).Times(1)
		mock.EXPECT().Get(gomock.Any(), id.PlaneScope()).Return(&store.Object{Data: plane}, nil).Times(1)
		mock.EXPECT().Get(gomock.Any(), id.RootScope()).Return(&store.Object{Data: resourceGroup}, nil).Times(1)

		expectedURL, err := url.Parse(downstream)
		require.NoError(t, err)

		downstreamURL, routingType, err := ValidateDownstream(testcontext.New(t), mock, id, "global")
		require.NoError(t, err)
		require.Equal(t, expectedURL, downstreamURL)
		require.Equal(t, RoutingTypeProxy, routingType)
	})

	t.Run("success (non resource group)", func(t *testing.T) {
		mock := setup(t)
		mock.EXPECT().Get(gomock.Any(), providerID.String()).Return(nil, &store.ErrNotFound{}).Times(1)
		mock.EXPECT().Get(gomock.Any(), idWithoutResourceGroup.PlaneScope()).Return(&store.Object{Data: plane}, nil).Times(1)

		expectedURL, err := url.Parse(downstream)
		require.NoError(t, err)

		downstreamURL, routingType, err := ValidateDownstream(testcontext.New(t), mock, idWithoutResourceGroup, "global")
		require.NoError(t, err)
		require.Equal(t, expectedURL, downstreamURL)
		require.Equal(t, RoutingTypeProxy, routingType)
	})

	t.Run("success (resource provider: internal)", func(t *testing.T) {
		resourceProvider := &datamodel.ResourceProvider{
			BaseResource: v1.BaseResource{
				TrackedResource: v1.TrackedResource{
					Name: "System.TestRP",
					ID:   providerID.String(),
				},
			},
			Properties: datamodel.ResourceProviderProperties{
				Locations: map[string]datamodel.ResourceProviderLocation{
					"global": {
						Address: "internal",
					},
				},
				ResourceTypes: []datamodel.ResourceType{
					{
						ResourceType: "testResources",
						Locations: []string{
							"global",
						},
					},
				},
			},
		}

		mock := setup(t)
		mock.EXPECT().Get(gomock.Any(), providerID.String()).Return(&store.Object{Data: resourceProvider}, nil).Times(1)
		mock.EXPECT().Get(gomock.Any(), id.PlaneScope()).Return(&store.Object{Data: plane}, nil).Times(1)
		mock.EXPECT().Get(gomock.Any(), id.RootScope()).Return(&store.Object{Data: resourceGroup}, nil).Times(1)

		downstreamURL, routingType, err := ValidateDownstream(testcontext.New(t), mock, id, "global")
		require.NoError(t, err)
		require.Nil(t, downstreamURL)
		require.Equal(t, RoutingTypeInternal, routingType)
	})

	t.Run("success (resource provider: proxy)", func(t *testing.T) {
		resourceProvider := &datamodel.ResourceProvider{
			BaseResource: v1.BaseResource{
				TrackedResource: v1.TrackedResource{
					Name: "System.TestRP",
					ID:   providerID.String(),
				},
			},
			Properties: datamodel.ResourceProviderProperties{
				Locations: map[string]datamodel.ResourceProviderLocation{
					"global": {
						Address: "http://localhost:7443",
					},
				},
				ResourceTypes: []datamodel.ResourceType{
					{
						ResourceType: "testResources",
						Locations: []string{
							"global",
						},
					},
				},
			},
		}

		mock := setup(t)
		mock.EXPECT().Get(gomock.Any(), providerID.String()).Return(&store.Object{Data: resourceProvider}, nil).Times(1)
		mock.EXPECT().Get(gomock.Any(), id.PlaneScope()).Return(&store.Object{Data: plane}, nil).Times(1)
		mock.EXPECT().Get(gomock.Any(), id.RootScope()).Return(&store.Object{Data: resourceGroup}, nil).Times(1)

		expectedURL, err := url.Parse("http://localhost:7443")
		require.NoError(t, err)

		downstreamURL, routingType, err := ValidateDownstream(testcontext.New(t), mock, id, "global")
		require.NoError(t, err)
		require.Equal(t, expectedURL, downstreamURL)
		require.Equal(t, RoutingTypeProxy, routingType)
	})

	t.Run("plane not found", func(t *testing.T) {
		mock := setup(t)
		mock.EXPECT().Get(gomock.Any(), id.PlaneScope()).Return(nil, &store.ErrNotFound{}).Times(1)

		downstreamURL, routingType, err := ValidateDownstream(testcontext.New(t), mock, id, "global")
		require.Error(t, err)
		require.Equal(t, &NotFoundError{Message: "plane \"/planes/radius/local\" not found"}, err)
		require.Nil(t, downstreamURL)
		require.Equal(t, RoutingTypeInvalid, routingType)
	})

	t.Run("plane retreival failure", func(t *testing.T) {
		mock := setup(t)

		expected := fmt.Errorf("failed to find plane \"/planes/radius/local\": %w", errors.New("test error"))
		mock.EXPECT().Get(gomock.Any(), id.PlaneScope()).Return(nil, errors.New("test error")).Times(1)

		downstreamURL, routingType, err := ValidateDownstream(testcontext.New(t), mock, id, "global")
		require.Error(t, err)
		require.Equal(t, expected, err)
		require.Nil(t, downstreamURL)
		require.Equal(t, RoutingTypeInvalid, routingType)
	})

	t.Run("resource group not found", func(t *testing.T) {
		mock := setup(t)
		mock.EXPECT().Get(gomock.Any(), id.PlaneScope()).Return(&store.Object{Data: plane}, nil).Times(1)
		mock.EXPECT().Get(gomock.Any(), id.RootScope()).Return(nil, &store.ErrNotFound{}).Times(1)

		downstreamURL, routingType, err := ValidateDownstream(testcontext.New(t), mock, id, "global")
		require.Error(t, err)
		require.Equal(t, &NotFoundError{Message: "resource group \"/planes/radius/local/resourceGroups/test-group\" not found"}, err)
		require.Nil(t, downstreamURL)
		require.Equal(t, RoutingTypeInvalid, routingType)
	})

	t.Run("resource group err", func(t *testing.T) {
		mock := setup(t)

		mock.EXPECT().Get(gomock.Any(), id.PlaneScope()).Return(&store.Object{Data: plane}, nil).Times(1)
		mock.EXPECT().Get(gomock.Any(), id.RootScope()).Return(nil, errors.New("test error")).Times(1)

		downstreamURL, routingType, err := ValidateDownstream(testcontext.New(t), mock, id, "global")
		require.Error(t, err)
		require.Equal(t, "failed to find resource group \"/planes/radius/local/resourceGroups/test-group\": test error", err.Error())
		require.Nil(t, downstreamURL)
		require.Equal(t, RoutingTypeInvalid, routingType)
	})

	t.Run("resource provider not found", func(t *testing.T) {
		plane := &datamodel.RadiusPlane{
			BaseResource: v1.BaseResource{
				TrackedResource: v1.TrackedResource{
					ID: id.PlaneScope(),
				},
			},
			Properties: datamodel.RadiusPlaneProperties{
				ResourceProviders: map[string]string{},
			},
		}

		resourceGroup := &datamodel.ResourceGroup{
			BaseResource: v1.BaseResource{
				TrackedResource: v1.TrackedResource{
					ID: id.RootScope(),
				},
			},
		}

		mock := setup(t)
		mock.EXPECT().Get(gomock.Any(), providerID.String()).Return(nil, &store.ErrNotFound{}).Times(1)
		mock.EXPECT().Get(gomock.Any(), id.PlaneScope()).Return(&store.Object{Data: plane}, nil).Times(1)
		mock.EXPECT().Get(gomock.Any(), id.RootScope()).Return(&store.Object{Data: resourceGroup}, nil).Times(1)

		downstreamURL, routingType, err := ValidateDownstream(testcontext.New(t), mock, id, "global")
		require.Error(t, err)
		require.Equal(t, &InvalidError{Message: "resource provider System.TestRP not configured"}, err)
		require.Nil(t, downstreamURL)
		require.Equal(t, RoutingTypeInvalid, routingType)
	})

	t.Run("resource provider invalid URL", func(t *testing.T) {
		plane := &datamodel.RadiusPlane{
			BaseResource: v1.BaseResource{
				TrackedResource: v1.TrackedResource{
					ID: id.PlaneScope(),
				},
			},
			Properties: datamodel.RadiusPlaneProperties{
				ResourceProviders: map[string]string{
					"System.TestRP": "\ninvalid",
				},
			},
		}

		resourceGroup := &datamodel.ResourceGroup{
			BaseResource: v1.BaseResource{
				TrackedResource: v1.TrackedResource{
					ID: id.RootScope(),
				},
			},
		}

		mock := setup(t)
		mock.EXPECT().Get(gomock.Any(), providerID.String()).Return(nil, &store.ErrNotFound{}).Times(1)
		mock.EXPECT().Get(gomock.Any(), id.PlaneScope()).Return(&store.Object{Data: plane}, nil).Times(1)
		mock.EXPECT().Get(gomock.Any(), id.RootScope()).Return(&store.Object{Data: resourceGroup}, nil).Times(1)

		downstreamURL, routingType, err := ValidateDownstream(testcontext.New(t), mock, id, "global")
		require.Error(t, err)
		require.Equal(t, &InvalidError{Message: "failed to parse downstream URL: parse \"\\ninvalid\": net/url: invalid control character in URL"}, err)
		require.Nil(t, downstreamURL)
		require.Equal(t, RoutingTypeInvalid, routingType)
	})
}

func Test_ValidateResourceType(t *testing.T) {
	id := resources.MustParse("/planes/radius/local/resourceGroups/test-group/providers/Applications.Test/testResources/testResource")

	provider := &datamodel.ResourceProvider{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				Name: "Applications.Test",
			},
		},
		Properties: datamodel.ResourceProviderProperties{
			Locations: map[string]datamodel.ResourceProviderLocation{
				"proxy": {
					Address: "http://localhost:7443", // Proxy
				},
				"internal": {
					Address: "internal", // Internal
				},
			},
			ResourceTypes: []datamodel.ResourceType{
				{
					ResourceType: "testResources",
					Locations: []string{
						"proxy",
						"internal",
					},
				},
			},
		},
	}

	t.Run("Success: proxy", func(t *testing.T) {
		parsed, err := url.Parse("http://localhost:7443")
		require.NoError(t, err)

		resourceType, downstream, routingType, err := ValidateResourceType(id, "proxy", provider)
		require.Equal(t, provider.Properties.ResourceTypes[0], *resourceType)
		require.Equal(t, downstream, parsed)
		require.Equal(t, RoutingTypeProxy, routingType)
		require.NoError(t, err)
	})

	t.Run("Success: internal", func(t *testing.T) {
		resourceType, downstream, routingType, err := ValidateResourceType(id, "internal", provider)
		require.Equal(t, provider.Properties.ResourceTypes[0], *resourceType)
		require.Nil(t, downstream)
		require.Equal(t, RoutingTypeInternal, routingType)
		require.NoError(t, err)
	})

	t.Run("Success: operationStatuses", func(t *testing.T) {
		id := resources.MustParse("/planes/radius/local/providers/Applications.Test/locations/internal/operationStatuses/abcd")
		resourceType, downstream, routingType, err := ValidateResourceType(id, "internal", provider)
		require.Nil(t, resourceType)
		require.Nil(t, downstream)
		require.Equal(t, RoutingTypeInternal, routingType)
		require.NoError(t, err)
	})

	t.Run("Success: operationResults", func(t *testing.T) {
		id := resources.MustParse("/planes/radius/local/providers/Applications.Test/locations/internal/operationResults/abcd")
		resourceType, downstream, routingType, err := ValidateResourceType(id, "internal", provider)
		require.Nil(t, resourceType)
		require.Nil(t, downstream)
		require.Equal(t, RoutingTypeInternal, routingType)
		require.NoError(t, err)
	})

	t.Run("ResourceType not found", func(t *testing.T) {
		id := resources.MustParse("/planes/radius/local/resourceGroups/test-group/providers/Applications.Test/anotherType/testResource")
		resourceType, downstream, routingType, err := ValidateResourceType(id, "internal", provider)
		require.Nil(t, resourceType)
		require.Nil(t, downstream)
		require.Equal(t, RoutingTypeInvalid, routingType)
		require.Error(t, err)
		require.ErrorIs(t, err, &NotFoundError{Message: "resource type \"Applications.Test/anotherType\" not found"})
		require.Equal(t, "resource type \"Applications.Test/anotherType\" not found", err.Error())
	})

	t.Run("Location not supported", func(t *testing.T) {
		resourceType, downstream, routingType, err := ValidateResourceType(id, "another-one", provider)
		require.Nil(t, resourceType)
		require.Nil(t, downstream)
		require.Equal(t, RoutingTypeInvalid, routingType)
		require.Error(t, err)
		require.ErrorIs(t, err, &InvalidError{Message: "resource type \"Applications.Test/testResources\" not supported at location \"another-one\""})
		require.Equal(t, "resource type \"Applications.Test/testResources\" not supported at location \"another-one\"", err.Error())
	})
}
