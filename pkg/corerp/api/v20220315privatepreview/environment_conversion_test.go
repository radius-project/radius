// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v20220315privatepreview

import (
	"encoding/json"
	"io/ioutil"
	"testing"

	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/stretchr/testify/require"
)

func loadTestData(testfile string) []byte {
	d, err := ioutil.ReadFile("./testdata/" + testfile)
	if err != nil {
		return nil
	}
	return d
}

func TestConvertVersionedToDataModel(t *testing.T) {
	// arrange
	rawPayload := loadTestData("environmentresource.json")
	r := &EnvironmentResource{}
	err := json.Unmarshal(rawPayload, r)
	require.NoError(t, err)

	// act
	dm, err := r.ConvertTo()
	ct := dm.(*datamodel.Environment)

	// assert
	require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env0", ct.ID)
	require.Equal(t, "env0", ct.Name)
	require.Equal(t, "Applications.Core/environments", ct.Type)
	require.Equal(t, "kubernetes", string(ct.Properties.Compute.Kind))
	require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup/providers/Microsoft.ContainerService/managedClusters/radiusTestCluster", ct.Properties.Compute.ResourceID)

}

func TestConvertDataModelToVersioned(t *testing.T) {
	// arrange
	rawPayload := loadTestData("environmentresourcedatamodel.json")
	r := &datamodel.Environment{}
	err := json.Unmarshal(rawPayload, r)
	require.NoError(t, err)

	// act
	versioned := &EnvironmentResource{}
	err = versioned.ConvertFrom(r)

	// assert
	require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env0", r.ID)
	require.Equal(t, "env0", r.Name)
	require.Equal(t, "Applications.Core/environments", r.Type)
	require.Equal(t, "kubernetes", string(r.Properties.Compute.Kind))
	require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup/providers/Microsoft.ContainerService/managedClusters/radiusTestCluster", r.Properties.Compute.ResourceID)
}

func TestToEnvironmentComputeKindDataModel(t *testing.T) {
	kindTests := []struct {
		versioned EnvironmentComputeKind
		datamodel datamodel.EnvironmentComputeKind
	}{
		{EnvironmentComputeKindKubernetes, datamodel.KubernetesComputeKind},
		{"", datamodel.UnknownComputeKind},
	}

	for _, tt := range kindTests {
		sc := toEnvironmentComputeKindDataModel(&tt.versioned)
		require.Equal(t, tt.datamodel, sc)
	}
}

func TestFromEnvironmentComputeKindDataModel(t *testing.T) {
	kindTests := []struct {
		datamodel datamodel.EnvironmentComputeKind
		versioned EnvironmentComputeKind
	}{
		{datamodel.KubernetesComputeKind, EnvironmentComputeKindKubernetes},
		{datamodel.UnknownComputeKind, EnvironmentComputeKindKubernetes},
	}

	for _, tt := range kindTests {
		sc := fromEnvironmentComputeKind(tt.datamodel)
		require.Equal(t, tt.versioned, *sc)
	}
}
