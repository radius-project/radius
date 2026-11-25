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
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	"github.com/radius-project/radius/pkg/armrpc/frontend/defaultoperation"
	"github.com/radius-project/radius/pkg/armrpc/rpctest"
	"github.com/radius-project/radius/pkg/authz"
	"github.com/radius-project/radius/pkg/components/database"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/kubernetes"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	resources_kubernetes "github.com/radius-project/radius/pkg/ucp/resources/kubernetes"
	"github.com/radius-project/radius/test/k8sutil"
	"github.com/radius-project/radius/test/testutil"
)

// The fake client does not assign the UID that a real API server returns on creation.
func newSecretClient() runtimeclient.Client {
	return interceptor.NewClient(k8sutil.NewFakeKubeClient(nil), interceptor.Funcs{
		Create: func(ctx context.Context, client runtimeclient.WithWatch, obj runtimeclient.Object, opts ...runtimeclient.CreateOption) error {
			if _, ok := obj.(*corev1.Secret); ok && obj.GetUID() == "" {
				obj.SetUID(types.UID(uuid.NewString()))
			}
			return client.Create(ctx, obj, opts...)
		},
	})
}

func TestUpsertSecret_Ownership(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		existing    bool
		oldManaged  *bool
		oldUID      string
		oldName     string
		wantManaged bool
	}{
		{name: "created by this store", wantManaged: true},
		{name: "adopted", existing: true},
		{name: "legacy label is not ownership", existing: true, oldName: "secret0"},
		{name: "previously adopted", existing: true, oldName: "secret0", oldManaged: new(false), oldUID: testSecretUID},
		{name: "owned update", existing: true, oldName: "secret0", oldManaged: new(true), oldUID: testSecretUID, wantManaged: true},
		{name: "replacement", existing: true, oldName: "secret0", oldManaged: new(true), oldUID: "previous-uid"},
		{name: "missing ownership UID", existing: true, oldName: "secret0", oldManaged: new(true)},
		{name: "ownership belongs to another reference", existing: true, oldName: "other-secret", oldManaged: new(true), oldUID: testSecretUID},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			client := newSecretClient()
			if tt.existing {
				require.NoError(t, client.Create(t.Context(), existingSecret("test-namespace", "secret0",
					map[string]string{kubernetes.LabelRadiusResourceType: "unrelated"})))
			}
			var old *datamodel.SecretStore
			if tt.oldName != "" {
				old = testutil.MustGetTestData[datamodel.SecretStore](testFileGenericValueGlobalScope)
				old.Properties.Status.OutputResources = []rpv1.OutputResource{{
					LocalID:              rpv1.LocalIDSecret,
					ID:                   resources_kubernetes.IDFromParts(resources_kubernetes.PlaneNameTODO, "", resources_kubernetes.KindSecret, "test-namespace", tt.oldName),
					RadiusManaged:        tt.oldManaged,
					AdditionalProperties: map[string]string{secretUIDProperty: tt.oldUID},
				}}
			}
			store := testutil.MustGetTestData[datamodel.SecretStore](testFileGenericValueGlobalScope)
			// Incoming status must never grant ownership of a pre-existing Secret.
			store.Properties.Status.OutputResources = []rpv1.OutputResource{{
				LocalID:              rpv1.LocalIDSecret,
				ID:                   resources_kubernetes.IDFromParts(resources_kubernetes.PlaneNameTODO, "", resources_kubernetes.KindSecret, "test-namespace", "secret0"),
				RadiusManaged:        new(true),
				AdditionalProperties: map[string]string{secretUIDProperty: testSecretUID},
			}}
			options := &controller.Options{KubeClient: client, Authorization: authzOptions(authz.ModeEnforce)}
			response, err := ValidateAndMutateRequest(t.Context(), store, old, options)
			require.NoError(t, err)
			require.Nil(t, response)
			response, err = UpsertSecret(t.Context(), store, old, options)
			require.NoError(t, err)
			require.Nil(t, response)
			secret := &corev1.Secret{}
			require.NoError(t, client.Get(t.Context(), runtimeclient.ObjectKey{Namespace: "test-namespace", Name: "secret0"}, secret))
			require.NotEmpty(t, secret.UID)

			// Ownership must survive the same serialization used by resource storage.
			persisted := &datamodel.SecretStore{}
			require.NoError(t, rpctest.FakeStoreObject(store).As(persisted))
			output := secretOutputResource(persisted.Properties.Status.OutputResources)
			require.Equal(t, new(tt.wantManaged), output.RadiusManaged)
			if tt.wantManaged {
				require.Equal(t, string(secret.UID), output.AdditionalProperties[secretUIDProperty])
			} else {
				require.Empty(t, output.AdditionalProperties)
			}
		})
	}
}

