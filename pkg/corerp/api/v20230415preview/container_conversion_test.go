// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v20230415preview

import (
	"encoding/json"
	"testing"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
	"github.com/project-radius/radius/pkg/to"
	"github.com/project-radius/radius/test/testutil"

	"github.com/stretchr/testify/require"
)

func TestContainerConvertVersionedToDataModel(t *testing.T) {
	conversionTests := []struct {
		filename string
		err      error
		emptyExt bool
	}{
		{
			filename: "containerresource.json",
			err:      nil,
			emptyExt: false,
		},
		{
			filename: "containerresourceemptyext.json",
			err:      nil,
			emptyExt: true,
		},
		{
			filename: "containerresourceemptyext2.json",
			err:      nil,
			emptyExt: true,
		},
	}

	for _, tt := range conversionTests {
		t.Run(tt.filename, func(t *testing.T) {
			// arrange
			rawPayload := testutil.ReadFixture(tt.filename)
			r := &ContainerResource{}
			err := json.Unmarshal(rawPayload, r)
			require.NoError(t, err)

			// act
			dm, err := r.ConvertTo()

			if tt.err != nil {
				require.ErrorIs(t, err, tt.err)
			} else {

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
				require.Equal(t, to.Ptr[float32](5), tcpProbe.TCP.InitialDelaySeconds)
				require.Equal(t, int32(8080), tcpProbe.TCP.ContainerPort)
				require.Equal(t, []rpv1.OutputResource(nil), ct.Properties.Status.OutputResources)
				require.Equal(t, "2023-04-15-preview", ct.InternalMetadata.UpdatedAPIVersion)
				require.Equal(t, 3, len(ct.Properties.Extensions))

				require.Equal(t, []string{"/bin/sh"}, ct.Properties.Container.Command)
				require.Equal(t, []string{"-c", "while true; do echo hello; sleep 10;done"}, ct.Properties.Container.Args)
				require.Equal(t, "/app", ct.Properties.Container.WorkingDir)
				if tt.emptyExt {
					require.Equal(t, getTestContainerEmptyKuberenetesMetadataExt(t), ct.Properties.Extensions)
				} else {
					require.Equal(t, getTestContainerExtensions(t), ct.Properties.Extensions)
				}
			}
		})
	}
}

func TestContainerConvertDataModelToVersioned(t *testing.T) {

	conversionTests := []struct {
		filename string
		err      error
		emptyExt bool
	}{
		{
			filename: "containerresourcedatamodel.json",
			err:      nil,
			emptyExt: false,
		},
		{
			filename: "containerresourcedatamodelemptyext.json",
			err:      nil,
			emptyExt: true,
		},
	}

	for _, tt := range conversionTests {
		t.Run(tt.filename, func(t *testing.T) {
			rawPayload := testutil.ReadFixture(tt.filename)
			r := &datamodel.ContainerResource{}
			err := json.Unmarshal(rawPayload, r)
			require.NoError(t, err)

			// act
			versioned := &ContainerResource{}
			err = versioned.ConvertFrom(r)

			if tt.err != nil {
				require.ErrorIs(t, err, tt.err)
			} else {
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
				require.Equal(t, "kubernetesMetadata", *versioned.Properties.Extensions[2].GetExtension().Kind)
				require.Equal(t, 3, len(versioned.Properties.Extensions))
				require.Equal(t, to.SliceOfPtrs([]string{"/bin/sh"}...), versioned.Properties.Container.Command)
				require.Equal(t, to.SliceOfPtrs([]string{"-c", "while true; do echo hello; sleep 10;done"}...), versioned.Properties.Container.Args)
				require.Equal(t, to.Ptr("/app"), versioned.Properties.Container.WorkingDir)
			}
		})
	}

}

func TestContainerConvertVersionedToDataModelEmptyProtocol(t *testing.T) {
	// arrange
	rawPayload := testutil.ReadFixture("containerresourcenegativetest.json")
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
	require.Equal(t, []rpv1.OutputResource(nil), ct.Properties.Status.OutputResources)
	require.Equal(t, "2023-04-15-preview", ct.InternalMetadata.UpdatedAPIVersion)

	var commands []string
	var args []string
	require.Equal(t, commands, ct.Properties.Container.Command)
	require.Equal(t, args, ct.Properties.Container.Args)
	require.Equal(t, "", ct.Properties.Container.WorkingDir)

}

func TestContainerConvertFromValidation(t *testing.T) {
	validationTests := []struct {
		src v1.DataModelInterface
		err error
	}{
		{&fakeResource{}, v1.ErrInvalidModelConversion},
		{nil, v1.ErrInvalidModelConversion},
	}

	for _, tc := range validationTests {
		versioned := &ContainerResource{}
		err := versioned.ConvertFrom(tc.src)
		require.ErrorAs(t, tc.err, &err)
	}
}

func getTestContainerExtensions(t *testing.T) []datamodel.Extension {
	var replicavalue int32 = 2
	ptrreplicaval := &replicavalue
	extensions := []datamodel.Extension{
		{
			Kind: datamodel.ManualScaling,
			ManualScaling: &datamodel.ManualScalingExtension{
				Replicas: ptrreplicaval,
			},
		},
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

func getTestContainerEmptyKuberenetesMetadataExt(t *testing.T) []datamodel.Extension {
	var replicavalue int32 = 2
	ptrreplicaval := &replicavalue
	extensions := []datamodel.Extension{
		{
			Kind: datamodel.ManualScaling,
			ManualScaling: &datamodel.ManualScalingExtension{
				Replicas: ptrreplicaval,
			},
		},
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
			Kind: datamodel.KubernetesMetadata,
			KubernetesMetadata: &datamodel.KubeMetadataExtension{
				Annotations: map[string]string{},
				Labels:      map[string]string{},
			},
		},
	}

	return extensions
}
