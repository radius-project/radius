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
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	ctrl "github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	"github.com/radius-project/radius/pkg/cli/clients_new/generated"
	"github.com/radius-project/radius/pkg/components/database"
	corerpv20250801preview "github.com/radius-project/radius/pkg/corerp/api/v20250801preview"
	"github.com/radius-project/radius/pkg/sdk"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// TestReconcile_Integration_EndToEnd exercises the reconcile orchestrator against a real HTTP
// server that impersonates UCP + downstream RPs. It verifies the parts that pure unit tests
// cannot: URL construction for each child's /reconcile endpoint, the resource-provider summary
// call that resolves each child's API version, request headers, and JSON response decoding into
// the aggregated ReconcileResponse. The child walk is still stubbed with a static list because
// standing up a fake UCP resource-listing surface would obscure what this test actually protects.
//
// Scenario mirrors the plan's exit criterion: a two-container application where one k8s Deployment
// has vanished (dispatched RP reports Failed) and the other is healthy (dispatched RP reports
// Succeeded). The orchestrator must return both outcomes verbatim.
func TestReconcile_Integration_EndToEnd(t *testing.T) {
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()

	// Track every request the fake server sees so the assertions can prove the orchestrator dispatched
	// to each child's specific /reconcile path.
	var (
		mu           sync.Mutex
		reconcileHit = map[string]bool{}
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		switch {
		// getAPIVersionForResourceType issues this GET to resolve the API version for the child's
		// resource type before POSTing the reconcile action.
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/providers/Radius.Compute"):
			require.Equal(t, "2023-10-01-preview", r.URL.Query().Get("api-version"))
			w.Header().Set("Content-Type", "application/json")
			require.NoError(t, json.NewEncoder(w).Encode(corerpv20250801PreviewProviderSummary()))

		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/containers/frontend/reconcile"):
			reconcileHit["frontend"] = true
			require.Equal(t, "2023-10-01-preview", r.URL.Query().Get("api-version"))
			require.Equal(t, "application/json", r.Header.Get("Content-Type"))
			// Verify the orchestrator sent an empty JSON object as the ReconcileRequest body.
			body, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			require.JSONEq(t, `{}`, string(body))

			w.Header().Set("Content-Type", "application/json")
			require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
				"resources": []map[string]any{{
					"resourceId": testContainerFrontend,
					"from":       string(v1.ProvisioningStateUpdating),
					"to":         string(v1.ProvisioningStateFailed),
					"reason":     "kubernetes object not found",
				}},
			}))

		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/containers/backend/reconcile"):
			reconcileHit["backend"] = true
			w.Header().Set("Content-Type", "application/json")
			require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
				"resources": []map[string]any{{
					"resourceId": testContainerBackend,
					"from":       string(v1.ProvisioningStateUpdating),
					"to":         string(v1.ProvisioningStateSucceeded),
				}},
			}))

		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.String())
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	connection, err := sdk.NewDirectConnection(server.URL)
	require.NoError(t, err)

	databaseClient := database.NewMockClient(mctrl)
	databaseClient.EXPECT().Get(gomock.Any(), gomock.Any()).Return(storedApplication(), nil)

	children := []generated.GenericResource{
		makeChild(testContainerFrontend, "Radius.Compute/containers", string(v1.ProvisioningStateUpdating)),
		makeChild(testContainerBackend, "Radius.Compute/containers", string(v1.ProvisioningStateUpdating)),
	}

	c := newReconcilev20250801preview(
		ctrl.Options{DatabaseClient: databaseClient},
		connection,
		func(context.Context, resources.ID) ([]generated.GenericResource, error) { return children, nil },
		nil, // real defaultReconcileChild — this is what we're testing.
	)

	w := runReconcile(t, c, makeReconcileRequest(t))
	require.Equal(t, http.StatusOK, w.Result().StatusCode)

	body := decodeReconcileResponse(t, w)
	require.Len(t, body.Resources, 2, "orchestrator must aggregate both children into the report")

	byID := map[string]*corerpv20250801preview.ReconcileResourceOutcome{}
	for _, o := range body.Resources {
		byID[to.String(o.ResourceID)] = o
	}
	require.Equal(t, string(v1.ProvisioningStateFailed), to.String(byID[testContainerFrontend].To))
	require.Equal(t, "kubernetes object not found", to.String(byID[testContainerFrontend].Reason))
	require.Equal(t, string(v1.ProvisioningStateSucceeded), to.String(byID[testContainerBackend].To))

	require.True(t, reconcileHit["frontend"], "orchestrator must POST /reconcile for the frontend container")
	require.True(t, reconcileHit["backend"], "orchestrator must POST /reconcile for the backend container")
}

// TestReconcile_Integration_RPWithoutReconcileIsSkipped verifies the forward-compat path: when a
// child's RP returns 404 or 405 for /reconcile (the route is not registered on that RP), the
// orchestrator records a skipped outcome instead of failing the whole reconcile.
func TestReconcile_Integration_RPWithoutReconcileIsSkipped(t *testing.T) {
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/providers/Radius.Compute"):
			w.Header().Set("Content-Type", "application/json")
			require.NoError(t, json.NewEncoder(w).Encode(corerpv20250801PreviewProviderSummary()))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/reconcile"):
			// Simulate an older RP that does not advertise /reconcile.
			http.Error(w, "not found", http.StatusNotFound)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	t.Cleanup(server.Close)

	connection, err := sdk.NewDirectConnection(server.URL)
	require.NoError(t, err)

	databaseClient := database.NewMockClient(mctrl)
	databaseClient.EXPECT().Get(gomock.Any(), gomock.Any()).Return(storedApplication(), nil)

	children := []generated.GenericResource{
		makeChild(testContainerFrontend, "Radius.Compute/containers", string(v1.ProvisioningStateUpdating)),
	}

	c := newReconcilev20250801preview(
		ctrl.Options{DatabaseClient: databaseClient},
		connection,
		func(context.Context, resources.ID) ([]generated.GenericResource, error) { return children, nil },
		nil,
	)

	w := runReconcile(t, c, makeReconcileRequest(t))
	require.Equal(t, http.StatusOK, w.Result().StatusCode)

	body := decodeReconcileResponse(t, w)
	require.Len(t, body.Resources, 1)
	require.Equal(t, string(v1.ProvisioningStateUpdating), to.String(body.Resources[0].To),
		"provisioningState must be left unchanged when the RP has no reconcile route")
	require.Contains(t, to.String(body.Resources[0].Reason), "RP does not implement reconcile")
}

// corerpv20250801PreviewProviderSummary builds a minimal ResourceProviderSummary payload that
// advertises Radius.Compute/containers with a default API version, which is all
// getAPIVersionForResourceType needs to resolve.
func corerpv20250801PreviewProviderSummary() map[string]any {
	return map[string]any{
		"name":      "Radius.Compute",
		"locations": map[string]any{"global": map[string]any{}},
		"resourceTypes": map[string]any{
			"containers": map[string]any{
				"defaultApiVersion": "2023-10-01-preview",
				"apiVersions": map[string]any{
					"2023-10-01-preview": map[string]any{},
				},
			},
		},
	}
}
