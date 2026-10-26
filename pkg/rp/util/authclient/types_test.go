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

package authclient

import (
	"fmt"
	"testing"

	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/rp/util"
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

	ctx := t.Context()
	for _, tc := range testset {
		ctrl := gomock.NewController(t)
		newClient, err := GetNewRegistryAuthClient(tc.secrets)
		require.NoError(t, err)
		require.Equal(t, tc.expNewAuthClient, newClient)
		mClient := NewMockAuthClient(ctrl)
		mClient.EXPECT().GetAuthClient(ctx, tc.templatePath).Times(1).Return(tc.expAuthClient, nil)
		ac, err := mClient.GetAuthClient(ctx, tc.templatePath)
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
			templatePath: "https://123456789012.dkr.ecr.us-west-2.amazonaws.com/test-registry:1.0",
			exp:          "us-west-2",
			err:          nil,
		},
		{
			templatePath: "account-id.ecr.us-east-2.amazonaws.com/test-registry:1.0",
			err:          fmt.Errorf("invalid ECR URL format"),
		},
	}
	for _, tc := range testset {
		host, err := util.RegistryName(tc.templatePath)
		require.NoError(t, err)
		reg, err := getECRRegion(host)
		if tc.err != nil {
			require.Equal(t, err, tc.err)
		} else {
			require.NoError(t, err)
			require.Equal(t, tc.exp, reg)
		}

	}
}

// Test_BasicAuthentication_CredentialBinding checks that the basic auth client answers
// with credentials only for the host that ORAS contacts for the template path.
func Test_BasicAuthentication_CredentialBinding(t *testing.T) {
	t.Parallel()
	const sha = "sha256:1234567890123456789012345678901234567890123456789012345678901234"
	probeHosts := []string{"registry-1.docker.io", "docker.io", "index.docker.io", "myuser", "registry.example.com:5000", "registry.example.com", "http:", "https:"}
	testset := []struct {
		name    string
		path    string
		want    []string
		wantErr bool
	}{
		{name: "docker.io namespace", path: "docker.io/myuser/redis:1.0", want: []string{"registry-1.docker.io"}},
		{name: "Docker Hub short name and digest", path: "myuser/redis@" + sha, want: []string{"registry-1.docker.io"}},
		{name: "Docker Hub official image", path: "redis:1.0", want: []string{"registry-1.docker.io"}},
		{name: "index.docker.io", path: "index.docker.io/myuser/redis:1.0", want: []string{"registry-1.docker.io"}},
		{name: "port", path: "registry.example.com:5000/recipes/redis:1.0", want: []string{"registry.example.com:5000"}},
		{name: "http scheme and port", path: "http://registry.example.com:5000/recipes/redis:1.0", want: []string{"registry.example.com:5000"}},
		{name: "https scheme and digest", path: "https://registry.example.com:5000/recipes/redis@" + sha, want: []string{"registry.example.com:5000"}},
		{name: "not valid", path: "br:registry.example.com/recipes/redis:1.0", wantErr: true},
	}
	for _, tc := range testset {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			client, err := NewBasicAuthentication(username, password).GetAuthClient(t.Context(), tc.path)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			authClient, ok := client.(*auth.Client)
			require.True(t, ok)
			got := []string{}
			for _, host := range probeHosts {
				cred, err := authClient.Credential(t.Context(), host)
				require.NoError(t, err)
				if cred != auth.EmptyCredential {
					require.Equal(t, auth.Credential{Username: username, Password: password}, cred)
					got = append(got, host)
				}
			}
			require.ElementsMatch(t, tc.want, got)
		})
	}
}

// Test_StaticCredential_HostMapping pins the ORAS behavior that the docker.io key relies
// on. Radius does not map keys itself.
func Test_StaticCredential_HostMapping(t *testing.T) {
	t.Parallel()
	cred := auth.Credential{Username: username, Password: password}
	testset := []struct {
		key         string
		requestHost string
		want        bool
	}{
		{key: "docker.io", requestHost: "registry-1.docker.io", want: true},
		{key: "docker.io", requestHost: "docker.io", want: false},
		{key: "docker.io", requestHost: "index.docker.io", want: false},
		{key: "registry-1.docker.io", requestHost: "registry-1.docker.io", want: true},
		{key: "index.docker.io", requestHost: "registry-1.docker.io", want: false},
		{key: "myuser", requestHost: "registry-1.docker.io", want: false},
		{key: "registry.example.com:5000", requestHost: "registry.example.com:5000", want: true},
		{key: "registry.example.com", requestHost: "registry.example.com:5000", want: false},
		{key: "myreg.example.com", requestHost: "MyReg.example.com", want: false},
	}
	for _, tc := range testset {
		t.Run(tc.key+" to "+tc.requestHost, func(t *testing.T) {
			t.Parallel()
			got, err := auth.StaticCredential(tc.key, cred)(t.Context(), tc.requestHost)
			require.NoError(t, err)
			if tc.want {
				require.Equal(t, cred, got)
				return
			}
			require.Equal(t, auth.EmptyCredential, got)
		})
	}
}
