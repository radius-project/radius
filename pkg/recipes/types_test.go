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

package recipes

import (
	"testing"

	"github.com/radius-project/radius/pkg/controller/api/radapp.io/v1alpha3"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	"github.com/stretchr/testify/require"
)

func TestRecipeOutput_PrepareRecipeResponse(t *testing.T) {
	tests := []struct {
		desc        string
		result      map[string]any
		recipe      rpv1.RecipeStatus
		expectedErr bool
	}{
		{
			desc: "all valid result values",
			result: map[string]any{
				"values": map[string]any{
					"host": "testhost",
					"port": float64(6379),
				},
				"secrets": map[string]any{
					"connectionString": "testConnectionString",
				},
				"resources": []string{"outputResourceId1"},
			},
		},
		{
			desc:   "empty result",
			result: map[string]any{},
		},
		{
			desc: "invalid field",
			result: map[string]any{
				"invalid": "invalid-field",
			},
			expectedErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			ro := &RecipeOutput{}
			if !tt.expectedErr {
				err := ro.PrepareRecipeResponse(tt.result)
				require.NoError(t, err)

				if tt.result["values"] != nil {
					require.Equal(t, tt.result["values"], ro.Values)
					require.Equal(t, tt.result["secrets"], ro.Secrets)
					require.Equal(t, tt.result["resources"], ro.Resources)
				} else {
					require.Equal(t, map[string]any{}, ro.Values)
					require.Equal(t, map[string]any{}, ro.Secrets)
					require.Equal(t, []string{}, ro.Resources)
				}
			} else {
				err := ro.PrepareRecipeResponse(tt.result)
				require.Error(t, err)
				require.Equal(t, "json: unknown field \"invalid\"", err.Error())
			}
		})
	}
}

