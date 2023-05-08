/*
------------------------------------------------------------
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
------------------------------------------------------------
*/

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
