// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package redisv1alpha3

import (
	"context"

	"github.com/project-radius/radius/pkg/azure/azresources"
	"github.com/project-radius/radius/pkg/azure/radclient"
	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/renderers"
)

var _ renderers.Renderer = (*KubernetesRenderer)(nil)

type KubernetesRenderer struct {
}

func (r *KubernetesRenderer) GetDependencyIDs(ctx context.Context, workload renderers.RendererResource) ([]azresources.ResourceID, []azresources.ResourceID, error) {
	return nil, nil, nil
}

func (r *KubernetesRenderer) Render(ctx context.Context, options renderers.RenderOptions) (renderers.RendererOutput, error) {
	resource := options.Resource

	properties := radclient.RedisCacheResourceProperties{}
	err := resource.ConvertDefinition(&properties)
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	output := renderers.RendererOutput{
		// It should be possible to handle the computed values in a
		// generic manner. However the slightly tricky part is knowing
		// which fields are part of the non-secret connection-provided
		// data. As a result We still have to duplicate this logic here
		// instead of sharing the same logic across renderers.
		//
		// If we have the schema of the connection-based properties, we
		// could do it more generically here.
		ComputedValues: map[string]renderers.ComputedValueReference{
			"host": {
				Value: properties.Host,
			},
			"port": {
				Value: properties.Port,
			},
			"username": {
				Value: "",
			},
		},
		SecretValues: map[string]renderers.SecretValueReference{
			"password": {
				LocalID:       outputresource.LocalIDScrapedSecret,
				ValueSelector: "password",
			},
			// TODO: generate a default connection string when the secret was not provided.
			"connectionString": {
				LocalID:       outputresource.LocalIDScrapedSecret,
				ValueSelector: "connectionString",
			},
		},
	}
	return output, nil
}
