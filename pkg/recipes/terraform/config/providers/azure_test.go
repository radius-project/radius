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
	require.ErrorContains(t, err, "invalid Azure provider scope \"/test-sub/resourceGroups/test-rg\" is configured on the Environment, subscription is required in the scope")
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
			expectedErrMsg:       "invalid Azure provider scope \"/test-sub/resourceGroups/test-rg\" is configured on the Environment, subscription is required in the scope",
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
			expectedErrMsg:       "invalid Azure provider scope \"invalid\" is configured on the Environment, error parsing: 'invalid' is not a valid resource id",
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
	endpoint := "http://example.com"
	connection, err := sdk.NewDirectConnection(endpoint)
	require.NoError(t, err)

	provider := &azureProvider{
		ucpConn: connection,
	}
	azureCredentialProvider, err := provider.getCredentialsProvider()
	require.NotNil(t, azureCredentialProvider)
	require.NoError(t, err)
}

func TestAzureProvider_FetchCredentials(t *testing.T) {
	tests := []struct {
		desc                string
		credentialsProvider *mockAzureCredentialsProvider
		expectedCreds       *ucp_credentials.AzureCredential
		expectedErr         bool
	}{
		{
			desc:                "valid credentials",
			credentialsProvider: newMockAzureCredentialsProvider(),
			expectedCreds:       &testAzureCredentials,
			expectedErr:         false,
		},
		{
			desc: "credentials not found - no error",
			credentialsProvider: &mockAzureCredentialsProvider{
				testCredential: nil,
			},
			expectedCreds: nil,
			expectedErr:   false,
		},
		{
			desc: "empty values - no error",
			credentialsProvider: &mockAzureCredentialsProvider{
				&ucp_credentials.AzureCredential{
					TenantID:     "",
					ClientID:     "",
					ClientSecret: "",
				},
			},
			expectedCreds: nil,
			expectedErr:   false,
		},
		{
			desc: "fetch credential error",
			credentialsProvider: &mockAzureCredentialsProvider{
				&ucp_credentials.AzureCredential{
					TenantID:     "",
					ClientID:     testAzureCredentials.ClientID,
					ClientSecret: testAzureCredentials.ClientSecret,
				},
			},
			expectedCreds: nil,
			expectedErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			c, err := fetchAzureCredentials(testcontext.New(t), tt.credentialsProvider)
			if tt.expectedErr {
				require.Error(t, err)
				require.Nil(t, c)
			} else {
				require.NoError(t, err)
				if tt.expectedCreds != nil {
					require.Equal(t, *tt.expectedCreds, *c)
				}
			}
		})
	}
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
				azureFeaturesParam:     map[string]any{},
				azureSubIDParam:        testSubscription,
				azureTenantIDParam:     testAzureCredentials.TenantID,
				azureClientIDParam:     testAzureCredentials.ClientID,
				azureClientSecretParam: testAzureCredentials.ClientSecret,
			},
		},
		{
			desc:        "missing subscription",
			credentials: testAzureCredentials,
			expectedConfig: map[string]any{
				azureFeaturesParam:     map[string]any{},
				azureTenantIDParam:     testAzureCredentials.TenantID,
				azureClientIDParam:     testAzureCredentials.ClientID,
				azureClientSecretParam: testAzureCredentials.ClientSecret,
			},
		},
		{
			desc:         "missing credentials",
			subscription: testSubscription,
			expectedConfig: map[string]any{
				azureFeaturesParam: map[string]any{},
				azureSubIDParam:    testSubscription,
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
				azureFeaturesParam: map[string]any{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			p := &azureProvider{}
			azConfig := map[string]any{
				azureFeaturesParam: map[string]any{},
			}
			config := p.generateProviderConfigMap(azConfig, &tt.credentials, tt.subscription)
			require.Equal(t, len(tt.expectedConfig), len(config))
			require.Equal(t, tt.expectedConfig[azureFeaturesParam], config[azureFeaturesParam])
			require.Equal(t, tt.expectedConfig[azureSubIDParam], config[azureSubIDParam])
			require.Equal(t, tt.expectedConfig[azureClientIDParam], config[azureClientIDParam])
			require.Equal(t, tt.expectedConfig[azureClientSecretParam], config[azureClientSecretParam])
			require.Equal(t, tt.expectedConfig[azureTenantIDParam], config[azureTenantIDParam])
		})
	}
}
