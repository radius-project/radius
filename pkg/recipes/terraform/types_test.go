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

func TestGetTerraformRegistrySecretIDs(t *testing.T) {
	tests := []struct {
		name       string
		envConfig  recipes.Configuration
		wantLength int
		wantKeys   map[string][]string
	}{
		{
			name: "no terraform config",
			envConfig: recipes.Configuration{
				RecipeConfig: datamodel.RecipeConfigProperties{},
			},
			wantLength: 0,
			wantKeys:   map[string][]string{},
		},
		{
			name: "registry token authentication only",
			envConfig: recipes.Configuration{
				RecipeConfig: datamodel.RecipeConfigProperties{
					Terraform: datamodel.TerraformConfigProperties{
						Registry: &datamodel.TerraformRegistryConfig{
							Mirror: "https://registry.example.com",
							Authentication: datamodel.RegistryAuthConfig{
								Token: &datamodel.TokenConfig{
									Secret: "/secret/store/registry",
								},
							},
						},
					},
				},
			},
			wantLength: 1,
			wantKeys: map[string][]string{
				"/secret/store/registry": {"token"},
			},
		},
		{
			name: "version token authentication only",
			envConfig: recipes.Configuration{
				RecipeConfig: datamodel.RecipeConfigProperties{
					Terraform: datamodel.TerraformConfigProperties{
						Version: &datamodel.TerraformVersionConfig{
							Version:            "1.7.0",
							ReleasesAPIBaseURL: "https://terraform-mirror.example.com",
							Authentication: &datamodel.RegistryAuthConfig{
								Token: &datamodel.TokenConfig{
									Secret: "/secret/store/version",
								},
							},
						},
					},
				},
			},
			wantLength: 1,
			wantKeys: map[string][]string{
				"/secret/store/version": {"token"},
			},
		},
		{
			name: "both registry and version token authentication",
			envConfig: recipes.Configuration{
				RecipeConfig: datamodel.RecipeConfigProperties{
					Terraform: datamodel.TerraformConfigProperties{
						Registry: &datamodel.TerraformRegistryConfig{
							Mirror: "https://registry.example.com",
							Authentication: datamodel.RegistryAuthConfig{
								Token: &datamodel.TokenConfig{
									Secret: "/secret/store/registry",
								},
							},
						},
						Version: &datamodel.TerraformVersionConfig{
							Version:            "1.7.0",
							ReleasesAPIBaseURL: "https://terraform-mirror.example.com",
							Authentication: &datamodel.RegistryAuthConfig{
								Token: &datamodel.TokenConfig{
									Secret: "/secret/store/version",
								},
							},
						},
					},
				},
			},
			wantLength: 2,
			wantKeys: map[string][]string{
				"/secret/store/registry": {"token"},
				"/secret/store/version":  {"token"},
			},
		},
		{
			name: "version with TLS CA certificate",
			envConfig: recipes.Configuration{
				RecipeConfig: datamodel.RecipeConfigProperties{
					Terraform: datamodel.TerraformConfigProperties{
						Version: &datamodel.TerraformVersionConfig{
							Version:            "1.7.0",
							ReleasesAPIBaseURL: "https://terraform-mirror.example.com",
							TLS: &datamodel.TerraformTLSConfig{
								CACertificate: &datamodel.SecretReference{
									Source: "/secret/store/tls",
									Key:    "ca-cert",
								},
							},
						},
					},
				},
			},
			wantLength: 1,
			wantKeys: map[string][]string{
				"/secret/store/tls": {"ca-cert"},
			},
		},
		{
			name: "all authentication types",
			envConfig: recipes.Configuration{
				RecipeConfig: datamodel.RecipeConfigProperties{
					Terraform: datamodel.TerraformConfigProperties{
						Registry: &datamodel.TerraformRegistryConfig{
							Mirror: "https://registry.example.com",
							Authentication: datamodel.RegistryAuthConfig{
								Token: &datamodel.TokenConfig{
									Secret: "/secret/store/registry",
								},
							},
						},
						Version: &datamodel.TerraformVersionConfig{
							Version:            "1.7.0",
							ReleasesAPIBaseURL: "https://terraform-mirror.example.com",
							Authentication: &datamodel.RegistryAuthConfig{
								Token: &datamodel.TokenConfig{
									Secret: "/secret/store/version",
								},
							},
							TLS: &datamodel.TerraformTLSConfig{
								CACertificate: &datamodel.SecretReference{
									Source: "/secret/store/tls",
									Key:    "ca-cert",
								},
							},
						},
					},
				},
			},
			wantLength: 3,
			wantKeys: map[string][]string{
				"/secret/store/registry": {"token"},
				"/secret/store/version":  {"token"},
				"/secret/store/tls":      {"ca-cert"},
			},
		},
		{
			name: "same secret store for multiple purposes",
			envConfig: recipes.Configuration{
				RecipeConfig: datamodel.RecipeConfigProperties{
					Terraform: datamodel.TerraformConfigProperties{
						Registry: &datamodel.TerraformRegistryConfig{
							Mirror: "https://registry.example.com",
							Authentication: datamodel.RegistryAuthConfig{
								Token: &datamodel.TokenConfig{
									Secret: "/secret/store/shared",
								},
							},
						},
						Version: &datamodel.TerraformVersionConfig{
							Version:            "1.7.0",
							ReleasesAPIBaseURL: "https://terraform-mirror.example.com",
							Authentication: &datamodel.RegistryAuthConfig{
								Token: &datamodel.TokenConfig{
									Secret: "/secret/store/shared",
								},
							},
						},
					},
				},
			},
			wantLength: 1,
			wantKeys: map[string][]string{
				"/secret/store/shared": {"token"},
			},
		},
		{
			name: "version with TLS client certificate",
			envConfig: recipes.Configuration{
				RecipeConfig: datamodel.RecipeConfigProperties{
					Terraform: datamodel.TerraformConfigProperties{
						Version: &datamodel.TerraformVersionConfig{
							Version:            "1.7.0",
							ReleasesAPIBaseURL: "https://terraform-mirror.example.com",
							TLS: &datamodel.TerraformTLSConfig{
								ClientCertificate: &datamodel.ClientCertConfig{
									Secret: "/secret/store/tls-client",
								},
							},
						},
					},
				},
			},
			wantLength: 1,
			wantKeys: map[string][]string{
				"/secret/store/tls-client": {"certificate", "key"},
			},
		},
		{
			name: "token auth with additional hosts",
			envConfig: recipes.Configuration{
				RecipeConfig: datamodel.RecipeConfigProperties{
					Terraform: datamodel.TerraformConfigProperties{
						Registry: &datamodel.TerraformRegistryConfig{
							Mirror: "https://registry.example.com",
							Authentication: datamodel.RegistryAuthConfig{
								Token: &datamodel.TokenConfig{
									Secret: "/secret/store/token",
								},
								AdditionalHosts: []string{"gitlab.com", "packages.gitlab.com"},
							},
						},
					},
				},
			},
			wantLength: 1,
			wantKeys: map[string][]string{
				"/secret/store/token": {"token"},
			},
		},
		{
			name: "empty additional hosts",
			envConfig: recipes.Configuration{
				RecipeConfig: datamodel.RecipeConfigProperties{
					Terraform: datamodel.TerraformConfigProperties{
						Registry: &datamodel.TerraformRegistryConfig{
							Mirror: "https://registry.example.com",
							Authentication: datamodel.RegistryAuthConfig{
								AdditionalHosts: []string{},
							},
						},
					},
				},
			},
			wantLength: 0,
			wantKeys:   map[string][]string{},
		},
		{
			name: "token auth with empty secret",
			envConfig: recipes.Configuration{
				RecipeConfig: datamodel.RecipeConfigProperties{
					Terraform: datamodel.TerraformConfigProperties{
						Registry: &datamodel.TerraformRegistryConfig{
							Mirror: "https://registry.example.com",
							Authentication: datamodel.RegistryAuthConfig{
								Token: &datamodel.TokenConfig{
									Secret: "",
								},
							},
						},
					},
				},
			},
			wantLength: 0,
			wantKeys:   map[string][]string{},
		},
		{
			name: "nil token config",
			envConfig: recipes.Configuration{
				RecipeConfig: datamodel.RecipeConfigProperties{
					Terraform: datamodel.TerraformConfigProperties{
						Registry: &datamodel.TerraformRegistryConfig{
							Mirror: "https://registry.example.com",
							Authentication: datamodel.RegistryAuthConfig{
								Token: nil,
							},
						},
					},
				},
			},
			wantLength: 0,
			wantKeys:   map[string][]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetTerraformRegistrySecretIDs(tt.envConfig)
			
			// Check total number of secret stores
			require.Equal(t, tt.wantLength, len(got))
			
			// Check each expected secret store and its keys
			for secretStore, expectedKeys := range tt.wantKeys {
				gotKeys, ok := got[secretStore]
				require.True(t, ok, "expected secret store %s not found", secretStore)
				
				// Sort both slices for comparison since order doesn't matter
				require.ElementsMatch(t, expectedKeys, gotKeys, "keys don't match for secret store %s", secretStore)
			}
		})
	}
}