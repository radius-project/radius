// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package inboundroutev1alpha3

import (
	"context"
	"fmt"

	"github.com/Azure/radius/pkg/azure/azresources"
	"github.com/Azure/radius/pkg/kubernetes"
	"github.com/Azure/radius/pkg/radrp/outputresource"
	"github.com/Azure/radius/pkg/renderers"
	"github.com/Azure/radius/pkg/resourcekinds"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Renderer is the WorkloadRenderer implementation for the 'radius.dev/InboundRoute' decorator.
type Renderer struct {
	Inner renderers.Renderer
}

// Need a step to take rendered routes to be usable by component
func (r Renderer) GetDependencyIDs(ctx context.Context, workload renderers.RendererResource) ([]azresources.ResourceID, error) {
	return r.Inner.GetDependencyIDs(ctx, workload)
}

// Render is the WorkloadRenderer implementation for the radius.dev/InboundRoute' decorator.
func (r Renderer) Render(ctx context.Context, resource renderers.RendererResource, dependencies map[string]renderers.RendererDependency) (renderers.RendererOutput, error) {
	// Let the inner renderer do its work
	resources, err := r.Inner.Render(ctx, resource, dependencies)
	if err != nil {
		// Even if the operation fails, return the output resources created so far
		// TODO: This is temporary. Once there are no resources actually deployed during render phase,
		// we no longer need to track the output resources on error
		// See: https://github.com/Azure/radius/issues/499
		return resources, err
	}
	traits := resource.Definition["traits"]
	if traits == nil {
		return resources, fmt.Errorf("InboundRoute decorator requires a 'traits' field")
	}

	casted, ok := traits.([]interface{})
	if !ok {
		return resources, fmt.Errorf("InboundRoute trait requires a 'traits' field of type []interface{}")
	}

	trait := InboundRouteTrait{}
	found, err := FindTrait(casted, Kind, &trait)
	if !found || err != nil {
		// Even if the operation fails, return the output resources created so far
		// TODO: This is temporary. Once there are no resources actually deployed during render phase,
		// we no longer need to track the output resources on error
		// See: https://github.com/Azure/radius/issues/499
		return resources, err
	}

	if trait.Binding == "" {
		// Even if the operation fails, return the output resources created so far
		// TODO: This is temporary. Once there are no resources actually deployed during render phase,
		// we no longer need to track the output resources on error
		return resources, fmt.Errorf("the binding field is required for trait '%s'", Kind)
	}

	ingress := &networkingv1.Ingress{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Ingress",
			APIVersion: networkingv1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      resource.ResourceName,
			Namespace: resource.ApplicationName,
			Labels:    kubernetes.MakeDescriptiveLabelsV3(resource.ApplicationName, resource.ResourceName),
		},
	}

	resourceProperties, ok := dependencies[trait.Binding]
	if !ok {
		return resources, fmt.Errorf("cannot find referenced resource '%s' referenced by '%s' trait", trait.Binding, Kind)
	}

	port, ok := resourceProperties.ComputedValues["port"]
	if !ok {
		return resources, fmt.Errorf("cannot find port property on '%s' for trait '%s'", trait.Binding, Kind)
	}

	portInt, ok := port.(int)
	if !ok {
		return resources, fmt.Errorf("port cannot be treated as int in '%s' for trait '%s'", trait.Binding, Kind)
	}

	backend := networkingv1.IngressBackend{
		Service: &networkingv1.IngressServiceBackend{
			Name: resource.ResourceName,
			Port: networkingv1.ServiceBackendPort{
				Number: int32(portInt),
			},
		},
	}

	if trait.Hostname == "" {
		spec := networkingv1.IngressSpec{
			DefaultBackend: &backend,
		}

		ingress.Spec = spec
	} else {
		spec := networkingv1.IngressSpec{
			Rules: []networkingv1.IngressRule{
				{
					Host: trait.Hostname,
					IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: &networkingv1.HTTPIngressRuleValue{
							Paths: []networkingv1.HTTPIngressPath{
								{
									Backend: backend,
								},
							},
						},
					},
				},
			},
		}

		ingress.Spec = spec
	}

	outputResource := outputresource.OutputResource{
		Kind:     resourcekinds.Kubernetes,
		LocalID:  outputresource.LocalIDIngress,
		Deployed: false,
		Managed:  true,
		Type:     outputresource.TypeKubernetes,
		Info: outputresource.K8sInfo{
			Kind:       ingress.TypeMeta.Kind,
			APIVersion: ingress.TypeMeta.APIVersion,
			Name:       ingress.ObjectMeta.Name,
			Namespace:  ingress.ObjectMeta.Namespace,
		},
		Resource: ingress,
	}

	resources.Resources = append(resources.Resources, outputResource)
	return resources, nil
}
