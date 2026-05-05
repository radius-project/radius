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

package applications

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	ctrl "github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	"github.com/radius-project/radius/pkg/armrpc/rest"
	"github.com/radius-project/radius/pkg/armrpc/rpctest"
	"github.com/radius-project/radius/pkg/components/database"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	"github.com/radius-project/radius/pkg/sdk"
	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestGetGraphRun_20231001Preview(t *testing.T) {
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()

	databaseClient := database.NewMockClient(mctrl)
	req, err := rpctest.NewHTTPRequestWithContent(
		context.Background(),
		v1.OperationPost.HTTPMethod(),
		"http://localhost:8080/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/Applications/myapp/getGraph?api-version=2023-10-01-preview", nil)

	require.NoError(t, err)

	t.Run("resource not found", func(t *testing.T) {
		databaseClient.
			EXPECT().
			Get(gomock.Any(), gomock.Any()).
			Return(nil, &database.ErrNotFound{})
		ctx := rpctest.NewARMRequestContext(req)
		opts := ctrl.Options{
			DatabaseClient: databaseClient,
		}

		conn, err := sdk.NewDirectConnection("http://localhost:9000/apis/api.ucp.dev/v1alpha3")
		require.NoError(t, err)

		ctl, err := NewGetGraph(opts, conn)
		require.NoError(t, err)

		w := httptest.NewRecorder()
		resp, err := ctl.Run(ctx, w, req)
		require.NoError(t, err)
		err = resp.Apply(ctx, w, req)
		require.NoError(t, err)
		require.Equal(t, 404, w.Result().StatusCode)
		require.NoError(t, err)
	})
}

func TestComputeGraphResponse_InvalidEnvironmentID(t *testing.T) {
	conn, err := sdk.NewDirectConnection("http://localhost:9000/apis/api.ucp.dev/v1alpha3")
	require.NoError(t, err)

	applicationID, err := resources.Parse("/planes/radius/local/resourceGroups/default/providers/Applications.Core/applications/myapp")
	require.NoError(t, err)

	// An empty/invalid environment ID string must surface as a parse error from
	// resources.Parse and not a panic or a successful response.
	resp, err := ComputeGraphResponse(context.Background(), applicationID, "not-a-valid-resource-id", conn)
	require.Error(t, err)
	require.Nil(t, resp)
}

// TestComputeGraphResponse_UCPProvidersError covers the error-propagation path from
// ListAllResourceTypesNames (~lines 57-59) by returning a 500 from the UCP providers endpoint.
func TestComputeGraphResponse_UCPProvidersError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/planes/radius/local/providers", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	conn, err := sdk.NewDirectConnection(server.URL)
	require.NoError(t, err)

	applicationID, err := resources.Parse("/planes/radius/local/resourceGroups/default/providers/Applications.Core/applications/myapp")
	require.NoError(t, err)

	resp, err := ComputeGraphResponse(
		context.Background(),
		applicationID,
		"/planes/radius/local/resourceGroups/default/providers/Applications.Core/environments/myenv",
		conn,
	)
	require.Error(t, err)
	require.Nil(t, resp)
}

// TestGetGraphRun_DatabaseError covers the error path from GetResource (line ~104) where the
// database client returns a non-NotFound error and the controller propagates it.
func TestGetGraphRun_DatabaseError(t *testing.T) {
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()

	databaseClient := database.NewMockClient(mctrl)
	req, err := rpctest.NewHTTPRequestWithContent(
		context.Background(),
		v1.OperationPost.HTTPMethod(),
		"http://localhost:8080/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/Applications/myapp/getGraph?api-version=2023-10-01-preview", nil)
	require.NoError(t, err)

	databaseClient.
		EXPECT().
		Get(gomock.Any(), gomock.Any()).
		Return(nil, errors.New("boom"))

	ctx := rpctest.NewARMRequestContext(req)
	conn, err := sdk.NewDirectConnection("http://localhost:9000/apis/api.ucp.dev/v1alpha3")
	require.NoError(t, err)

	ctl, err := NewGetGraph(ctrl.Options{DatabaseClient: databaseClient}, conn)
	require.NoError(t, err)

	w := httptest.NewRecorder()
	resp, err := ctl.Run(ctx, w, req)
	require.Error(t, err)
	require.Nil(t, resp)
}

// TestGetGraphRun_ComputeGraphSuccess covers the success path through the controller's Run into
// ComputeGraphResponse (line ~110 and the body of ComputeGraphResponse) by serving an empty UCP
// resource-providers list from an httptest server, which yields an empty graph.
func TestGetGraphRun_ComputeGraphSuccess(t *testing.T) {
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()

	mux := http.NewServeMux()
	mux.HandleFunc("/planes/radius/local/providers", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"value":[]}`))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	const appIDStr = "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/applications/myapp"
	const envIDStr = "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/myenv"

	databaseClient := database.NewMockClient(mctrl)
	databaseClient.
		EXPECT().
		Get(gomock.Any(), gomock.Any()).
		Return(rpctest.FakeStoreObject(&datamodel.Application{
			BaseResource: v1.BaseResource{
				TrackedResource: v1.TrackedResource{ID: appIDStr},
			},
			Properties: datamodel.ApplicationProperties{
				BasicResourceProperties: rpv1.BasicResourceProperties{Environment: envIDStr},
			},
		}), nil)

	req, err := rpctest.NewHTTPRequestWithContent(
		context.Background(),
		v1.OperationPost.HTTPMethod(),
		"http://localhost:8080"+appIDStr+"/getGraph?api-version=2023-10-01-preview", nil)
	require.NoError(t, err)

	ctx := rpctest.NewARMRequestContext(req)
	conn, err := sdk.NewDirectConnection(server.URL)
	require.NoError(t, err)

	ctl, err := NewGetGraph(ctrl.Options{DatabaseClient: databaseClient}, conn)
	require.NoError(t, err)

	w := httptest.NewRecorder()
	resp, err := ctl.Run(ctx, w, req)
	require.NoError(t, err)
	_, ok := resp.(*rest.OKResponse)
	require.True(t, ok, "expected an OK response, got %T", resp)
}
