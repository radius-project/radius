// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package containerv1alpha3

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/Azure/go-autorest/autorest/to"
	"github.com/go-logr/logr"
	"github.com/google/uuid"
	"github.com/project-radius/radius/pkg/azure/azresources"
	"github.com/project-radius/radius/pkg/azure/radclient"
	"github.com/project-radius/radius/pkg/handlers"
	"github.com/project-radius/radius/pkg/kubernetes"
	"github.com/project-radius/radius/pkg/providers"
	"github.com/project-radius/radius/pkg/radlogger"
	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/renderers"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/project-radius/radius/pkg/resourcemodel"
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
const envVarValue2 = "81"

func createContext(t *testing.T) context.Context {
	logger, err := radlogger.NewTestLogger(t)
	if err != nil {
		t.Log("Unable to initialize logger")
		return context.Background()
	}
	return logr.NewContext(context.Background(), logger)
}

func makeResource(t *testing.T, properties radclient.ContainerProperties) renderers.RendererResource {
	b, err := json.Marshal(&properties)
	require.NoError(t, err)

	definition := map[string]interface{}{}
	err = json.Unmarshal(b, &definition)
	require.NoError(t, err)

	return renderers.RendererResource{
		ApplicationName: applicationName,
		ResourceName:    resourceName,
		ResourceType:    "Container",
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
	testAzureResourceID := makeResourceID(t, "Microsoft.ServiceBus/namespaces", "testAzureResource")
	properties := radclient.ContainerProperties{
		Connections: map[string]*radclient.Connection{
			"A": {
				Kind:   radclient.ConnectionKindHTTP.ToPtr(),
				Source: to.StringPtr(makeResourceID(t, "HttpRoute", "A").ID),
			},
			"B": {
				Kind:   radclient.ConnectionKindHTTP.ToPtr(),
				Source: to.StringPtr(makeResourceID(t, "HttpRoute", "B").ID),
			},
			"testAzureConnection": {
				Kind:   radclient.ConnectionKindAzure.ToPtr(),
				Source: to.StringPtr(testAzureResourceID.ID),
				Roles:  []*string{to.StringPtr("administrator")},
			},
		},
		Container: &radclient.Container{
			Image: to.StringPtr("someimage:latest"),
			Ports: map[string]*radclient.ContainerPort{
				"web": {
					ContainerPort: to.Int32Ptr(5000),
					Provides:      to.StringPtr(makeResourceID(t, "HttpRoute", "C").ID),
				},
			},
			Volumes: map[string]radclient.VolumeClassification{
				"vol1": &radclient.PersistentVolume{
					MountPath: to.StringPtr("/tmpfs"),
					Source:    to.StringPtr(testResourceID),
				},
			},
		},
	}
	resource := makeResource(t, properties)

	renderer := Renderer{}
	radiusResourceIDs, azureResourceIDs, err := renderer.GetDependencyIDs(createContext(t), resource)
	require.NoError(t, err)
	require.Len(t, radiusResourceIDs, 4)
	require.Len(t, azureResourceIDs, 1)

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

	expectedRadiusResourceIDs := []azresources.ResourceID{
		makeResourceID(t, "HttpRoute", "A"),
		makeResourceID(t, "HttpRoute", "B"),
		makeResourceID(t, "HttpRoute", "C"),
		storageID,
	}
	require.ElementsMatch(t, expectedRadiusResourceIDs, radiusResourceIDs)

	expectedAzureResourceIDs := []azresources.ResourceID{
		testAzureResourceID,
	}
	require.ElementsMatch(t, expectedAzureResourceIDs, azureResourceIDs)
}

func Test_GetDependencyIDs_InvalidId(t *testing.T) {
	properties := radclient.ContainerProperties{
		Connections: map[string]*radclient.Connection{
			"A": {
				Kind:   radclient.ConnectionKindHTTP.ToPtr(),
				Source: to.StringPtr("not a resource id obviously..."),
			},
		},
		Container: &radclient.Container{
			Image: to.StringPtr("someimage:latest"),
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
	properties := radclient.ContainerProperties{
		Connections: map[string]*radclient.Connection{
			"AzureResourceTest": {
				Kind:   radclient.ConnectionKindAzure.ToPtr(),
				Source: to.StringPtr("/subscriptions/test-sub-id/providers/Microsoft.ServiceBus/namespaces/testNamespace"),
				Roles:  []*string{to.StringPtr("reader")},
			},
		},
		Container: &radclient.Container{
			Image: to.StringPtr("test-image:latest"),
		},
	}
	resource := makeResource(t, properties)

	renderer := Renderer{}
	ids, azureIDs, err := renderer.GetDependencyIDs(createContext(t), resource)
	require.Error(t, err)
	require.Equal(t, err.Error(), "'subscriptions/test-sub-id/providers/Microsoft.ServiceBus/namespaces/testNamespace' is not a valid resource id")
	require.Empty(t, ids)
	require.Empty(t, azureIDs)
}

// This test verifies most of the 'basics' of rendering a deployment. These verifications are not
// repeated in other tests becase the code is simple for these cases.
//
// If you add minor features, add them here.
func Test_Render_Basic(t *testing.T) {
	properties := radclient.ContainerProperties{
		Container: &radclient.Container{
			Image: to.StringPtr("someimage:latest"),
			Env: map[string]*string{
				envVarName1: to.StringPtr(envVarValue1),
				envVarName2: to.StringPtr(envVarValue2),
			},
		},
	}
	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{}

	renderer := Renderer{}
	output, err := renderer.Render(createContext(t), renderers.RenderOptions{Resource: resource, Dependencies: dependencies})
	require.NoError(t, err)
	require.Empty(t, output.ComputedValues)
	require.Empty(t, output.SecretValues)

	labels := kubernetes.MakeDescriptiveLabels(resource.ApplicationName, resource.ResourceName)
	matchLabels := kubernetes.MakeSelectorLabels(resource.ApplicationName, resource.ResourceName)

	t.Run("verify deployment", func(t *testing.T) {
		deployment, outputResource := kubernetes.FindDeployment(output.Resources)
		require.NotNil(t, deployment)

		expectedOutputResource := outputresource.NewKubernetesOutputResource(resourcekinds.Deployment, outputresource.LocalIDDeployment, deployment, deployment.ObjectMeta)
		require.Equal(t, outputResource, expectedOutputResource)

		// Only real thing to verify here is the image and the labels
		require.Equal(t, labels, deployment.Labels)
		require.Equal(t, labels, deployment.Spec.Template.Labels)
		require.Equal(t, matchLabels, deployment.Spec.Selector.MatchLabels)

		require.Len(t, deployment.Spec.Template.Spec.Containers, 1)

		container := deployment.Spec.Template.Spec.Containers[0]
		require.Equal(t, resourceName, container.Name)
		require.Equal(t, *properties.Container.Image, container.Image)
		require.Equal(t, v1.PullAlways, container.ImagePullPolicy)

		expectedEnv := []v1.EnvVar{
			{Name: envVarName1, Value: envVarValue1},
			{Name: envVarName2, Value: envVarValue2},
		}
		require.Equal(t, expectedEnv, container.Env)

	})
	require.Len(t, output.Resources, 1)
}

func Test_Render_PortWithoutRoute(t *testing.T) {
	properties := radclient.ContainerProperties{
		Container: &radclient.Container{
			Image: to.StringPtr("someimage:latest"),
			Ports: map[string]*radclient.ContainerPort{
				"web": {
					ContainerPort: to.Int32Ptr(5000),
					Protocol:      radclient.ContainerPortProtocolTCP.ToPtr(),
				},
			},
		},
	}
	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{}

	renderer := Renderer{}
	output, err := renderer.Render(createContext(t), renderers.RenderOptions{Resource: resource, Dependencies: dependencies})
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
	properties := radclient.ContainerProperties{
		Container: &radclient.Container{
			Image: to.StringPtr("someimage:latest"),
			Ports: map[string]*radclient.ContainerPort{
				"web": {
					ContainerPort: to.Int32Ptr(5000),
					Protocol:      radclient.ContainerPortProtocolTCP.ToPtr(),
					Provides:      to.StringPtr(makeResourceID(t, "HttpRoute", "A").ID),
				},
			},
		},
	}
	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{}

	renderer := Renderer{}
	output, err := renderer.Render(createContext(t), renderers.RenderOptions{Resource: resource, Dependencies: dependencies})
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
	properties := radclient.ContainerProperties{
		Connections: map[string]*radclient.Connection{
			"A": {
				Kind:   radclient.ConnectionKindHTTP.ToPtr(),
				Source: to.StringPtr(makeResourceID(t, "ResourceType", "A").ID),
			},
		},
		Container: &radclient.Container{
			Image: to.StringPtr("someimage:latest"),
			Env: map[string]*string{
				envVarName1: to.StringPtr(envVarValue1),
				envVarName2: to.StringPtr(envVarValue2),
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
	output, err := renderer.Render(createContext(t), renderers.RenderOptions{Resource: resource, Dependencies: dependencies})
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
		require.Equal(t, *properties.Container.Image, container.Image)
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
			{Name: envVarName2, Value: envVarValue2},
		}
		require.Equal(t, expectedEnv, container.Env)
	})

	t.Run("verify secret", func(t *testing.T) {
		secret, outputResource := kubernetes.FindSecret(output.Resources)
		require.NotNil(t, secret)

		expectedOutputResource := outputresource.NewKubernetesOutputResource(resourcekinds.Secret, outputresource.LocalIDSecret, secret, secret.ObjectMeta)
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
	properties := radclient.ContainerProperties{
		Connections: map[string]*radclient.Connection{
			"A": {
				Kind:   radclient.ConnectionKindHTTP.ToPtr(),
				Source: to.StringPtr(makeResourceID(t, "ResourceType", "A").ID),
			},
		},
		Container: &radclient.Container{
			Image: to.StringPtr("someimage:latest"),
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
				"TargetLocalID": resourcemodel.NewARMIdentity(
					&resourcemodel.ResourceType{
						Type:     "dummy",
						Provider: providers.ProviderAzure,
					},
					makeResourceID(t, "TargetResourceType", "TargetResource").ID,
					"2020-01-01"),
			},
		},
	}

	renderer := Renderer{
		RoleAssignmentMap: map[radclient.ConnectionKind]RoleAssignmentData{
			radclient.ConnectionKindHTTP: {
				LocalID:   "TargetLocalID",
				RoleNames: []string{"TestRole1", "TestRole2"},
			},
		},
	}
	output, err := renderer.Render(createContext(t), renderers.RenderOptions{Resource: resource, Dependencies: dependencies})
	require.NoError(t, err)
	require.Empty(t, output.ComputedValues)
	require.Empty(t, output.SecretValues)
	require.Len(t, output.Resources, 6)

	resourceMap := outputResourcesToKindMap(output.Resources)

	// We're just verifying the role assignments and related things, we'll ignore kubernetes types.
	matches := resourceMap[resourcekinds.Deployment]
	require.Equal(t, 1, len(matches))

	matches = resourceMap[resourcekinds.Secret]
	require.Equal(t, 1, len(matches))

	matches = resourceMap[resourcekinds.AzureRoleAssignment]
	require.Equal(t, 2, len(matches))
	expected := []outputresource.OutputResource{
		{
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.AzureRoleAssignment,
				Provider: providers.ProviderAzure,
			},
			LocalID:  outputresource.GenerateLocalIDForRoleAssignment(makeResourceID(t, "TargetResourceType", "TargetResource").ID, "TestRole1"),
			Deployed: false,
			Resource: map[string]string{
				handlers.RoleNameKey:         "TestRole1",
				handlers.RoleAssignmentScope: makeResourceID(t, "TargetResourceType", "TargetResource").ID,
			},
			Dependencies: []outputresource.Dependency{
				{
					LocalID: outputresource.LocalIDUserAssignedManagedIdentity,
				},
			},
		},
		{
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.AzureRoleAssignment,
				Provider: providers.ProviderAzure,
			},
			LocalID:  outputresource.GenerateLocalIDForRoleAssignment(makeResourceID(t, "TargetResourceType", "TargetResource").ID, "TestRole2"),
			Deployed: false,
			Resource: map[string]string{
				handlers.RoleNameKey:         "TestRole2",
				handlers.RoleAssignmentScope: makeResourceID(t, "TargetResourceType", "TargetResource").ID,
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
	require.Equal(t, 1, len(matches))

	expected = []outputresource.OutputResource{
		{
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.AzureUserAssignedManagedIdentity,
				Provider: providers.ProviderAzure,
			},
			LocalID:  outputresource.LocalIDUserAssignedManagedIdentity,
			Deployed: false,
			Resource: map[string]string{
				handlers.UserAssignedIdentityNameKey: resource.ApplicationName + "-" + resource.ResourceName + "-msi",
			},
		},
	}
	require.ElementsMatch(t, expected, matches)

	matches = resourceMap[resourcekinds.AzurePodIdentity]
	require.Equal(t, 1, len(matches))

	expected = []outputresource.OutputResource{
		{
			LocalID:  outputresource.LocalIDAADPodIdentity,
			Deployed: false,
			Resource: map[string]string{
				handlers.PodIdentityNameKey: fmt.Sprintf("podid-%s-%s", strings.ToLower(resource.ApplicationName), strings.ToLower(resource.ResourceName)),
				handlers.PodNamespaceKey:    resource.ApplicationName,
			},
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.AzurePodIdentity,
				Provider: providers.ProviderAzureKubernetesService,
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

func Test_Render_AzureConnection(t *testing.T) {
	testARMID := makeResourceID(t, "ResourceType", "test-azure-resource").ID
	expectedRole := "administrator"
	properties := radclient.ContainerProperties{
		Connections: map[string]*radclient.Connection{
			"testAzureResourceConnection": {
				Kind:   radclient.ConnectionKindAzure.ToPtr(),
				Source: to.StringPtr(testARMID),
				Roles:  []*string{to.StringPtr(expectedRole)},
			},
		},
		Container: &radclient.Container{
			Image: to.StringPtr("test-image:latest"),
		},
	}
	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{}

	renderer := Renderer{
		RoleAssignmentMap: map[radclient.ConnectionKind]RoleAssignmentData{
			radclient.ConnectionKindAzure: {},
		},
	}
	output, err := renderer.Render(createContext(t), renderers.RenderOptions{Resource: resource, Dependencies: dependencies})
	require.NoError(t, err)
	require.Empty(t, output.ComputedValues)
	require.Empty(t, output.SecretValues)
	require.Len(t, output.Resources, 4)

	kindResourceMap := outputResourcesToKindMap(output.Resources)

	_, ok := kindResourceMap[resourcekinds.Deployment]
	require.Equal(t, true, ok)

	roleOutputResource, ok := kindResourceMap[resourcekinds.AzureRoleAssignment]
	require.Equal(t, true, ok)
	require.Len(t, roleOutputResource, 1)
	expected := []outputresource.OutputResource{
		{
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.AzureRoleAssignment,
				Provider: providers.ProviderAzure,
			},
			LocalID:  outputresource.GenerateLocalIDForRoleAssignment(testARMID, expectedRole),
			Deployed: false,
			Resource: map[string]string{
				handlers.RoleNameKey:         expectedRole,
				handlers.RoleAssignmentScope: testARMID,
			},
			Dependencies: []outputresource.Dependency{
				{
					LocalID: outputresource.LocalIDUserAssignedManagedIdentity,
				},
			},
		},
	}
	require.ElementsMatch(t, expected, roleOutputResource)

	outputResource := kindResourceMap[resourcekinds.AzureUserAssignedManagedIdentity]
	require.Len(t, outputResource, 1)

	outputResource = kindResourceMap[resourcekinds.AzurePodIdentity]
	require.Len(t, outputResource, 1)
}

func Test_Render_AzureConnectionMissingRoleError(t *testing.T) {
	testARMID := makeResourceID(t, "ResourceType", "test-azure-resource").ID
	properties := radclient.ContainerProperties{
		Connections: map[string]*radclient.Connection{
			"testAzureResourceConnection": {
				Kind:   radclient.ConnectionKindAzure.ToPtr(),
				Source: to.StringPtr(testARMID),
			},
		},
		Container: &radclient.Container{
			Image: to.StringPtr("test-image:latest"),
		},
	}
	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{}

	renderer := Renderer{
		RoleAssignmentMap: map[radclient.ConnectionKind]RoleAssignmentData{
			radclient.ConnectionKindAzure: {},
		},
	}
	_, err := renderer.Render(createContext(t), renderers.RenderOptions{Resource: resource, Dependencies: dependencies})
	require.Error(t, err)
	require.Equal(t, "rbac permissions are required to access Azure connections", err.Error())
}

func Test_Render_EphemeralVolumes(t *testing.T) {
	const tempVolName = "TempVolume"
	const tempVolMountPath = "/tmpfs"
	properties := radclient.ContainerProperties{
		Container: &radclient.Container{
			Image: to.StringPtr("someimage:latest"),
			Env: map[string]*string{
				envVarName1: to.StringPtr(envVarValue1),
				envVarName2: to.StringPtr(envVarValue2),
			},
			Volumes: map[string]radclient.VolumeClassification{
				tempVolName: &radclient.EphemeralVolume{
					MountPath:    to.StringPtr(tempVolMountPath),
					ManagedStore: radclient.EphemeralVolumeManagedStoreMemory.ToPtr(),
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
	output, err := renderer.Render(createContext(t), renderers.RenderOptions{Resource: resource, Dependencies: dependencies})
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
	const tempVolName = "TempVolume"
	const tempVolMountPath = "/tmpfs"
	const testShareName = "myshare"
	testResourceID := fmt.Sprintf("/subscriptions/%s/resourceGroups/test/providers/Microsoft.Storage/storageaccounts/testaccount/fileservices/default/share/%s", uuid.New(), testShareName)

	properties := radclient.ContainerProperties{
		Container: &radclient.Container{
			Image: to.StringPtr("someimage:latest"),
			Volumes: map[string]radclient.VolumeClassification{
				tempVolName: &radclient.PersistentVolume{
					MountPath: to.StringPtr(tempVolMountPath),
					Source:    to.StringPtr(testResourceID),
				},
			},
		},
	}
	resource := makeResource(t, properties)
	resourceID, _ := azresources.Parse(testResourceID)
	dependencies := map[string]renderers.RendererDependency{
		testResourceID: {
			ResourceID: resourceID,
			Definition: map[string]interface{}{
				"kind": "azure.com.fileshare",
			},
			ComputedValues: map[string]interface{}{
				"azurestorageaccountname": "accountname",
				"azurestorageaccountkey":  "storagekey",
			},
		},
	}

	renderer := Renderer{}
	renderOutput, err := renderer.Render(createContext(t), renderers.RenderOptions{Resource: resource, Dependencies: dependencies})
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

func Test_Render_PersistentAzureKeyVaultVolumes(t *testing.T) {
	const tempVolName = "TempVolume"
	const tempVolMountPath = "/tmpfs"
	testResourceID := "/subscriptions/sub/resourceGroups/rg/providers/Microsoft.KeyVault/vaults/azure-kv"

	properties := radclient.ContainerProperties{
		Container: &radclient.Container{
			Image: to.StringPtr("someimage:latest"),
			Volumes: map[string]radclient.VolumeClassification{
				tempVolName: &radclient.PersistentVolume{
					MountPath: to.StringPtr(tempVolMountPath),
					Source:    to.StringPtr(testResourceID),
				},
			},
		},
	}
	resource := makeResource(t, properties)
	resourceID, _ := azresources.Parse(testResourceID)
	dependencies := map[string]renderers.RendererDependency{
		testResourceID: {
			ResourceID: resourceID,
			Definition: map[string]interface{}{
				"kind":     "azure.com.keyvault",
				"resource": testResourceID,
				"secrets": map[string]*radclient.SecretObjectProperties{
					"mysecret": {
						Name: to.StringPtr("secret1"),
					},
				},
				"keys": map[string]*radclient.KeyObjectProperties{
					"mykey": {
						Name: to.StringPtr("key1"),
					},
				},
			},
			OutputResources: map[string]resourcemodel.ResourceIdentity{
				outputresource.LocalIDSecretProviderClass: resourcemodel.ResourceIdentity{
					ResourceType: &resourcemodel.ResourceType{
						Type:     resourcekinds.SecretProviderClass,
						Provider: providers.ProviderKubernetes,
					},
					Data: resourcemodel.KubernetesIdentity{
						Kind:       "SecretProviderClass",
						APIVersion: "secrets-store.csi.x-k8s.io/v1alpha1",
						Name:       "test-volume-sp",
						Namespace:  "test-ns",
					},
				},
			},
		},
	}

	renderer := Renderer{}
	renderOutput, err := renderer.Render(createContext(t), renderers.RenderOptions{Resource: resource, Dependencies: dependencies})
	require.NoError(t, err)
	require.Lenf(t, renderOutput.Resources, 5, "expected 5 output resource, instead got %+v", len(renderOutput.Resources))

	// Verify Managed Identity
	require.Equal(t, outputresource.LocalIDUserAssignedManagedIdentity, renderOutput.Resources[0].LocalID, "expected output resource of kind user assigned managed identity instead got :%v", renderOutput.Resources[0].LocalID)

	// Verify Role Assignments
	require.True(t, strings.Contains(renderOutput.Resources[1].LocalID, "RoleAssignment"), "expected output resource of kind role assignment instead got :%v", renderOutput.Resources[0].LocalID)
	require.True(t, strings.Contains(renderOutput.Resources[2].LocalID, "RoleAssignment"), "expected output resource of kind role assignment instead got :%v", renderOutput.Resources[0].LocalID)

	// Verify AAD Pod Identity
	require.Equal(t, outputresource.LocalIDAADPodIdentity, renderOutput.Resources[3].LocalID, "expected output resource of kind aad pod identity instead got :%v", renderOutput.Resources[1].LocalID)

	// Verify deployment
	require.Equal(t, outputresource.LocalIDDeployment, renderOutput.Resources[4].LocalID, "expected output resource of kind deployment instead got :%v", renderOutput.Resources[2].LocalID)

	// Verify volume spec
	volumes := renderOutput.Resources[4].Resource.(*appsv1.Deployment).Spec.Template.Spec.Volumes
	require.Lenf(t, volumes, 1, "expected 1 volume, instead got %+v", len(volumes))
	require.Equal(t, tempVolName, volumes[0].Name)
	require.Equal(t, "secrets-store.csi.k8s.io", volumes[0].VolumeSource.CSI.Driver, "expected volumesource azurefile to be not nil")
	require.Equal(t, "test-volume-sp", volumes[0].VolumeSource.CSI.VolumeAttributes["secretProviderClass"], "expected secret provider class to match the input test-volume-sp")
	require.Equal(t, true, *volumes[0].VolumeSource.CSI.ReadOnly, "expected readonly attribute to be true")

	// Verify volume mount spec
	volumeMounts := renderOutput.Resources[4].Resource.(*appsv1.Deployment).Spec.Template.Spec.Containers[0].VolumeMounts
	require.Lenf(t, volumeMounts, 1, "expected 1 volume mount, instead got %+v", len(volumeMounts))
	require.Equal(t, tempVolMountPath, volumeMounts[0].MountPath)
	require.Equal(t, tempVolName, volumeMounts[0].Name)
	require.Equal(t, true, volumeMounts[0].ReadOnly)
}

func outputResourcesToKindMap(resources []outputresource.OutputResource) map[string][]outputresource.OutputResource {
	results := map[string][]outputresource.OutputResource{}
	for _, resource := range resources {
		matches := results[resource.ResourceType.Type]
		matches = append(matches, resource)
		results[resource.ResourceType.Type] = matches
	}

	return results
}

func Test_Render_ReadinessProbeHttpGet(t *testing.T) {
	properties := radclient.ContainerProperties{
		Container: &radclient.Container{
			Image: to.StringPtr("someimage:latest"),
			Env: map[string]*string{
				envVarName1: to.StringPtr(envVarValue1),
				envVarName2: to.StringPtr(envVarValue2),
			},
			ReadinessProbe: &radclient.HTTPGetHealthProbeProperties{
				Path:                to.StringPtr("/healthz"),
				ContainerPort:       to.Int32Ptr(8080),
				Headers:             map[string]*string{"header1": to.StringPtr("value1")},
				InitialDelaySeconds: to.Float32Ptr(30),
				FailureThreshold:    to.Float32Ptr(10),
				PeriodSeconds:       to.Float32Ptr(2),
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
	output, err := renderer.Render(createContext(t), renderers.RenderOptions{Resource: resource, Dependencies: dependencies})
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
	properties := radclient.ContainerProperties{
		Container: &radclient.Container{
			Image: to.StringPtr("someimage:latest"),
			ReadinessProbe: &radclient.TCPHealthProbeProperties{
				ContainerPort:       to.Int32Ptr(8080),
				InitialDelaySeconds: to.Float32Ptr(30),
				FailureThreshold:    to.Float32Ptr(10),
				PeriodSeconds:       to.Float32Ptr(2),
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
	output, err := renderer.Render(createContext(t), renderers.RenderOptions{Resource: resource, Dependencies: dependencies})
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
	properties := radclient.ContainerProperties{
		Container: &radclient.Container{
			Image: to.StringPtr("someimage:latest"),
			LivenessProbe: &radclient.ExecHealthProbeProperties{
				Command:             to.StringPtr("a b c"),
				InitialDelaySeconds: to.Float32Ptr(30),
				FailureThreshold:    to.Float32Ptr(10),
				PeriodSeconds:       to.Float32Ptr(2),
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
	output, err := renderer.Render(createContext(t), renderers.RenderOptions{Resource: resource, Dependencies: dependencies})
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
	properties := radclient.ContainerProperties{
		Container: &radclient.Container{
			Image: to.StringPtr("someimage:latest"),
			LivenessProbe: &radclient.ExecHealthProbeProperties{
				Command: to.StringPtr("a b c"),
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
	output, err := renderer.Render(createContext(t), renderers.RenderOptions{Resource: resource, Dependencies: dependencies})
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
