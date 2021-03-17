// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package webappv1alpha1

import (
	"context"
	"fmt"
	"net/url"

	"github.com/Azure/radius/pkg/workloads"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// Renderer is the WorkloadRenderer implementation for the Azure Web App workload.
type Renderer struct {
}

// Allocate is the WorkloadRenderer implementation for Azure Web App workload.
func (r Renderer) Allocate(ctx context.Context, w workloads.InstantiatedWorkload, wrp []workloads.WorkloadResourceProperties, service workloads.WorkloadService) (map[string]interface{}, error) {
	if service.Kind != "http" {
		return nil, fmt.Errorf("the service is not supported")
	}

	if w.Workload.Run == nil {
		return nil, fmt.Errorf("component is invalid")
	}

	// We can ignore the name, functions always provide an HTTP service.
	//
	// httpOptions is optional, as is appPort
	port := 80
	if node, ok := w.Workload.Run["httpOptions"]; ok {
		if httpOptions, ok := node.(map[string]interface{}); ok {
			if node, ok := httpOptions["appPort"]; ok {
				if appPort, ok := node.(int); ok {
					port = appPort
				}
			}
		}
	}

	uri := url.URL{
		Scheme: service.Kind,
		Host:   fmt.Sprintf("%v.%v.svc.cluster.local", w.Name, w.Application),
	}

	if port != 80 {
		uri.Host = uri.Host + fmt.Sprintf(":%d", port)
	}

	values := map[string]interface{}{}
	values["uri"] = uri.String()
	values["scheme"] = uri.Scheme
	values["host"] = uri.Hostname()
	values["port"] = fmt.Sprintf("%d", port)
	return values, nil
}

// Render is the WorkloadRenderer implementation for the Azure Web App workload.
func (r Renderer) Render(ctx context.Context, w workloads.InstantiatedWorkload) ([]workloads.WorkloadResource, error) {
	// The workload is mostly in the right format - we need to massage the APIVersion and Kind to
	// match k4se.
	resource := unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "k8se.microsoft.com/v1alpha1",
			"kind":       "App",
			"metadata": map[string]interface{}{
				"name":      w.Workload.Name,
				"namespace": w.Application,
				"labels": map[string]interface{}{
					"radius.dev/application": w.Application,
					"radius.dev/component":   w.Name,
					// TODO get the component revision here...
					"app.kubernetes.io/name":       w.Name,
					"app.kubernetes.io/part-of":    w.Application,
					"app.kubernetes.io/managed-by": "radius-rp",
				},
			},

			// Config section is already in the right format
			"spec": w.Workload.Config,
		},
	}

	return []workloads.WorkloadResource{workloads.NewKubernetesResource("App", &resource)}, nil
}
