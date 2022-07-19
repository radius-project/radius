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

func (r Renderer) GetDependencyIDs(ctx context.Context, resource renderers.RendererResource) (radiusResourceIDs []azresources.ResourceID, azureResourceIDs []azresources.ResourceID, err error) {
	properties, err := r.convert(resource)
	if err != nil {
		return nil, nil, err
	}
	if properties == nil || len(properties.Routes) == 0 {
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
	resource := options.Resource
	route, err := r.convert(resource)
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	if route == nil {
		defaultPort := kubernetes.GetDefaultPort()
		route = &radclient.HTTPRouteProperties{
			Port: &defaultPort,
		}
	}
	if route.Port == nil {
		defaultPort := kubernetes.GetDefaultPort()
		route.Port = &defaultPort
	}
	computedValues := map[string]renderers.ComputedValueReference{
		"host": {
			Value: kubernetes.MakeResourceName(resource.ApplicationName, resource.ResourceName),
		},
		"port": {
			Value: *route.Port,
		},
		"url": {
			Value: fmt.Sprintf("http://%s:%d", kubernetes.MakeResourceName(resource.ApplicationName, resource.ResourceName), *route.Port),
		},
		"scheme": {
			Value: "http",
		},
	}

	outputs := []outputresource.OutputResource{}
	var portNum int
	// Check if trafficsplit properties are configured for this HttpRoute. If yes, a TrafficSplit object will be created.
	if len(route.Routes) > 0 {
		trafficsplit, pNum, err := r.makeTrafficSplit(resource, route, options)
		if err != nil {
			return renderers.RendererOutput{
				Resources:      outputs,
				ComputedValues: computedValues,
			}, err
		}
		outputs = append(outputs, trafficsplit)
		portNum = pNum
	}
	if route.ContainerPort != nil {
		portNum = int(*route.ContainerPort)
	}
	service := r.makeService(resource, route, portNum)
	outputs = append(outputs, service)
	return renderers.RendererOutput{
		Resources:      outputs,
		ComputedValues: computedValues,
	}, nil
}

func (r *Renderer) makeService(resource renderers.RendererResource, route *radclient.HTTPRouteProperties, specifiedTargetPort int) outputresource.OutputResource {
	typeParts := strings.Split(resource.ResourceType, "/")
	resourceType := typeParts[len(typeParts)-1]

	// The variable 'target' will be used as the designated value for the TargetPort of this Kuberentes service
	// if specifiedTargetPort is not zero, we are currently working with a TrafficSplit service, and we should use
	// this value as the TargetPort. Otherwise, we will use a hash string.
	var target intstr.IntOrString
	if specifiedTargetPort != 0 {
		target = intstr.FromInt(specifiedTargetPort)
	} else {
		target = intstr.FromString(kubernetes.GetShortenedTargetPortName(resource.ApplicationName + resourceType + resource.ResourceName))
	}
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
			Selector: kubernetes.MakeRouteSelectorLabelsTrafficSplit(resource.ApplicationName, resourceType, resource.ResourceName, specifiedTargetPort),
			Type:     corev1.ServiceTypeClusterIP,
			Ports: []corev1.ServicePort{
				{
					Name:       resource.ResourceName,
					Port:       *route.Port,
					TargetPort: target,
					Protocol:   corev1.ProtocolTCP,
				},
			},
		},
	}
	return outputresource.NewKubernetesOutputResource(resourcekinds.Service, outputresource.LocalIDService, service, service.ObjectMeta)
}

func (r *Renderer) makeTrafficSplit(resource renderers.RendererResource, route *radclient.HTTPRouteProperties, options renderers.RenderOptions) (outputresource.OutputResource, int, error) {
	namespace := resource.ApplicationName
	dependencies := options.Dependencies
	numBackends := len(dependencies)
	var backends []tsv1.TrafficSplitBackend
	routeName := resource.ResourceName
	rootService := kubernetes.MakeResourceName(resource.ApplicationName, resource.ResourceName) + "." + namespace + ".svc.cluster.local"
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
	// populating the values in the backends array && retrieve the port vlaues
	portNum := -1
	var err error
	for i := 0; i < numBackends; i++ {
		destination := *route.Routes[i].Destination
		if _, ok := dependencies[destination].ComputedValues["port"]; ok {
			destPort := (int)(dependencies[destination].ComputedValues["port"].(int32))
			if portNum != -1 && destPort != portNum {
				err = fmt.Errorf("backend services have different port values")
			}
			portNum = destPort
		}
		httpRouteName := dependencies[destination].ResourceID.Name()
		tsBackend := tsv1.TrafficSplitBackend{
			Service: kubernetes.MakeResourceName(resource.ApplicationName, httpRouteName),
			Weight:  (int)(*(route.Routes[i].Weight)),
		}
		trafficsplit.Spec.Backends = append(trafficsplit.Spec.Backends, tsBackend)
	}
	if portNum == -1 {
		err = fmt.Errorf("backend services have invalid port values")
	}
	return outputresource.NewKubernetesOutputResource(resourcekinds.TrafficSplit, outputresource.LocalIDTrafficSplit, trafficsplit, trafficsplit.ObjectMeta), portNum, err
}

func (r Renderer) convert(resource renderers.RendererResource) (*radclient.HTTPRouteProperties, error) {
	properties := &radclient.HTTPRouteProperties{}
	err := resource.ConvertDefinition(&properties)
	if err != nil {
		return nil, err
	}

	return properties, nil
}
