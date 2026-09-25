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

package backends

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"

	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/recipes"
	"github.com/stretchr/testify/require"
)

func TestCloudBackendRendering(t *testing.T) {
	_, resource := getTestInputs()
	digest := sha256.Sum256([]byte(strings.ToLower("env-app-" + resource.ResourceID)))
	key := "radius/" + hex.EncodeToString(digest[:])[:40] + ".tfstate"
	for _, tt := range []struct {
		settings datamodel.TerraformBackend
		expected map[string]any
	}{
		{datamodel.TerraformBackend{Type: "s3", Bucket: "states", Region: "us-west-2"},
			map[string]any{"bucket": "states", "region": "us-west-2", "key": key, "use_lockfile": true}},
		{datamodel.TerraformBackend{Type: "azurerm", StorageAccountName: "states", ContainerName: "radius"},
			map[string]any{"storage_account_name": "states", "container_name": "radius", "key": key, "use_azuread_auth": true}},
	} {
		t.Run(tt.settings.Type, func(t *testing.T) {
			b := CloudBackend{Settings: &tt.settings}
			actual, err := b.BuildBackend(&resource)
			require.NoError(t, err)
			require.Equal(t, map[string]any{tt.settings.Type: tt.expected}, actual)
			encoded, err := json.Marshal(actual)
			require.NoError(t, err)
			for _, forbidden := range []string{"secret", "token", "access_key", "dynamodb", "kubernetes"} {
				require.NotContains(t, string(encoded), forbidden)
			}
			tt.settings.KeyPrefix = new("installation/team")
			actual, err = b.BuildBackend(&resource)
			require.NoError(t, err)
			require.Equal(t, strings.Replace(key, "radius/", "installation/team/", 1), actual[tt.settings.Type].(map[string]any)["key"])
		})
	}
}

func TestCloudBackendKeyLengthBoundary(t *testing.T) {
	_, resource := getTestInputs()
	for _, settings := range []datamodel.TerraformBackend{
		{Type: "s3", Bucket: "states", Region: "us-west-2"},
		{Type: "azurerm", StorageAccountName: "states", ContainerName: "radius"},
	} {
		t.Run(settings.Type, func(t *testing.T) {
			prefix := strings.Repeat("a", 968)
			settings.KeyPrefix = &prefix
			b := CloudBackend{Settings: &settings}
			actual, err := b.BuildBackend(&resource)
			require.NoError(t, err)
			rendered := actual[settings.Type].(map[string]any)
			key := rendered["key"].(string)
			require.Regexp(t, "^"+prefix+"/[0-9a-f]{40}\\.tfstate$", key)
			require.Len(t, []byte(key), 1017)
			if settings.Type == "s3" {
				require.Equal(t, true, rendered["use_lockfile"])
				require.Len(t, []byte(key+".tflock"), 1024)
			}

			prefix += "a"
			require.ErrorContains(t, settings.Validate(), "backend.keyPrefix")
			actual, err = b.BuildBackend(&resource)
			require.ErrorContains(t, err, "backend.keyPrefix")
			require.Nil(t, actual)
		})
	}
}

func TestCloudStateKeyStabilityAndIsolation(t *testing.T) {
	_, resource := getTestInputs()
	b := CloudBackend{Settings: &datamodel.TerraformBackend{Type: "s3", Bucket: "states", Region: "us-west-2"}}
	first, err := b.BuildBackend(&resource)
	require.NoError(t, err)
	key := first["s3"].(map[string]any)["key"]
	// Parameter and recipe updates must continue to address the original state.
	resource.Parameters = map[string]any{"new": "value"}
	resource.Name = "updated-recipe"
	for range 3 {
		next, err := b.BuildBackend(&resource)
		require.NoError(t, err)
		require.Equal(t, key, next["s3"].(map[string]any)["key"])
	}
	for _, mutate := range []func(*recipes.ResourceMetadata){
		func(r *recipes.ResourceMetadata) { r.ResourceID += "2" },
		func(r *recipes.ResourceMetadata) {
			r.ResourceID = strings.Replace(r.ResourceID, "test-group", "other-group", 1)
		},
		func(r *recipes.ResourceMetadata) { r.EnvironmentID += "2" },
		func(r *recipes.ResourceMetadata) { r.ApplicationID += "2" },
	} {
		other := resource
		mutate(&other)
		next, err := b.BuildBackend(&other)
		require.NoError(t, err)
		require.NotEqual(t, key, next["s3"].(map[string]any)["key"])
	}
	resource.ResourceID = strings.TrimSuffix(resource.ResourceID, "/redis") + "/REDIS"
	next, err := b.BuildBackend(&resource)
	require.NoError(t, err)
	require.Equal(t, key, next["s3"].(map[string]any)["key"])
	resource.ResourceID = "invalid"
	_, err = b.BuildBackend(&resource)
	require.Error(t, err)
	_, err = b.BuildBackend(nil)
	require.Error(t, err)
	b.Settings.Type = "local"
	_, err = b.BuildBackend(&resource)
	require.Error(t, err)
}
