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

	"github.com/golang/mock/gomock"
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/pkg/ucp/datamodel"
	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/radius-project/radius/pkg/ucp/rest"
	"github.com/radius-project/radius/pkg/ucp/store"
	"github.com/radius-project/radius/test/testcontext"
	"github.com/stretchr/testify/require"
)

func Test_ValidateDownstream(t *testing.T) {
	id, err := resources.ParseResource("/planes/radius/local/resourceGroups/test-group/providers/System.TestRP/testResources/name")
	require.NoError(t, err)

	idWithoutResourceGroup, err := resources.Parse("/planes/radius/local/providers/System.TestRP/testResources")
	require.NoError(t, err)

	downstream := "http://localhost:7443"

	plane := &datamodel.Plane{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID: id.PlaneScope(),
			},
		},
		Properties: datamodel.PlaneProperties{
			Kind: rest.PlaneKindUCPNative,
			ResourceProviders: map[string]*string{
				"System.TestRP": to.Ptr(downstream),
			},
		},
	}

	setup := func(t *testing.T) *store.MockStorageClient {
		ctrl := gomock.NewController(t)
		return store.NewMockStorageClient(ctrl)
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
		mock.EXPECT().Get(gomock.Any(), id.PlaneScope()).Return(&store.Object{Data: plane}, nil).Times(1)
		mock.EXPECT().Get(gomock.Any(), id.RootScope()).Return(&store.Object{Data: resourceGroup}, nil).Times(1)

		expectedURL, err := url.Parse(downstream)
		require.NoError(t, err)

		downstreamURL, err := ValidateDownstream(testcontext.New(t), mock, id)
		require.NoError(t, err)
		require.Equal(t, expectedURL, downstreamURL)
	})

	t.Run("success (non resource group)", func(t *testing.T) {
		mock := setup(t)
		mock.EXPECT().Get(gomock.Any(), idWithoutResourceGroup.PlaneScope()).Return(&store.Object{Data: plane}, nil).Times(1)

		expectedURL, err := url.Parse(downstream)
		require.NoError(t, err)

		downstreamURL, err := ValidateDownstream(testcontext.New(t), mock, idWithoutResourceGroup)
		require.NoError(t, err)
		require.Equal(t, expectedURL, downstreamURL)
	})

	t.Run("plane not found", func(t *testing.T) {
		mock := setup(t)
		mock.EXPECT().Get(gomock.Any(), id.PlaneScope()).Return(nil, &store.ErrNotFound{}).Times(1)

		downstreamURL, err := ValidateDownstream(testcontext.New(t), mock, id)
		require.Error(t, err)
		require.Equal(t, &NotFoundError{Message: "plane \"/planes/radius/local\" not found"}, err)
		require.Nil(t, downstreamURL)
	})

	t.Run("plane retreival failure", func(t *testing.T) {
		mock := setup(t)

		expected := fmt.Errorf("failed to find plane \"/planes/radius/local\": %w", errors.New("test error"))
		mock.EXPECT().Get(gomock.Any(), id.PlaneScope()).Return(nil, errors.New("test error")).Times(1)

		downstreamURL, err := ValidateDownstream(testcontext.New(t), mock, id)
		require.Error(t, err)
		require.Equal(t, expected, err)
		require.Nil(t, downstreamURL)
	})

	t.Run("resource group not found", func(t *testing.T) {
		mock := setup(t)
		mock.EXPECT().Get(gomock.Any(), id.PlaneScope()).Return(&store.Object{Data: plane}, nil).Times(1)
		mock.EXPECT().Get(gomock.Any(), id.RootScope()).Return(nil, &store.ErrNotFound{}).Times(1)

		downstreamURL, err := ValidateDownstream(testcontext.New(t), mock, id)
		require.Error(t, err)
		require.Equal(t, &NotFoundError{Message: "resource group \"/planes/radius/local/resourceGroups/test-group\" not found"}, err)
		require.Nil(t, downstreamURL)
	})

	t.Run("resource group err", func(t *testing.T) {
		mock := setup(t)

		mock.EXPECT().Get(gomock.Any(), id.PlaneScope()).Return(&store.Object{Data: plane}, nil).Times(1)
		mock.EXPECT().Get(gomock.Any(), id.RootScope()).Return(nil, errors.New("test error")).Times(1)

		downstreamURL, err := ValidateDownstream(testcontext.New(t), mock, id)
		require.Error(t, err)
		require.Equal(t, "failed to find resource group \"/planes/radius/local/resourceGroups/test-group\": test error", err.Error())
		require.Nil(t, downstreamURL)
	})

	t.Run("resource provider not found", func(t *testing.T) {
		plane := &datamodel.Plane{
			BaseResource: v1.BaseResource{
				TrackedResource: v1.TrackedResource{
					ID: id.PlaneScope(),
				},
			},
			Properties: datamodel.PlaneProperties{
				Kind:              rest.PlaneKindUCPNative,
				ResourceProviders: map[string]*string{},
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
		mock.EXPECT().Get(gomock.Any(), id.PlaneScope()).Return(&store.Object{Data: plane}, nil).Times(1)
		mock.EXPECT().Get(gomock.Any(), id.RootScope()).Return(&store.Object{Data: resourceGroup}, nil).Times(1)

		downstreamURL, err := ValidateDownstream(testcontext.New(t), mock, id)
		require.Error(t, err)
		require.Equal(t, &InvalidError{Message: "resource provider System.TestRP not configured"}, err)
		require.Nil(t, downstreamURL)
	})

	t.Run("resource provider invalid URL", func(t *testing.T) {
		plane := &datamodel.Plane{
			BaseResource: v1.BaseResource{
				TrackedResource: v1.TrackedResource{
					ID: id.PlaneScope(),
				},
			},
			Properties: datamodel.PlaneProperties{
				Kind: rest.PlaneKindUCPNative,
				ResourceProviders: map[string]*string{
					"System.TestRP": to.Ptr("\ninvalid"),
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
		mock.EXPECT().Get(gomock.Any(), id.PlaneScope()).Return(&store.Object{Data: plane}, nil).Times(1)
		mock.EXPECT().Get(gomock.Any(), id.RootScope()).Return(&store.Object{Data: resourceGroup}, nil).Times(1)

		downstreamURL, err := ValidateDownstream(testcontext.New(t), mock, id)
		require.Error(t, err)
		require.Equal(t, &InvalidError{Message: "failed to parse downstream URL: parse \"\\ninvalid\": net/url: invalid control character in URL"}, err)
		require.Nil(t, downstreamURL)
	})
}
