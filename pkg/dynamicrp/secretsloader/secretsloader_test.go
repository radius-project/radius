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

package secretsloader

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	armpolicy "github.com/Azure/azure-sdk-for-go/sdk/azcore/arm/policy"
	azfake "github.com/Azure/azure-sdk-for-go/sdk/azcore/fake"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/armrpc/rpctest"
	aztoken "github.com/radius-project/radius/pkg/azure/tokencredentials"
	"github.com/radius-project/radius/pkg/components/database"
	"github.com/radius-project/radius/pkg/components/kubernetesclient/kubernetesclientprovider"
	"github.com/radius-project/radius/pkg/crypto/encryption"
	"github.com/radius-project/radius/pkg/dynamicrp/datamodel"
	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/recipes/configloader"
	"github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	ucpfake "github.com/radius-project/radius/pkg/ucp/api/v20231001preview/fake"
)

const (
	testSecretID  = "/planes/radius/local/resourceGroups/test-rg/providers/Radius.Security/secrets/db-secret"
	testStoreID   = "/planes/radius/local/resourceGroups/test-rg/providers/Applications.Core/secretStores/store"
	testInvalidID = "not-a-valid-id"
)

// newKubeProvider builds a Kubernetes client provider backed by a fake runtime client holding the given objects.
func newKubeProvider(objects ...*corev1.Secret) *kubernetesclientprovider.KubernetesClientProvider {
	builder := fake.NewClientBuilder()
	for _, object := range objects {
		builder = builder.WithObjects(object)
	}

	provider := kubernetesclientprovider.FromConfig(nil)
	provider.SetRuntimeClient(builder.Build())
	return provider
}

func Test_DispatchingLoader_RadiusSecuritySecrets(t *testing.T) {
	t.Run("fails closed when the resource cannot be read from the database", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		databaseClient := database.NewMockClient(ctrl)
		databaseClient.EXPECT().Get(gomock.Any(), testSecretID).Return(nil, errors.New("boom"))

		loader := NewDispatchingLoader(nil, databaseClient, newKubeProvider(), nil)

		_, err := loader.LoadSecrets(context.Background(), map[string][]string{testSecretID: nil})
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to get secret resource")
	})

	t.Run("fails closed when not configured", func(t *testing.T) {
		loader := NewDispatchingLoader(nil, nil, nil, nil)

		_, err := loader.LoadSecrets(context.Background(), map[string][]string{testSecretID: nil})
		require.Error(t, err)
		require.Contains(t, err.Error(), "not fully configured")
	})
}

func Test_DispatchingLoader_Routing(t *testing.T) {
	t.Run("delegates non-UDT secret types to the store loader", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		storeLoader := configloader.NewMockSecretsLoader(ctrl)
		storeLoader.EXPECT().
			LoadSecrets(gomock.Any(), map[string][]string{testStoreID: nil}).
			Return(map[string]recipes.SecretData{
				testStoreID: {Type: "generic", Data: map[string]string{"key": "value"}},
			}, nil)

		loader := NewDispatchingLoader(storeLoader, nil, nil, nil)

		result, err := loader.LoadSecrets(context.Background(), map[string][]string{testStoreID: nil})
		require.NoError(t, err)
		require.Equal(t, map[string]string{"key": "value"}, result[testStoreID].Data)
	})

	t.Run("routes each type to its loader and merges the results", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		key, err := encryption.GenerateKey()
		require.NoError(t, err)

		databaseClient := database.NewMockClient(ctrl)
		databaseClient.EXPECT().Get(gomock.Any(), testSecretID).Return(
			newEncryptedSecretObject(t, testSecretID, key, map[string]any{"password": "s3cr3t"}), nil)
		kubeProvider := newKubeProvider(newEncryptionKeySecret(t, key))

		storeLoader := configloader.NewMockSecretsLoader(ctrl)
		storeLoader.EXPECT().
			LoadSecrets(gomock.Any(), map[string][]string{testStoreID: nil}).
			Return(map[string]recipes.SecretData{
				testStoreID: {Type: "generic", Data: map[string]string{"key": "value"}},
			}, nil)

		loader := NewDispatchingLoader(storeLoader, databaseClient, kubeProvider, testSecretsUCPClientFactory(t, testSecretsSchema()))

		result, err := loader.LoadSecrets(context.Background(), map[string][]string{
			testSecretID: nil,
			testStoreID:  nil,
		})
		require.NoError(t, err)
		require.Equal(t, map[string]string{"password": "s3cr3t"}, result[testSecretID].Data)
		require.Equal(t, map[string]string{"key": "value"}, result[testStoreID].Data)
	})

	t.Run("errors when a non-UDT secret is requested but no store loader is configured", func(t *testing.T) {
		loader := NewDispatchingLoader(nil, nil, nil, nil)

		_, err := loader.LoadSecrets(context.Background(), map[string][]string{testStoreID: nil})
		require.Error(t, err)
		require.Contains(t, err.Error(), "no secret store loader is configured")
	})

	t.Run("errors on an unparseable secret ID", func(t *testing.T) {
		loader := NewDispatchingLoader(nil, nil, nil, nil)

		_, err := loader.LoadSecrets(context.Background(), map[string][]string{testInvalidID: nil})
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to parse secret resource ID")
	})
}

