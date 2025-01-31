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

package v20231001preview

import (
	"encoding/json"
	"fmt"
	"testing"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	dapr_ctrl "github.com/radius-project/radius/pkg/daprrp/frontend/controller"
	ds_ctrl "github.com/radius-project/radius/pkg/datastoresrp/frontend/controller"
	"github.com/radius-project/radius/pkg/recipes"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/test/testutil"
	"github.com/radius-project/radius/test/testutil/resourcetypeutil"
	"github.com/stretchr/testify/require"
)

func TestConvertVersionedToDataModel(t *testing.T) {
	conversionTests := []struct {
		filename string
		expected *datamodel.Environment
		err      error
	}{
		{
			filename: "environmentresource-with-workload-identity.json",
			expected: &datamodel.Environment{
				BaseResource: v1.BaseResource{
					TrackedResource: v1.TrackedResource{
						ID:   "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env0",
						Name: "env0",
						Type: "Applications.Core/environments",
						Tags: map[string]string{},
					},
					InternalMetadata: v1.InternalMetadata{
						CreatedAPIVersion:      "2023-10-01-preview",
						UpdatedAPIVersion:      "2023-10-01-preview",
						AsyncProvisioningState: v1.ProvisioningStateAccepted,
					},
				},
				Properties: datamodel.EnvironmentProperties{
					Compute: rpv1.EnvironmentCompute{
						Kind: "kubernetes",
						KubernetesCompute: rpv1.KubernetesComputeProperties{
							ResourceID: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup/providers/Microsoft.ContainerService/managedClusters/radiusTestCluster",
							Namespace:  "default",
						},
						Identity: &rpv1.IdentitySettings{
							Kind:       rpv1.AzureIdentityWorkload,
							Resource:   "/subscriptions/testSub/resourcegroups/testGroup/providers/Microsoft.ManagedIdentity/userAssignedIdentities/radius-mi-app",
							OIDCIssuer: "https://oidcurl/guid",
						},
					},
					Providers: datamodel.Providers{
						Azure: datamodel.ProvidersAzure{
							Scope: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup",
						},
					},
					RecipeConfig: datamodel.RecipeConfigProperties{
						Terraform: datamodel.TerraformConfigProperties{
							Authentication: datamodel.AuthConfig{
								Git: datamodel.GitAuthConfig{},
							},
							Providers: map[string][]datamodel.ProviderConfigProperties{},
						},
						Env: datamodel.EnvironmentVariables{
							AdditionalProperties: map[string]string{},
						},
					},
					Recipes: map[string]map[string]datamodel.EnvironmentRecipeProperties{
						ds_ctrl.MongoDatabasesResourceType: {
							"cosmos-recipe": datamodel.EnvironmentRecipeProperties{
								TemplateKind: recipes.TemplateKindBicep,
								TemplatePath: "br:ghcr.io/sampleregistry/radius/recipes/cosmosdb",
							},
						},
					},
				},
			},
			err: nil,
		},
		{
			filename: "environmentresource.json",
			expected: &datamodel.Environment{
				BaseResource: v1.BaseResource{
					TrackedResource: v1.TrackedResource{
						ID:   "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env0",
						Name: "env0",
						Type: "Applications.Core/environments",
						Tags: map[string]string{},
					},
					InternalMetadata: v1.InternalMetadata{
						CreatedAPIVersion:      "2023-10-01-preview",
						UpdatedAPIVersion:      "2023-10-01-preview",
						AsyncProvisioningState: v1.ProvisioningStateAccepted,
					},
				},
				Properties: datamodel.EnvironmentProperties{
					Compute: rpv1.EnvironmentCompute{
						Kind: "kubernetes",
						KubernetesCompute: rpv1.KubernetesComputeProperties{
							ResourceID: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup/providers/Microsoft.ContainerService/managedClusters/radiusTestCluster",
							Namespace:  "default",
						},
					},
					Providers: datamodel.Providers{
						Azure: datamodel.ProvidersAzure{
							Scope: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup",
						},
						AWS: datamodel.ProvidersAWS{
							Scope: "/planes/aws/aws/accounts/140313373712/regions/us-west-2",
						},
					},
					RecipeConfig: datamodel.RecipeConfigProperties{
						Terraform: datamodel.TerraformConfigProperties{
							Authentication: datamodel.AuthConfig{
								Git: datamodel.GitAuthConfig{
									PAT: map[string]datamodel.SecretConfig{
										"dev.azure.com": {
											Secret: "/planes/radius/local/resourcegroups/default/providers/Applications.Core/secretStores/github",
										},
									},
								},
							},
							Providers: map[string][]datamodel.ProviderConfigProperties{
								"azurerm": {
									{
										Secrets: map[string]datamodel.SecretReference{
											"secret1": {
												Source: "/planes/radius/local/resourcegroups/default/providers/Applications.Core/secretStores/secretstore1",
												Key:    "key1",
											},
											"secret2": {
												Source: "/planes/radius/local/resourcegroups/default/providers/Applications.Core/secretStores/secretstore2",
												Key:    "key2",
											},
										},
										AdditionalProperties: map[string]any{
											"subscriptionId": "00000000-0000-0000-0000-000000000000",
										},
									},
								},
							},
						},
						Bicep: datamodel.BicepConfigProperties{
							Authentication: map[string]datamodel.RegistrySecretConfig{
								"test.azurecr.io": {
									Secret: "/planes/radius/local/resourcegroups/default/providers/Applications.Core/secretStores/acr-secret",
								},
							},
						},
						Env: datamodel.EnvironmentVariables{
							AdditionalProperties: map[string]string{
								"myEnvVar": "myEnvValue",
							},
						},
						EnvSecrets: map[string]datamodel.SecretReference{
							"myEnvSecretVar": {
								Source: "/planes/radius/local/resourcegroups/default/providers/Applications.Core/secretStores/envSecretStore1",
								Key:    "envKey1",
							},
						},
					},
					Recipes: map[string]map[string]datamodel.EnvironmentRecipeProperties{
						ds_ctrl.MongoDatabasesResourceType: {
							"cosmos-recipe": datamodel.EnvironmentRecipeProperties{
								TemplateKind: recipes.TemplateKindBicep,
								TemplatePath: "br:ghcr.io/sampleregistry/radius/recipes/mongodatabases",
								Parameters: map[string]any{
									"throughput": float64(400),
								},
							},
							"terraform-recipe": datamodel.EnvironmentRecipeProperties{
								TemplateKind:    recipes.TemplateKindTerraform,
								TemplatePath:    "Azure/cosmosdb/azurerm",
								TemplateVersion: "1.1.0",
							},
							"terraform-without-version": datamodel.EnvironmentRecipeProperties{
								TemplateKind: recipes.TemplateKindTerraform,
								TemplatePath: "http://example.com/myrecipe.zip",
							},
						},
						ds_ctrl.RedisCachesResourceType: {
							"redis-recipe": datamodel.EnvironmentRecipeProperties{
								TemplateKind: recipes.TemplateKindBicep,
								TemplatePath: "br:ghcr.io/sampleregistry/radius/recipes/rediscaches",
								PlainHTTP:    true,
							},
						},
						dapr_ctrl.DaprStateStoresResourceType: {
							"statestore-recipe": datamodel.EnvironmentRecipeProperties{
								TemplateKind:    recipes.TemplateKindTerraform,
								TemplatePath:    "Azure/storage/azurerm",
								TemplateVersion: "1.1.0",
							},
						},
					},
					Extensions: getTestKubernetesMetadataExtensions(),
				},
			},
			err: nil,
		},
		{
			filename: "environmentresourceemptyext.json",
			expected: &datamodel.Environment{
				BaseResource: v1.BaseResource{
					TrackedResource: v1.TrackedResource{
						ID:   "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env0",
						Name: "env0",
						Type: "Applications.Core/environments",
						Tags: map[string]string{},
					},
					InternalMetadata: v1.InternalMetadata{
						CreatedAPIVersion:      "2023-10-01-preview",
						UpdatedAPIVersion:      "2023-10-01-preview",
						AsyncProvisioningState: v1.ProvisioningStateAccepted,
					},
				},
				Properties: datamodel.EnvironmentProperties{
					Compute: rpv1.EnvironmentCompute{
						Kind: "kubernetes",
						KubernetesCompute: rpv1.KubernetesComputeProperties{
							ResourceID: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup/providers/Microsoft.ContainerService/managedClusters/radiusTestCluster",
							Namespace:  "default",
						},
					},
					Providers: datamodel.Providers{
						Azure: datamodel.ProvidersAzure{
							Scope: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup",
						},
					},
					Recipes: map[string]map[string]datamodel.EnvironmentRecipeProperties{
						ds_ctrl.MongoDatabasesResourceType: {
							"cosmos-recipe": datamodel.EnvironmentRecipeProperties{
								TemplateKind: recipes.TemplateKindBicep,
								TemplatePath: "br:ghcr.io/sampleregistry/radius/recipes/cosmosdb",
							},
						},
					},
					Extensions: getTestKubernetesEmptyMetadataExtensions(),
				},
			},
			err: nil,
		},
		{
			filename: "environmentresourceemptyext2.json",
			expected: &datamodel.Environment{
				BaseResource: v1.BaseResource{
					TrackedResource: v1.TrackedResource{
						ID:   "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env0",
						Name: "env0",
						Type: "Applications.Core/environments",
						Tags: map[string]string{},
					},
					InternalMetadata: v1.InternalMetadata{
						CreatedAPIVersion:      "2023-10-01-preview",
						UpdatedAPIVersion:      "2023-10-01-preview",
						AsyncProvisioningState: v1.ProvisioningStateAccepted,
					},
				},
				Properties: datamodel.EnvironmentProperties{
					Compute: rpv1.EnvironmentCompute{
						Kind: "kubernetes",
						KubernetesCompute: rpv1.KubernetesComputeProperties{
							ResourceID: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup/providers/Microsoft.ContainerService/managedClusters/radiusTestCluster",
							Namespace:  "default",
						},
					},
					Providers: datamodel.Providers{
						Azure: datamodel.ProvidersAzure{
							Scope: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup",
						},
					},
					Recipes: map[string]map[string]datamodel.EnvironmentRecipeProperties{
						ds_ctrl.MongoDatabasesResourceType: {
							"cosmos-recipe": datamodel.EnvironmentRecipeProperties{
								TemplateKind: recipes.TemplateKindBicep,
								TemplatePath: "br:ghcr.io/sampleregistry/radius/recipes/cosmosdb",
							},
						},
					},
					Extensions: getTestKubernetesEmptyMetadataExtensions(),
				},
			},
			err: nil,
		},
		{
			filename: "environmentresource-with-simulated-enabled.json",
			expected: &datamodel.Environment{
				BaseResource: v1.BaseResource{
					TrackedResource: v1.TrackedResource{
						ID:   "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env0",
						Name: "env0",
						Type: "Applications.Core/environments",
						Tags: map[string]string{},
					},
					InternalMetadata: v1.InternalMetadata{
						CreatedAPIVersion:      "2023-10-01-preview",
						UpdatedAPIVersion:      "2023-10-01-preview",
						AsyncProvisioningState: v1.ProvisioningStateAccepted,
					},
				},
				Properties: datamodel.EnvironmentProperties{
					Compute: rpv1.EnvironmentCompute{
						Kind: "kubernetes",
						KubernetesCompute: rpv1.KubernetesComputeProperties{
							ResourceID: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup/providers/Microsoft.ContainerService/managedClusters/radiusTestCluster",
							Namespace:  "default",
						},
					},
					Simulated: true,
				},
			},
			err: nil,
		},
		{
			filename: "environmentresource-invalid-missing-namespace.json",
			err:      &v1.ErrModelConversion{PropertyName: "$.properties.compute.namespace", ValidValue: "63 characters or less"},
		},
		{
			filename: "environmentresource-invalid-namespace.json",
			err:      &v1.ErrModelConversion{PropertyName: "$.properties.compute.namespace", ValidValue: "63 characters or less"},
		},
		{
			filename: "environmentresource-invalid-resourcetype.json",
			err:      &v1.ErrClientRP{Code: v1.CodeInvalid, Message: "invalid resource type: \"pubsub\". Must be in the format \"ResourceProvider.Namespace/resourceType\""},
		},
		{
			filename: "environmentresource-invalid-templatekind.json",
			err:      &v1.ErrClientRP{Code: v1.CodeInvalid, Message: "invalid template kind. Allowed formats: \"bicep\", \"terraform\""},
		},
		{
			filename: "environmentresource-missing-templatekind.json",
			err:      &v1.ErrClientRP{Code: v1.CodeInvalid, Message: "invalid template kind. Allowed formats: \"bicep\", \"terraform\""},
		},
		{
			filename: "environmentresource-terraformrecipe-localpath.json",
			err:      &v1.ErrClientRP{Code: v1.CodeInvalid, Message: fmt.Sprintf(invalidLocalModulePathFmt, "../not-allowed/")},
		},
	}

	for _, tt := range conversionTests {
		t.Run(tt.filename, func(t *testing.T) {
			rawPayload := testutil.ReadFixture(tt.filename)
			r := &EnvironmentResource{}
			err := json.Unmarshal(rawPayload, r)
			require.NoError(t, err)

			// act
			dm, err := r.ConvertTo()

			if tt.err != nil {
				require.ErrorIs(t, err, tt.err)
				require.Equal(t, tt.err.Error(), err.Error())
			} else {
				require.NoError(t, err)
				ct := dm.(*datamodel.Environment)
				require.Equal(t, tt.expected, ct)
			}
		})
	}
}

