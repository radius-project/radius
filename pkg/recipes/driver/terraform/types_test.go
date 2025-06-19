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

package terraform

import (
	"testing"

	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/recipes"
	"github.com/stretchr/testify/require"
)

func Test_GetPrivateGitRepoSecretStoreID(t *testing.T) {
	tests := []struct {
		desc                string
		envConfig           recipes.Configuration
		templatePath        string
		expectedSecretStore string
		expectedErr         bool
	}{
		{
			desc: "success",
			envConfig: recipes.Configuration{
				RecipeConfig: datamodel.RecipeConfigProperties{
					Terraform: datamodel.TerraformConfigProperties{
						Authentication: datamodel.AuthConfig{
							Git: datamodel.GitAuthConfig{
								PAT: map[string]datamodel.SecretConfig{
									"dev.azure.com": {
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
			desc: "hostname not in PAT config",
			envConfig: recipes.Configuration{
				RecipeConfig: datamodel.RecipeConfigProperties{
					Terraform: datamodel.TerraformConfigProperties{
						Authentication: datamodel.AuthConfig{
							Git: datamodel.GitAuthConfig{
								PAT: map[string]datamodel.SecretConfig{
									"github.com": {
										Secret: "github-secret",
									},
								},
							},
						},
					},
				},
			},
			templatePath:        "git::https://gitlab.com/project/module",
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
			ss, err := GetPrivateGitRepoSecretStoreID(tt.envConfig, tt.templatePath)
			if !tt.expectedErr {
				require.NoError(t, err)
				require.Equal(t, ss, tt.expectedSecretStore)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func Test_GetPrivateGitRepoSecretStoreID_WithSSH(t *testing.T) {
	tests := []struct {
		desc                string
		envConfig           recipes.Configuration
		templatePath        string
		expectedSecretStore string
		expectedErr         bool
	}{
		{
			desc: "SSH authentication takes precedence over PAT",
			envConfig: recipes.Configuration{
				RecipeConfig: datamodel.RecipeConfigProperties{
					Terraform: datamodel.TerraformConfigProperties{
						Authentication: datamodel.AuthConfig{
							Git: datamodel.GitAuthConfig{
								SSH: map[string]datamodel.SSHConfig{
									"github.com": {
										Secret:                "ssh-secret-store",
										StrictHostKeyChecking: false,
									},
								},
								PAT: map[string]datamodel.SecretConfig{
									"github.com": {
										Secret: "pat-secret-store",
									},
								},
							},
						},
					},
				},
			},
			templatePath:        "git::ssh://git@github.com/myorg/myrepo.git",
			expectedSecretStore: "ssh-secret-store",
			expectedErr:         false,
		},
		{
			desc: "SSH authentication for specific hostname",
			envConfig: recipes.Configuration{
				RecipeConfig: datamodel.RecipeConfigProperties{
					Terraform: datamodel.TerraformConfigProperties{
						Authentication: datamodel.AuthConfig{
							Git: datamodel.GitAuthConfig{
								SSH: map[string]datamodel.SSHConfig{
									"gitlab.com": {
										Secret:                "gitlab-ssh-secret",
										StrictHostKeyChecking: true,
									},
								},
							},
						},
					},
				},
			},
			templatePath:        "git::ssh://git@gitlab.com/project/module",
			expectedSecretStore: "gitlab-ssh-secret",
			expectedErr:         false,
		},
		{
			desc: "No SSH authentication configured",
			envConfig: recipes.Configuration{
				RecipeConfig: datamodel.RecipeConfigProperties{
					Terraform: datamodel.TerraformConfigProperties{
						Authentication: datamodel.AuthConfig{
							Git: datamodel.GitAuthConfig{
								PAT: map[string]datamodel.SecretConfig{
									"github.com": {
										Secret: "pat-secret-store",
									},
								},
							},
						},
					},
				},
			},
			templatePath:        "git::ssh://git@github.com/myorg/myrepo.git",
			expectedSecretStore: "pat-secret-store",
			expectedErr:         false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			ss, err := GetPrivateGitRepoSecretStoreID(tt.envConfig, tt.templatePath)
			if !tt.expectedErr {
				require.NoError(t, err)
				require.Equal(t, tt.expectedSecretStore, ss)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func Test_GetGitAuthType(t *testing.T) {
	tests := []struct {
		desc         string
		envConfig    recipes.Configuration
		hostname     string
		expectedType string
	}{
		{
			desc: "SSH authentication",
			envConfig: recipes.Configuration{
				RecipeConfig: datamodel.RecipeConfigProperties{
					Terraform: datamodel.TerraformConfigProperties{
						Authentication: datamodel.AuthConfig{
							Git: datamodel.GitAuthConfig{
								SSH: map[string]datamodel.SSHConfig{
									"github.com": {
										Secret: "ssh-secret",
									},
								},
							},
						},
					},
				},
			},
			hostname:     "github.com",
			expectedType: "ssh",
		},
		{
			desc: "PAT authentication",
			envConfig: recipes.Configuration{
				RecipeConfig: datamodel.RecipeConfigProperties{
					Terraform: datamodel.TerraformConfigProperties{
						Authentication: datamodel.AuthConfig{
							Git: datamodel.GitAuthConfig{
								PAT: map[string]datamodel.SecretConfig{
									"gitlab.com": {
										Secret: "pat-secret",
									},
								},
							},
						},
					},
				},
			},
			hostname:     "gitlab.com",
			expectedType: "pat",
		},
		{
			desc: "SSH takes precedence over PAT",
			envConfig: recipes.Configuration{
				RecipeConfig: datamodel.RecipeConfigProperties{
					Terraform: datamodel.TerraformConfigProperties{
						Authentication: datamodel.AuthConfig{
							Git: datamodel.GitAuthConfig{
								SSH: map[string]datamodel.SSHConfig{
									"github.com": {
										Secret: "ssh-secret",
									},
								},
								PAT: map[string]datamodel.SecretConfig{
									"github.com": {
										Secret: "pat-secret",
									},
								},
							},
						},
					},
				},
			},
			hostname:     "github.com",
			expectedType: "ssh",
		},
		{
			desc:         "No authentication configured",
			envConfig:    recipes.Configuration{},
			hostname:     "github.com",
			expectedType: "none",
		},
		{
			desc: "Hostname with www prefix",
			envConfig: recipes.Configuration{
				RecipeConfig: datamodel.RecipeConfigProperties{
					Terraform: datamodel.TerraformConfigProperties{
						Authentication: datamodel.AuthConfig{
							Git: datamodel.GitAuthConfig{
								SSH: map[string]datamodel.SSHConfig{
									"github.com": {
										Secret: "ssh-secret",
									},
								},
							},
						},
					},
				},
			},
			hostname:     "www.github.com",
			expectedType: "ssh",
		},
	}
	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			authType := GetGitAuthType(tt.envConfig, tt.hostname)
			require.Equal(t, tt.expectedType, authType)
		})
	}
}
