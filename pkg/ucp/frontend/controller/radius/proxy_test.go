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

	"github.com/golang/mock/gomock"
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/armrpc/asyncoperation/statusmanager"
	"github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	"github.com/radius-project/radius/pkg/armrpc/rest"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/ucp/datamodel"
	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/radius-project/radius/pkg/ucp/store"
	"github.com/radius-project/radius/pkg/ucp/trackedresource"
	"github.com/radius-project/radius/test/testcontext"
	"github.com/stretchr/testify/require"
)

// The Run function is also tested by integration tests in the pkg/ucp/integrationtests/radius package.

func createController(t *testing.T) (*ProxyController, *store.MockStorageClient, *mockUpdater, *mockRoundTripper, *statusmanager.MockStatusManager) {
	ctrl := gomock.NewController(t)
	storageClient := store.NewMockStorageClient(ctrl)
	statusManager := statusmanager.NewMockStatusManager(ctrl)

	p, err := NewProxyController(controller.Options{StorageClient: storageClient, StatusManager: statusManager})
	require.NoError(t, err)

	updater := mockUpdater{}
	roundTripper := mockRoundTripper{}

	pc := p.(*ProxyController)
	pc.updater = &updater
	pc.transport = &roundTripper

	return pc, storageClient, &updater, &roundTripper, statusManager
}

func Test_Run(t *testing.T) {
	id := resources.MustParse("/planes/test/local/resourceGroups/test-rg/providers/Applications.Test/testResources/my-resource")

	plane := datamodel.Plane{
		Properties: datamodel.PlaneProperties{
			Kind: datamodel.PlaneKind(v20231001preview.PlaneKindUCPNative),
			ResourceProviders: map[string]*string{
				"Applications.Test": to.Ptr("https://localhost:1234"),
			},
		},
	}
	resourceGroup := datamodel.ResourceGroup{}

	t.Run("success (non-tracked)", func(t *testing.T) {
		p, storageClient, _, roundTripper, _ := createController(t)

		svcContext := &v1.ARMRequestContext{
			ResourceID: id,
		}
		ctx := testcontext.New(t)
		ctx = v1.WithARMRequestContext(ctx, svcContext)

		w := httptest.NewRecorder()

		// Not a mutating request
		req := httptest.NewRequest(http.MethodGet, id.String(), nil)

		storageClient.EXPECT().
			Get(gomock.Any(), "/planes/"+id.PlaneNamespace(), gomock.Any()).
			Return(&store.Object{Data: plane}, nil).Times(1)

		storageClient.EXPECT().
			Get(gomock.Any(), id.RootScope(), gomock.Any()).
			Return(&store.Object{Data: resourceGroup}, nil).Times(1)

		downstreamResponse := httptest.NewRecorder()
		downstreamResponse.WriteHeader(http.StatusOK)
		roundTripper.Response = downstreamResponse.Result()

		response, err := p.Run(ctx, w, req.WithContext(ctx))
		require.NoError(t, err)
		require.Nil(t, response)
	})

	t.Run("success (tracked terminal response)", func(t *testing.T) {
		p, storageClient, updater, roundTripper, _ := createController(t)

		svcContext := &v1.ARMRequestContext{
			ResourceID: id,
		}
		ctx := testcontext.New(t)
		ctx = v1.WithARMRequestContext(ctx, svcContext)

		w := httptest.NewRecorder()

		// Mutating request that will complete synchronously
		req := httptest.NewRequest(http.MethodDelete, id.String(), nil)

		storageClient.EXPECT().
			Get(gomock.Any(), "/planes/"+id.PlaneNamespace(), gomock.Any()).
			Return(&store.Object{Data: plane}, nil).Times(1)

		storageClient.EXPECT().
			Get(gomock.Any(), id.RootScope(), gomock.Any()).
			Return(&store.Object{Data: resourceGroup}, nil).Times(1)

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
		p, storageClient, updater, roundTripper, statusManager := createController(t)

		svcContext := &v1.ARMRequestContext{
			ResourceID: id,
		}
		ctx := testcontext.New(t)
		ctx = v1.WithARMRequestContext(ctx, svcContext)

		w := httptest.NewRecorder()

		// Mutating request that will complete synchronously
		req := httptest.NewRequest(http.MethodDelete, id.String(), nil)

		storageClient.EXPECT().
			Get(gomock.Any(), "/planes/"+id.PlaneNamespace(), gomock.Any()).
			Return(&store.Object{Data: plane}, nil).Times(1)

		storageClient.EXPECT().
			Get(gomock.Any(), id.RootScope(), gomock.Any()).
			Return(&store.Object{Data: resourceGroup}, nil).Times(1)

		// Tracking entry created
		storageClient.EXPECT().
			Get(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil, &store.ErrNotFound{}).Times(1)
		storageClient.EXPECT().
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
		p, storageClient, updater, roundTripper, _ := createController(t)

		svcContext := &v1.ARMRequestContext{
			ResourceID: id,
		}
		ctx := testcontext.New(t)
		ctx = v1.WithARMRequestContext(ctx, svcContext)

		w := httptest.NewRecorder()

		// Mutating request that will complete asynchronously
		req := httptest.NewRequest(http.MethodDelete, id.String(), nil)

		storageClient.EXPECT().
			Get(gomock.Any(), "/planes/"+id.PlaneNamespace(), gomock.Any()).
			Return(&store.Object{Data: plane}, nil).Times(1)

		storageClient.EXPECT().
			Get(gomock.Any(), id.RootScope(), gomock.Any()).
			Return(&store.Object{Data: resourceGroup}, nil).Times(1)

		// Tracking entry created
		existingEntry := &store.Object{
			Data: &datamodel.GenericResource{
				BaseResource: v1.BaseResource{
					InternalMetadata: v1.InternalMetadata{
						AsyncProvisioningState: v1.ProvisioningStateAccepted,
					},
				},
			},
		}
		storageClient.EXPECT().
			Get(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(existingEntry, nil).Times(1)
		storageClient.EXPECT().
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
		p, storageClient, _, _, _ := createController(t)

		svcContext := &v1.ARMRequestContext{
			ResourceID: id,
		}
		ctx := testcontext.New(t)
		ctx = v1.WithARMRequestContext(ctx, svcContext)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, id.String(), nil)

		storageClient.EXPECT().
			Get(gomock.Any(), "/planes/"+id.PlaneNamespace(), gomock.Any()).
			Return(nil, &store.ErrNotFound{}).Times(1)

		expected := rest.NewNotFoundResponse(id)

		response, err := p.Run(ctx, w, req)
		require.NoError(t, err)
		require.Equal(t, expected, response)
	})

	t.Run("failure (validate downstream: invalid downstream)", func(t *testing.T) {
		p, storageClient, _, _, _ := createController(t)

		svcContext := &v1.ARMRequestContext{
			ResourceID: id,
		}
		ctx := testcontext.New(t)
		ctx = v1.WithARMRequestContext(ctx, svcContext)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, id.String(), nil)

		storageClient.EXPECT().
			Get(gomock.Any(), "/planes/"+id.PlaneNamespace(), gomock.Any()).
			Return(&store.Object{Data: datamodel.Plane{}}, nil).Times(1)

		expected := rest.NewBadRequestARMResponse(v1.ErrorResponse{Error: v1.ErrorDetails{Code: v1.CodeInvalid, Message: "unexpected plane type ", Target: id.String()}})
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
