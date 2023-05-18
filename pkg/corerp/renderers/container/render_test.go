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
	"context"
	"fmt"
	"reflect"
	"sort"
	"testing"

	apiv1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/corerp/handlers"
	"github.com/project-radius/radius/pkg/corerp/renderers"
	azrenderer "github.com/project-radius/radius/pkg/corerp/renderers/container/azure"
	azvolrenderer "github.com/project-radius/radius/pkg/corerp/renderers/volume/azure"
	"github.com/project-radius/radius/pkg/kubernetes"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/project-radius/radius/pkg/resourcemodel"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
	"github.com/project-radius/radius/pkg/to"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/pkg/ucp/ucplog"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	applicationName       = "test-app"
	applicationResourceID = "/subscriptions/test-sub-id/resourceGroups/test-rg/providers/Applications.Core/applications/test-app"
	resourceName          = "test-container"
	envVarName1           = "TEST_VAR_1"
	envVarValue1          = "TEST_VALUE_1"
	envVarName2           = "TEST_VAR_2"
	envVarValue2          = "81"
	secretName            = "test-container"

	tempVolName      = "TempVolume"
	tempVolMountPath = "/tmpfs"
	testResourceID   = "/subscriptions/sub/resourceGroups/rg/providers/Microsoft.KeyVault/vaults/azure-kv"
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

func createContext(t *testing.T) context.Context {
	logger, err := ucplog.NewTestLogger(t)
	if err != nil {
		t.Log("Unable to initialize logger")
		return context.Background()
	}
	return logr.NewContext(context.Background(), logger)
}

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

func makeResourceID(t *testing.T, resourceType string, resourceName string) resources.ID {
	id, err := resources.ParseResource(resources.MakeRelativeID(
		[]resources.ScopeSegment{
			{Type: "subscriptions", Name: "test-subscription"},
			{Type: "resourceGroups", Name: "test-resourcegroup"},
		},
		resources.TypeSegment{
			Type: resourceType,
			Name: resourceName,
		}))
	require.NoError(t, err)

	return id
}

