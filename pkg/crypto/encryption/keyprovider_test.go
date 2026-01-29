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
	"encoding/base64"
	"encoding/json"
	"strconv"
	"testing"

	"github.com/radius-project/radius/test/k8sutil"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kubectl/pkg/scheme"
	controller_runtime "sigs.k8s.io/controller-runtime/pkg/client"
)

// createTestKeyStore creates a KeyStore JSON for testing
func createTestKeyStore(t *testing.T, keys map[int][]byte, currentVersion int) []byte {
	keyStore := KeyStore{
		CurrentVersion: currentVersion,
		Keys:           make(map[string]KeyData),
	}
	for version, key := range keys {
		versionStr := strconv.Itoa(version)
		keyStore.Keys[versionStr] = KeyData{
			Key:       base64.StdEncoding.EncodeToString(key),
			Version:   version,
			CreatedAt: "2024-01-01T00:00:00Z",
			ExpiresAt: "2024-04-01T00:00:00Z",
		}
	}
	data, err := json.Marshal(keyStore)
	require.NoError(t, err)
	return data
}

func TestKubernetesKeyProvider_GetCurrentKey(t *testing.T) {
	ctx := context.Background()
	validKey := make([]byte, KeySize)
	for i := range validKey {
		validKey[i] = byte(i)
	}

	tests := []struct {
		name        string
		setupFunc   func(k8sClient controller_runtime.Client)
		opts        *KubernetesKeyProviderOptions
		wantErr     error
		wantKey     []byte
		wantVersion int
		wantErrMsg  string
	}{
		{
			name: "success-with-default-options",
			setupFunc: func(k8sClient controller_runtime.Client) {
				keyStoreJSON := createTestKeyStore(t, map[int][]byte{1: validKey}, 1)
				secret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      DefaultEncryptionKeySecretName,
						Namespace: RadiusNamespace,
					},
					Data: map[string][]byte{
						DefaultEncryptionKeySecretKey: keyStoreJSON,
					},
				}
				err := k8sClient.Create(ctx, secret)
				require.NoError(t, err)
			},
			opts:        nil,
			wantKey:     validKey,
			wantVersion: 1,
		},
		{
			name: "success-with-multiple-versions",
			setupFunc: func(k8sClient controller_runtime.Client) {
				key1 := make([]byte, KeySize)
				key2 := make([]byte, KeySize)
				for i := range key1 {
					key1[i] = byte(i)
					key2[i] = byte(i + 100)
				}
				keyStoreJSON := createTestKeyStore(t, map[int][]byte{1: key1, 2: key2}, 2)
				secret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      DefaultEncryptionKeySecretName,
						Namespace: RadiusNamespace,
					},
					Data: map[string][]byte{
						DefaultEncryptionKeySecretKey: keyStoreJSON,
					},
				}
				err := k8sClient.Create(ctx, secret)
				require.NoError(t, err)
			},
			opts: nil,
			wantKey: func() []byte {
				key := make([]byte, KeySize)
				for i := range key {
					key[i] = byte(i + 100)
				}
				return key
			}(),
			wantVersion: 2,
		},
		{
			name: "success-with-custom-options",
			setupFunc: func(k8sClient controller_runtime.Client) {
				keyStoreJSON := createTestKeyStore(t, map[int][]byte{1: validKey}, 1)
				secret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "custom-secret",
						Namespace: "custom-namespace",
					},
					Data: map[string][]byte{
						"custom-key": keyStoreJSON,
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
			wantKey:     validKey,
			wantVersion: 1,
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
						"wrong-key": []byte("{}"),
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
			name: "error-invalid-json",
			setupFunc: func(k8sClient controller_runtime.Client) {
				secret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      DefaultEncryptionKeySecretName,
						Namespace: RadiusNamespace,
					},
					Data: map[string][]byte{
						DefaultEncryptionKeySecretKey: []byte("not-valid-json"),
					},
				}
				err := k8sClient.Create(ctx, secret)
				require.NoError(t, err)
			},
			opts:       nil,
			wantErr:    ErrKeyLoadFailed,
			wantErrMsg: "failed to parse key store JSON",
		},
		{
			name: "error-current-version-not-found",
			setupFunc: func(k8sClient controller_runtime.Client) {
				keyStoreJSON := createTestKeyStore(t, map[int][]byte{1: validKey}, 99)
				secret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      DefaultEncryptionKeySecretName,
						Namespace: RadiusNamespace,
					},
					Data: map[string][]byte{
						DefaultEncryptionKeySecretKey: keyStoreJSON,
					},
				}
				err := k8sClient.Create(ctx, secret)
				require.NoError(t, err)
			},
			opts:       nil,
			wantErr:    ErrKeyVersionNotFound,
			wantErrMsg: "current version 99 not found",
		},
		{
			name: "error-invalid-key-size",
			setupFunc: func(k8sClient controller_runtime.Client) {
				shortKey := make([]byte, 16) // Too short
				keyStoreJSON := createTestKeyStore(t, map[int][]byte{1: shortKey}, 1)
				secret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      DefaultEncryptionKeySecretName,
						Namespace: RadiusNamespace,
					},
					Data: map[string][]byte{
						DefaultEncryptionKeySecretKey: keyStoreJSON,
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
			key, version, err := provider.GetCurrentKey(ctx)

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				if tt.wantErrMsg != "" {
					require.Contains(t, err.Error(), tt.wantErrMsg)
				}
				require.Nil(t, key)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.wantKey, key)
				require.Equal(t, tt.wantVersion, version)
			}
		})
	}
}

