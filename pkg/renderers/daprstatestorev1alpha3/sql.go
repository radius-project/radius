// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package daprstatestorev1alpha1

import (
	"errors"

	"github.com/Azure/radius/pkg/handlers"
	"github.com/Azure/radius/pkg/radrp/outputresource"
	"github.com/Azure/radius/pkg/renderers"
	"github.com/Azure/radius/pkg/resourcekinds"
	"github.com/Azure/radius/pkg/workloads"
)

func GetDaprStateStoreSQLServer(w workloads.InstantiatedWorkload, component DaprStateStoreComponent) ([]outputresource.OutputResource, error) {
	if !component.Config.Managed {
		return nil, errors.New("only Radius managed resources are supported for Dapr SQL Server")
	}
	if component.Config.Resource != "" {
		return nil, renderers.ErrResourceSpecifiedForManagedResource
	}
	// generate data we can use to connect to a Storage Account
	resource := outputresource.OutputResource{
		LocalID: outputresource.LocalIDDaprStateStoreSQLServer,
		Kind:    resourcekinds.DaprStateStoreSQLServer,
		Type:    outputresource.TypeARM,
		Managed: true,
		Resource: map[string]string{
			handlers.ManagedKey:              "true",
			handlers.KubernetesNameKey:       w.Name,
			handlers.KubernetesNamespaceKey:  w.Application,
			handlers.KubernetesAPIVersionKey: "dapr.io/v1alpha1",
			handlers.KubernetesKindKey:       "Component",
			handlers.ComponentNameKey:        w.Name,
		},
	}

	return []outputresource.OutputResource{resource}, nil
}
