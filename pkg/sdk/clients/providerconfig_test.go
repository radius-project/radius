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

package clients

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_GetProviderConfigs(t *testing.T) {
	expectedConfig := ProviderConfig{
		Deployments: &Deployments{
			Type: ProviderTypeDeployments,
			Value: Value{
				Scope: "/planes/radius/local/resourceGroups/" + "testrg",
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

func Test_GenerateProviderConfig_AWSOwner(t *testing.T) {
	t.Parallel()

	const awsScope = "/planes/aws/aws/accounts/123456789012/regions/us-west-2"

	tests := []struct {
		name          string
		resourceGroup string
		awsScope      string
		wantAWS       *AWS
	}{
		{
			name:          "AWS scope and resource group",
			resourceGroup: "default",
			awsScope:      awsScope,
			wantAWS:       &AWS{Type: ProviderTypeAWS, Value: Value{Scope: awsScope, Owner: "deploy;/planes/radius/local/resourceGroups/default;;"}},
		},
		{
			name:     "AWS scope without a resource group",
			awsScope: awsScope,
			wantAWS:  &AWS{Type: ProviderTypeAWS, Value: Value{Scope: awsScope}},
		},
		{
			name:          "no AWS scope",
			resourceGroup: "default",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			config := GenerateProviderConfig(tt.resourceGroup, tt.awsScope, "/subscriptions/s/resourceGroups/g")
			require.Equal(t, tt.wantAWS, config.AWS)
			require.Empty(t, config.Az.Value.Owner, "only the AWS provider has an owner")
		})
	}
}

// Test_ProviderConfig_OwnerRoundTrip checks that a provider config read back from JSON, as the DeploymentTemplate
// reconciler does with spec.providerConfig, keeps the AWS owner.
func Test_ProviderConfig_OwnerRoundTrip(t *testing.T) {
	t.Parallel()

	b, err := json.Marshal(GenerateProviderConfig("default", "/planes/aws/aws/accounts/1/regions/r", ""))
	require.NoError(t, err)

	var config ProviderConfig
	require.NoError(t, json.Unmarshal(b, &config))
	require.Equal(t, "deploy;/planes/radius/local/resourceGroups/default;;", config.AWS.Value.Owner)
}