func TestConvertDataModelToVersioned(t *testing.T) {
	baseSecretStorePath := "/planes/radius/local/resourcegroups/default/providers/Applications.Core/secretStores/"
	conversionTests := []struct {
		filename string
		err      error
		emptyExt bool
	}{
		{
			filename: "environmentresourcedatamodel.json",
			err:      nil,
			emptyExt: false,
		},
		{
			filename: "environmentresourcedatamodelemptyext.json",
			err:      nil,
			emptyExt: true,
		},
	}

	for _, tt := range conversionTests {
		t.Run(tt.filename, func(t *testing.T) {
			rawPayload := testutil.ReadFixture(tt.filename)
			r := &datamodel.Environment{}
			err := json.Unmarshal(rawPayload, r)
			require.NoError(t, err)

			// act
			versioned := &EnvironmentResource{}
			err = versioned.ConvertFrom(r)

			if tt.err != nil {
				require.ErrorIs(t, err, tt.err)
			} else {
				// assert
				require.NoError(t, err)
				require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env0", string(*versioned.ID))
				require.Equal(t, "env0", string(*versioned.Name))
				require.Equal(t, "Applications.Core/environments", string(*versioned.Type))
				require.Equal(t, "kubernetes", string(*versioned.Properties.Compute.GetEnvironmentCompute().Kind))
				require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup/providers/Microsoft.ContainerService/managedClusters/radiusTestCluster", string(*versioned.Properties.Compute.GetEnvironmentCompute().ResourceID))
				require.Equal(t, 1, len(versioned.Properties.Recipes))
				require.Equal(t, "br:ghcr.io/sampleregistry/radius/recipes/cosmosdb", string(*versioned.Properties.Recipes[ds_ctrl.MongoDatabasesResourceType]["cosmos-recipe"].GetRecipeProperties().TemplatePath))
				require.Equal(t, recipes.TemplateKindBicep, string(*versioned.Properties.Recipes[ds_ctrl.MongoDatabasesResourceType]["cosmos-recipe"].GetRecipeProperties().TemplateKind))
				require.Equal(t, map[string]any{"throughput": float64(400)}, versioned.Properties.Recipes[ds_ctrl.MongoDatabasesResourceType]["cosmos-recipe"].GetRecipeProperties().Parameters)
				require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup", string(*versioned.Properties.Providers.Azure.Scope))
				require.Equal(t, "/planes/aws/aws/accounts/140313373712/regions/us-west-2", string(*versioned.Properties.Providers.Aws.Scope))
				require.Equal(t, "kubernetesMetadata", *versioned.Properties.Extensions[0].GetExtension().Kind)
				require.Equal(t, 1, len(versioned.Properties.Extensions))
				recipeDetails := versioned.Properties.Recipes[ds_ctrl.MongoDatabasesResourceType]["terraform-recipe"]

				if tt.filename == "environmentresourcedatamodel.json" {
					require.Equal(t, "Azure/cosmosdb/azurerm", string(*versioned.Properties.Recipes[ds_ctrl.MongoDatabasesResourceType]["terraform-recipe"].GetRecipeProperties().TemplatePath))
					require.Equal(t, recipes.TemplateKindTerraform, string(*versioned.Properties.Recipes[ds_ctrl.MongoDatabasesResourceType]["terraform-recipe"].GetRecipeProperties().TemplateKind))
					require.Equal(t, baseSecretStorePath+"github", string(*versioned.Properties.RecipeConfig.Terraform.Authentication.Git.Pat["dev.azure.com"].Secret))
					require.Equal(t, baseSecretStorePath+"acr-secret", string(*versioned.Properties.RecipeConfig.Bicep.Authentication["test.azurecr.io"].Secret))
					switch c := recipeDetails.(type) {
					case *TerraformRecipeProperties:
						require.Equal(t, "1.1.0", string(*c.TemplateVersion))
					case *BicepRecipeProperties:
						require.Equal(t, true, bool(*c.PlainHTTP))
					}
					require.Equal(t, 1, len(versioned.Properties.RecipeConfig.Terraform.Providers))
					require.Equal(t, 1, len(versioned.Properties.RecipeConfig.Terraform.Providers["azurerm"]))
					subscriptionId := versioned.Properties.RecipeConfig.Terraform.Providers["azurerm"][0].AdditionalProperties["subscriptionId"]
					require.Equal(t, "00000000-0000-0000-0000-000000000000", subscriptionId)

					providerSecretIDs := versioned.Properties.RecipeConfig.Terraform.Providers["azurerm"][0].Secrets
					require.Equal(t, 2, len(providerSecretIDs))
					require.Equal(t, providerSecretIDs["secret1"], to.Ptr(SecretReference{Source: to.Ptr(baseSecretStorePath + "secretstore1"), Key: to.Ptr("key1")}))
					require.Equal(t, providerSecretIDs["secret2"], to.Ptr(SecretReference{Source: to.Ptr(baseSecretStorePath + "secretstore2"), Key: to.Ptr("key2")}))

					require.Equal(t, 1, len(versioned.Properties.RecipeConfig.Env))
					require.Equal(t, to.Ptr("myEnvValue"), versioned.Properties.RecipeConfig.Env["myEnvVar"])

					envSecretIDs := versioned.Properties.RecipeConfig.EnvSecrets
					envSecretRef, ok := envSecretIDs["myEnvSecretVar"]
					require.True(t, ok)
					require.Equal(t, envSecretRef, to.Ptr(SecretReference{Source: to.Ptr(baseSecretStorePath + "envSecretStore1"), Key: to.Ptr("envKey1")}))
					require.Equal(t, 1, len(envSecretIDs))
				}

				if tt.filename == "environmentresourcedatamodelemptyext.json" {
					switch c := recipeDetails.(type) {
					case *TerraformRecipeProperties:
						require.Nil(t, c.TemplateVersion)
					}

					require.Nil(t, versioned.Properties.RecipeConfig)
				}
			}
		})
	}
}

