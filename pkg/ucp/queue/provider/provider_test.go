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

func TestGetClient_ValidQueue(t *testing.T) {
	p := New("Applications.Core", QueueProviderOptions{
		Provider: TypeInmemory,
		InMemory: &InMemoryQueueOptions{},
	})

	oldcli, err := p.GetClient(context.TODO())
	require.NoError(t, err)
	require.NotNil(t, oldcli)
	newcli, err := p.GetClient(context.TODO())
	require.NoError(t, err)
	require.Equal(t, oldcli, newcli)
}

func TestGetClient_InvalidQueue(t *testing.T) {
	p := New("Applications.Core", QueueProviderOptions{
		Provider: QueueProviderType("undefined"),
	})

	_, err := p.GetClient(context.TODO())
	require.ErrorIs(t, ErrUnsupportedStorageProvider, err)
}
