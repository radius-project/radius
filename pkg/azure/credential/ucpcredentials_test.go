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

package credential

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	sdk_cred "github.com/radius-project/radius/pkg/ucp/credentials"
)

type mockProvider struct {
	fakeCredential *sdk_cred.AzureCredential
}

// Fetch gets the Azure credentials from secret storage.
func (p *mockProvider) Fetch(ctx context.Context, planeName, name string) (*sdk_cred.AzureCredential, error) {
	if p.fakeCredential == nil {
		return nil, errors.New("failed to fetch credential")
	}
	return p.fakeCredential, nil
}

func newServicePrincipalMockProvider() *mockProvider {
	return &mockProvider{
		fakeCredential: &sdk_cred.AzureCredential{
			Kind: sdk_cred.AzureServicePrincipalCredentialKind,
			ServicePrincipal: &sdk_cred.AzureServicePrincipalCredential{
				ClientID:     "fakeClientID",
				TenantID:     "fakeTenantID",
				ClientSecret: "fakeSecret",
			},
		},
	}
}

func newWorkloadIdentityMockProvider() *mockProvider {
	return &mockProvider{
		fakeCredential: &sdk_cred.AzureCredential{
			Kind: sdk_cred.AzureWorkloadIdentityCredentialKind,
			WorkloadIdentity: &sdk_cred.AzureWorkloadIdentityCredential{
				ClientID: "fakeClientID",
				TenantID: "fakeTenantID",
			},
		},
	}
}

func Test_NewUCPCredential_AzureServicePrincipal(t *testing.T) {
	_, err := NewUCPCredential(UCPCredentialOptions{})
	require.Error(t, err)

	c, err := NewUCPCredential(UCPCredentialOptions{Provider: newServicePrincipalMockProvider()})
	require.NoError(t, err)
	require.Equal(t, DefaultExpireDuration, c.options.Duration)
	require.True(t, c.isExpired())
}

func Test_NewUCPCredential_WorkloadIdentity(t *testing.T) {
	_, err := NewUCPCredential(UCPCredentialOptions{})
	require.Error(t, err)

	c, err := NewUCPCredential(UCPCredentialOptions{Provider: newWorkloadIdentityMockProvider()})
	require.NoError(t, err)
	require.Equal(t, DefaultExpireDuration, c.options.Duration)
	require.True(t, c.isExpired())
}

func Test_RefreshCredentials_ServicePrincipal(t *testing.T) {
	t.Run("invalid service principal credential", func(t *testing.T) {
		p := newServicePrincipalMockProvider()
		c, err := NewUCPCredential(UCPCredentialOptions{Provider: p})
		require.NoError(t, err)
		p.fakeCredential.ServicePrincipal.ClientID = ""

		err = c.refreshCredentials(context.TODO())
		require.Error(t, err)
	})

	t.Run("do not refresh service principal credential", func(t *testing.T) {
		p := newServicePrincipalMockProvider()
		c, err := NewUCPCredential(UCPCredentialOptions{Provider: p})
		require.NoError(t, err)

		err = c.refreshCredentials(context.TODO())
		require.NoError(t, err)
		require.False(t, c.isExpired())
	})

	t.Run("same service principal credentials", func(t *testing.T) {
		p := newServicePrincipalMockProvider()
		c, err := NewUCPCredential(UCPCredentialOptions{Provider: p})
		require.NoError(t, err)

		err = c.refreshCredentials(context.TODO())
		require.NoError(t, err)

		// reset next refresh time.
		c.nextExpiry.Store(0)
		require.True(t, c.isExpired())
		old := c.tokenCred

		err = c.refreshCredentials(context.TODO())
		require.NoError(t, err)
		require.False(t, c.isExpired())
		require.Equal(t, old, c.tokenCred)
	})
}

func Test_RefreshCredentials_WorkloadIdentity(t *testing.T) {
	t.Run("invalid workload identity credential", func(t *testing.T) {
		p := newWorkloadIdentityMockProvider()
		c, err := NewUCPCredential(UCPCredentialOptions{Provider: p})
		require.NoError(t, err)
		p.fakeCredential.WorkloadIdentity.ClientID = ""

		err = c.refreshCredentials(context.TODO())
		require.Error(t, err)
	})

	t.Run("do not refresh workload identity credential", func(t *testing.T) {
		p := newWorkloadIdentityMockProvider()
		c, err := NewUCPCredential(UCPCredentialOptions{Provider: p, TokenFilePath: "/var/run/secrets/azure/tokens/azure-identity-token"})
		require.NoError(t, err)

		err = c.refreshCredentials(context.TODO())
		require.NoError(t, err)
		require.False(t, c.isExpired())
	})

	t.Run("same workload identity credentials", func(t *testing.T) {
		p := newWorkloadIdentityMockProvider()
		c, err := NewUCPCredential(UCPCredentialOptions{Provider: p, TokenFilePath: "/var/run/secrets/azure/tokens/azure-identity-token"})
		require.NoError(t, err)

		err = c.refreshCredentials(context.TODO())
		require.NoError(t, err)

		// reset next refresh time.
		c.nextExpiry.Store(0)
		require.True(t, c.isExpired())
		old := c.tokenCred

		err = c.refreshCredentials(context.TODO())
		require.NoError(t, err)
		require.False(t, c.isExpired())
		require.Equal(t, old, c.tokenCred)
	})
}
