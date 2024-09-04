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

package v20231001preview

import (
	"encoding/json"
	"testing"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/test/testutil"
	"github.com/radius-project/radius/test/testutil/resourcetypeutil"

	"github.com/stretchr/testify/require"
)

func TestSecretStoreConvertVersionedToDataModel(t *testing.T) {
	t.Run("only values", func(t *testing.T) {
		// arrange
		rawPayload := testutil.ReadFixture("secretstore-versioned.json")
		r := &SecretStoreResource{}
		err := json.Unmarshal(rawPayload, r)
		require.NoError(t, err)

		// act
		dm, err := r.ConvertTo()

		// assert
		require.NoError(t, err)
		ct := dm.(*datamodel.SecretStore)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/secretStores/secret0", ct.ID)
		require.Equal(t, "secret0", ct.Name)
		require.Equal(t, "global", ct.Location)
		require.Equal(t, "Applications.Core/secretStores", ct.Type)
		require.Equal(t, "dev", ct.Tags["env"])
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup/providers/Applications.Core/applications/app0", ct.Properties.Application)
		require.Equal(t, []rpv1.OutputResource(nil), ct.Properties.Status.OutputResources)
		require.Equal(t, "2023-10-01-preview", ct.InternalMetadata.UpdatedAPIVersion)
		require.Equal(t, "certificate", string(ct.Properties.Type))
		require.Equal(t, datamodel.SecretValueEncodingBase64, ct.Properties.Data["tls.crt"].Encoding)
		require.Equal(t, "-----BEGIN CERT---- ...", to.String(ct.Properties.Data["tls.crt"].Value))
		require.Nil(t, ct.Properties.Data["tls.crt"].ValueFrom)
		require.Equal(t, datamodel.SecretValueEncodingNone, ct.Properties.Data["tls.key"].Encoding)
		require.Equal(t, "-----BEGIN KEY---- ...", to.String(ct.Properties.Data["tls.key"].Value))
		require.Nil(t, ct.Properties.Data["tls.key"].ValueFrom)
	})

	t.Run("using valueFrom", func(t *testing.T) {
		// arrange
		rawPayload := testutil.ReadFixture("secretstore-versioned-resource.json")
		r := &SecretStoreResource{}
		err := json.Unmarshal(rawPayload, r)
		require.NoError(t, err)

		// act
		dm, err := r.ConvertTo()

		// assert
		require.NoError(t, err)
		ct := dm.(*datamodel.SecretStore)
		require.Equal(t, "certificate", string(ct.Properties.Type))
		require.Equal(t, "secret/tls_cert", ct.Properties.Data["tls.crt"].ValueFrom.Name)
		require.Equal(t, "1", ct.Properties.Data["tls.crt"].ValueFrom.Version)
		require.Nil(t, ct.Properties.Data["tls.crt"].Value)

		require.Equal(t, "secret/tls_key", ct.Properties.Data["tls.key"].ValueFrom.Name)
		require.Equal(t, "1", ct.Properties.Data["tls.key"].ValueFrom.Version)
		require.Nil(t, ct.Properties.Data["tls.key"].Value)

		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup/providers/Microsoft.KeyVault/vaults/vault0", ct.Properties.Resource)
	})
}

