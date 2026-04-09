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
	"github.com/stretchr/testify/require"
)

func Test_credentialFormatAzureServicePrincipal(t *testing.T) {
	obj := credential.ProviderCredentialConfiguration{
		CloudProviderStatus: credential.CloudProviderStatus{
			Name:    "test",
			Enabled: true,
		},
		AzureCredentials: &credential.AzureCredentialProperties{
			Kind: new("ServicePrincipal"),
			ServicePrincipal: &credential.AzureServicePrincipalCredentialProperties{
				ClientID: new("test-client-id"),
				TenantID: new("test-tenant-id"),
			},
		},
	}

	buffer := &bytes.Buffer{}
	credentialFormatOutput := credentialFormatAzureServicePrincipal()

	err := output.Write(output.FormatTable, obj, buffer, credentialFormatOutput)
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
			Kind: new("WorkloadIdentity"),
			WorkloadIdentity: &credential.AzureWorkloadIdentityCredentialProperties{
				ClientID: new("test-client-id"),
				TenantID: new("test-tenant-id"),
			},
		},
	}

	buffer := &bytes.Buffer{}
	credentialFormatOutput := credentialFormatAzureWorkloadIdentity()

	err := output.Write(output.FormatTable, obj, buffer, credentialFormatOutput)
	require.NoError(t, err)

	expected := "NAME      REGISTERED  KIND              CLIENTID        TENANTID\ntest      true        WorkloadIdentity  test-client-id  test-tenant-id\n"
	require.Equal(t, expected, buffer.String())
}

func Test_credentialFormatAWSAccessKey(t *testing.T) {
	obj := credential.ProviderCredentialConfiguration{
		CloudProviderStatus: credential.CloudProviderStatus{
			Name:    "test",
			Enabled: true,
		},
		AWSCredentials: &credential.AWSCredentialProperties{
			Kind: new("AccessKey"),
			AccessKey: &credential.AWSAccessKeyCredentialProperties{
				Kind:        new("AccessKey"),
				AccessKeyID: new("test-access-key-id"),
			},
		},
	}

	buffer := &bytes.Buffer{}
	credentialFormatOutput := credentialFormatAWSAccessKey()

	err := output.Write(output.FormatTable, obj, buffer, credentialFormatOutput)
	require.NoError(t, err)

	expected := "NAME      REGISTERED  KIND       ACCESSKEYID\ntest      true        AccessKey  test-access-key-id\n"
	require.Equal(t, expected, buffer.String())
}

func Test_credentialFormatAWSIRSA(t *testing.T) {
	obj := credential.ProviderCredentialConfiguration{
		CloudProviderStatus: credential.CloudProviderStatus{
			Name:    "test",
			Enabled: true,
		},
		AWSCredentials: &credential.AWSCredentialProperties{
			Kind: new("IRSA"),
			IRSA: &credential.AWSIRSACredentialProperties{
				Kind:    new("IRSA"),
				RoleARN: new("test-role-arn"),
			},
		},
	}

	buffer := &bytes.Buffer{}
	credentialFormatOutput := credentialFormatAWSIRSA()

	err := output.Write(output.FormatTable, obj, buffer, credentialFormatOutput)
	require.NoError(t, err)

	expected := "NAME      REGISTERED  KIND      ROLEARN\ntest      true        IRSA      test-role-arn\n"
	require.Equal(t, expected, buffer.String())
}