func Test_GetEnvAppResourceNames(t *testing.T) {
	tests := []struct {
		desc        string
		metadata    ResourceMetadata
		expApp      string
		expEnv      string
		expResource string
		expectedErr bool
	}{
		{
			desc: "success",
			metadata: ResourceMetadata{
				Name:          "redis-azure",
				ApplicationID: "/planes/radius/local/resourcegroups/test-rg/providers/applications.core/applications/app1",
				EnvironmentID: "/planes/radius/local/resourcegroups/test-rg/providers/applications.core/environments/env1",
				ResourceID:    "/planes/radius/local/resourceGroups/test-rg/providers/applications.datastores/rediscaches/test-redis-recipe",
				Parameters: map[string]any{
					"redis_cache_name": "redis-test",
				},
			},
			expApp:      "app1",
			expEnv:      "env1",
			expResource: "test-redis-recipe",
			expectedErr: false,
		},
		{
			desc: "invalid env id",
			metadata: ResourceMetadata{
				Name:          "redis-azure",
				ApplicationID: "/planes/radius/local/resourcegroups/test-rg/providers/applications.core/applications/app1",
				EnvironmentID: "//planes/radius/local/resourcegroups/test-rg/providers/applications.core/environments/env1",
				ResourceID:    "/planes/radius/local/resourceGroups/test-rg/providers/applications.datastores/rediscaches/test-redis-recipe",
				Parameters: map[string]any{
					"redis_cache_name": "redis-test",
				},
			},
			expApp:      "app1",
			expEnv:      "env1",
			expResource: "test-redis-recipe",
			expectedErr: true,
		},
		{
			desc: "invalid app id",
			metadata: ResourceMetadata{
				Name:          "redis-azure",
				ApplicationID: "//planes/radius/local/resourcegroups/test-rg/providers/applications.core/applications/app1",
				EnvironmentID: "/planes/radius/local/resourcegroups/test-rg/providers/applications.core/environments/env1",
				ResourceID:    "/planes/radius/local/resourceGroups/test-rg/providers/applications.datastores/rediscaches/test-redis-recipe",
				Parameters: map[string]any{
					"redis_cache_name": "redis-test",
				},
			},
			expApp:      "app1",
			expEnv:      "env1",
			expResource: "test-redis-recipe",
			expectedErr: true,
		},
		{
			desc: "invalid resource id",
			metadata: ResourceMetadata{
				Name:          "redis-azure",
				ApplicationID: "/planes/radius/local/resourcegroups/test-rg/providers/applications.core/applications/app1",
				EnvironmentID: "/planes/radius/local/resourcegroups/test-rg/providers/applications.core/environments/env1",
				ResourceID:    "//planes/radius/local/resourceGroups/test-rg/providers/applications.datastores/rediscaches/test-redis-recipe",
				Parameters: map[string]any{
					"redis_cache_name": "redis-test",
				},
			},
			expApp:      "app1",
			expEnv:      "env1",
			expResource: "test-redis-recipe",
			expectedErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			env, app, res, err := GetEnvAppResourceNames(&tt.metadata)
			if !tt.expectedErr {
				require.Equal(t, tt.expApp, app)
				require.Equal(t, tt.expEnv, env)
				require.Equal(t, tt.expResource, res)
			} else {
				require.Error(t, err)
				require.Contains(t, err.Error(), "not a valid resource id")
			}
		})
	}
}
func Test_GetGitURL(t *testing.T) {
	tests := []struct {
		desc         string
		templatePath string
		expectedURL  string
		expectedErr  bool
	}{
		{
			desc:         "success",
			templatePath: "git::https://dev.azure.com/project/module",
			expectedURL:  "https://dev.azure.com/project/module",
			expectedErr:  false,
		},
		{
			desc:         "invalid url",
			templatePath: "git::https://dev.az  ure.com/project/module",
			expectedErr:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			url, err := GetGitURL(tt.templatePath)
			if !tt.expectedErr {
				require.NoError(t, err)
				require.Equal(t, tt.expectedURL, url.String())
			} else {
				require.Error(t, err)
			}
		})
	}

}
func Test_GetSecretStoreID(t *testing.T) {
	tests := []struct {
		desc                string
		envConfig           Configuration
		templatePath        string
		expectedSecretStore string
		expectedErr         bool
	}{
		{
			desc: "success",
			envConfig: Configuration{
				RecipeConfig: v1alpha3.RecipeConfigProperties{
					Terraform: v1alpha3.TerraformConfigProperties{
						Authentication: v1alpha3.AuthConfig{
							Git: v1alpha3.GitAuthConfig{
								PAT: map[string]v1alpha3.SecretConfig{
									"dev.azure.com": v1alpha3.SecretConfig{
										Secret: "secret-store1",
									},
								},
							},
						},
					},
				},
			},
			templatePath:        "git::https://dev.azure.com/project/module",
			expectedSecretStore: "secret-store1",
			expectedErr:         false,
		},
		{
			desc:                "empty config",
			templatePath:        "git::https://dev.azure.com/project/module",
			expectedSecretStore: "",
			expectedErr:         false,
		},
		{
			desc:                "invalid template path",
			templatePath:        "git::https://dev.azu  re.com/project/module",
			expectedSecretStore: "",
			expectedErr:         true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			ss, err := GetSecretStoreID(tt.envConfig, tt.templatePath)
			if !tt.expectedErr {
				require.NoError(t, err)
				require.Equal(t, ss, tt.expectedSecretStore)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func Test_GetURLPrefix(t *testing.T) {
	tests := []struct {
		desc           string
		metadata       ResourceMetadata
		expectedPrefix string
		expectedErr    bool
	}{
		{
			desc: "success",
			metadata: ResourceMetadata{
				Name:          "redis-azure",
				ApplicationID: "/planes/radius/local/resourcegroups/test-rg/providers/applications.core/applications/app1",
				EnvironmentID: "/planes/radius/local/resourcegroups/test-rg/providers/applications.core/environments/env1",
				ResourceID:    "/planes/radius/local/resourceGroups/test-rg/providers/applications.datastores/rediscaches/redis",
				Parameters: map[string]any{
					"redis_cache_name": "redis-test",
				},
			},
			expectedPrefix: "https://env1-app1-redis-",
			expectedErr:    false,
		},
		{
			desc: "success",
			metadata: ResourceMetadata{
				Name:          "redis-azure",
				ApplicationID: "//planes/radius/local/resourcegroups/test-rg/providers/applications.core/applications/app1",
				EnvironmentID: "/planes/radius/local/resourcegroups/test-rg/providers/applications.core/environments/env1",
				ResourceID:    "/planes/radius/local/resourceGroups/test-rg/providers/applications.datastores/rediscaches/redis",
				Parameters: map[string]any{
					"redis_cache_name": "redis-test",
				},
			},
			expectedPrefix: "",
			expectedErr:    true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			ss, err := GetURLPrefix(&tt.metadata)
			if !tt.expectedErr {
				require.NoError(t, err)
				require.Equal(t, ss, tt.expectedPrefix)
			} else {
				require.Error(t, err)
			}
		})
	}
}
