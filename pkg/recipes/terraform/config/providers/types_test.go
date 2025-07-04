package providers

import (
	"context"
	"testing"

	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/recipes"
	"github.com/stretchr/testify/require"
)

func Test_GetRecipeProviderConfigs(t *testing.T) {
	testCases := []struct {
		desc      string
		envConfig *recipes.Configuration
		secrets   map[string]recipes.SecretData
		expected  map[string][]map[string]any
	}{
		{
			desc:      "envConfig not set",
			envConfig: nil,
			expected:  map[string][]map[string]any{},
		},
		{
			desc:      "no providers configured",
			envConfig: &recipes.Configuration{},
			expected:  map[string][]map[string]any{},
		},
		{
			desc: "empty provider config",
			envConfig: &recipes.Configuration{
				RecipeConfig: datamodel.RecipeConfigProperties{
					Terraform: datamodel.TerraformConfigProperties{
						Providers: map[string][]datamodel.ProviderConfigProperties{
							"aws": {},
						},
					},
				},
			},
			expected: map[string][]map[string]any{},
		},
		{
			desc: "Additional Properties set to nil in provider config",
			envConfig: &recipes.Configuration{
				RecipeConfig: datamodel.RecipeConfigProperties{
					Terraform: datamodel.TerraformConfigProperties{
						Providers: map[string][]datamodel.ProviderConfigProperties{
							"aws": {
								{
									AdditionalProperties: nil,
								},
							},
						},
					},
				},
			},
			expected: map[string][]map[string]any{"aws": {}},
		},
		{
			desc: "provider with config",
			envConfig: &recipes.Configuration{
				RecipeConfig: datamodel.RecipeConfigProperties{
					Terraform: datamodel.TerraformConfigProperties{
						Providers: map[string][]datamodel.ProviderConfigProperties{
							"azurerm": {
								{
									AdditionalProperties: map[string]any{
										"subscriptionid": 1234,
										"tenant_id":      "745fg88bf-86f1-41af-43ut",
									},
								},
								{
									AdditionalProperties: map[string]any{
										"alias":          "az-paymentservice",
										"subscriptionid": 45678,
										"tenant_id":      "gfhf45345-5d73-gh34-wh84",
									},
								},
							},
						},
					},
				},
			},
			expected: map[string][]map[string]any{
				"azurerm": {
					{
						"subscriptionid": 1234,
						"tenant_id":      "745fg88bf-86f1-41af-43ut",
					},
					{
						"alias":          "az-paymentservice",
						"subscriptionid": 45678,
						"tenant_id":      "gfhf45345-5d73-gh34-wh84",
					},
				},
			},
		},
		{
			desc: "provider with secrets",
			envConfig: &recipes.Configuration{
				RecipeConfig: datamodel.RecipeConfigProperties{
					Terraform: datamodel.TerraformConfigProperties{
						Providers: map[string][]datamodel.ProviderConfigProperties{
							"azurerm": {
								{
									AdditionalProperties: map[string]any{
										"subscriptionid": 1234,
										"tenant_id":      "745fg88bf-86f1-41af-43ut",
									},
									Secrets: map[string]datamodel.SecretReference{
										"secret1": {
											Source: "secretstoreid1",
											Key:    "secretkey1",
										},
									},
								},
								{
									AdditionalProperties: map[string]any{
										"alias":          "az-paymentservice",
										"subscriptionid": 45678,
										"tenant_id":      "gfhf45345-5d73-gh34-wh84",
									},
									Secrets: map[string]datamodel.SecretReference{
										"secret2": {
											Source: "secretstoreid2",
											Key:    "secretkey2",
										},
									},
								},
							},
						},
					},
				},
			},
			secrets: map[string]recipes.SecretData{
				"secretstoreid1": {
					Type: "generic",
					Data: map[string]string{"secretkey1": "secretvalue1"},
				},
				"secretstoreid2": {
					Type: "generic",
					Data: map[string]string{"secretkey2": "secretvalue2"},
				},
			},
			expected: map[string][]map[string]any{
				"azurerm": {
					{
						"subscriptionid": 1234,
						"tenant_id":      "745fg88bf-86f1-41af-43ut",
						"secret1":        "secretvalue1",
					},
					{
						"alias":          "az-paymentservice",
						"subscriptionid": 45678,
						"tenant_id":      "gfhf45345-5d73-gh34-wh84",
						"secret2":        "secretvalue2",
					},
				},
			},
		},
		{
			desc: "provider with Secrets and no Additional Properties",
			envConfig: &recipes.Configuration{
				RecipeConfig: datamodel.RecipeConfigProperties{
					Terraform: datamodel.TerraformConfigProperties{
						Providers: map[string][]datamodel.ProviderConfigProperties{
							"azurerm": {
								{
									Secrets: map[string]datamodel.SecretReference{
										"secret1": {
											Source: "secretstoreid1",
											Key:    "secretkey1",
										},
									},
								},
							},
						},
					},
				},
			},
			secrets: map[string]recipes.SecretData{
				"secretstoreid1": {
					Type: "generic",
					Data: map[string]string{"secretkey1": "secretvalue1"},
				},
				"secretstoreid2": {
					Type: "generic",
					Data: map[string]string{"secretkey2": "secretvalue2"},
				},
			},
			expected: map[string][]map[string]any{
				"azurerm": {
					{
						"secret1": "secretvalue1",
					},
				},
			},
		},
		{
			desc: "provider and env with secrets",
			envConfig: &recipes.Configuration{
				RecipeConfig: datamodel.RecipeConfigProperties{
					Terraform: datamodel.TerraformConfigProperties{
						Providers: map[string][]datamodel.ProviderConfigProperties{
							"azurerm": {
								{
									AdditionalProperties: map[string]any{
										"subscriptionid": 1234,
										"tenant_id":      "745fg88bf-86f1-41af-43ut",
									},
									Secrets: map[string]datamodel.SecretReference{
										"secret1": {
											Source: "secretstoreid1",
											Key:    "secretkey1",
										},
									},
								},
							},
						},
					},
					EnvSecrets: map[string]datamodel.SecretReference{
						"secret-env": {
							Source: "secretstoreid-env",
							Key:    "secretkey-env",
						},
						"secret-usedid-env": {
							Source: "secretstoreid1",
							Key:    "secret-usedid-envkey",
						},
					},
				},
			},
			secrets: map[string]recipes.SecretData{
				"secretstoreid1": {
					Type: "generic",
					Data: map[string]string{"secretkey1": "secretvalue1",
						"secret-usedid-env": "secretvalue-usedid-env"},
				},
				"secretstore-env": {
					Type: "generic",
					Data: map[string]string{"secretkey-env": "secretvalue-env"},
				},
			},
			expected: map[string][]map[string]any{
				"azurerm": {
					{
						"subscriptionid": 1234,
						"tenant_id":      "745fg88bf-86f1-41af-43ut",
						"secret1":        "secretvalue1",
					},
				},
			},
		},
		{
			desc: "provider additional prop and secrets with same secret id",
			envConfig: &recipes.Configuration{
				RecipeConfig: datamodel.RecipeConfigProperties{
					Terraform: datamodel.TerraformConfigProperties{
						Providers: map[string][]datamodel.ProviderConfigProperties{
							"azurerm": {
								{
									AdditionalProperties: map[string]any{
										"subscriptionid": 1234,
										"tenant_id":      "745fg88bf-86f1-41af-43ut",
										"client_id":      "abc123",
									},
									Secrets: map[string]datamodel.SecretReference{
										"client_id": {
											Source: "secretstoreid1",
											Key:    "secretkey1",
										},
									},
								},
							},
						},
					},
				},
			},
			secrets: map[string]recipes.SecretData{
				"secretstoreid1": {
					Type: "generic",
					Data: map[string]string{"secretkey1": "secretvalue-clientid"},
				},
			},
			expected: map[string][]map[string]any{
				"azurerm": {
					{
						"subscriptionid": 1234,
						"tenant_id":      "745fg88bf-86f1-41af-43ut",
						"client_id":      "secretvalue-clientid",
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			result, err := GetRecipeProviderConfigs(context.Background(), tc.envConfig, tc.secrets)
			require.NoError(t, err)
			require.Equal(t, tc.expected, result)
		})
	}
}

func Test_extractSecretsFromRecipeConfig(t *testing.T) {
	tests := []struct {
		name                 string
		currentConfig        map[string]any
		recipeConfigSecrets  map[string]datamodel.SecretReference
		secrets              map[string]recipes.SecretData
		expectedConfig       map[string]any
		expectError          bool
		expectedErrorMessage string
	}{
		{
			name: "success",
			recipeConfigSecrets: map[string]datamodel.SecretReference{
				"password": {Source: "dbSecrets", Key: "dbPass"},
			},
			secrets: map[string]recipes.SecretData{
				"dbSecrets": {
					Type: "generic",
					Data: map[string]string{"dbPass": "secretPassword"},
				},
			},
			expectedConfig: map[string]any{
				"password": "secretPassword",
			},
			expectError: false,
		},
		{
			name: "missing secret source",
			recipeConfigSecrets: map[string]datamodel.SecretReference{
				"password": {Source: "missingSource", Key: "dbPass"},
			},
			secrets: map[string]recipes.SecretData{
				"dbSecrets": {
					Type: "generic",
					Data: map[string]string{"dbPass": "secretPassword"},
				},
			},
			expectError:          true,
			expectedErrorMessage: "missing secret store id: missingSource",
		},
		{
			name: "missing secret key",
			recipeConfigSecrets: map[string]datamodel.SecretReference{
				"password": {Source: "dbSecrets", Key: "missingKey"},
			},
			secrets: map[string]recipes.SecretData{
				"dbSecrets": {
					Type: "generic",
					Data: map[string]string{"dbPass": "secretPassword"},
				},
			},
			expectError:          true,
			expectedErrorMessage: "missing secret key in secret store id: dbSecrets",
		},
		{
			name: "missing secrets",
			recipeConfigSecrets: map[string]datamodel.SecretReference{
				"password": {Source: "dbSecrets", Key: "missingKey"},
			},
			secrets:              nil,
			expectError:          true,
			expectedErrorMessage: "missing secret store id: dbSecrets",
		},
		{
			name:                "missing recipeConfigSecrets",
			recipeConfigSecrets: nil,
			secrets: map[string]recipes.SecretData{
				"dbSecrets": {
					Type: "generic",
					Data: map[string]string{"dbPass": "secretPassword"},
				},
			},
			expectedConfig: map[string]any{},
			expectError:    false,
		},
		{
			name: "missing secrets data",
			recipeConfigSecrets: map[string]datamodel.SecretReference{
				"password": {Source: "dbSecrets", Key: "missingKey"},
			},
			secrets: map[string]recipes.SecretData{
				"dbSecrets": {
					Type: "generic",
				},
			},
			expectedConfig:       map[string]any{},
			expectError:          true,
			expectedErrorMessage: "missing secret key in secret store id: dbSecrets",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			secretsConfig, err := extractSecretsFromRecipeConfig(context.Background(), tt.recipeConfigSecrets, tt.secrets)
			if tt.expectError {
				require.EqualError(t, err, tt.expectedErrorMessage, err.Error())
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.expectedConfig, secretsConfig)
			}
		})
	}
}
