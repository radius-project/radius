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

package container

import (
	"fmt"
	"testing"

	apiv1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/corerp/handlers"
	"github.com/project-radius/radius/pkg/corerp/renderers"
	azrenderer "github.com/project-radius/radius/pkg/corerp/renderers/container/azure"
	azvolrenderer "github.com/project-radius/radius/pkg/corerp/renderers/volume/azure"
	"github.com/project-radius/radius/pkg/kubernetes"
	"github.com/project-radius/radius/pkg/resourcemodel"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
	"github.com/project-radius/radius/pkg/to"
	"github.com/project-radius/radius/pkg/ucp/resources"
	resources_azure "github.com/project-radius/radius/pkg/ucp/resources/azure"
	resources_kubernetes "github.com/project-radius/radius/pkg/ucp/resources/kubernetes"
	"github.com/project-radius/radius/test/testcontext"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	applicationName       = "test-app"
	applicationResourceID = "/subscriptions/test-sub-id/resourceGroups/test-rg/providers/Applications.Core/applications/test-app"
	applicationPath       = "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup/providers/Applications.Core/applications/"
	resourceName          = "test-container"
	envVarName1           = "TEST_VAR_1"
	envVarValue1          = "TEST_VALUE_1"
	envVarName2           = "TEST_VAR_2"
	envVarValue2          = "81"
	secretName            = "test-container"

	tempVolName      = "TempVolume"
	tempVolMountPath = "/tmpfs"
	testResourceID   = "/subscriptions/sub/resourceGroups/rg/providers/Microsoft.KeyVault/vaults/azure-kv"

	// User Inputs for testing labels and annotations
	appAnnotationKey1 = "app.ann1"
	appAnnotationKey2 = "app.ann2"
	appAnnotationVal1 = "app.annval1"
	appAnnotationVal2 = "app.annval2"

	appLabelKey1 = "app.lbl1"
	appLabelKey2 = "app.lbl2"
	appLabelVal1 = "env.lblval1"
	appLabelVal2 = "env.lblval2"

	overrideKey1 = "test.ann1"
	overrideKey2 = "test.lbl1"
	overrideVal1 = "override.app.annval1"
	overrideVal2 = "override.app.lblval1"
)

var (
	testEnvironmentOptions = renderers.EnvironmentOptions{
		Namespace: "default",
		CloudProviders: &datamodel.Providers{
			Azure: datamodel.ProvidersAzure{
				Scope: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup",
			},
		},
		Identity: &rpv1.IdentitySettings{
			Kind:       rpv1.AzureIdentityWorkload,
			OIDCIssuer: "https://radiusoidc/00000000-0000-0000-0000-000000000000",
		},
	}
)

func makeResource(t *testing.T, properties datamodel.ContainerProperties) *datamodel.ContainerResource {
	resource := datamodel.ContainerResource{
		BaseResource: apiv1.BaseResource{
			TrackedResource: apiv1.TrackedResource{
				ID:   "/subscriptions/test-sub-id/resourceGroups/test-group/providers/Applications.Core/containers/test-container",
				Name: resourceName,
				Type: "Applications.Core/containers",
			},
		},
		Properties: properties,
	}
	return &resource
}

func makeAzureResourceID(t *testing.T, resourceType string, resourceName string) resources.ID {
	id, err := resources.ParseResource(resources.MakeRelativeID(
		[]resources.ScopeSegment{
			{Type: "subscriptions", Name: "test-subscription"},
			{Type: "resourceGroups", Name: "test-resourcegroup"},
		},
		[]resources.TypeSegment{
			{Type: resourceType, Name: resourceName},
		}, nil))
	require.NoError(t, err)

	return id
}

func makeRadiusResourceID(t *testing.T, resourceType string, resourceName string) resources.ID {
	id, err := resources.ParseResource(resources.MakeRelativeID(
		[]resources.ScopeSegment{
			{Type: "radius", Name: "local"},
			{Type: "resourceGroups", Name: "test-resourcegroup"},
		},
		[]resources.TypeSegment{
			{
				Type: resourceType,
				Name: resourceName,
			},
		},
		nil))
	require.NoError(t, err)

	return id
}

func Test_GetDependencyIDs_Success(t *testing.T) {
	testStorageResourceID := "/subscriptions/test-sub-id/resourceGroups/test-rg/providers/Microsoft.Storage/storageaccounts/testaccount/fileservices/default/shares/testShareName"
	testAzureResourceID := makeAzureResourceID(t, "Microsoft.ServiceBus/namespaces", "testAzureResource")
	properties := datamodel.ContainerProperties{
		BasicResourceProperties: rpv1.BasicResourceProperties{
			Application: applicationResourceID,
		},
		Connections: map[string]datamodel.ConnectionProperties{
			"A": {
				Source: makeRadiusResourceID(t, "Applications.Core/httpRoutes", "A").String(),
			},
			"B": {
				Source: makeRadiusResourceID(t, "Applications.Core/httpRoutes", "B").String(),
				IAM: datamodel.IAMProperties{
					Kind:  datamodel.KindHTTP,
					Roles: []string{"administrator"},
				},
			},
			"testAzureConnection": {
				Source: testAzureResourceID.String(),
				IAM: datamodel.IAMProperties{
					Kind:  datamodel.KindAzure,
					Roles: []string{"administrator"},
				},
			},
			"testNonRadiusConnectionWithoutIAM": {
				Source: testAzureResourceID.String(),
			},
		},
		Container: datamodel.Container{
			Image: "someimage:latest",
			Ports: map[string]datamodel.ContainerPort{
				"web": {
					ContainerPort: 5000,
					Provides:      makeRadiusResourceID(t, "Applications.Core/httpRoutes", "C").String(),
				},
			},
			Volumes: map[string]datamodel.VolumeProperties{
				"vol1": {
					Kind: datamodel.Persistent,
					Persistent: &datamodel.PersistentVolume{
						VolumeBase: datamodel.VolumeBase{
							MountPath: "/tmpfs",
						},
						Source: testStorageResourceID,
					},
				},
			},
		},
	}
	resource := makeResource(t, properties)

	ctx := testcontext.New(t)

	renderer := Renderer{}
	radiusResourceIDs, azureResourceIDs, err := renderer.GetDependencyIDs(ctx, resource)
	require.NoError(t, err)
	require.Len(t, radiusResourceIDs, 3)
	require.Len(t, azureResourceIDs, 1)

	expectedRadiusResourceIDs := []resources.ID{
		makeRadiusResourceID(t, "Applications.Core/httpRoutes", "A"),
		makeRadiusResourceID(t, "Applications.Core/httpRoutes", "B"),
		makeRadiusResourceID(t, "Applications.Core/httpRoutes", "C"),
	}
	require.ElementsMatch(t, expectedRadiusResourceIDs, radiusResourceIDs)

	expectedAzureResourceIDs := []resources.ID{
		testAzureResourceID,
	}
	require.ElementsMatch(t, expectedAzureResourceIDs, azureResourceIDs)
}

func Test_GetDependencyIDs_InvalidId(t *testing.T) {
	properties := datamodel.ContainerProperties{
		Connections: map[string]datamodel.ConnectionProperties{
			"A": {
				Source: "not a resource id obviously...",
				IAM: datamodel.IAMProperties{
					Kind: datamodel.KindHTTP,
				},
			},
		},
		Container: datamodel.Container{
			Image: "someimage:latest",
		},
	}
	resource := makeResource(t, properties)

	ctx := testcontext.New(t)
	renderer := Renderer{}
	ids, azureIDs, err := renderer.GetDependencyIDs(ctx, resource)
	require.Error(t, err)
	require.Empty(t, ids)
	require.Empty(t, azureIDs)
}

