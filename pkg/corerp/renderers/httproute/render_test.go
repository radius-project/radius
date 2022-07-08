// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package httproute

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/corerp/renderers"
	"github.com/project-radius/radius/pkg/kubernetes"
	"github.com/project-radius/radius/pkg/radlogger"
	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/project-radius/radius/pkg/rp"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	applicationName = "test-application"
	resourceName    = "test-route"
	applicationPath = "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup/providers/Applications.Core/applications/"
)

func createContext(t *testing.T) context.Context {
	logger, err := radlogger.NewTestLogger(t)
	if err != nil {
		t.Log("Unable to initialize logger")
		return context.Background()
	}
	return logr.NewContext(context.Background(), logger)
}

func Test_GetDependencyIDs_Empty(t *testing.T) {
	r := &Renderer{}

	dependencies, _, err := r.GetDependencyIDs(createContext(t), datamodel.HTTPRoute{})
	require.NoError(t, err)
	require.Empty(t, dependencies)
}

func Test_Render_WithPort(t *testing.T) {
	r := &Renderer{}
	var port int32 = 6379

	dependencies := map[string]renderers.RendererDependency{}
	properties := makeHTTPRouteProperties(port)
	resource := makeResource(t, &properties)

	output, err := r.Render(context.Background(), resource, renderers.RenderOptions{Dependencies: dependencies, Environment: renderers.EnvironmentOptions{}})
	require.NoError(t, err)
	require.Len(t, output.Resources, 1)
	require.Empty(t, output.SecretValues)

	expectedValues := map[string]rp.ComputedValueReference{
		"host":   {Value: kubernetes.MakeResourceName(applicationName, resourceName)},
		"port":   {Value: port},
		"scheme": {Value: "http"},
		"url":    {Value: fmt.Sprintf("http://%s:%d", kubernetes.MakeResourceName(applicationName, resourceName), port)},
	}
	require.Equal(t, expectedValues, output.ComputedValues)

	service, outputResource := kubernetes.FindService(output.Resources)

	expectedOutputResource := outputresource.NewKubernetesOutputResource(resourcekinds.Service, outputresource.LocalIDService, service, service.ObjectMeta)
	require.Equal(t, expectedOutputResource, outputResource)

	require.Equal(t, kubernetes.MakeResourceName(applicationName, resource.Name), service.Name)
	require.Equal(t, "", service.Namespace)
	require.Equal(t, kubernetes.MakeDescriptiveLabels(applicationName, resourceName), service.Labels)

	require.Equal(t, kubernetes.MakeRouteSelectorLabels(applicationName, ResourceType, resourceName), service.Spec.Selector)
	require.Equal(t, corev1.ServiceTypeClusterIP, service.Spec.Type)

	require.Len(t, service.Spec.Ports, 1)
	servicePort := service.Spec.Ports[0]

	expectedServicePort := corev1.ServicePort{
		Name:       resourceName,
		Port:       port,
		TargetPort: intstr.FromString(kubernetes.GetShortenedTargetPortName(applicationName + ResourceType + resource.Name)),
		Protocol:   "TCP",
	}
	require.Equal(t, expectedServicePort, servicePort)
}