func Test_GetDependencyIDs_Success(t *testing.T) {
	testStorageResourceID := "/subscriptions/test-sub-id/resourceGroups/test-rg/providers/Microsoft.Storage/storageaccounts/testaccount/fileservices/default/shares/testShareName"
	testAzureResourceID := makeResourceID(t, "Microsoft.ServiceBus/namespaces", "testAzureResource")
	properties := datamodel.ContainerProperties{
		BasicResourceProperties: rpv1.BasicResourceProperties{
			Application: applicationResourceID,
		},
		Connections: map[string]datamodel.ConnectionProperties{
			"A": {
				Source: makeResourceID(t, "Applications.Core/httpRoutes", "A").String(),
			},
			"B": {
				Source: makeResourceID(t, "Applications.Core/httpRoutes", "B").String(),
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
					Provides:      makeResourceID(t, "Applications.Core/httpRoutes", "C").String(),
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

	renderer := Renderer{}
	radiusResourceIDs, azureResourceIDs, err := renderer.GetDependencyIDs(createContext(t), resource)
	require.NoError(t, err)
	require.Len(t, radiusResourceIDs, 3)
	require.Len(t, azureResourceIDs, 1)

	expectedRadiusResourceIDs := []resources.ID{
		makeResourceID(t, "Applications.Core/httpRoutes", "A"),
		makeResourceID(t, "Applications.Core/httpRoutes", "B"),
		makeResourceID(t, "Applications.Core/httpRoutes", "C"),
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

	renderer := Renderer{}
	ids, azureIDs, err := renderer.GetDependencyIDs(createContext(t), resource)
	require.Error(t, err)
	require.Empty(t, ids)
	require.Empty(t, azureIDs)
}

func Test_GetDependencyIDs_InvalidAzureResourceId(t *testing.T) {
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
	ids, azureIDs, err := renderer.GetDependencyIDs(createContext(t), resource)
	require.Error(t, err)
	require.Equal(t, err.(*apiv1.ErrClientRP).Code, apiv1.CodeInvalid)
	require.Equal(t, err.(*apiv1.ErrClientRP).Message, "'/subscriptions/test-sub-id/providers/Microsoft.ServiceBus/namespaces/testNamespace' is not a valid resource id")
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

	renderer := Renderer{}
	output, err := renderer.Render(createContext(t), resource, renderers.RenderOptions{Dependencies: dependencies})
	require.NoError(t, err)
	require.Empty(t, output.ComputedValues)
	require.Empty(t, output.SecretValues)

	labels := kubernetes.MakeDescriptiveLabels(applicationName, resource.Name, resource.ResourceTypeName())
	labels[kubernetes.LabelDeployedBy] = kubernetes.CoreRP
	podLabels := kubernetes.MakeDescriptiveLabels(applicationName, resource.Name, resource.ResourceTypeName())
	matchLabels := kubernetes.MakeSelectorLabels(applicationName, resource.Name)

	t.Run("verify deployment", func(t *testing.T) {
		deployment, outputResource := kubernetes.FindDeployment(output.Resources)
		require.NotNil(t, deployment)

		expected := rpv1.NewKubernetesOutputResource(resourcekinds.Deployment, rpv1.LocalIDDeployment, deployment, deployment.ObjectMeta)
		expected.Dependencies = []rpv1.Dependency{
			{
				LocalID: rpv1.LocalIDKubernetesRole,
			},
			{
				LocalID: rpv1.LocalIDKubernetesRoleBinding,
			},
		}
		require.Equal(t, outputResource, expected)

		// Only real thing to verify here is the image and the labels
		assertMapsEqual(t, labels, deployment.Labels)
		require.Equal(t, podLabels, deployment.Spec.Template.Labels)
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
		require.Equal(t, v1.PullAlways, container.ImagePullPolicy)

		var commands []string
		var args []string
		require.Equal(t, commands, container.Command)
		require.Equal(t, args, container.Args)
		require.Equal(t, "", container.WorkingDir)

		expectedEnv := []v1.EnvVar{
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

	renderer := Renderer{}
	output, err := renderer.Render(createContext(t), resource, renderers.RenderOptions{Dependencies: dependencies})
	require.NoError(t, err)
	require.Empty(t, output.ComputedValues)
	require.Empty(t, output.SecretValues)

	labels := kubernetes.MakeDescriptiveLabels(applicationName, resource.Name, resource.ResourceTypeName())
	labels[kubernetes.LabelDeployedBy] = kubernetes.CoreRP
	podLabels := kubernetes.MakeDescriptiveLabels(applicationName, resource.Name, resource.ResourceTypeName())
	matchLabels := kubernetes.MakeSelectorLabels(applicationName, resource.Name)

	t.Run("verify deployment", func(t *testing.T) {
		deployment, outputResource := kubernetes.FindDeployment(output.Resources)
		require.NotNil(t, deployment)

		expected := rpv1.NewKubernetesOutputResource(resourcekinds.Deployment, rpv1.LocalIDDeployment, deployment, deployment.ObjectMeta)
		expected.Dependencies = []rpv1.Dependency{
			{
				LocalID: rpv1.LocalIDKubernetesRole,
			},
			{
				LocalID: rpv1.LocalIDKubernetesRoleBinding,
			},
		}
		require.Equal(t, outputResource, expected)

		// Only real thing to verify here is the image and the labels
		assertMapsEqual(t, labels, deployment.Labels)
		require.Equal(t, podLabels, deployment.Spec.Template.Labels)
		require.Equal(t, matchLabels, deployment.Spec.Selector.MatchLabels)

		require.Len(t, deployment.Spec.Template.Spec.Containers, 1)

		container := deployment.Spec.Template.Spec.Containers[0]
		require.Equal(t, resourceName, container.Name)
		require.Equal(t, properties.Container.Image, container.Image)
		require.Equal(t, v1.PullAlways, container.ImagePullPolicy)
		require.Equal(t, []string{"command1", "command2"}, container.Command)
		require.Equal(t, []string{"arg1", "arg2"}, container.Args)
		require.Equal(t, "/some/path", container.WorkingDir)

		expectedEnv := []v1.EnvVar{
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

	renderer := Renderer{}
	output, err := renderer.Render(createContext(t), resource, renderers.RenderOptions{Dependencies: dependencies})
	require.NoError(t, err)
	require.Empty(t, output.ComputedValues)
	require.Empty(t, output.SecretValues)

	t.Run("verify deployment", func(t *testing.T) {
		deployment, _ := kubernetes.FindDeployment(output.Resources)
		require.NotNil(t, deployment)

		require.Len(t, deployment.Spec.Template.Spec.Containers, 1)
		container := deployment.Spec.Template.Spec.Containers[0]

		require.Len(t, container.Ports, 1)
		port := container.Ports[0]

		expected := v1.ContainerPort{
			ContainerPort: 5000,
			Protocol:      v1.ProtocolTCP,
		}
		require.Equal(t, expected, port)

	})
	require.Len(t, output.Resources, 3)
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
					Provides:      makeResourceID(t, "Applications.Core/httpRoutes", "A").String(),
				},
			},
		},
	}
	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{}

	renderer := Renderer{}
	output, err := renderer.Render(createContext(t), resource, renderers.RenderOptions{Dependencies: dependencies})
	require.NoError(t, err)
	require.Empty(t, output.ComputedValues)
	require.Empty(t, output.SecretValues)

	labels := kubernetes.MakeDescriptiveLabels(applicationName, resource.Name, resource.ResourceTypeName())
	labels[kubernetes.LabelDeployedBy] = kubernetes.CoreRP
	podLabels := kubernetes.MakeDescriptiveLabels(applicationName, resource.Name, resource.ResourceTypeName())
	podLabels["radius.dev/route-httproutes-a"] = "true"

	t.Run("verify deployment", func(t *testing.T) {
		deployment, _ := kubernetes.FindDeployment(output.Resources)
		require.NotNil(t, deployment)

		require.Len(t, deployment.Spec.Template.Spec.Containers, 1)
		container := deployment.Spec.Template.Spec.Containers[0]

		// Labels are somewhat specialized when a route is involved
		assertMapsEqual(t, labels, deployment.Labels)
		require.Equal(t, podLabels, deployment.Spec.Template.Labels)

		require.Len(t, container.Ports, 1)
		port := container.Ports[0]

		routeID := makeResourceID(t, "Applications.Core/httpRoutes", "A")

		expected := v1.ContainerPort{
			Name:          kubernetes.GetShortenedTargetPortName("httpRoutes" + routeID.Name()),
			ContainerPort: 5000,
			Protocol:      v1.ProtocolTCP,
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
				Source: makeResourceID(t, "SomeProvider/ResourceType", "A").String(),
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
		(makeResourceID(t, "SomeProvider/ResourceType", "A").String()): {
			ResourceID: makeResourceID(t, "SomeProvider/ResourceType", "A"),
			ComputedValues: map[string]any{
				"ComputedKey1": "ComputedValue1",
				"ComputedKey2": 82,
			},
		},
	}

	renderer := Renderer{}
	output, err := renderer.Render(createContext(t), resource, renderers.RenderOptions{Dependencies: dependencies, Environment: renderers.EnvironmentOptions{Namespace: "default"}})
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
		require.Equal(t, v1.PullAlways, container.ImagePullPolicy)

		expectedEnv := []v1.EnvVar{
			{
				Name: "CONNECTION_A_COMPUTEDKEY1",
				ValueFrom: &v1.EnvVarSource{
					SecretKeyRef: &v1.SecretKeySelector{
						LocalObjectReference: v1.LocalObjectReference{
							Name: secretName,
						},
						Key: "CONNECTION_A_COMPUTEDKEY1",
					},
				},
			},
			{
				Name: "CONNECTION_A_COMPUTEDKEY2",
				ValueFrom: &v1.EnvVarSource{
					SecretKeyRef: &v1.SecretKeySelector{
						LocalObjectReference: v1.LocalObjectReference{
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

		expectedOutputResource := rpv1.NewKubernetesOutputResource(resourcekinds.Secret, rpv1.LocalIDSecret, secret, secret.ObjectMeta)
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
				Source:                makeResourceID(t, "SomeProvider/ResourceType", "A").String(),
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
		(makeResourceID(t, "SomeProvider/ResourceType", "A").String()): {
			ResourceID: makeResourceID(t, "SomeProvider/ResourceType", "A"),
			ComputedValues: map[string]any{
				"ComputedKey1": "ComputedValue1",
				"ComputedKey2": 82,
			},
		},
	}

	renderer := Renderer{}
	output, err := renderer.Render(createContext(t), resource, renderers.RenderOptions{Dependencies: dependencies, Environment: renderers.EnvironmentOptions{Namespace: "default"}})
	require.NoError(t, err)

	deployment, _ := kubernetes.FindDeployment(output.Resources)
	require.NotNil(t, deployment)

	require.Len(t, deployment.Spec.Template.Spec.Containers, 1)

	container := deployment.Spec.Template.Spec.Containers[0]
	require.Equal(t, resourceName, container.Name)
	require.Equal(t, properties.Container.Image, container.Image)
	require.Equal(t, v1.PullAlways, container.ImagePullPolicy)

	expectedEnv := []v1.EnvVar{}
	require.Equal(t, expectedEnv, container.Env)
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
				Source: makeResourceID(t, "SomeProvider/ResourceType", "A").String(),
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
		(makeResourceID(t, "SomeProvider/ResourceType", "A").String()): {
			ResourceID: makeResourceID(t, "SomeProvider/ResourceType", "A"),
			ComputedValues: map[string]any{
				"ComputedKey1": "ComputedValue1",
				"ComputedKey2": 82,
			},
		},
	}

	renderer := Renderer{}
	output, err := renderer.Render(createContext(t), resource, renderers.RenderOptions{Dependencies: dependencies, Environment: renderers.EnvironmentOptions{Namespace: "default"}})
	require.NoError(t, err)
	require.Empty(t, output.ComputedValues)
	require.Empty(t, output.SecretValues)

	deployment, _ := kubernetes.FindDeployment(output.Resources)
	require.NotNil(t, deployment)

	require.Contains(t, deployment.Spec.Template.Annotations, kubernetes.AnnotationSecretHash)
	hash1 := deployment.Spec.Template.Annotations[kubernetes.AnnotationSecretHash]

	// Update and render again
	dependencies[makeResourceID(t, "SomeProvider/ResourceType", "A").String()].ComputedValues["ComputedKey1"] = "new value"

	output, err = renderer.Render(createContext(t), resource, renderers.RenderOptions{Dependencies: dependencies, Environment: renderers.EnvironmentOptions{Namespace: "default"}})
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
				Source: makeResourceID(t, "SomeProvider/ResourceType", "A").String(),
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
		(makeResourceID(t, "SomeProvider/ResourceType", "A").String()): {
			ResourceID: makeResourceID(t, "SomeProvider/ResourceType", "A"),
			ComputedValues: map[string]any{
				"ComputedKey1": "ComputedValue1",
				"ComputedKey2": 82,
			},
			OutputResources: map[string]resourcemodel.ResourceIdentity{
				// This is the resource that the role assignments target!
				"TargetLocalID": resourcemodel.NewARMIdentity(
					&resourcemodel.ResourceType{
						Type:     "dummy",
						Provider: resourcemodel.ProviderAzure,
					},
					makeResourceID(t, "SomeProvider/TargetResourceType", "TargetResource").String(),
					"2020-01-01"),
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
	output, err := renderer.Render(createContext(t), resource, renderers.RenderOptions{Dependencies: dependencies, Environment: testEnvironmentOptions})
	require.NoError(t, err)
	require.Len(t, output.ComputedValues, 2)
	require.Equal(t, output.ComputedValues[handlers.IdentityProperties].Value.(*rpv1.IdentitySettings).Kind, rpv1.AzureIdentityWorkload)
	require.Equal(t, output.ComputedValues[handlers.UserAssignedIdentityIDKey].PropertyReference, handlers.UserAssignedIdentityIDKey)
	require.Empty(t, output.SecretValues)
	require.Len(t, output.Resources, 9)

	resourceMap := outputResourcesToKindMap(output.Resources)

	// We're just verifying the role assignments and related things, we'll ignore kubernetes types.
	matches := resourceMap[resourcekinds.Deployment]
	require.Equal(t, 1, len(matches))

	matches = resourceMap[resourcekinds.Secret]
	require.Equal(t, 1, len(matches))

	matches = resourceMap[resourcekinds.AzureRoleAssignment]
	require.Equal(t, 2, len(matches))
	expected := []rpv1.OutputResource{
		{
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.AzureRoleAssignment,
				Provider: resourcemodel.ProviderAzure,
			},
			LocalID:  rpv1.GenerateLocalIDForRoleAssignment(makeResourceID(t, "SomeProvider/TargetResourceType", "TargetResource").String(), "TestRole1"),
			Deployed: false,
			Resource: map[string]string{
				handlers.RoleNameKey:         "TestRole1",
				handlers.RoleAssignmentScope: makeResourceID(t, "SomeProvider/TargetResourceType", "TargetResource").String(),
			},
			Dependencies: []rpv1.Dependency{
				{
					LocalID: rpv1.LocalIDUserAssignedManagedIdentity,
				},
			},
		},
		{
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.AzureRoleAssignment,
				Provider: resourcemodel.ProviderAzure,
			},
			LocalID:  rpv1.GenerateLocalIDForRoleAssignment(makeResourceID(t, "SomeProvider/TargetResourceType", "TargetResource").String(), "TestRole2"),
			Deployed: false,
			Resource: map[string]string{
				handlers.RoleNameKey:         "TestRole2",
				handlers.RoleAssignmentScope: makeResourceID(t, "SomeProvider/TargetResourceType", "TargetResource").String(),
			},
			Dependencies: []rpv1.Dependency{
				{
					LocalID: rpv1.LocalIDUserAssignedManagedIdentity,
				},
			},
		},
	}
	require.ElementsMatch(t, expected, matches)

	matches = resourceMap[resourcekinds.AzureUserAssignedManagedIdentity]
	require.Equal(t, 1, len(matches))

	expected = []rpv1.OutputResource{
		{
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.AzureUserAssignedManagedIdentity,
				Provider: resourcemodel.ProviderAzure,
			},
			LocalID:  rpv1.LocalIDUserAssignedManagedIdentity,
			Deployed: false,
			Resource: map[string]string{
				"userassignedidentityname":           "test-app-test-container",
				"userassignedidentitysubscriptionid": "00000000-0000-0000-0000-000000000000",
				"userassignedidentityresourcegroup":  "testGroup",
			},
		},
	}
	require.ElementsMatch(t, expected, matches)

	matches = resourceMap[resourcekinds.AzureFederatedIdentity]
	require.Equal(t, 1, len(matches))

	expected = []rpv1.OutputResource{
		{
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.AzureFederatedIdentity,
				Provider: resourcemodel.ProviderAzure,
			},
			LocalID:  rpv1.LocalIDFederatedIdentity,
			Deployed: false,
			Resource: map[string]string{
				"federatedidentityname":    "test-container",
				"federatedidentitysubject": "system:serviceaccount:default:test-container",
				"federatedidentityissuer":  "https://radiusoidc/00000000-0000-0000-0000-000000000000",
			},
			Dependencies: []rpv1.Dependency{
				{
					LocalID: rpv1.LocalIDUserAssignedManagedIdentity,
				},
			},
		}}
	require.ElementsMatch(t, expected, matches)

	matches = resourceMap[resourcekinds.ServiceAccount]
	require.Equal(t, 1, len(matches))
}

func Test_Render_AzureConnection(t *testing.T) {
	testARMID := makeResourceID(t, "SomeProvider/ResourceType", "test-azure-resource").String()
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

	output, err := renderer.Render(createContext(t), resource, renderers.RenderOptions{Dependencies: dependencies, Environment: testEnvironmentOptions})
	require.NoError(t, err)

	require.Len(t, output.ComputedValues, 2)
	require.Equal(t, output.ComputedValues[handlers.IdentityProperties].Value.(*rpv1.IdentitySettings).Kind, rpv1.AzureIdentityWorkload)
	require.Equal(t, output.ComputedValues[handlers.UserAssignedIdentityIDKey].PropertyReference, handlers.UserAssignedIdentityIDKey)

	require.Empty(t, output.SecretValues)
	require.Len(t, output.Resources, 7)

	kindResourceMap := outputResourcesToKindMap(output.Resources)

	_, ok := kindResourceMap[resourcekinds.Deployment]
	require.Equal(t, true, ok)

	roleOutputResource, ok := kindResourceMap[resourcekinds.AzureRoleAssignment]
	require.Equal(t, true, ok)
	require.Len(t, roleOutputResource, 1)
	expected := []rpv1.OutputResource{
		{
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.AzureRoleAssignment,
				Provider: resourcemodel.ProviderAzure,
			},
			LocalID:  rpv1.GenerateLocalIDForRoleAssignment(testARMID, expectedRole),
			Deployed: false,
			Resource: map[string]string{
				handlers.RoleNameKey:         expectedRole,
				handlers.RoleAssignmentScope: testARMID,
			},
			Dependencies: []rpv1.Dependency{
				{
					LocalID: rpv1.LocalIDUserAssignedManagedIdentity,
				},
			},
		},
	}
	require.ElementsMatch(t, expected, roleOutputResource)

	require.Len(t, kindResourceMap[resourcekinds.AzureUserAssignedManagedIdentity], 1)
	require.Len(t, kindResourceMap[resourcekinds.AzureFederatedIdentity], 1)
	require.Len(t, kindResourceMap[resourcekinds.ServiceAccount], 1)
}

func Test_Render_AzureConnectionEmptyRoleAllowed(t *testing.T) {
	testARMID := makeResourceID(t, "SomeProvider/ResourceType", "test-azure-resource").String()
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
	_, err := renderer.Render(createContext(t), resource, renderers.RenderOptions{Dependencies: dependencies})
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
		(makeResourceID(t, "SomeProvider/ResourceType", "A").String()): {
			ResourceID:     makeResourceID(t, "SomeProvider/ResourceType", "A"),
			ComputedValues: map[string]any{},
		},
	}
	renderer := Renderer{}
	output, err := renderer.Render(createContext(t), resource, renderers.RenderOptions{Dependencies: dependencies})
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

		expectedVolumeMounts := []v1.VolumeMount{
			{
				Name:      tempVolName,
				MountPath: tempVolMountPath,
			},
		}

		expectedVolumes := []v1.Volume{
			{
				Name: tempVolName,
				VolumeSource: v1.VolumeSource{
					EmptyDir: &v1.EmptyDirVolumeSource{
						Medium: v1.StorageMediumMemory,
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

	renderer := Renderer{}
	renderOutput, err := renderer.Render(createContext(t), resource, renderers.RenderOptions{Dependencies: dependencies})
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
	volumes := deploymentResource.Resource.(*appsv1.Deployment).Spec.Template.Spec.Volumes
	require.Lenf(t, volumes, 1, "expected 1 volume, instead got %+v", len(volumes))
	require.Equal(t, tempVolName, volumes[0].Name)
	require.NotNil(t, volumes[0].VolumeSource.AzureFile, "expected volumesource azurefile to be not nil")
	require.Equal(t, volumes[0].VolumeSource.AzureFile.SecretName, resourceName)
	require.Equal(t, volumes[0].VolumeSource.AzureFile.ShareName, testShareName)

	// Verify Kubernetes secret
	secret := secretResource.Resource.(*v1.Secret)
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
			OutputResources: map[string]resourcemodel.ResourceIdentity{
				rpv1.LocalIDSecretProviderClass: {
					ResourceType: &resourcemodel.ResourceType{
						Type:     resourcekinds.SecretProviderClass,
						Provider: resourcemodel.ProviderKubernetes,
					},
					Data: resourcemodel.KubernetesIdentity{
						Kind:       "SecretProviderClass",
						APIVersion: "secrets-store.csi.x-k8s.io/v1alpha1",
						Name:       testVolName,
						Namespace:  "test-ns",
					},
				},
			},
		},
	}

	renderer := Renderer{}
	renderOutput, err := renderer.Render(createContext(t), resource, renderers.RenderOptions{Dependencies: dependencies, Environment: testEnvironmentOptions})

	require.NoError(t, err)
	require.Lenf(t, renderOutput.Resources, 9, "expected 9 output resources, instead got %+v", len(renderOutput.Resources))

	// Verify deployment
	deploymentSpec := renderOutput.Resources[7]
	require.Equal(t, rpv1.LocalIDDeployment, deploymentSpec.LocalID, "expected output resource of kind deployment instead got :%v", renderOutput.Resources[0].LocalID)
	require.Contains(t, deploymentSpec.Dependencies[0].LocalID, "RoleAssignment")
	require.Equal(t, deploymentSpec.Dependencies[1].LocalID, "SecretProviderClass")
	require.Equal(t, deploymentSpec.Dependencies[2].LocalID, "ServiceAccount")
	require.Equal(t, deploymentSpec.Dependencies[3].LocalID, "KubernetesRole")
	require.Equal(t, deploymentSpec.Dependencies[4].LocalID, "KubernetesRoleBinding")
	require.Equal(t, deploymentSpec.Dependencies[5].LocalID, "Secret")

	// Verify pod template
	podTemplate := deploymentSpec.Resource.(*appsv1.Deployment).Spec.Template
	require.Equal(t, "true", podTemplate.ObjectMeta.Labels[azrenderer.AzureWorkloadIdentityUseKey])

	// Verify volume spec
	volumes := deploymentSpec.Resource.(*appsv1.Deployment).Spec.Template.Spec.Volumes
	require.Lenf(t, volumes, 1, "expected 1 volume, instead got %+v", len(volumes))
	require.Equal(t, tempVolName, volumes[0].Name)
	require.Equal(t, "secrets-store.csi.k8s.io", volumes[0].VolumeSource.CSI.Driver, "expected volumesource azurefile to be not nil")
	require.Equalf(t, testVolName, volumes[0].VolumeSource.CSI.VolumeAttributes["secretProviderClass"], "expected secret provider class to match the input %s", testVolName)
	require.Equal(t, true, *volumes[0].VolumeSource.CSI.ReadOnly, "expected readonly attribute to be true")

	// Verify volume mount spec
	volumeMounts := deploymentSpec.Resource.(*appsv1.Deployment).Spec.Template.Spec.Containers[0].VolumeMounts
	require.Lenf(t, volumeMounts, 1, "expected 1 volume mount, instead got %+v", len(volumeMounts))
	require.Equal(t, tempVolMountPath, volumeMounts[0].MountPath)
	require.Equal(t, tempVolName, volumeMounts[0].Name)
	require.Equal(t, true, volumeMounts[0].ReadOnly)
}

func outputResourcesToKindMap(resources []rpv1.OutputResource) map[string][]rpv1.OutputResource {
	results := map[string][]rpv1.OutputResource{}
	for _, resource := range resources {
		matches := results[resource.ResourceType.Type]
		matches = append(matches, resource)
		results[resource.ResourceType.Type] = matches
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
		(makeResourceID(t, "SomeProvider/ResourceType", "A").String()): {
			ResourceID: makeResourceID(t, "SomeProvider/ResourceType", "A"),
			ComputedValues: map[string]any{
				"ComputedKey1": "ComputedValue1",
				"ComputedKey2": 82,
			},
		},
	}

	renderer := Renderer{}
	output, err := renderer.Render(createContext(t), resource, renderers.RenderOptions{Dependencies: dependencies})
	require.NoError(t, err)
	require.Empty(t, output.ComputedValues)
	require.Empty(t, output.SecretValues)

	t.Run("verify deployment", func(t *testing.T) {
		deployment, _ := kubernetes.FindDeployment(output.Resources)
		require.NotNil(t, deployment)

		require.Len(t, deployment.Spec.Template.Spec.Containers, 1)

		container := deployment.Spec.Template.Spec.Containers[0]
		require.Equal(t, resourceName, container.Name)

		expectedReadinessProbe := &v1.Probe{
			InitialDelaySeconds: 30,
			FailureThreshold:    10,
			PeriodSeconds:       2,
			TimeoutSeconds:      5,
			ProbeHandler: v1.ProbeHandler{
				HTTPGet: &v1.HTTPGetAction{
					Path: "/healthz",
					Port: intstr.FromInt(8080),
					HTTPHeaders: []v1.HTTPHeader{
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
		(makeResourceID(t, "SomeProvider/ResourceType", "A").String()): {
			ResourceID: makeResourceID(t, "SomeProvider/ResourceType", "A"),
			ComputedValues: map[string]any{
				"ComputedKey1": "ComputedValue1",
				"ComputedKey2": 82,
			},
		},
	}

	renderer := Renderer{}
	output, err := renderer.Render(createContext(t), resource, renderers.RenderOptions{Dependencies: dependencies})
	require.NoError(t, err)
	require.Empty(t, output.ComputedValues)
	require.Empty(t, output.SecretValues)

	t.Run("verify deployment", func(t *testing.T) {
		deployment, _ := kubernetes.FindDeployment(output.Resources)
		require.NotNil(t, deployment)

		require.Len(t, deployment.Spec.Template.Spec.Containers, 1)

		container := deployment.Spec.Template.Spec.Containers[0]
		require.Equal(t, resourceName, container.Name)

		expectedReadinessProbe := &v1.Probe{
			InitialDelaySeconds: 30,
			FailureThreshold:    10,
			PeriodSeconds:       2,
			TimeoutSeconds:      5,
			ProbeHandler: v1.ProbeHandler{
				HTTPGet: nil,
				TCPSocket: &v1.TCPSocketAction{
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
		(makeResourceID(t, "SomeProvider/ResourceType", "A").String()): {
			ResourceID: makeResourceID(t, "SomeProvider/ResourceType", "A"),
			ComputedValues: map[string]any{
				"ComputedKey1": "ComputedValue1",
				"ComputedKey2": 82,
			},
		},
	}

	renderer := Renderer{}
	output, err := renderer.Render(createContext(t), resource, renderers.RenderOptions{Dependencies: dependencies})
	require.NoError(t, err)
	require.Empty(t, output.ComputedValues)
	require.Empty(t, output.SecretValues)

	t.Run("verify deployment", func(t *testing.T) {
		deployment, _ := kubernetes.FindDeployment(output.Resources)
		require.NotNil(t, deployment)

		require.Len(t, deployment.Spec.Template.Spec.Containers, 1)

		container := deployment.Spec.Template.Spec.Containers[0]
		require.Equal(t, resourceName, container.Name)

		expectedLivenessProbe := &v1.Probe{
			InitialDelaySeconds: 30,
			FailureThreshold:    10,
			PeriodSeconds:       2,
			TimeoutSeconds:      5,
			ProbeHandler: v1.ProbeHandler{
				HTTPGet:   nil,
				TCPSocket: nil,
				Exec: &v1.ExecAction{
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
		(makeResourceID(t, "SomeProvider/ResourceType", "A").String()): {
			ResourceID: makeResourceID(t, "SomeProvider/ResourceType", "A"),
			ComputedValues: map[string]any{
				"ComputedKey1": "ComputedValue1",
				"ComputedKey2": 82,
			},
		},
	}

	renderer := Renderer{}
	output, err := renderer.Render(createContext(t), resource, renderers.RenderOptions{Dependencies: dependencies})
	require.NoError(t, err)
	require.Empty(t, output.ComputedValues)
	require.Empty(t, output.SecretValues)

	t.Run("verify deployment", func(t *testing.T) {
		deployment, _ := kubernetes.FindDeployment(output.Resources)
		require.NotNil(t, deployment)

		require.Len(t, deployment.Spec.Template.Spec.Containers, 1)

		container := deployment.Spec.Template.Spec.Containers[0]
		require.Equal(t, resourceName, container.Name)

		expectedLivenessProbe := &v1.Probe{
			// Aligining with Kubernetes defaults
			InitialDelaySeconds: DefaultInitialDelaySeconds,
			FailureThreshold:    DefaultFailureThreshold,
			PeriodSeconds:       DefaultPeriodSeconds,
			TimeoutSeconds:      DefaultTimeoutSeconds,
			ProbeHandler: v1.ProbeHandler{
				HTTPGet:   nil,
				TCPSocket: nil,
				Exec: &v1.ExecAction{
					Command: []string{"a", "b", "c"},
				},
			},
		}

		require.Equal(t, expectedLivenessProbe, container.LivenessProbe)
	})
}

func assertMapsEqual(t *testing.T, expected, actual map[string]string) {
	// Get the keys of both maps
	expectedKeys := make([]string, 0, len(expected))
	for k := range expected {
		expectedKeys = append(expectedKeys, k)
	}
	actualKeys := make([]string, 0, len(actual))
	for k := range actual {
		actualKeys = append(actualKeys, k)
	}

	// Sort the keys
	sort.Strings(expectedKeys)
	sort.Strings(actualKeys)

	// Compare the lengths of the maps
	if len(expected) != len(actual) {
		t.Errorf("Maps have different lengths")
	}

	// Compare the sorted keys
	if !reflect.DeepEqual(expectedKeys, actualKeys) {
		t.Errorf("Map keys are different")
	}

	// Compare the values for each key
	for _, k := range expectedKeys {
		if expected[k] != actual[k] {
			t.Errorf("Maps are different")
		}
	}
}