const testSecretAPIVersion = "2023-10-01-preview"

// Test_DispatchingLoader_RadiusSecuritySecrets_FromStore covers the primary (multi-cluster safe) path:
// resolving a secret by decrypting the value retained in the Radius store with the control-plane key.
func Test_DispatchingLoader_RadiusSecuritySecrets_FromStore(t *testing.T) {
	t.Run("decrypts the retained value from the store", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		key, err := encryption.GenerateKey()
		require.NoError(t, err)

		databaseClient := database.NewMockClient(ctrl)
		databaseClient.EXPECT().Get(gomock.Any(), testSecretID).Return(
			newEncryptedSecretObject(t, testSecretID, key, map[string]any{"password": "s3cr3t", "username": "admin"}), nil)

		loader := NewDispatchingLoader(nil, databaseClient, newKubeProvider(newEncryptionKeySecret(t, key)), testSecretsUCPClientFactory(t, testSecretsSchema()))

		result, err := loader.LoadSecrets(context.Background(), map[string][]string{testSecretID: nil})
		require.NoError(t, err)
		require.Equal(t, "Radius.Security/secrets", result[testSecretID].Type)
		require.Equal(t, map[string]string{"password": "s3cr3t", "username": "admin"}, result[testSecretID].Data)
	})

	t.Run("returns only the requested keys from the store", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		key, err := encryption.GenerateKey()
		require.NoError(t, err)

		databaseClient := database.NewMockClient(ctrl)
		databaseClient.EXPECT().Get(gomock.Any(), testSecretID).Return(
			newEncryptedSecretObject(t, testSecretID, key, map[string]any{"password": "s3cr3t", "username": "admin"}), nil)

		loader := NewDispatchingLoader(nil, databaseClient, newKubeProvider(newEncryptionKeySecret(t, key)), testSecretsUCPClientFactory(t, testSecretsSchema()))

		result, err := loader.LoadSecrets(context.Background(), map[string][]string{testSecretID: {"password"}})
		require.NoError(t, err)
		require.Equal(t, map[string]string{"password": "s3cr3t"}, result[testSecretID].Data)
	})

	t.Run("fails closed when a requested key is absent from the store", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		key, err := encryption.GenerateKey()
		require.NoError(t, err)

		databaseClient := database.NewMockClient(ctrl)
		databaseClient.EXPECT().Get(gomock.Any(), testSecretID).Return(
			newEncryptedSecretObject(t, testSecretID, key, map[string]any{"password": "s3cr3t"}), nil)

		loader := NewDispatchingLoader(nil, databaseClient, newKubeProvider(newEncryptionKeySecret(t, key)), testSecretsUCPClientFactory(t, testSecretsSchema()))

		_, err = loader.LoadSecrets(context.Background(), map[string][]string{testSecretID: {"missing"}})
		require.Error(t, err)
		require.Contains(t, err.Error(), "was not found")
	})

	t.Run("fails closed when the value is nil at rest (pre-retain secret)", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		key, err := encryption.GenerateKey()
		require.NoError(t, err)

		// A secret created before retain-at-rest landed has a nil value in the store. Rather than silently
		// falling back to a single-cluster Kubernetes read, the loader must fail closed and direct the
		// operator to redeploy the secret.
		databaseClient := database.NewMockClient(ctrl)
		databaseClient.EXPECT().Get(gomock.Any(), testSecretID).Return(
			newEncryptedSecretObject(t, testSecretID, key, map[string]any{"password": nil}), nil)

		loader := NewDispatchingLoader(nil, databaseClient, newKubeProvider(newEncryptionKeySecret(t, key)), testSecretsUCPClientFactory(t, testSecretsSchema()))

		_, err = loader.LoadSecrets(context.Background(), map[string][]string{testSecretID: nil})
		require.Error(t, err)
		require.Contains(t, err.Error(), "redeploy the secret")
	})

	t.Run("fails closed when no schema is available to decrypt", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		key, err := encryption.GenerateKey()
		require.NoError(t, err)

		// With no ucpClient the schema cannot be fetched, so the retained value cannot be decrypted. The
		// loader must fail closed rather than silently returning empty data.
		databaseClient := database.NewMockClient(ctrl)
		databaseClient.EXPECT().Get(gomock.Any(), testSecretID).Return(
			newEncryptedSecretObject(t, testSecretID, key, map[string]any{"password": "s3cr3t"}), nil)

		loader := NewDispatchingLoader(nil, databaseClient, newKubeProvider(newEncryptionKeySecret(t, key)), nil)

		_, err = loader.LoadSecrets(context.Background(), map[string][]string{testSecretID: nil})
		require.Error(t, err)
		require.Contains(t, err.Error(), "no schema is available")
	})
}

