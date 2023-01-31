// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package clients

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_GetProviderConfigs(t *testing.T) {
	expectedConfig := ProviderConfig{
		Deployments: &Deployments{
			Type: ProviderTypeDeployments,
			Value: Value{
				Scope: "/planes/deployments/local/resourceGroups/" + "testrg",
			},
		},
		Radius: &Radius{
			Type: ProviderTypeRadius,
			Value: Value{
				Scope: "/planes/radius/local/resourceGroups/" + "testrg",
			},
		},
	}

	providerConfig := NewDefaultProviderConfig("testrg")
	require.Equal(t, providerConfig, expectedConfig)
}
