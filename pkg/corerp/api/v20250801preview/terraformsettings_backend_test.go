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

package v20250801preview

import (
	"encoding/json"
	"testing"

	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/stretchr/testify/require"
)

func TestTerraformSettingsBackendConversion(t *testing.T) {
	for _, backend := range []string{
		`null`,
		`{"type":"s3","bucket":"states","region":"us-west-2"}`,
		`{"type":"s3","bucket":"states","region":"us-west-2","keyPrefix":"team/install"}`,
		`{"type":"azurerm","storageAccountName":"states","containerName":"radius"}`,
		`{"type":"azurerm","storageAccountName":"states","containerName":"radius","keyPrefix":"radius"}`,
	} {
		t.Run(backend, func(t *testing.T) {
			var versioned TerraformSettingsResource
			require.NoError(t, json.Unmarshal([]byte(`{"properties":{"backend":`+backend+`}}`), &versioned))
			dm, err := versioned.ConvertTo()
			require.NoError(t, err)
			actual := dm.(*datamodel.TerraformSettings).Properties.Backend
			if backend == "null" {
				require.Nil(t, actual)
			} else {
				require.NoError(t, actual.Validate())
				if actual.KeyPrefix == nil {
					require.Equal(t, "radius", actual.EffectiveKeyPrefix())
				}
			}
			var roundtrip TerraformSettingsResource
			require.NoError(t, roundtrip.ConvertFrom(dm))
			dm2, err := roundtrip.ConvertTo()
			require.NoError(t, err)
			require.Equal(t, actual, dm2.(*datamodel.TerraformSettings).Properties.Backend)
			if backend != "null" {
				encoded, err := json.Marshal(roundtrip.Properties.Backend)
				require.NoError(t, err)
				require.JSONEq(t, backend, string(encoded))
			}
		})
	}
	for _, backend := range []string{
		`{"type":"local"}`,
		`{"type":"s3","bucket":"states"}`,
		`{"type":"azurerm","containerName":"radius"}`,
		`{"type":"s3","bucket":"states","region":"us-west-2","keyPrefix":""}`,
	} {
		var versioned TerraformSettingsResource
		require.NoError(t, json.Unmarshal([]byte(`{"properties":{"backend":`+backend+`}}`), &versioned))
		_, err := versioned.ConvertTo()
		require.Error(t, err)
	}
}