func Test_buildSecretDataFromStore(t *testing.T) {
	const id = "/planes/radius/local/resourceGroups/test-rg/providers/Radius.Security/secrets/s"

	t.Run("returns all keys when no filter is given", func(t *testing.T) {
		props := map[string]any{"data": map[string]any{
			"a": map[string]any{"value": "1"},
			"b": map[string]any{"value": "2"},
		}}
		data, err := buildSecretDataFromStore(id, props, nil)
		require.NoError(t, err)
		require.Equal(t, map[string]string{"a": "1", "b": "2"}, data.Data)
		require.Equal(t, secretResourceType, data.Type)
	})

	t.Run("returns only the requested keys", func(t *testing.T) {
		props := map[string]any{"data": map[string]any{
			"a": map[string]any{"value": "1"},
			"b": map[string]any{"value": "2"},
		}}
		data, err := buildSecretDataFromStore(id, props, []string{"a"})
		require.NoError(t, err)
		require.Equal(t, map[string]string{"a": "1"}, data.Data)
	})

	t.Run("a missing requested key is an error", func(t *testing.T) {
		props := map[string]any{"data": map[string]any{"a": map[string]any{"value": "1"}}}
		_, err := buildSecretDataFromStore(id, props, []string{"c"})
		require.Error(t, err)
		require.Contains(t, err.Error(), "was not found")
	})

	t.Run("a nil value fails closed (pre-retain secret)", func(t *testing.T) {
		props := map[string]any{"data": map[string]any{"a": map[string]any{"value": nil}}}
		_, err := buildSecretDataFromStore(id, props, nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "redeploy the secret")
	})

	t.Run("an absent data property fails closed", func(t *testing.T) {
		_, err := buildSecretDataFromStore(id, map[string]any{}, nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "no data stored at rest")
	})

	t.Run("a non-string value is an error", func(t *testing.T) {
		props := map[string]any{"data": map[string]any{"a": map[string]any{"value": 123}}}
		_, err := buildSecretDataFromStore(id, props, []string{"a"})
		require.Error(t, err)
		require.Contains(t, err.Error(), "is not a string")
	})

	t.Run("an unexpected entry format is an error", func(t *testing.T) {
		props := map[string]any{"data": map[string]any{"a": "not-a-map"}}
		_, err := buildSecretDataFromStore(id, props, []string{"a"})
		require.Error(t, err)
		require.Contains(t, err.Error(), "unexpected format")
	})
}