func TestSecretStoreConvertDataModelToVersioned(t *testing.T) {
	t.Run("only values", func(t *testing.T) {
		// arrange
		rawPayload := testutil.ReadFixture("secretstore-datamodel.json")
		r := &datamodel.SecretStore{}
		err := json.Unmarshal(rawPayload, r)
		require.NoError(t, err)

		// act
		versioned := &SecretStoreResource{}
		err = versioned.ConvertFrom(r)

		// assert
		require.NoError(t, err)
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/secretStores/secret0", r.ID)
		require.Equal(t, "secret0", r.Name)
		require.Equal(t, "global", r.Location)
		require.Equal(t, "Applications.Core/secretStores", r.Type)
		require.Equal(t, "dev", r.Tags["env"])
		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup/providers/Applications.Core/applications/app0", r.Properties.Application)
		require.Equal(t, resourcetypeutil.MustPopulateResourceStatus(&ResourceStatus{}), versioned.Properties.Status)
		require.Equal(t, "certificate", string(*versioned.Properties.Type))
		require.Nil(t, versioned.Properties.Data["tls.crt"].Encoding)
		require.Equal(t, "", to.String(versioned.Properties.Data["tls.crt"].Value))
		require.Nil(t, versioned.Properties.Data["tls.crt"].ValueFrom)
		require.Equal(t, SecretValueEncodingBase64, *versioned.Properties.Data["tls.key"].Encoding)
		require.Equal(t, "", to.String(versioned.Properties.Data["tls.key"].Value))
		require.Nil(t, versioned.Properties.Data["tls.key"].ValueFrom)
	})

	t.Run("valueFrom", func(t *testing.T) {
		// arrange
		rawPayload := testutil.ReadFixture("secretstore-datamodel-resource.json")
		r := &datamodel.SecretStore{}
		err := json.Unmarshal(rawPayload, r)
		require.NoError(t, err)

		// act
		versioned := &SecretStoreResource{}
		err = versioned.ConvertFrom(r)

		// assert
		require.NoError(t, err)

		require.Equal(t, "certificate", string(*versioned.Properties.Type))
		require.Equal(t, "secret/tls_cert", to.String(versioned.Properties.Data["tls.crt"].ValueFrom.Name))
		require.Equal(t, "1", to.String(versioned.Properties.Data["tls.crt"].ValueFrom.Version))
		require.Nil(t, versioned.Properties.Data["tls.crt"].Value)

		require.Equal(t, "secret/tls_key", to.String(versioned.Properties.Data["tls.key"].ValueFrom.Name))
		require.Equal(t, "1", to.String(versioned.Properties.Data["tls.key"].ValueFrom.Version))
		require.Nil(t, versioned.Properties.Data["tls.key"].Value)

		require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup/providers/Microsoft.KeyVault/vaults/vault0", to.String(versioned.Properties.Resource))
	})
}

func TestSecretStoreConvertFromValidation(t *testing.T) {
	validationTests := []struct {
		src v1.ResourceDataModel
		err error
	}{
		{&resourcetypeutil.FakeResource{}, v1.ErrInvalidModelConversion},
		{nil, v1.ErrInvalidModelConversion},
	}

	for _, tc := range validationTests {
		versioned := &SecretStoreResource{}
		err := versioned.ConvertFrom(tc.src)
		require.ErrorAs(t, tc.err, &err)
	}
}

func TestSecretStorefromSecretStoreDataTypeDataModel(t *testing.T) {
	tests := []struct {
		name     string
		input    datamodel.SecretType
		expected *SecretStoreDataType
	}{
		{
			name:     "Generic Secret Type",
			input:    datamodel.SecretTypeGeneric,
			expected: to.Ptr(SecretStoreDataTypeGeneric),
		},
		{
			name:     "Certificate Secret Type",
			input:    datamodel.SecretTypeCert,
			expected: to.Ptr(SecretStoreDataTypeCertificate),
		},
		{
			name:     "Basic Authentication Secret Type",
			input:    datamodel.SecretTypeBasicAuthentication,
			expected: to.Ptr(SecretStoreDataTypeBasicAuthentication),
		},
		{
			name:     "Azure Workload Identity Secret Type",
			input:    datamodel.SecretTypeAzureWorkloadIdentity,
			expected: to.Ptr(SecretStoreDataTypeAzureWorkloadIdentity),
		},
		{
			name:     "AWS IRSA Secret Type",
			input:    datamodel.SecretTypeAWSIRSA,
			expected: to.Ptr(SecretStoreDataTypeAwsIRSA),
		},
		{
			name:     "None Secret Type",
			input:    datamodel.SecretTypeNone,
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := fromSecretStoreDataTypeDataModel(tt.input)
			require.Equal(t, tt.expected, result)
		})
	}
}
