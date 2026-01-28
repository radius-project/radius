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
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	k8s_error "k8s.io/apimachinery/pkg/api/errors"
	controller_runtime "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// DefaultEncryptionKeySecretName is the default name of the Kubernetes Secret containing the encryption key.
	
	// This is the Secret's name, not actual credentials.
	DefaultEncryptionKeySecretName = "radius-encryption-key" //nolint:gosec // This is a Secret name, not credentials

	// DefaultEncryptionKeySecretKey is the key within the Secret that contains the encryption key.
	DefaultEncryptionKeySecretKey = "key"

	// RadiusNamespace is the namespace where Radius secrets are stored.
	RadiusNamespace = "radius-system"
)

var (
	// ErrKeyNotFound is returned when the encryption key is not found.
	ErrKeyNotFound = errors.New("encryption key not found")

	// ErrKeyLoadFailed is returned when loading the encryption key fails.
	ErrKeyLoadFailed = errors.New("failed to load encryption key")
)

// KeyProvider defines the interface for retrieving encryption keys.
//
//go:generate mockgen -typed -destination=./mock_keyprovider.go -package=encryption -self_package github.com/radius-project/radius/pkg/crypto/encryption github.com/radius-project/radius/pkg/crypto/encryption KeyProvider
type KeyProvider interface {
	// GetKey retrieves the encryption key.
	// Returns ErrKeyNotFound if the key does not exist.
	GetKey(ctx context.Context) ([]byte, error)
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

// GetKey retrieves the encryption key from the Kubernetes Secret.
func (p *KubernetesKeyProvider) GetKey(ctx context.Context) ([]byte, error) {
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

	key, ok := secret.Data[p.secretKey]
	if !ok {
		return nil, fmt.Errorf("%w: key %q not found in secret %s/%s", ErrKeyNotFound, p.secretKey, p.namespace, p.secretName)
	}

	if len(key) != KeySize {
		return nil, fmt.Errorf("%w: key in secret %s/%s has invalid size (expected %d bytes, got %d)", ErrKeyLoadFailed, p.namespace, p.secretName, KeySize, len(key))
	}

	return key, nil
}

// InMemoryKeyProvider implements KeyProvider with an in-memory key.
// This is useful for testing or development environments.
type InMemoryKeyProvider struct {
	key []byte
}

// NewInMemoryKeyProvider creates a new InMemoryKeyProvider with the given key.
func NewInMemoryKeyProvider(key []byte) (*InMemoryKeyProvider, error) {
	if len(key) != KeySize {
		return nil, ErrInvalidKeySize
	}
	keyCopy := make([]byte, KeySize)
	copy(keyCopy, key)
	return &InMemoryKeyProvider{key: keyCopy}, nil
}

// GetKey returns a copy of the in-memory encryption key.
// A copy is returned to prevent callers from mutating the provider's internal state.
func (p *InMemoryKeyProvider) GetKey(ctx context.Context) ([]byte, error) {
	if p.key == nil {
		return nil, ErrKeyNotFound
	}
	// Return a copy to prevent mutation of the internal key
	return append([]byte(nil), p.key...), nil
}