func Test_GetDependencyIDs_InvalidAzureResourceId(t *testing.T) {
	ctx := testcontext.New(t)

	properties := datamodel.ContainerProperties{
		// Simulating error code path
		// Revert this once TODO: https://github.com/project-radius/core-team/issues/238 is done.
		Connections: map[string]datamodel.ConnectionProperties{
			"AzureResourceTest": {
				Source: "//subscriptions/test-sub-id/providers/Microsoft.ServiceBus/namespaces/testNamespace",
				IAM: datamodel.IAMProperties{
					Kind: datamodel.KindAzure,
				},
			},
		},
		Container: datamodel.Container{
			Image: "test-image:latest",
		},
	}
	resource := makeResource(t, properties)

	renderer := Renderer{}
	ids, azureIDs, err := renderer.GetDependencyIDs(ctx, resource)
	require.Error(t, err)
	require.Equal(t, err.(*apiv1.ErrClientRP).Code, apiv1.CodeInvalid)
	require.Equal(t, err.(*apiv1.ErrClientRP).Message, "invalid source: //subscriptions/test-sub-id/providers/Microsoft.ServiceBus/namespaces/testNamespace. Must be either a URL or a valid resourceID")
	require.Empty(t, ids)
	require.Empty(t, azureIDs)
}

// This test verifies most of the 'basics' of rendering a deployment. These verifications are not
// repeated in other tests becase the code is simple for these cases.
//
// If you add minor features, add them here.
func Test_Render_Basic(t *testing.T) {
	properties := datamodel.ContainerProperties{
		BasicResourceProperties: rpv1.BasicResourceProperties{
			Application: applicationResourceID,
		},
		Container: datamodel.Container{
			Image: "someimage:latest",
			Env: map[string]string{
				envVarName1: envVarValue1,
				envVarName2: envVarValue2,
			},
		},
	}
	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{}

	ctx := testcontext.New(t)
	renderer := Renderer{}
	output, err := renderer.Render(ctx, resource, renderers.RenderOptions{Dependencies: dependencies})
	require.NoError(t, err)
	require.Empty(t, output.ComputedValues)
	require.Empty(t, output.SecretValues)

	labels := kubernetes.MakeDescriptiveLabels(applicationName, resource.Name, resource.ResourceTypeName())
	matchLabels := kubernetes.MakeSelectorLabels(applicationName, resource.Name)

	t.Run("verify deployment", func(t *testing.T) {
		deployment, outputResource := kubernetes.FindDeployment(output.Resources)
		require.NotNil(t, deployment)

		expected := rpv1.NewKubernetesOutputResource(rpv1.LocalIDDeployment, deployment, deployment.ObjectMeta)
		expected.CreateResource.Dependencies = []string{rpv1.LocalIDKubernetesRole, rpv1.LocalIDKubernetesRoleBinding}
		require.Equal(t, outputResource, expected)

		// Only real thing to verify here is the image and the labels
		require.Equal(t, labels, deployment.Labels)
		require.Equal(t, labels, deployment.Spec.Template.Labels)
		require.Equal(t, matchLabels, deployment.Spec.Selector.MatchLabels)

		// See https://github.com/project-radius/radius/issues/3002
		//
		// We disable service links and rely on Radius' connections feature instead.
		require.NotNil(t, deployment.Spec.Template.Spec.EnableServiceLinks)
		require.False(t, *deployment.Spec.Template.Spec.EnableServiceLinks)

		require.Len(t, deployment.Spec.Template.Spec.Containers, 1)

		container := deployment.Spec.Template.Spec.Containers[0]
		require.Equal(t, resourceName, container.Name)
		require.Equal(t, properties.Container.Image, container.Image)
		require.Empty(t, container.ImagePullPolicy)

		var commands []string
		var args []string
		require.Equal(t, commands, container.Command)
		require.Equal(t, args, container.Args)
		require.Equal(t, "", container.WorkingDir)

		expectedEnv := []corev1.EnvVar{
			{Name: envVarName1, Value: envVarValue1},
			{Name: envVarName2, Value: envVarValue2},
		}
		require.Equal(t, expectedEnv, container.Env)

	})
	require.Len(t, output.Resources, 3)
}

func Test_Render_WithCommandArgsWorkingDir(t *testing.T) {
	properties := datamodel.ContainerProperties{
		BasicResourceProperties: rpv1.BasicResourceProperties{
			Application: applicationResourceID,
		},
		Container: datamodel.Container{
			Image: "someimage:latest",
			Env: map[string]string{
				envVarName1: envVarValue1,
				envVarName2: envVarValue2,
			},
			Command:    []string{"command1", "command2"},
			Args:       []string{"arg1", "arg2"},
			WorkingDir: "/some/path",
		},
	}
	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{}

	ctx := testcontext.New(t)
	renderer := Renderer{}
	output, err := renderer.Render(ctx, resource, renderers.RenderOptions{Dependencies: dependencies})
	require.NoError(t, err)
	require.Empty(t, output.ComputedValues)
	require.Empty(t, output.SecretValues)

	labels := kubernetes.MakeDescriptiveLabels(applicationName, resource.Name, resource.ResourceTypeName())
	matchLabels := kubernetes.MakeSelectorLabels(applicationName, resource.Name)

	t.Run("verify deployment", func(t *testing.T) {
		deployment, outputResource := kubernetes.FindDeployment(output.Resources)
		require.NotNil(t, deployment)

		expected := rpv1.NewKubernetesOutputResource(rpv1.LocalIDDeployment, deployment, deployment.ObjectMeta)
		expected.CreateResource.Dependencies = []string{rpv1.LocalIDKubernetesRole, rpv1.LocalIDKubernetesRoleBinding}
		require.Equal(t, outputResource, expected)

		// Only real thing to verify here is the image and the labels
		require.Equal(t, labels, deployment.Labels)
		require.Equal(t, labels, deployment.Spec.Template.Labels)
		require.Equal(t, matchLabels, deployment.Spec.Selector.MatchLabels)

		require.Len(t, deployment.Spec.Template.Spec.Containers, 1)

		container := deployment.Spec.Template.Spec.Containers[0]
		require.Equal(t, resourceName, container.Name)
		require.Equal(t, properties.Container.Image, container.Image)
		require.Empty(t, container.ImagePullPolicy)
		require.Equal(t, []string{"command1", "command2"}, container.Command)
		require.Equal(t, []string{"arg1", "arg2"}, container.Args)
		require.Equal(t, "/some/path", container.WorkingDir)

		expectedEnv := []corev1.EnvVar{
			{Name: envVarName1, Value: envVarValue1},
			{Name: envVarName2, Value: envVarValue2},
		}
		require.Equal(t, expectedEnv, container.Env)

	})
	require.Len(t, output.Resources, 3)
}

func Test_Render_PortWithoutRoute(t *testing.T) {
	properties := datamodel.ContainerProperties{
		BasicResourceProperties: rpv1.BasicResourceProperties{
			Application: applicationResourceID,
		},
		Container: datamodel.Container{
			Image: "someimage:latest",
			Ports: map[string]datamodel.ContainerPort{
				"web": {
					ContainerPort: 5000,
					Protocol:      datamodel.ProtocolTCP,
				},
			},
		},
	}
	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{}

	ctx := testcontext.New(t)
	renderer := Renderer{}
	output, err := renderer.Render(ctx, resource, renderers.RenderOptions{Dependencies: dependencies})
	require.NoError(t, err)
	require.Len(t, output.ComputedValues, 0)
	require.Empty(t, output.SecretValues)

	t.Run("verify deployment", func(t *testing.T) {
		deployment, _ := kubernetes.FindDeployment(output.Resources)
		require.NotNil(t, deployment)

		require.Len(t, deployment.Spec.Template.Spec.Containers, 1)
		container := deployment.Spec.Template.Spec.Containers[0]

		require.Len(t, container.Ports, 1)
		port := container.Ports[0]

		expected := corev1.ContainerPort{
			ContainerPort: 5000,
			Protocol:      corev1.ProtocolTCP,
		}
		require.Equal(t, expected, port)

	})
	require.Len(t, output.Resources, 4)
}

