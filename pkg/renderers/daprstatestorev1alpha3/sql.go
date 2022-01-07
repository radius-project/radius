// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package daprstatestorev1alpha3

import (
	"errors"

	"github.com/project-radius/radius/pkg/handlers"
	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/renderers"
	"github.com/project-radius/radius/pkg/resourcekinds"
)

func GetDaprStateStoreSQLServer(resource renderers.RendererResource, properties Properties) ([]outputresource.OutputResource, error) {
	if !properties.Managed {
		return nil, errors.New("only Radius managed resources are supported for Dapr SQL Server")
	}
	if properties.Resource != "" {
		return nil, renderers.ErrResourceSpecifiedForManagedResource
	}
	// generate data we can use to connect to a Storage Account
	output := outputresource.OutputResource{
		LocalID:      outputresource.LocalIDDaprStateStoreSQLServer,
		ResourceKind: resourcekinds.DaprStateStoreSQLServer,
		Managed:      true,
		Resource: map[string]string{
			handlers.ManagedKey:              "true",
			handlers.KubernetesNameKey:       resource.ResourceName,
			handlers.KubernetesNamespaceKey:  resource.ApplicationName,
			handlers.KubernetesAPIVersionKey: "dapr.io/v1alpha1",
			handlers.KubernetesKindKey:       "Component",
			handlers.ResourceName:            resource.ResourceName,
		},
	}

	return []outputresource.OutputResource{output}, nil
}
