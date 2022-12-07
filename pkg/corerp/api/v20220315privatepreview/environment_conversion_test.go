// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v20220315privatepreview

import (
	"encoding/json"
	"testing"

	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/rp"
	"github.com/stretchr/testify/require"

	radiustesting "github.com/project-radius/radius/pkg/corerp/testing"
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
					Compute: rp.EnvironmentCompute{
						Kind: "kubernetes",
						KubernetesCompute: rp.KubernetesComputeProperties{
							ResourceID: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup/providers/Microsoft.ContainerService/managedClusters/radiusTestCluster",
							Namespace:  "default",
						},
						Identity: &rp.IdentitySettings{
							Kind:       rp.AzureIdentityWorkload,
							Resource:   "/subscriptions/testSub/resourcegroups/testGroup/providers/Microsoft.ManagedIdentity/userAssignedIdentities/radius-mi-app",
							OIDCIssuer: "https://oidcurl/guid",
						},
					},
					Providers: datamodel.Providers{
						Azure: datamodel.ProvidersAzure{
							Scope: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup",
						},
					},
					Recipes: map[string]datamodel.EnvironmentRecipeProperties{
						"cosmos-recipe": {
							LinkType:     "Applications.Link/mongoDatabases",
							TemplatePath: "br:sampleregistry.azureacr.io/radius/recipes/cosmosdb",
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
					Compute: rp.EnvironmentCompute{
						Kind: "kubernetes",
						KubernetesCompute: rp.KubernetesComputeProperties{
							ResourceID: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup/providers/Microsoft.ContainerService/managedClusters/radiusTestCluster",
							Namespace:  "default",
						},
					},
					Providers: datamodel.Providers{
						Azure: datamodel.ProvidersAzure{
							Scope: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup",
						},
					},
					Recipes: map[string]datamodel.EnvironmentRecipeProperties{
						"cosmos-recipe": {
							LinkType:     "Applications.Link/mongoDatabases",
							TemplatePath: "br:sampleregistry.azureacr.io/radius/recipes/cosmosdb",
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
					Compute: rp.EnvironmentCompute{
						Kind: "kubernetes",
						KubernetesCompute: rp.KubernetesComputeProperties{
							ResourceID: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup/providers/Microsoft.ContainerService/managedClusters/radiusTestCluster",
							Namespace:  "default",
						},
					},
					Providers: datamodel.Providers{
						Azure: datamodel.ProvidersAzure{
							Scope: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup",
						},
					},
					Recipes: map[string]datamodel.EnvironmentRecipeProperties{
						"cosmos-recipe": {
							LinkType:     "Applications.Link/mongoDatabases",
							TemplatePath: "br:sampleregistry.azureacr.io/radius/recipes/cosmosdb",
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
					Compute: rp.EnvironmentCompute{
						Kind: "kubernetes",
						KubernetesCompute: rp.KubernetesComputeProperties{
							ResourceID: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup/providers/Microsoft.ContainerService/managedClusters/radiusTestCluster",
							Namespace:  "default",
						},
					},
					Providers: datamodel.Providers{
						Azure: datamodel.ProvidersAzure{
							Scope: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup",
						},
					},
					Recipes: map[string]datamodel.EnvironmentRecipeProperties{
						"cosmos-recipe": {
							LinkType:     "Applications.Link/mongoDatabases",
							TemplatePath: "br:sampleregistry.azureacr.io/radius/recipes/cosmosdb",
						},
					},
					Extensions: getTestKubernetesEmptyMetadataExtensions(t),
				},
			},
			err: nil,
		},
		{
			filename: "environmentresource-invalid-missing-namespace.json",
			err:      &conv.ErrModelConversion{PropertyName: "$.properties.compute.namespace", ValidValue: "63 characters or less"},
		},
		{
			filename: "environmentresource-invalid-namespace.json",
			err:      &conv.ErrModelConversion{PropertyName: "$.properties.compute.namespace", ValidValue: "63 characters or less"},
		},
	}

	for _, tt := range conversionTests {
		t.Run(tt.filename, func(t *testing.T) {
			rawPayload := radiustesting.ReadFixture(tt.filename)
			r := &EnvironmentResource{}
			err := json.Unmarshal(rawPayload, r)
			require.NoError(t, err)

			// act
			dm, err := r.ConvertTo()

			if tt.err != nil {
				require.ErrorIs(t, err, tt.err)
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
			rawPayload := radiustesting.ReadFixture(tt.filename)
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
				require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env0", r.ID)
				require.Equal(t, "env0", r.Name)
				require.Equal(t, "Applications.Core/environments", r.Type)
				require.Equal(t, "kubernetes", string(r.Properties.Compute.Kind))
				require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup/providers/Microsoft.ContainerService/managedClusters/radiusTestCluster", r.Properties.Compute.KubernetesCompute.ResourceID)
				require.Equal(t, 1, len(r.Properties.Recipes))
				require.Equal(t, "Applications.Link/mongoDatabases", r.Properties.Recipes["cosmos-recipe"].LinkType)
				require.Equal(t, "br:sampleregistry.azureacr.io/radius/recipes/cosmosdb", r.Properties.Recipes["cosmos-recipe"].TemplatePath)
				require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup", r.Properties.Providers.Azure.Scope)
				require.Equal(t, "kubernetesMetadata", *versioned.Properties.Extensions[0].GetExtension().Kind)
				require.Equal(t, 1, len(versioned.Properties.Extensions))
			}
		})
	}
}

func TestConvertDataModelWithIdentityToVersioned(t *testing.T) {
	// arrange
	rawPayload := radiustesting.ReadFixture("environmentresourcedatamodel-with-workload-identity.json")
	r := &datamodel.Environment{}
	err := json.Unmarshal(rawPayload, r)
	require.NoError(t, err)

	// act
	versioned := &EnvironmentResource{}
	err = versioned.ConvertFrom(r)

	// assert
	require.NoError(t, err)
	require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env0", r.ID)
	require.Equal(t, "env0", r.Name)
	require.Equal(t, "Applications.Core/environments", r.Type)
	require.Equal(t, "kubernetes", string(r.Properties.Compute.Kind))
	require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup/providers/Microsoft.ContainerService/managedClusters/radiusTestCluster", r.Properties.Compute.KubernetesCompute.ResourceID)
	require.Equal(t, 1, len(r.Properties.Recipes))
	require.Equal(t, "Applications.Link/mongoDatabases", r.Properties.Recipes["cosmos-recipe"].LinkType)
	require.Equal(t, "br:sampleregistry.azureacr.io/radius/recipes/cosmosdb", r.Properties.Recipes["cosmos-recipe"].TemplatePath)
	require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup", r.Properties.Providers.Azure.Scope)

	require.Equal(t, &rp.IdentitySettings{
		Kind:       rp.AzureIdentityWorkload,
		Resource:   "/subscriptions/testSub/resourcegroups/testGroup/providers/Microsoft.ManagedIdentity/userAssignedIdentities/radius-mi-app",
		OIDCIssuer: "https://oidcurl/guid",
	}, r.Properties.Compute.Identity)
	require.Equal(t, rp.AzureIdentityWorkload, r.Properties.Compute.Identity.Kind)
	require.Equal(t, "/subscriptions/testSub/resourcegroups/testGroup/providers/Microsoft.ManagedIdentity/userAssignedIdentities/radius-mi-app", r.Properties.Compute.Identity.Resource)
	require.Equal(t, "https://oidcurl/guid", r.Properties.Compute.Identity.OIDCIssuer)
}

type fakeResource struct{}

func (f *fakeResource) ResourceTypeName() string {
	return "FakeResource"
}

func (f *fakeResource) GetSystemData() *v1.SystemData {
	return nil
}

func (f *fakeResource) ProvisioningState() v1.ProvisioningState {
	return v1.ProvisioningStateAccepted
}

func (f *fakeResource) SetProvisioningState(state v1.ProvisioningState) {
}

func (f *fakeResource) UpdateMetadata(ctx *v1.ARMRequestContext) {
}

func TestConvertFromValidation(t *testing.T) {
	validationTests := []struct {
		src conv.DataModelInterface
		err error
	}{
		{&fakeResource{}, conv.ErrInvalidModelConversion},
		{nil, conv.ErrInvalidModelConversion},
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
		datamodel rp.EnvironmentComputeKind
		err       error
	}{
		{EnvironmentComputeKindKubernetes, rp.KubernetesComputeKind, nil},
		{"", rp.UnknownComputeKind, &conv.ErrModelConversion{PropertyName: "$.properties.compute.kind", ValidValue: "[kubernetes]"}},
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
		datamodel rp.EnvironmentComputeKind
		versioned string
	}{
		{rp.KubernetesComputeKind, EnvironmentComputeKindKubernetes},
		{rp.UnknownComputeKind, EnvironmentComputeKindKubernetes},
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
