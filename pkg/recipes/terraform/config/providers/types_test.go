package providers

import (
	"context"
	"testing"

	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/recipes"
	"github.com/stretchr/testify/require"
)

func TestGetRecipeProviderConfigs(t *testing.T) {
	testCases := []struct {
		desc      string
		envConfig *recipes.Configuration
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
			expected: map[string][]map[string]any{"aws": []map[string]any{}},
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
				"azurerm": []map[string]any{
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
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			result := GetRecipeProviderConfigs(context.Background(), tc.envConfig)
			require.Equal(t, tc.expected, result)
		})
	}
}
