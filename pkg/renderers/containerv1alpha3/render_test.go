// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package containerv1alpha3

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/Azure/go-autorest/autorest/to"
	"github.com/Azure/radius/pkg/azure/azresources"
	"github.com/Azure/radius/pkg/handlers"
	"github.com/Azure/radius/pkg/kubernetes"
	"github.com/Azure/radius/pkg/radlogger"
	"github.com/Azure/radius/pkg/radrp/outputresource"
	"github.com/Azure/radius/pkg/renderers"
	"github.com/Azure/radius/pkg/resourcekinds"
	"github.com/Azure/radius/pkg/resourcemodel"
	"github.com/go-logr/logr"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
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
	testResourceID := "/subscriptions/test-sub-id/resourceGroups/test-rg/providers/Microsoft.Storage/storageaccounts/testaccount/fileservices/default/shares/testShareName"
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
			Volumes: map[string]map[string]interface{}{
				"vol1": {
					"kind":      "persistent",
					"mountPath": "/tmpfs",
					"source":    testResourceID,
				},
			},
		},
	}
	resource := makeResource(t, properties)

	renderer := Renderer{}
	ids, err := renderer.GetDependencyIDs(createContext(t), resource)
	require.NoError(t, err)

	storageID, _ := azresources.Parse(azresources.MakeID(
		"test-sub-id",
		"test-rg",
		azresources.ResourceType{
			Type: "Microsoft.Storage/storageaccounts",
			Name: "testaccount",
		},
		azresources.ResourceType{
			Type: "fileservices",
			Name: "default",
		},
		azresources.ResourceType{
			Type: "shares",
			Name: "testShareName",
		}))

	expected := []azresources.ResourceID{
		makeResourceID(t, "HttpRoute", "A"),
		makeResourceID(t, "HttpRoute", "B"),
		makeResourceID(t, "HttpRoute", "C"),
		storageID,
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

		expectedOutputResource := outputresource.NewKubernetesOutputResource(outputresource.LocalIDDeployment, deployment, deployment.ObjectMeta)
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

	labels := kubernetes.MakeDescriptiveLabels(resource.ApplicationName, resource.ResourceName)
	podLabels := kubernetes.MakeDescriptiveLabels(resource.ApplicationName, resource.ResourceName)
	podLabels["radius.dev/route-http-a"] = "true"

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

		routeID := makeResourceID(t, "HttpRoute", "A")

		expected := v1.ContainerPort{
			Name:          kubernetes.GetShortenedTargetPortName(resource.ApplicationName + "HttpRoute" + routeID.Name()),
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

		expectedOutputResource := outputresource.NewKubernetesOutputResource(outputresource.LocalIDSecret, secret, secret.ObjectMeta)
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

func Test_Render_ConnectionWithRoleAssignment(t *testing.T) {
	properties := ContainerProperties{
		Connections: map[string]ContainerConnection{
			"A": {
				Kind:   "A",
				Source: makeResourceID(t, "ResourceType", "A").ID,
			},
		},
		Container: Container{
			Image: "someimage:latest",
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
			OutputResources: map[string]resourcemodel.ResourceIdentity{
				// This is the resource that the role assignments target!
				"TargetLocalID": resourcemodel.NewARMIdentity(makeResourceID(t, "TargetResourceType", "TargetResource").ID, "2020-01-01"),
			},
		},
	}

	renderer := Renderer{
		RoleAssignmentMap: map[string]RoleAssignmentData{
			"A": {
				LocalID:   "TargetLocalID",
				RoleNames: []string{"TestRole1", "TestRole2"},
			},
		},
	}
	output, err := renderer.Render(createContext(t), resource, dependencies)
	require.NoError(t, err)
	require.Empty(t, output.ComputedValues)
	require.Empty(t, output.SecretValues)
	require.Len(t, output.Resources, 6)

	resourceMap := outputResourcesToKindMap(output.Resources)

	// We're just verifying the role assignments and related things, we'll ignore kubernetes types.
	matches := resourceMap[resourcekinds.Kubernetes]
	require.Len(t, matches, 2)

	matches = resourceMap[resourcekinds.AzureRoleAssignment]
	require.Len(t, matches, 2)
	expected := []outputresource.OutputResource{
		{
			ResourceKind: resourcekinds.AzureRoleAssignment,
			LocalID:      outputresource.GenerateLocalIDForRoleAssignment(makeResourceID(t, "TargetResourceType", "TargetResource").ID, "TestRole1"),
			Managed:      true,
			Deployed:     false,
			Resource: map[string]string{
				handlers.RoleNameKey:             "TestRole1",
				handlers.RoleAssignmentTargetKey: makeResourceID(t, "TargetResourceType", "TargetResource").ID,
			},
			Dependencies: []outputresource.Dependency{
				{
					LocalID: outputresource.LocalIDUserAssignedManagedIdentity,
				},
			},
		},
		{
			ResourceKind: resourcekinds.AzureRoleAssignment,
			LocalID:      outputresource.GenerateLocalIDForRoleAssignment(makeResourceID(t, "TargetResourceType", "TargetResource").ID, "TestRole2"),
			Managed:      true,
			Deployed:     false,
			Resource: map[string]string{
				handlers.RoleNameKey:             "TestRole2",
				handlers.RoleAssignmentTargetKey: makeResourceID(t, "TargetResourceType", "TargetResource").ID,
			},
			Dependencies: []outputresource.Dependency{
				{
					LocalID: outputresource.LocalIDUserAssignedManagedIdentity,
				},
			},
		},
	}
	require.ElementsMatch(t, expected, matches)

	matches = resourceMap[resourcekinds.AzureUserAssignedManagedIdentity]
	require.Len(t, matches, 1)

	expected = []outputresource.OutputResource{
		{
			ResourceKind: resourcekinds.AzureUserAssignedManagedIdentity,
			LocalID:      outputresource.LocalIDUserAssignedManagedIdentity,
			Deployed:     false,
			Managed:      true,
			Resource: map[string]string{
				handlers.ManagedKey:                  "true",
				handlers.UserAssignedIdentityNameKey: resource.ApplicationName + "-" + resource.ResourceName + "-msi",
			},
		},
	}
	require.ElementsMatch(t, expected, matches)

	matches = resourceMap[resourcekinds.AzurePodIdentity]
	require.Len(t, matches, 1)

	expected = []outputresource.OutputResource{
		{
			LocalID:      outputresource.LocalIDAADPodIdentity,
			ResourceKind: resourcekinds.AzurePodIdentity,
			Managed:      true,
			Deployed:     false,
			Resource: map[string]string{
				handlers.ManagedKey:         "true",
				handlers.PodIdentityNameKey: fmt.Sprintf("podid-%s-%s", strings.ToLower(resource.ApplicationName), strings.ToLower(resource.ResourceName)),
				handlers.PodNamespaceKey:    resource.ApplicationName,
			},
			Dependencies: []outputresource.Dependency{
				{
					LocalID: outputresource.LocalIDUserAssignedManagedIdentity,
				},
				{
					LocalID: outputresource.GenerateLocalIDForRoleAssignment(makeResourceID(t, "TargetResourceType", "TargetResource").ID, "TestRole1"),
				},
				{
					LocalID: outputresource.GenerateLocalIDForRoleAssignment(makeResourceID(t, "TargetResourceType", "TargetResource").ID, "TestRole2"),
				},
			},
		},
	}
	require.ElementsMatch(t, expected, matches)
}

func Test_Render_EphemeralVolumes(t *testing.T) {
	const tempVolName = "TempVolume"
	const tempVolMountPath = "/tmpfs"
	properties := ContainerProperties{
		Container: Container{
			Image: "someimage:latest",
			Env: map[string]interface{}{
				envVarName1: envVarValue1,
				envVarName2: envVarValue2,
			},
			Volumes: map[string]map[string]interface{}{
				tempVolName: {
					"kind":         "ephemeral",
					"mountPath":    tempVolMountPath,
					"managedStore": "memory",
				},
			},
		},
	}
	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{
		(makeResourceID(t, "ResourceType", "A").ID): {
			ResourceID:     makeResourceID(t, "ResourceType", "A"),
			Definition:     map[string]interface{}{},
			ComputedValues: map[string]interface{}{},
		},
	}
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

func Test_Render_PersistentVolumes(t *testing.T) {
	const tempVolName = "TempVolume"
	const tempVolMountPath = "/tmpfs"
	const testShareName = "myshare"
	testResourceID := fmt.Sprintf("/subscriptions/%s/resourceGroups/test/providers/Microsoft.Storage/storageaccounts/testaccount/fileservices/default/share/%s", uuid.New(), testShareName)

	properties := ContainerProperties{
		Container: Container{
			Image: "someimage:latest",
			Volumes: map[string]map[string]interface{}{
				tempVolName: {
					"kind":      "persistent",
					"mountPath": tempVolMountPath,
					"source":    testResourceID,
				},
			},
		},
	}
	resource := makeResource(t, properties)
	resourceID, _ := azresources.Parse(testResourceID)
	dependencies := map[string]renderers.RendererDependency{
		testResourceID: {
			ResourceID: resourceID,
			Definition: map[string]interface{}{},
			ComputedValues: map[string]interface{}{
				"azurestorageaccountname": "accountname",
				"azurestorageaccountkey":  "storagekey",
			},
		},
	}

	renderer := Renderer{}
	renderOutput, err := renderer.Render(createContext(t), resource, dependencies)
	require.Lenf(t, renderOutput.Resources, 2, "expected 2 output resource, instead got %+v", len(renderOutput.Resources))

	// Verify deployment
	require.Equal(t, outputresource.LocalIDDeployment, renderOutput.Resources[0].LocalID, "expected output resource of kind deployment instead got :%v", renderOutput.Resources[0].LocalID)
	volumes := renderOutput.Resources[0].Resource.(*appsv1.Deployment).Spec.Template.Spec.Volumes
	require.Lenf(t, volumes, 1, "expected 1 volume, instead got %+v", len(volumes))
	require.Equal(t, tempVolName, volumes[0].Name)
	require.NotNil(t, volumes[0].VolumeSource.AzureFile, "expected volumesource azurefile to be not nil")
	require.Equal(t, volumes[0].VolumeSource.AzureFile.SecretName, resourceName)
	require.Equal(t, volumes[0].VolumeSource.AzureFile.ShareName, testShareName)

	// Verify Kubernetes secret
	require.Equal(t, outputresource.LocalIDSecret, renderOutput.Resources[1].LocalID, "expected output resource of kind secret instead got :%v", renderOutput.Resources[0].LocalID)
	secret := renderOutput.Resources[1].Resource.(*corev1.Secret)
	require.Lenf(t, secret.Data, 2, "expected 2 secret key-value pairs, instead got %+v", len(secret.Data))
	require.NoError(t, err)
}

func outputResourcesToKindMap(resources []outputresource.OutputResource) map[string][]outputresource.OutputResource {
	results := map[string][]outputresource.OutputResource{}
	for _, resource := range resources {
		matches := results[resource.ResourceKind]
		matches = append(matches, resource)
		results[resource.ResourceKind] = matches
	}

	return results
}

func Test_Render_ReadinessProbeHttpGet(t *testing.T) {
	properties := ContainerProperties{
		Container: Container{
			Image: "someimage:latest",
			Env: map[string]interface{}{
				envVarName1: envVarValue1,
				envVarName2: envVarValue2,
			},
			ReadinessProbe: map[string]interface{}{
				"kind":                "httpGet",
				"path":                "/healthz",
				"containerPort":       8080,
				"headers":             map[string]string{"header1": "value1"},
				"initialDelaySeconds": to.IntPtr(30),
				"failureThreshold":    to.IntPtr(10),
				"periodSeconds":       to.IntPtr(2),
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
			Handler: v1.Handler{
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
	properties := ContainerProperties{
		Container: Container{
			Image: "someimage:latest",
			Env:   map[string]interface{}{},
			ReadinessProbe: map[string]interface{}{
				"kind":                "tcp",
				"containerPort":       8080,
				"initialDelaySeconds": to.IntPtr(30),
				"failureThreshold":    to.IntPtr(10),
				"periodSeconds":       to.IntPtr(2),
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
			Handler: v1.Handler{
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
	properties := ContainerProperties{
		Container: Container{
			Image: "someimage:latest",
			Env:   map[string]interface{}{},
			LivenessProbe: map[string]interface{}{
				"kind":                "exec",
				"command":             "a b c",
				"initialDelaySeconds": to.IntPtr(30),
				"failureThreshold":    to.IntPtr(10),
				"periodSeconds":       to.IntPtr(2),
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
			Handler: v1.Handler{
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
	properties := ContainerProperties{
		Container: Container{
			Image: "someimage:latest",
			Env:   map[string]interface{}{},
			LivenessProbe: map[string]interface{}{
				"kind":    "exec",
				"command": "a b c",
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

	t.Run("verify deployment", func(t *testing.T) {
		deployment, _ := kubernetes.FindDeployment(output.Resources)
		require.NotNil(t, deployment)

		require.Len(t, deployment.Spec.Template.Spec.Containers, 1)

		container := deployment.Spec.Template.Spec.Containers[0]
		require.Equal(t, resourceName, container.Name)

		expectedLivenessProbe := &v1.Probe{
			InitialDelaySeconds: DefaultInitialDelaySeconds,
			FailureThreshold:    DefaultFailureThreshold,
			PeriodSeconds:       DefaultPeriodSeconds,
			Handler: v1.Handler{
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
