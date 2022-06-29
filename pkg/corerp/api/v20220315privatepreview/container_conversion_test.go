// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v20220315privatepreview

import (
	"encoding/json"
	"testing"

	"github.com/Azure/go-autorest/autorest/to"
	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/stretchr/testify/require"
)

func TestContainerConvertVersionedToDataModel(t *testing.T) {
	// arrange
	rawPayload := loadTestData("containerresource.json")
	r := &ContainerResource{}
	err := json.Unmarshal(rawPayload, r)
	require.NoError(t, err)
	resourceType := map[string]interface{}{"Provider": "aks", "Type": "containers"}

	// act
	dm, err := r.ConvertTo()

	// assert
	require.NoError(t, err)
	ct := dm.(*datamodel.ContainerResource)
	require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/containers/container0", ct.ID)
	require.Equal(t, "container0", ct.Name)
	require.Equal(t, "Applications.Core/containers", ct.Type)
	require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup/providers/Applications.Core/applications/app0", ct.Properties.Application)
	val, ok := ct.Properties.Connections["inventory"]
	require.True(t, ok)
	require.Equal(t, "inventory_route_id", val.Source)
	require.Equal(t, "azure", string(val.IAM.Kind))
	require.Equal(t, "read", val.IAM.Roles[0])
	require.Equal(t, "radius.azurecr.io/webapptutorial-todoapp", ct.Properties.Container.Image)
	tcpProbe := ct.Properties.Container.LivenessProbe
	require.Equal(t, datamodel.TCPHealthProbe, tcpProbe.Kind)
	require.Equal(t, to.Float32Ptr(5), tcpProbe.TCP.InitialDelaySeconds)
	require.Equal(t, int32(8080), tcpProbe.TCP.ContainerPort)
	require.Equal(t, "Deployment", ct.Properties.Status.OutputResources[0]["LocalID"])
	require.Equal(t, resourceType, ct.Properties.Status.OutputResources[0]["ResourceType"])
	require.Equal(t, "2022-03-15-privatepreview", ct.InternalMetadata.UpdatedAPIVersion)
}

func TestContainerConvertDataModelToVersioned(t *testing.T) {
	// arrange
	rawPayload := loadTestData("containerresourcedatamodel.json")
	r := &datamodel.ContainerResource{}
	err := json.Unmarshal(rawPayload, r)
	require.NoError(t, err)
	resourceType := map[string]interface{}{"Provider": "aks", "Type": "containers"}

	// act
	versioned := &ContainerResource{}
	err = versioned.ConvertFrom(r)

	// assert
	require.NoError(t, err)
	require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/containers/container0", r.ID)
	require.Equal(t, "container0", r.Name)
	require.Equal(t, "Applications.Core/containers", r.Type)
	require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup/providers/Applications.Core/applications/app0", r.Properties.Application)
	val, ok := r.Properties.Connections["inventory"]
	require.True(t, ok)
	require.Equal(t, "inventory_route_id", val.Source)
	require.Equal(t, "azure", string(val.IAM.Kind))
	require.Equal(t, "read", val.IAM.Roles[0])
	require.Equal(t, "radius.azurecr.io/webapptutorial-todoapp", r.Properties.Container.Image)
	require.Equal(t, "Deployment", r.Properties.Status.OutputResources[0]["LocalID"])
	require.Equal(t, resourceType, r.Properties.Status.OutputResources[0]["ResourceType"])
}

func TestContainerConvertFromValidation(t *testing.T) {
	validationTests := []struct {
		src conv.DataModelInterface
		err error
	}{
		{&fakeResource{}, conv.ErrInvalidModelConversion},
		{nil, conv.ErrInvalidModelConversion},
	}

	for _, tc := range validationTests {
		versioned := &ContainerResource{}
		err := versioned.ConvertFrom(tc.src)
		require.ErrorAs(t, tc.err, &err)
	}
}
