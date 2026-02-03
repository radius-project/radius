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
	"errors"
	"fmt"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	k8s_error "k8s.io/apimachinery/pkg/api/errors"
	controller_runtime "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// DefaultEncryptionKeySecretName is the default name of the Kubernetes Secret containing the encryption key.

	// This is the Secret's name, not actual credentials.
	DefaultEncryptionKeySecretName = "radius-encryption-key" //nolint:gosec // This is a Secret name, not credentials

	// DefaultEncryptionKeySecretKey is the key within the Secret that contains the versioned key store JSON.
	DefaultEncryptionKeySecretKey = "keys.json"

	// RadiusNamespace is the namespace where Radius secrets are stored.
	RadiusNamespace = "radius-system"
)

// KeyStore represents a versioned key store containing multiple encryption keys.
// This structure matches the format used by the key rotation CronJob.
type KeyStore struct {
	// CurrentVersion is the version number of the key to use for encryption.
	CurrentVersion int `json:"currentVersion"`
	// Keys is a map of version number (as string) to key data.
	Keys map[string]KeyData `json:"keys"`
}

// KeyData represents a single encryption key with its metadata.
type KeyData struct {
	// Key is the base64-encoded encryption key.
	Key string `json:"key"`
	// Version is the version number of this key.
	Version int `json:"version"`
	// CreatedAt is the timestamp when this key was created (RFC3339 format).
	CreatedAt string `json:"createdAt"`
	// ExpiresAt is the timestamp when this key expires (RFC3339 format).
	ExpiresAt string `json:"expiresAt"`
}

var (
	// ErrKeyNotFound is returned when the encryption key is not found.
	ErrKeyNotFound = errors.New("encryption key not found")

	// ErrKeyLoadFailed is returned when loading the encryption key fails.
	ErrKeyLoadFailed = errors.New("failed to load encryption key")

	// ErrKeyVersionNotFound is returned when a specific key version is not found.
	ErrKeyVersionNotFound = errors.New("key version not found")
)

// KeyProvider defines the interface for retrieving encryption keys.
// It supports versioned keys to enable key rotation without data loss.
//
//go:generate mockgen -typed -destination=./mock_keyprovider.go -package=encryption -self_package github.com/radius-project/radius/pkg/crypto/encryption github.com/radius-project/radius/pkg/crypto/encryption KeyProvider
type KeyProvider interface {
	// GetCurrentKey retrieves the current (latest) encryption key for encrypting new data.
	// Returns the key bytes, the version number, and any error.
	// Returns ErrKeyNotFound if no key exists.
	GetCurrentKey(ctx context.Context) (key []byte, version int, err error)

	// GetKeyByVersion retrieves a specific key version for decryption.
	// This is used when decrypting data that was encrypted with an older key.
	// Returns ErrKeyVersionNotFound if the specified version does not exist.
	GetKeyByVersion(ctx context.Context, version int) ([]byte, error)
}

// KubernetesKeyProvider implements KeyProvider by loading the encryption key from a Kubernetes Secret.
type KubernetesKeyProvider struct {
	client     controller_runtime.Client
	secretName string
	secretKey  string
	namespace  string
}

// KubernetesKeyProviderOptions contains options for creating a KubernetesKeyProvider.
type KubernetesKeyProviderOptions struct {
	// SecretName is the name of the Kubernetes Secret containing the encryption key.
	// Defaults to DefaultEncryptionKeySecretName if not specified.
	SecretName string

	// SecretKey is the key within the Secret that contains the encryption key.
	// Defaults to DefaultEncryptionKeySecretKey if not specified.
	SecretKey string

	// Namespace is the namespace where the Secret is located.
	// Defaults to RadiusNamespace if not specified.
	Namespace string
}

// NewKubernetesKeyProvider creates a new KubernetesKeyProvider with the given Kubernetes client and options.
func NewKubernetesKeyProvider(client controller_runtime.Client, opts *KubernetesKeyProviderOptions) *KubernetesKeyProvider {
	secretName := DefaultEncryptionKeySecretName
	secretKey := DefaultEncryptionKeySecretKey
	namespace := RadiusNamespace

	if opts != nil {
		if opts.SecretName != "" {
			secretName = opts.SecretName
		}
		if opts.SecretKey != "" {
			secretKey = opts.SecretKey
		}
		if opts.Namespace != "" {
			namespace = opts.Namespace
		}
	}

	return &KubernetesKeyProvider{
		client:     client,
		secretName: secretName,
		secretKey:  secretKey,
		namespace:  namespace,
	}
}

// loadKeyStore loads and parses the key store from the Kubernetes Secret.
func (p *KubernetesKeyProvider) loadKeyStore(ctx context.Context) (*KeyStore, error) {
	secret := &corev1.Secret{}
	objectKey := controller_runtime.ObjectKey{
		Name:      p.secretName,
		Namespace: p.namespace,
	}

	if err := p.client.Get(ctx, objectKey, secret); err != nil {
		if k8s_error.IsNotFound(err) {
			return nil, fmt.Errorf("%w: secret %s/%s not found", ErrKeyNotFound, p.namespace, p.secretName)
		}
		return nil, fmt.Errorf("%w: %v", ErrKeyLoadFailed, err)
	}

	keysJSON, ok := secret.Data[p.secretKey]
	if !ok {
		return nil, fmt.Errorf("%w: key %q not found in secret %s/%s", ErrKeyNotFound, p.secretKey, p.namespace, p.secretName)
	}

	var keyStore KeyStore
	if err := json.Unmarshal(keysJSON, &keyStore); err != nil {
		return nil, fmt.Errorf("%w: failed to parse key store JSON: %v", ErrKeyLoadFailed, err)
	}

	return &keyStore, nil
}

