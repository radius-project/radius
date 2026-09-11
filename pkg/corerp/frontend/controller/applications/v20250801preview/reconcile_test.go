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
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	ctrl "github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	"github.com/radius-project/radius/pkg/armrpc/rpctest"
	"github.com/radius-project/radius/pkg/cli/clients_new/generated"
	"github.com/radius-project/radius/pkg/components/database"
	corerpv20250801preview "github.com/radius-project/radius/pkg/corerp/api/v20250801preview"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

const (
	reconcileRoute        = "http://localhost:8080/planes/radius/local/resourcegroups/default/providers/Radius.Core/applications/myapp/reconcile?api-version=2025-08-01-preview"
	testApplicationID     = "/planes/radius/local/resourceGroups/default/providers/Radius.Core/applications/myapp"
	testContainerFrontend = "/planes/radius/local/resourceGroups/default/providers/Radius.Compute/containers/frontend"
	testContainerBackend  = "/planes/radius/local/resourceGroups/default/providers/Radius.Compute/containers/backend"
)

// storedApplication returns a minimal application data-model stored under testApplicationID that
// the controller's GetResource can hand back. The properties are not read by the orchestrator, so
// a bare shell is enough.
func storedApplication() *database.Object {
	return &database.Object{
		Metadata: database.Metadata{ID: testApplicationID},
		Data: &datamodel.Application_v20250801preview{
			BaseResource: v1.BaseResource{
				TrackedResource: v1.TrackedResource{
					ID:   testApplicationID,
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
}

// newTestReconcileController builds a controller with test-supplied child walk and per-child
// dispatch hooks so the orchestration logic can be exercised without a live UCP.
func newTestReconcileController(
	t *testing.T,
	databaseClient database.Client,
	children []generated.GenericResource,
	reconcileChild func(ctx context.Context, child generated.GenericResource) ([]*corerpv20250801preview.ReconcileResourceOutcome, error),
) ctrl.Controller {
	t.Helper()
	return newReconcilev20250801preview(
		ctrl.Options{DatabaseClient: databaseClient},
		nil,
		func(ctx context.Context, _ resources.ID) ([]generated.GenericResource, error) {
			return children, nil
		},
		reconcileChild,
	)
}

func makeReconcileRequest(t *testing.T) *http.Request {
	t.Helper()
	req, err := rpctest.NewHTTPRequestWithContent(
		t.Context(),
		v1.OperationPost.HTTPMethod(),
		reconcileRoute, nil,
	)
	require.NoError(t, err)
	return req
}

func runReconcile(t *testing.T, c ctrl.Controller, req *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	ctx := rpctest.NewARMRequestContext(req)
	w := httptest.NewRecorder()
	resp, err := c.Run(ctx, w, req)
	require.NoError(t, err)
	require.NoError(t, resp.Apply(ctx, w, req))
	return w
}

func decodeReconcileResponse(t *testing.T, w *httptest.ResponseRecorder) corerpv20250801preview.ReconcileResponse {
	t.Helper()
	var body corerpv20250801preview.ReconcileResponse
	require.NoError(t, json.NewDecoder(w.Result().Body).Decode(&body))
	return body
}

// makeChild builds a GenericResource with the given ID, type, and provisioningState.
func makeChild(id, resourceType, state string) generated.GenericResource {
	return generated.GenericResource{
		ID:   to.Ptr(id),
		Name: to.Ptr(resources.MustParse(id).Name()),
		Type: to.Ptr(resourceType),
		Properties: map[string]any{
			"provisioningState": state,
		},
	}
}

func TestReconcileRun_NotFound(t *testing.T) {
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()

	databaseClient := database.NewMockClient(mctrl)
	databaseClient.EXPECT().Get(gomock.Any(), gomock.Any()).Return(nil, &database.ErrNotFound{})

	c := newTestReconcileController(t, databaseClient, nil, nil)

	w := runReconcile(t, c, makeReconcileRequest(t))
	require.Equal(t, http.StatusNotFound, w.Result().StatusCode)
}

func TestReconcileRun_DatabaseError(t *testing.T) {
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()

	databaseClient := database.NewMockClient(mctrl)
	databaseClient.EXPECT().Get(gomock.Any(), gomock.Any()).Return(nil, errors.New("boom"))

	c := newTestReconcileController(t, databaseClient, nil, nil)

	req := makeReconcileRequest(t)
	ctx := rpctest.NewARMRequestContext(req)
	w := httptest.NewRecorder()
	resp, err := c.Run(ctx, w, req)
	require.Error(t, err)
	require.Nil(t, resp)
}

// With no children, the report is empty. Application-scope succeeds without any child dispatch.
func TestReconcileRun_NoChildren(t *testing.T) {
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()

	databaseClient := database.NewMockClient(mctrl)
	databaseClient.EXPECT().Get(gomock.Any(), gomock.Any()).Return(storedApplication(), nil)

	dispatched := 0
	c := newTestReconcileController(t, databaseClient, []generated.GenericResource{},
		func(_ context.Context, _ generated.GenericResource) ([]*corerpv20250801preview.ReconcileResourceOutcome, error) {
			dispatched++
			return nil, nil
		})

	w := runReconcile(t, c, makeReconcileRequest(t))
	require.Equal(t, http.StatusOK, w.Result().StatusCode)
	body := decodeReconcileResponse(t, w)
	require.Empty(t, body.Resources)
	require.Equal(t, 0, dispatched, "no dispatch should occur when there are no children")
}

// Terminal children are filtered out before dispatch; only non-terminal children are POSTed to
// their RP and appear in the aggregated report.
func TestReconcileRun_SkipsTerminalChildren(t *testing.T) {
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()

	databaseClient := database.NewMockClient(mctrl)
	databaseClient.EXPECT().Get(gomock.Any(), gomock.Any()).Return(storedApplication(), nil)

	children := []generated.GenericResource{
		makeChild(testContainerFrontend, "Radius.Compute/containers", string(v1.ProvisioningStateSucceeded)),
		makeChild(testContainerBackend, "Radius.Compute/containers", string(v1.ProvisioningStateUpdating)),
	}

	var dispatchedIDs []string
	c := newTestReconcileController(t, databaseClient, children,
		func(_ context.Context, child generated.GenericResource) ([]*corerpv20250801preview.ReconcileResourceOutcome, error) {
			dispatchedIDs = append(dispatchedIDs, to.String(child.ID))
			return []*corerpv20250801preview.ReconcileResourceOutcome{{
				ResourceID: child.ID,
				From:       to.Ptr(string(v1.ProvisioningStateUpdating)),
				To:         to.Ptr(string(v1.ProvisioningStateFailed)),
				Reason:     to.Ptr("kubernetes object not found"),
			}}, nil
		})

	w := runReconcile(t, c, makeReconcileRequest(t))
	require.Equal(t, http.StatusOK, w.Result().StatusCode)

	body := decodeReconcileResponse(t, w)
	require.Len(t, body.Resources, 1)
	require.Equal(t, testContainerBackend, to.String(body.Resources[0].ResourceID))
	require.Equal(t, []string{testContainerBackend}, dispatchedIDs)
}

// Fan-out aggregates each child's outcomes into the app-level response. Order-independent because
// the orchestrator dispatches concurrently.
func TestReconcileRun_AggregatesMultipleChildren(t *testing.T) {
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()

	databaseClient := database.NewMockClient(mctrl)
	databaseClient.EXPECT().Get(gomock.Any(), gomock.Any()).Return(storedApplication(), nil)

	children := []generated.GenericResource{
		makeChild(testContainerFrontend, "Radius.Compute/containers", string(v1.ProvisioningStateUpdating)),
		makeChild(testContainerBackend, "Radius.Compute/containers", string(v1.ProvisioningStateUpdating)),
	}

	c := newTestReconcileController(t, databaseClient, children,
		func(_ context.Context, child generated.GenericResource) ([]*corerpv20250801preview.ReconcileResourceOutcome, error) {
			toState := string(v1.ProvisioningStateSucceeded)
			if to.String(child.ID) == testContainerFrontend {
				toState = string(v1.ProvisioningStateFailed)
			}
			return []*corerpv20250801preview.ReconcileResourceOutcome{{
				ResourceID: child.ID,
				From:       to.Ptr(string(v1.ProvisioningStateUpdating)),
				To:         to.Ptr(toState),
			}}, nil
		})

	w := runReconcile(t, c, makeReconcileRequest(t))
	require.Equal(t, http.StatusOK, w.Result().StatusCode)

	body := decodeReconcileResponse(t, w)
	require.Len(t, body.Resources, 2)

	byID := map[string]string{}
	for _, o := range body.Resources {
		byID[to.String(o.ResourceID)] = to.String(o.To)
	}
	require.Equal(t, string(v1.ProvisioningStateFailed), byID[testContainerFrontend])
	require.Equal(t, string(v1.ProvisioningStateSucceeded), byID[testContainerBackend])
}

// A single unreachable RP is recorded as a per-child skipped outcome; the reconcile as a whole
// still succeeds and reports outcomes for the healthy siblings.
func TestReconcileRun_DispatchFailureIsSkipped(t *testing.T) {
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()

	databaseClient := database.NewMockClient(mctrl)
	databaseClient.EXPECT().Get(gomock.Any(), gomock.Any()).Return(storedApplication(), nil)

	children := []generated.GenericResource{
		makeChild(testContainerFrontend, "Radius.Compute/containers", string(v1.ProvisioningStateUpdating)),
		makeChild(testContainerBackend, "Radius.Compute/containers", string(v1.ProvisioningStateUpdating)),
	}

	c := newTestReconcileController(t, databaseClient, children,
		func(_ context.Context, child generated.GenericResource) ([]*corerpv20250801preview.ReconcileResourceOutcome, error) {
			if to.String(child.ID) == testContainerBackend {
				return nil, errors.New("connection refused")
			}
			return []*corerpv20250801preview.ReconcileResourceOutcome{{
				ResourceID: child.ID,
				From:       to.Ptr(string(v1.ProvisioningStateUpdating)),
				To:         to.Ptr(string(v1.ProvisioningStateFailed)),
			}}, nil
		})

	w := runReconcile(t, c, makeReconcileRequest(t))
	require.Equal(t, http.StatusOK, w.Result().StatusCode)

	body := decodeReconcileResponse(t, w)
	require.Len(t, body.Resources, 2)

	ids := []string{}
	byID := map[string]*corerpv20250801preview.ReconcileResourceOutcome{}
	for _, o := range body.Resources {
		ids = append(ids, to.String(o.ResourceID))
		byID[to.String(o.ResourceID)] = o
	}
	sort.Strings(ids)
	require.Equal(t, []string{testContainerBackend, testContainerFrontend}, ids)

	require.Equal(t, string(v1.ProvisioningStateFailed), to.String(byID[testContainerFrontend].To))
	require.Equal(t, string(v1.ProvisioningStateUpdating), to.String(byID[testContainerBackend].To),
		"failed dispatch must not move the child out of Updating")
	require.Contains(t, to.String(byID[testContainerBackend].Reason), "reconcile dispatch failed")
}