func TestSecretStore_SharedSecretOwnership(t *testing.T) {
	t.Parallel()
	client := newSecretClient()
	options := &controller.Options{KubeClient: client, Authorization: authzOptions(authz.ModeEnforce)}
	owner := testutil.MustGetTestData[datamodel.SecretStore](testFileGenericValueGlobalScope)
	response, err := UpsertSecret(t.Context(), owner, nil, options)
	require.NoError(t, err)
	require.Nil(t, response)

	adopter := testutil.MustGetTestData[datamodel.SecretStore](testFileGenericValueGlobalScope)
	adopter.ID += "-adopter"
	adopter.Name += "-adopter"
	adopter.Properties.Status.OutputResources = owner.Properties.Status.OutputResources
	response, err = UpsertSecret(t.Context(), adopter, nil, options)
	require.NoError(t, err)
	require.Nil(t, response)
	require.False(t, *secretOutputResource(adopter.Properties.Status.OutputResources).RadiusManaged)
	_, err = DeleteRadiusSecret(t.Context(), adopter, options)
	require.NoError(t, err)
	secret := &corev1.Secret{}
	key := runtimeclient.ObjectKey{Namespace: "test-namespace", Name: "secret0"}
	require.NoError(t, client.Get(t.Context(), key, secret), "an adopter cannot delete the owner's Secret")

	_, err = DeleteRadiusSecret(t.Context(), owner, options)
	require.NoError(t, err)
	require.True(t, apierrors.IsNotFound(client.Get(t.Context(), key, secret)))
}

func TestUpsertSecret_CreationFailureDoesNotRecordOwnership(t *testing.T) {
	t.Parallel()
	for _, createError := range []error{
		nil,
		apierrors.NewAlreadyExists(schema.GroupResource{Resource: "secrets"}, "secret0"),
	} {
		name := "missing UID"
		if createError != nil {
			name = "creation race"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			backing := k8sutil.NewFakeKubeClient(nil)
			client := interceptor.NewClient(backing, interceptor.Funcs{
				Create: func(ctx context.Context, client runtimeclient.WithWatch, obj runtimeclient.Object, opts ...runtimeclient.CreateOption) error {
					if _, ok := obj.(*corev1.Secret); ok && createError != nil {
						return createError
					}
					return client.Create(ctx, obj, opts...)
				},
			})
			store := testutil.MustGetTestData[datamodel.SecretStore](testFileGenericValueGlobalScope)
			store.Properties.Status.OutputResources = nil
			response, err := UpsertSecret(t.Context(), store, nil, &controller.Options{KubeClient: client})
			require.Nil(t, response)
			if createError != nil {
				require.ErrorIs(t, err, createError)
			} else {
				require.ErrorContains(t, err, "has no UID")
			}
			require.Empty(t, store.Properties.Status.OutputResources)
		})
	}
}

