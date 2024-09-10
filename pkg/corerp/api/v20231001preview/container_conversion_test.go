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
	"testing"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/test/testutil"
	"github.com/radius-project/radius/test/testutil/resourcetypeutil"

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
			filename: "containerresource-runtimes.json",
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
		{
			filename: "containerresource-manual.json",
			err:      nil,
			emptyExt: true,
		},
		{
			filename: "containerresource-nil-env-variables.json",
			err:      v1.NewClientErrInvalidRequest("Environment variable DB_USER has neither value nor secret value"),
			emptyExt: false,
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

				if tt.filename == "containerresource-manual.json" {
					require.Equal(t, datamodel.ContainerResourceProvisioningManual, ct.Properties.ResourceProvisioning)
					require.Equal(t, []datamodel.ResourceReference{{ID: "/planes/test/local/providers/Test.Namespace/testResources/test-resource"}}, ct.Properties.Resources)
					return
				}

				if tt.filename == "containerresource.json" {
					require.Equal(t, map[string]datamodel.EnvironmentVariable{
						"DB_USER": {
							Value: to.Ptr("DB_USER"),
						},
						"DB_PASSWORD": {
							ValueFrom: &datamodel.EnvironmentVariableReference{
								SecretRef: &datamodel.EnvironmentVariableSecretReference{
									Source: "secret.id",
									Key:    "DB_PASSWORD",
								},
							},
						},
					}, ct.Properties.Container.Env)
				}

				val, ok := ct.Properties.Connections["inventory"]
				require.True(t, ok)
				require.Equal(t, "inventory_route_id", val.Source)
				require.Equal(t, true, *val.DisableDefaultEnvVars)
				require.Equal(t, "azure", string(val.IAM.Kind))
				require.Equal(t, "read", val.IAM.Roles[0])
				require.Equal(t, "ghcr.io/radius-project/webapptutorial-todoapp", ct.Properties.Container.Image)
				tcpProbe := ct.Properties.Container.LivenessProbe
				require.Equal(t, datamodel.TCPHealthProbe, tcpProbe.Kind)
				require.Equal(t, to.Ptr[float32](5), tcpProbe.TCP.InitialDelaySeconds)
				require.Equal(t, int32(8080), tcpProbe.TCP.ContainerPort)
				require.Equal(t, []rpv1.OutputResource(nil), ct.Properties.Status.OutputResources)
				require.Equal(t, "2023-10-01-preview", ct.InternalMetadata.UpdatedAPIVersion)
				require.Equal(t, 3, len(ct.Properties.Extensions))

				require.Equal(t, []string{"/bin/sh"}, ct.Properties.Container.Command)
				require.Equal(t, []string{"-c", "while true; do echo hello; sleep 10;done"}, ct.Properties.Container.Args)
				require.Equal(t, "Always", ct.Properties.RestartPolicy)
				require.Equal(t, "/app", ct.Properties.Container.WorkingDir)
				if tt.emptyExt {
					require.Equal(t, getTestContainerEmptyKubernetesMetadataExt(), ct.Properties.Extensions)
				} else {
					require.Equal(t, getTestContainerExtensions(), ct.Properties.Extensions)
				}

				if r.Properties.Runtimes != nil {
					require.NotNil(t, ct.Properties.Runtimes.Kubernetes)
					require.NotEmpty(t, ct.Properties.Runtimes.Kubernetes.Base)
					require.Equal(t, *r.Properties.Runtimes.Kubernetes.Base, ct.Properties.Runtimes.Kubernetes.Base)
					require.Equal(t, "{\"containers\":[{\"name\":\"sidecar\"}],\"hostNetwork\":true}", ct.Properties.Runtimes.Kubernetes.Pod)
				}

			}
		})
	}
}

