// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package rabbitmqv1alpha3

import (
	"testing"

	"github.com/Azure/go-autorest/autorest/to"
	"github.com/project-radius/radius/pkg/renderers"
	"github.com/stretchr/testify/require"
)

const (
	applicationName = "test-app"
	resourceName    = "test-rabbitmq"
)

func Test_Azure_Render_Unmanaged_User_Secrets(t *testing.T) {
	ctx := createContext(t)
	renderer := AzureRenderer{}

	resource := renderers.RendererResource{
		ApplicationName: applicationName,
		ResourceName:    resourceName,
		ResourceType:    ResourceType,
		Definition: map[string]interface{}{
			"host": "localhost",
			"port": 42,
			"secrets": map[string]string{
				"connectionString": "admin:deadbeef@localhost:42",
			},
		},
	}

	output, err := renderer.Render(ctx, renderers.RenderOptions{Resource: resource, Dependencies: map[string]renderers.RendererDependency{}})
	require.NoError(t, err)

	require.Len(t, output.Resources, 0)

	expectedComputedValues := map[string]renderers.ComputedValueReference{
		"connectionString": {
			Value: to.StringPtr("admin:deadbeef@localhost:42"),
		},
	}
	require.Equal(t, expectedComputedValues, output.ComputedValues)
	require.Equal(t, 0, len(output.SecretValues))
}
