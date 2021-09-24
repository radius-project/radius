// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package containerv1alpha3

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"

	"github.com/Azure/go-autorest/autorest/to"
	"github.com/Azure/radius/pkg/azure/azresources"
	"github.com/Azure/radius/pkg/kubernetes"
	"github.com/Azure/radius/pkg/radlogger"
	"github.com/Azure/radius/pkg/radrp/outputresource"
	"github.com/Azure/radius/pkg/renderers"
	"github.com/Azure/radius/pkg/resourcekinds"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
)

const applicationName = "test-app"
const resourceName = "test-container"
const envVarName1 = "TEST_VAR_1"
const envVarValue1 = "TEST_VALUE_1"
const envVarName2 = "TEST_VAR_2"
const envVarValue2 = 81

func createContext(t *testing.T) context.Context {
	logger, err := radlogger.NewTestLogger(t)
	if err != nil {
		t.Log("Unable to initialize logger")
		return context.Background()
	}
	return logr.NewContext(context.Background(), logger)
}

func makeResource(t *testing.T, properties ContainerProperties) renderers.RendererResource {
	b, err := json.Marshal(&properties)
	require.NoError(t, err)

	definition := map[string]interface{}{}
	err = json.Unmarshal(b, &definition)
	require.NoError(t, err)

	return renderers.RendererResource{
		ApplicationName: applicationName,
		ResourceName:    resourceName,
		ResourceType:    "ContainerComponent",
		Definition:      definition,
	}
}

func makeResourceID(t *testing.T, resourceType string, resourceName string) azresources.ResourceID {
	id, err := azresources.Parse(azresources.MakeID(
		"test-subscription",
		"test-resourcegroup",
		azresources.ResourceType{
			Type: "radius.dev/Application",
			Name: applicationName,
		},
		azresources.ResourceType{
			Type: resourceType,
			Name: resourceName,
		}))
	require.NoError(t, err)

	return id
}

func Test_GetDependencyIDs_Success(t *testing.T) {
	properties := ContainerProperties{
		Connections: map[string]ContainerConnection{
			"A": {
				Kind:   "Http",
				Source: makeResourceID(t, "HttpRoute", "A").ID,
			},
			"B": {
				Kind:   "Http",
				Source: makeResourceID(t, "HttpRoute", "B").ID,
			},
		},
		Container: Container{
			Image: "someimage:latest",
			Ports: map[string]ContainerPort{
				"web": {
					ContainerPort: to.IntPtr(5000),
					Provides:      makeResourceID(t, "HttpRoute", "C").ID,
				},
			},
		},
	}
	resource := makeResource(t, properties)

	renderer := Renderer{}
	ids, err := renderer.GetDependencyIDs(createContext(t), resource)
	require.NoError(t, err)

	expected := []azresources.ResourceID{
		makeResourceID(t, "HttpRoute", "A"),
		makeResourceID(t, "HttpRoute", "B"),
		makeResourceID(t, "HttpRoute", "C"),
	}
	require.ElementsMatch(t, expected, ids)
}

func Test_GetDependencyIDs_InvalidId(t *testing.T) {
	properties := ContainerProperties{
		Connections: map[string]ContainerConnection{
			"A": {
				Kind:   "Http",
				Source: "not a resource id obviously...",
			},
		},
		Container: Container{
			Image: "someimage:latest",
		},
	}
	resource := makeResource(t, properties)

	renderer := Renderer{}
	ids, err := renderer.GetDependencyIDs(createContext(t), resource)
	require.Error(t, err)
	require.Empty(t, ids)
}

// This test verifies most of the 'basics' of rendering a deployment. These verifications are not
// repeated in other tests becase the code is simple for these cases.
//
// If you add minor features, add them here.
func Test_Render_Basic(t *testing.T) {
	properties := ContainerProperties{
		Container: Container{
			Image: "someimage:latest",
			Env: map[string]interface{}{
				envVarName1: envVarValue1,
				envVarName2: envVarValue2,
			},
		},
	}
	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{}

	renderer := Renderer{}
	output, err := renderer.Render(createContext(t), resource, dependencies)
	require.NoError(t, err)
	require.Empty(t, output.ComputedValues)
	require.Empty(t, output.SecretValues)

	labels := kubernetes.MakeDescriptiveLabels(resource.ApplicationName, resource.ResourceName)
	matchLabels := kubernetes.MakeSelectorLabels(resource.ApplicationName, resource.ResourceName)

	t.Run("verify deployment", func(t *testing.T) {
		deployment, outputResource := kubernetes.FindDeployment(output.Resources)
		require.NotNil(t, deployment)

		expectedOutputResource := outputresource.OutputResource{
			Kind:     resourcekinds.Kubernetes,
			LocalID:  outputresource.LocalIDDeployment,
			Deployed: false,
			Managed:  true,
			Type:     outputresource.TypeKubernetes,
			Info: outputresource.K8sInfo{
				Kind:       deployment.TypeMeta.Kind,
				APIVersion: deployment.TypeMeta.APIVersion,
				Name:       deployment.ObjectMeta.Name,
				Namespace:  deployment.ObjectMeta.Namespace,
			},
			Resource: deployment,
		}

		require.Equal(t, outputResource, expectedOutputResource)

		// Only real thing to verify here is the image and the labels
		require.Equal(t, labels, deployment.Labels)
		require.Equal(t, labels, deployment.Spec.Template.Labels)
		require.Equal(t, matchLabels, deployment.Spec.Selector.MatchLabels)

		require.Len(t, deployment.Spec.Template.Spec.Containers, 1)

		container := deployment.Spec.Template.Spec.Containers[0]
		require.Equal(t, resourceName, container.Name)
		require.Equal(t, properties.Container.Image, container.Image)
		require.Equal(t, v1.PullAlways, container.ImagePullPolicy)

		expectedEnv := []v1.EnvVar{
			{Name: envVarName1, Value: envVarValue1},
			{Name: envVarName2, Value: strconv.Itoa(envVarValue2)},
		}
		require.Equal(t, expectedEnv, container.Env)

	})
	require.Len(t, output.Resources, 1)
}