func TestConvertDataModelToVersioned_EmptyTemplateKind(t *testing.T) {
	rawPayload := testutil.ReadFixture("environmentresourcedatamodelemptytemplatekind.json")
	r := &datamodel.Environment{}
	err := json.Unmarshal(rawPayload, r)
	require.NoError(t, err)

	// act
	versioned := &EnvironmentResource{}
	err = versioned.ConvertFrom(r)

	// assert
	require.NoError(t, err)
	require.Equal(t, r.Name, string(*versioned.Name))
	require.Equal(t, r.Type, string(*versioned.Type))
	require.Equal(t, string(r.Properties.Compute.Kind), string(*versioned.Properties.Compute.GetEnvironmentCompute().Kind))
	require.Equal(t, r.Properties.Compute.KubernetesCompute.ResourceID, string(*versioned.Properties.Compute.GetEnvironmentCompute().ResourceID))
	require.Equal(t, len(r.Properties.Recipes), len(versioned.Properties.Recipes))
	require.Equal(t, r.Properties.Providers.Azure.Scope, string(*versioned.Properties.Providers.Azure.Scope))
}

func TestConvertDataModelWithIdentityToVersioned(t *testing.T) {
	// arrange
	rawPayload := testutil.ReadFixture("environmentresourcedatamodel-with-workload-identity.json")
	r := &datamodel.Environment{}
	err := json.Unmarshal(rawPayload, r)
	require.NoError(t, err)

	// act
	versioned := &EnvironmentResource{}
	err = versioned.ConvertFrom(r)

	// assert
	require.NoError(t, err)
	require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env0", string(*versioned.ID))
	require.Equal(t, "env0", string(*versioned.Name))
	require.Equal(t, "Applications.Core/environments", string(*versioned.Type))
	require.Equal(t, "kubernetes", string(*versioned.Properties.Compute.GetEnvironmentCompute().Kind))
	require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup/providers/Microsoft.ContainerService/managedClusters/radiusTestCluster", string(*versioned.Properties.Compute.GetEnvironmentCompute().ResourceID))
	require.Equal(t, 1, len(versioned.Properties.Recipes))
	require.Equal(t, "br:ghcr.io/sampleregistry/radius/recipes/cosmosdb", string(*versioned.Properties.Recipes[ds_ctrl.MongoDatabasesResourceType]["cosmos-recipe"].GetRecipeProperties().TemplatePath))
	require.Equal(t, recipes.TemplateKindBicep, string(*versioned.Properties.Recipes[ds_ctrl.MongoDatabasesResourceType]["cosmos-recipe"].GetRecipeProperties().TemplateKind))
	require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup", string(*versioned.Properties.Providers.Azure.Scope))
	require.Equal(t, &IdentitySettings{
		Kind:       to.Ptr(IdentitySettingKindAzureComWorkload),
		Resource:   to.Ptr("/subscriptions/testSub/resourcegroups/testGroup/providers/Microsoft.ManagedIdentity/userAssignedIdentities/radius-mi-app"),
		OidcIssuer: to.Ptr("https://oidcurl/guid"),
	}, versioned.Properties.Compute.GetEnvironmentCompute().Identity)
	require.Equal(t, "azure.com.workload", string(*versioned.Properties.Compute.GetEnvironmentCompute().Identity.Kind))
	require.Equal(t, "/subscriptions/testSub/resourcegroups/testGroup/providers/Microsoft.ManagedIdentity/userAssignedIdentities/radius-mi-app", string(*versioned.Properties.Compute.GetEnvironmentCompute().Identity.Resource))
	require.Equal(t, "https://oidcurl/guid", string(*versioned.Properties.Compute.GetEnvironmentCompute().Identity.OidcIssuer))
	require.Equal(t, map[string][]*ProviderConfigProperties{}, versioned.Properties.RecipeConfig.Terraform.Providers)
	require.Equal(t, map[string]*string{}, versioned.Properties.RecipeConfig.Env)
}

