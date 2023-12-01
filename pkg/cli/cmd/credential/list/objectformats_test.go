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

package list

import (
	"bytes"
	"testing"

	"github.com/radius-project/radius/pkg/cli/credential"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/stretchr/testify/require"
)

func Test_credentialFormat(t *testing.T) {
	obj := credential.ProviderCredentialConfiguration{
		CloudProviderStatus: credential.CloudProviderStatus{
			Name:    "test",
			Enabled: true,
		},
	}

	buffer := &bytes.Buffer{}
	err := output.Write(output.FormatTable, obj, buffer, credentialFormat())
	require.NoError(t, err)

	expected := "PROVIDER  REGISTERED\ntest      true\n"
	require.Equal(t, expected, buffer.String())
}