func Test_Render_PortConnectedToRoute(t *testing.T) {
	properties := datamodel.ContainerProperties{
		BasicResourceProperties: rpv1.BasicResourceProperties{
			Application: applicationResourceID,
		},
		Container: datamodel.Container{
			Image: "someimage:latest",
			Ports: map[string]datamodel.ContainerPort{
				"web": {
					ContainerPort: 5000,
					Protocol:      datamodel.ProtocolTCP,
					Provides:      makeRadiusResourceID(t, "Applications.Core/httpRoutes", "A").String(),
				},
			},
		},
	}
	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{}

	ctx := testcontext.New(t)
	renderer := Renderer{}
	output, err := renderer.Render(ctx, resource, renderers.RenderOptions{Dependencies: dependencies})
	require.NoError(t, err)
	require.Empty(t, output.ComputedValues)
	require.Empty(t, output.SecretValues)

	labels := kubernetes.MakeDescriptiveLabels(applicationName, resource.Name, resource.ResourceTypeName())
	podLabels := kubernetes.MakeDescriptiveLabels(applicationName, resource.Name, resource.ResourceTypeName())
	podLabels["radius.dev/route-httproutes-a"] = "true"

	t.Run("verify deployment", func(t *testing.T) {
		deployment, _ := kubernetes.FindDeployment(output.Resources)
		require.NotNil(t, deployment)

		require.Len(t, deployment.Spec.Template.Spec.Containers, 1)
		container := deployment.Spec.Template.Spec.Containers[0]

		// Labels are somewhat specialized when a route is involved
		require.Equal(t, labels, deployment.Labels)
		require.Equal(t, podLabels, deployment.Spec.Template.Labels)

		require.Len(t, container.Ports, 1)
		port := container.Ports[0]

		routeID := makeRadiusResourceID(t, "Applications.Core/httpRoutes", "A")

		expected := corev1.ContainerPort{
			Name:          kubernetes.GetShortenedTargetPortName("httpRoutes" + routeID.Name()),
			ContainerPort: 5000,
			Protocol:      corev1.ProtocolTCP,
		}
		require.Equal(t, expected, port)
	})
	require.Len(t, output.Resources, 3)
}

func Test_Render_Connections(t *testing.T) {
	properties := datamodel.ContainerProperties{
		BasicResourceProperties: rpv1.BasicResourceProperties{
			Application: applicationResourceID,
		},
		Connections: map[string]datamodel.ConnectionProperties{
			"A": {
				Source: makeRadiusResourceID(t, "SomeProvider/ResourceType", "A").String(),
				IAM: datamodel.IAMProperties{
					Kind: datamodel.KindHTTP,
				},
			},
		},
		Container: datamodel.Container{
			Image: "someimage:latest",
			Env: map[string]string{
				envVarName1: envVarValue1,
				envVarName2: envVarValue2,
			},
		},
	}
	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{
		(makeRadiusResourceID(t, "SomeProvider/ResourceType", "A").String()): {
			ResourceID: makeRadiusResourceID(t, "SomeProvider/ResourceType", "A"),
			ComputedValues: map[string]any{
				"ComputedKey1": "ComputedValue1",
				"ComputedKey2": 82,
			},
		},
	}

	ctx := testcontext.New(t)
	renderer := Renderer{}
	output, err := renderer.Render(ctx, resource, renderers.RenderOptions{Dependencies: dependencies, Environment: renderers.EnvironmentOptions{Namespace: "default"}})
	require.NoError(t, err)
	require.Empty(t, output.ComputedValues)
	require.Empty(t, output.SecretValues)

	labels := kubernetes.MakeDescriptiveLabels(applicationName, resource.Name, resource.ResourceTypeName())

	t.Run("verify deployment", func(t *testing.T) {
		deployment, _ := kubernetes.FindDeployment(output.Resources)
		require.NotNil(t, deployment)

		require.Len(t, deployment.Spec.Template.Spec.Containers, 1)

		container := deployment.Spec.Template.Spec.Containers[0]
		require.Equal(t, resourceName, container.Name)
		require.Equal(t, properties.Container.Image, container.Image)
		require.Empty(t, container.ImagePullPolicy)

		expectedEnv := []corev1.EnvVar{
			{
				Name: "CONNECTION_A_COMPUTEDKEY1",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: secretName,
						},
						Key: "CONNECTION_A_COMPUTEDKEY1",
					},
				},
			},
			{
				Name: "CONNECTION_A_COMPUTEDKEY2",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: secretName,
						},
						Key: "CONNECTION_A_COMPUTEDKEY2",
					},
				},
			},
			{Name: envVarName1, Value: envVarValue1},
			{Name: envVarName2, Value: envVarValue2},
		}
		require.Equal(t, expectedEnv, container.Env)
	})

	t.Run("verify secret", func(t *testing.T) {
		secret, outputResource := kubernetes.FindSecret(output.Resources)
		require.NotNil(t, secret)

		expectedOutputResource := rpv1.NewKubernetesOutputResource(rpv1.LocalIDSecret, secret, secret.ObjectMeta)
		require.Equal(t, outputResource, expectedOutputResource)

		require.Equal(t, secretName, secret.Name)
		require.Equal(t, "default", secret.Namespace)
		require.Equal(t, labels, secret.Labels)
		require.Empty(t, secret.Annotations)

		require.Equal(t, outputResource.LocalID, rpv1.LocalIDSecret)
		require.Len(t, secret.Data, 2)
		require.Equal(t, "ComputedValue1", string(secret.Data["CONNECTION_A_COMPUTEDKEY1"]))
		require.Equal(t, "82", string(secret.Data["CONNECTION_A_COMPUTEDKEY2"]))
	})
	require.Len(t, output.Resources, 4)
}

func Test_RenderConnections_DisableDefaultEnvVars(t *testing.T) {
	properties := datamodel.ContainerProperties{
		BasicResourceProperties: rpv1.BasicResourceProperties{
			Application: applicationResourceID,
		},
		Connections: map[string]datamodel.ConnectionProperties{
			"A": {
				Source:                makeRadiusResourceID(t, "SomeProvider/ResourceType", "A").String(),
				DisableDefaultEnvVars: to.Ptr(true),
				IAM: datamodel.IAMProperties{
					Kind: datamodel.KindHTTP,
				},
			},
		},
		Container: datamodel.Container{
			Image: "someimage:latest",
		},
	}
	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{
		(makeRadiusResourceID(t, "SomeProvider/ResourceType", "A").String()): {
			ResourceID: makeRadiusResourceID(t, "SomeProvider/ResourceType", "A"),
			ComputedValues: map[string]any{
				"ComputedKey1": "ComputedValue1",
				"ComputedKey2": 82,
			},
		},
	}

	ctx := testcontext.New(t)
	renderer := Renderer{}
	output, err := renderer.Render(ctx, resource, renderers.RenderOptions{Dependencies: dependencies, Environment: renderers.EnvironmentOptions{Namespace: "default"}})
	require.NoError(t, err)

	deployment, _ := kubernetes.FindDeployment(output.Resources)
	require.NotNil(t, deployment)

	require.Len(t, deployment.Spec.Template.Spec.Containers, 1)

	container := deployment.Spec.Template.Spec.Containers[0]
	require.Equal(t, resourceName, container.Name)
	require.Equal(t, properties.Container.Image, container.Image)
	require.Empty(t, container.ImagePullPolicy)

	require.Nil(t, container.Env)
}

