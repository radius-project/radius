// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v20220315privatepreview

import (
	"encoding/json"
	"testing"

	azto "github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	radiustesting "github.com/project-radius/radius/pkg/corerp/testing"
	"github.com/project-radius/radius/pkg/rp/outputresource"
	"github.com/stretchr/testify/require"
)

func TestContainerConvertVersionedToDataModel(t *testing.T) {
	// arrange
	rawPayload := radiustesting.ReadFixture("containerresource.json")
	r := &ContainerResource{}
	err := json.Unmarshal(rawPayload, r)
	require.NoError(t, err)

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
	require.Equal(t, true, *val.DisableDefaultEnvVars)
	require.Equal(t, "azure", string(val.IAM.Kind))
	require.Equal(t, "read", val.IAM.Roles[0])
	require.Equal(t, "radius.azurecr.io/webapptutorial-todoapp", ct.Properties.Container.Image)
	tcpProbe := ct.Properties.Container.LivenessProbe
	require.Equal(t, datamodel.TCPHealthProbe, tcpProbe.Kind)
	require.Equal(t, to.Float32Ptr(5), tcpProbe.TCP.InitialDelaySeconds)
	require.Equal(t, int32(8080), tcpProbe.TCP.ContainerPort)
	require.Equal(t, []outputresource.OutputResource(nil), ct.Properties.Status.OutputResources)
	require.Equal(t, "2022-03-15-privatepreview", ct.InternalMetadata.UpdatedAPIVersion)
	require.Equal(t, 3, len(ct.Properties.Extensions))
	require.Equal(t, GetContainerExtensions(t), ct.Properties.Extensions)

	require.Equal(t, []string{"/bin/sh"}, ct.Properties.Container.Command)
	require.Equal(t, []string{"-c", "while true; do echo hello; sleep 10;done"}, ct.Properties.Container.Args)
	require.Equal(t, "/app", ct.Properties.Container.WorkingDir)
}

func TestContainerConvertDataModelToVersioned(t *testing.T) {
	// arrange
	rawPayload := radiustesting.ReadFixture("containerresourcedatamodel.json")
	r := &datamodel.ContainerResource{}
	err := json.Unmarshal(rawPayload, r)
	require.NoError(t, err)

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
	require.Equal(t, "Deployment", versioned.Properties.Status.OutputResources[0]["LocalID"])
	require.Equal(t, "aks", versioned.Properties.Status.OutputResources[0]["Provider"])
	require.Equal(t, 3, len(versioned.Properties.Extensions))

	require.Equal(t, azto.SliceOfPtrs([]string{"/bin/sh"}...), versioned.Properties.Container.Command)
	require.Equal(t, azto.SliceOfPtrs([]string{"-c", "while true; do echo hello; sleep 10;done"}...), versioned.Properties.Container.Args)
	require.Equal(t, to.StringPtr("/app"), versioned.Properties.Container.WorkingDir)
}

func TestContainerConvertVersionedToDataModelEmptyProtocol(t *testing.T) {
	// arrange
	rawPayload := radiustesting.ReadFixture("containerresourcenegativetest.json")
	r := &ContainerResource{}
	err := json.Unmarshal(rawPayload, r)
	require.NoError(t, err)

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
	require.Equal(t, false, *val.DisableDefaultEnvVars)
	require.Equal(t, "read", val.IAM.Roles[0])
	require.Equal(t, "radius.azurecr.io/webapptutorial-todoapp", ct.Properties.Container.Image)
	require.Equal(t, []outputresource.OutputResource(nil), ct.Properties.Status.OutputResources)
	require.Equal(t, "2022-03-15-privatepreview", ct.InternalMetadata.UpdatedAPIVersion)

	var commands []string
	var args []string
	require.Equal(t, commands, ct.Properties.Container.Command)
	require.Equal(t, args, ct.Properties.Container.Args)
	require.Equal(t, "", ct.Properties.Container.WorkingDir)

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

func GetContainerExtensions(t *testing.T) []datamodel.Extension {
	var replicavalue int32 = 3
	ptrreplicaval := &replicavalue
	extensions := []datamodel.Extension{
		{
			Kind: datamodel.DaprSidecar,
			DaprSidecar: &datamodel.DaprSidecarExtension{
				AppID:    "app-id",
				AppPort:  80,
				Config:   "config",
				Protocol: "http",
				Provides: "provides",
			},
		},
		{
			Kind: datamodel.ManualScaling,
			ManualScaling: &datamodel.ManualScalingExtension{
				Replicas: ptrreplicaval,
			},
		},
		{
			Kind: datamodel.KubernetesMetadata,
			KubernetesMetadata: &datamodel.BaseKubernetesMetadataExtension{
				Annotations: map[string]string{
					"prometheus.io/scrape": "true",
					"prometheus.io/port":   "80",
				},
				Labels: map[string]string{
					"mbcp.pt/team":    "Credit",
					"mbcp.pt/contact": "radiususer",
				},
			},
		},
	}

	return extensions
}
