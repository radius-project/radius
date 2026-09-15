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
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/stretchr/testify/require"

	sdk_cred "github.com/radius-project/radius/pkg/ucp/credentials"
)

type mockProvider struct {
	fakeCredential *sdk_cred.AzureCredential
}

type failingTokenCredential struct {
	err error
}

func (c failingTokenCredential) GetToken(context.Context, policy.TokenRequestOptions) (azcore.AccessToken, error) {
	return azcore.AccessToken{}, c.err
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

		err = c.refreshCredentials(t.Context())
		require.Error(t, err)
	})

	t.Run("do not refresh service principal credential", func(t *testing.T) {
		p := newServicePrincipalMockProvider()
		c, err := NewUCPCredential(UCPCredentialOptions{Provider: p})
		require.NoError(t, err)

		err = c.refreshCredentials(t.Context())
		require.NoError(t, err)
		require.False(t, c.isExpired())
	})

	t.Run("same service principal credentials", func(t *testing.T) {
		p := newServicePrincipalMockProvider()
		c, err := NewUCPCredential(UCPCredentialOptions{Provider: p})
		require.NoError(t, err)

		err = c.refreshCredentials(t.Context())
		require.NoError(t, err)

		// reset next refresh time.
		c.nextExpiry.Store(0)
		require.True(t, c.isExpired())
		old := c.tokenCred

		err = c.refreshCredentials(t.Context())
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

		err = c.refreshCredentials(t.Context())
		require.Error(t, err)
	})

	t.Run("do not refresh workload identity credential", func(t *testing.T) {
		p := newWorkloadIdentityMockProvider()
		c, err := NewUCPCredential(UCPCredentialOptions{Provider: p, TokenFilePath: "/var/run/secrets/azure/tokens/azure-identity-token"})
		require.NoError(t, err)

		err = c.refreshCredentials(t.Context())
		require.NoError(t, err)
		require.False(t, c.isExpired())
	})

	t.Run("missing workload identity token file", func(t *testing.T) {
		t.Setenv(azureFederatedTokenFileEnvironmentVariable, "")
		p := newWorkloadIdentityMockProvider()
		c, err := NewUCPCredential(UCPCredentialOptions{Provider: p})
		require.NoError(t, err)

		err = c.refreshCredentials(t.Context())
		require.ErrorContains(t, err, "no Azure workload identity token file specified")
	})

	t.Run("same workload identity credentials", func(t *testing.T) {
		p := newWorkloadIdentityMockProvider()
		c, err := NewUCPCredential(UCPCredentialOptions{Provider: p, TokenFilePath: "/var/run/secrets/azure/tokens/azure-identity-token"})
		require.NoError(t, err)

		err = c.refreshCredentials(t.Context())
		require.NoError(t, err)

		// reset next refresh time.
		c.nextExpiry.Store(0)
		require.True(t, c.isExpired())
		old := c.tokenCred

		err = c.refreshCredentials(t.Context())
		require.NoError(t, err)
		require.False(t, c.isExpired())
		require.Equal(t, old, c.tokenCred)
	})
}

func Test_ReadWorkloadIdentityAssertion_ReadsRotatedToken(t *testing.T) {
	tokenFilePath := filepath.Join(t.TempDir(), "azure-identity-token")
	require.NoError(t, os.WriteFile(tokenFilePath, []byte("first-assertion\n"), 0600))

	assertion, err := readWorkloadIdentityAssertion(tokenFilePath)
	require.NoError(t, err)
	require.Equal(t, "first-assertion", assertion)

	// The workflow rewrites the mounted Secret while the pod keeps running, so a later
	// exchange must observe the rotated assertion rather than a cached copy.
	require.NoError(t, os.WriteFile(tokenFilePath, []byte("second-assertion"), 0600))

	assertion, err = readWorkloadIdentityAssertion(tokenFilePath)
	require.NoError(t, err)
	require.Equal(t, "second-assertion", assertion)
}

func Test_GetToken_WorkloadIdentityAuthenticationError(t *testing.T) {
	authErr := errors.New("ClientAssertionCredential authentication failed")
	tokenFilePath := filepath.Join(t.TempDir(), "azure-identity-token")
	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"exp":1}`))
	require.NoError(t, os.WriteFile(tokenFilePath, []byte("header."+payload+".signature"), 0600))
	t.Setenv(azureFederatedTokenFileEnvironmentVariable, tokenFilePath)

	credential := &UCPCredential{
		options: UCPCredentialOptions{},
		credential: &sdk_cred.AzureCredential{
			Kind:             sdk_cred.AzureWorkloadIdentityCredentialKind,
			WorkloadIdentity: &sdk_cred.AzureWorkloadIdentityCredential{},
		},
		tokenCred: failingTokenCredential{err: authErr},
	}
	credential.nextExpiry.Store(time.Now().Add(time.Minute).Unix())

	_, err := credential.GetToken(t.Context(), policy.TokenRequestOptions{})
	require.ErrorContains(t, err, "federated assertion")
	require.ErrorContains(t, err, "expired at 1970-01-01T00:00:01Z")
	require.ErrorContains(t, err, "refresh the mounted token")
	require.ErrorIs(t, err, authErr)
}

func Test_WorkloadIdentityAuthenticationError_DoesNotMisdiagnoseAssertion(t *testing.T) {
	authErr := errors.New("authentication failed")
	unexpiredPayload := base64.RawURLEncoding.EncodeToString([]byte(`{"exp":4102444800}`))

	tests := map[string]string{
		"malformed token": "not-a-jwt",
		"unexpired token": "header." + unexpiredPayload + ".signature",
	}

	for name, token := range tests {
		t.Run(name, func(t *testing.T) {
			tokenFilePath := filepath.Join(t.TempDir(), "azure-identity-token")
			require.NoError(t, os.WriteFile(tokenFilePath, []byte(token), 0600))

			err := workloadIdentityAuthenticationError(tokenFilePath, authErr)
			require.ErrorContains(t, err, "azure workload identity authentication failed")
			require.NotContains(t, err.Error(), "refresh the mounted token")
			require.ErrorIs(t, err, authErr)
		})
	}
}
