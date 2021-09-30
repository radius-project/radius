// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package redisv1alpha3

import (
	"context"
	"fmt"

	"github.com/Azure/radius/pkg/azure/azresources"
	"github.com/Azure/radius/pkg/handlers"
	"github.com/Azure/radius/pkg/radrp/outputresource"
	"github.com/Azure/radius/pkg/renderers"
	"github.com/Azure/radius/pkg/resourcekinds"
)

var _ renderers.Renderer = (*AzureRenderer)(nil)

type AzureRenderer struct {
}

func (r *AzureRenderer) GetDependencyIDs(ctx context.Context, workload renderers.RendererResource) ([]azresources.ResourceID, error) {
	return nil, nil
}

func (r *AzureRenderer) Render(ctx context.Context, resource renderers.RendererResource, dependencies map[string]renderers.RendererDependency) (renderers.RendererOutput, error) {
	properties := RedisComponentProperties{}
	err := resource.ConvertDefinition(&properties)
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	if properties.Managed {
		output := renderers.RendererOutput{
			Resources: []outputresource.OutputResource{
				{
					LocalID:      outputresource.LocalIDAzureRedis,
					ResourceKind: resourcekinds.AzureRedis,
					Managed:      true,
					Resource: map[string]string{
						handlers.ManagedKey:    "true",
						handlers.RedisBaseName: resource.ResourceName,
					},
				},
			},
			ComputedValues: map[string]renderers.ComputedValueReference{
				// NOTE: this is NOT a secret, it doesn't contain the access keys.
				"connectionString": {
					LocalID:           outputresource.LocalIDAzureRedis,
					PropertyReference: handlers.RedisConnectionStringKey,
				},
				"host": {
					LocalID:           outputresource.LocalIDAzureRedis,
					PropertyReference: handlers.RedisHostKey,
				},
				"port": {
					LocalID:           outputresource.LocalIDAzureRedis,
					PropertyReference: handlers.RedisPortKey,
				},
			},
			SecretValues: map[string]renderers.SecretValueReference{
				"primaryKey": {
					LocalID:       outputresource.LocalIDAzureRedis,
					Action:        "listKeys",
					ValueSelector: "/primaryKey",
				},
				"secondaryKey": {
					LocalID:       outputresource.LocalIDAzureRedis,
					Action:        "listKeys",
					ValueSelector: "/secondaryKey",
				},
			},
		}

		return output, nil
	} else {
		// TODO support managed redis workload
		return renderers.RendererOutput{}, fmt.Errorf("only managed = true is support for azure redis workload")
	}
}