func TestDeleteRadiusSecret_OwnershipAndRecordDeletion(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name           string
		managed        *bool
		recordUID      string
		liveUID        types.UID
		missing        bool
		protected      bool
		getError       error
		deleteError    error
		wantDelete     bool
		wantRecordGone bool
	}{
		{name: "owned", managed: new(true), recordUID: testSecretUID, liveUID: testSecretUID, wantDelete: true, wantRecordGone: true},
		{name: "adopted", managed: new(false), liveUID: testSecretUID, wantRecordGone: true},
		{name: "legacy labeled", liveUID: testSecretUID, wantRecordGone: true},
		{name: "missing ownership UID", managed: new(true), liveUID: testSecretUID, wantRecordGone: true},
		{name: "replaced", managed: new(true), recordUID: "old-uid", liveUID: testSecretUID, wantRecordGone: true},
		{name: "missing", managed: new(true), recordUID: testSecretUID, missing: true, wantRecordGone: true},
		{name: "protected adopted", protected: true, liveUID: testSecretUID},
		{name: "protected missing", protected: true, missing: true},
		{name: "read failed", managed: new(true), recordUID: testSecretUID, liveUID: testSecretUID, getError: errors.New("read unavailable")},
		{name: "delete failed", managed: new(true), recordUID: testSecretUID, liveUID: testSecretUID, deleteError: errors.New("delete unavailable"), wantDelete: true},
		{name: "deleted concurrently", managed: new(true), recordUID: testSecretUID, liveUID: testSecretUID,
			deleteError: apierrors.NewNotFound(schema.GroupResource{Resource: "secrets"}, "letsencrypt-prod"), wantDelete: true, wantRecordGone: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx, logs := admissionContext(t)
			namespace := "default"
			if tt.protected {
				namespace = "radius-system"
			}
			store := testutil.MustGetTestData[datamodel.SecretStore](testFileCertValueFrom)
			store.Properties.Status.OutputResources = []rpv1.OutputResource{{
				LocalID:              rpv1.LocalIDSecret,
				ID:                   resources_kubernetes.IDFromParts(resources_kubernetes.PlaneNameTODO, "", resources_kubernetes.KindSecret, namespace, "letsencrypt-prod"),
				RadiusManaged:        tt.managed,
				AdditionalProperties: map[string]string{secretUIDProperty: tt.recordUID},
			}}
			backing := k8sutil.NewFakeKubeClient(nil)
			key := runtimeclient.ObjectKey{Namespace: namespace, Name: "letsencrypt-prod"}
			if !tt.missing {
				secret := existingSecret(namespace, key.Name, map[string]string{kubernetes.LabelRadiusResourceType: "unrelated"})
				secret.UID = tt.liveUID
				require.NoError(t, backing.Create(ctx, secret))
			}
			deletes, reads := 0, 0
			client := interceptor.NewClient(backing, interceptor.Funcs{
				Get: func(ctx context.Context, client runtimeclient.WithWatch, key runtimeclient.ObjectKey, obj runtimeclient.Object, opts ...runtimeclient.GetOption) error {
					if _, ok := obj.(*corev1.Secret); ok {
						reads++
						if tt.getError != nil {
							return tt.getError
						}
					}
					return client.Get(ctx, key, obj, opts...)
				},
				Delete: func(ctx context.Context, client runtimeclient.WithWatch, obj runtimeclient.Object, opts ...runtimeclient.DeleteOption) error {
					deletes++
					deleteOptions := (&runtimeclient.DeleteOptions{}).ApplyOptions(opts)
					require.NotNil(t, deleteOptions.Preconditions)
					require.Equal(t, tt.recordUID, string(*deleteOptions.Preconditions.UID))
					if tt.deleteError != nil {
						if apierrors.IsNotFound(tt.deleteError) {
							require.NoError(t, client.Delete(ctx, obj))
						}
						return tt.deleteError
					}
					return client.Delete(ctx, obj, opts...)
				},
			})
			db := database.NewMockClient(gomock.NewController(t))
			db.EXPECT().Get(gomock.Any(), testSecretID).Return(rpctest.FakeStoreObject(store), nil)
			if tt.wantRecordGone {
				db.EXPECT().Delete(gomock.Any(), testSecretID).Return(nil)
			}
			ctl, err := defaultoperation.NewDefaultSyncDelete[*datamodel.SecretStore](
				controller.Options{KubeClient: client, DatabaseClient: db, Authorization: authzOptions(authz.ModeEnforce)},
				controller.ResourceOptions[datamodel.SecretStore]{DeleteFilters: []controller.DeleteFilter[datamodel.SecretStore]{DeleteRadiusSecret}})
			require.NoError(t, err)
			req := httptest.NewRequest(http.MethodDelete, "/", nil)
			writer := httptest.NewRecorder()
			response, err := ctl.Run(ctx, writer, req)
			requireNoSecretValue(t, err, logs)
			if tt.protected {
				requireAdmission(t, err, logs, authz.ModeEnforce, authz.RuleControlPlaneNamespace)
			} else if tt.getError != nil {
				require.ErrorIs(t, err, tt.getError)
			} else if tt.deleteError != nil && !apierrors.IsNotFound(tt.deleteError) {
				require.ErrorIs(t, err, tt.deleteError)
			} else {
				require.NoError(t, err)
				require.NoError(t, response.Apply(ctx, writer, req))
				require.Equal(t, http.StatusOK, writer.Code)
			}
			if tt.protected || tt.managed == nil || !*tt.managed || tt.recordUID == "" {
				require.Zero(t, reads)
			}
			if tt.wantDelete {
				require.Equal(t, 1, deletes)
			} else {
				require.Zero(t, deletes)
			}
			getErr := backing.Get(ctx, key, &corev1.Secret{})
			if tt.missing || (tt.wantDelete && tt.wantRecordGone) {
				require.True(t, apierrors.IsNotFound(getErr))
			} else {
				require.NoError(t, getErr)
			}
		})
	}
}

