//go:build !windows

/*
Copyright 2026 The Radius Authors.

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

package secretstores

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	"github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	"github.com/radius-project/radius/pkg/armrpc/frontend/defaultoperation"
	"github.com/radius-project/radius/pkg/authz"
	"github.com/radius-project/radius/pkg/components/database"
	"github.com/radius-project/radius/pkg/components/database/inmemory"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/test/testutil"
)

func TestDeleteRadiusSecret_UIDPreconditionAPIServer(t *testing.T) {
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		t.Skip("Run make test or set KUBEBUILDER_ASSETS to run the isolated API-server regression.")
	}
	environment := &envtest.Environment{UseExistingCluster: new(false)}
	cfg, err := environment.Start()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, environment.Stop()) })
	client, err := runtimeclient.NewWithWatch(cfg, runtimeclient.Options{})
	require.NoError(t, err)
	ctx, _ := admissionContext(t)
	require.NoError(t, client.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "test-namespace"}}))
	options := controller.Options{KubeClient: client, Authorization: authzOptions(authz.ModeEnforce)}
	store := testutil.MustGetTestData[datamodel.SecretStore](testFileGenericValueGlobalScope)
	store.ID = testSecretID
	_, err = UpsertSecret(ctx, store, nil, &options)
	require.NoError(t, err)
	db := inmemory.NewClient()
	require.NoError(t, db.Save(ctx, &database.Object{ID: store.ID, Data: store}))
	options.DatabaseClient = db

	// Replace the object after the controller's GET, immediately before its DELETE reaches
	// the real API server. No sleeps, timing race, fake UID, or fake conflict response.
	var replacement *corev1.Secret
	options.KubeClient = interceptor.NewClient(client, interceptor.Funcs{
		Delete: func(ctx context.Context, backing runtimeclient.WithWatch, obj runtimeclient.Object, opts ...runtimeclient.DeleteOption) error {
			originalUID := obj.GetUID()
			require.NoError(t, backing.Delete(ctx, obj, runtimeclient.Preconditions{UID: &originalUID}))
			replacement = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: obj.GetName(), Namespace: obj.GetNamespace()},
				Data:       map[string][]byte{"token": []byte("synthetic-replacement")},
			}
			require.NoError(t, backing.Create(ctx, replacement))
			require.NotEqual(t, originalUID, replacement.UID)
			return backing.Delete(ctx, obj, opts...)
		},
	})
	remove, err := defaultoperation.NewDefaultSyncDelete[*datamodel.SecretStore](options,
		controller.ResourceOptions[datamodel.SecretStore]{DeleteFilters: []controller.DeleteFilter[datamodel.SecretStore]{DeleteRadiusSecret}})
	require.NoError(t, err)
	_, err = remove.Run(ctx, httptest.NewRecorder(), httptest.NewRequest(http.MethodDelete, "/", nil))
	require.True(t, apierrors.IsConflict(err), "the API server must reject the stale UID: %v", err)
	_, err = db.Get(ctx, store.ID)
	require.NoError(t, err, "failed deletion must leave the Radius record retryable")
	live := &corev1.Secret{}
	require.NoError(t, client.Get(ctx, runtimeclient.ObjectKeyFromObject(replacement), live))
	require.Equal(t, replacement.UID, live.UID)
	require.Equal(t, replacement.ResourceVersion, live.ResourceVersion)

	// On retry the stale ownership is recognized and only the Radius record is removed.
	response, err := remove.Run(ctx, httptest.NewRecorder(), httptest.NewRequest(http.MethodDelete, "/", nil))
	require.NoError(t, err)
	require.NotNil(t, response)
	_, err = db.Get(ctx, store.ID)
	var notFound *database.ErrNotFound
	require.ErrorAs(t, err, &notFound)
	require.NoError(t, client.Get(ctx, runtimeclient.ObjectKeyFromObject(replacement), live))
	require.Equal(t, replacement.UID, live.UID)
	require.Equal(t, replacement.ResourceVersion, live.ResourceVersion)
}