func TestContainerConvertDataModelToVersioned(t *testing.T) {
	conversionTests := []struct {
		filename string
		err      error
	}{
		{
			filename: "containerresourcedatamodel.json",
			err:      nil,
		},
		{
			filename: "containerresourcedatamodel-runtime.json",
			err:      nil,
		},
		{
			filename: "containerresourcedatamodelemptyext.json",
			err:      nil,
		},
		{
			filename: "containerresourcedatamodel-manual.json",
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
				require.NoError(t, err)
				require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/containers/container0", *versioned.ID)
				require.Equal(t, "container0", r.Name)
				require.Equal(t, "Applications.Core/containers", r.Type)
				require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup/providers/Applications.Core/applications/app0", *versioned.Properties.Application)

				if tt.filename == "containerresourcedatamodel-manual.json" {
					require.Equal(t, ContainerResourceProvisioning("manual"), *versioned.Properties.ResourceProvisioning)
					require.Equal(t, []*ResourceReference{{ID: to.Ptr("/planes/test/local/providers/Test.Namespace/testResources/test-resource")}}, versioned.Properties.Resources)
					return
				}

				if tt.filename == "containerresourcedatamodel.json" {
					require.Equal(t, map[string]datamodel.EnvironmentVariable{
						"DB_USER": {
							Value: to.Ptr("DB_USER"),
						},
						"DB_PASSWORD": {
							ValueFrom: &datamodel.EnvironmentVariableReference{
								SecretRef: &datamodel.EnvironmentVariableSecretReference{
									Source: "secret.id",
									Key:    "DB_PASSWORD",
								},
							},
						},
					}, r.Properties.Container.Env)
				}

				val, ok := r.Properties.Connections["inventory"]
				require.True(t, ok)
				require.Equal(t, "inventory_route_id", val.Source)
				require.Equal(t, "azure", string(val.IAM.Kind))
				require.Equal(t, "read", val.IAM.Roles[0])
				require.Equal(t, "ghcr.io/radius-project/webapptutorial-todoapp", *versioned.Properties.Container.Image)
				require.Equal(t, resourcetypeutil.MustPopulateResourceStatus(&ResourceStatus{}), versioned.Properties.Status)
				require.Equal(t, "kubernetesMetadata", *versioned.Properties.Extensions[2].GetExtension().Kind)
				require.Equal(t, 3, len(versioned.Properties.Extensions))
				require.Equal(t, to.SliceOfPtrs([]string{"/bin/sh"}...), versioned.Properties.Container.Command)
				require.Equal(t, to.SliceOfPtrs([]string{"-c", "while true; do echo hello; sleep 10;done"}...), versioned.Properties.Container.Args)
				require.Equal(t, to.Ptr("/app"), versioned.Properties.Container.WorkingDir)

				if r.Properties.Runtimes != nil {
					require.NotNil(t, versioned.Properties.Runtimes)
					require.NotEmpty(t, *versioned.Properties.Runtimes.Kubernetes.Base)
					require.Equal(t, r.Properties.Runtimes.Kubernetes.Base, *versioned.Properties.Runtimes.Kubernetes.Base)
					require.Equal(t, map[string]any{
						"containers": []any{
							map[string]any{
								"name": "sidecar",
							},
						},
						"hostNetwork": true,
					}, versioned.Properties.Runtimes.Kubernetes.Pod)
				}
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
	require.Equal(t, "ghcr.io/radius-project/webapptutorial-todoapp", ct.Properties.Container.Image)
	require.Equal(t, []rpv1.OutputResource(nil), ct.Properties.Status.OutputResources)
	require.Equal(t, "2023-10-01-preview", ct.InternalMetadata.UpdatedAPIVersion)

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
		{&resourcetypeutil.FakeResource{}, v1.ErrInvalidModelConversion},
		{nil, v1.ErrInvalidModelConversion},
	}

	for _, tc := range validationTests {
		versioned := &ContainerResource{}
		err := versioned.ConvertFrom(tc.src)
		require.ErrorAs(t, tc.err, &err)
	}
}

func getTestContainerExtensions() []datamodel.Extension {
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

func getTestContainerEmptyKubernetesMetadataExt() []datamodel.Extension {
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
