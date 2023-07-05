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

package v20220315privatepreview

import (
	"encoding/json"
	"testing"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/linkrp"
	"github.com/project-radius/radius/pkg/recipes"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
	"github.com/project-radius/radius/pkg/to"
	"github.com/project-radius/radius/test/testutil"

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
						CreatedAPIVersion:      "2022-03-15-privatepreview",
						UpdatedAPIVersion:      "2022-03-15-privatepreview",
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
					Recipes: map[string]map[string]datamodel.EnvironmentRecipeProperties{
						linkrp.MongoDatabasesResourceType: {
							"cosmos-recipe": datamodel.EnvironmentRecipeProperties{
								TemplateKind: recipes.TemplateKindBicep,
								TemplatePath: "br:sampleregistry.azureacr.io/radius/recipes/cosmosdb",
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
						CreatedAPIVersion:      "2022-03-15-privatepreview",
						UpdatedAPIVersion:      "2022-03-15-privatepreview",
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
					Recipes: map[string]map[string]datamodel.EnvironmentRecipeProperties{
						linkrp.MongoDatabasesResourceType: {
							"cosmos-recipe": datamodel.EnvironmentRecipeProperties{
								TemplateKind: recipes.TemplateKindBicep,
								TemplatePath: "br:sampleregistry.azureacr.io/radius/recipes/mongodatabases",
								Parameters: map[string]any{
									"throughput": float64(400),
								},
							},
							"terraform-recipe": datamodel.EnvironmentRecipeProperties{
								TemplateKind: recipes.TemplateKindTerraform,
								TemplatePath: "Azure/cosmosdb/azurerm",
							},
						},
						linkrp.RedisCachesResourceType: {
							"redis-recipe": datamodel.EnvironmentRecipeProperties{
								TemplateKind: recipes.TemplateKindBicep,
								TemplatePath: "br:sampleregistry.azureacr.io/radius/recipes/rediscaches",
							},
						},
						linkrp.DaprStateStoresResourceType: {
							"statestore-recipe": datamodel.EnvironmentRecipeProperties{
								TemplateKind: recipes.TemplateKindTerraform,
								TemplatePath: "Azure/storage/azurerm",
							},
						},
					},
					Extensions: getTestKubernetesMetadataExtensions(t),
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
						CreatedAPIVersion:      "2022-03-15-privatepreview",
						UpdatedAPIVersion:      "2022-03-15-privatepreview",
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
						linkrp.MongoDatabasesResourceType: {
							"cosmos-recipe": datamodel.EnvironmentRecipeProperties{
								TemplateKind: recipes.TemplateKindBicep,
								TemplatePath: "br:sampleregistry.azureacr.io/radius/recipes/cosmosdb",
							},
						},
					},
					Extensions: getTestKubernetesEmptyMetadataExtensions(t),
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
						CreatedAPIVersion:      "2022-03-15-privatepreview",
						UpdatedAPIVersion:      "2022-03-15-privatepreview",
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
						linkrp.MongoDatabasesResourceType: {
							"cosmos-recipe": datamodel.EnvironmentRecipeProperties{
								TemplateKind: recipes.TemplateKindBicep,
								TemplatePath: "br:sampleregistry.azureacr.io/radius/recipes/cosmosdb",
							},
						},
					},
					Extensions: getTestKubernetesEmptyMetadataExtensions(t),
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
			filename: "environmentresource-invalid-linktype.json",
			err:      &v1.ErrClientRP{Code: v1.CodeInvalid, Message: "invalid link type: \"Applications.Link/pubsub\""},
		},
		{
			filename: "environmentresource-invalid-templatekind.json",
			err:      &v1.ErrClientRP{Code: v1.CodeInvalid, Message: "invalid template kind. Allowed formats: \"bicep\""},
		},
		{
			filename: "environmentresource-missing-templatekind.json",
			err:      &v1.ErrClientRP{Code: v1.CodeInvalid, Message: "invalid template kind. Allowed formats: \"bicep\""},
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
				require.Equal(t, "br:sampleregistry.azureacr.io/radius/recipes/cosmosdb", string(*versioned.Properties.Recipes[linkrp.MongoDatabasesResourceType]["cosmos-recipe"].GetEnvironmentRecipeProperties().TemplatePath))
				require.Equal(t, recipes.TemplateKindBicep, string(*versioned.Properties.Recipes[linkrp.MongoDatabasesResourceType]["cosmos-recipe"].GetEnvironmentRecipeProperties().TemplateKind))
				require.Equal(t, map[string]any{"throughput": float64(400)}, versioned.Properties.Recipes[linkrp.MongoDatabasesResourceType]["cosmos-recipe"].GetEnvironmentRecipeProperties().Parameters)
				require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup", string(*versioned.Properties.Providers.Azure.Scope))
				require.Equal(t, "/planes/aws/aws/accounts/140313373712/regions/us-west-2", string(*versioned.Properties.Providers.Aws.Scope))
				require.Equal(t, "kubernetesMetadata", *versioned.Properties.Extensions[0].GetExtension().Kind)
				require.Equal(t, 1, len(versioned.Properties.Extensions))
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
	require.Equal(t, r.ID, string(*versioned.ID))
	require.Equal(t, r.Name, string(*versioned.Name))
	require.Equal(t, r.Type, string(*versioned.Type))
	require.Equal(t, string(r.Properties.Compute.Kind), string(*versioned.Properties.Compute.GetEnvironmentCompute().Kind))
	require.Equal(t, r.Properties.Compute.KubernetesCompute.ResourceID, string(*versioned.Properties.Compute.GetEnvironmentCompute().ResourceID))
	require.Equal(t, len(r.Properties.Recipes), len(versioned.Properties.Recipes))
	require.Equal(t, r.Properties.Recipes[linkrp.MongoDatabasesResourceType]["cosmos-recipe"].TemplatePath, string(*versioned.Properties.Recipes[linkrp.MongoDatabasesResourceType]["cosmos-recipe"].GetEnvironmentRecipeProperties().TemplatePath))
	require.Equal(t, r.Properties.Recipes[linkrp.MongoDatabasesResourceType]["cosmos-recipe"].TemplateKind, string(*versioned.Properties.Recipes[linkrp.MongoDatabasesResourceType]["cosmos-recipe"].GetEnvironmentRecipeProperties().TemplateKind))
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
	require.Equal(t, "br:sampleregistry.azureacr.io/radius/recipes/cosmosdb", string(*versioned.Properties.Recipes[linkrp.MongoDatabasesResourceType]["cosmos-recipe"].GetEnvironmentRecipeProperties().TemplatePath))
	require.Equal(t, recipes.TemplateKindBicep, string(*versioned.Properties.Recipes[linkrp.MongoDatabasesResourceType]["cosmos-recipe"].GetEnvironmentRecipeProperties().TemplateKind))
	require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup", string(*versioned.Properties.Providers.Azure.Scope))

	require.Equal(t, &IdentitySettings{
		Kind:       to.Ptr(IdentitySettingKindAzureComWorkload),
		Resource:   to.Ptr("/subscriptions/testSub/resourcegroups/testGroup/providers/Microsoft.ManagedIdentity/userAssignedIdentities/radius-mi-app"),
		OidcIssuer: to.Ptr("https://oidcurl/guid"),
	}, versioned.Properties.Compute.GetEnvironmentCompute().Identity)
	require.Equal(t, "azure.com.workload", string(*versioned.Properties.Compute.GetEnvironmentCompute().Identity.Kind))
	require.Equal(t, "/subscriptions/testSub/resourcegroups/testGroup/providers/Microsoft.ManagedIdentity/userAssignedIdentities/radius-mi-app", string(*versioned.Properties.Compute.GetEnvironmentCompute().Identity.Resource))
	require.Equal(t, "https://oidcurl/guid", string(*versioned.Properties.Compute.GetEnvironmentCompute().Identity.OidcIssuer))
}

type fakeResource struct{}

func (f *fakeResource) ResourceTypeName() string {
	return "FakeResource"
}

func (f *fakeResource) GetSystemData() *v1.SystemData {
	return nil
}

func (f *fakeResource) GetBaseResource() *v1.BaseResource {
	return nil
}

func (f *fakeResource) ProvisioningState() v1.ProvisioningState {
	return v1.ProvisioningStateAccepted
}

func (f *fakeResource) SetProvisioningState(state v1.ProvisioningState) {
}

func (f *fakeResource) UpdateMetadata(ctx *v1.ARMRequestContext, oldResource *v1.BaseResource) {
}

func TestConvertFromValidation(t *testing.T) {
	validationTests := []struct {
		src v1.DataModelInterface
		err error
	}{
		{&fakeResource{}, v1.ErrInvalidModelConversion},
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

func getTestKubernetesMetadataExtensions(t *testing.T) []datamodel.Extension {
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

func getTestKubernetesEmptyMetadataExtensions(t *testing.T) []datamodel.Extension {
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
