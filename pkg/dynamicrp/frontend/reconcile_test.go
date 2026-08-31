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

package frontend

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	"github.com/radius-project/radius/pkg/armrpc/rpctest"
	"github.com/radius-project/radius/pkg/components/database"
	"github.com/radius-project/radius/pkg/dynamicrp/datamodel"
	"github.com/radius-project/radius/pkg/dynamicrp/datamodel/converter"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

const reconcileTestURL = "/planes/radius/local/resourceGroups/test-group/providers/Applications.Test/testResources/myResource/reconcile?api-version=2023-10-01-preview"

func newReconcileController(t *testing.T, databaseClient database.Client) controller.Controller {
	t.Helper()

	ucpClient, err := testUCPClientFactoryWithSensitiveFields()
	require.NoError(t, err)

	opts := controller.Options{DatabaseClient: databaseClient}
	resourceOpts := controller.ResourceOptions[datamodel.DynamicResource]{
		RequestConverter:  converter.DynamicResourceDataModelFromVersioned,
		ResponseConverter: converter.DynamicResourceDataModelToVersioned,
	}

	c, err := NewReconcile(opts, resourceOpts, ucpClient)
	require.NoError(t, err)
	return c
}

func TestReconcile_ReturnsEmptyReport(t *testing.T) {
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()

	resource := &datamodel.DynamicResource{
		ID:                     testResourceID,
		Name:                   "myResource",
		Type:                   "Applications.Test/testResources",
		UpdatedAPIVersion:      testAPIVersion,
		AsyncProvisioningState: v1.ProvisioningStateSucceeded,
		Properties:             map[string]any{},
	}
	storeObject := rpctest.FakeStoreObject(resource)
	storeObject.Metadata = database.Metadata{ID: testResourceID, ETag: "etag-1"}

	databaseClient := database.NewMockClient(mctrl)
	databaseClient.EXPECT().Get(gomock.Any(), testResourceID).Return(storeObject, nil)

	c := newReconcileController(t, databaseClient)

	req, err := http.NewRequest(http.MethodPost, reconcileTestURL, nil)
	require.NoError(t, err)
	ctx := rpctest.NewARMRequestContext(req)
	w := httptest.NewRecorder()

	resp, err := c.Run(ctx, w, req)
	require.NoError(t, err)
	require.NoError(t, resp.Apply(ctx, w, req))
	require.Equal(t, http.StatusOK, w.Result().StatusCode)

	var body ReconcileResponse
	require.NoError(t, json.NewDecoder(w.Result().Body).Decode(&body))
	require.NotNil(t, body.Resources)
	require.Empty(t, body.Resources, "Phase 1 wiring stub must return an empty resources array")
}

func TestReconcile_NotFound(t *testing.T) {
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()

	databaseClient := database.NewMockClient(mctrl)
	databaseClient.EXPECT().Get(gomock.Any(), testResourceID).Return(nil, &database.ErrNotFound{})

	c := newReconcileController(t, databaseClient)

	req, err := http.NewRequest(http.MethodPost, reconcileTestURL, nil)
	require.NoError(t, err)
	ctx := rpctest.NewARMRequestContext(req)
	w := httptest.NewRecorder()

	resp, err := c.Run(ctx, w, req)
	require.NoError(t, err)
	require.NoError(t, resp.Apply(ctx, w, req))
	require.Equal(t, http.StatusNotFound, w.Result().StatusCode)
}
