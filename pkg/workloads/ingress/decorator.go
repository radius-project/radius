// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package ingress

import (
	"context"
	"errors"
	"fmt"

	"github.com/Azure/radius/pkg/workloads"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

// Renderer is the WorkloadRenderer implementation for the ingress decorator.
type Renderer struct {
	Inner workloads.WorkloadRenderer
}

// Allocate is the WorkloadRenderer implementation for the ingress decorator.
func (r Renderer) Allocate(ctx context.Context, w workloads.InstantiatedWorkload, wrp []workloads.WorkloadResourceProperties, service workloads.WorkloadService) (map[string]interface{}, error) {
	return r.Inner.Allocate(ctx, w, wrp, service)
}

// Render is the WorkloadRenderer implementation for the ingress decorator.
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
		// no trait
		return resources, err
	}

	// ingress detected, confirm that we have a matching service.
	name := ""
	port := int32(80)
	for _, res := range resources {
		if res.Type != workloads.ResourceKindKubernetes {
			// Not a kubernetes resource
			continue
		}

		o, ok := res.Resource.(runtime.Object)
		if !ok {
			return []workloads.WorkloadResource{}, errors.New("Found kubernetes resource with non-Kubernetes paylod")
		}

		name = r.getName(o)
		port = r.getPort(o)
		if name != "" {
			break
		}
	}

	// TODO match the name specified by the trait.
	if name == "" {
		return []workloads.WorkloadResource{}, fmt.Errorf("could not find service matching %s", trait.Properties.Service)
	}

	if trait.Properties.Hostname == "" {
		return []workloads.WorkloadResource{}, errors.New("hostname property is required")
	}

	ingress := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "networking.k8s.io/v1",
			"kind":       "Ingress",
			"metadata": map[string]interface{}{
				"name":      w.Name,
				"namespace": w.Application,
				"annotations": map[string]interface{}{
					"cert-manager.io/cluster-issuer": "letsencrypt",
				},
				"labels": map[string]interface{}{
					"radius.dev/application": w.Application,
					"radius.dev/component":   w.Name,
					// TODO get the component revision here...
					"app.kubernetes.io/name":       w.Name,
					"app.kubernetes.io/part-of":    w.Application,
					"app.kubernetes.io/managed-by": "radius-rp",
				},
			},
			"spec": map[string]interface{}{
				"rules": []interface{}{
					map[string]interface{}{
						"host": trait.Properties.Hostname,
						"http": map[string]interface{}{
							"paths": []interface{}{
								map[string]interface{}{
									"pathType": "Prefix",
									"path":     "/",
									"backend": map[string]interface{}{
										"service": map[string]interface{}{
											"name": name,
											"port": map[string]interface{}{
												"number": port,
											},
										},
									},
								},
							},
						},
					},
				},
				"tls": []interface{}{
					map[string]interface{}{
						"hosts": []interface{}{
							trait.Properties.Hostname,
						},
						"secretName": trait.Properties.Service + "-cert",
					},
				},
			},
		},
	}

	resources = append(resources, workloads.NewKubernetesResource("Ingress", ingress))
	return resources, nil
}

func (r Renderer) getName(o runtime.Object) string {
	dep, ok := o.(*corev1.Service)
	if ok {
		return dep.Name
	}

	un, ok := o.(*unstructured.Unstructured)
	if ok {
		return un.GetName()
	}

	return ""
}

func (r Renderer) getPort(o runtime.Object) int32 {
	dep, ok := o.(*corev1.Service)
	if ok {
		for _, port := range dep.Spec.Ports {
			return port.Port
		}
	}

	return 80
}