func TestKubernetesKeyProvider_GetKeyByVersion(t *testing.T) {
	ctx := context.Background()
	key1 := make([]byte, KeySize)
	key2 := make([]byte, KeySize)
	for i := range key1 {
		key1[i] = byte(i)
		key2[i] = byte(i + 100)
	}

	tests := []struct {
		name       string
		setupFunc  func(k8sClient controller_runtime.Client)
		version    int
		wantErr    error
		wantKey    []byte
		wantErrMsg string
	}{
		{
			name: "success-get-version-1",
			setupFunc: func(k8sClient controller_runtime.Client) {
				keyStoreJSON := createTestKeyStore(t, map[int][]byte{1: key1, 2: key2}, 2)
				secret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      DefaultEncryptionKeySecretName,
						Namespace: RadiusNamespace,
					},
					Data: map[string][]byte{
						DefaultEncryptionKeySecretKey: keyStoreJSON,
					},
				}
				err := k8sClient.Create(ctx, secret)
				require.NoError(t, err)
			},
			version: 1,
			wantKey: key1,
		},
		{
			name: "success-get-version-2",
			setupFunc: func(k8sClient controller_runtime.Client) {
				keyStoreJSON := createTestKeyStore(t, map[int][]byte{1: key1, 2: key2}, 2)
				secret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      DefaultEncryptionKeySecretName,
						Namespace: RadiusNamespace,
					},
					Data: map[string][]byte{
						DefaultEncryptionKeySecretKey: keyStoreJSON,
					},
				}
				err := k8sClient.Create(ctx, secret)
				require.NoError(t, err)
			},
			version: 2,
			wantKey: key2,
		},
		{
			name: "error-version-not-found",
			setupFunc: func(k8sClient controller_runtime.Client) {
				keyStoreJSON := createTestKeyStore(t, map[int][]byte{1: key1}, 1)
				secret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      DefaultEncryptionKeySecretName,
						Namespace: RadiusNamespace,
					},
					Data: map[string][]byte{
						DefaultEncryptionKeySecretKey: keyStoreJSON,
					},
				}
				err := k8sClient.Create(ctx, secret)
				require.NoError(t, err)
			},
			version:    99,
			wantErr:    ErrKeyVersionNotFound,
			wantErrMsg: "version 99 not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k8sClient := k8sutil.NewFakeKubeClient(scheme.Scheme)
			tt.setupFunc(k8sClient)

			provider := NewKubernetesKeyProvider(k8sClient, nil)
			key, err := provider.GetKeyByVersion(ctx, tt.version)

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

	t.Run("success-get-current-key", func(t *testing.T) {
		provider, err := NewInMemoryKeyProvider(validKey)
		require.NoError(t, err)

		key, version, err := provider.GetCurrentKey(ctx)
		require.NoError(t, err)
		require.Equal(t, validKey, key)
		require.Equal(t, 1, version)
	})

	t.Run("success-get-key-by-version", func(t *testing.T) {
		provider, err := NewInMemoryKeyProvider(validKey)
		require.NoError(t, err)

		key, err := provider.GetKeyByVersion(ctx, 1)
		require.NoError(t, err)
		require.Equal(t, validKey, key)
	})

	t.Run("error-invalid-key-size", func(t *testing.T) {
		_, err := NewInMemoryKeyProvider(make([]byte, 16))
		require.ErrorIs(t, err, ErrInvalidKeySize)
	})

	t.Run("error-version-not-found", func(t *testing.T) {
		provider, err := NewInMemoryKeyProvider(validKey)
		require.NoError(t, err)

		_, err = provider.GetKeyByVersion(ctx, 99)
		require.ErrorIs(t, err, ErrKeyVersionNotFound)
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
		key, _, err := provider.GetCurrentKey(ctx)
		require.NoError(t, err)
		require.NotEqual(t, originalKey[0], key[0])
		require.Equal(t, byte(0), key[0])
	})
}