// testSecretsSchema returns a minimal Radius.Security/secrets schema whose data[*].value field is marked
// sensitive, matching the production schema shape used to extract sensitive field paths.
func testSecretsSchema() map[string]any {
	return map[string]any{
		"properties": map[string]any{
			"data": map[string]any{
				"additionalProperties": map[string]any{
					"properties": map[string]any{
						"value": map[string]any{
							"type":               "string",
							"x-radius-sensitive": true,
						},
					},
				},
			},
		},
	}
}

// testSecretsUCPClientFactory builds a v20231001preview.ClientFactory backed by a fake transport that
// returns the given schema for any API version lookup.
func testSecretsUCPClientFactory(t *testing.T, schema map[string]any) *v20231001preview.ClientFactory {
	t.Helper()

	apiVersionsServer := ucpfake.APIVersionsServer{
		Get: func(ctx context.Context, planeName string, resourceProviderName string, resourceTypeName string, apiVersionName string, options *v20231001preview.APIVersionsClientGetOptions) (resp azfake.Responder[v20231001preview.APIVersionsClientGetResponse], errResp azfake.ErrorResponder) {
			response := v20231001preview.APIVersionsClientGetResponse{
				APIVersionResource: v20231001preview.APIVersionResource{
					Properties: &v20231001preview.APIVersionProperties{
						Schema: schema,
					},
				},
			}
			resp.SetResponse(http.StatusOK, response, nil)
			return
		},
	}

	factory, err := v20231001preview.NewClientFactory(&aztoken.AnonymousCredential{}, &armpolicy.ClientOptions{
		ClientOptions: policy.ClientOptions{
			Transport: ucpfake.NewAPIVersionsServerTransport(&apiVersionsServer),
		},
	})
	require.NoError(t, err)
	return factory
}

// newEncryptionKeySecret builds the radius-system Kubernetes Secret that holds the versioned encryption key
// store the loader uses to decrypt retained secret values.
func newEncryptionKeySecret(t *testing.T, key []byte) *corev1.Secret {
	t.Helper()

	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      encryption.DefaultEncryptionKeySecretName,
			Namespace: encryption.RadiusNamespace,
		},
		Data: map[string][]byte{
			encryption.DefaultEncryptionKeySecretKey: mustKeyStoreJSON(t, key),
		},
	}
}

// newEncryptedSecretObject builds a database.Object for a Radius.Security/secrets resource whose data
// property holds values encrypted with the given key, mirroring how the frontend encrypts and the backend
// retains them at rest. A nil entry value represents a pre-retain (redacted) secret, which the loader must
// fail closed on rather than silently resolve.
func newEncryptedSecretObject(t *testing.T, resourceID string, key []byte, values map[string]any) *database.Object {
	t.Helper()

	provider, err := encryption.NewInMemoryKeyProvider(key)
	require.NoError(t, err)
	handler, err := encryption.NewSensitiveDataHandlerFromProvider(context.Background(), provider)
	require.NoError(t, err)

	data := map[string]any{}
	for name, value := range values {
		data[name] = map[string]any{"value": value}
	}

	properties := map[string]any{
		"provisioningState": "Succeeded",
		"data":              data,
	}

	require.NoError(t, handler.EncryptSensitiveFields(properties, []string{"data[*].value"}, resourceID))

	resource := &datamodel.DynamicResource{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:   resourceID,
				Name: "db-secret",
				Type: "Radius.Security/secrets",
			},
			InternalMetadata: v1.InternalMetadata{
				UpdatedAPIVersion: testSecretAPIVersion,
			},
		},
		Properties: properties,
	}

	return rpctest.FakeStoreObject(resource)
}

// mustKeyStoreJSON serializes a single-version key store for the radius-system encryption key Secret.
func mustKeyStoreJSON(t *testing.T, key []byte) []byte {
	t.Helper()

	keyStore := encryption.KeyStore{
		CurrentVersion: 1,
		Keys: map[string]encryption.KeyData{
			"1": {
				Key:       base64.StdEncoding.EncodeToString(key),
				Version:   1,
				CreatedAt: "2026-01-01T00:00:00Z",
				ExpiresAt: "2026-12-31T00:00:00Z",
			},
		},
	}

	bytes, err := json.Marshal(keyStore)
	require.NoError(t, err)
	return bytes
}
