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
	"github.com/project-radius/radius/pkg/ucp/secret"
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
		return nil, &secret.ErrNotFound{}
	}

	if p.testCredential.TenantID == "" && p.testCredential.ClientID == "" && p.testCredential.ClientSecret == "" {
		return p.testCredential, nil
	}

	if p.testCredential.TenantID == "" {
		return nil, errors.New("failed to fetch credential")
	}

	return p.testCredential, nil
}

func TestAzureProvider_BuildConfig_InvalidScope_Error(t *testing.T) {
	envConfig := &recipes.Configuration{
		Providers: datamodel.Providers{
			Azure: datamodel.ProvidersAzure{
				Scope: "/test-sub/resourceGroups/test-rg",
			},
		},
	}
	p := &azureProvider{}
	config, err := p.BuildConfig(testcontext.New(t), envConfig)
	require.Nil(t, config)
	require.Error(t, err)
	require.ErrorContains(t, err, "code BadRequest: err Invalid Azure provider scope \"/test-sub/resourceGroups/test-rg\" is configured on the Environment, subscription is required in the scope")
}

func TestAzureProvider_ParseScope(t *testing.T) {
	tests := []struct {
		desc                 string
		envConfig            *recipes.Configuration
		expectedSubscription string
		expectedErrMsg       string
	}{
		{
			desc: "valid config scope",
			envConfig: &recipes.Configuration{
				Providers: datamodel.Providers{
					Azure: datamodel.ProvidersAzure{
						Scope: "/subscriptions/test-sub/resourceGroups/test-rg",
					},
				},
			},
			expectedSubscription: testSubscription,
		},
		{
			desc:                 "nil config - no error",
			envConfig:            nil,
			expectedSubscription: "",
		},
		{
			desc: "missing Azure provider config - no error",
			envConfig: &recipes.Configuration{
				Providers: datamodel.Providers{},
			},
			expectedSubscription: "",
		},
		{
			desc: "missing Azure provider scope - no error",
			envConfig: &recipes.Configuration{
				Providers: datamodel.Providers{
					Azure: datamodel.ProvidersAzure{},
				},
			},
			expectedSubscription: "",
		},
		{
			desc: "missing subscription segment - error",
			envConfig: &recipes.Configuration{
				Providers: datamodel.Providers{
					Azure: datamodel.ProvidersAzure{
						Scope: "/test-sub/resourceGroups/test-rg",
					},
				},
			},
			expectedSubscription: "",
			expectedErrMsg:       "code BadRequest: err Invalid Azure provider scope \"/test-sub/resourceGroups/test-rg\" is configured on the Environment, subscription is required in the scope",
		},
		{
			desc: "invalid scope - error",
			envConfig: &recipes.Configuration{
				Providers: datamodel.Providers{
					Azure: datamodel.ProvidersAzure{
						Scope: "invalid",
					},
				},
			},
			expectedSubscription: "",
			expectedErrMsg:       "code BadRequest: err Invalid Azure provider scope \"invalid\" is configured on the Environment, error parsing: 'invalid' is not a valid resource id",
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			p := &azureProvider{}
			subscription, err := p.parseScope(testcontext.New(t), tt.envConfig)
			if tt.expectedErrMsg != "" {
				require.Error(t, err)
				require.ErrorContains(t, err, tt.expectedErrMsg)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.expectedSubscription, subscription)
			}
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
	azureCredentialProvider, _ := provider.getCredentialsProvider()
	require.NotNil(t, azureCredentialProvider)
}

func TestFetchAzureCredentials_Success(t *testing.T) {
	credentialsProvider := newMockAzureCredentialsProvider()
	c, _ := fetchAzureCredentials(testcontext.New(t), credentialsProvider)
	require.NotNil(t, c)
	require.Equal(t, testAzureCredentials, *c)
}

func TestFetchAzureCredentialsNotFound_Success(t *testing.T) {
	credentialsProvider := newMockAzureCredentialsProvider()
	credentialsProvider.testCredential = nil
	c, err := fetchAzureCredentials(testcontext.New(t), credentialsProvider)
	require.NoError(t, err)
	require.Nil(t, c)
}

func TestAzureFetchCredentialsEmptyValues_Success(t *testing.T) {
	credentialsProvider := newMockAzureCredentialsProvider()
	credentialsProvider.testCredential.TenantID = ""
	credentialsProvider.testCredential.ClientID = ""
	credentialsProvider.testCredential.ClientSecret = ""
	c, err := fetchAzureCredentials(testcontext.New(t), credentialsProvider)
	require.NoError(t, err)
	require.Nil(t, c)
}

func TestFetchAzureCredentialsError_Failure(t *testing.T) {
	credentialsProvider := newMockAzureCredentialsProvider()
	credentialsProvider.testCredential.TenantID = ""
	c, err := fetchAzureCredentials(testcontext.New(t), credentialsProvider)
	require.Error(t, err)
	require.Nil(t, c)
}

func TestAzureProvider_generateProviderConfigMap(t *testing.T) {
	tests := []struct {
		desc           string
		subscription   string
		credentials    ucp_credentials.AzureCredential
		expectedConfig map[string]any
	}{
		{
			desc:         "valid config",
			subscription: testSubscription,
			credentials:  testAzureCredentials,
			expectedConfig: map[string]any{
				AzureFeaturesParam:     map[string]any{},
				AzureSubIDParam:        testSubscription,
				AzureTenantIDParam:     testAzureCredentials.TenantID,
				AzureClientIDParam:     testAzureCredentials.ClientID,
				AzureClientSecretParam: testAzureCredentials.ClientSecret,
			},
		},
		{
			desc:        "missing subscription",
			credentials: testAzureCredentials,
			expectedConfig: map[string]any{
				AzureFeaturesParam:     map[string]any{},
				AzureTenantIDParam:     testAzureCredentials.TenantID,
				AzureClientIDParam:     testAzureCredentials.ClientID,
				AzureClientSecretParam: testAzureCredentials.ClientSecret,
			},
		},
		{
			desc:         "missing credentials",
			subscription: testSubscription,
			expectedConfig: map[string]any{
				AzureFeaturesParam: map[string]any{},
				AzureSubIDParam:    testSubscription,
			},
		},
		{
			desc: "invalid credentials",
			credentials: ucp_credentials.AzureCredential{
				TenantID:     "",
				ClientID:     testAzureCredentials.ClientID,
				ClientSecret: testAzureCredentials.ClientSecret,
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
			config := p.generateProviderConfigMap(azConfig, &tt.credentials, tt.subscription)
			require.Equal(t, len(tt.expectedConfig), len(config))
			require.Equal(t, tt.expectedConfig[AzureFeaturesParam], config[AzureFeaturesParam])
			require.Equal(t, tt.expectedConfig[AzureSubIDParam], config[AzureSubIDParam])
			require.Equal(t, tt.expectedConfig[AzureClientIDParam], config[AzureClientIDParam])
			require.Equal(t, tt.expectedConfig[AzureClientSecretParam], config[AzureClientSecretParam])
			require.Equal(t, tt.expectedConfig[AzureTenantIDParam], config[AzureTenantIDParam])
		})
	}
}