// This test is testing that we hash the connection data and include it in the output. We don't care about the content
// of the hash, just that it can change when the data changes.
func Test_Render_Connections_SecretsGetHashed(t *testing.T) {
	properties := datamodel.ContainerProperties{
		BasicResourceProperties: rpv1.BasicResourceProperties{
			Application: applicationResourceID,
		},
		Connections: map[string]datamodel.ConnectionProperties{
			"A": {
				Source: makeRadiusResourceID(t, "SomeProvider/ResourceType", "A").String(),
				IAM: datamodel.IAMProperties{
					Kind: datamodel.KindHTTP,
				},
			},
		},
		Container: datamodel.Container{
			Image: "someimage:latest",
			Env: map[string]string{
				envVarName1: envVarValue1,
				envVarName2: envVarValue2,
			},
		},
	}
	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{
		(makeRadiusResourceID(t, "SomeProvider/ResourceType", "A").String()): {
			ResourceID: makeRadiusResourceID(t, "SomeProvider/ResourceType", "A"),
			ComputedValues: map[string]any{
				"ComputedKey1": "ComputedValue1",
				"ComputedKey2": 82,
			},
		},
	}

	ctx := testcontext.New(t)
	renderer := Renderer{}
	output, err := renderer.Render(ctx, resource, renderers.RenderOptions{Dependencies: dependencies, Environment: renderers.EnvironmentOptions{Namespace: "default"}})
	require.NoError(t, err)
	require.Empty(t, output.ComputedValues)
	require.Empty(t, output.SecretValues)

	deployment, _ := kubernetes.FindDeployment(output.Resources)
	require.NotNil(t, deployment)

	require.Contains(t, deployment.Spec.Template.Annotations, kubernetes.AnnotationSecretHash)
	hash1 := deployment.Spec.Template.Annotations[kubernetes.AnnotationSecretHash]

	// Update and render again
	dependencies[makeRadiusResourceID(t, "SomeProvider/ResourceType", "A").String()].ComputedValues["ComputedKey1"] = "new value"

	output, err = renderer.Render(ctx, resource, renderers.RenderOptions{Dependencies: dependencies, Environment: renderers.EnvironmentOptions{Namespace: "default"}})
	require.NoError(t, err)
	deployment, _ = kubernetes.FindDeployment(output.Resources)
	require.NotNil(t, deployment)

	require.Contains(t, deployment.Spec.Template.Annotations, kubernetes.AnnotationSecretHash)
	hash2 := deployment.Spec.Template.Annotations[kubernetes.AnnotationSecretHash]

	require.NotEqual(t, hash1, hash2)
}

func Test_Render_ConnectionWithRoleAssignment(t *testing.T) {
	properties := datamodel.ContainerProperties{
		BasicResourceProperties: rpv1.BasicResourceProperties{
			Application: applicationResourceID,
		},
		Connections: map[string]datamodel.ConnectionProperties{
			"A": {
				Source: makeAzureResourceID(t, "SomeProvider/ResourceType", "A").String(),
				IAM: datamodel.IAMProperties{
					Kind: datamodel.KindHTTP,
				},
			},
		},
		Container: datamodel.Container{
			Image: "someimage:latest",
		},
	}
	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{
		(makeAzureResourceID(t, "SomeProvider/ResourceType", "A").String()): {
			ResourceID: makeAzureResourceID(t, "SomeProvider/ResourceType", "A"),
			ComputedValues: map[string]any{
				"ComputedKey1": "ComputedValue1",
				"ComputedKey2": 82,
			},
			OutputResources: map[string]resources.ID{
				// This is the resource that the role assignments target!
				"TargetLocalID": makeAzureResourceID(t, "SomeProvider/TargetResourceType", "TargetResource"),
			},
		},
	}

	renderer := Renderer{
		RoleAssignmentMap: map[datamodel.IAMKind]RoleAssignmentData{
			datamodel.KindHTTP: {
				LocalID:   "TargetLocalID",
				RoleNames: []string{"TestRole1", "TestRole2"},
			},
		},
	}
	ctx := testcontext.New(t)
	output, err := renderer.Render(ctx, resource, renderers.RenderOptions{Dependencies: dependencies, Environment: testEnvironmentOptions})
	require.NoError(t, err)
	require.Len(t, output.ComputedValues, 2)
	require.Equal(t, output.ComputedValues[handlers.IdentityProperties].Value.(*rpv1.IdentitySettings).Kind, rpv1.AzureIdentityWorkload)
	require.Equal(t, output.ComputedValues[handlers.UserAssignedIdentityIDKey].PropertyReference, handlers.UserAssignedIdentityIDKey)
	require.Empty(t, output.SecretValues)
	require.Len(t, output.Resources, 9)

	resourceMap := outputResourcesToResourceTypeMap(output.Resources)

	// We're just verifying the role assignments and related things, we'll ignore kubernetes types.
	matches := resourceMap[resources_kubernetes.ResourceTypeDeployment]
	require.Equal(t, 1, len(matches))

	matches = resourceMap[resources_kubernetes.ResourceTypeSecret]
	require.Equal(t, 1, len(matches))

	matches = resourceMap[resources_azure.ResourceTypeAuthorizationRoleAssignment]
	require.Equal(t, 2, len(matches))
	expected := []rpv1.OutputResource{
		{
			LocalID: rpv1.NewLocalID(rpv1.LocalIDRoleAssignmentPrefix, makeAzureResourceID(t, "SomeProvider/TargetResourceType", "TargetResource").String()+"TestRole1"),
			CreateResource: &rpv1.Resource{
				ResourceType: resourcemodel.ResourceType{
					Type:     resources_azure.ResourceTypeAuthorizationRoleAssignment,
					Provider: resourcemodel.ProviderAzure,
				},
				Data: map[string]string{
					handlers.RoleNameKey:         "TestRole1",
					handlers.RoleAssignmentScope: makeAzureResourceID(t, "SomeProvider/TargetResourceType", "TargetResource").String(),
				},
				Dependencies: []string{rpv1.LocalIDUserAssignedManagedIdentity},
			},
		},
		{
			LocalID: rpv1.NewLocalID(rpv1.LocalIDRoleAssignmentPrefix, makeAzureResourceID(t, "SomeProvider/TargetResourceType", "TargetResource").String()+"TestRole2"),
			CreateResource: &rpv1.Resource{
				ResourceType: resourcemodel.ResourceType{
					Type:     resources_azure.ResourceTypeAuthorizationRoleAssignment,
					Provider: resourcemodel.ProviderAzure,
				},
				Data: map[string]string{
					handlers.RoleNameKey:         "TestRole2",
					handlers.RoleAssignmentScope: makeAzureResourceID(t, "SomeProvider/TargetResourceType", "TargetResource").String(),
				},
				Dependencies: []string{rpv1.LocalIDUserAssignedManagedIdentity},
			},
		},
	}
	require.ElementsMatch(t, expected, matches)

	matches = resourceMap[resources_azure.ResourceTypeManagedIdentityUserAssignedManagedIdentity]
	require.Equal(t, 1, len(matches))

	expected = []rpv1.OutputResource{
		{

			LocalID: rpv1.LocalIDUserAssignedManagedIdentity,
			CreateResource: &rpv1.Resource{
				ResourceType: resourcemodel.ResourceType{
					Type:     resources_azure.ResourceTypeManagedIdentityUserAssignedManagedIdentity,
					Provider: resourcemodel.ProviderAzure,
				},
				Data: map[string]string{
					"userassignedidentityname":           "test-app-test-container",
					"userassignedidentitysubscriptionid": "00000000-0000-0000-0000-000000000000",
					"userassignedidentityresourcegroup":  "testGroup",
				},
			},
		},
	}
	require.ElementsMatch(t, expected, matches)

	matches = resourceMap[resources_azure.ResourceTypeManagedIdentityUserAssignedManagedIdentityFederatedIdentityCredential]
	require.Equal(t, 1, len(matches))

	expected = []rpv1.OutputResource{
		{
			LocalID: rpv1.LocalIDFederatedIdentity,
			CreateResource: &rpv1.Resource{
				ResourceType: resourcemodel.ResourceType{
					Type:     resources_azure.ResourceTypeManagedIdentityUserAssignedManagedIdentityFederatedIdentityCredential,
					Provider: resourcemodel.ProviderAzure,
				},
				Data: map[string]string{
					"federatedidentityname":    "test-container",
					"federatedidentitysubject": "system:serviceaccount:default:test-container",
					"federatedidentityissuer":  "https://radiusoidc/00000000-0000-0000-0000-000000000000",
				},
				Dependencies: []string{rpv1.LocalIDUserAssignedManagedIdentity},
			},
		}}
	require.ElementsMatch(t, expected, matches)

	matches = resourceMap[resources_kubernetes.ResourceTypeServiceAccount]
	require.Equal(t, 1, len(matches))
}

