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
	"fmt"
	"testing"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/portableresources"
	"github.com/radius-project/radius/pkg/recipes"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	"github.com/radius-project/radius/pkg/to"
	"github.com/stretchr/testify/require"
)

func TestToProvisioningStateDataModel(t *testing.T) {
	stateTests := []struct {
		versioned ProvisioningState
		datamodel v1.ProvisioningState
	}{
		{ProvisioningStateUpdating, v1.ProvisioningStateUpdating},
		{ProvisioningStateDeleting, v1.ProvisioningStateDeleting},
		{ProvisioningStateAccepted, v1.ProvisioningStateAccepted},
		{ProvisioningStateSucceeded, v1.ProvisioningStateSucceeded},
		{ProvisioningStateFailed, v1.ProvisioningStateFailed},
		{ProvisioningStateCanceled, v1.ProvisioningStateCanceled},
		{"", v1.ProvisioningStateAccepted},
	}

	for _, tt := range stateTests {
		sc := toProvisioningStateDataModel(&tt.versioned)
		require.Equal(t, tt.datamodel, sc)
	}
}

func TestFromProvisioningStateDataModel(t *testing.T) {
	testCases := []struct {
		datamodel v1.ProvisioningState
		versioned ProvisioningState
	}{
		{v1.ProvisioningStateUpdating, ProvisioningStateUpdating},
		{v1.ProvisioningStateDeleting, ProvisioningStateDeleting},
		{v1.ProvisioningStateAccepted, ProvisioningStateAccepted},
		{v1.ProvisioningStateSucceeded, ProvisioningStateSucceeded},
		{v1.ProvisioningStateFailed, ProvisioningStateFailed},
		{v1.ProvisioningStateCanceled, ProvisioningStateCanceled},
		{"", ProvisioningStateAccepted},
	}

	for _, testCase := range testCases {
		sc := fromProvisioningStateDataModel(testCase.datamodel)
		require.Equal(t, testCase.versioned, *sc)
	}
}

func TestFromSystemDataModel(t *testing.T) {
	systemDataTests := []v1.SystemData{
		{
			CreatedBy:          "",
			CreatedByType:      "",
			CreatedAt:          "",
			LastModifiedBy:     "",
			LastModifiedByType: "",
			LastModifiedAt:     "",
		}, {
			CreatedBy:          "fakeid@live.com",
			CreatedByType:      "",
			CreatedAt:          "2021-09-24T19:09:00Z",
			LastModifiedBy:     "fakeid@live.com",
			LastModifiedByType: "",
			LastModifiedAt:     "2021-09-25T19:09:00Z",
		}, {
			CreatedBy:          "fakeid@live.com",
			CreatedByType:      "User",
			CreatedAt:          "2021-09-24T19:09:00Z",
			LastModifiedBy:     "fakeid@live.com",
			LastModifiedByType: "User",
			LastModifiedAt:     "2021-09-25T19:09:00Z",
		},
	}

	for _, tt := range systemDataTests {
		versioned := fromSystemDataModel(tt)
		require.Equal(t, tt.CreatedBy, string(*versioned.CreatedBy))
		require.Equal(t, tt.CreatedByType, string(*versioned.CreatedByType))
		c, err := versioned.CreatedAt.MarshalText()
		require.NoError(t, err)
		if tt.CreatedAt == "" {
			tt.CreatedAt = "0001-01-01T00:00:00Z"
		}
		require.Equal(t, tt.CreatedAt, string(c))

		require.Equal(t, tt.LastModifiedBy, string(*versioned.LastModifiedBy))
		require.Equal(t, tt.LastModifiedByType, string(*versioned.LastModifiedByType))
		c, err = versioned.LastModifiedAt.MarshalText()
		require.NoError(t, err)
		if tt.LastModifiedAt == "" {
			tt.LastModifiedAt = "0001-01-01T00:00:00Z"
		}
		require.Equal(t, tt.LastModifiedAt, string(c))
	}
}

func TestToResourcesDataModel(t *testing.T) {
	testset := []struct {
		DMResources        []*portableresources.ResourceReference
		VersionedResources []*ResourceReference
	}{
		{
			DMResources:        []*portableresources.ResourceReference{{ID: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Microsoft.Cache/Redis/testCache"}, {ID: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Microsoft.Cache/Redis/testCache1"}},
			VersionedResources: []*ResourceReference{{ID: to.Ptr("/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Microsoft.Cache/Redis/testCache")}, {ID: to.Ptr("/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Microsoft.Cache/Redis/testCache1")}},
		},
		{
			DMResources:        []*portableresources.ResourceReference{},
			VersionedResources: []*ResourceReference{},
		},
	}

	for _, tt := range testset {
		dm := toResourcesDataModel(tt.VersionedResources)
		require.Equal(t, tt.DMResources, dm)

	}
}