func Test_Render_PortWithoutRoute(t *testing.T) {
	properties := ContainerProperties{
		Container: Container{
			Image: "someimage:latest",
			Ports: map[string]ContainerPort{
				"web": {
					ContainerPort: to.IntPtr(5000),
					Protocol:      "TCP",
				},
			},
		},
	}
	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{}

	renderer := Renderer{}
	output, err := renderer.Render(createContext(t), resource, dependencies)
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
	require.Len(t, output.Resources, 1)
}

func Test_Render_PortConnectedToRoute(t *testing.T) {
	properties := ContainerProperties{
		Container: Container{
			Image: "someimage:latest",
			Ports: map[string]ContainerPort{
				"web": {
					ContainerPort: to.IntPtr(5000),
					Protocol:      "TCP",
					Provides:      makeResourceID(t, "HttpRoute", "A").ID,
				},
			},
		},
	}
	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{}

	renderer := Renderer{}
	output, err := renderer.Render(createContext(t), resource, dependencies)
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

		routeID := makeResourceID(t, "HttpRoute", "A")

		expected := v1.ContainerPort{
			Name:          kubernetes.GetShortenedTargetPortName(routeID.Type() + routeID.Name()),
			ContainerPort: 5000,
			Protocol:      v1.ProtocolTCP,
		}
		require.Equal(t, expected, port)
	})
	require.Len(t, output.Resources, 1)
}

func Test_Render_Connections(t *testing.T) {
	properties := ContainerProperties{
		Connections: map[string]ContainerConnection{
			"A": {
				Kind:   "A",
				Source: makeResourceID(t, "ResourceType", "A").ID,
			},
		},
		Container: Container{
			Image: "someimage:latest",
			Env: map[string]interface{}{
				envVarName1: envVarValue1,
				envVarName2: envVarValue2,
			},
		},
	}
	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{
		(makeResourceID(t, "ResourceType", "A").ID): {
			ResourceID: makeResourceID(t, "ResourceType", "A"),
			Definition: map[string]interface{}{},
			ComputedValues: map[string]interface{}{
				"ComputedKey1": "ComputedValue1",
				"ComputedKey2": 82,
			},
		},
	}

	renderer := Renderer{}
	output, err := renderer.Render(createContext(t), resource, dependencies)
	require.NoError(t, err)
	require.Empty(t, output.ComputedValues)
	require.Empty(t, output.SecretValues)

	labels := kubernetes.MakeDescriptiveLabels(resource.ApplicationName, resource.ResourceName)

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
							Name: resource.ResourceName,
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
							Name: resource.ResourceName,
						},
						Key: "CONNECTION_A_COMPUTEDKEY2",
					},
				},
			},
			{Name: envVarName1, Value: envVarValue1},
			{Name: envVarName2, Value: strconv.Itoa(envVarValue2)},
		}
		require.Equal(t, expectedEnv, container.Env)
	})

	t.Run("verify secret", func(t *testing.T) {
		secret, outputResource := kubernetes.FindSecret(output.Resources)
		require.NotNil(t, secret)

		expectedOutputResource := outputresource.OutputResource{
			Kind:     resourcekinds.Kubernetes,
			LocalID:  outputresource.LocalIDSecret,
			Deployed: false,
			Managed:  true,
			Type:     outputresource.TypeKubernetes,
			Info: outputresource.K8sInfo{
				Kind:       secret.TypeMeta.Kind,
				APIVersion: secret.TypeMeta.APIVersion,
				Name:       secret.ObjectMeta.Name,
				Namespace:  secret.ObjectMeta.Namespace,
			},
			Resource: secret,
		}
		require.Equal(t, outputResource, expectedOutputResource)

		require.Equal(t, resourceName, secret.Name)
		require.Equal(t, applicationName, secret.Namespace)
		require.Equal(t, labels, secret.Labels)
		require.Empty(t, secret.Annotations)

		require.Equal(t, outputResource.LocalID, outputresource.LocalIDSecret)
		require.Len(t, secret.Data, 2)
		require.Equal(t, "ComputedValue1", string(secret.Data["CONNECTION_A_COMPUTEDKEY1"]))
		require.Equal(t, "82", string(secret.Data["CONNECTION_A_COMPUTEDKEY2"]))
	})
	require.Len(t, output.Resources, 2)
}
