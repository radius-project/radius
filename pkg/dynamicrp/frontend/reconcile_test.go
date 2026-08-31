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
	"context"
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
	k8stest "github.com/radius-project/radius/test/k8sutil"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const reconcileTestURL = "/planes/radius/local/resourceGroups/test-group/providers/Applications.Test/testResources/myResource/reconcile?api-version=2023-10-01-preview"

const (
	deploymentOutputID = "/planes/kubernetes/local/namespaces/default/providers/apps/Deployment/my-deployment"
	azureOutputID      = "/planes/azure/mycloud/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/rg1/providers/Microsoft.Storage/storageAccounts/mystorage"
)

// scheme registers the built-in Kubernetes types the reconcile handler will look up.
func reconcileTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	require.NoError(t, appsv1.AddToScheme(s))
	require.NoError(t, corev1.AddToScheme(s))
	return s
}

// reconcileTestDiscovery returns a discovery client that reports the two built-in kinds the
// reconcile handler needs to resolve for these tests.
func reconcileTestDiscovery() *k8stest.DiscoveryClient {
	return &k8stest.DiscoveryClient{
		Resources: []*metav1.APIResourceList{
			{
				GroupVersion: "apps/v1",
				APIResources: []metav1.APIResource{
					{Name: "deployments", Kind: "Deployment", Namespaced: true, Version: "v1"},
				},
			},
			{
				GroupVersion: "v1",
				APIResources: []metav1.APIResource{
					{Name: "services", Kind: "Service", Namespaced: true, Version: "v1"},
				},
			},
		},
	}
}

func newReconcileController(t *testing.T, databaseClient database.Client, kubeClient runtimeclient.Client) controller.Controller {
	t.Helper()

	opts := controller.Options{
		DatabaseClient: databaseClient,
		KubeClient:     kubeClient,
	}
	resourceOpts := controller.ResourceOptions[datamodel.DynamicResource]{
		RequestConverter:  converter.DynamicResourceDataModelFromVersioned,
		ResponseConverter: converter.DynamicResourceDataModelToVersioned,
	}

	c, err := NewReconcile(opts, resourceOpts, nil, reconcileTestDiscovery())
	require.NoError(t, err)
	return c
}

// newReconcileResource builds a DynamicResource with the given provisioningState and
// outputResources planted under properties.status. outputResources are stored as []map[string]any
// on disk; OutputResources() JSON-unmarshals them back into rpv1.OutputResource values.
func newReconcileResource(state v1.ProvisioningState, outputIDs ...string) *datamodel.DynamicResource {
	outputs := make([]map[string]any, 0, len(outputIDs))
	radiusManaged := true
	for _, id := range outputIDs {
		outputs = append(outputs, map[string]any{
			"localID":       "output-" + id,
			"id":            id,
			"radiusManaged": radiusManaged,
		})
	}

	props := map[string]any{}
	if len(outputs) > 0 {
		props["status"] = map[string]any{
			"outputResources": outputs,
		}
	}

	return &datamodel.DynamicResource{
		ID:                     testResourceID,
		Name:                   "myResource",
		Type:                   "Applications.Test/testResources",
		UpdatedAPIVersion:      testAPIVersion,
		AsyncProvisioningState: state,
		Properties:             props,
	}
}

func runReconcile(t *testing.T, c controller.Controller) *httptest.ResponseRecorder {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, reconcileTestURL, nil)
	require.NoError(t, err)
	ctx := rpctest.NewARMRequestContext(req)
	w := httptest.NewRecorder()

	resp, err := c.Run(ctx, w, req)
	require.NoError(t, err)
	require.NoError(t, resp.Apply(ctx, w, req))
	return w
}

func decodeReconcileResponse(t *testing.T, w *httptest.ResponseRecorder) ReconcileResponse {
	t.Helper()
	var body ReconcileResponse
	require.NoError(t, json.NewDecoder(w.Result().Body).Decode(&body))
	return body
}

func TestReconcile_NotFound(t *testing.T) {
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()

	databaseClient := database.NewMockClient(mctrl)
	databaseClient.EXPECT().Get(gomock.Any(), testResourceID).Return(nil, &database.ErrNotFound{})

	c := newReconcileController(t, databaseClient, k8stest.NewFakeKubeClient(reconcileTestScheme(t)))

	w := runReconcile(t, c)
	require.Equal(t, http.StatusNotFound, w.Result().StatusCode)
}

// A resource already in a terminal state has nothing to reconcile: return 200 with an empty
// resources array so the orchestrator can distinguish "no-op" from "reconciled".
func TestReconcile_TerminalState_ReturnsEmpty(t *testing.T) {
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()

	resource := newReconcileResource(v1.ProvisioningStateSucceeded, deploymentOutputID)
	storeObject := rpctest.FakeStoreObject(resource)
	storeObject.Metadata = database.Metadata{ID: testResourceID, ETag: "etag-1"}

	databaseClient := database.NewMockClient(mctrl)
	databaseClient.EXPECT().Get(gomock.Any(), testResourceID).Return(storeObject, nil)
	// Save must not be called: no state transition when already terminal.

	c := newReconcileController(t, databaseClient, k8stest.NewFakeKubeClient(reconcileTestScheme(t)))

	w := runReconcile(t, c)
	require.Equal(t, http.StatusOK, w.Result().StatusCode)

	body := decodeReconcileResponse(t, w)
	require.Empty(t, body.Resources)
}

