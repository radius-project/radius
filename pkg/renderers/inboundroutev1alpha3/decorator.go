// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package inboundroutev1alpha3

import (
	"context"
	"fmt"
	"strconv"

	"github.com/Azure/radius/pkg/kubernetes"
	"github.com/Azure/radius/pkg/radrp/outputresource"
	"github.com/Azure/radius/pkg/resourcekinds"
	"github.com/Azure/radius/pkg/workloadsv1alpha3"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Renderer is the WorkloadRenderer implementation for the 'radius.dev/InboundRoute' decorator.
type Renderer struct {
	Inner workloadsv1alpha3.WorkloadRenderer
}

// Need a step to take rendered routes to be usable by component
func (r Renderer) GetDependencies(ctx context.Context, workload workloadsv1alpha3.InstantiatedWorkload) ([]string, error) {

	return r.Inner.GetDependencies(ctx, workload)
}

// Render is the WorkloadRenderer implementation for the radius.dev/InboundRoute' decorator.
func (r Renderer) Render(ctx context.Context, w workloadsv1alpha3.InstantiatedWorkload) ([]outputresource.OutputResource, error) {
	// Let the inner renderer do its work
	resources, err := r.Inner.Render(ctx, w)
	if err != nil {
		// Even if the operation fails, return the output resources created so far
		// TODO: This is temporary. Once there are no resources actually deployed during render phase,
		// we no longer need to track the output resources on error
		// See: https://github.com/Azure/radius/issues/499
		return resources, err
	}

	trait := Trait{}
	found, err := w.Workload.FindTrait(Kind, &trait)
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

	// TODO this needs to get the HttpRoute, right?
	properties, ok := w.References[trait.Binding]
	if !ok {
		// Even if the operation fails, return the output resources created so far
		// TODO: This is temporary. Once there are no resources actually deployed during render phase,
		// we no longer need to track the output resources on error
		// See: https://github.com/Azure/radius/issues/499
		return resources, fmt.Errorf("cannot find referenced resource '%s' referenced by '%s' trait", trait.Binding, Kind)
	}

	ingress := &networkingv1.Ingress{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Ingress",
			APIVersion: networkingv1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      w.Name,
			Namespace: w.Application,
			Labels:    kubernetes.MakeDescriptiveLabels(w.Application, w.Name),
		},
	}

	port, ok := properties["port"]
	if !ok {
		return resources, fmt.Errorf("cannot find port property on '%s' for trait '%s'", trait.Binding, Kind)
	}

	portInt, err := strconv.Atoi(port)

	backend := networkingv1.IngressBackend{
		Service: &networkingv1.IngressServiceBackend{
			Name: w.Name,
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

	resource := outputresource.OutputResource{
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
	resources = append(resources, resource)
	return resources, nil
}
