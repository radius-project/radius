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

func TestAWSCredentialConvertVersionedToDataModel(t *testing.T) {
	conversionTests := []struct {
		filename string
		expected *datamodel.AWSCredential
		err      error
	}{
		{
			filename: "credentialresource-aws.json",
			expected: &datamodel.AWSCredential{
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
				Properties: &datamodel.AWSCredentialResourceProperties{
					Kind: "AccessKey",
					AWSCredential: &datamodel.AWSCredentialProperties{
						AccessKeyID:     "00000000-0000-0000-0000-000000000000",
						SecretAccessKey: "00000000-0000-0000-0000-000000000000",
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
			filename: "credentialresource-empty-storage-aws.json",
			err:      &v1.ErrModelConversion{PropertyName: "$.properties.storage", ValidValue: "not nil"},
		},
		{
			filename: "credentialresource-empty-storage-kind-aws.json",
			err:      &v1.ErrModelConversion{PropertyName: "$.properties.storage.kind", ValidValue: "not nil"},
		},
		{
			filename: "credentialresource-invalid-storagekind-aws.json",
			err:      &v1.ErrModelConversion{PropertyName: "$.properties.storage.kind", ValidValue: fmt.Sprintf("one of %q", PossibleCredentialStorageKindValues())},
		},
	}
	for _, tt := range conversionTests {
		t.Run(tt.filename, func(t *testing.T) {
			rawPayload := testutil.ReadFixture(tt.filename)
			r := &AWSCredentialResource{}
			err := json.Unmarshal(rawPayload, r)
			require.NoError(t, err)

			dm, err := r.ConvertTo()

			if tt.err != nil {
				require.ErrorIs(t, err, tt.err)
			} else {
				require.NoError(t, err)
				ct := dm.(*datamodel.AWSCredential)
				require.Equal(t, tt.expected, ct)
			}
		})
	}
}

func TestAWSCredentialConvertDataModelToVersioned(t *testing.T) {
	conversionTests := []struct {
		filename string
		expected *AWSCredentialResource
		err      error
	}{
		{
			filename: "credentialresourcedatamodel-aws.json",
			expected: &AWSCredentialResource{
				ID:       to.Ptr("/planes/aws/awscloud/providers/System.AWS/credentials/default"),
				Name:     to.Ptr("default"),
				Type:     to.Ptr("System.AWS/credentials"),
				Location: to.Ptr("west-us-2"),
				Tags: map[string]*string{
					"env": to.Ptr("dev"),
				},
				Properties: &AWSAccessKeyCredentialProperties{
					Kind:        to.Ptr("AccessKey"),
					AccessKeyID: to.Ptr("00000000-0000-0000-0000-000000000000"),
					Storage: &InternalCredentialStorageProperties{
						Kind:       to.Ptr(string(CredentialStorageKindInternal)),
						SecretName: to.Ptr("aws-awscloud-default"),
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
			r := &datamodel.AWSCredential{}
			err := json.Unmarshal(rawPayload, r)
			require.NoError(t, err)

			versioned := &AWSCredentialResource{}
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