func Test_Render_WithDefaultPort(t *testing.T) {
	r := &Renderer{}

	defaultPort := kubernetes.GetDefaultPort()
	dependencies := map[string]renderers.RendererDependency{}
	properties := makeHTTPRouteProperties(defaultPort)
	resource := makeResource(t, &properties)

	output, err := r.Render(context.Background(), resource, renderers.RenderOptions{Dependencies: dependencies, Environment: renderers.EnvironmentOptions{}})
	require.NoError(t, err)
	require.Len(t, output.Resources, 1)
	require.Empty(t, output.SecretValues)

	expectedValues := map[string]rp.ComputedValueReference{
		"host":   {Value: kubernetes.MakeResourceName(applicationName, resourceName)},
		"port":   {Value: defaultPort},
		"scheme": {Value: "http"},
		"url":    {Value: fmt.Sprintf("http://%s:%d", kubernetes.MakeResourceName(applicationName, resourceName), defaultPort)},
	}
	require.Equal(t, expectedValues, output.ComputedValues)

	service, outputResource := kubernetes.FindService(output.Resources)

	expectedOutputResource := outputresource.NewKubernetesOutputResource(resourcekinds.Service, outputresource.LocalIDService, service, service.ObjectMeta)
	require.Equal(t, expectedOutputResource, outputResource)

	require.Equal(t, kubernetes.MakeResourceName(applicationName, resource.Name), service.Name)
	require.Equal(t, "", service.Namespace)
	require.Equal(t, kubernetes.MakeDescriptiveLabels(applicationName, resourceName), service.Labels)

	require.Equal(t, kubernetes.MakeRouteSelectorLabels(applicationName, ResourceType, resourceName), service.Spec.Selector)
	require.Equal(t, corev1.ServiceTypeClusterIP, service.Spec.Type)

	require.Len(t, service.Spec.Ports, 1)
	port := service.Spec.Ports[0]

	expectedPort := corev1.ServicePort{
		Name:       resourceName,
		Port:       defaultPort,
		TargetPort: intstr.FromString(kubernetes.GetShortenedTargetPortName(applicationName + ResourceType + resource.Name)),
		Protocol:   "TCP",
	}
	require.Equal(t, expectedPort, port)
}

func Test_Render_WithNameSpace(t *testing.T) {
	r := &Renderer{}

	defaultPort := kubernetes.GetDefaultPort()
	dependencies := map[string]renderers.RendererDependency{}
	properties := makeHTTPRouteProperties(defaultPort)
	resource := makeResource(t, &properties)
	options := renderers.RenderOptions{Dependencies: dependencies, Environment: renderers.EnvironmentOptions{Namespace: "testNamespace"}}

	output, err := r.Render(context.Background(), resource, options)
	require.NoError(t, err)
	require.Len(t, output.Resources, 1)
	require.Empty(t, output.SecretValues)

	expectedValues := map[string]rp.ComputedValueReference{
		"host":   {Value: kubernetes.MakeResourceName(applicationName, resourceName)},
		"port":   {Value: defaultPort},
		"scheme": {Value: "http"},
		"url":    {Value: fmt.Sprintf("http://%s:%d", kubernetes.MakeResourceName(applicationName, resourceName), defaultPort)},
	}
	require.Equal(t, expectedValues, output.ComputedValues)

	service, outputResource := kubernetes.FindService(output.Resources)

	expectedOutputResource := outputresource.NewKubernetesOutputResource(resourcekinds.Service, outputresource.LocalIDService, service, service.ObjectMeta)
	require.Equal(t, expectedOutputResource, outputResource)

	require.Equal(t, kubernetes.MakeResourceName(applicationName, resource.Name), service.Name)
	require.Equal(t, options.Environment.Namespace, service.Namespace)
	require.Equal(t, kubernetes.MakeDescriptiveLabels(applicationName, resourceName), service.Labels)

	require.Equal(t, kubernetes.MakeRouteSelectorLabels(applicationName, ResourceType, resourceName), service.Spec.Selector)
	require.Equal(t, corev1.ServiceTypeClusterIP, service.Spec.Type)

	require.Len(t, service.Spec.Ports, 1)
	port := service.Spec.Ports[0]

	expectedPort := corev1.ServicePort{
		Name:       resourceName,
		Port:       defaultPort,
		TargetPort: intstr.FromString(kubernetes.GetShortenedTargetPortName(applicationName + ResourceType + resource.Name)),
		Protocol:   "TCP",
	}
	require.Equal(t, expectedPort, port)
}

func makeHTTPRouteProperties(port int32) datamodel.HTTPRouteProperties {
	properties := datamodel.HTTPRouteProperties{}
	str := []string{applicationPath, applicationName}
	properties.Application = strings.Join(str, "")
	if port > 0 {
		properties.Port = port
	}

	return properties
}

func makeResource(t *testing.T, properties *datamodel.HTTPRouteProperties) *datamodel.HTTPRoute {

	dm := datamodel.HTTPRoute{Properties: properties}
	dm.Name = resourceName

	return &dm
}
