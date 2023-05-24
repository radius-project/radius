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

package azure

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/corerp/renderers"
	"github.com/project-radius/radius/pkg/to"
	"github.com/project-radius/radius/test/testutil"

	azcsi "github.com/Azure/secrets-store-csi-driver-provider-azure/pkg/provider/types"
	"github.com/stretchr/testify/require"
)

func TestGetValuesOrDefaultsForSecrets(t *testing.T) {
	secretTests := []struct {
		name string
		prop *datamodel.SecretObjectProperties
		out  objectValues
	}{
		{
			name: "custom",
			prop: &datamodel.SecretObjectProperties{
				Alias:    "alias",
				Version:  "1",
				Encoding: to.Ptr(datamodel.SecretObjectPropertiesEncodingUTF8),
			},
			out: objectValues{
				alias:    "alias",
				version:  "1",
				encoding: string(datamodel.SecretObjectPropertiesEncodingUTF8),
			},
		},
		{
			name: "default",
			prop: &datamodel.SecretObjectProperties{
				Version: "1",
			},
			out: objectValues{
				alias:    "default",
				version:  "1",
				encoding: string(datamodel.SecretObjectPropertiesEncodingUTF8),
			},
		},
	}

	for _, tc := range secretTests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.out, getValuesOrDefaultsForSecrets(tc.name, tc.prop))
		})
	}
}

func TestGetValuesOrDefaultsForKeys(t *testing.T) {
	keyTests := []struct {
		name string
		prop *datamodel.KeyObjectProperties
		out  objectValues
	}{
		{
			name: "custom",
			prop: &datamodel.KeyObjectProperties{
				Alias:   "alias",
				Version: "1",
			},
			out: objectValues{
				alias:   "alias",
				version: "1",
			},
		},
		{
			name: "default",
			prop: &datamodel.KeyObjectProperties{
				Version: "1",
			},
			out: objectValues{
				alias:   "default",
				version: "1",
			},
		},
	}

	for _, tc := range keyTests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.out, getValuesOrDefaultsForKeys(tc.name, tc.prop))
		})
	}
}

func TestGetValuesOrDefaultsForCertificates(t *testing.T) {
	certTests := []struct {
		name string
		prop *datamodel.CertificateObjectProperties
		out  objectValues
	}{
		{
			name: "custom",
			prop: &datamodel.CertificateObjectProperties{
				Alias:    "alias",
				Version:  "1",
				Format:   to.Ptr(datamodel.CertificateFormatPEM),
				CertType: to.Ptr(datamodel.CertificateTypePrivateKey),
				Encoding: to.Ptr(datamodel.SecretObjectPropertiesEncodingHex),
			},
			out: objectValues{
				alias:    "alias",
				version:  "1",
				format:   string(datamodel.CertificateFormatPEM),
				encoding: string(datamodel.SecretObjectPropertiesEncodingHex),
			},
		},
		{
			name: "default",
			prop: &datamodel.CertificateObjectProperties{
				Alias:    "alias",
				Version:  "1",
				CertType: to.Ptr(datamodel.CertificateTypePrivateKey),
			},
			out: objectValues{
				alias:    "alias",
				version:  "1",
				format:   string(datamodel.CertificateFormatPFX),
				encoding: string(datamodel.SecretObjectPropertiesEncodingUTF8),
			},
		},
	}

	for _, tc := range certTests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.out, getValuesOrDefaultsForCertificates(tc.name, tc.prop))
		})
	}
}

func TestGetKeyVaultObjectsSpec(t *testing.T) {
	keyVaultObjects := []azcsi.KeyVaultObject{
		{
			ObjectName:     "mysecret",
			ObjectAlias:    "myalias",
			ObjectType:     "secret",
			ObjectVersion:  "1",
			ObjectEncoding: "utf-8",
		},
		{
			ObjectName:    "mykey",
			ObjectAlias:   "mykeyalias",
			ObjectType:    "key",
			ObjectVersion: "1",
		},
	}
	expected := `array:
    - |
      objectName: mysecret
      objectAlias: myalias
      objectVersion: "1"
      objectVersionHistory: 0
      objectType: secret
      objectFormat: ""
      objectEncoding: utf-8
      filePermission: ""
    - |
      objectName: mykey
      objectAlias: mykeyalias
      objectVersion: "1"
      objectVersionHistory: 0
      objectType: key
      objectFormat: ""
      objectEncoding: ""
      filePermission: ""
`
	serialized, err := getKeyVaultObjectsSpec(keyVaultObjects)
	require.NoError(t, err)
	require.Equal(t, expected, serialized)
}

func TestKeyVaultRenderer_Render(t *testing.T) {
	r := KeyVaultRenderer{}
	ctx := context.Background()

	vol := &datamodel.VolumeResource{}
	err := json.Unmarshal(testutil.ReadFixture("volume-az-kv-systemassigned.json"), vol)
	require.NoError(t, err)
	param := "array:\n    - |\n      objectName: mysecret\n      objectAlias: mysecret\n      objectVersion: \"\"\n      objectVersionHistory: 0\n      objectType: secret\n      objectFormat: \"\"\n      objectEncoding: base64\n      filePermission: \"\"\n    - |\n      objectName: mykey\n      objectAlias: mykey\n      objectVersion: \"\"\n      objectVersionHistory: 0\n      objectType: key\n      objectFormat: \"\"\n      objectEncoding: \"\"\n      filePermission: \"\"\n    - |\n      objectName: mycert\n      objectAlias: myalias\n      objectVersion: \"\"\n      objectVersionHistory: 0\n      objectType: certificate\n      objectFormat: pfx\n      objectEncoding: \"\"\n      filePermission: \"\"\n"

	actual, err := r.Render(ctx, vol, &renderers.RenderOptions{
		Environment: renderers.EnvironmentOptions{
			Namespace: "default",
		},
	})

	require.NoError(t, err)
	require.Equal(t, param, actual.ComputedValues[SPCVolumeObjectSpecKey].Value.(string))
}
