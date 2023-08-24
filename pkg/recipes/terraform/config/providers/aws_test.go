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
	testRegion         = "test-region"
	testAWSCredentials = ucp_credentials.AWSCredential{
		AccessKeyID:     "testAccessKey",
		SecretAccessKey: "testSecretKey",
	}
)

type mockAWSCredentialsProvider struct {
	testCredential *ucp_credentials.AWSCredential
}

func newMockAWSCredentialsProvider() *mockAWSCredentialsProvider {
	return &mockAWSCredentialsProvider{
		testCredential: &ucp_credentials.AWSCredential{
			AccessKeyID:     testAWSCredentials.AccessKeyID,
			SecretAccessKey: testAWSCredentials.SecretAccessKey,
		},
	}
}

// Fetch returns mock AWS credentials for testing. It takes in a context, planeName and name and returns
// an AWSCredential or an error if the credentials are empty.
func (p *mockAWSCredentialsProvider) Fetch(ctx context.Context, planeName, name string) (*ucp_credentials.AWSCredential, error) {
	if p.testCredential == nil {
		return nil, &secret.ErrNotFound{}
	}

	if p.testCredential.AccessKeyID == "" && p.testCredential.SecretAccessKey == "" {
		return p.testCredential, nil
	}

	if p.testCredential.AccessKeyID == "" {
		return nil, errors.New("failed to fetch credential")
	}

	return p.testCredential, nil
}

func TestAWSProvider_BuildConfig_InvalidScope_Error(t *testing.T) {
	envConfig := &recipes.Configuration{
		Providers: datamodel.Providers{
			AWS: datamodel.ProvidersAWS{
				Scope: "/planes/aws/aws/accounts/0000/test-region",
			},
		},
	}
	p := &awsProvider{}
	config, err := p.BuildConfig(testcontext.New(t), envConfig)
	require.Nil(t, config)
	require.Error(t, err)
	require.ErrorContains(t, err, "invalid AWS provider scope \"/planes/aws/aws/accounts/0000/test-region\" is configured on the Environment, region is required in the scope")
}

func TestAWSProvider_ParseScope(t *testing.T) {
	tests := []struct {
		desc           string
		envConfig      *recipes.Configuration
		expectedRegion string
		expectedErrMsg string
	}{
		{
			desc: "valid config scope",
			envConfig: &recipes.Configuration{
				Providers: datamodel.Providers{
					AWS: datamodel.ProvidersAWS{
						Scope: "/planes/aws/aws/accounts/0000/regions/test-region",
					},
				},
			},
			expectedRegion: testRegion,
		},
		{
			desc:           "nil config - no error",
			envConfig:      nil,
			expectedRegion: "",
		},
		{
			desc: "missing AWS provider config - no error",
			envConfig: &recipes.Configuration{
				Providers: datamodel.Providers{},
			},
			expectedRegion: "",
		},
		{
			desc: "missing AWS provider scope - no error",
			envConfig: &recipes.Configuration{
				Providers: datamodel.Providers{
					AWS: datamodel.ProvidersAWS{},
				},
			},
			expectedRegion: "",
		},
		{
			desc: "missing region segment - error",
			envConfig: &recipes.Configuration{
				Providers: datamodel.Providers{
					AWS: datamodel.ProvidersAWS{
						Scope: "/planes/aws/aws/accounts/0000/test-region",
					},
				},
			},
			expectedRegion: "",
			expectedErrMsg: "invalid AWS provider scope \"/planes/aws/aws/accounts/0000/test-region\" is configured on the Environment, region is required in the scope",
		},
		{
			desc: "invalid scope - error",
			envConfig: &recipes.Configuration{
				Providers: datamodel.Providers{
					AWS: datamodel.ProvidersAWS{
						Scope: "invalid",
					},
				},
			},
			expectedRegion: "",
			expectedErrMsg: "invalid AWS provider scope \"invalid\" is configured on the Environment, error parsing: 'invalid' is not a valid resource id",
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			p := &awsProvider{}
			region, err := p.parseScope(testcontext.New(t), tt.envConfig)
			if tt.expectedErrMsg != "" {
				require.Error(t, err)
				require.ErrorContains(t, err, tt.expectedErrMsg)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.expectedRegion, region)
			}
		})
	}
}