// The transcript's failing case: a resource is hydrated in Updating but its Kubernetes object no
// longer exists. Reality check reports "gone" for the single output, and the handler transitions
// provisioningState to Failed and persists it so subsequent deletes are unblocked.
func TestReconcile_KubernetesGone_TransitionsToFailed(t *testing.T) {
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()

	resource := newReconcileResource(v1.ProvisioningStateUpdating, deploymentOutputID)
	storeObject := rpctest.FakeStoreObject(resource)
	storeObject.Metadata = database.Metadata{ID: testResourceID, ETag: "etag-1"}

	databaseClient := database.NewMockClient(mctrl)
	databaseClient.EXPECT().Get(gomock.Any(), testResourceID).Return(storeObject, nil)
	databaseClient.EXPECT().
		Save(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, obj *database.Object, _ ...database.SaveOptions) error {
			saved, ok := obj.Data.(*datamodel.DynamicResource)
			require.True(t, ok, "saved payload must be a DynamicResource")
			require.Equal(t, v1.ProvisioningStateFailed, saved.ProvisioningState())
			return nil
		})

	// Empty cluster: the Deployment does not exist.
	c := newReconcileController(t, databaseClient, k8stest.NewFakeKubeClient(reconcileTestScheme(t)))

	w := runReconcile(t, c)
	require.Equal(t, http.StatusOK, w.Result().StatusCode)

	body := decodeReconcileResponse(t, w)
	require.Len(t, body.Resources, 1)
	require.Equal(t, testResourceID, body.Resources[0].ResourceID)
	require.Equal(t, string(v1.ProvisioningStateUpdating), body.Resources[0].From)
	require.Equal(t, string(v1.ProvisioningStateFailed), body.Resources[0].To)
	require.Contains(t, body.Resources[0].Reason, "gone")
}

// A resource whose Kubernetes output exists gets promoted from Updating to Succeeded.
func TestReconcile_KubernetesSettled_TransitionsToSucceeded(t *testing.T) {
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()

	resource := newReconcileResource(v1.ProvisioningStateUpdating, deploymentOutputID)
	storeObject := rpctest.FakeStoreObject(resource)
	storeObject.Metadata = database.Metadata{ID: testResourceID, ETag: "etag-1"}

	databaseClient := database.NewMockClient(mctrl)
	databaseClient.EXPECT().Get(gomock.Any(), testResourceID).Return(storeObject, nil)
	databaseClient.EXPECT().
		Save(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, obj *database.Object, _ ...database.SaveOptions) error {
			saved := obj.Data.(*datamodel.DynamicResource)
			require.Equal(t, v1.ProvisioningStateSucceeded, saved.ProvisioningState())
			return nil
		})

	// Populate the cluster with the deployment that outputResources references.
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "my-deployment", Namespace: "default"},
	}
	kubeClient := k8stest.NewFakeKubeClient(reconcileTestScheme(t), deployment)

	c := newReconcileController(t, databaseClient, kubeClient)

	w := runReconcile(t, c)
	require.Equal(t, http.StatusOK, w.Result().StatusCode)

	body := decodeReconcileResponse(t, w)
	require.Len(t, body.Resources, 1)
	require.Equal(t, string(v1.ProvisioningStateUpdating), body.Resources[0].From)
	require.Equal(t, string(v1.ProvisioningStateSucceeded), body.Resources[0].To)
	require.Contains(t, body.Resources[0].Reason, "settled")
}

// A Terraform-backed cloud output cannot be reality-checked in Phase 1: it is recorded as
// "skipped" and the resource's provisioningState is left unchanged. No Save must be issued.
func TestReconcile_CloudOutput_Skipped(t *testing.T) {
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()

	resource := newReconcileResource(v1.ProvisioningStateUpdating, azureOutputID)
	storeObject := rpctest.FakeStoreObject(resource)
	storeObject.Metadata = database.Metadata{ID: testResourceID, ETag: "etag-1"}

	databaseClient := database.NewMockClient(mctrl)
	databaseClient.EXPECT().Get(gomock.Any(), testResourceID).Return(storeObject, nil)
	// Save must not be called: skipped output → state unchanged.

	c := newReconcileController(t, databaseClient, k8stest.NewFakeKubeClient(reconcileTestScheme(t)))

	w := runReconcile(t, c)
	require.Equal(t, http.StatusOK, w.Result().StatusCode)

	body := decodeReconcileResponse(t, w)
	require.Len(t, body.Resources, 1)
	require.Equal(t, string(v1.ProvisioningStateUpdating), body.Resources[0].From)
	require.Equal(t, string(v1.ProvisioningStateUpdating), body.Resources[0].To)
	require.Contains(t, body.Resources[0].Reason, "skipped")
	require.Contains(t, body.Resources[0].Reason, "cloud output")
}