func TestFromResourcesDataModel(t *testing.T) {
	testset := []struct {
		DMResources        []*portableresources.ResourceReference
		VersionedResources []*ResourceReference
	}{
		{
			DMResources:        []*portableresources.ResourceReference{{ID: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Microsoft.Cache/Redis/testCache"}, {ID: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Microsoft.Cache/Redis/testCache1"}},
			VersionedResources: []*ResourceReference{{ID: to.Ptr("/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Microsoft.Cache/Redis/testCache")}, {ID: to.Ptr("/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Microsoft.Cache/Redis/testCache1")}},
		},
		{
			DMResources:        []*portableresources.ResourceReference{},
			VersionedResources: []*ResourceReference{},
		},
	}

	for _, tt := range testset {
		versioned := fromResourcesDataModel(tt.DMResources)
		require.Equal(t, tt.VersionedResources, versioned)

	}
}

func TestToResourceProvisiongDataModel(t *testing.T) {
	testset := []struct {
		versioned ResourceProvisioning
		datamodel portableresources.ResourceProvisioning
		err       error
	}{
		{
			ResourceProvisioningManual,
			portableresources.ResourceProvisioningManual,
			nil,
		},
		{
			ResourceProvisioningRecipe,
			portableresources.ResourceProvisioningRecipe,
			nil,
		},
		{
			"",
			"",
			&v1.ErrModelConversion{
				PropertyName: "$.properties.resourceProvisioning",
				ValidValue:   fmt.Sprintf("one of %s", PossibleResourceProvisioningValues()),
			},
		},
	}
	for _, tt := range testset {
		sc, err := toResourceProvisiongDataModel(&tt.versioned)

		if tt.err != nil {
			require.EqualError(t, err, tt.err.Error())
			continue
		}

		require.NoError(t, err)
		require.Equal(t, tt.datamodel, sc)
	}
}

func TestFromResourceProvisiongDataModel(t *testing.T) {
	testCases := []struct {
		datamodel portableresources.ResourceProvisioning
		versioned ResourceProvisioning
	}{
		{portableresources.ResourceProvisioningManual, ResourceProvisioningManual},
		{portableresources.ResourceProvisioningRecipe, ResourceProvisioningRecipe},
		{"", ResourceProvisioningRecipe},
	}

	for _, testCase := range testCases {
		sc := fromResourceProvisioningDataModel(testCase.datamodel)
		require.Equal(t, testCase.versioned, *sc)
	}
}

func Test_fromRecipeStatus(t *testing.T) {
	testCases := []struct {
		recipeStatus *rpv1.RecipeStatus
		expected     *RecipeStatus
	}{
		{&rpv1.RecipeStatus{
			TemplateKind:    recipes.TemplateKindTerraform,
			TemplatePath:    "/path/to/template.tf",
			TemplateVersion: "1.0",
		}, &RecipeStatus{
			TemplateKind:    to.Ptr(recipes.TemplateKindTerraform),
			TemplatePath:    to.Ptr("/path/to/template.tf"),
			TemplateVersion: to.Ptr("1.0"),
		}},
		{nil, nil},
		{&rpv1.RecipeStatus{
			TemplateKind: recipes.TemplateKindBicep,
			TemplatePath: "/path/to/template.bicep",
		}, &RecipeStatus{
			TemplateKind:    to.Ptr(recipes.TemplateKindBicep),
			TemplatePath:    to.Ptr("/path/to/template.bicep"),
			TemplateVersion: nil,
		}},
	}

	for _, tt := range testCases {
		status := fromRecipeStatus(tt.recipeStatus)
		if tt.expected == nil {
			require.Nil(t, status)
		} else {
			require.Equal(t, *tt.expected, *status)
		}
	}
}

func TestToRecipeDataModel(t *testing.T) {
	testset := []struct {
		versioned *Recipe
		datamodel portableresources.ResourceRecipe
	}{
		{
			nil,
			portableresources.ResourceRecipe{
				Name: portableresources.DefaultRecipeName,
			},
		},
		{
			&Recipe{
				Name: to.Ptr("test"),
				Parameters: map[string]any{
					"foo": "bar",
				},
			},
			portableresources.ResourceRecipe{
				Name: "test",
				Parameters: map[string]any{
					"foo": "bar",
				},
			},
		},
		{
			&Recipe{
				Parameters: map[string]any{
					"foo": "bar",
				},
			},
			portableresources.ResourceRecipe{
				Name: portableresources.DefaultRecipeName,
				Parameters: map[string]any{
					"foo": "bar",
				},
			},
		},
	}
	for _, testCase := range testset {
		sc := toRecipeDataModel(testCase.versioned)
		require.Equal(t, testCase.datamodel, sc)
	}
}

