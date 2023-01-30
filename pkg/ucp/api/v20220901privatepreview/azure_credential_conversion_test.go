// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v20220901privatepreview

import (
	"encoding/json"
	"fmt"
	"testing"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/ucp/datamodel"
	"github.com/project-radius/radius/test/testutil"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/stretchr/testify/require"
)

func TestAzureCredentialConvertVersionedToDataModel(t *testing.T) {
	conversionTests := []struct {
		filename string
		expected *datamodel.AzureCredential
		err      error
	}{
		{
			filename: "credentialresource-azure.json",
			expected: &datamodel.AzureCredential{
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
				Properties: &datamodel.AzureCredentialResourceProperties{
					Kind: "ServicePrincipal",
					AzureCredential: &datamodel.AzureCredentialProperties{
						TenantID:     "00000000-0000-0000-0000-000000000000",
						ClientID:     "00000000-0000-0000-0000-000000000000",
						ClientSecret: "secret",
					},
					Storage: &datamodel.CredentialStorageProperties{
						Kind:               datamodel.InternalStorageKind,
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
			filename: "credentialresource-empty-storage-azure.json",
			err:      &v1.ErrModelConversion{PropertyName: "$.properties.storage", ValidValue: "not nil"},
		},
		{
			filename: "credentialresource-empty-storage-kind-azure.json",
			err:      &v1.ErrModelConversion{PropertyName: "$.properties.storage.kind", ValidValue: "not nil"},
		},
		{
			filename: "credentialresource-invalid-storagekind-azure.json",
			err:      &v1.ErrModelConversion{PropertyName: "$.properties.storage.kind", ValidValue: fmt.Sprintf("one of %q", PossibleCredentialStorageKindValues())},
		},
	}
	for _, tt := range conversionTests {
		t.Run(tt.filename, func(t *testing.T) {
			rawPayload := testutil.ReadFixture(tt.filename)
			r := &AzureCredentialResource{}
			err := json.Unmarshal(rawPayload, r)
			require.NoError(t, err)

			dm, err := r.ConvertTo()

			if tt.err != nil {
				require.ErrorIs(t, err, tt.err)
			} else {
				require.NoError(t, err)
				ct := dm.(*datamodel.AzureCredential)
				require.Equal(t, tt.expected, ct)
			}
		})
	}
}

func TestAzureCredentialConvertDataModelToVersioned(t *testing.T) {
	conversionTests := []struct {
		filename string
		expected *AzureCredentialResource
		err      error
	}{
		{
			filename: "credentialresourcedatamodel-azure.json",
			expected: &AzureCredentialResource{
				ID:       to.Ptr("/planes/azure/azurecloud/providers/System.Azure/credentials/default"),
				Name:     to.Ptr("default"),
				Type:     to.Ptr("System.Azure/credentials"),
				Location: to.Ptr("west-us-2"),
				Tags: map[string]*string{
					"env": to.Ptr("dev"),
				},
				Properties: &AzureServicePrincipalProperties{
					Kind:     to.Ptr("ServicePrincipal"),
					ClientID: to.Ptr("00000000-0000-0000-0000-000000000000"),
					TenantID: to.Ptr("00000000-0000-0000-0000-000000000000"),
					Storage: &InternalCredentialStorageProperties{
						Kind:       to.Ptr(string(CredentialStorageKindInternal)),
						SecretName: to.Ptr("azure-azurecloud-default"),
					},
				},
			},
		},
		{
			filename: "credentialresourcedatamodel-default.json",
			err:      v1.ErrInvalidModelConversion,
		},
	}
	for _, tt := range conversionTests {
		t.Run(tt.filename, func(t *testing.T) {
			rawPayload := testutil.ReadFixture(tt.filename)
			r := &datamodel.AzureCredential{}
			err := json.Unmarshal(rawPayload, r)
			require.NoError(t, err)

			versioned := &AzureCredentialResource{}
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