func Test_Render_AzureConnection(t *testing.T) {
	testARMID := makeAzureResourceID(t, "SomeProvider/ResourceType", "test-azure-resource").String()
	expectedRole := "administrator"
	properties := datamodel.ContainerProperties{
		BasicResourceProperties: rpv1.BasicResourceProperties{
			Application: applicationResourceID,
		},
		Connections: map[string]datamodel.ConnectionProperties{
			"testAzureResourceConnection": {
				Source: testARMID,
				IAM: datamodel.IAMProperties{
					Kind:  datamodel.KindAzure,
					Roles: []string{expectedRole},
				},
			},
		},
		Container: datamodel.Container{
			Image: "testimage:latest",
		},
	}
	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{}

	renderer := Renderer{
		RoleAssignmentMap: map[datamodel.IAMKind]RoleAssignmentData{
			datamodel.KindAzure: {},
		},
	}

	ctx := testcontext.New(t)
	output, err := renderer.Render(ctx, resource, renderers.RenderOptions{Dependencies: dependencies, Environment: testEnvironmentOptions})
	require.NoError(t, err)

	require.Len(t, output.ComputedValues, 2)
	require.Equal(t, output.ComputedValues[handlers.IdentityProperties].Value.(*rpv1.IdentitySettings).Kind, rpv1.AzureIdentityWorkload)
	require.Equal(t, output.ComputedValues[handlers.UserAssignedIdentityIDKey].PropertyReference, handlers.UserAssignedIdentityIDKey)

	require.Empty(t, output.SecretValues)
	require.Len(t, output.Resources, 7)

	resourceMap := outputResourcesToResourceTypeMap(output.Resources)

	_, ok := resourceMap[resources_kubernetes.ResourceTypeDeployment]
	require.Equal(t, true, ok)

	roleOutputResource, ok := resourceMap[resources_azure.ResourceTypeAuthorizationRoleAssignment]
	require.Equal(t, true, ok)
	require.Len(t, roleOutputResource, 1)
	expected := []rpv1.OutputResource{
		{

			LocalID: rpv1.NewLocalID(rpv1.LocalIDRoleAssignmentPrefix, testARMID+expectedRole),
			CreateResource: &rpv1.Resource{
				ResourceType: resourcemodel.ResourceType{
					Type:     resources_azure.ResourceTypeAuthorizationRoleAssignment,
					Provider: resourcemodel.ProviderAzure,
				},
				Data: map[string]string{
					handlers.RoleNameKey:         expectedRole,
					handlers.RoleAssignmentScope: testARMID,
				},
				Dependencies: []string{rpv1.LocalIDUserAssignedManagedIdentity},
			},
		},
	}
	require.ElementsMatch(t, expected, roleOutputResource)

	require.Len(t, resourceMap[resources_azure.ResourceTypeManagedIdentityUserAssignedManagedIdentity], 1)
	require.Len(t, resourceMap[resources_azure.ResourceTypeManagedIdentityUserAssignedManagedIdentityFederatedIdentityCredential], 1)
	require.Len(t, resourceMap[resources_kubernetes.ResourceTypeServiceAccount], 1)
}

func Test_Render_AzureConnectionEmptyRoleAllowed(t *testing.T) {
	testARMID := makeAzureResourceID(t, "SomeProvider/ResourceType", "test-azure-resource").String()
	properties := datamodel.ContainerProperties{
		BasicResourceProperties: rpv1.BasicResourceProperties{
			Application: applicationResourceID,
		},
		Connections: map[string]datamodel.ConnectionProperties{
			"testAzureResourceConnection": {
				Source: testARMID,
				IAM: datamodel.IAMProperties{
					Kind: datamodel.KindAzure,
				},
			},
		},
		Container: datamodel.Container{
			Image: "testimage:latest",
		},
	}
	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{}

	renderer := Renderer{
		RoleAssignmentMap: map[datamodel.IAMKind]RoleAssignmentData{
			datamodel.KindAzure: {},
		},
	}
	ctx := testcontext.New(t)
	_, err := renderer.Render(ctx, resource, renderers.RenderOptions{Dependencies: dependencies})
	require.NoError(t, err)
}

func Test_Render_EphemeralVolumes(t *testing.T) {
	const tempVolName = "TempVolume"
	const tempVolMountPath = "/tmpfs"
	properties := datamodel.ContainerProperties{
		BasicResourceProperties: rpv1.BasicResourceProperties{
			Application: applicationResourceID,
		},
		Container: datamodel.Container{
			Image: "someimage:latest",
			Env: map[string]string{
				envVarName1: envVarValue1,
				envVarName2: envVarValue2,
			},
			Volumes: map[string]datamodel.VolumeProperties{
				tempVolName: {
					Kind: datamodel.Ephemeral,
					Ephemeral: &datamodel.EphemeralVolume{
						VolumeBase: datamodel.VolumeBase{
							MountPath: tempVolMountPath,
						},
						ManagedStore: datamodel.ManagedStoreMemory,
					},
				},
			},
		},
	}
	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{
		(makeRadiusResourceID(t, "SomeProvider/ResourceType", "A").String()): {
			ResourceID:     makeRadiusResourceID(t, "SomeProvider/ResourceType", "A"),
			ComputedValues: map[string]any{},
		},
	}
	ctx := testcontext.New(t)
	renderer := Renderer{}
	output, err := renderer.Render(ctx, resource, renderers.RenderOptions{Dependencies: dependencies})
	require.NoError(t, err)
	require.Empty(t, output.ComputedValues)
	require.Empty(t, output.SecretValues)

	t.Run("verify deployment", func(t *testing.T) {
		deployment, _ := kubernetes.FindDeployment(output.Resources)
		require.NotNil(t, deployment)

		require.Len(t, deployment.Spec.Template.Spec.Containers, 1)

		container := deployment.Spec.Template.Spec.Containers[0]
		require.Equal(t, resourceName, container.Name)

		volumes := deployment.Spec.Template.Spec.Volumes

		expectedVolumeMounts := []corev1.VolumeMount{
			{
				Name:      tempVolName,
				MountPath: tempVolMountPath,
			},
		}

		expectedVolumes := []corev1.Volume{
			{
				Name: tempVolName,
				VolumeSource: corev1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{
						Medium: corev1.StorageMediumMemory,
					},
				},
			},
		}

		require.Equal(t, expectedVolumeMounts, container.VolumeMounts)
		require.Equal(t, expectedVolumes, volumes)
	})
}

