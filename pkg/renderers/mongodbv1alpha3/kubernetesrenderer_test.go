// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package mongodbv1alpha3

import (
	"testing"

	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/renderers"
	"github.com/stretchr/testify/assert"
)

func Test_KubernetesRenderer_Render_Unmanaged(t *testing.T) {
	ctx := createContext(t)
	renderer := KubernetesRenderer{}

	resource := renderers.RendererResource{
		ApplicationName: applicationName,
		ResourceName:    resourceName,
		ResourceType:    ResourceType,
		Definition: map[string]interface{}{
			"managed": false,
		},
	}

	output, err := renderer.Render(ctx, renderers.RenderOptions{Resource: resource, Dependencies: map[string]renderers.RendererDependency{}})
	assert.NoError(t, err)
	assert.Equal(t, renderers.RendererOutput{
		ComputedValues: map[string]renderers.ComputedValueReference{
			"database": {
				Value: resource.ResourceName,
			},
		},
		SecretValues: map[string]renderers.SecretValueReference{
			"connectionString": {
				LocalID:       outputresource.LocalIDScrapedSecret,
				ValueSelector: "connectionString",
			},
		},
	}, output)
}
