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

package v20250801preview

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	ctrl "github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	"github.com/radius-project/radius/pkg/armrpc/rpctest"
	"github.com/radius-project/radius/pkg/components/database"
	corerpv20250801preview "github.com/radius-project/radius/pkg/corerp/api/v20250801preview"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	"github.com/radius-project/radius/pkg/sdk"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

const reconcileRoute = "http://localhost:8080/planes/radius/local/resourcegroups/default/providers/Radius.Core/applications/myapp/reconcile?api-version=2025-08-01-preview"

func newReconcileTestConnection(t *testing.T) sdk.Connection {
	t.Helper()
	conn, err := sdk.NewDirectConnection("http://localhost:9000/apis/api.ucp.dev/v1alpha3")
	require.NoError(t, err)
	return conn
}

func TestReconcileRun_ReturnsEmptyReport(t *testing.T) {
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()

	databaseClient := database.NewMockClient(mctrl)
	req, err := rpctest.NewHTTPRequestWithContent(
		t.Context(),
		v1.OperationPost.HTTPMethod(),
		reconcileRoute, nil,
	)
	require.NoError(t, err)

	// Return an application resource so the stub proceeds past the existence check. The stub
	// does not read any properties from the resource yet, so a minimal stored record is enough.
	stored := &database.Object{
		Metadata: database.Metadata{ID: "/planes/radius/local/resourceGroups/default/providers/Radius.Core/applications/myapp"},
		Data: &datamodel.Application_v20250801preview{
			BaseResource: v1.BaseResource{
				TrackedResource: v1.TrackedResource{
					ID:   "/planes/radius/local/resourceGroups/default/providers/Radius.Core/applications/myapp",
					Name: "myapp",
					Type: "Radius.Core/applications",
				},
				InternalMetadata: v1.InternalMetadata{
					UpdatedAPIVersion:      "2025-08-01-preview",
					AsyncProvisioningState: v1.ProvisioningStateSucceeded,
				},
			},
			Properties: datamodel.ApplicationProperties_v20250801preview{
				BasicResourceProperties: rpv1.BasicResourceProperties{
					Environment: "/planes/radius/local/resourceGroups/default/providers/Radius.Core/environments/env0",
				},
			},
		},
	}
	databaseClient.EXPECT().Get(gomock.Any(), gomock.Any()).Return(stored, nil)

	ctx := rpctest.NewARMRequestContext(req)
	opts := ctrl.Options{DatabaseClient: databaseClient}
	c, err := NewReconcilev20250801preview(opts, newReconcileTestConnection(t))
	require.NoError(t, err)

	w := httptest.NewRecorder()
	resp, err := c.Run(ctx, w, req)
	require.NoError(t, err)
	require.NoError(t, resp.Apply(ctx, w, req))

	require.Equal(t, http.StatusOK, w.Result().StatusCode)

	var body corerpv20250801preview.ReconcileResponse
	require.NoError(t, json.NewDecoder(w.Result().Body).Decode(&body))
	require.NotNil(t, body.Resources)
	require.Empty(t, body.Resources, "Phase 0 stub must return an empty report")
}

func TestReconcileRun_NotFound(t *testing.T) {
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()

	databaseClient := database.NewMockClient(mctrl)
	req, err := rpctest.NewHTTPRequestWithContent(
		t.Context(),
		v1.OperationPost.HTTPMethod(),
		reconcileRoute, nil,
	)
	require.NoError(t, err)

	databaseClient.EXPECT().Get(gomock.Any(), gomock.Any()).Return(nil, &database.ErrNotFound{})

	ctx := rpctest.NewARMRequestContext(req)
	c, err := NewReconcilev20250801preview(ctrl.Options{DatabaseClient: databaseClient}, newReconcileTestConnection(t))
	require.NoError(t, err)

	w := httptest.NewRecorder()
	resp, err := c.Run(ctx, w, req)
	require.NoError(t, err)
	require.NoError(t, resp.Apply(ctx, w, req))
	require.Equal(t, http.StatusNotFound, w.Result().StatusCode)
}

func TestReconcileRun_DatabaseError(t *testing.T) {
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()

	databaseClient := database.NewMockClient(mctrl)
	req, err := rpctest.NewHTTPRequestWithContent(
		t.Context(),
		v1.OperationPost.HTTPMethod(),
		reconcileRoute, nil,
	)
	require.NoError(t, err)

	databaseClient.EXPECT().Get(gomock.Any(), gomock.Any()).Return(nil, errors.New("boom"))

	ctx := rpctest.NewARMRequestContext(req)
	c, err := NewReconcilev20250801preview(ctrl.Options{DatabaseClient: databaseClient}, newReconcileTestConnection(t))
	require.NoError(t, err)

	w := httptest.NewRecorder()
	resp, actErr := c.Run(ctx, w, req)
	require.Error(t, actErr)
	require.Nil(t, resp)
}
