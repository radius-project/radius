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
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	"github.com/radius-project/radius/pkg/armrpc/frontend/defaultoperation"
	"github.com/radius-project/radius/pkg/authz"
	"github.com/radius-project/radius/pkg/components/database"
	"github.com/radius-project/radius/pkg/components/database/inmemory"
	"github.com/radius-project/radius/pkg/corerp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/corerp/datamodel/converter"
	"github.com/radius-project/radius/pkg/kubernetes"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	resources_kubernetes "github.com/radius-project/radius/pkg/ucp/resources/kubernetes"
	"github.com/radius-project/radius/test/testutil"
)

func TestSecretStore_ExistingRecordLifecycle(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		mode      authz.Mode
		namespace string
	}{
		{name: "ordinary unset", namespace: "default"},
		{name: "ordinary off", mode: authz.ModeOff, namespace: "default"},
		{name: "ordinary dryRun", mode: authz.ModeDryRun, namespace: "default"},
		{name: "ordinary enforce", mode: authz.ModeEnforce, namespace: "default"},
		{name: "control-plane unset", namespace: "radius-system"},
		{name: "control-plane off", mode: authz.ModeOff, namespace: "radius-system"},
		{name: "control-plane dryRun", mode: authz.ModeDryRun, namespace: "radius-system"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx, logs := admissionContext(t)
			v1.ARMRequestContextFromContext(ctx).APIVersion = v20231001preview.Version
			db := inmemory.NewClient()
			client := newSecretClient()
			secret := existingSecret(tt.namespace, testControlPlaneSecret,
				map[string]string{kubernetes.LabelRadiusResourceType: "applications.core-secretstores"})
			secret.Data = map[string][]byte{"token": []byte("b3JpZ2luYWw=")}
			require.NoError(t, client.Create(ctx, secret))

			// Seed the persisted shape from before E4: no managed flag or ownership UID.
			legacy := testutil.MustGetTestData[datamodel.SecretStore](testFileGenericValueGlobalScope)
			legacy.Properties.Resource = tt.namespace + "/" + testControlPlaneSecret
			legacy.Properties.Data = map[string]*datamodel.SecretStoreDataValue{
				"token": {Encoding: datamodel.SecretValueEncodingBase64},
			}
			legacy.Properties.Status.OutputResources = []rpv1.OutputResource{{
				LocalID: rpv1.LocalIDSecret,
				ID:      resources_kubernetes.IDFromParts(resources_kubernetes.PlaneNameTODO, "", resources_kubernetes.KindSecret, tt.namespace, testControlPlaneSecret),
			}}
			require.NoError(t, db.Save(ctx, &database.Object{ID: legacy.ID, Data: legacy}))

			opts := controller.Options{KubeClient: client, DatabaseClient: db, Authorization: authzOptions(tt.mode)}
			resourceOpts := controller.ResourceOptions[datamodel.SecretStore]{
				RequestConverter:  converter.SecretStoreModelFromVersioned,
				ResponseConverter: converter.SecretStoreModelToVersioned,
				UpdateFilters:     []controller.UpdateFilter[datamodel.SecretStore]{ValidateAndMutateRequest, UpsertSecret},
				DeleteFilters:     []controller.DeleteFilter[datamodel.SecretStore]{DeleteRadiusSecret},
			}
			put, err := defaultoperation.NewDefaultSyncPut[*datamodel.SecretStore](opts, resourceOpts)
			require.NoError(t, err)
			get, err := defaultoperation.NewGetResource[*datamodel.SecretStore](opts, resourceOpts)
			require.NoError(t, err)
			remove, err := defaultoperation.NewDefaultSyncDelete[*datamodel.SecretStore](opts, resourceOpts)
			require.NoError(t, err)
			list, err := NewListSecrets(opts)
			require.NoError(t, err)

			run := func(ctl controller.Controller, method string, body any, wantStatus int) *httptest.ResponseRecorder {
				t.Helper()
				var content []byte
				if body != nil {
					var err error
					content, err = json.Marshal(body)
					require.NoError(t, err)
				}
				url := testSecretID
				if method == http.MethodPost {
					url += "/listSecrets"
				}
				req := httptest.NewRequest(method, url+"?api-version="+v20231001preview.Version, bytes.NewReader(content))
				req.Header.Set("Content-Type", "application/json")
				writer := httptest.NewRecorder()
				resp, runErr := ctl.Run(ctx, writer, req)
				require.NoError(t, runErr, "legacy records must not fail because ownership metadata is absent")
				require.NotNil(t, resp)
				require.NoError(t, resp.Apply(ctx, writer, req))
				require.Equal(t, wantStatus, writer.Code)
				if method != http.MethodPost {
					for _, value := range []string{"b3JpZ2luYWw=", "dXBkYXRlZA==", "c2Vjb25k"} {
						require.NotContains(t, writer.Body.String(), value)
					}
				}
				return writer
			}
			readValue := func(expected string) {
				t.Helper()
				writer := run(list, http.MethodPost, nil, http.StatusOK)
				var result datamodel.SecretStoreListSecrets
				require.NoError(t, json.Unmarshal(writer.Body.Bytes(), &result))
				require.NotNil(t, result.Data["token"])
				require.NotNil(t, result.Data["token"].Value)
				require.Equal(t, expected, *result.Data["token"].Value)
			}
			request := &v20231001preview.SecretStoreResource{
				Location: new("global"),
				Properties: &v20231001preview.SecretStoreProperties{
					Type: new(v20231001preview.SecretStoreDataTypeGeneric),
					Data: map[string]*v20231001preview.SecretValueProperties{
						"token": {Encoding: new(v20231001preview.SecretValueEncodingBase64), Value: new("dXBkYXRlZA==")},
					},
				},
			}

			run(get, http.MethodGet, nil, http.StatusOK)
			readValue("b3JpZ2luYWw=")
			run(put, http.MethodPatch, request, http.StatusOK)
			readValue("dXBkYXRlZA==")
			key := runtimeclient.ObjectKeyFromObject(secret)
			beforeDelete := &corev1.Secret{}
			require.NoError(t, client.Get(ctx, key, beforeDelete))

			run(remove, http.MethodDelete, nil, http.StatusOK)
			run(get, http.MethodGet, nil, http.StatusNotFound)
			run(remove, http.MethodDelete, nil, http.StatusNoContent)
			retained := &corev1.Secret{}
			require.NoError(t, client.Get(ctx, key, retained))
			require.Equal(t, beforeDelete, retained, "record-only cleanup must not modify the retained Secret")

			// Recreating the same store must reuse the retained Secret without AlreadyExists errors.
			request.Properties.Resource = new(legacy.Properties.Resource)
			request.Properties.Data["token"].Value = nil
			run(put, http.MethodPut, request, http.StatusOK)
			readValue("dXBkYXRlZA==")
			saved, err := db.Get(ctx, legacy.ID)
			require.NoError(t, err)
			recreated := &datamodel.SecretStore{}
			require.NoError(t, saved.As(recreated))
			output := secretOutputResource(recreated.Properties.Status.OutputResources)
			require.NotNil(t, output)
			require.NotNil(t, output.RadiusManaged)
			require.False(t, *output.RadiusManaged)

			request.Properties.Resource = nil
			request.Properties.Data["token"].Value = new("c2Vjb25k")
			run(put, http.MethodPatch, request, http.StatusOK)
			readValue("c2Vjb25k")
			run(remove, http.MethodDelete, nil, http.StatusOK)
			run(get, http.MethodGet, nil, http.StatusNotFound)
			require.NoError(t, client.Get(ctx, key, retained))
			require.Equal(t, secret.UID, retained.UID)
			for _, entry := range logs.All() {
				for _, value := range []string{"b3JpZ2luYWw=", "dXBkYXRlZA==", "c2Vjb25k"} {
					require.NotContains(t, entry.Message, value)
					require.NotContains(t, fmt.Sprint(entry.ContextMap()), value)
				}
			}
		})
	}
}
