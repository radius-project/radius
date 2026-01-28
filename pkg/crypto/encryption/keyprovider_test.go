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

package encryption

import (
	"context"
	"testing"

	"github.com/radius-project/radius/test/k8sutil"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kubectl/pkg/scheme"
	controller_runtime "sigs.k8s.io/controller-runtime/pkg/client"
)

func TestKubernetesKeyProvider_GetKey(t *testing.T) {
	ctx := context.Background()
	validKey := make([]byte, KeySize)
	for i := range validKey {
		validKey[i] = byte(i)
	}

	tests := []struct {
		name       string
		setupFunc  func(k8sClient controller_runtime.Client)
		opts       *KubernetesKeyProviderOptions
		wantErr    error
		wantKey    []byte
		wantErrMsg string
	}{
		{
			name: "success-with-default-options",
			setupFunc: func(k8sClient controller_runtime.Client) {
				secret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      DefaultEncryptionKeySecretName,
						Namespace: RadiusNamespace,
					},
					Data: map[string][]byte{
						DefaultEncryptionKeySecretKey: validKey,
					},
				}
				err := k8sClient.Create(ctx, secret)
				require.NoError(t, err)
			},
			opts:    nil,
			wantKey: validKey,
		},
		{
			name: "success-with-custom-options",
			setupFunc: func(k8sClient controller_runtime.Client) {
				secret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "custom-secret",
						Namespace: "custom-namespace",
					},
					Data: map[string][]byte{
						"custom-key": validKey,
					},
				}
				err := k8sClient.Create(ctx, secret)
				require.NoError(t, err)
			},
			opts: &KubernetesKeyProviderOptions{
				SecretName: "custom-secret",
				SecretKey:  "custom-key",
				Namespace:  "custom-namespace",
			},
			wantKey: validKey,
		},
		{
			name:       "error-secret-not-found",
			setupFunc:  func(k8sClient controller_runtime.Client) {},
			opts:       nil,
			wantErr:    ErrKeyNotFound,
			wantErrMsg: "not found",
		},
		{
			name: "error-key-not-in-secret",
			setupFunc: func(k8sClient controller_runtime.Client) {
				secret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      DefaultEncryptionKeySecretName,
						Namespace: RadiusNamespace,
					},
					Data: map[string][]byte{
						"wrong-key": validKey,
					},
				}
				err := k8sClient.Create(ctx, secret)
				require.NoError(t, err)
			},
			opts:       nil,
			wantErr:    ErrKeyNotFound,
			wantErrMsg: "not found in secret",
		},
		{
			name: "error-invalid-key-size",
			setupFunc: func(k8sClient controller_runtime.Client) {
				secret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      DefaultEncryptionKeySecretName,
						Namespace: RadiusNamespace,
					},
					Data: map[string][]byte{
						DefaultEncryptionKeySecretKey: make([]byte, 16), // Too short
					},
				}
				err := k8sClient.Create(ctx, secret)
				require.NoError(t, err)
			},
			opts:       nil,
			wantErr:    ErrKeyLoadFailed,
			wantErrMsg: "invalid size",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k8sClient := k8sutil.NewFakeKubeClient(scheme.Scheme)
			tt.setupFunc(k8sClient)

			provider := NewKubernetesKeyProvider(k8sClient, tt.opts)
			key, err := provider.GetKey(ctx)

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				if tt.wantErrMsg != "" {
					require.Contains(t, err.Error(), tt.wantErrMsg)
				}
				require.Nil(t, key)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.wantKey, key)
			}
		})
	}
}

func TestNewKubernetesKeyProvider_DefaultOptions(t *testing.T) {
	k8sClient := k8sutil.NewFakeKubeClient(scheme.Scheme)

	// Test with nil options
	provider := NewKubernetesKeyProvider(k8sClient, nil)
	require.Equal(t, DefaultEncryptionKeySecretName, provider.secretName)
	require.Equal(t, DefaultEncryptionKeySecretKey, provider.secretKey)
	require.Equal(t, RadiusNamespace, provider.namespace)

	// Test with empty options
	provider = NewKubernetesKeyProvider(k8sClient, &KubernetesKeyProviderOptions{})
	require.Equal(t, DefaultEncryptionKeySecretName, provider.secretName)
	require.Equal(t, DefaultEncryptionKeySecretKey, provider.secretKey)
	require.Equal(t, RadiusNamespace, provider.namespace)
}

func TestInMemoryKeyProvider(t *testing.T) {
	ctx := context.Background()
	validKey := make([]byte, KeySize)
	for i := range validKey {
		validKey[i] = byte(i)
	}

	t.Run("success", func(t *testing.T) {
		provider, err := NewInMemoryKeyProvider(validKey)
		require.NoError(t, err)

		key, err := provider.GetKey(ctx)
		require.NoError(t, err)
		require.Equal(t, validKey, key)
	})

	t.Run("error-invalid-key-size", func(t *testing.T) {
		_, err := NewInMemoryKeyProvider(make([]byte, 16))
		require.ErrorIs(t, err, ErrInvalidKeySize)
	})

	t.Run("key-is-copied", func(t *testing.T) {
		originalKey := make([]byte, KeySize)
		for i := range originalKey {
			originalKey[i] = byte(i)
		}

		provider, err := NewInMemoryKeyProvider(originalKey)
		require.NoError(t, err)

		// Modify the original key
		originalKey[0] = 0xff

		// The provider's key should not be affected
		key, err := provider.GetKey(ctx)
		require.NoError(t, err)
		require.NotEqual(t, originalKey[0], key[0])
		require.Equal(t, byte(0), key[0])
	})
}

func TestKeyProviderIntegration(t *testing.T) {
	ctx := context.Background()

	// Generate a key
	key, err := GenerateKey()
	require.NoError(t, err)

	// Create an in-memory provider
	provider, err := NewInMemoryKeyProvider(key)
	require.NoError(t, err)

	// Get the key from the provider
	retrievedKey, err := provider.GetKey(ctx)
	require.NoError(t, err)

	// Create an encryptor with the retrieved key
	enc, err := NewEncryptor(retrievedKey)
	require.NoError(t, err)

	// Test encryption/decryption
	plaintext := []byte("secret data from key provider")
	encrypted, err := enc.Encrypt(plaintext, nil)
	require.NoError(t, err)

	decrypted, err := enc.Decrypt(encrypted, nil)
	require.NoError(t, err)
	require.Equal(t, plaintext, decrypted)
}