func Test_Render_PersistentAzureFileShareVolumes(t *testing.T) {
	t.Skipf("Currently we support only azure CSI keyvault volume. We will enable it when we support azure file share.")

	const tempVolName = "TempVolume"
	const tempVolMountPath = "/tmpfs"
	const testShareName = "myshare"
	testResourceID := fmt.Sprintf("/subscriptions/%s/resourceGroups/test/providers/Microsoft.Storage/storageaccounts/testaccount/fileservices/default/share/%s", uuid.New(), testShareName)

	properties := datamodel.ContainerProperties{
		BasicResourceProperties: rpv1.BasicResourceProperties{
			Application: applicationResourceID,
		},
		Container: datamodel.Container{
			Image: "someimage:latest",
			Volumes: map[string]datamodel.VolumeProperties{
				tempVolName: {
					Kind: datamodel.Persistent,
					Persistent: &datamodel.PersistentVolume{
						VolumeBase: datamodel.VolumeBase{
							MountPath: tempVolMountPath,
						},
						Source: testResourceID,
					},
				},
			},
		},
	}
	resource := makeResource(t, properties)
	resourceID, _ := resources.ParseResource(testResourceID)
	dependencies := map[string]renderers.RendererDependency{
		testResourceID: {
			ResourceID: resourceID,
			ComputedValues: map[string]any{
				"azurestorageaccountname": "accountname",
				"azurestorageaccountkey":  "storagekey",
			},
		},
	}

	ctx := testcontext.New(t)
	renderer := Renderer{}
	renderOutput, err := renderer.Render(ctx, resource, renderers.RenderOptions{Dependencies: dependencies})
	require.Lenf(t, renderOutput.Resources, 2, "expected 2 output resource, instead got %+v", len(renderOutput.Resources))

	deploymentResource := rpv1.OutputResource{}
	secretResource := rpv1.OutputResource{}
	for _, resource := range renderOutput.Resources {
		if resource.LocalID == rpv1.LocalIDDeployment {
			deploymentResource = resource
		}

		if resource.LocalID == rpv1.LocalIDSecret {
			secretResource = resource
		}
	}

	require.NotEmpty(t, deploymentResource)
	require.NotEmpty(t, secretResource)

	// Verify deployment
	volumes := deploymentResource.CreateResource.Data.(*appsv1.Deployment).Spec.Template.Spec.Volumes
	require.Lenf(t, volumes, 1, "expected 1 volume, instead got %+v", len(volumes))
	require.Equal(t, tempVolName, volumes[0].Name)
	require.NotNil(t, volumes[0].VolumeSource.AzureFile, "expected volumesource azurefile to be not nil")
	require.Equal(t, volumes[0].VolumeSource.AzureFile.SecretName, resourceName)
	require.Equal(t, volumes[0].VolumeSource.AzureFile.ShareName, testShareName)

	// Verify Kubernetes secret
	secret := secretResource.CreateResource.Data.(*corev1.Secret)
	require.Lenf(t, secret.Data, 2, "expected 2 secret key-value pairs, instead got %+v", len(secret.Data))
	require.NoError(t, err)
}

func Test_Render_PersistentAzureKeyVaultVolumes(t *testing.T) {
	properties := datamodel.ContainerProperties{
		BasicResourceProperties: rpv1.BasicResourceProperties{
			Application: applicationResourceID,
		},

		Container: datamodel.Container{
			Image: "someimage:latest",
			Volumes: map[string]datamodel.VolumeProperties{
				tempVolName: {
					Kind: datamodel.Persistent,
					Persistent: &datamodel.PersistentVolume{
						VolumeBase: datamodel.VolumeBase{
							MountPath: tempVolMountPath,
						},
						Source: testResourceID,
					},
				},
			},
		},
	}
	resource := makeResource(t, properties)
	resourceID, _ := resources.ParseResource(testResourceID)
	testVolName := "test-volume-sp"
	dependencies := map[string]renderers.RendererDependency{
		testResourceID: {
			ResourceID: resourceID,
			Resource: &datamodel.VolumeResource{
				BaseResource: apiv1.BaseResource{
					TrackedResource: apiv1.TrackedResource{
						Name: testVolName,
					},
				},
				Properties: datamodel.VolumeResourceProperties{
					BasicResourceProperties: rpv1.BasicResourceProperties{
						Application: applicationResourceID,
					},
					Kind: datamodel.AzureKeyVaultVolume,
					AzureKeyVault: &datamodel.AzureKeyVaultVolumeProperties{
						Resource: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup/providers/Microsoft.KeyVault/vaults/vault0",
						Secrets: map[string]datamodel.SecretObjectProperties{
							"my-secret": {
								Name:     "mysecret",
								Version:  "1",
								Encoding: to.Ptr(datamodel.SecretObjectPropertiesEncodingUTF8),
							},
						},
					},
				},
			},
			ComputedValues: map[string]any{
				azvolrenderer.SPCVolumeObjectSpecKey: "objectspecs",
			},
			OutputResources: map[string]resources.ID{
				rpv1.LocalIDSecretProviderClass: resources_kubernetes.IDFromParts(
					resources_kubernetes.PlaneNameTODO,
					"secrets-store.csi.x-k8s.io",
					"SecretProviderClass",
					"test-ns",
					testVolName),
			},
		},
	}

	ctx := testcontext.New(t)
	renderer := Renderer{}
	renderOutput, err := renderer.Render(ctx, resource, renderers.RenderOptions{Dependencies: dependencies, Environment: testEnvironmentOptions})

	require.NoError(t, err)
	require.Lenf(t, renderOutput.Resources, 9, "expected 9 output resources, instead got %+v", len(renderOutput.Resources))

	// Verify deployment
	deploymentSpec := renderOutput.Resources[7]
	require.Equal(t, rpv1.LocalIDDeployment, deploymentSpec.LocalID, "expected output resource of kind deployment instead got :%v", renderOutput.Resources[0].LocalID)
	require.Contains(t, deploymentSpec.CreateResource.Dependencies[0], "RoleAssignment")
	require.Equal(t, deploymentSpec.CreateResource.Dependencies[1], "SecretProviderClass")
	require.Equal(t, deploymentSpec.CreateResource.Dependencies[2], "ServiceAccount")
	require.Equal(t, deploymentSpec.CreateResource.Dependencies[3], "KubernetesRole")
	require.Equal(t, deploymentSpec.CreateResource.Dependencies[4], "KubernetesRoleBinding")
	require.Equal(t, deploymentSpec.CreateResource.Dependencies[5], "Secret")

	// Verify pod template
	podTemplate := deploymentSpec.CreateResource.Data.(*appsv1.Deployment).Spec.Template
	require.Equal(t, "true", podTemplate.ObjectMeta.Labels[azrenderer.AzureWorkloadIdentityUseKey])

	// Verify volume spec
	volumes := deploymentSpec.CreateResource.Data.(*appsv1.Deployment).Spec.Template.Spec.Volumes
	require.Lenf(t, volumes, 1, "expected 1 volume, instead got %+v", len(volumes))
	require.Equal(t, tempVolName, volumes[0].Name)
	require.Equal(t, "secrets-store.csi.k8s.io", volumes[0].VolumeSource.CSI.Driver, "expected volumesource azurefile to be not nil")
	require.Equalf(t, testVolName, volumes[0].VolumeSource.CSI.VolumeAttributes["secretProviderClass"], "expected secret provider class to match the input %s", testVolName)
	require.Equal(t, true, *volumes[0].VolumeSource.CSI.ReadOnly, "expected readonly attribute to be true")

	// Verify volume mount spec
	volumeMounts := deploymentSpec.CreateResource.Data.(*appsv1.Deployment).Spec.Template.Spec.Containers[0].VolumeMounts
	require.Lenf(t, volumeMounts, 1, "expected 1 volume mount, instead got %+v", len(volumeMounts))
	require.Equal(t, tempVolMountPath, volumeMounts[0].MountPath)
	require.Equal(t, tempVolName, volumeMounts[0].Name)
	require.Equal(t, true, volumeMounts[0].ReadOnly)
}