func TestInMemoryKeyProviderWithVersions(t *testing.T) {
	ctx := context.Background()
	key1 := make([]byte, KeySize)
	key2 := make([]byte, KeySize)
	for i := range key1 {
		key1[i] = byte(i)
		key2[i] = byte(i + 100)
	}

	t.Run("success-multiple-versions", func(t *testing.T) {
		provider, err := NewInMemoryKeyProviderWithVersions(map[int][]byte{1: key1, 2: key2}, 2)
		require.NoError(t, err)

		// Current key should be version 2
		key, version, err := provider.GetCurrentKey(ctx)
		require.NoError(t, err)
		require.Equal(t, key2, key)
		require.Equal(t, 2, version)

		// Should be able to get version 1
		key, err = provider.GetKeyByVersion(ctx, 1)
		require.NoError(t, err)
		require.Equal(t, key1, key)

		// Should be able to get version 2
		key, err = provider.GetKeyByVersion(ctx, 2)
		require.NoError(t, err)
		require.Equal(t, key2, key)
	})

	t.Run("error-empty-keys", func(t *testing.T) {
		_, err := NewInMemoryKeyProviderWithVersions(map[int][]byte{}, 1)
		require.ErrorIs(t, err, ErrKeyNotFound)
	})

	t.Run("error-current-version-not-in-keys", func(t *testing.T) {
		_, err := NewInMemoryKeyProviderWithVersions(map[int][]byte{1: key1}, 99)
		require.ErrorIs(t, err, ErrKeyVersionNotFound)
	})

	t.Run("error-invalid-key-size-in-map", func(t *testing.T) {
		_, err := NewInMemoryKeyProviderWithVersions(map[int][]byte{1: make([]byte, 16)}, 1)
		require.ErrorIs(t, err, ErrInvalidKeySize)
	})
}

func TestInMemoryKeyProvider_AddKeyAndSetVersion(t *testing.T) {
	ctx := context.Background()
	key1 := make([]byte, KeySize)
	key2 := make([]byte, KeySize)
	for i := range key1 {
		key1[i] = byte(i)
		key2[i] = byte(i + 100)
	}

	provider, err := NewInMemoryKeyProvider(key1)
	require.NoError(t, err)

	// Initial state: version 1
	key, version, err := provider.GetCurrentKey(ctx)
	require.NoError(t, err)
	require.Equal(t, key1, key)
	require.Equal(t, 1, version)

	// Add version 2
	err = provider.AddKey(2, key2)
	require.NoError(t, err)

	// Version 2 should be accessible but not current
	key, err = provider.GetKeyByVersion(ctx, 2)
	require.NoError(t, err)
	require.Equal(t, key2, key)

	_, version, err = provider.GetCurrentKey(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, version) // Still version 1

	// Set current to version 2
	err = provider.SetCurrentVersion(2)
	require.NoError(t, err)

	key, version, err = provider.GetCurrentKey(ctx)
	require.NoError(t, err)
	require.Equal(t, key2, key)
	require.Equal(t, 2, version)

	// Error: set version that doesn't exist
	err = provider.SetCurrentVersion(99)
	require.ErrorIs(t, err, ErrKeyVersionNotFound)

	// Error: add key with invalid size
	err = provider.AddKey(3, make([]byte, 16))
	require.ErrorIs(t, err, ErrInvalidKeySize)
}

func TestKeyProviderIntegration(t *testing.T) {
	ctx := context.Background()

	// Generate keys
	key1, err := GenerateKey()
	require.NoError(t, err)
	key2, err := GenerateKey()
	require.NoError(t, err)

	// Create an in-memory provider with multiple versions
	provider, err := NewInMemoryKeyProviderWithVersions(map[int][]byte{1: key1, 2: key2}, 2)
	require.NoError(t, err)

	// Get the current key from the provider
	retrievedKey, version, err := provider.GetCurrentKey(ctx)
	require.NoError(t, err)
	require.Equal(t, 2, version)

	// Create an encryptor with the retrieved key and version
	enc, err := NewEncryptorWithVersion(retrievedKey, version)
	require.NoError(t, err)

	// Test encryption/decryption
	plaintext := []byte("secret data from key provider")
	encrypted, err := enc.Encrypt(plaintext, nil)
	require.NoError(t, err)

	// Verify the encrypted data contains the version
	encVersion, err := GetEncryptedDataVersion(encrypted)
	require.NoError(t, err)
	require.Equal(t, 2, encVersion)

	// Decrypt with the same encryptor
	decrypted, err := enc.Decrypt(encrypted, nil)
	require.NoError(t, err)
	require.Equal(t, plaintext, decrypted)

	// Simulate decryption with old key version
	// Get the old key
	oldKey, err := provider.GetKeyByVersion(ctx, 1)
	require.NoError(t, err)
	oldEnc, err := NewEncryptorWithVersion(oldKey, 1)
	require.NoError(t, err)

	// Encrypt with old key
	oldEncrypted, err := oldEnc.Encrypt([]byte("old secret"), nil)
	require.NoError(t, err)

	// Verify version is 1
	oldVersion, err := GetEncryptedDataVersion(oldEncrypted)
	require.NoError(t, err)
	require.Equal(t, 1, oldVersion)

	// Decrypt with old key
	oldDecrypted, err := oldEnc.Decrypt(oldEncrypted, nil)
	require.NoError(t, err)
	require.Equal(t, []byte("old secret"), oldDecrypted)
}