func TestConvertFromValidation(t *testing.T) {
	validationTests := []struct {
		src v1.DataModelInterface
		err error
	}{
		{&resourcetypeutil.FakeResource{}, v1.ErrInvalidModelConversion},
		{nil, v1.ErrInvalidModelConversion},
	}

	for _, tc := range validationTests {
		versioned := &EnvironmentResource{}
		err := versioned.ConvertFrom(tc.src)
		require.ErrorAs(t, tc.err, &err)
	}
}

func TestToEnvironmentComputeKindDataModel(t *testing.T) {
	kindTests := []struct {
		versioned string
		datamodel rpv1.EnvironmentComputeKind
		err       error
	}{
		{EnvironmentComputeKindKubernetes, rpv1.KubernetesComputeKind, nil},
		{"", rpv1.UnknownComputeKind, &v1.ErrModelConversion{PropertyName: "$.properties.compute.kind", ValidValue: "[kubernetes]"}},
	}

	for _, tt := range kindTests {
		sc, err := toEnvironmentComputeKindDataModel(tt.versioned)
		if tt.err != nil {
			require.ErrorIs(t, err, tt.err)
		}
		require.Equal(t, tt.datamodel, sc)
	}
}

func TestFromEnvironmentComputeKindDataModel(t *testing.T) {
	kindTests := []struct {
		datamodel rpv1.EnvironmentComputeKind
		versioned string
	}{
		{rpv1.KubernetesComputeKind, EnvironmentComputeKindKubernetes},
		{rpv1.UnknownComputeKind, EnvironmentComputeKindKubernetes},
	}

	for _, tt := range kindTests {
		sc := fromEnvironmentComputeKind(tt.datamodel)
		require.Equal(t, tt.versioned, *sc)
	}
}

