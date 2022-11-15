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

func TestGetClient_InvalidType(t *testing.T) {
	secretProvider := NewSecretProvider(SecretProviderOptions{
		Provider: "invalid_client_type",
	})
	client, err := secretProvider.GetClient(context.TODO())
	require.Equal(t, err, ErrUnsupportedSecretProvider)
	require.Nil(t, client)
}
