// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package containerv1alpha1

import (
	"context"
	"strings"
	"testing"

	"github.com/Azure/radius/pkg/kubernetes"
	"github.com/Azure/radius/pkg/model/components"
	"github.com/Azure/radius/pkg/radlogger"
	"github.com/Azure/radius/pkg/radrp/outputresource"
	"github.com/Azure/radius/pkg/workloads"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const testAppName = "test-app"
const testContainerComponentName = "test-container"

func createContext(t *testing.T) context.Context {
	logger, err := radlogger.NewTestLogger(t)
	if err != nil {
		t.Log("Unable to initialize logger")
		return context.Background()
	}
	return logr.NewContext(context.Background(), logger)
}

func TestAllocateBindings_NoHTTPBinding(t *testing.T) {
	ctx := createContext(t)
	renderer := &Renderer{}

	w := workloads.InstantiatedWorkload{
		Application: testAppName,
		Name:        testContainerComponentName,
		Workload: components.GenericComponent{
			Name: testContainerComponentName,
			Kind: Kind,
			Run: map[string]interface{}{
				"container": map[string]interface{}{
					"image": "test/test-image:latest",
				},
			},
			Bindings: map[string]components.GenericBinding{
				"test-binding": {
					Kind: "test", // will be ignored
				},
			},
		},
	}

	bindings, err := renderer.AllocateBindings(ctx, w, nil)
	require.NoError(t, err)

	require.Len(t, bindings, 0)
}

func TestAllocateBindings_HTTPBindings(t *testing.T) {
	ctx := createContext(t)
	renderer := &Renderer{}

	w := workloads.InstantiatedWorkload{
		Application: testAppName,
		Name:        testContainerComponentName,
		Workload: components.GenericComponent{
			Name: testContainerComponentName,
			Kind: Kind,
			Run: map[string]interface{}{
				"container": map[string]interface{}{
					"image": "test/test-image:latest",
				},
			},
			Bindings: map[string]components.GenericBinding{
				"test-binding": {
					Kind: "http",
					AdditionalProperties: map[string]interface{}{
						"port":       2000,
						"targetPort": 3000,
					},
				},
				"test-binding2": {
					Kind: "http",
					AdditionalProperties: map[string]interface{}{
						// Using default value for port
						"targetPort": 5000,
					},
				},
			},
		},
	}

	bindings, err := renderer.AllocateBindings(ctx, w, nil)
	require.NoError(t, err)

	expected := map[string]components.BindingState{
		"test-binding": {
			Component: testContainerComponentName,
			Binding:   "test-binding",
			Kind:      "http",
			Properties: map[string]interface{}{
				"host":   "test-container.test-app.svc.cluster.local",
				"port":   "2000",
				"scheme": "http",
				"uri":    "http://test-container.test-app.svc.cluster.local:2000",
			},
		},
		"test-binding2": {
			Component: testContainerComponentName,
			Binding:   "test-binding2",
			Kind:      "http",
			Properties: map[string]interface{}{
				"host":   "test-container.test-app.svc.cluster.local",
				"port":   "80",
				"scheme": "http",
				"uri":    "http://test-container.test-app.svc.cluster.local",
			},
		},
	}
	require.Equal(t, expected, bindings)

}

func TestRender_Success_DefaultPort(t *testing.T) {
	ctx := createContext(t)
	renderer := &Renderer{}

	targetPort := 3000
	w := workloads.InstantiatedWorkload{
		Application: testAppName,
		Name:        testContainerComponentName,
		Workload: components.GenericComponent{
			Name: testContainerComponentName,
			Kind: Kind,
			Run: map[string]interface{}{
				"container": map[string]interface{}{
					"image": "test/test-image:latest",
					"env": map[string]string{
						"APPLICATION_NAME": "Awesome Test Application",
					},
				},
			},
			Bindings: map[string]components.GenericBinding{
				"test-binding": {
					Kind: "http",
					AdditionalProperties: map[string]interface{}{
						"targetPort": targetPort,
					},
				},
			},
		},
	}

	resources, err := renderer.Render(ctx, w)
	require.NoError(t, err)
	require.Len(t, resources, 2)

	deployment, deploymentOutputResource := kubernetes.FindDeployment(resources)
	require.NotNil(t, deployment)

	service, serviceOutputResource := kubernetes.FindService(resources)
	require.NotNil(t, service)

	labels := kubernetes.MakeDescriptiveLabels(testAppName, testContainerComponentName)
	matchLabels := kubernetes.MakeSelectorLabels(testAppName, testContainerComponentName)

	t.Run("verify deployment", func(t *testing.T) {
		require.Equal(t, testContainerComponentName, deployment.Name)
		require.Equal(t, testAppName, deployment.Namespace)
		require.Equal(t, labels, deployment.Labels)
		require.Empty(t, deployment.Annotations)

		spec := deployment.Spec
		require.Equal(t, matchLabels, spec.Selector.MatchLabels)

		template := spec.Template
		require.Equal(t, labels, template.Labels)
		require.Len(t, template.Spec.Containers, 1)

		container := template.Spec.Containers[0]
		require.Equal(t, testContainerComponentName, container.Name)
		require.Equal(t, "test/test-image:latest", container.Image)
		require.Equal(t, v1.PullAlways, container.ImagePullPolicy)
		require.Len(t, container.Ports, 1)

		env := container.Env
		require.Equal(t, 1, len(env))
		require.Equal(t, "APPLICATION_NAME", env[0].Name)
		require.Equal(t, "Awesome Test Application", env[0].Value)

		port := container.Ports[0]
		require.Equal(t, "test-binding", port.Name)
		require.Equal(t, v1.ProtocolTCP, port.Protocol)
		require.Equal(t, int32(targetPort), port.ContainerPort)

		require.Empty(t, deploymentOutputResource.Dependencies)
	})

	t.Run("verify service", func(t *testing.T) {
		require.Equal(t, testContainerComponentName, service.Name)
		require.Equal(t, testAppName, service.Namespace)
		require.Equal(t, labels, service.Labels)
		require.Empty(t, service.Annotations)

		spec := service.Spec
		require.Equal(t, matchLabels, spec.Selector)
		require.Len(t, spec.Ports, 1)

		port := spec.Ports[0]
		require.Equal(t, "test-binding", port.Name)
		require.Equal(t, v1.ProtocolTCP, port.Protocol)
		require.Equal(t, int32(80), port.Port)
		require.Equal(t, intstr.FromInt(targetPort), port.TargetPort)

		dependencies := serviceOutputResource.Dependencies
		require.Len(t, dependencies, 1)
		require.Equal(t, dependencies[0].LocalID, outputresource.LocalIDDeployment)
	})
}

func TestRender_Success_NonDefaultPort(t *testing.T) {
	ctx := createContext(t)
	renderer := &Renderer{}

	port := 2000
	targerPort := 3000
	w := workloads.InstantiatedWorkload{
		Application: testAppName,
		Name:        testContainerComponentName,
		Workload: components.GenericComponent{
			Name: testContainerComponentName,
			Kind: Kind,
			Run: map[string]interface{}{
				"container": map[string]interface{}{
					"image": "test/test-image:latest",
				},
			},
			Bindings: map[string]components.GenericBinding{
				"test-binding": {
					Kind: "http",
					AdditionalProperties: map[string]interface{}{
						"port":       port,
						"targetPort": targerPort,
					},
				},
			},
		},
		BindingValues: map[components.BindingKey]components.BindingState{},
	}

	resources, err := renderer.Render(ctx, w)
	require.NoError(t, err)
	require.Len(t, resources, 2)

	deployment, deploymentOutputResource := kubernetes.FindDeployment(resources)
	require.NotNil(t, deployment)

	service, serviceOutputResource := kubernetes.FindService(resources)
	require.NotNil(t, service)

	labels := kubernetes.MakeDescriptiveLabels(testAppName, testContainerComponentName)
	matchLabels := kubernetes.MakeSelectorLabels(testAppName, testContainerComponentName)

	t.Run("verify deployment", func(t *testing.T) {
		require.Equal(t, testContainerComponentName, deployment.Name)
		require.Equal(t, testAppName, deployment.Namespace)
		require.Equal(t, labels, deployment.Labels)
		require.Empty(t, deployment.Annotations)

		spec := deployment.Spec
		require.Equal(t, matchLabels, spec.Selector.MatchLabels)

		template := spec.Template
		require.Equal(t, labels, template.Labels)
		require.Len(t, template.Spec.Containers, 1)

		container := template.Spec.Containers[0]
		require.Equal(t, testContainerComponentName, container.Name)
		require.Equal(t, "test/test-image:latest", container.Image)
		require.Equal(t, v1.PullAlways, container.ImagePullPolicy)
		require.Len(t, container.Ports, 1)

		port := container.Ports[0]
		require.Equal(t, "test-binding", port.Name)
		require.Equal(t, v1.ProtocolTCP, port.Protocol)
		require.Equal(t, int32(targerPort), port.ContainerPort)

		require.Empty(t, deploymentOutputResource.Dependencies)
	})

	t.Run("verify service", func(t *testing.T) {
		require.Equal(t, testContainerComponentName, service.Name)
		require.Equal(t, testAppName, service.Namespace)
		require.Equal(t, labels, service.Labels)
		require.Empty(t, service.Annotations)

		spec := service.Spec
		require.Equal(t, matchLabels, spec.Selector)
		require.Len(t, spec.Ports, 1)

		actualPort := spec.Ports[0]
		require.Equal(t, "test-binding", actualPort.Name)
		require.Equal(t, v1.ProtocolTCP, actualPort.Protocol)
		require.Equal(t, int32(port), actualPort.Port)
		require.Equal(t, intstr.FromInt(targerPort), actualPort.TargetPort)

		dependencies := serviceOutputResource.Dependencies
		require.Len(t, dependencies, 1)
		require.Equal(t, dependencies[0].LocalID, outputresource.LocalIDDeployment)
	})
}

func TestRenderWithKeyVault_Success(t *testing.T) {
	ctx := createContext(t)
	renderer := &Renderer{}

	targetPort := 3000
	w := workloads.InstantiatedWorkload{
		Application: testAppName,
		Name:        testContainerComponentName,
		Workload: components.GenericComponent{
			Name: testContainerComponentName,
			Kind: Kind,
			Run: map[string]interface{}{
				"container": map[string]interface{}{
					"image": "test/test-image:latest",
					"env": map[string]string{
						"APPLICATION_NAME": "Awesome Test Application",
					},
				},
			},
			Bindings: map[string]components.GenericBinding{
				"test-binding": {
					Kind: "http",
					AdditionalProperties: map[string]interface{}{
						"targetPort": targetPort,
					},
				},
			},
			Uses: []components.GenericDependency{
				{
					Binding: components.BindingExpression{
						Kind: "component",
						Value: &components.ComponentBindingValue{
							Application: testAppName,
							Component:   "kv",
							Binding:     "default",
							Property:    "uri",
						},
					},
					Secrets: &components.GenericDependencySecrets{
						Store: components.BindingExpression{
							Kind: "component",
							Value: &components.ComponentBindingValue{
								Application: testAppName,
								Component:   "kv",
								Binding:     "default",
							},
						},
						Keys: map[string]components.BindingExpression{
							"DBCONNECTIONSTRING": {
								Kind: "component",
								Value: &components.ComponentBindingValue{
									Application: testAppName,
									Component:   "db",
									Binding:     "mongo",
									Property:    "connectionString",
								},
							},
						},
					},
				},
			},
		},
		BindingValues: map[components.BindingKey]components.BindingState{
			{Component: "kv", Binding: "default"}: {
				Component: "kv",
				Binding:   "default",
				Kind:      "azure.com/KeyVault",
				Properties: map[string]interface{}{
					"uri": "https://test-keyvault.vault.azure.net",
				},
			},
			{Component: "db", Binding: "mongo"}: {
				Component: "db",
				Binding:   "mongo",
				Kind:      "azure.com/CosmosDBMongo",
				Properties: map[string]interface{}{
					"connectionString": "test-connection-string",
				},
			},
		},
	}

	resources, err := renderer.Render(ctx, w)
	require.NoError(t, err)
	require.Len(t, resources, 7)

	deployment, deploymentOutputResource := kubernetes.FindDeployment(resources)
	require.NotNil(t, deployment)

	service, serviceOutputResource := kubernetes.FindService(resources)
	require.NotNil(t, service)

	labels := kubernetes.MakeDescriptiveLabels(testAppName, testContainerComponentName)
	matchLabels := kubernetes.MakeSelectorLabels(testAppName, testContainerComponentName)

	t.Run("verify deployment", func(t *testing.T) {
		expectedPodIdentityName := "podid-" + strings.ToLower(testContainerComponentName)

		require.Equal(t, testContainerComponentName, deployment.Name)
		require.Equal(t, testAppName, deployment.Namespace)
		require.Equal(t, labels, deployment.Labels)
		require.Empty(t, deployment.Annotations)

		spec := deployment.Spec
		require.Equal(t, matchLabels, spec.Selector.MatchLabels)

		template := spec.Template
		require.Equal(t, template.ObjectMeta.Labels["aadpodidbinding"], expectedPodIdentityName)

		container := template.Spec.Containers[0]
		require.Equal(t, testContainerComponentName, container.Name)
		require.Equal(t, "test/test-image:latest", container.Image)
		require.Equal(t, v1.PullAlways, container.ImagePullPolicy)
		require.Len(t, container.Ports, 1)

		env := container.Env
		require.Equal(t, 1, len(env))
		require.Equal(t, "APPLICATION_NAME", env[0].Name)
		require.Equal(t, "Awesome Test Application", env[0].Value)

		port := container.Ports[0]
		require.Equal(t, "test-binding", port.Name)
		require.Equal(t, v1.ProtocolTCP, port.Protocol)
		require.Equal(t, int32(targetPort), port.ContainerPort)

		dependencies := deploymentOutputResource.Dependencies
		require.Len(t, dependencies, 2)
		require.Equal(t, dependencies[0].LocalID, outputresource.LocalIDKeyVaultSecret)
		require.Equal(t, dependencies[1].LocalID, outputresource.LocalIDAADPodIdentity)
	})

	t.Run("verify service", func(t *testing.T) {
		require.Equal(t, testContainerComponentName, service.Name)
		require.Equal(t, testAppName, service.Namespace)
		require.Equal(t, labels, service.Labels)
		require.Empty(t, service.Annotations)

		spec := service.Spec
		require.Equal(t, matchLabels, spec.Selector)
		require.Len(t, spec.Ports, 1)

		port := spec.Ports[0]
		require.Equal(t, "test-binding", port.Name)
		require.Equal(t, v1.ProtocolTCP, port.Protocol)
		require.Equal(t, int32(80), port.Port)
		require.Equal(t, intstr.FromInt(targetPort), port.TargetPort)

		dependencies := serviceOutputResource.Dependencies
		require.Len(t, dependencies, 1)
		require.Equal(t, dependencies[0].LocalID, outputresource.LocalIDDeployment)
	})
}
