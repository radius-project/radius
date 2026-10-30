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

const (
	testAppID = "/planes/radius/local/resourceGroups/rg/providers/Applications.Core/applications/app"
	testEnvID = "/planes/radius/local/resourceGroups/rg/providers/Applications.Core/environments/env"
)

func Test_AWSRecipeOwner(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		applicationID string
		want          string
	}{
		{
			name:          "with an application",
			applicationID: testAppID,
			want:          "recipe;" + testAppID + ";" + testEnvID + ";/planes/radius/local/resourceGroups/rg/providers/Applications.Core/extenders/e",
		},
		{
			name:          "without an application",
			applicationID: "",
			want:          "recipe;;" + testEnvID + ";/planes/radius/local/resourceGroups/rg/providers/Applications.Core/extenders/e",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := AWSRecipeOwner(tt.applicationID, testEnvID, "/planes/radius/local/resourceGroups/rg/providers/Applications.Core/extenders/e")
			require.Equal(t, tt.want, got)
		})
	}
}

func Test_AWSDeploymentOwner(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		applicationID string
		environmentID string
		want          string
	}{
		{
			name:          "with an application",
			applicationID: testAppID,
			environmentID: testEnvID,
			want:          "deploy;/planes/radius/local/resourceGroups/rg;" + testAppID + ";" + testEnvID,
		},
		{
			name:          "without an application",
			environmentID: testEnvID,
			want:          "deploy;/planes/radius/local/resourceGroups/rg;;" + testEnvID,
		},
		{
			name: "without an application or environment",
			want: "deploy;/planes/radius/local/resourceGroups/rg;;",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, AWSDeploymentOwner("rg", tt.applicationID, tt.environmentID))
		})
	}
}

func Test_AWSOwners_AreDistinct(t *testing.T) {
	t.Parallel()

	resourceID := "/planes/radius/local/resourceGroups/rg/providers/Applications.Core/extenders/e"
	otherEnvID := "/planes/radius/local/resourceGroups/rg/providers/Applications.Core/environments/other"

	// One app deployed to two environments has two owners.
	require.NotEqual(t, AWSDeploymentOwner("rg", testAppID, testEnvID), AWSDeploymentOwner("rg", testAppID, otherEnvID))
	require.NotEqual(t, AWSRecipeOwner(testAppID, testEnvID, resourceID), AWSRecipeOwner(testAppID, otherEnvID, resourceID))
}

func Test_Value_OwnerJSON(t *testing.T) {
	t.Parallel()

	b, err := json.Marshal(ProviderConfig{AWS: &AWS{Type: ProviderTypeAWS, Value: Value{Scope: "/planes/aws/aws/accounts/1/regions/r", Owner: "o"}}})
	require.NoError(t, err)
	require.JSONEq(t, `{"aws":{"type":"AWS","value":{"scope":"/planes/aws/aws/accounts/1/regions/r","owner":"o"}}}`, string(b))

	b, err = json.Marshal(NewDefaultProviderConfig("rg"))
	require.NoError(t, err)
	require.NotContains(t, string(b), "owner", "providers without an owner do not send one")
}
