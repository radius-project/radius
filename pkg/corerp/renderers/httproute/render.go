// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package httproute

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/corerp/renderers"
	"github.com/project-radius/radius/pkg/kubernetes"
	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/project-radius/radius/pkg/rp"
	"github.com/project-radius/radius/pkg/ucp/resources"
	tsv1 "github.com/servicemeshinterface/smi-sdk-go/pkg/apis/split/v1alpha2"
)

type Renderer struct {
}

func (r Renderer) GetDependencyIDs(ctx context.Context, resource conv.DataModelInterface) (radiusResourceIDs []resources.ID, resourceIDs []resources.ID, err error) {
	route, ok := resource.(*datamodel.HTTPRoute)
	if !ok {
		return radiusResourceIDs, resourceIDs, nil
	}

	if route.Properties == nil || len(route.Properties.Routes) == 0 {
		return nil, nil, nil
	}
	for _, routes := range route.Properties.Routes {
		destination := routes.Destination
		resourceID, err := resources.Parse(destination)
		if err != nil {
			return nil, nil, err
		}
		radiusResourceIDs = append(radiusResourceIDs, resourceID)
	}
	return radiusResourceIDs, resourceIDs, nil
}

func (r Renderer) Render(ctx context.Context, dm conv.DataModelInterface, options renderers.RenderOptions) (renderers.RendererOutput, error) {
	route, ok := dm.(*datamodel.HTTPRoute)
	if !ok {
		return renderers.RendererOutput{}, conv.ErrInvalidModelConversion
	}
	outputResources := []outputresource.OutputResource{}
	appId, err := resources.Parse(route.Properties.Application)
	if err != nil {
		return renderers.RendererOutput{}, fmt.Errorf("invalid application id: %w. id: %s", err, route.Properties.Application)
	}
	applicationName := appId.Name()

	if route.Properties.Port == 0 {
		defaultPort := kubernetes.GetDefaultPort()
		route.Properties.Port = defaultPort
	}

	computedValues := map[string]rp.ComputedValueReference{
		"hostname": {
			Value: kubernetes.MakeResourceName(applicationName, route.Name),
		},
		"port": {
			Value: route.Properties.Port,
		},
		"url": {
			Value: fmt.Sprintf("http://%s:%d", kubernetes.MakeResourceName(applicationName, route.Name), route.Properties.Port),
		},
		"scheme": {
			Value: "http",
		},
	}
	var portNum int
	if len(route.Properties.Routes) > 0 {
		trafficsplit, pNum, err := r.makeTrafficSplit(route, options)
		if err != nil {
			return renderers.RendererOutput{
				Resources:      outputResources,
				ComputedValues: computedValues,
			}, err
		}
		outputResources = append(outputResources, trafficsplit)
		portNum = pNum
	}
	if route.Properties.ContainerPort != 0 {
		portNum = int(route.Properties.ContainerPort)
	}
	service, err := r.makeService(route, options, portNum)
	if err != nil {
		return renderers.RendererOutput{}, err
	}
	outputResources = append(outputResources, service)

	return renderers.RendererOutput{
		Resources:      outputResources,
		ComputedValues: computedValues,
	}, nil
}

func (r *Renderer) makeService(route *datamodel.HTTPRoute, options renderers.RenderOptions, specifiedTargetPort int) (outputresource.OutputResource, error) {
	appId, err := resources.Parse(route.Properties.Application)

	if err != nil {
		return outputresource.OutputResource{}, fmt.Errorf("invalid application id: %w. id: %s", err, route.Properties.Application)
	}
	applicationName := appId.Name()

	typeParts := strings.Split(ResourceType, "/")
	resourceTypeSuffix := typeParts[len(typeParts)-1]

	var target intstr.IntOrString
	if specifiedTargetPort != 0 {
		target = intstr.FromInt(specifiedTargetPort)
	} else {
		target = intstr.FromString(kubernetes.GetShortenedTargetPortName(options.Environment.Namespace + resourceTypeSuffix + route.Name))
	}

	service := &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Service",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      kubernetes.MakeResourceName(applicationName, route.Name),
			Namespace: options.Environment.Namespace,
			Labels:    kubernetes.MakeDescriptiveLabels(applicationName, route.Name),
		},
		Spec: corev1.ServiceSpec{
			Selector: kubernetes.MakeRouteSelectorLabelsTrafficSplit(applicationName, resourceTypeSuffix, route.Name, specifiedTargetPort),
			Type:     corev1.ServiceTypeClusterIP,
			Ports: []corev1.ServicePort{
				{
					Name:       route.Name,
					Port:       route.Properties.Port,
					TargetPort: target,
					Protocol:   corev1.ProtocolTCP,
				},
			},
		},
	}

	return outputresource.NewKubernetesOutputResource(resourcekinds.Service, outputresource.LocalIDService, service, service.ObjectMeta), nil
}

func (r *Renderer) makeTrafficSplit(route *datamodel.HTTPRoute, options renderers.RenderOptions) (outputresource.OutputResource, int, error) {
	dependencies := options.Dependencies
	numBackends := len(dependencies)
	var backends []tsv1.TrafficSplitBackend
	routeName := route.Name
	rootService := kubernetes.MakeResourceName(options.Environment.Namespace, routeName) + "." + options.Environment.Namespace + ".svc.cluster.local"
	trafficsplit := &tsv1.TrafficSplit{
		TypeMeta: metav1.TypeMeta{
			Kind:       "TrafficSplit",
			APIVersion: "split.smi-spec.io/v1alpha2",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      routeName,
			Namespace: options.Environment.Namespace,
			Labels:    kubernetes.MakeDescriptiveLabels(options.Environment.Namespace, routeName),
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
		destination := route.Properties.Routes[i].Destination
		if _, ok := dependencies[destination].ComputedValues["port"]; ok {
			destPort := (int)(dependencies[destination].ComputedValues["port"].(int32))
			if portNum != -1 && destPort != portNum {
				err = fmt.Errorf("backend services have different port values")
			}
			portNum = destPort
		}
		httpRouteName := dependencies[destination].ResourceID.Name()
		tsBackend := tsv1.TrafficSplitBackend{
			Service: kubernetes.MakeResourceName(options.Environment.Namespace, httpRouteName),
			Weight:  int(route.Properties.Routes[i].Weight),
		}
		trafficsplit.Spec.Backends = append(trafficsplit.Spec.Backends, tsBackend)
	}
	if portNum == -1 {
		err = fmt.Errorf("backend services have invalid port values")
	}
	return outputresource.NewKubernetesOutputResource(resourcekinds.TrafficSplit, outputresource.LocalIDTrafficSplit, trafficsplit, trafficsplit.ObjectMeta), portNum, err
}