// GetCurrentKey retrieves the current encryption key from the Kubernetes Secret.
// Returns the key bytes, version number, and any error.
func (p *KubernetesKeyProvider) GetCurrentKey(ctx context.Context) ([]byte, int, error) {
	keyStore, err := p.loadKeyStore(ctx)
	if err != nil {
		return nil, 0, err
	}

	versionStr := strconv.Itoa(keyStore.CurrentVersion)
	keyData, ok := keyStore.Keys[versionStr]
	if !ok {
		return nil, 0, fmt.Errorf("%w: current version %d not found in key store", ErrKeyVersionNotFound, keyStore.CurrentVersion)
	}

	key, err := base64.StdEncoding.DecodeString(keyData.Key)
	if err != nil {
		return nil, 0, fmt.Errorf("%w: failed to decode key: %v", ErrKeyLoadFailed, err)
	}

	if len(key) != KeySize {
		return nil, 0, fmt.Errorf("%w: key version %d has invalid size (expected %d bytes, got %d)", ErrKeyLoadFailed, keyStore.CurrentVersion, KeySize, len(key))
	}

	return key, keyStore.CurrentVersion, nil
}

// GetKeyByVersion retrieves a specific key version from the Kubernetes Secret.
func (p *KubernetesKeyProvider) GetKeyByVersion(ctx context.Context, version int) ([]byte, error) {
	keyStore, err := p.loadKeyStore(ctx)
	if err != nil {
		return nil, err
	}

	versionStr := strconv.Itoa(version)
	keyData, ok := keyStore.Keys[versionStr]
	if !ok {
		return nil, fmt.Errorf("%w: version %d not found in key store", ErrKeyVersionNotFound, version)
	}

	key, err := base64.StdEncoding.DecodeString(keyData.Key)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to decode key version %d: %v", ErrKeyLoadFailed, version, err)
	}

	if len(key) != KeySize {
		return nil, fmt.Errorf("%w: key version %d has invalid size (expected %d bytes, got %d)", ErrKeyLoadFailed, version, KeySize, len(key))
	}

	return key, nil
}

// InMemoryKeyProvider implements KeyProvider with in-memory versioned keys.
// This is useful for testing environments.
type InMemoryKeyProvider struct {
	keys           map[int][]byte
	currentVersion int
}

// NewInMemoryKeyProvider creates a new InMemoryKeyProvider with a single key at version 1.
func NewInMemoryKeyProvider(key []byte) (*InMemoryKeyProvider, error) {
	if len(key) != KeySize {
		return nil, ErrInvalidKeySize
	}
	keyCopy := make([]byte, KeySize)
	copy(keyCopy, key)
	return &InMemoryKeyProvider{
		keys:           map[int][]byte{1: keyCopy},
		currentVersion: 1,
	}, nil
}

// NewInMemoryKeyProviderWithVersions creates a new InMemoryKeyProvider with multiple versioned keys.
func NewInMemoryKeyProviderWithVersions(keys map[int][]byte, currentVersion int) (*InMemoryKeyProvider, error) {
	if len(keys) == 0 {
		return nil, ErrKeyNotFound
	}

	keysCopy := make(map[int][]byte, len(keys))
	for version, key := range keys {
		if len(key) != KeySize {
			return nil, fmt.Errorf("%w: key version %d", ErrInvalidKeySize, version)
		}
		keyCopy := make([]byte, KeySize)
		copy(keyCopy, key)
		keysCopy[version] = keyCopy
	}

	if _, ok := keysCopy[currentVersion]; !ok {
		return nil, fmt.Errorf("%w: current version %d", ErrKeyVersionNotFound, currentVersion)
	}

	return &InMemoryKeyProvider{
		keys:           keysCopy,
		currentVersion: currentVersion,
	}, nil
}

// GetCurrentKey returns the current encryption key and its version.
func (p *InMemoryKeyProvider) GetCurrentKey(ctx context.Context) ([]byte, int, error) {
	if len(p.keys) == 0 {
		return nil, 0, ErrKeyNotFound
	}

	key, ok := p.keys[p.currentVersion]
	if !ok {
		return nil, 0, fmt.Errorf("%w: current version %d", ErrKeyVersionNotFound, p.currentVersion)
	}

	// Return a copy to prevent mutation of the internal key
	return append([]byte(nil), key...), p.currentVersion, nil
}

// GetKeyByVersion returns the key for a specific version.
func (p *InMemoryKeyProvider) GetKeyByVersion(ctx context.Context, version int) ([]byte, error) {
	if p.keys == nil {
		return nil, ErrKeyNotFound
	}

	key, ok := p.keys[version]
	if !ok {
		return nil, fmt.Errorf("%w: version %d", ErrKeyVersionNotFound, version)
	}

	// Return a copy to prevent mutation of the internal key
	return append([]byte(nil), key...), nil
}

// AddKey adds a new key version to the provider (useful for testing rotation).
func (p *InMemoryKeyProvider) AddKey(version int, key []byte) error {
	if len(key) != KeySize {
		return ErrInvalidKeySize
	}
	keyCopy := make([]byte, KeySize)
	copy(keyCopy, key)
	p.keys[version] = keyCopy
	return nil
}

// SetCurrentVersion sets the current version (useful for testing rotation).
func (p *InMemoryKeyProvider) SetCurrentVersion(version int) error {
	if _, ok := p.keys[version]; !ok {
		return fmt.Errorf("%w: version %d", ErrKeyVersionNotFound, version)
	}
	p.currentVersion = version
	return nil
}
