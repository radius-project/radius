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
package credentials

import (
	"testing"

	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_Credential(t *testing.T) {
	id, err := resources.Parse("/planes/azure/azurecloud/providers/System.Azure/credentials/default")
	require.NoError(t, err)
	secretName := GetSecretName(id)
	assert.Equal(t, secretName, "azure-azurecloud-default")
}