func getTestKubernetesMetadataExtensions() []datamodel.Extension {
	extensions := []datamodel.Extension{
		{
			Kind: datamodel.KubernetesMetadata,
			KubernetesMetadata: &datamodel.KubeMetadataExtension{
				Annotations: map[string]string{
					"prometheus.io/scrape": "true",
					"prometheus.io/port":   "80",
				},
				Labels: map[string]string{
					"foo/bar/team":    "credit",
					"foo/bar/contact": "radiususer",
				},
			},
		},
	}

	return extensions
}

func getTestKubernetesEmptyMetadataExtensions() []datamodel.Extension {
	extensions := []datamodel.Extension{
		{
			Kind: datamodel.KubernetesMetadata,
			KubernetesMetadata: &datamodel.KubeMetadataExtension{
				Annotations: map[string]string{},
				Labels:      map[string]string{},
			},
		},
	}

	return extensions
}

func Test_toRecipeConfigTerraformProvidersDatamodel(t *testing.T) {
	tests := []struct {
		name        string
		config      *RecipeConfigProperties
		want        map[string][]datamodel.ProviderConfigProperties
		expectError bool
	}{
		{
			name:   "Empty Recipe Configuration",
			config: &RecipeConfigProperties{},
			want:   nil,
		},
		{
			name: "Single Provider Configuration",
			config: &RecipeConfigProperties{
				Terraform: &TerraformConfigProperties{
					Providers: map[string][]*ProviderConfigProperties{
						"azurerm": {
							&ProviderConfigProperties{
								AdditionalProperties: map[string]any{
									"subscription_id": "00000000-0000-0000-0000-000000000000",
								},
							},
						},
					},
				},
			},
			want: map[string][]datamodel.ProviderConfigProperties{
				"azurerm": {
					{
						AdditionalProperties: map[string]any{
							"subscription_id": "00000000-0000-0000-0000-000000000000",
						},
					},
				},
			},
		},
		{
			name: "Single Provider With Multiple Configuration",
			config: &RecipeConfigProperties{
				Terraform: &TerraformConfigProperties{
					Providers: map[string][]*ProviderConfigProperties{
						"azurerm": {
							{
								AdditionalProperties: map[string]any{
									"subscription_id": "00000000-0000-0000-0000-000000000000",
								},
							},
							{
								AdditionalProperties: map[string]any{
									"tenant_id": "00000000-0000-0000-0000-000000000000",
									"alias":     "az-example-service",
								},
							},
						},
					},
				},
			},
			want: map[string][]datamodel.ProviderConfigProperties{
				"azurerm": {
					{
						AdditionalProperties: map[string]any{
							"subscription_id": "00000000-0000-0000-0000-000000000000",
						},
					},
					{
						AdditionalProperties: map[string]any{
							"tenant_id": "00000000-0000-0000-0000-000000000000",
							"alias":     "az-example-service",
						},
					},
				},
			},
		},
		{
			name: "Multiple Providers With Multiple Configurations",
			config: &RecipeConfigProperties{
				Terraform: &TerraformConfigProperties{
					Providers: map[string][]*ProviderConfigProperties{
						"azurerm": {
							{
								AdditionalProperties: map[string]any{
									"subscription_id": "00000000-0000-0000-0000-000000000000",
								},
							},
							{
								AdditionalProperties: map[string]any{
									"tenant_id": "00000000-0000-0000-0000-000000000000",
									"alias":     "az-example-service",
								},
							},
						},
						"aws": {
							{
								AdditionalProperties: map[string]any{
									"region": "us-west-2",
								},
							},
							{
								AdditionalProperties: map[string]any{
									"account_id": "140313373712",
									"alias":      "account-service",
								},
							},
						},
					},
				},
			},
			want: map[string][]datamodel.ProviderConfigProperties{
				"azurerm": {
					{
						AdditionalProperties: map[string]any{
							"subscription_id": "00000000-0000-0000-0000-000000000000",
						},
					},
					{
						AdditionalProperties: map[string]any{
							"tenant_id": "00000000-0000-0000-0000-000000000000",
							"alias":     "az-example-service",
						},
					},
				},
				"aws": {
					{
						AdditionalProperties: map[string]any{
							"region": "us-west-2",
						},
					},
					{
						AdditionalProperties: map[string]any{
							"account_id": "140313373712",
							"alias":      "account-service",
						},
					},
				},
			},
		},
		{
			name: "Provider Configuration With Secret",
			config: &RecipeConfigProperties{
				Terraform: &TerraformConfigProperties{
					Providers: map[string][]*ProviderConfigProperties{
						"azurerm": {
							&ProviderConfigProperties{
								AdditionalProperties: map[string]any{
									"subscription_id": "00000000-0000-0000-0000-000000000000",
								},
								Secrets: map[string]*SecretReference{
									"secret1": {
										Source: to.Ptr("/planes/radius/local/resourcegroups/default/providers/Applications.Core/secretStores/secretstore1"),
										Key:    to.Ptr("key1"),
									},
								},
							},
						},
					},
				},
			},
			want: map[string][]datamodel.ProviderConfigProperties{
				"azurerm": {
					{
						AdditionalProperties: map[string]any{
							"subscription_id": "00000000-0000-0000-0000-000000000000",
						},
						Secrets: map[string]datamodel.SecretReference{
							"secret1": {
								Source: "/planes/radius/local/resourcegroups/default/providers/Applications.Core/secretStores/secretstore1",
								Key:    "key1",
							},
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := toRecipeConfigTerraformProvidersDatamodel(tt.config)
			require.Equal(t, tt.want, result)
		})
	}
}

func Test_fromRecipeConfigTerraformProvidersDatamodel(t *testing.T) {
	tests := []struct {
		name   string
		config datamodel.RecipeConfigProperties
		want   map[string][]*ProviderConfigProperties
	}{
		{
			name:   "Empty Recipe Configuration",
			config: datamodel.RecipeConfigProperties{},
			want:   nil,
		},
		{
			name: "Single Provider Configuration",
			config: datamodel.RecipeConfigProperties{
				Terraform: datamodel.TerraformConfigProperties{
					Providers: map[string][]datamodel.ProviderConfigProperties{
						"azurerm": {
							{
								AdditionalProperties: map[string]any{
									"subscription_id": "00000000-0000-0000-0000-000000000000",
								},
							},
						},
					},
				},
			},
			want: map[string][]*ProviderConfigProperties{
				"azurerm": {
					{
						AdditionalProperties: map[string]any{
							"subscription_id": "00000000-0000-0000-0000-000000000000",
						},
					},
				},
			},
		},
		{
			name: "Single Provider With Multiple Configuration",
			config: datamodel.RecipeConfigProperties{
				Terraform: datamodel.TerraformConfigProperties{
					Providers: map[string][]datamodel.ProviderConfigProperties{
						"azurerm": {
							{
								AdditionalProperties: map[string]any{
									"subscription_id": "00000000-0000-0000-0000-000000000000",
								},
							},
							{
								AdditionalProperties: map[string]any{
									"tenant_id": "00000000-0000-0000-0000-000000000000",
									"alias":     "tenant",
								},
							},
						},
					},
				},
			},
			want: map[string][]*ProviderConfigProperties{
				"azurerm": {
					{
						AdditionalProperties: map[string]any{
							"subscription_id": "00000000-0000-0000-0000-000000000000",
						},
					},
					{
						AdditionalProperties: map[string]any{
							"tenant_id": "00000000-0000-0000-0000-000000000000",
							"alias":     "tenant",
						},
					},
				},
			},
		},
		{
			name: "Multiple Providers With Multiple Configurations",
			config: datamodel.RecipeConfigProperties{
				Terraform: datamodel.TerraformConfigProperties{
					Providers: map[string][]datamodel.ProviderConfigProperties{
						"azurerm": {
							{
								AdditionalProperties: map[string]any{
									"subscription_id": "00000000-0000-0000-0000-000000000000",
								},
							},
						},
						"aws": {
							{
								AdditionalProperties: map[string]any{
									"region": "us-west-2",
								},
							},
							{
								AdditionalProperties: map[string]any{
									"account_id": "140313373712",
									"alias":      "account",
								},
							},
						},
					},
				},
			},
			want: map[string][]*ProviderConfigProperties{
				"azurerm": {
					{
						AdditionalProperties: map[string]any{
							"subscription_id": "00000000-0000-0000-0000-000000000000",
						},
					},
				},
				"aws": {
					{
						AdditionalProperties: map[string]any{
							"region": "us-west-2",
						},
					},
					{
						AdditionalProperties: map[string]any{
							"account_id": "140313373712",
							"alias":      "account",
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := fromRecipeConfigTerraformProvidersDatamodel(tt.config)
			require.Equal(t, tt.want, result)
		})
	}
}

func Test_toRecipeConfigEnvDatamodel(t *testing.T) {
	tests := []struct {
		name   string
		config *RecipeConfigProperties
		want   datamodel.EnvironmentVariables
	}{
		{
			name:   "Empty Recipe Configuration",
			config: &RecipeConfigProperties{},
			want:   datamodel.EnvironmentVariables{},
		},
		{
			name: "With Multiple Environment Variables",
			config: &RecipeConfigProperties{
				Env: map[string]*string{
					"key1": to.Ptr("value1"),
					"key2": to.Ptr("value2"),
				},
			},
			want: datamodel.EnvironmentVariables{
				AdditionalProperties: map[string]string{
					"key1": "value1",
					"key2": "value2",
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := toRecipeConfigEnvDatamodel(tt.config)
			require.Equal(t, tt.want, result)
		})
	}
}

func Test_fromRecipeConfigEnvDatamodel(t *testing.T) {
	tests := []struct {
		name   string
		config datamodel.RecipeConfigProperties
		want   map[string]*string
	}{
		{
			name:   "Empty Recipe Configuration",
			config: datamodel.RecipeConfigProperties{},
			want:   map[string]*string{},
		},
		{
			name: "With Multiple Environment Variables",
			config: datamodel.RecipeConfigProperties{
				Env: datamodel.EnvironmentVariables{
					AdditionalProperties: map[string]string{
						"key1": "value1",
						"key2": "value2",
					},
				},
			},
			want: map[string]*string{
				"key1": to.Ptr("value1"),
				"key2": to.Ptr("value2"),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := fromRecipeConfigEnvDatamodel(tt.config)
			require.Equal(t, tt.want, result)
		})
	}
}

func Test_toSecretReferenceDatamodel(t *testing.T) {
	tests := []struct {
		name           string
		configSecrets  map[string]*SecretReference
		expectedResult map[string]datamodel.SecretReference
	}{
		{
			name: "Multiple Provider Secrets",
			configSecrets: map[string]*SecretReference{
				"secret1": {
					Source: to.Ptr("source1"),
					Key:    to.Ptr("key1"),
				},
				"secret2": {
					Source: to.Ptr("source2"),
					Key:    to.Ptr("key2"),
				},
			},
			expectedResult: map[string]datamodel.SecretReference{
				"secret1": {
					Source: "source1",
					Key:    "key1",
				},
				"secret2": {
					Source: "source2",
					Key:    "key2",
				},
			},
		},
		{
			name:           "Nil Provider Secrets",
			configSecrets:  nil,
			expectedResult: nil,
		},
		{
			name: "Nil Secret in Provider Properties",
			configSecrets: map[string]*SecretReference{
				"secret1": nil,
			},
			expectedResult: nil,
		},
		{
			name: "Nil + Valid Secret in Provider Properties",
			configSecrets: map[string]*SecretReference{
				"secret1": nil,
				"secret2": {
					Source: to.Ptr("source2"),
					Key:    to.Ptr("key2"),
				},
			},
			expectedResult: map[string]datamodel.SecretReference{
				"secret2": {
					Source: "source2",
					Key:    "key2",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := toSecretReferenceDatamodel(tt.configSecrets)
			require.Equal(t, tt.expectedResult, result)
		})
	}
}

func Test_fromSecretReferenceDatamodel(t *testing.T) {
	tests := []struct {
		name     string
		secrets  map[string]datamodel.SecretReference
		expected map[string]*SecretReference
	}{
		{
			name:     "Empty Secret",
			secrets:  map[string]datamodel.SecretReference{},
			expected: nil,
		},
		{
			name:     "Nil Secret",
			secrets:  nil,
			expected: nil,
		},
		{
			name: "Single Secret",
			secrets: map[string]datamodel.SecretReference{
				"secret1": {Source: "source1", Key: "key1"},
			},
			expected: map[string]*SecretReference{
				"secret1": {Source: to.Ptr("source1"), Key: to.Ptr("key1")},
			},
		},
		{
			name: "Multiple Secrets",
			secrets: map[string]datamodel.SecretReference{
				"secret1": {Source: "source1", Key: "key1"},
				"secret2": {Source: "source2", Key: "key2"},
			},
			expected: map[string]*SecretReference{
				"secret1": {Source: to.Ptr("source1"), Key: to.Ptr("key1")},
				"secret2": {Source: to.Ptr("source2"), Key: to.Ptr("key2")},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := fromSecretReferenceDatamodel(tt.secrets)
			require.Equal(t, tt.expected, result)
		})
	}
}
