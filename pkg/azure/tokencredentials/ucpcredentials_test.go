// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package tokencredentials

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	sdk "github.com/project-radius/radius/pkg/sdk/credentials"
)

type mockProvider struct {
	fakeCredential *sdk.AzureCredential
}

// Fetch gets the Azure credentials from secret storage.
func (p *mockProvider) Fetch(ctx context.Context, planeName, name string) (*sdk.AzureCredential, error) {
	if p.fakeCredential == nil {
		return nil, errors.New("failed to fetch credential")
	}
	return p.fakeCredential, nil
}

func newMockProvider() *mockProvider {
	return &mockProvider{
		fakeCredential: &sdk.AzureCredential{
			ClientID:     "fakeid",
			TenantID:     "fakeid",
			ClientSecret: "fakeSecret",
		},
	}
}

func TestNewUCPCredential(t *testing.T) {
	_, err := NewUCPCredential(UCPCredentialOptions{})
	require.Error(t, err)

	c, err := NewUCPCredential(UCPCredentialOptions{Provider: newMockProvider()})
	require.NoError(t, err)
	require.Equal(t, DefaultExpireDuration, c.options.Duration)
	require.True(t, c.isRefreshRequired())
}

func TestRefreshCredentials(t *testing.T) {
	t.Run("invalid credential", func(t *testing.T) {
		p := newMockProvider()
		c, err := NewUCPCredential(UCPCredentialOptions{Provider: p})
		require.NoError(t, err)
		p.fakeCredential.ClientID = ""

		err = c.refreshCredentials(context.TODO())
		require.Error(t, err)
	})

	t.Run("do not refresh credential", func(t *testing.T) {
		p := newMockProvider()
		c, err := NewUCPCredential(UCPCredentialOptions{Provider: p})
		require.NoError(t, err)

		err = c.refreshCredentials(context.TODO())
		require.NoError(t, err)
		require.False(t, c.isRefreshRequired())
	})

	t.Run("same credentials", func(t *testing.T) {
		p := newMockProvider()
		c, err := NewUCPCredential(UCPCredentialOptions{Provider: p})
		require.NoError(t, err)

		err = c.refreshCredentials(context.TODO())
		require.NoError(t, err)

		// reset next refresh time.
		c.nextRefresh.Store(0)
		require.True(t, c.isRefreshRequired())
		old := c.tokenCred

		err = c.refreshCredentials(context.TODO())
		require.NoError(t, err)
		require.False(t, c.isRefreshRequired())
		require.Equal(t, old, c.tokenCred)
	})
}
