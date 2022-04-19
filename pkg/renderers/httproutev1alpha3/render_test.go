// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package httproutev1alpha3

import (
	"context"
	"fmt"
	"testing"

	"github.com/project-radius/radius/pkg/kubernetes"
	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/renderers"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	namespace       = "test-namespace"
	applicationName = "test-application"
	resourceName    = "test-route"
)

func Test_GetDependencyIDs_Empty(t *testing.T) {
	r := &Renderer{}

	resource := renderers.RendererResource{}
	dependencies, _, err := r.GetDependencyIDs(context.Background(), resource)
	require.NoError(t, err)
	require.Empty(t, dependencies)
}

func Test_Render(t *testing.T) {
	r := &Renderer{}

	resource := renderers.RendererResource{
		ApplicationName: applicationName,
		ResourceName:    resourceName,
		ResourceType:    ResourceType,
		Definition:      map[string]interface{}{},
	}
	dependencies := map[string]renderers.RendererDependency{}

	output, err := r.Render(context.Background(), renderers.RenderOptions{Resource: resource, Dependencies: dependencies, Runtime: renderers.RuntimeOptions{}})
	require.NoError(t, err)
	require.Len(t, output.Resources, 1)
	require.Empty(t, output.SecretValues)

	expectedValues := map[string]renderers.ComputedValueReference{
		"host":   {Value: kubernetes.MakeResourceName(applicationName, resourceName)},
		"port":   {Value: 80},
		"scheme": {Value: "http"},
		"url":    {Value: fmt.Sprintf("http://%s:80", kubernetes.MakeResourceName(applicationName, resourceName))},
	}
	require.Equal(t, expectedValues, output.ComputedValues)

	service, outputResource := kubernetes.FindService(output.Resources)

	expectedOutputResource := outputresource.NewKubernetesOutputResource(resourcekinds.Service, outputresource.LocalIDService, service, service.ObjectMeta)
	require.Equal(t, expectedOutputResource, outputResource)

	require.Equal(t, kubernetes.MakeResourceName(resource.ApplicationName, resource.ResourceName), service.Name)
	require.Equal(t, applicationName, service.Namespace)
	require.Equal(t, kubernetes.MakeDescriptiveLabels(applicationName, resourceName), service.Labels)

	require.Equal(t, kubernetes.MakeRouteSelectorLabels(applicationName, resource.ResourceType, resourceName), service.Spec.Selector)
	require.Equal(t, corev1.ServiceTypeClusterIP, service.Spec.Type)

	require.Len(t, service.Spec.Ports, 1)
	port := service.Spec.Ports[0]

	expectedPort := corev1.ServicePort{
		Name:       resourceName,
		Port:       80,
		TargetPort: intstr.FromString(kubernetes.GetShortenedTargetPortName(resource.ApplicationName + resource.ResourceType + resource.ResourceName)),
		Protocol:   "TCP",
	}
	require.Equal(t, expectedPort, port)
}
