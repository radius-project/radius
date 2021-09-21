// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package volumev1alpha1

import (
	"context"
	"fmt"
	"net/url"

	"github.com/Azure/radius/pkg/azure/armauth"
	"github.com/Azure/radius/pkg/handlers"
	"github.com/Azure/radius/pkg/model/components"
	"github.com/Azure/radius/pkg/radrp/outputresource"
	"github.com/Azure/radius/pkg/renderers"
	"github.com/Azure/radius/pkg/resourcekinds"
	"github.com/Azure/radius/pkg/workloads"
)

// Renderer is the WorkloadRenderer implementation for containerized workload.
type Renderer struct {
	Arm armauth.ArmConfig
}

// Allocate allocates bindings for containerized workload
func (r Renderer) AllocateBindings(ctx context.Context, workload workloads.InstantiatedWorkload, resources []workloads.WorkloadResourceProperties) (map[string]components.BindingState, error) {
	// For containers we only *natively* expose HTTP as a binding type - other binding types
	// might be handled by traits (decorators), so don't error out on those.
	//
	// The calling infrastructure will validate that any bindings specified by the user have a matching
	// binding in the outputs.
	bindings := map[string]components.BindingState{}

	for name, connection := range workload.Workload.Connections {
		// TODO: Look at rbac and mount path
		if connection.Kind != VolumeKindConfigMap {
			continue
		}

		http := HTTPBinding{}
		err := binding.AsRequired(KindHTTP, &http)
		if err != nil {
			return nil, err
		}

		namespace := workload.Namespace
		if namespace == "" {
			namespace = workload.Application
		}

		uri := url.URL{
			Scheme: binding.Kind,
			Host:   fmt.Sprintf("%v.%v.svc.cluster.local", workload.Name, namespace),
		}

		if http.GetEffectivePort() != 80 {
			uri.Host = uri.Host + fmt.Sprintf(":%d", http.GetEffectivePort())
		}

		bindings[name] = components.BindingState{
			Component: workload.Name,
			Binding:   name,
			Kind:      KindHTTP,
			Properties: map[string]interface{}{
				"uri":    uri.String(),
				"scheme": uri.Scheme,
				"host":   uri.Hostname(),
				"port":   fmt.Sprintf("%d", http.GetEffectivePort()),
			},
		}
	}

	return bindings, nil
}

// Render is the WorkloadRenderer implementation for containerized workload.
func (r Renderer) Render(ctx context.Context, workload workloads.InstantiatedWorkload) ([]outputresource.OutputResource, error) {
	component := &VolumeComponent{}
	err := workload.Workload.AsRequired(Kind, &component)
	if err != nil {
		return []outputresource.OutputResource{}, err
	}

	if component.Config.Managed {
		if component.Config.Resource != "" {
			return nil, renderers.ErrResourceSpecifiedForManagedResource
		}

		// generate data we can use to manage a cosmosdb instance
		resource := outputresource.OutputResource{
			Kind:    resourcekinds.Volume,
			Type:    outputresource.TypeKubernetes,
			LocalID: outputresource.LocalIDVolume,
			Resource: map[string]string{
				handlers.ManagedKey: "true",
			},
		}

		return []outputresource.OutputResource{resource}, nil
	}

	if component.Config.Resource == "" {
		return nil, renderers.ErrResourceMissingForUnmanagedResource
	}

	// TODO
	// Verify existence of volume

	resource := outputresource.OutputResource{
		Kind:    resourcekinds.AzureCosmosDBSQL,
		Type:    outputresource.TypeARM,
		LocalID: outputresource.LocalIDAzureCosmosDBSQL,
		Resource: map[string]string{
			handlers.ManagedKey: "false",

			//TODO : Set details of the unmanaged volume
		},
	}
	return []outputresource.OutputResource{resource}, nil
}