func outputResourcesToResourceTypeMap(resources []rpv1.OutputResource) map[string][]rpv1.OutputResource {
	results := map[string][]rpv1.OutputResource{}
	for _, resource := range resources {
		resourceType := resource.GetResourceType()
		matches := results[resourceType.Type]
		matches = append(matches, resource)
		results[resourceType.Type] = matches
	}

	return results
}

func Test_Render_ReadinessProbeHttpGet(t *testing.T) {
	properties := datamodel.ContainerProperties{
		BasicResourceProperties: rpv1.BasicResourceProperties{
			Application: applicationResourceID,
		},
		Container: datamodel.Container{
			Image: "someimage:latest",
			Env: map[string]string{
				envVarName1: envVarValue1,
				envVarName2: envVarValue2,
			},
			ReadinessProbe: datamodel.HealthProbeProperties{
				Kind: datamodel.HTTPGetHealthProbe,
				HTTPGet: &datamodel.HTTPGetHealthProbeProperties{
					HealthProbeBase: datamodel.HealthProbeBase{
						InitialDelaySeconds: to.Ptr[float32](30),
						FailureThreshold:    to.Ptr[float32](10),
						PeriodSeconds:       to.Ptr[float32](2),
						TimeoutSeconds:      to.Ptr[float32](5),
					},
					Path:          "/healthz",
					ContainerPort: 8080,
					Headers:       map[string]string{"header1": "value1"},
				},
			},
		},
	}
	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{
		(makeAzureResourceID(t, "SomeProvider/ResourceType", "A").String()): {
			ResourceID: makeAzureResourceID(t, "SomeProvider/ResourceType", "A"),
			ComputedValues: map[string]any{
				"ComputedKey1": "ComputedValue1",
				"ComputedKey2": 82,
			},
		},
	}

	ctx := testcontext.New(t)
	renderer := Renderer{}
	output, err := renderer.Render(ctx, resource, renderers.RenderOptions{Dependencies: dependencies})
	require.NoError(t, err)
	require.Empty(t, output.ComputedValues)
	require.Empty(t, output.SecretValues)

	t.Run("verify deployment", func(t *testing.T) {
		deployment, _ := kubernetes.FindDeployment(output.Resources)
		require.NotNil(t, deployment)

		require.Len(t, deployment.Spec.Template.Spec.Containers, 1)

		container := deployment.Spec.Template.Spec.Containers[0]
		require.Equal(t, resourceName, container.Name)

		expectedReadinessProbe := &corev1.Probe{
			InitialDelaySeconds: 30,
			FailureThreshold:    10,
			PeriodSeconds:       2,
			TimeoutSeconds:      5,
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: "/healthz",
					Port: intstr.FromInt(8080),
					HTTPHeaders: []corev1.HTTPHeader{
						{
							Name:  "header1",
							Value: "value1",
						},
					},
				},
				TCPSocket: nil,
				Exec:      nil,
			},
		}

		require.Equal(t, expectedReadinessProbe, container.ReadinessProbe)
	})
}

func Test_Render_ReadinessProbeTcp(t *testing.T) {
	properties := datamodel.ContainerProperties{
		BasicResourceProperties: rpv1.BasicResourceProperties{
			Application: applicationResourceID,
		},
		Container: datamodel.Container{
			Image: "someimage:latest",
			Env: map[string]string{
				envVarName1: envVarValue1,
				envVarName2: envVarValue2,
			},
			ReadinessProbe: datamodel.HealthProbeProperties{
				Kind: datamodel.TCPHealthProbe,
				TCP: &datamodel.TCPHealthProbeProperties{
					HealthProbeBase: datamodel.HealthProbeBase{
						InitialDelaySeconds: to.Ptr[float32](30),
						FailureThreshold:    to.Ptr[float32](10),
						PeriodSeconds:       to.Ptr[float32](2),
						TimeoutSeconds:      to.Ptr[float32](5),
					},
					ContainerPort: 8080,
				},
			},
		},
	}
	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{
		(makeAzureResourceID(t, "SomeProvider/ResourceType", "A").String()): {
			ResourceID: makeAzureResourceID(t, "SomeProvider/ResourceType", "A"),
			ComputedValues: map[string]any{
				"ComputedKey1": "ComputedValue1",
				"ComputedKey2": 82,
			},
		},
	}

	ctx := testcontext.New(t)
	renderer := Renderer{}
	output, err := renderer.Render(ctx, resource, renderers.RenderOptions{Dependencies: dependencies})
	require.NoError(t, err)
	require.Empty(t, output.ComputedValues)
	require.Empty(t, output.SecretValues)

	t.Run("verify deployment", func(t *testing.T) {
		deployment, _ := kubernetes.FindDeployment(output.Resources)
		require.NotNil(t, deployment)

		require.Len(t, deployment.Spec.Template.Spec.Containers, 1)

		container := deployment.Spec.Template.Spec.Containers[0]
		require.Equal(t, resourceName, container.Name)

		expectedReadinessProbe := &corev1.Probe{
			InitialDelaySeconds: 30,
			FailureThreshold:    10,
			PeriodSeconds:       2,
			TimeoutSeconds:      5,
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: nil,
				TCPSocket: &corev1.TCPSocketAction{
					Port: intstr.FromInt(8080),
				},
				Exec: nil,
			},
		}

		require.Equal(t, expectedReadinessProbe, container.ReadinessProbe)
	})
}

func Test_Render_LivenessProbeExec(t *testing.T) {
	properties := datamodel.ContainerProperties{
		BasicResourceProperties: rpv1.BasicResourceProperties{
			Application: applicationResourceID,
		},
		Container: datamodel.Container{
			Image: "someimage:latest",
			Env: map[string]string{
				envVarName1: envVarValue1,
				envVarName2: envVarValue2,
			},
			LivenessProbe: datamodel.HealthProbeProperties{
				Kind: datamodel.ExecHealthProbe,
				Exec: &datamodel.ExecHealthProbeProperties{
					HealthProbeBase: datamodel.HealthProbeBase{
						InitialDelaySeconds: to.Ptr[float32](30),
						FailureThreshold:    to.Ptr[float32](10),
						PeriodSeconds:       to.Ptr[float32](2),
						TimeoutSeconds:      to.Ptr[float32](5),
					},
					Command: "a b c",
				},
			},
		},
	}
	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{
		(makeAzureResourceID(t, "SomeProvider/ResourceType", "A").String()): {
			ResourceID: makeAzureResourceID(t, "SomeProvider/ResourceType", "A"),
			ComputedValues: map[string]any{
				"ComputedKey1": "ComputedValue1",
				"ComputedKey2": 82,
			},
		},
	}

	ctx := testcontext.New(t)
	renderer := Renderer{}
	output, err := renderer.Render(ctx, resource, renderers.RenderOptions{Dependencies: dependencies})
	require.NoError(t, err)
	require.Empty(t, output.ComputedValues)
	require.Empty(t, output.SecretValues)

	t.Run("verify deployment", func(t *testing.T) {
		deployment, _ := kubernetes.FindDeployment(output.Resources)
		require.NotNil(t, deployment)

		require.Len(t, deployment.Spec.Template.Spec.Containers, 1)

		container := deployment.Spec.Template.Spec.Containers[0]
		require.Equal(t, resourceName, container.Name)

		expectedLivenessProbe := &corev1.Probe{
			InitialDelaySeconds: 30,
			FailureThreshold:    10,
			PeriodSeconds:       2,
			TimeoutSeconds:      5,
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet:   nil,
				TCPSocket: nil,
				Exec: &corev1.ExecAction{
					Command: []string{"a", "b", "c"},
				},
			},
		}

		require.Equal(t, expectedLivenessProbe, container.LivenessProbe)
	})
}