func TestAwsProvider_getCredentialsProvider(t *testing.T) {
	endpoint := "http://example.com"
	connection, err := sdk.NewDirectConnection(endpoint)
	require.NoError(t, err)

	provider := &awsProvider{
		ucpConn: connection,
	}
	awsCredentialProvider, err := provider.getCredentialsProvider()
	require.NotNil(t, awsCredentialProvider)
	require.NoError(t, err)
}

func TestAWSProvider_FetchCredentials(t *testing.T) {
	tests := []struct {
		desc                string
		credentialsProvider *mockAWSCredentialsProvider
		expectedCreds       *ucp_credentials.AWSCredential
		expectedErr         bool
	}{
		{
			desc:                "valid credentials",
			credentialsProvider: newMockAWSCredentialsProvider(),
			expectedCreds:       &testAWSCredentials,
			expectedErr:         false,
		},
		{
			desc: "credentials not found - no error",
			credentialsProvider: &mockAWSCredentialsProvider{
				testCredential: nil,
			},
			expectedCreds: nil,
			expectedErr:   false,
		},
		{
			desc: "empty values - no error",
			credentialsProvider: &mockAWSCredentialsProvider{
				&ucp_credentials.AWSCredential{
					AccessKeyID:     "",
					SecretAccessKey: "",
				},
			},
			expectedCreds: nil,
			expectedErr:   false,
		},
		{
			desc: "fetch credential error",
			credentialsProvider: &mockAWSCredentialsProvider{
				&ucp_credentials.AWSCredential{
					AccessKeyID:     "",
					SecretAccessKey: testAWSCredentials.SecretAccessKey,
				},
			},
			expectedCreds: nil,
			expectedErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			c, err := fetchAWSCredentials(testcontext.New(t), tt.credentialsProvider)
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

func TestAWSProvider_generateProviderConfigMap(t *testing.T) {
	tests := []struct {
		desc           string
		region         string
		credentials    ucp_credentials.AWSCredential
		expectedConfig map[string]any
	}{
		{
			desc:        "valid config",
			region:      testRegion,
			credentials: testAWSCredentials,
			expectedConfig: map[string]any{
				awsRegionParam:    testRegion,
				awsAccessKeyParam: testAWSCredentials.AccessKeyID,
				awsSecretKeyParam: testAWSCredentials.SecretAccessKey,
			},
		},
		{
			desc:        "missing region",
			credentials: testAWSCredentials,
			expectedConfig: map[string]any{
				awsAccessKeyParam: testAWSCredentials.AccessKeyID,
				awsSecretKeyParam: testAWSCredentials.SecretAccessKey,
			},
		},
		{
			desc:   "missing credentials",
			region: testRegion,
			expectedConfig: map[string]any{
				awsRegionParam: testRegion,
			},
		},
		{
			desc: "invalid credentials",
			credentials: ucp_credentials.AWSCredential{
				AccessKeyID:     "",
				SecretAccessKey: testAWSCredentials.SecretAccessKey,
			},
			expectedConfig: map[string]any{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			p := &awsProvider{}
			config := p.generateProviderConfigMap(&tt.credentials, tt.region)
			require.Equal(t, len(tt.expectedConfig), len(config))
			require.Equal(t, tt.expectedConfig[awsRegionParam], config[awsRegionParam])
			require.Equal(t, tt.expectedConfig[awsAccessKeyParam], config[awsAccessKeyParam])
			require.Equal(t, tt.expectedConfig[awsSecretKeyParam], config[awsSecretKeyParam])
		})
	}
}
