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

package converter

import (
	"testing"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	v20250801preview "github.com/radius-project/radius/pkg/corerp/api/v20250801preview"
	"github.com/stretchr/testify/require"
)

func TestTerraformBackendInvalidRequest(t *testing.T) {
	for _, backend := range []string{
		`{"type":"s3","bucket":"states","region":"us-west-2","containerName":"other"}`,
		`{"type":"azurerm","storageAccountName":"states","containerName":"radius","bucket":"other"}`,
		`{"type":"s3","bucket":"states","region":"us-west-2","access_key":"do-not-accept"}`,
		`{"type":"s3","bucket":"states"}`,
		`{"type":"azurerm","storageAccountName":"states"}`,
		`{"type":"local"}`,
		`{"type":"s3","bucket":"states","region":"us-west-2","keyPrefix":""}`,
		`{"type":"s3","bucket":"states","region":"us-west-2","keyPrefix":"a//b"}`,
	} {
		t.Run(backend, func(t *testing.T) {
			_, err := TerraformSettingsDataModelFromVersioned([]byte(`{"properties":{"backend":`+backend+`}}`), v20250801preview.Version)
			var clientErr *v1.ErrClientRP
			require.ErrorAs(t, err, &clientErr)
			require.NotContains(t, err.Error(), "do-not-accept")
		})
	}
}
