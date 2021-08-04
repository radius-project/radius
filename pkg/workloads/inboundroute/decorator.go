// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package inboundroute

import (
	"context"
	"fmt"

	"github.com/Azure/radius/pkg/kubernetes"
	"github.com/Azure/radius/pkg/radrp/components"
	"github.com/Azure/radius/pkg/radrp/outputresource"
	"github.com/Azure/radius/pkg/workloads"
	"github.com/Azure/radius/pkg/workloads/containerv1alpha1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Renderer is the WorkloadRenderer implementation for the 'radius.dev/InboundRoute' decorator.
type Renderer struct {
	Inner workloads.WorkloadRenderer
}

// AllocateBindings is the WorkloadRenderer implementation for the radius.dev/InboundRoute' decorator.
func (r Renderer) AllocateBindings(ctx context.Context, workload workloads.InstantiatedWorkload, resources []workloads.WorkloadResourceProperties) (map[string]components.BindingState, error) {

	// InboundRoute doesn't affect bindings
	return r.Inner.AllocateBindings(ctx, workload, resources)
}

// Render is the WorkloadRenderer implementation for the radius.dev/InboundRoute' decorator.
func (r Renderer) Render(ctx context.Context, w workloads.InstantiatedWorkload) ([]outputresource.OutputResource, error) {
	// Let the inner renderer do its work
	resources, err := r.Inner.Render(ctx, w)
	if err != nil {
		// Even if the operation fails, return the output resources created so far
		// TODO: This is temporary. Once there are no resources actually deployed during render phase,
		// we no longer need to track the output resources on error
		return resources, err
	}

	trait := Trait{}
	found, err := w.Workload.FindTrait(Kind, &trait)
	if !found || err != nil {
		// Even if the operation fails, return the output resources created so far
		// TODO: This is temporary. Once there are no resources actually deployed during render phase,
		// we no longer need to track the output resources on error
		return resources, err
	}

	if trait.Binding == "" {
		// Even if the operation fails, return the output resources created so far
		// TODO: This is temporary. Once there are no resources actually deployed during render phase,
		// we no longer need to track the output resources on error
		return resources, fmt.Errorf("the binding field is required for trait '%s'", Kind)
	}

	provides, ok := w.Workload.Bindings[trait.Binding]
	if !ok {
		// Even if the operation fails, return the output resources created so far
		// TODO: This is temporary. Once there are no resources actually deployed during render phase,
		// we no longer need to track the output resources on error
		return resources, fmt.Errorf("cannot find the binding '%s' referenced by '%s' trait", trait.Binding, Kind)
	}

	httpBinding := containerv1alpha1.HTTPBinding{}
	err = provides.AsRequired(containerv1alpha1.KindHTTP, &httpBinding)
	if err != nil {
		return resources, err
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

	backend := networkingv1.IngressBackend{
		Service: &networkingv1.IngressServiceBackend{
			Name: w.Name,
			Port: networkingv1.ServiceBackendPort{
				Number: int32(httpBinding.GetEffectivePort()),
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
		Kind:     outputresource.KindKubernetes,
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
