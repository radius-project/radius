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
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/containers/azcontainerregistry"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
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
			require.ElementsMatch(t, tc.want, credentialHosts(t, client, auth.Credential{Username: username, Password: password}))
		})
	}
}

func Test_AzureWorkloadIdentity_TokenExchange(t *testing.T) {
	t.Parallel()
	transport := &recordingTransport{contentType: "application/json", body: `{"refresh_token":"test-refresh-token"}`}
	wi := &azureWorkloadIdentity{clientID: clientID, tenantID: tenantID}

	client, err := wi.exchangeForACRClient(t.Context(), "https://myreg.azurecr.io/recipes/redis:1.0", "test-aad-token",
		&azcontainerregistry.AuthenticationClientOptions{ClientOptions: azcore.ClientOptions{Transport: transport}})
	require.NoError(t, err)

	require.Len(t, transport.requests, 1)
	req := transport.requests[0]
	require.Equal(t, http.MethodPost, req.method)
	require.Equal(t, "myreg.azurecr.io", req.url.Host)
	require.Equal(t, "/oauth2/exchange", req.url.Path)
	form, err := url.ParseQuery(req.body)
	require.NoError(t, err)
	require.Equal(t, url.Values{
		"grant_type":   {"access_token"},
		"service":      {"myreg.azurecr.io"},
		"tenant":       {tenantID},
		"access_token": {"test-aad-token"},
	}, form)

	require.Equal(t, []string{"myreg.azurecr.io"}, credentialHosts(t, client, auth.Credential{RefreshToken: "test-refresh-token"}))
}

func Test_AwsIRSA_ECRAuthentication(t *testing.T) {
	t.Parallel()
	const ecrHost = "123456789012.dkr.ecr.us-west-2.amazonaws.com"
	token := base64.StdEncoding.EncodeToString([]byte("AWS:test-ecr-password"))
	transport := &recordingTransport{
		contentType: "application/x-amz-json-1.1",
		body:        `{"authorizationData":[{"authorizationToken":"` + token + `"}]}`,
	}
	irsa := &awsIRSA{
		roleARN: "arn:aws:iam::123456789012:role/test-role",
		ecrConfigOptions: []func(*config.LoadOptions) error{
			config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("test-key-id", "test-secret", "")),
			config.WithHTTPClient(transport),
		},
	}

	client, err := irsa.GetAuthClient(t.Context(), "https://"+ecrHost+"/recipes/redis:1.0")
	require.NoError(t, err)

	require.Len(t, transport.requests, 1)
	req := transport.requests[0]
	require.Equal(t, "api.ecr.us-west-2.amazonaws.com", req.url.Host)
	require.Equal(t, "AmazonEC2ContainerRegistry_V20150921.GetAuthorizationToken", req.header.Get("X-Amz-Target"))
	require.Contains(t, req.header.Get("Authorization"), "/us-west-2/ecr/aws4_request")

	require.Equal(t, []string{ecrHost}, credentialHosts(t, client, auth.Credential{Username: "AWS", Password: "test-ecr-password"}))
}

var probeHosts = []string{
	"registry-1.docker.io", "docker.io", "index.docker.io", "myuser",
	"registry.example.com:5000", "registry.example.com", "http:", "https:",
	"myreg.azurecr.io", "123456789012.dkr.ecr.us-west-2.amazonaws.com",
}

func credentialHosts(t *testing.T, client remote.Client, want auth.Credential) []string {
	t.Helper()
	authClient, ok := client.(*auth.Client)
	require.True(t, ok)
	got := []string{}
	for _, host := range probeHosts {
		cred, err := authClient.Credential(t.Context(), host)
		require.NoError(t, err)
		if cred != auth.EmptyCredential {
			require.Equal(t, want, cred)
			got = append(got, host)
		}
	}
	return got
}

type recordedRequest struct {
	method string
	url    *url.URL
	header http.Header
	body   string
}

type recordingTransport struct {
	contentType string
	body        string
	requests    []recordedRequest
}

func (rt *recordingTransport) Do(req *http.Request) (*http.Response, error) {
	body := ""
	if req.Body != nil {
		b, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		body = string(b)
	}
	rt.requests = append(rt.requests, recordedRequest{method: req.Method, url: req.URL, header: req.Header.Clone(), body: body})
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": {rt.contentType}},
		Body:       io.NopCloser(strings.NewReader(rt.body)),
		Request:    req,
	}, nil
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
