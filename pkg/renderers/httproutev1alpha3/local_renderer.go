// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package httproutev1alpha3

import (
	"context"
	"fmt"

	"github.com/Azure/radius/pkg/azure/azresources"
	"github.com/Azure/radius/pkg/azure/radclient"
	"github.com/Azure/radius/pkg/kubernetes"
	"github.com/Azure/radius/pkg/renderers"
)

type LocalRenderer struct {
}

func (r LocalRenderer) GetDependencyIDs(ctx context.Context, resource renderers.RendererResource) ([]azresources.ResourceID, error) {
	// No Dependencies when running locally
	return nil, nil
}

func (r LocalRenderer) Render(ctx context.Context, options renderers.RenderOptions) (renderers.RendererOutput, error) {
	route := radclient.HTTPRouteProperties{}
	resource := options.Resource

	err := resource.ConvertDefinition(&route)
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	computedValues := map[string]renderers.ComputedValueReference{
		"host": {
			Value: "localhost",
		},
		"port": {
			Value: GetEffectivePort(route),
		},
		"url": {
			Value: fmt.Sprintf("http://%s:%d", kubernetes.MakeResourceName(resource.ApplicationName, resource.ResourceName), GetEffectivePort(route)),
		},
		"scheme": {
			Value: "http",
		},
	}
	return renderers.RendererOutput{
		ComputedValues: computedValues,
	}, nil
}
