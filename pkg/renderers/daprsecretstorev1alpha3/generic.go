// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package daprsecretstorev1alpha3

import (
	"github.com/project-radius/radius/pkg/azure/radclient"
	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/renderers"
	"github.com/project-radius/radius/pkg/renderers/dapr"
)

func GetDaprSecretStoreGeneric(resource renderers.RendererResource) ([]outputresource.OutputResource, error) {
	properties := radclient.DaprSecretStoreProperties{}
	err := resource.ConvertDefinition(&properties)
	if err != nil {
		return nil, err
	}

	daprGeneric := dapr.DaprGeneric{
		Type:     properties.Type,
		Version:  properties.Version,
		Metadata: properties.Metadata,
	}

	return dapr.GetDaprGeneric(daprGeneric, resource)
}
