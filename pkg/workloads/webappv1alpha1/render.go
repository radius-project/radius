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
)

// Renderer is the WorkloadRenderer implementation for the Azure Web App workload.
type Renderer struct {
}

// Allocate is the WorkloadRenderer implementation for Azure Web App workload.
func (r Renderer) Allocate(ctx context.Context, w workloads.InstantiatedWorkload, wrp []workloads.WorkloadResourceProperties, service workloads.WorkloadService) (map[string]interface{}, error) {
	if service.Kind != "http" {
		return nil, fmt.Errorf("the service is not supported")
	}

	// We can ignore the name, web apps always provide an HTTP service.
	if node, ok := w.Workload.UnstructuredContent()["spec"]; ok {
		if spec, ok := node.(map[string]interface{}); ok {
			// httpOptions is optional, as is appPort
			port := 80
			if node, ok := spec["httpOptions"]; ok {
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
				Host:   fmt.Sprintf("%v.%v.svc.cluster.local", w.Workload.GetName(), w.Workload.GetNamespace()),
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
	}

	// if we get here then the workload is likely malformed
	return nil, fmt.Errorf("the service could not be found")
}

// Render is the WorkloadRenderer implementation for the Azure Web App workload.
func (r Renderer) Render(ctx context.Context, w workloads.InstantiatedWorkload) ([]workloads.WorkloadResource, error) {
	// The workload is mostly in the right format - we need to massage the APIVersion and Kind to
	// match k4se.
	rendered := w.Workload.DeepCopy()
	rendered.SetAPIVersion("k8se.microsoft.com/v1alpha1")
	rendered.SetKind("App")

	return []workloads.WorkloadResource{workloads.NewKubernetesResource("App", rendered)}, nil
}
