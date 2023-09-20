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

package v20220901privatepreview

import (
	"encoding/json"
	"fmt"
	"testing"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/ucp/datamodel"
	"github.com/radius-project/radius/test/testutil"

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
						ID:       "/planes/aws/aws/providers/System.AWS/credentials/default",
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
			r := &AwsCredentialResource{}
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
		expected *AwsCredentialResource
		err      error
	}{
		{
			filename: "credentialresourcedatamodel-aws.json",
			expected: &AwsCredentialResource{
				ID:       to.Ptr("/planes/aws/aws/providers/System.AWS/credentials/default"),
				Name:     to.Ptr("default"),
				Type:     to.Ptr("System.AWS/credentials"),
				Location: to.Ptr("west-us-2"),
				Tags: map[string]*string{
					"env": to.Ptr("dev"),
				},
				Properties: &AwsAccessKeyCredentialProperties{
					Kind:        to.Ptr(AWSCredentialKindAccessKey),
					AccessKeyID: to.Ptr("00000000-0000-0000-0000-000000000000"),
					Storage: &InternalCredentialStorageProperties{
						Kind:       to.Ptr(CredentialStorageKindInternal),
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

			versioned := &AwsCredentialResource{}
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
