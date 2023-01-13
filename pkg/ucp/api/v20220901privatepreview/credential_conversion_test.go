// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v20220901privatepreview

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	radiustesting "github.com/project-radius/radius/pkg/corerp/testing"
	"github.com/project-radius/radius/pkg/ucp/datamodel"
	"github.com/stretchr/testify/require"
)

func TestCredentialConvertVersionedToDataModel(t *testing.T) {
	internalStorageKind := datamodel.CredentialStorageKind(CredentialStorageKindInternal)
	conversionTests := []struct {
		filename string
		expected *datamodel.Credential
		err      error
	}{
		{
			filename: "credentialresource-aws.json",
			expected: &datamodel.Credential{
				BaseResource: v1.BaseResource{
					TrackedResource: v1.TrackedResource{
						ID:       "/planes/aws/awscloud/providers/System.AWS/credentials/default",
						Name:     "default",
						Type:     "System.AWS/credentials",
						Location: "west-us-2",
						Tags: map[string]string{
							"env": "dev",
						},
					},
					InternalMetadata: v1.InternalMetadata{
						UpdatedAPIVersion: Version,
					},
				},
				Properties: &datamodel.CredentialResourceProperties{
					Kind: "aws.com.iam",
					AWSCredential: &datamodel.AWSCredentialProperties{
						AccessKeyID:     to.Ptr("00000000-0000-0000-0000-000000000000"),
						SecretAccessKey: to.Ptr("00000000-0000-0000-0000-000000000000"),
					},
					Storage: &datamodel.CredentialStorageProperties{
						Kind:               &internalStorageKind,
						InternalCredential: &datamodel.InternalCredentialStorageProperties{},
					},
				},
			},
		},
		{
			filename: "credentialresource-azure.json",
			expected: &datamodel.Credential{
				BaseResource: v1.BaseResource{
					TrackedResource: v1.TrackedResource{
						ID:       "/planes/azure/azurecloud/providers/System.Azure/credentials/default",
						Name:     "default",
						Type:     "System.Azure/credentials",
						Location: "west-us-2",
						Tags: map[string]string{
							"env": "dev",
						},
					},
					InternalMetadata: v1.InternalMetadata{
						UpdatedAPIVersion: Version,
					},
				},
				Properties: &datamodel.CredentialResourceProperties{
					Kind: "azure.com.serviceprincipal",
					AzureCredential: &datamodel.AzureCredentialProperties{
						TenantID: to.Ptr("00000000-0000-0000-0000-000000000000"),
						ClientID: to.Ptr("00000000-0000-0000-0000-000000000000"),
					},
					Storage: &datamodel.CredentialStorageProperties{
						Kind:               &internalStorageKind,
						InternalCredential: &datamodel.InternalCredentialStorageProperties{},
					},
				},
			},
		},
		{
			filename: "credentialresource-other.json",
			err:      v1.ErrInvalidModelConversion,
		},
		{
			filename: "credentialresource-empty-properties.json",
			err:      &v1.ErrModelConversion{PropertyName: "$.properties", ValidValue: "not nil"},
		},
		{
			filename: "credentialresource-empty-storage.json",
			err:      &v1.ErrModelConversion{PropertyName: "$.properties.storage", ValidValue: "not nil"},
		},
		{
			filename: "credentialresource-empty-storage-kind.json",
			err:      &v1.ErrModelConversion{PropertyName: "$.properties.storage.kind", ValidValue: "not nil"},
		},
		{
			filename: "credentialresource-invalid-storagekind.json",
			err:      &v1.ErrModelConversion{PropertyName: "$.properties.storage.kind", ValidValue: fmt.Sprintf("one of %s", PossibleCredentialStorageKindValues())},
		},
	}
	for _, tt := range conversionTests {
		t.Run(tt.filename, func(t *testing.T) {
			rawPayload := radiustesting.ReadFixture(tt.filename)
			r := &CredentialResource{}
			err := json.Unmarshal(rawPayload, r)
			require.NoError(t, err)

			dm, err := r.ConvertTo()

			if tt.err != nil {
				require.ErrorIs(t, err, tt.err)
			} else {
				require.NoError(t, err)
				ct := dm.(*datamodel.Credential)
				require.Equal(t, tt.expected, ct)
			}
		})
	}
}

func TestCredentialConvertDataModelToVersioned(t *testing.T) {
	internalStorageKind := CredentialStorageKindInternal
	conversionTests := []struct {
		filename string
		expected *CredentialResource
		err      error
	}{
		{
			filename: "credentialresourcedatamodel-aws.json",
			expected: &CredentialResource{
				ID:       to.Ptr("/planes/aws/awscloud/providers/System.AWS/credentials/default"),
				Name:     to.Ptr("default"),
				Type:     to.Ptr("System.AWS/credentials"),
				Location: to.Ptr("west-us-2"),
				Tags: map[string]*string{
					"env": to.Ptr("dev"),
				},
				Properties: &AWSCredentialProperties{
					Kind:            to.Ptr("aws.com.credential"),
					AccessKeyID:     to.Ptr("00000000-0000-0000-0000-000000000000"),
					SecretAccessKey: to.Ptr("00000000-0000-0000-0000-000000000000"),
					Storage: &InternalCredentialStorageProperties{
						Kind:       &internalStorageKind,
						SecretName: to.Ptr("aws_awscloud_default"),
					},
				},
			},
		},
		{
			filename: "credentialresourcedatamodel-azure.json",
			expected: &CredentialResource{
				ID:       to.Ptr("/planes/azure/azurecloud/providers/System.Azure/credentials/default"),
				Name:     to.Ptr("default"),
				Type:     to.Ptr("System.Azure/credentials"),
				Location: to.Ptr("west-us-2"),
				Tags: map[string]*string{
					"env": to.Ptr("dev"),
				},
				Properties: &AzureServicePrincipalProperties{
					Kind:     to.Ptr("azure.com.credential"),
					ClientID: to.Ptr("00000000-0000-0000-0000-000000000000"),
					TenantID: to.Ptr("00000000-0000-0000-0000-000000000000"),
					Storage: &InternalCredentialStorageProperties{
						Kind:       &internalStorageKind,
						SecretName: to.Ptr("azure_azurecloud_default"),
					},
				},
			},
		},
		{
			filename: "credentialresourcedatamodel-default.json",
			expected: &CredentialResource{
				ID:       to.Ptr("/planes/other/othercloud/providers/System.Other/credentials/default"),
				Name:     to.Ptr("default"),
				Type:     to.Ptr("System.Other/credentials"),
				Location: to.Ptr("west-us-2"),
				Tags: map[string]*string{
					"env": to.Ptr("dev"),
				},
				Properties: &CredentialResourceProperties{
					Kind: to.Ptr("other.com.credential"),
					Storage: &InternalCredentialStorageProperties{
						Kind:       &internalStorageKind,
						SecretName: to.Ptr("other_othercloud_default"),
					},
				},
			},
		},
	}
	for _, tt := range conversionTests {
		t.Run(tt.filename, func(t *testing.T) {
			rawPayload := radiustesting.ReadFixture(tt.filename)
			r := &datamodel.Credential{}
			err := json.Unmarshal(rawPayload, r)
			require.NoError(t, err)

			versioned := &CredentialResource{}
			err = versioned.ConvertFrom(r)

			if tt.err != nil {
				require.ErrorIs(t, err, tt.err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.expected, versioned)
			}
		})
	}
}
