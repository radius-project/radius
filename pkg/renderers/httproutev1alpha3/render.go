// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package httproutev1alpha3

import (
	"context"
	"fmt"

	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/project-radius/radius/pkg/azure/azresources"
	"github.com/project-radius/radius/pkg/azure/radclient"
	"github.com/project-radius/radius/pkg/kubernetes"
	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/renderers"
	"github.com/project-radius/radius/pkg/resourcekinds"
	tsv1 "github.com/servicemeshinterface/smi-sdk-go/pkg/apis/split/v1alpha2"
)

type Renderer struct {
}

func (r Renderer) GetDependencyIDs(ctx context.Context, workload renderers.RendererResource) (radiusResourceIDs []azresources.ResourceID, azureResourceIDs []azresources.ResourceID, err error) {
	properties, err := r.convert(workload)
	if err != nil {
		return nil, nil, err
	}
	// If the HttpRoute does not have any traffic split properties
	if len(properties.Routes) == 0 {
		return nil, nil, nil
	}
	for _, routes := range properties.Routes {
		destination := routes.Destination
		resourceID, err := azresources.Parse(*destination)
		if err != nil {
			return nil, nil, err
		}
		radiusResourceIDs = append(radiusResourceIDs, resourceID)
	}
	return radiusResourceIDs, azureResourceIDs, nil
}

func (r Renderer) Render(ctx context.Context, options renderers.RenderOptions) (renderers.RendererOutput, error) {
	route := radclient.HTTPRouteProperties{}
	resource := options.Resource
	radiusResourceIDs, _, _ := r.GetDependencyIDs(ctx, resource)
	port := kubernetes.GetDefaultPort()

	err := resource.ConvertDefinition(&route)
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	computedValues := map[string]renderers.ComputedValueReference{
		"host": {
			Value: kubernetes.MakeResourceName(resource.ApplicationName, resource.ResourceName),
		},
		"port": {
			Value: port,
		},
		"url": {
			Value: fmt.Sprintf("http://%s:%d", kubernetes.MakeResourceName(resource.ApplicationName, resource.ResourceName), port),
		},
		"scheme": {
			Value: "http",
		},
	}

	outputs := []outputresource.OutputResource{}

	service := r.makeService(resource, route)
	outputs = append(outputs, service)
	var trafficsplit outputresource.OutputResource
	//Check if trafficsplit properties are configured for this HttpRoute. If yes, a TrafficSplit object will be created.
	if len(route.Routes) > 0 {
		trafficsplit = r.makeTrafficSplit(resource, route, radiusResourceIDs)
	}
	if trafficsplit.Resource != nil {
		outputs = append(outputs, trafficsplit)
	}

	return renderers.RendererOutput{
		Resources:      outputs,
		ComputedValues: computedValues,
	}, nil
}

func (r *Renderer) makeService(resource renderers.RendererResource, route radclient.HTTPRouteProperties) outputresource.OutputResource {
	typeParts := strings.Split(resource.ResourceType, "/")
	port := kubernetes.GetDefaultPort()
	service := &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Service",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      kubernetes.MakeResourceName(resource.ApplicationName, resource.ResourceName),
			Namespace: resource.ApplicationName,
			Labels:    kubernetes.MakeDescriptiveLabels(resource.ApplicationName, resource.ResourceName),
		},
		Spec: corev1.ServiceSpec{
			Selector: kubernetes.MakeRouteSelectorLabels(resource.ApplicationName, typeParts[len(typeParts)-1], resource.ResourceName),
			Type:     corev1.ServiceTypeClusterIP,
			Ports: []corev1.ServicePort{
				{
					Name:       resource.ResourceName,
					Port:       int32(port),
					TargetPort: intstr.FromString(kubernetes.GetShortenedTargetPortName(resource.ApplicationName + typeParts[len(typeParts)-1] + resource.ResourceName)),
					Protocol:   corev1.ProtocolTCP,
				},
			},
		},
	}

	return outputresource.NewKubernetesOutputResource(resourcekinds.Service, outputresource.LocalIDService, service, service.ObjectMeta)
}

func (r *Renderer) makeTrafficSplit(resource renderers.RendererResource, route radclient.HTTPRouteProperties, radiusResourceIDs []azresources.ResourceID) outputresource.OutputResource {
	namespace := resource.ApplicationName
	numBackends := len(radiusResourceIDs)
	backends := make([]tsv1.TrafficSplitBackend, int(numBackends))
	routeName := resource.ResourceName
	rootService := namespace + "." + routeName
	trafficsplit := &tsv1.TrafficSplit{
		TypeMeta: metav1.TypeMeta{
			Kind:       "TrafficSplit",
			APIVersion: "split.smi-spec.io/v1alpha2",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      routeName,
			Namespace: namespace,
			Labels:    kubernetes.MakeDescriptiveLabels(resource.ApplicationName, resource.ResourceName),
		},
		Spec: tsv1.TrafficSplitSpec{
			Service:  rootService,
			Backends: backends,
		},
	}
	//populating the values in the backends array
	for i := 0; i < numBackends; i++ {
		httpRouteName := radiusResourceIDs[i].Types[2].Name
		trafficsplit.Spec.Backends[i] = tsv1.TrafficSplitBackend{
			Service: httpRouteName,
			Weight:  (int)(*(route.Routes[i].Weight)),
		}
	}
	return outputresource.NewKubernetesOutputResource(resourcekinds.Service, outputresource.LocalIDService, trafficsplit, trafficsplit.ObjectMeta)
}

func (r Renderer) convert(resource renderers.RendererResource) (*radclient.HTTPRouteProperties, error) {
	properties := &radclient.HTTPRouteProperties{}
	err := resource.ConvertDefinition(properties)
	if err != nil {
		return nil, err
	}

	return properties, nil
}
