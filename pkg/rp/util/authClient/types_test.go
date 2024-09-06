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

package authClient

import (
	"context"
	"fmt"
	"testing"

	"github.com/radius-project/radius/pkg/recipes"
	"github.com/stretchr/testify/require"
	gomock "go.uber.org/mock/gomock"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"
	"oras.land/oras-go/v2/registry/remote/retry"
)

const (
	username     = "test-username"
	password     = "test-password"
	templatePath = "test.azurecr.io/test-private-registry:latest"
	clientID     = "test-client-id"
	tenantID     = "test-tenant-id"
)

func Test_getRegistryAuthClient(t *testing.T) {
	testset := []struct {
		secrets          recipes.SecretData
		templatePath     string
		expNewAuthClient AuthClient
		expAuthClient    remote.Client
	}{
		{
			secrets: recipes.SecretData{
				Type: "basicAuthentication",
				Data: map[string]string{
					"username": username,
					"password": password,
				},
			},
			templatePath:     templatePath,
			expNewAuthClient: NewBasicAuthentication(username, password),
			expAuthClient: &auth.Client{
				Client: retry.DefaultClient,
				Credential: auth.StaticCredential("test.azurecr.io", auth.Credential{
					Username: username,
					Password: password,
				}),
			},
		},
		{
			secrets: recipes.SecretData{
				Type: "azureWorkloadIdentity",
				Data: map[string]string{
					"clientId": clientID,
					"tenantId": tenantID,
				},
			},
			templatePath:     templatePath,
			expNewAuthClient: NewAzureWorkloadIdentity(clientID, tenantID),
			expAuthClient: &auth.Client{
				Client: retry.DefaultClient,
				Credential: auth.StaticCredential("test.azurecr.io", auth.Credential{
					RefreshToken: "test-refresh-token",
				}),
			},
		},
	}

	for _, tc := range testset {
		ctrl := gomock.NewController(t)
		newClient, err := GetNewRegistryAuthClient(tc.secrets)
		require.NoError(t, err)
		require.Equal(t, tc.expNewAuthClient, newClient)
		mClient := NewMockAuthClient(ctrl)
		mClient.EXPECT().GetAuthClient(context.Background(), templatePath).Times(1).Return(tc.expAuthClient, nil)
		ac, err := mClient.GetAuthClient(context.Background(), tc.templatePath)
		require.NoError(t, err)
		require.Equal(t, ac, tc.expAuthClient)
	}
}

func Test_getECRRegion(t *testing.T) {
	testset := []struct {
		templatePath string
		exp          string
		err          error
	}{
		{
			templatePath: "account-id.dkr.ecr.us-east-2.amazonaws.com/test-registry:1.0",
			exp:          "us-east-2",
			err:          nil,
		},
		{
			templatePath: "account-id.ecr.us-east-2.amazonaws.com/test-registry:1.0",
			err:          fmt.Errorf("invalid ECR URL format"),
		},
	}
	for _, tc := range testset {
		host, err := getRegistryHostname(tc.templatePath)
		require.NoError(t, err)
		reg, err := getECRRegion(host)
		if tc.err != nil {
			require.Equal(t, err, tc.err)
		} else {
			require.NoError(t, err)
			require.Equal(t, reg, "us-east-2")
		}

	}
}
