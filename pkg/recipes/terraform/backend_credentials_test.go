/*
Copyright 2026 The Radius Authors.

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

package terraform

import (
	"context"
	"errors"
	"maps"
	"os"
	"testing"

	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/ucp/credentials"
	"github.com/stretchr/testify/require"
)

type backendCredentialStub[T any] struct {
	value  *T
	err    error
	planes []string
	names  []string
}

func (s *backendCredentialStub[T]) Fetch(ctx context.Context, plane, name string) (*T, error) {
	s.planes = append(s.planes, plane)
	s.names = append(s.names, name)
	return s.value, s.err
}

func backendTestAWSCredential(irsa bool) *credentials.AWSCredential {
	if irsa {
		return &credentials.AWSCredential{Kind: credentials.AWSIRSACredentialKind,
			IRSACredential: &credentials.AWSIRSACredential{RoleARN: "arn:aws:iam::123456789012:role/radius-state"}}
	}
	return &credentials.AWSCredential{Kind: credentials.AWSAccessKeyCredentialKind,
		AccessKeyCredential: &credentials.AWSAccessKeyCredential{AccessKeyID: "registered-access", SecretAccessKey: "registered-secret"}}
}

func backendTestAzureCredential(wi bool) *credentials.AzureCredential {
	if wi {
		return &credentials.AzureCredential{Kind: credentials.AzureWorkloadIdentityCredentialKind,
			WorkloadIdentity: &credentials.AzureWorkloadIdentityCredential{ClientID: "registered-client", TenantID: "registered-tenant"}}
	}
	return &credentials.AzureCredential{Kind: credentials.AzureServicePrincipalCredentialKind,
		ServicePrincipal: &credentials.AzureServicePrincipalCredential{ClientID: "registered-client", TenantID: "registered-tenant", ClientSecret: "registered-secret"}}
}

func TestBackendCredentialFetchAndEnvironment(t *testing.T) {
	for _, federated := range []bool{false, true} {
		for _, cloud := range []string{"s3", "azurerm"} {
			t.Run(cloud+map[bool]string{false: " static", true: " federated"}[federated], func(t *testing.T) {
				aws := &backendCredentialStub[credentials.AWSCredential]{value: backendTestAWSCredential(federated)}
				azure := &backendCredentialStub[credentials.AzureCredential]{value: backendTestAzureCredential(federated)}
				e := executor{awsCredentials: aws, azureCredentials: azure}
				backend := &datamodel.TerraformBackend{Type: "s3", Bucket: "states", Region: "us-west-2"}
				if cloud == "azurerm" {
					backend = &datamodel.TerraformBackend{Type: "azurerm", StorageAccountName: "states", ContainerName: "radius"}
				}
				env := map[string]string{
					"KEEP": "user-value", envTFCLIConfigFile: "private-registry",
					"AWS_ACCESS_KEY_ID": "stale", "AWS_SECRET_ACCESS_KEY": "stale", "AWS_SESSION_TOKEN": "stale",
					"AWS_ROLE_ARN": "stale", "AWS_WEB_IDENTITY_TOKEN_FILE": "stale", "AWS_PROFILE": "stale",
					"ARM_CLIENT_SECRET": "stale", "ARM_OIDC_TOKEN": "stale", "ARM_OIDC_TOKEN_FILE_PATH": "stale",
					"ARM_ACCESS_KEY": "stale", "ARM_SAS_TOKEN": "stale", "ARM_USE_MSI": "true", "ARM_USE_CLI": "true",
					"ARM_CLIENT_CERTIFICATE_PATH": "stale", "ARM_CLIENT_SECRET_FILE_PATH": "stale",
					"ARM_CLIENT_ID_FILE_PATH": "stale",
				}
				for range 2 {
					require.NoError(t, e.setBackendEnvironment(t.Context(), backend, env))
				}
				require.Equal(t, "user-value", env["KEEP"])
				require.Equal(t, "private-registry", env[envTFCLIConfigFile])
				if cloud == "s3" {
					require.Equal(t, []string{credentials.AWSPublic, credentials.AWSPublic}, aws.planes)
					require.Equal(t, []string{"default", "default"}, aws.names)
					require.Empty(t, azure.names)
					require.Equal(t, "stale", env["ARM_CLIENT_SECRET"], "other cloud must be unchanged")
					require.NotContains(t, env, "AWS_SESSION_TOKEN")
					require.NotContains(t, env, "AWS_PROFILE")
					require.Equal(t, os.DevNull, env["AWS_CONFIG_FILE"])
					if federated {
						require.NotContains(t, env, "AWS_ACCESS_KEY_ID")
						require.NotContains(t, env, "AWS_SECRET_ACCESS_KEY")
						require.Equal(t, awsBackendTokenFile, env["AWS_WEB_IDENTITY_TOKEN_FILE"])
						require.Equal(t, aws.value.IRSACredential.RoleARN, env["AWS_ROLE_ARN"])
					} else {
						require.Equal(t, "registered-access", env["AWS_ACCESS_KEY_ID"])
						require.Equal(t, "registered-secret", env["AWS_SECRET_ACCESS_KEY"])
						require.NotContains(t, env, "AWS_WEB_IDENTITY_TOKEN_FILE")
						require.NotContains(t, env, "AWS_ROLE_ARN")
					}
				} else {
					require.Equal(t, []string{credentials.AzureCloud, credentials.AzureCloud}, azure.planes)
					require.Equal(t, []string{"default", "default"}, azure.names)
					require.Empty(t, aws.names)
					require.Equal(t, "stale", env["AWS_SESSION_TOKEN"], "other cloud must be unchanged")
					require.Equal(t, "registered-client", env["ARM_CLIENT_ID"])
					require.Equal(t, "registered-tenant", env["ARM_TENANT_ID"])
					require.Equal(t, "true", env["ARM_USE_AZUREAD"])
					require.Equal(t, "false", env["ARM_USE_MSI"])
					require.Equal(t, "false", env["ARM_USE_CLI"])
					for _, key := range []string{"ARM_ACCESS_KEY", "ARM_SAS_TOKEN", "ARM_OIDC_TOKEN", "ARM_CLIENT_CERTIFICATE_PATH", "ARM_CLIENT_SECRET_FILE_PATH", "ARM_CLIENT_ID_FILE_PATH"} {
						require.NotContains(t, env, key)
					}
					if federated {
						require.Equal(t, "true", env["ARM_USE_OIDC"])
						require.Equal(t, azureBackendTokenFile, env["ARM_OIDC_TOKEN_FILE_PATH"])
						require.NotContains(t, env, "ARM_CLIENT_SECRET")
					} else {
						require.Equal(t, "false", env["ARM_USE_OIDC"])
						require.Equal(t, "registered-secret", env["ARM_CLIENT_SECRET"])
						require.NotContains(t, env, "ARM_OIDC_TOKEN_FILE_PATH")
					}
				}
			})
		}
	}
}

func TestBackendCredentialErrors(t *testing.T) {
	for _, c := range []*credentials.AWSCredential{
		nil, {}, {Kind: credentials.AWSAccessKeyCredentialKind},
		{Kind: credentials.AWSAccessKeyCredentialKind, AccessKeyCredential: &credentials.AWSAccessKeyCredential{AccessKeyID: "secret-marker"}},
		{Kind: credentials.AWSAccessKeyCredentialKind, AccessKeyCredential: &credentials.AWSAccessKeyCredential{AccessKeyID: " ", SecretAccessKey: " "}},
		{Kind: credentials.AWSIRSACredentialKind}, {Kind: credentials.AWSIRSACredentialKind, IRSACredential: &credentials.AWSIRSACredential{}},
	} {
		env := map[string]string{"UNCHANGED": "value"}
		before := maps.Clone(env)
		err := setAWSBackendEnvironment(c, env)
		require.Error(t, err)
		require.NotContains(t, err.Error(), "secret-marker")
		require.Equal(t, before, env)
	}
	for _, c := range []*credentials.AzureCredential{
		nil, {}, {Kind: credentials.AzureServicePrincipalCredentialKind},
		{Kind: credentials.AzureServicePrincipalCredentialKind, ServicePrincipal: &credentials.AzureServicePrincipalCredential{ClientSecret: "secret-marker"}},
		{Kind: credentials.AzureServicePrincipalCredentialKind, ServicePrincipal: &credentials.AzureServicePrincipalCredential{ClientID: " ", TenantID: " ", ClientSecret: " "}},
		{Kind: credentials.AzureWorkloadIdentityCredentialKind}, {Kind: credentials.AzureWorkloadIdentityCredentialKind, WorkloadIdentity: &credentials.AzureWorkloadIdentityCredential{}},
	} {
		env := map[string]string{"UNCHANGED": "value"}
		before := maps.Clone(env)
		err := setAzureBackendEnvironment(c, env)
		require.Error(t, err)
		require.NotContains(t, err.Error(), "secret-marker")
		require.Equal(t, before, env)
	}
	fetchErr := errors.New("registered credential missing")
	e := executor{
		awsCredentials:   &backendCredentialStub[credentials.AWSCredential]{err: fetchErr},
		azureCredentials: &backendCredentialStub[credentials.AzureCredential]{err: fetchErr},
	}
	for _, backend := range []*datamodel.TerraformBackend{
		{Type: "s3", Bucket: "states", Region: "us-west-2"},
		{Type: "azurerm", StorageAccountName: "states", ContainerName: "radius"},
	} {
		err := e.setBackendEnvironment(t.Context(), backend, map[string]string{})
		require.ErrorIs(t, err, fetchErr)
		require.Contains(t, err.Error(), backend.Type+" backend")
	}
}
