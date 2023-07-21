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

package providers

import (
	"context"
	"errors"
	"testing"

	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/recipes"
	"github.com/project-radius/radius/pkg/sdk"
	ucp_credentials "github.com/project-radius/radius/pkg/ucp/credentials"
	ucp_provider "github.com/project-radius/radius/pkg/ucp/secret/provider"
	"github.com/project-radius/radius/test/testcontext"
	"github.com/stretchr/testify/require"
)

var (
	testSubscription     = "test-sub"
	testAzureCredentials = ucp_credentials.AzureCredential{
		TenantID:     "testTenantID",
		ClientSecret: "testClientSecret",
		ClientID:     "testClientID",
	}
)

type mockAzureCredentialsProvider struct {
	testCredential *ucp_credentials.AzureCredential
}

func newMockAzureCredentialsProvider() *mockAzureCredentialsProvider {
	return &mockAzureCredentialsProvider{
		testCredential: &ucp_credentials.AzureCredential{
			TenantID:     testAzureCredentials.TenantID,
			ClientSecret: testAzureCredentials.ClientSecret,
			ClientID:     testAzureCredentials.ClientID,
		},
	}
}

// Fetch returns mock Azure credentials for testing. It takes in a context, planeName and name and returns
// an AzureCredential or an error if the credentials are empty.
func (p *mockAzureCredentialsProvider) Fetch(ctx context.Context, planeName, name string) (*ucp_credentials.AzureCredential, error) {
	if p.testCredential == nil {
		return nil, errors.New("failed to fetch credential")
	}
	return p.testCredential, nil
}

func TestAzureProvider_BuildConfig(t *testing.T) {
	tests := []struct {
		desc           string
		envConfig      *recipes.Configuration
		expectedConfig map[string]any
		expectedErrMsg string
	}{
		{
			desc:      "nil config",
			envConfig: nil,
			expectedConfig: map[string]any{
				AzureFeaturesParam: map[string]any{},
			},
			expectedErrMsg: "",
		},
		{
			desc: "empty config",
			envConfig: &recipes.Configuration{
				Providers: datamodel.Providers{},
			},
			expectedConfig: map[string]any{
				AzureFeaturesParam: map[string]any{},
			},
			expectedErrMsg: "",
		},
		{
			desc: "missing Azure provider scope - no error",
			envConfig: &recipes.Configuration{
				Providers: datamodel.Providers{
					Azure: datamodel.ProvidersAzure{},
				},
			},
			expectedConfig: map[string]any{
				AzureFeaturesParam: map[string]any{},
			},
			expectedErrMsg: "code BadRequest: err Invalid Azure provider scope \"/test-sub/resourceGroups/test-rg\" is configured on the Environment, subscriptionID is required in the scope",
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			p := &azureProvider{}
			config := p.BuildConfig(testcontext.New(t), tt.envConfig)
			require.Equal(t, len(tt.expectedConfig), len(config))
		})
	}
}

func TestAzureProvider_generateProviderConfigMap(t *testing.T) {
	tests := []struct {
		desc           string
		subscriptionID string
		credentials    ucp_credentials.AzureCredential
		expectedConfig map[string]any
	}{
		{
			desc:           "valid config",
			subscriptionID: testSubscription,
			credentials:    testAzureCredentials,
			expectedConfig: map[string]any{
				AzureFeaturesParam:     map[string]any{},
				AzureSubIDParam:        testSubscription,
				AzureClientIDParam:     testAzureCredentials.ClientID,
				AzureClientSecretParam: testAzureCredentials.ClientSecret,
				AzureTenantIDParam:     testAzureCredentials.TenantID,
			},
		},
		{
			desc:        "missing subscription",
			credentials: testAzureCredentials,
			expectedConfig: map[string]any{
				AzureFeaturesParam:     map[string]any{},
				AzureClientIDParam:     testAzureCredentials.ClientID,
				AzureClientSecretParam: testAzureCredentials.ClientSecret,
				AzureTenantIDParam:     testAzureCredentials.TenantID,
			},
		},
		{
			desc:           "missing credentials",
			subscriptionID: testSubscription,
			expectedConfig: map[string]any{
				AzureFeaturesParam: map[string]any{},
				AzureSubIDParam:    testSubscription,
			},
		},
		{
			desc: "invalid credentials",
			credentials: ucp_credentials.AzureCredential{
				ClientID:     "",
				ClientSecret: testAzureCredentials.ClientSecret,
				TenantID:     testAzureCredentials.TenantID,
			},
			expectedConfig: map[string]any{
				AzureFeaturesParam: map[string]any{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			p := &azureProvider{}
			azConfig := map[string]any{
				AzureFeaturesParam: map[string]any{},
			}
			config := p.generateProviderConfigMap(testcontext.New(t), azConfig, &tt.credentials, tt.subscriptionID)
			require.Equal(t, len(tt.expectedConfig), len(config))
			require.Equal(t, tt.expectedConfig[AzureFeaturesParam], config[AzureFeaturesParam])
			require.Equal(t, tt.expectedConfig[AzureSubIDParam], config[AzureSubIDParam])
			require.Equal(t, tt.expectedConfig[AzureClientIDParam], config[AzureClientIDParam])
			require.Equal(t, tt.expectedConfig[AzureClientSecretParam], config[AzureClientSecretParam])
			require.Equal(t, tt.expectedConfig[AzureTenantIDParam], config[AzureTenantIDParam])
		})
	}
}

func TestAzureProvider_ParseScope(t *testing.T) {
	tests := []struct {
		desc                 string
		scope                string
		expectedSubscription string
	}{
		{
			desc:                 "valid scope",
			scope:                "/subscriptions/test-sub/resourceGroups/test-rg",
			expectedSubscription: testSubscription,
		},
		{
			desc:                 "empty scope",
			scope:                "",
			expectedSubscription: "",
		},
		{
			desc:                 "invalid scope",
			scope:                "/test-sub/resourceGroups/test-rg",
			expectedSubscription: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			p := &azureProvider{}
			region, _ := p.parseScope(testcontext.New(t), tt.scope)
			require.Equal(t, tt.expectedSubscription, region)
		})
	}
}

func TestAzureProvider_getCredentialsProvider(t *testing.T) {
	secretProviderOptions := ucp_provider.SecretProviderOptions{
		Provider: ucp_provider.TypeKubernetesSecret,
	}

	endpoint := "http://test.endpoint.com"
	connection, err := sdk.NewDirectConnection(endpoint)
	require.NoError(t, err)

	provider := &azureProvider{
		secretProviderOptions: secretProviderOptions,
		ucpConn:               connection,
	}
	azureCredentialProvider := provider.getCredentialsProvider(testcontext.New(t))
	require.NotNil(t, azureCredentialProvider)
}

func TestAzureProvider_FetchCredentials_Success(t *testing.T) {
	credentialsProvider := newMockAzureCredentialsProvider()
	c := fetchAzureCredentials(testcontext.New(t), credentialsProvider)
	require.NotNil(t, c)
	require.Equal(t, testAzureCredentials, *c)
}

func TestAzureProvider_FetchCredentialsError(t *testing.T) {
	credentialsProvider := newMockAzureCredentialsProvider()
	credentialsProvider.testCredential = nil
	c := fetchAzureCredentials(testcontext.New(t), credentialsProvider)
	require.Nil(t, c)
}
