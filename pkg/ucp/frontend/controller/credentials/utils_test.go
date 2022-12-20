// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package credentials

import (
	"testing"

	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_Credential(t *testing.T) {
	id, err := resources.Parse("/planes/azure/azurecloud/providers/System.Azure/credentials/default")
	require.NoError(t, err)
	secretName := GetSecretName(id)
	assert.Equal(t, secretName, "azure_azurecloud_default")
}
