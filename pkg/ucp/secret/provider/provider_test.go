// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package provider

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetClient_ValidType(t *testing.T) {
	secretProvider := NewSecretProvider(SecretProviderOptions{
		Provider: TypeKubernetesSecrets,
	})
	etcdSecretClient, err := secretProvider.GetSecretClient(context.TODO())
	require.NoError(t, err)
	require.NotNil(t, etcdSecretClient)
	k8SecretClient, err := secretProvider.GetSecretClient(context.TODO())
	require.NoError(t, err)
	require.NotNil(t, k8SecretClient)
}

func TestGetClient_InvalidType(t *testing.T) {
	secretProvider := NewSecretProvider(SecretProviderOptions{
		Provider: "invalid_client_type",
	})
	client, err := secretProvider.GetSecretClient(context.TODO())
	require.Equal(t, err, ErrUnsupportedSecretProvider)
	require.Nil(t, client)
}
