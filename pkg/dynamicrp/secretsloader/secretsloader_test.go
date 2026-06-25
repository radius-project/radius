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
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/armrpc/rpctest"
	"github.com/radius-project/radius/pkg/components/database"
	"github.com/radius-project/radius/pkg/components/kubernetesclient/kubernetesclientprovider"
	"github.com/radius-project/radius/pkg/dynamicrp/datamodel"
	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/recipes/configloader"
	resources_kubernetes "github.com/radius-project/radius/pkg/ucp/resources/kubernetes"
)

const (
	testSecretID    = "/planes/radius/local/resourceGroups/test-rg/providers/Radius.Security/secrets/db-secret"
	testStoreID     = "/planes/radius/local/resourceGroups/test-rg/providers/Applications.Core/secretStores/store"
	testNamespace   = "test-namespace"
	testK8sSecret   = "db-secret-k8s"
	testInvalidID   = "not-a-valid-id"
	testNoOutputsID = "/planes/radius/local/resourceGroups/test-rg/providers/Radius.Security/secrets/no-outputs"
)

// newSecretStoreObject builds a database.Object for a Radius.Security/secrets resource whose status
// references the given Kubernetes Secret as an output resource. When secretName is empty, no Kubernetes
// Secret output resource is included.
func newSecretStoreObject(t *testing.T, resourceID, namespace, secretName string) *database.Object {
	t.Helper()

	outputResources := []any{}
	if secretName != "" {
		k8sID := resources_kubernetes.IDFromParts("local", "", "Secret", namespace, secretName)
		outputResources = append(outputResources, map[string]any{
			"localID":       "Secret",
			"id":            k8sID.String(),
			"radiusManaged": true,
		})
	}

	resource := &datamodel.DynamicResource{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:   resourceID,
				Name: "db-secret",
				Type: "Radius.Security/secrets",
			},
		},
		Properties: map[string]any{
			"status": map[string]any{
				"outputResources": outputResources,
			},
		},
	}

	return rpctest.FakeStoreObject(resource)
}

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
	t.Run("reads cleartext from the backing Kubernetes Secret", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		databaseClient := database.NewMockClient(ctrl)
		databaseClient.EXPECT().Get(gomock.Any(), testSecretID).Return(newSecretStoreObject(t, testSecretID, testNamespace, testK8sSecret), nil)

		kubeProvider := newKubeProvider(&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: testK8sSecret, Namespace: testNamespace},
			Data:       map[string][]byte{"password": []byte("s3cr3t"), "username": []byte("admin")},
		})

		loader := NewDispatchingLoader(nil, databaseClient, kubeProvider)

		result, err := loader.LoadSecrets(context.Background(), map[string][]string{testSecretID: nil})
		require.NoError(t, err)
		require.Equal(t, map[string]recipes.SecretData{
			testSecretID: {
				Type: "Radius.Security/secrets",
				Data: map[string]string{"password": "s3cr3t", "username": "admin"},
			},
		}, result)
	})

	t.Run("returns only the requested keys", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		databaseClient := database.NewMockClient(ctrl)
		databaseClient.EXPECT().Get(gomock.Any(), testSecretID).Return(newSecretStoreObject(t, testSecretID, testNamespace, testK8sSecret), nil)

		kubeProvider := newKubeProvider(&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: testK8sSecret, Namespace: testNamespace},
			Data:       map[string][]byte{"password": []byte("s3cr3t"), "username": []byte("admin")},
		})

		loader := NewDispatchingLoader(nil, databaseClient, kubeProvider)

		result, err := loader.LoadSecrets(context.Background(), map[string][]string{testSecretID: {"password"}})
		require.NoError(t, err)
		require.Equal(t, map[string]string{"password": "s3cr3t"}, result[testSecretID].Data)
	})

	t.Run("fails closed when the resource has no Kubernetes Secret output resource", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		databaseClient := database.NewMockClient(ctrl)
		databaseClient.EXPECT().Get(gomock.Any(), testNoOutputsID).Return(newSecretStoreObject(t, testNoOutputsID, testNamespace, ""), nil)

		loader := NewDispatchingLoader(nil, databaseClient, newKubeProvider())

		_, err := loader.LoadSecrets(context.Background(), map[string][]string{testNoOutputsID: nil})
		require.Error(t, err)
		require.Contains(t, err.Error(), "no Kubernetes Secret output resource")
	})

	t.Run("fails closed when the Kubernetes Secret is absent", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		databaseClient := database.NewMockClient(ctrl)
		databaseClient.EXPECT().Get(gomock.Any(), testSecretID).Return(newSecretStoreObject(t, testSecretID, testNamespace, testK8sSecret), nil)

		loader := NewDispatchingLoader(nil, databaseClient, newKubeProvider())

		_, err := loader.LoadSecrets(context.Background(), map[string][]string{testSecretID: nil})
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to read Kubernetes Secret")
	})

	t.Run("fails closed when a requested key is missing", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		databaseClient := database.NewMockClient(ctrl)
		databaseClient.EXPECT().Get(gomock.Any(), testSecretID).Return(newSecretStoreObject(t, testSecretID, testNamespace, testK8sSecret), nil)

		kubeProvider := newKubeProvider(&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: testK8sSecret, Namespace: testNamespace},
			Data:       map[string][]byte{"password": []byte("s3cr3t")},
		})

		loader := NewDispatchingLoader(nil, databaseClient, kubeProvider)

		_, err := loader.LoadSecrets(context.Background(), map[string][]string{testSecretID: {"missing"}})
		require.Error(t, err)
		require.Contains(t, err.Error(), "was not found")
	})

	t.Run("fails closed when the resource cannot be read from the database", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		databaseClient := database.NewMockClient(ctrl)
		databaseClient.EXPECT().Get(gomock.Any(), testSecretID).Return(nil, errors.New("boom"))

		loader := NewDispatchingLoader(nil, databaseClient, newKubeProvider())

		_, err := loader.LoadSecrets(context.Background(), map[string][]string{testSecretID: nil})
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to get secret resource")
	})

	t.Run("fails closed when not configured", func(t *testing.T) {
		loader := NewDispatchingLoader(nil, nil, nil)

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

		loader := NewDispatchingLoader(storeLoader, nil, nil)

		result, err := loader.LoadSecrets(context.Background(), map[string][]string{testStoreID: nil})
		require.NoError(t, err)
		require.Equal(t, map[string]string{"key": "value"}, result[testStoreID].Data)
	})

	t.Run("routes each type to its loader and merges the results", func(t *testing.T) {
		ctrl := gomock.NewController(t)

		databaseClient := database.NewMockClient(ctrl)
		databaseClient.EXPECT().Get(gomock.Any(), testSecretID).Return(newSecretStoreObject(t, testSecretID, testNamespace, testK8sSecret), nil)
		kubeProvider := newKubeProvider(&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: testK8sSecret, Namespace: testNamespace},
			Data:       map[string][]byte{"password": []byte("s3cr3t")},
		})

		storeLoader := configloader.NewMockSecretsLoader(ctrl)
		storeLoader.EXPECT().
			LoadSecrets(gomock.Any(), map[string][]string{testStoreID: nil}).
			Return(map[string]recipes.SecretData{
				testStoreID: {Type: "generic", Data: map[string]string{"key": "value"}},
			}, nil)

		loader := NewDispatchingLoader(storeLoader, databaseClient, kubeProvider)

		result, err := loader.LoadSecrets(context.Background(), map[string][]string{
			testSecretID: nil,
			testStoreID:  nil,
		})
		require.NoError(t, err)
		require.Equal(t, map[string]string{"password": "s3cr3t"}, result[testSecretID].Data)
		require.Equal(t, map[string]string{"key": "value"}, result[testStoreID].Data)
	})

	t.Run("errors when a non-UDT secret is requested but no store loader is configured", func(t *testing.T) {
		loader := NewDispatchingLoader(nil, nil, nil)

		_, err := loader.LoadSecrets(context.Background(), map[string][]string{testStoreID: nil})
		require.Error(t, err)
		require.Contains(t, err.Error(), "no secret store loader is configured")
	})

	t.Run("errors on an unparseable secret ID", func(t *testing.T) {
		loader := NewDispatchingLoader(nil, nil, nil)

		_, err := loader.LoadSecrets(context.Background(), map[string][]string{testInvalidID: nil})
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to parse secret resource ID")
	})
}
