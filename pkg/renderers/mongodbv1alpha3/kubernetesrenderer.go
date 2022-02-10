// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package mongodbv1alpha3

import (
	"context"

	"github.com/project-radius/radius/pkg/azure/azresources"
	"github.com/project-radius/radius/pkg/azure/radclient"
	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/renderers"
)

const (
	SecretKeyMongoDBAdminUsername    = "MONGO_ROOT_USERNAME"
	SecretKeyMongoDBAdminPassword    = "MONGO_ROOT_PASSWORD"
	SecretKeyMongoDBConnectionString = "MONGO_CONNECTIONSTRING"
)

var _ renderers.Renderer = (*KubernetesRenderer)(nil)

type KubernetesRenderer struct {
}

type KubernetesOptions struct {
	DescriptiveLabels map[string]string
	SelectorLabels    map[string]string
	Namespace         string
	Name              string
}

func (r *KubernetesRenderer) GetDependencyIDs(ctx context.Context, resource renderers.RendererResource) ([]azresources.ResourceID, []azresources.ResourceID, error) {
	return nil, nil, nil
}

func (r *KubernetesRenderer) Render(ctx context.Context, options renderers.RenderOptions) (renderers.RendererOutput, error) {
	properties := radclient.MongoDBResourceProperties{}
	resource := options.Resource
	err := resource.ConvertDefinition(&properties)
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	computedValues := map[string]renderers.ComputedValueReference{
		"database": {
			Value: resource.ResourceName,
		},
	}

	output := renderers.RendererOutput{
		ComputedValues: computedValues,
		SecretValues: map[string]renderers.SecretValueReference{
			"connectionString": {
				LocalID:       outputresource.LocalIDScrapedSecret,
				ValueSelector: "connectionString",
			},
		},
	}
	return output, nil
}
