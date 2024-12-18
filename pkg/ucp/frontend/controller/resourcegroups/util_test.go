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
	"github.com/radius-project/radius/pkg/components/database"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/pkg/ucp/datamodel"
	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/radius-project/radius/test/testcontext"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

const (
	location   = "east"
	apiVersion = "2025-01-01"
)

func Test_ValidateDownstream(t *testing.T) {
	id, err := resources.ParseResource("/planes/radius/local/resourceGroups/test-group/providers/System.TestRP/testResources/name")
	require.NoError(t, err)

	idWithoutResourceGroup, err := resources.Parse("/planes/radius/local/providers/System.TestRP/testResources")
	require.NoError(t, err)

	resourceTypeID, err := datamodel.ResourceTypeIDFromResourceID(id)
	require.NoError(t, err)

	locationID, err := datamodel.ResourceProviderLocationIDFromResourceID(id, location)
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

	resourceTypeResource := &datamodel.ResourceType{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				Name: "testResources",
				ID:   resourceTypeID.String(),
			},
		},
		Properties: datamodel.ResourceTypeProperties{},
	}

	locationResource := &datamodel.Location{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				Name: location,
				ID:   locationID.String(),
			},
		},
		Properties: datamodel.LocationProperties{
			Address: to.Ptr("http://localhost:7443"),
			ResourceTypes: map[string]datamodel.LocationResourceTypeConfiguration{
				"testResources": {
					APIVersions: map[string]datamodel.LocationAPIVersionConfiguration{
						apiVersion: {},
					},
				},
			},
		},
	}

	setup := func(t *testing.T) *database.MockClient {
		ctrl := gomock.NewController(t)
		return database.NewMockClient(ctrl)
	}

	t.Run("success (resource group)", func(t *testing.T) {
		resourceGroup := &datamodel.ResourceGroup{
			BaseResource: v1.BaseResource{
				TrackedResource: v1.TrackedResource{
					ID: id.RootScope(),
				},
			},
		}

		mock := setup(t)
		mock.EXPECT().Get(gomock.Any(), id.PlaneScope()).Return(&database.Object{Data: plane}, nil).Times(1)
		mock.EXPECT().Get(gomock.Any(), id.RootScope()).Return(&database.Object{Data: resourceGroup}, nil).Times(1)
		mock.EXPECT().Get(gomock.Any(), resourceTypeResource.ID).Return(&database.Object{Data: resourceTypeResource}, nil).Times(1)
		mock.EXPECT().Get(gomock.Any(), locationResource.ID).Return(&database.Object{Data: locationResource}, nil).Times(1)

		expectedURL, err := url.Parse(downstream)
		require.NoError(t, err)

		downstreamURL, err := ValidateDownstream(testcontext.New(t), mock, id, location, apiVersion)
		require.NoError(t, err)
		require.Equal(t, expectedURL, downstreamURL)
	})

	t.Run("success (non resource group)", func(t *testing.T) {
		mock := setup(t)
		mock.EXPECT().Get(gomock.Any(), idWithoutResourceGroup.PlaneScope()).Return(&database.Object{Data: plane}, nil).Times(1)
		mock.EXPECT().Get(gomock.Any(), resourceTypeResource.ID).Return(&database.Object{Data: resourceTypeResource}, nil).Times(1)
		mock.EXPECT().Get(gomock.Any(), locationResource.ID).Return(&database.Object{Data: locationResource}, nil).Times(1)

		expectedURL, err := url.Parse(downstream)
		require.NoError(t, err)

		downstreamURL, err := ValidateDownstream(testcontext.New(t), mock, idWithoutResourceGroup, location, apiVersion)
		require.NoError(t, err)
		require.Equal(t, expectedURL, downstreamURL)
	})

	// The deployment engine models its operation status resources as child resources of the deployment resource.
	t.Run("success (operationstatuses as child resource)", func(t *testing.T) {
		mock := setup(t)
		mock.EXPECT().Get(gomock.Any(), idWithoutResourceGroup.PlaneScope()).Return(&database.Object{Data: plane}, nil).Times(1)
		mock.EXPECT().Get(gomock.Any(), locationResource.ID).Return(&database.Object{Data: locationResource}, nil).Times(1)

		operationStatusID := resources.MustParse("/planes/radius/local/providers/System.TestRP/deployments/xzy/operationStatuses/abcd")

		expectedURL, err := url.Parse(downstream)
		require.NoError(t, err)

		downstreamURL, err := ValidateDownstream(testcontext.New(t), mock, operationStatusID, location, apiVersion)
		require.NoError(t, err)
		require.Equal(t, expectedURL, downstreamURL)
	})

	// All of the Radius RPs include a location in the operation status child resource.
	t.Run("success (operationstatuses with location)", func(t *testing.T) {
		mock := setup(t)
		mock.EXPECT().Get(gomock.Any(), idWithoutResourceGroup.PlaneScope()).Return(&database.Object{Data: plane}, nil).Times(1)
		mock.EXPECT().Get(gomock.Any(), locationResource.ID).Return(&database.Object{Data: locationResource}, nil).Times(1)

		operationStatusID := resources.MustParse("/planes/radius/local/providers/System.TestRP/locations/east/operationStatuses/abcd")

		expectedURL, err := url.Parse(downstream)
		require.NoError(t, err)

		downstreamURL, err := ValidateDownstream(testcontext.New(t), mock, operationStatusID, location, apiVersion)
		require.NoError(t, err)
		require.Equal(t, expectedURL, downstreamURL)
	})

	// The deployment engine models its operation result resources as child resources of the deployment resource.
	t.Run("success (operationresults as child resource)", func(t *testing.T) {
		mock := setup(t)
		mock.EXPECT().Get(gomock.Any(), idWithoutResourceGroup.PlaneScope()).Return(&database.Object{Data: plane}, nil).Times(1)
		mock.EXPECT().Get(gomock.Any(), locationResource.ID).Return(&database.Object{Data: locationResource}, nil).Times(1)

		operationResultID := resources.MustParse("/planes/radius/local/providers/System.TestRP/deployments/xzy/operationResults/abcd")

		expectedURL, err := url.Parse(downstream)
		require.NoError(t, err)

		downstreamURL, err := ValidateDownstream(testcontext.New(t), mock, operationResultID, location, apiVersion)
		require.NoError(t, err)
		require.Equal(t, expectedURL, downstreamURL)
	})

	// All of the Radius RPs include a location in the operation result child resource.
	t.Run("success (operationresults with location)", func(t *testing.T) {
		mock := setup(t)
		mock.EXPECT().Get(gomock.Any(), idWithoutResourceGroup.PlaneScope()).Return(&database.Object{Data: plane}, nil).Times(1)
		mock.EXPECT().Get(gomock.Any(), locationResource.ID).Return(&database.Object{Data: locationResource}, nil).Times(1)

		operationResultID := resources.MustParse("/planes/radius/local/providers/System.TestRP/locations/east/operationResults/abcd")

		expectedURL, err := url.Parse(downstream)
		require.NoError(t, err)

		downstreamURL, err := ValidateDownstream(testcontext.New(t), mock, operationResultID, location, apiVersion)
		require.NoError(t, err)
		require.Equal(t, expectedURL, downstreamURL)
	})

	t.Run("plane not found", func(t *testing.T) {
		mock := setup(t)
		mock.EXPECT().Get(gomock.Any(), id.PlaneScope()).Return(nil, &database.ErrNotFound{}).Times(1)

		downstreamURL, err := ValidateDownstream(testcontext.New(t), mock, id, location, apiVersion)
		require.Error(t, err)
		require.Equal(t, &NotFoundError{Message: "plane \"/planes/radius/local\" not found"}, err)
		require.Nil(t, downstreamURL)
	})

	t.Run("plane retreival failure", func(t *testing.T) {
		mock := setup(t)

		expected := fmt.Errorf("failed to fetch plane \"/planes/radius/local\": %w", errors.New("test error"))
		mock.EXPECT().Get(gomock.Any(), id.PlaneScope()).Return(nil, errors.New("test error")).Times(1)

		downstreamURL, err := ValidateDownstream(testcontext.New(t), mock, id, location, apiVersion)
		require.Error(t, err)
		require.Equal(t, expected, err)
		require.Nil(t, downstreamURL)
	})

	t.Run("resource group not found", func(t *testing.T) {
		mock := setup(t)
		mock.EXPECT().Get(gomock.Any(), id.PlaneScope()).Return(&database.Object{Data: plane}, nil).Times(1)
		mock.EXPECT().Get(gomock.Any(), id.RootScope()).Return(nil, &database.ErrNotFound{}).Times(1)

		downstreamURL, err := ValidateDownstream(testcontext.New(t), mock, id, location, apiVersion)
		require.Error(t, err)
		require.Equal(t, &NotFoundError{Message: "resource group \"/planes/radius/local/resourceGroups/test-group\" not found"}, err)
		require.Nil(t, downstreamURL)
	})

	t.Run("resource group err", func(t *testing.T) {
		mock := setup(t)

		mock.EXPECT().Get(gomock.Any(), id.PlaneScope()).Return(&database.Object{Data: plane}, nil).Times(1)
		mock.EXPECT().Get(gomock.Any(), id.RootScope()).Return(nil, errors.New("test error")).Times(1)

		downstreamURL, err := ValidateDownstream(testcontext.New(t), mock, id, location, apiVersion)
		require.Error(t, err)
		require.Equal(t, "failed to fetch resource group \"/planes/radius/local/resourceGroups/test-group\": test error", err.Error())
		require.Nil(t, downstreamURL)
	})

	t.Run("resource type error", func(t *testing.T) {
		resourceGroup := &datamodel.ResourceGroup{
			BaseResource: v1.BaseResource{
				TrackedResource: v1.TrackedResource{
					ID: id.RootScope(),
				},
			},
		}

		expected := fmt.Errorf("failed to fetch resource type %q: %w", "System.TestRP/testResources", errors.New("test error"))

		mock := setup(t)
		mock.EXPECT().Get(gomock.Any(), id.PlaneScope()).Return(&database.Object{Data: plane}, nil).Times(1)
		mock.EXPECT().Get(gomock.Any(), id.RootScope()).Return(&database.Object{Data: resourceGroup}, nil).Times(1)
		mock.EXPECT().Get(gomock.Any(), resourceTypeResource.ID).Return(nil, errors.New("test error")).Times(1)

		downstreamURL, err := ValidateDownstream(testcontext.New(t), mock, id, location, apiVersion)
		require.Error(t, err)
		require.Equal(t, expected, err)
		require.Nil(t, downstreamURL)
	})

	t.Run("location error", func(t *testing.T) {
		resourceGroup := &datamodel.ResourceGroup{
			BaseResource: v1.BaseResource{
				TrackedResource: v1.TrackedResource{
					ID: id.RootScope(),
				},
			},
		}

		expected := fmt.Errorf("failed to fetch location %q: %w", locationResource.ID, errors.New("test error"))

		mock := setup(t)
		mock.EXPECT().Get(gomock.Any(), id.PlaneScope()).Return(&database.Object{Data: plane}, nil).Times(1)
		mock.EXPECT().Get(gomock.Any(), id.RootScope()).Return(&database.Object{Data: resourceGroup}, nil).Times(1)
		mock.EXPECT().Get(gomock.Any(), resourceTypeResource.ID).Return(&database.Object{Data: resourceTypeID}, nil).Times(1)
		mock.EXPECT().Get(gomock.Any(), locationResource.ID).Return(nil, errors.New("test error")).Times(1)

		downstreamURL, err := ValidateDownstream(testcontext.New(t), mock, id, location, apiVersion)
		require.Error(t, err)
		require.Equal(t, expected, err)
		require.Nil(t, downstreamURL)
	})

	t.Run("resource type not found in location", func(t *testing.T) {
		resourceGroup := &datamodel.ResourceGroup{
			BaseResource: v1.BaseResource{
				TrackedResource: v1.TrackedResource{
					ID: id.RootScope(),
				},
			},
		}

		locationResource := &datamodel.Location{
			BaseResource: v1.BaseResource{
				TrackedResource: v1.TrackedResource{
					Name: location,
					ID:   locationResource.ID,
				},
			},
			Properties: datamodel.LocationProperties{
				Address: to.Ptr("http://localhost:7443"),
				ResourceTypes: map[string]datamodel.LocationResourceTypeConfiguration{
					"testResources2": {
						APIVersions: map[string]datamodel.LocationAPIVersionConfiguration{
							apiVersion: {},
						},
					},
				},
			},
		}

		mock := setup(t)
		mock.EXPECT().Get(gomock.Any(), id.PlaneScope()).Return(&database.Object{Data: plane}, nil).Times(1)
		mock.EXPECT().Get(gomock.Any(), id.RootScope()).Return(&database.Object{Data: resourceGroup}, nil).Times(1)
		mock.EXPECT().Get(gomock.Any(), resourceTypeResource.ID).Return(&database.Object{Data: resourceTypeID}, nil).Times(1)
		mock.EXPECT().Get(gomock.Any(), locationResource.ID).Return(&database.Object{Data: locationResource}, nil).Times(1)

		downstreamURL, err := ValidateDownstream(testcontext.New(t), mock, id, location, apiVersion)
		require.Error(t, err)
		require.Equal(t, &InvalidError{Message: "resource type \"System.TestRP/testResources\" not supported by location \"east\""}, err)
		require.Nil(t, downstreamURL)
	})

	t.Run("api-version not found in location", func(t *testing.T) {
		resourceGroup := &datamodel.ResourceGroup{
			BaseResource: v1.BaseResource{
				TrackedResource: v1.TrackedResource{
					ID: id.RootScope(),
				},
			},
		}

		locationResource := &datamodel.Location{
			BaseResource: v1.BaseResource{
				TrackedResource: v1.TrackedResource{
					Name: location,
					ID:   locationResource.ID,
				},
			},
			Properties: datamodel.LocationProperties{
				Address: to.Ptr("http://localhost:7443"),
				ResourceTypes: map[string]datamodel.LocationResourceTypeConfiguration{
					"testResources": {
						APIVersions: map[string]datamodel.LocationAPIVersionConfiguration{
							apiVersion + "-preview": {},
						},
					},
				},
			},
		}

		mock := setup(t)
		mock.EXPECT().Get(gomock.Any(), id.PlaneScope()).Return(&database.Object{Data: plane}, nil).Times(1)
		mock.EXPECT().Get(gomock.Any(), id.RootScope()).Return(&database.Object{Data: resourceGroup}, nil).Times(1)
		mock.EXPECT().Get(gomock.Any(), resourceTypeResource.ID).Return(&database.Object{Data: resourceTypeID}, nil).Times(1)
		mock.EXPECT().Get(gomock.Any(), locationResource.ID).Return(&database.Object{Data: locationResource}, nil).Times(1)

		downstreamURL, err := ValidateDownstream(testcontext.New(t), mock, id, location, apiVersion)
		require.Error(t, err)
		require.Equal(t, &InvalidError{Message: "api version \"2025-01-01\" is not supported for resource type \"System.TestRP/testResources\" by location \"east\""}, err)
		require.Nil(t, downstreamURL)
	})

	t.Run("location invalid URL", func(t *testing.T) {
		resourceGroup := &datamodel.ResourceGroup{
			BaseResource: v1.BaseResource{
				TrackedResource: v1.TrackedResource{
					ID: id.RootScope(),
				},
			},
		}

		locationResource := &datamodel.Location{
			BaseResource: v1.BaseResource{
				TrackedResource: v1.TrackedResource{
					Name: location,
					ID:   locationResource.ID,
				},
			},
			Properties: datamodel.LocationProperties{
				Address: to.Ptr("\ninvalid"),
				ResourceTypes: map[string]datamodel.LocationResourceTypeConfiguration{
					"testResources": {
						APIVersions: map[string]datamodel.LocationAPIVersionConfiguration{
							apiVersion: {},
						},
					},
				},
			},
		}

		mock := setup(t)
		mock.EXPECT().Get(gomock.Any(), id.PlaneScope()).Return(&database.Object{Data: plane}, nil).Times(1)
		mock.EXPECT().Get(gomock.Any(), id.RootScope()).Return(&database.Object{Data: resourceGroup}, nil).Times(1)
		mock.EXPECT().Get(gomock.Any(), resourceTypeResource.ID).Return(&database.Object{Data: resourceTypeID}, nil).Times(1)
		mock.EXPECT().Get(gomock.Any(), locationResource.ID).Return(&database.Object{Data: locationResource}, nil).Times(1)

		downstreamURL, err := ValidateDownstream(testcontext.New(t), mock, id, location, apiVersion)
		require.Error(t, err)
		require.Equal(t, &InvalidError{Message: "failed to parse location address: parse \"\\ninvalid\": net/url: invalid control character in URL"}, err)
		require.Nil(t, downstreamURL)
	})
}