func TestToMetadataDataModel(t *testing.T) {
	testCases := []struct {
		metadata map[string]*MetadataValue
		expected map[string]*rpv1.DaprComponentMetadataValue
	}{
		{
			metadata: nil,
			expected: nil,
		},
		{
			metadata: map[string]*MetadataValue{"config": {Value: to.Ptr("extrasecure")}},
			expected: map[string]*rpv1.DaprComponentMetadataValue{"config": {Value: "extrasecure"}},
		},
		{
			metadata: map[string]*MetadataValue{
				"secret": {
					SecretKeyRef: &MetadataValueFromSecret{
						Key:  to.Ptr("secretKey"),
						Name: to.Ptr("secretValue"),
					},
				},
			},
			expected: map[string]*rpv1.DaprComponentMetadataValue{
				"secret": {
					SecretKeyRef: &rpv1.DaprComponentSecretRef{
						Key:  "secretKey",
						Name: "secretValue",
					},
				},
			},
		},
	}

	for _, tt := range testCases {
		actual := toMetadataDataModel(tt.metadata)
		require.Equal(t, tt.expected, actual)
	}
}

func TestFromMetadataDataModel(t *testing.T) {
	testCases := []struct {
		metadata map[string]*rpv1.DaprComponentMetadataValue
		expected map[string]*MetadataValue
	}{
		{
			metadata: nil,
			expected: nil,
		},
		{
			metadata: map[string]*rpv1.DaprComponentMetadataValue{"config": {Value: "extrasecure"}},
			expected: map[string]*MetadataValue{"config": {Value: to.Ptr("extrasecure")}},
		},
		{
			metadata: map[string]*rpv1.DaprComponentMetadataValue{
				"secret": {
					SecretKeyRef: &rpv1.DaprComponentSecretRef{
						Key:  "secretKey",
						Name: "secretValue",
					},
				},
			},
			expected: map[string]*MetadataValue{
				"secret": {
					Value: to.Ptr(""),
					SecretKeyRef: &MetadataValueFromSecret{
						Key:  to.Ptr("secretKey"),
						Name: to.Ptr("secretValue"),
					},
				},
			},
		},
	}

	for _, tt := range testCases {
		actual := fromMetadataDataModel(tt.metadata)
		require.Equal(t, tt.expected, actual)
	}
}

func TestToAuthDataModel(t *testing.T) {
	testCases := []struct {
		auth     *DaprResourceAuth
		expected *rpv1.DaprComponentAuth
	}{
		{
			auth:     nil,
			expected: nil,
		},
		{
			auth: &DaprResourceAuth{
				SecretStore: to.Ptr("test-secretstore"),
			},
			expected: &rpv1.DaprComponentAuth{
				SecretStore: "test-secretstore",
			},
		},
		{
			auth: &DaprResourceAuth{
				SecretStore: nil,
			},
			expected: &rpv1.DaprComponentAuth{
				SecretStore: "",
			},
		},
		{
			auth: &DaprResourceAuth{
				SecretStore: to.Ptr(""),
			},
			expected: &rpv1.DaprComponentAuth{
				SecretStore: "",
			},
		},
	}

	for _, tt := range testCases {
		actual := toAuthDataModel(tt.auth)
		require.Equal(t, tt.expected, actual)
	}
}

func TestFromAuthDataModel(t *testing.T) {
	testCases := []struct {
		auth     *rpv1.DaprComponentAuth
		expected *DaprResourceAuth
	}{
		{
			auth:     nil,
			expected: nil,
		},
		{
			auth: &rpv1.DaprComponentAuth{
				SecretStore: "test-secretstore",
			},
			expected: &DaprResourceAuth{
				SecretStore: to.Ptr("test-secretstore"),
			},
		},
		{
			auth: &rpv1.DaprComponentAuth{
				SecretStore: "",
			},
			expected: &DaprResourceAuth{
				SecretStore: to.Ptr(""),
			},
		},
	}

	for _, tt := range testCases {
		actual := fromAuthDataModel(tt.auth)
		require.Equal(t, tt.expected, actual)
	}
}
