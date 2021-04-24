// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package inboundroute

import (
	"context"
	"fmt"

	"github.com/Azure/radius/pkg/workloads"
	"github.com/Azure/radius/pkg/workloads/containerv1alpha1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Renderer is the WorkloadRenderer implementation for the 'radius.dev/InboundRoute' decorator.
type Renderer struct {
	Inner workloads.WorkloadRenderer
}

// Allocate is the WorkloadRenderer implementation for the radius.dev/InboundRoute' decorator.
func (r Renderer) Allocate(ctx context.Context, w workloads.InstantiatedWorkload, wrp []workloads.WorkloadResourceProperties, service workloads.WorkloadService) (map[string]interface{}, error) {
	return r.Inner.Allocate(ctx, w, wrp, service)
}

// Render is the WorkloadRenderer implementation for the radius.dev/InboundRoute' decorator.
func (r Renderer) Render(ctx context.Context, w workloads.InstantiatedWorkload) ([]workloads.WorkloadResource, error) {
	// Let the inner renderer do its work
	resources, err := r.Inner.Render(ctx, w)
	if err != nil {
		return []workloads.WorkloadResource{}, err
	}

	trait := Trait{}
	found, err := w.Workload.FindTrait(Kind, &trait)
	if err != nil {
		return []workloads.WorkloadResource{}, err
	} else if !found {
		return resources, err
	}

	if trait.Properties.Service == "" {
		return []workloads.WorkloadResource{}, fmt.Errorf("the service field is required for trait '%s'", Kind)
	}

	provides, err := w.Workload.FindProvidesServiceRequired(trait.Properties.Service)
	if err != nil {
		return []workloads.WorkloadResource{}, err
	}
	httpProvides := containerv1alpha1.HTTPProvidesService{}
	err = provides.AsRequired(containerv1alpha1.KindHTTP, &httpProvides)
	if err != nil {
		return []workloads.WorkloadResource{}, err
	}

	ingress := &networkingv1.Ingress{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Ingress",
			APIVersion: networkingv1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      w.Name,
			Namespace: w.Application,
			Labels: map[string]string{
				"radius.dev/application": w.Application,
				"radius.dev/component":   w.Name,
				// TODO get the component revision here...
				"app.kubernetes.io/name":       w.Name,
				"app.kubernetes.io/part-of":    w.Application,
				"app.kubernetes.io/managed-by": "radius-rp",
			},
		},
	}

	backend := networkingv1.IngressBackend{
		Service: &networkingv1.IngressServiceBackend{
			Name: w.Name,
			Port: networkingv1.ServiceBackendPort{
				Number: int32(httpProvides.GetEffectivePort()),
			},
		},
	}

	if trait.Properties.Hostname == "" {
		spec := networkingv1.IngressSpec{
			DefaultBackend: &backend,
		}

		ingress.Spec = spec
	} else {
		spec := networkingv1.IngressSpec{
			Rules: []networkingv1.IngressRule{
				{
					Host: trait.Properties.Hostname,
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

	resources = append(resources, workloads.NewKubernetesResource("Ingress", ingress))
	return resources, nil
}