// This test validates the pre-UDT before where resource providers are registered as part of the plane.
// This can be deleted once the legacy routing behavior is removed.
func Test_ValidateDownstream_Legacy(t *testing.T) {
	id, err := resources.ParseResource("/planes/radius/local/resourceGroups/test-group/providers/System.TestRP/testResources/name")
	require.NoError(t, err)

	idWithoutResourceGroup, err := resources.Parse("/planes/radius/local/providers/System.TestRP/testResources")
	require.NoError(t, err)

	resourceTypeID, err := datamodel.ResourceTypeIDFromResourceID(id)
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

	setup := func(t *testing.T) *database.MockClient {
		ctrl := gomock.NewController(t)
		return database.NewMockClient(ctrl)
	}

	t.Run("success (resource group)", func(t *testing.T) {
		resourceGroup := &datamodel.ResourceGroup{
			BaseResource: v1.BaseResource{
				TrackedResource: v1.TrackedResource{
					ID: id.RootScope(),
				},
			},
		}

		mock := setup(t)
		mock.EXPECT().Get(gomock.Any(), id.PlaneScope()).Return(&database.Object{Data: plane}, nil).Times(1)
		mock.EXPECT().Get(gomock.Any(), id.RootScope()).Return(&database.Object{Data: resourceGroup}, nil).Times(1)
		mock.EXPECT().Get(gomock.Any(), resourceTypeID.String()).Return(nil, &database.ErrNotFound{}).Times(1)

		expectedURL, err := url.Parse(downstream)
		require.NoError(t, err)

		downstreamURL, err := ValidateDownstream(testcontext.New(t), mock, id, location, apiVersion)
		require.NoError(t, err)
		require.Equal(t, expectedURL, downstreamURL)
	})

	t.Run("success (non resource group)", func(t *testing.T) {
		mock := setup(t)
		mock.EXPECT().Get(gomock.Any(), idWithoutResourceGroup.PlaneScope()).Return(&database.Object{Data: plane}, nil).Times(1)
		mock.EXPECT().Get(gomock.Any(), resourceTypeID.String()).Return(nil, &database.ErrNotFound{}).Times(1)

		expectedURL, err := url.Parse(downstream)
		require.NoError(t, err)

		downstreamURL, err := ValidateDownstream(testcontext.New(t), mock, idWithoutResourceGroup, location, apiVersion)
		require.NoError(t, err)
		require.Equal(t, expectedURL, downstreamURL)
	})

	t.Run("plane not found", func(t *testing.T) {
		mock := setup(t)
		mock.EXPECT().Get(gomock.Any(), id.PlaneScope()).Return(nil, &database.ErrNotFound{}).Times(1)

		downstreamURL, err := ValidateDownstream(testcontext.New(t), mock, id, location, apiVersion)
		require.Error(t, err)
		require.Equal(t, &NotFoundError{Message: "plane \"/planes/radius/local\" not found"}, err)
		require.Nil(t, downstreamURL)
	})

	t.Run("plane retrieval failure", func(t *testing.T) {
		mock := setup(t)

		expected := fmt.Errorf("failed to fetch plane \"/planes/radius/local\": %w", errors.New("test error"))
		mock.EXPECT().Get(gomock.Any(), id.PlaneScope()).Return(nil, errors.New("test error")).Times(1)

		downstreamURL, err := ValidateDownstream(testcontext.New(t), mock, id, location, apiVersion)
		require.Error(t, err)
		require.Equal(t, expected, err)
		require.Nil(t, downstreamURL)
	})

	t.Run("resource group not found", func(t *testing.T) {
		mock := setup(t)
		mock.EXPECT().Get(gomock.Any(), id.PlaneScope()).Return(&database.Object{Data: plane}, nil).Times(1)
		mock.EXPECT().Get(gomock.Any(), id.RootScope()).Return(nil, &database.ErrNotFound{}).Times(1)

		downstreamURL, err := ValidateDownstream(testcontext.New(t), mock, id, location, apiVersion)
		require.Error(t, err)
		require.Equal(t, &NotFoundError{Message: "resource group \"/planes/radius/local/resourceGroups/test-group\" not found"}, err)
		require.Nil(t, downstreamURL)
	})

	t.Run("resource group err", func(t *testing.T) {
		mock := setup(t)

		mock.EXPECT().Get(gomock.Any(), id.PlaneScope()).Return(&database.Object{Data: plane}, nil).Times(1)
		mock.EXPECT().Get(gomock.Any(), id.RootScope()).Return(nil, errors.New("test error")).Times(1)

		downstreamURL, err := ValidateDownstream(testcontext.New(t), mock, id, location, apiVersion)
		require.Error(t, err)
		require.Equal(t, "failed to fetch resource group \"/planes/radius/local/resourceGroups/test-group\": test error", err.Error())
		require.Nil(t, downstreamURL)
	})

	t.Run("legacy resource provider not configured", func(t *testing.T) {
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
		mock.EXPECT().Get(gomock.Any(), id.PlaneScope()).Return(&database.Object{Data: plane}, nil).Times(1)
		mock.EXPECT().Get(gomock.Any(), id.RootScope()).Return(&database.Object{Data: resourceGroup}, nil).Times(1)
		mock.EXPECT().Get(gomock.Any(), resourceTypeID.String()).Return(nil, &database.ErrNotFound{}).Times(1)

		downstreamURL, err := ValidateDownstream(testcontext.New(t), mock, id, location, apiVersion)
		require.Error(t, err)
		require.Equal(t, &InvalidError{Message: "resource provider System.TestRP not configured"}, err)
		require.Nil(t, downstreamURL)
	})

	t.Run("location not found", func(t *testing.T) {
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
		mock.EXPECT().Get(gomock.Any(), id.PlaneScope()).Return(&database.Object{Data: plane}, nil).Times(1)
		mock.EXPECT().Get(gomock.Any(), id.RootScope()).Return(&database.Object{Data: resourceGroup}, nil).Times(1)
		mock.EXPECT().Get(gomock.Any(), resourceTypeID.String()).Return(nil, &database.ErrNotFound{}).Times(1)

		downstreamURL, err := ValidateDownstream(testcontext.New(t), mock, id, location, apiVersion)
		require.Error(t, err)
		require.Equal(t, &InvalidError{Message: "resource provider System.TestRP not configured"}, err)
		require.Nil(t, downstreamURL)
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
		mock.EXPECT().Get(gomock.Any(), id.PlaneScope()).Return(&database.Object{Data: plane}, nil).Times(1)
		mock.EXPECT().Get(gomock.Any(), id.RootScope()).Return(&database.Object{Data: resourceGroup}, nil).Times(1)
		mock.EXPECT().Get(gomock.Any(), resourceTypeID.String()).Return(nil, &database.ErrNotFound{}).Times(1)

		downstreamURL, err := ValidateDownstream(testcontext.New(t), mock, id, location, apiVersion)
		require.Error(t, err)
		require.Equal(t, &InvalidError{Message: "failed to parse downstream URL: parse \"\\ninvalid\": net/url: invalid control character in URL"}, err)
		require.Nil(t, downstreamURL)
	})
}