func Test_Render_LivenessProbeWithDefaults(t *testing.T) {
	properties := datamodel.ContainerProperties{
		BasicResourceProperties: rpv1.BasicResourceProperties{
			Application: applicationResourceID,
		},
		Container: datamodel.Container{
			Image: "someimage:latest",
			LivenessProbe: datamodel.HealthProbeProperties{
				Kind: datamodel.ExecHealthProbe,
				Exec: &datamodel.ExecHealthProbeProperties{
					Command: "a b c",
				},
			},
		},
	}
	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{
		(makeAzureResourceID(t, "SomeProvider/ResourceType", "A").String()): {
			ResourceID: makeAzureResourceID(t, "SomeProvider/ResourceType", "A"),
			ComputedValues: map[string]any{
				"ComputedKey1": "ComputedValue1",
				"ComputedKey2": 82,
			},
		},
	}

	ctx := testcontext.New(t)
	renderer := Renderer{}
	output, err := renderer.Render(ctx, resource, renderers.RenderOptions{Dependencies: dependencies})
	require.NoError(t, err)
	require.Empty(t, output.ComputedValues)
	require.Empty(t, output.SecretValues)

	t.Run("verify deployment", func(t *testing.T) {
		deployment, _ := kubernetes.FindDeployment(output.Resources)
		require.NotNil(t, deployment)

		require.Len(t, deployment.Spec.Template.Spec.Containers, 1)

		container := deployment.Spec.Template.Spec.Containers[0]
		require.Equal(t, resourceName, container.Name)

		expectedLivenessProbe := &corev1.Probe{
			// Aligining with Kubernetes defaults
			InitialDelaySeconds: DefaultInitialDelaySeconds,
			FailureThreshold:    DefaultFailureThreshold,
			PeriodSeconds:       DefaultPeriodSeconds,
			TimeoutSeconds:      DefaultTimeoutSeconds,
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet:   nil,
				TCPSocket: nil,
				Exec: &corev1.ExecAction{
					Command: []string{"a", "b", "c"},
				},
			},
		}

		require.Equal(t, expectedLivenessProbe, container.LivenessProbe)
	})
}

func Test_IsURL(t *testing.T) {
	const valid_url = "http://examplehost:80"
	const invalid_url = "http://abc:def"
	const path = "/testpath/testfolder/testfile.txt"

	require.True(t, isURL(valid_url))
	require.False(t, isURL(invalid_url))
	require.False(t, isURL(path))
}

func Test_ParseURL(t *testing.T) {
	const valid_url = "http://examplehost:80"
	const invalid_url = "http://abc:def"

	t.Run("valid URL test", func(t *testing.T) {
		scheme, hostname, port, err := parseURL(valid_url)
		require.Equal(t, scheme, "http")
		require.Equal(t, hostname, "examplehost")
		require.Equal(t, port, "80")
		require.Equal(t, err, nil)
	})

	t.Run("invalid URL test", func(t *testing.T) {
		scheme, hostname, port, err := parseURL(invalid_url)
		require.Equal(t, scheme, "")
		require.Equal(t, hostname, "")
		require.Equal(t, port, "")
		require.NotEqual(t, err, nil)
	})
}

func Test_DNS_Service_Generation(t *testing.T) {
	var containerPortNumber int32 = 80
	t.Run("verify service generation", func(t *testing.T) {
		properties := datamodel.ContainerProperties{
			BasicResourceProperties: rpv1.BasicResourceProperties{
				Application: applicationResourceID,
			},
			Container: datamodel.Container{
				Image: "someimage:latest",
				Ports: map[string]datamodel.ContainerPort{
					"web": {
						ContainerPort: int32(containerPortNumber),
					},
				},
			},
		}

		resource := makeResource(t, properties)
		ctx := testcontext.New(t)
		renderer := Renderer{}
		output, err := renderer.Render(ctx, resource, renderOptionsEnvAndAppKubeMetadata())

		require.NoError(t, err)
		require.Len(t, output.Resources, 4)
		require.Empty(t, output.SecretValues)

		expectedServicePort := corev1.ServicePort{
			Name:       "web",
			Port:       containerPortNumber,
			TargetPort: intstr.FromInt(80),
			Protocol:   "TCP",
		}

		require.Len(t, output.ComputedValues, 0)

		service, outputResource := kubernetes.FindService(output.Resources)
		expectedOutputResource := rpv1.NewKubernetesOutputResource(rpv1.LocalIDService, service, service.ObjectMeta)

		require.Equal(t, expectedOutputResource, outputResource)
		require.Equal(t, kubernetes.NormalizeResourceName(resource.Name), service.Name)
		require.Equal(t, "", service.Namespace)
		require.Equal(t, kubernetes.MakeSelectorLabels(applicationName, resource.Name), service.Spec.Selector)
		require.Equal(t, corev1.ServiceTypeClusterIP, service.Spec.Type)
		require.Len(t, service.Spec.Ports, 1)

		servicePort := service.Spec.Ports[0]
		require.Equal(t, expectedServicePort, servicePort)
	})
}

func Test_Render_ImagePullPolicySpecified(t *testing.T) {
	properties := datamodel.ContainerProperties{
		BasicResourceProperties: rpv1.BasicResourceProperties{
			Application: applicationResourceID,
		},
		Container: datamodel.Container{
			Image:           "someimage:latest",
			ImagePullPolicy: "Never",
			Env: map[string]string{
				envVarName1: envVarValue1,
				envVarName2: envVarValue2,
			},
		},
	}
	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{}

	ctx := testcontext.New(t)
	renderer := Renderer{}
	output, err := renderer.Render(ctx, resource, renderers.RenderOptions{Dependencies: dependencies})
	require.NoError(t, err)
	require.Empty(t, output.ComputedValues)
	require.Empty(t, output.SecretValues)

	t.Run("verify deployment", func(t *testing.T) {
		deployment, _ := kubernetes.FindDeployment(output.Resources)
		require.NotNil(t, deployment)

		require.Len(t, deployment.Spec.Template.Spec.Containers, 1)

		container := deployment.Spec.Template.Spec.Containers[0]
		require.Equal(t, resourceName, container.Name)
		require.Equal(t, properties.Container.Image, container.Image)
		require.Equal(t, properties.Container.ImagePullPolicy, string(container.ImagePullPolicy))
	})
}

func renderOptionsEnvAndAppKubeMetadata() renderers.RenderOptions {
	dependencies := map[string]renderers.RendererDependency{}
	option := renderers.RenderOptions{Dependencies: dependencies}

	option.Application = renderers.ApplicationOptions{
		KubernetesMetadata: &datamodel.KubeMetadataExtension{
			Annotations: getAppSetup().appKubeMetadataExt.Annotations,
			Labels:      getAppSetup().appKubeMetadataExt.Labels,
		},
	}

	return option
}

func getAppSetup() *setupMaps {
	setupMap := setupMaps{}

	appKubeMetadataExt := &datamodel.KubeMetadataExtension{
		Annotations: map[string]string{
			appAnnotationKey1: appAnnotationVal1,
			appAnnotationKey2: appAnnotationVal2,
			overrideKey1:      overrideVal1,
		},
		Labels: map[string]string{
			appLabelKey1: appLabelVal1,
			appLabelKey2: appLabelVal2,
			overrideKey2: overrideVal2,
		},
	}

	setupMap.appKubeMetadataExt = appKubeMetadataExt

	return &setupMap
}

type setupMaps struct {
	appKubeMetadataExt *datamodel.KubeMetadataExtension
}
