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

package radius

import (
	"context"
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/armrpc/asyncoperation/statusmanager"
	"github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	"github.com/radius-project/radius/pkg/armrpc/rest"
	"github.com/radius-project/radius/pkg/components/database"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/pkg/ucp/datamodel"
	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/radius-project/radius/pkg/ucp/trackedresource"
	"github.com/radius-project/radius/test/testcontext"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

const (
	apiVersion = "2025-01-01"
	location   = "global"
)

// The Run function is also tested by integration tests in the pkg/ucp/integrationtests/radius package.

func createController(t *testing.T) (*ProxyController, *database.MockClient, *mockUpdater, *mockRoundTripper, *statusmanager.MockStatusManager) {
	ctrl := gomock.NewController(t)
	databaseClient := database.NewMockClient(ctrl)
	statusManager := statusmanager.NewMockStatusManager(ctrl)

	roundTripper := mockRoundTripper{}

	p, err := NewProxyController(
		controller.Options{DatabaseClient: databaseClient, StatusManager: statusManager},
		&roundTripper,
		"http://localhost:1234")
	require.NoError(t, err)

	updater := mockUpdater{}

	pc := p.(*ProxyController)
	pc.updater = &updater

	return pc, databaseClient, &updater, &roundTripper, statusManager
}

func Test_Run(t *testing.T) {
	id := resources.MustParse("/planes/test/local/resourceGroups/test-rg/providers/Applications.Test/testResources/my-resource")

	resourceTypeID, err := datamodel.ResourceTypeIDFromResourceID(id)
	require.NoError(t, err)

	locationID, err := datamodel.ResourceProviderLocationIDFromResourceID(id, "global")
	require.NoError(t, err)

	plane := datamodel.RadiusPlane{
		Properties: datamodel.RadiusPlaneProperties{
			ResourceProviders: map[string]string{
				"Applications.Test": "https://localhost:1234",
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
				Name: "global",
				ID:   locationID.String(),
			},
		},
		Properties: datamodel.LocationProperties{
			Address: to.Ptr("https://localhost:1234"),
			ResourceTypes: map[string]datamodel.LocationResourceTypeConfiguration{
				"testResources": {
					APIVersions: map[string]datamodel.LocationAPIVersionConfiguration{
						"2025-01-01": {},
					},
				},
			},
		},
	}

	t.Run("success (non-tracked)", func(t *testing.T) {
		p, databaseClient, _, roundTripper, _ := createController(t)

		svcContext := &v1.ARMRequestContext{
			APIVersion: apiVersion,
			ResourceID: id,
		}
		ctx := testcontext.New(t)
		ctx = v1.WithARMRequestContext(ctx, svcContext)

		w := httptest.NewRecorder()

		// Not a mutating request
		req := httptest.NewRequest(http.MethodGet, id.String()+"?api-version="+apiVersion, nil)

		databaseClient.EXPECT().
			Get(gomock.Any(), id.PlaneScope(), gomock.Any()).
			Return(&database.Object{Data: plane}, nil).Times(1)

		databaseClient.EXPECT().
			Get(gomock.Any(), resourceTypeID.String(), gomock.Any()).
			Return(&database.Object{Data: resourceTypeResource}, nil).Times(1)

		databaseClient.EXPECT().
			Get(gomock.Any(), id.RootScope(), gomock.Any()).
			Return(&database.Object{Data: resourceGroup}, nil).Times(1)

		databaseClient.EXPECT().
			Get(gomock.Any(), locationResource.ID).
			Return(&database.Object{Data: locationResource}, nil).Times(1)

		downstreamResponse := httptest.NewRecorder()
		downstreamResponse.WriteHeader(http.StatusOK)
		roundTripper.Response = downstreamResponse.Result()

		response, err := p.Run(ctx, w, req.WithContext(ctx))
		require.NoError(t, err)
		require.Nil(t, response)
	})

	t.Run("success (tracked terminal response)", func(t *testing.T) {
		p, databaseClient, updater, roundTripper, _ := createController(t)

		svcContext := &v1.ARMRequestContext{
			APIVersion: apiVersion,
			ResourceID: id,
		}
		ctx := testcontext.New(t)
		ctx = v1.WithARMRequestContext(ctx, svcContext)

		w := httptest.NewRecorder()

		// Mutating request that will complete synchronously
		req := httptest.NewRequest(http.MethodDelete, id.String()+"?api-version="+apiVersion, nil)

		databaseClient.EXPECT().
			Get(gomock.Any(), id.PlaneScope(), gomock.Any()).
			Return(&database.Object{Data: plane}, nil).Times(1)

		databaseClient.EXPECT().
			Get(gomock.Any(), resourceTypeID.String(), gomock.Any()).
			Return(&database.Object{Data: resourceTypeResource}, nil).Times(1)

		databaseClient.EXPECT().
			Get(gomock.Any(), id.RootScope(), gomock.Any()).
			Return(&database.Object{Data: resourceGroup}, nil).Times(1)

		databaseClient.EXPECT().
			Get(gomock.Any(), locationResource.ID).
			Return(&database.Object{Data: locationResource}, nil).Times(1)

		downstreamResponse := httptest.NewRecorder()
		downstreamResponse.WriteHeader(http.StatusOK)
		roundTripper.Response = downstreamResponse.Result()

		// Successful update
		updater.Result = nil

		response, err := p.Run(ctx, w, req.WithContext(ctx))
		require.NoError(t, err)
		require.Nil(t, response)
	})

	t.Run("success (fallback to async)", func(t *testing.T) {
		p, databaseClient, updater, roundTripper, statusManager := createController(t)

		svcContext := &v1.ARMRequestContext{
			APIVersion: apiVersion,
			ResourceID: id,
		}
		ctx := testcontext.New(t)
		ctx = v1.WithARMRequestContext(ctx, svcContext)

		w := httptest.NewRecorder()

		// Mutating request that will complete synchronously
		req := httptest.NewRequest(http.MethodDelete, id.String()+"?api-version="+apiVersion, nil)

		databaseClient.EXPECT().
			Get(gomock.Any(), id.PlaneScope(), gomock.Any()).
			Return(&database.Object{Data: plane}, nil).Times(1)

		databaseClient.EXPECT().
			Get(gomock.Any(), resourceTypeID.String(), gomock.Any()).
			Return(&database.Object{Data: resourceTypeResource}, nil).Times(1)

		databaseClient.EXPECT().
			Get(gomock.Any(), id.RootScope(), gomock.Any()).
			Return(&database.Object{Data: resourceGroup}, nil).Times(1)

		databaseClient.EXPECT().
			Get(gomock.Any(), locationResource.ID).
			Return(&database.Object{Data: locationResource}, nil).Times(1)

		// Tracking entry created
		databaseClient.EXPECT().
			Get(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil, &database.ErrNotFound{}).Times(1)
		databaseClient.EXPECT().
			Save(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil).Times(1)

		downstreamResponse := httptest.NewRecorder()
		downstreamResponse.WriteHeader(http.StatusOK)
		roundTripper.Response = downstreamResponse.Result()

		// Contended update, fallback to async
		updater.Result = &trackedresource.InProgressErr{}

		statusManager.EXPECT().
			QueueAsyncOperation(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil).Times(1)

		response, err := p.Run(ctx, w, req.WithContext(ctx))
		require.NoError(t, err)
		require.Nil(t, response)
	})

	t.Run("success (fallback to async without workitem)", func(t *testing.T) {
		p, databaseClient, updater, roundTripper, _ := createController(t)

		svcContext := &v1.ARMRequestContext{
			APIVersion: apiVersion,
			ResourceID: id,
		}
		ctx := testcontext.New(t)
		ctx = v1.WithARMRequestContext(ctx, svcContext)

		w := httptest.NewRecorder()

		// Mutating request that will complete asynchronously
		req := httptest.NewRequest(http.MethodDelete, id.String()+"?api-version="+apiVersion, nil)

		databaseClient.EXPECT().
			Get(gomock.Any(), id.PlaneScope(), gomock.Any()).
			Return(&database.Object{Data: plane}, nil).Times(1)

		databaseClient.EXPECT().
			Get(gomock.Any(), resourceTypeID.String(), gomock.Any()).
			Return(&database.Object{Data: resourceTypeResource}, nil).Times(1)

		databaseClient.EXPECT().
			Get(gomock.Any(), id.RootScope(), gomock.Any()).
			Return(&database.Object{Data: resourceGroup}, nil).Times(1)

		databaseClient.EXPECT().
			Get(gomock.Any(), locationResource.ID).
			Return(&database.Object{Data: locationResource}, nil).Times(1)

		// Tracking entry created
		existingEntry := &database.Object{
			Data: &datamodel.GenericResource{
				BaseResource: v1.BaseResource{
					InternalMetadata: v1.InternalMetadata{
						AsyncProvisioningState: v1.ProvisioningStateAccepted,
					},
				},
			},
		}
		databaseClient.EXPECT().
			Get(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(existingEntry, nil).Times(1)
		databaseClient.EXPECT().
			Save(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil).Times(1)

		downstreamResponse := httptest.NewRecorder()
		downstreamResponse.WriteHeader(http.StatusAccepted)
		roundTripper.Response = downstreamResponse.Result()

		// Contended update, fallback to async
		updater.Result = &trackedresource.InProgressErr{}

		// No work item created, it was already in the queue.

		response, err := p.Run(ctx, w, req.WithContext(ctx))
		require.NoError(t, err)
		require.Nil(t, response)
	})

	t.Run("failure (validate downstream: not found)", func(t *testing.T) {
		p, databaseClient, _, _, _ := createController(t)

		svcContext := &v1.ARMRequestContext{
			APIVersion: apiVersion,
			ResourceID: id,
		}
		ctx := testcontext.New(t)
		ctx = v1.WithARMRequestContext(ctx, svcContext)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, id.String()+"?api-version="+apiVersion, nil)

		databaseClient.EXPECT().
			Get(gomock.Any(), "/planes/"+id.PlaneNamespace(), gomock.Any()).
			Return(nil, &database.ErrNotFound{}).Times(1)

		expected := rest.NewNotFoundResponseWithCause(id, "plane \"/planes/test/local\" not found")

		response, err := p.Run(ctx, w, req)
		require.NoError(t, err)
		require.Equal(t, expected, response)
	})
}

func Test_ProxyController_PrepareProxyRequest(t *testing.T) {
	downstream := "http://localhost:7443"
	relativePath := "/planes/radius/local/resourceGroups/test-group/providers/System.TestRP"
	t.Run("success (http)", func(t *testing.T) {
		originalURL, err := url.Parse("http://localhost:9443/path/base/planes/radius/local/resourceGroups/test-group/providers/System.TestRP?test=yes")
		require.NoError(t, err)
		originalReq := &http.Request{
			Host:   originalURL.Host,
			Header: http.Header{"Copied": []string{"yes"}},
			TLS:    nil,
			URL:    originalURL}

		p, _, _, _, _ := createController(t)
		proxyReq, err := p.PrepareProxyRequest(testcontext.New(t), originalReq, downstream, relativePath)
		require.NoError(t, err)
		require.NotNil(t, proxyReq)

		require.Equal(t, "http://localhost:7443/planes/radius/local/resourceGroups/test-group/providers/System.TestRP?test=yes", proxyReq.URL.String())
		require.Equal(t, "http", proxyReq.Header.Get("X-Forwarded-Proto"))
		require.Equal(t, "http://localhost:9443/path/base/planes/radius/local/resourceGroups/test-group/providers/System.TestRP?test=yes", proxyReq.Header.Get("Referer"))
		require.Equal(t, "yes", proxyReq.Header.Get("Copied"))
	})

	t.Run("success (http)", func(t *testing.T) {
		originalURL, err := url.Parse("http://localhost:9443/path/base/planes/radius/local/resourceGroups/test-group/providers/System.TestRP?test=yes")
		require.NoError(t, err)
		originalReq := &http.Request{
			Host:   originalURL.Host,
			Header: http.Header{"Copied": []string{"yes"}},
			TLS:    &tls.ConnectionState{},
			URL:    originalURL}

		p, _, _, _, _ := createController(t)
		proxyReq, err := p.PrepareProxyRequest(testcontext.New(t), originalReq, downstream, relativePath)
		require.NoError(t, err)
		require.NotNil(t, proxyReq)

		require.Equal(t, "http://localhost:7443/planes/radius/local/resourceGroups/test-group/providers/System.TestRP?test=yes", proxyReq.URL.String())
		require.Equal(t, "https", proxyReq.Header.Get("X-Forwarded-Proto"))
		require.Equal(t, "https://localhost:9443/path/base/planes/radius/local/resourceGroups/test-group/providers/System.TestRP?test=yes", proxyReq.Header.Get("Referer"))
		require.Equal(t, "yes", proxyReq.Header.Get("Copied"))
	})

	t.Run("invalid downstream URL", func(t *testing.T) {
		originalReq := &http.Request{Header: http.Header{}, URL: &url.URL{}}

		p, _, _, _, _ := createController(t)
		proxyReq, err := p.PrepareProxyRequest(testcontext.New(t), originalReq, "\ninvalid", relativePath)
		require.Error(t, err)
		require.Equal(t, "failed to parse downstream URL: parse \"\\ninvalid\": net/url: invalid control character in URL", err.Error())
		require.Nil(t, proxyReq)
	})
}

type mockUpdater struct {
	Result error
}

func (u *mockUpdater) Update(ctx context.Context, downstreamURL string, originalID resources.ID, version string) error {
	return u.Result
}

type mockRoundTripper struct {
	Response *http.Response
	Err      error
}

func (rt *mockRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	if rt.Response != nil {
		rt.Response.Request = r
	}
	return rt.Response, rt.Err
}
