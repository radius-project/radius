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

package secretstores

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/golang/mock/gomock"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/rpctest"
	"github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/project-radius/radius/test/k8sutil"
	"github.com/project-radius/radius/test/testutil"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestListSecrets_20220315PrivatePreview(t *testing.T) {
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()

	mStorageClient := store.NewMockStorageClient(mctrl)
	req, err := rpctest.NewHTTPRequestWithContent(
		context.Background(),
		v1.OperationPost.HTTPMethod(),
		"http://localhost:8080/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/secretStores/secret0/listsecrets?api-version=2022-03-15-privatepreview", nil)
	require.NoError(t, err)

	t.Run("not found the resource", func(t *testing.T) {
		mStorageClient.
			EXPECT().
			Get(gomock.Any(), gomock.Any()).
			Return(nil, &store.ErrNotFound{})
		ctx := rpctest.ARMTestContextFromRequest(req)
		opts := ctrl.Options{
			StorageClient: mStorageClient,
		}

		ctl, err := NewListSecrets(opts)
		require.NoError(t, err)

		w := httptest.NewRecorder()
		resp, err := ctl.Run(ctx, w, req)
		require.NoError(t, err)

		_ = resp.Apply(ctx, w, req)
		require.Equal(t, 404, w.Result().StatusCode)
	})

	t.Run("return secrets successfully", func(t *testing.T) {
		secretdm := testutil.MustGetTestData[datamodel.SecretStore](testFileCertValueFrom)
		mStorageClient.
			EXPECT().
			Get(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, id string, _ ...store.GetOptions) (*store.Object, error) {
				return &store.Object{
					Metadata: store.Metadata{ID: id, ETag: "etag"},
					Data:     secretdm,
				}, nil
			})
		ctx := rpctest.ARMTestContextFromRequest(req)
		ksecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "letsencrypt-prod",
				Namespace: "default",
			},
			Data: map[string][]byte{
				"tls.crt": []byte("cert"),
				"tls.key": []byte("key"),
			},
		}
		opts := ctrl.Options{
			StorageClient: mStorageClient,
			KubeClient:    k8sutil.NewFakeKubeClient(nil, ksecret),
		}

		ctl, err := NewListSecrets(opts)
		require.NoError(t, err)

		w := httptest.NewRecorder()
		resp, err := ctl.Run(ctx, w, req)
		require.NoError(t, err)

		_ = resp.Apply(ctx, w, req)
		require.Equal(t, 200, w.Result().StatusCode)

		actualOutput := &v20220315privatepreview.SecretStoreListSecretsResult{}
		_ = json.Unmarshal(w.Body.Bytes(), actualOutput)
	})
}

func TestListSecrets_InvalidKubernetesSecret(t *testing.T) {
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()

	mStorageClient := store.NewMockStorageClient(mctrl)
	req, err := rpctest.NewHTTPRequestWithContent(
		context.Background(),
		v1.OperationPost.HTTPMethod(),
		"http://localhost:8080/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/secretStores/secret0/listsecrets?api-version=2022-03-15-privatepreview", nil)
	require.NoError(t, err)

	secretdm := testutil.MustGetTestData[datamodel.SecretStore](testFileCertValueFrom)
	secretdm.Properties.Data["tls.key"].Encoding = datamodel.SecretValueEncodingRaw

	kubeSecretTests := []struct {
		name string
		in   *corev1.Secret
		err  error
	}{
		{
			name: "backing kubernetes secret not found",
			in: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "notfound",
					Namespace: "default",
				},
				Data: map[string][]byte{},
			},
			err: errors.New("referenced secret is not found"),
		},
		{
			name: "secret is not found",
			in: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "letsencrypt-prod",
					Namespace: "default",
				},
				Data: map[string][]byte{
					"tls.crt": []byte("dGxzLmtleS1wcmlrZXkK"),
				},
			},
			err: errors.New("cannot find tls.key key from secret data"),
		},
		{
			name: "invalid base64 encoded secret",
			in: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "letsencrypt-prod",
					Namespace: "default",
				},
				Data: map[string][]byte{
					"tls.crt": []byte("dGxzLmtleS1wcmlrZXkK"),
					"tls.key": []byte("_"),
				},
			},
			err: errors.New("tls.key is the invalid base64 encoded value: illegal base64 data at input byte 0"),
		},
	}

	for _, tc := range kubeSecretTests {
		t.Run(tc.name, func(t *testing.T) {
			mStorageClient.
				EXPECT().
				Get(gomock.Any(), gomock.Any()).
				DoAndReturn(func(ctx context.Context, id string, _ ...store.GetOptions) (*store.Object, error) {
					return &store.Object{
						Metadata: store.Metadata{ID: id, ETag: "etag"},
						Data:     secretdm,
					}, nil
				})
			ctx := rpctest.ARMTestContextFromRequest(req)
			opts := ctrl.Options{
				StorageClient: mStorageClient,
				KubeClient:    k8sutil.NewFakeKubeClient(nil, tc.in),
			}

			ctl, err := NewListSecrets(opts)
			require.NoError(t, err)

			w := httptest.NewRecorder()
			_, err = ctl.Run(ctx, w, req)
			require.ErrorContains(t, err, tc.err.Error())
		})
	}
}