func TestDeleteRadiusSecret_ReplacementDuringDelete(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	secret := existingSecret("default", "letsencrypt-prod", nil)
	backing := k8sutil.NewFakeKubeClient(nil, secret)
	store := testutil.MustGetTestData[datamodel.SecretStore](testFileCertValueFrom)
	output := secretOutputResource(store.Properties.Status.OutputResources)
	output.RadiusManaged = new(true)
	output.AdditionalProperties = map[string]string{secretUIDProperty: testSecretUID}
	conflict := apierrors.NewConflict(schema.GroupResource{Resource: "secrets"}, secret.Name, errors.New("UID precondition failed"))
	client := interceptor.NewClient(backing, interceptor.Funcs{
		Delete: func(ctx context.Context, client runtimeclient.WithWatch, obj runtimeclient.Object, opts ...runtimeclient.DeleteOption) error {
			deleteOptions := (&runtimeclient.DeleteOptions{}).ApplyOptions(opts)
			require.NotNil(t, deleteOptions.Preconditions)
			require.Equal(t, types.UID(testSecretUID), *deleteOptions.Preconditions.UID)
			require.NoError(t, client.Delete(ctx, obj))
			replacement := existingSecret(secret.Namespace, secret.Name, nil)
			replacement.UID = "replacement-uid"
			require.NoError(t, client.Create(ctx, replacement))
			return conflict
		},
	})
	_, err := DeleteRadiusSecret(ctx, store, &controller.Options{KubeClient: client})
	require.ErrorIs(t, err, conflict)
	_, err = DeleteRadiusSecret(ctx, store, &controller.Options{KubeClient: client})
	require.NoError(t, err, "retry retains the replacement and allows record-only cleanup")
	actual := &corev1.Secret{}
	require.NoError(t, backing.Get(ctx, runtimeclient.ObjectKeyFromObject(secret), actual))
	require.Equal(t, types.UID("replacement-uid"), actual.UID)
}
