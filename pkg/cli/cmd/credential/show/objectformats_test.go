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

package show

import (
	"bytes"
	"testing"

	"github.com/radius-project/radius/pkg/cli/credential"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/to"
	"github.com/stretchr/testify/require"
)

func Test_credentialFormat_Azure_ServicePrincipal(t *testing.T) {
	obj := credential.ProviderCredentialConfiguration{
		CloudProviderStatus: credential.CloudProviderStatus{
			Name:    "test",
			Enabled: true,
		},
		AzureCredentials: &credential.AzureCredentialProperties{
			Kind: to.Ptr("ServicePrincipal"),
			ServicePrincipal: &credential.AzureServicePrincipalCredentialProperties{
				ClientID: to.Ptr("test-client-id"),
				TenantID: to.Ptr("test-tenant-id"),
			},
		},
	}

	buffer := &bytes.Buffer{}
	credentialFormatOutput, err := credentialFormat("azure", obj)
	require.NoError(t, err)

	err = output.Write(output.FormatTable, obj, buffer, credentialFormatOutput)
	require.NoError(t, err)

	expected := "NAME      REGISTERED  KIND              CLIENTID        TENANTID\ntest      true        ServicePrincipal  test-client-id  test-tenant-id\n"
	require.Equal(t, expected, buffer.String())
}

func Test_credentialFormat_Azure_WorkloadIdentity(t *testing.T) {
	obj := credential.ProviderCredentialConfiguration{
		CloudProviderStatus: credential.CloudProviderStatus{
			Name:    "test",
			Enabled: true,
		},
		AzureCredentials: &credential.AzureCredentialProperties{
			Kind: to.Ptr("WorkloadIdentity"),
			WorkloadIdentity: &credential.AzureWorkloadIdentityCredentialProperties{
				ClientID: to.Ptr("test-client-id"),
				TenantID: to.Ptr("test-tenant-id"),
			},
		},
	}

	buffer := &bytes.Buffer{}
	credentialFormatOutput, err := credentialFormat("azure", obj)
	require.NoError(t, err)

	err = output.Write(output.FormatTable, obj, buffer, credentialFormatOutput)
	require.NoError(t, err)

	expected := "NAME      REGISTERED  KIND              CLIENTID        TENANTID\ntest      true        WorkloadIdentity  test-client-id  test-tenant-id\n"
	require.Equal(t, expected, buffer.String())
}

func Test_credentialFormat_Azure_UnknownKind(t *testing.T) {
	obj := credential.ProviderCredentialConfiguration{
		CloudProviderStatus: credential.CloudProviderStatus{
			Name:    "test",
			Enabled: true,
		},
		AzureCredentials: &credential.AzureCredentialProperties{
			Kind: to.Ptr("UnknownKind"),
		},
	}

	credentialFormatOutput, err := credentialFormat("azure", obj)
	require.Equal(t, "unknown Azure credential kind, expected ServicePrincipal or WorkloadIdentity (got UnknownKind)", err.Error())
	require.Equal(t, output.FormatterOptions{}, credentialFormatOutput)
}

func Test_credentialFormat_AWS(t *testing.T) {
	obj := credential.ProviderCredentialConfiguration{
		CloudProviderStatus: credential.CloudProviderStatus{
			Name:    "test",
			Enabled: true,
		},
		AWSCredentials: &credential.AWSCredentialProperties{
			AccessKeyID: to.Ptr("test-access-key-id"),
		},
	}

	buffer := &bytes.Buffer{}
	credentialFormatOutput, err := credentialFormat("aws", obj)
	require.NoError(t, err)

	err = output.Write(output.FormatTable, obj, buffer, credentialFormatOutput)
	require.NoError(t, err)

	expected := "NAME      REGISTERED  ACCESSKEYID\ntest      true        test-access-key-id\n"
	require.Equal(t, expected, buffer.String())
}

func Test_credentialFormat_UnknownProvider(t *testing.T) {
	obj := credential.ProviderCredentialConfiguration{
		CloudProviderStatus: credential.CloudProviderStatus{
			Name:    "test",
			Enabled: true,
		},
		AzureCredentials: &credential.AzureCredentialProperties{
			Kind: to.Ptr("UnknownKind"),
		},
	}

	credentialFormatOutput, err := credentialFormat("unknown", obj)
	require.Equal(t, "unknown credential type: unknown", err.Error())
	require.Equal(t, output.FormatterOptions{}, credentialFormatOutput)
}
