// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package httproute

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/go-logr/logr"
	//"github.com/project-radius/radius/pkg/azure/radclient"

	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/kubernetes"
	"github.com/project-radius/radius/pkg/radlogger"
	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/renderers"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	applicationName = "test-application"
	resourceName    = "test-route"
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
	properties := datamodel.HTTPRouteProperties{}
	properties.Port = port
	dm := datamodel.HTTPRoute{Properties: properties}

	resource := makeResource(t, properties)

	output, err := r.Render(context.Background(), dm, renderers.RenderOptions{Resource: resource, Dependencies: dependencies, Runtime: renderers.RuntimeOptions{}})
	require.NoError(t, err)
	require.Len(t, output.Resources, 1)
	require.Empty(t, output.SecretValues)

	expectedValues := map[string]renderers.ComputedValueReference{
		"host":   {Value: kubernetes.MakeResourceName(applicationName, resourceName)},
		"port":   {Value: port},
		"scheme": {Value: "http"},
		"url":    {Value: fmt.Sprintf("http://%s:%d", kubernetes.MakeResourceName(applicationName, resourceName), port)},
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
	servicePort := service.Spec.Ports[0]

	expectedServicePort := corev1.ServicePort{
		Name:       resourceName,
		Port:       port,
		TargetPort: intstr.FromString(kubernetes.GetShortenedTargetPortName(resource.ApplicationName + resource.ResourceType + resource.ResourceName)),
		Protocol:   "TCP",
	}
	require.Equal(t, expectedServicePort, servicePort)
}

func Test_Render_WithDefaultPort(t *testing.T) {
	r := &Renderer{}

	defaultPort := kubernetes.GetDefaultPort()
	resource := renderers.RendererResource{
		ApplicationName: applicationName,
		ResourceName:    resourceName,
		ResourceType:    ResourceTypeName,
		Definition:      map[string]interface{}{},
	}
	dependencies := map[string]renderers.RendererDependency{}
	properties := datamodel.HTTPRouteProperties{}
	properties.Port = defaultPort
	dm := datamodel.HTTPRoute{Properties: properties}

	output, err := r.Render(context.Background(), dm, renderers.RenderOptions{Resource: resource, Dependencies: dependencies, Runtime: renderers.RuntimeOptions{}})
	require.NoError(t, err)
	require.Len(t, output.Resources, 1)
	require.Empty(t, output.SecretValues)

	expectedValues := map[string]renderers.ComputedValueReference{
		"host":   {Value: kubernetes.MakeResourceName(applicationName, resourceName)},
		"port":   {Value: defaultPort},
		"scheme": {Value: "http"},
		"url":    {Value: fmt.Sprintf("http://%s:%d", kubernetes.MakeResourceName(applicationName, resourceName), defaultPort)},
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
		Port:       defaultPort,
		TargetPort: intstr.FromString(kubernetes.GetShortenedTargetPortName(resource.ApplicationName + resource.ResourceType + resource.ResourceName)),
		Protocol:   "TCP",
	}
	require.Equal(t, expectedPort, port)
}

func makeResource(t *testing.T, T any) renderers.RendererResource {
	b, err := json.Marshal(&T)
	require.NoError(t, err)

	definition := map[string]interface{}{}
	err = json.Unmarshal(b, &definition)
	require.NoError(t, err)

	return renderers.RendererResource{
		ApplicationName: applicationName,
		ResourceName:    resourceName,
		ResourceType:    ResourceTypeName,
		Definition:      definition,
	}
}
