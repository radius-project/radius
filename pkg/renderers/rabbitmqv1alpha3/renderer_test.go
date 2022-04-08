// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package rabbitmqv1alpha3

import (
	"context"
	"testing"

	"github.com/Azure/go-autorest/autorest/to"
	"github.com/go-logr/logr"
	"github.com/project-radius/radius/pkg/radlogger"
	"github.com/project-radius/radius/pkg/renderers"
	"github.com/stretchr/testify/require"
)

const (
	applicationName = "test-app"
	resourceName    = "test-rabbitmq"
)

func createContext(t *testing.T) context.Context {
	logger, err := radlogger.NewTestLogger(t)
	if err != nil {
		t.Log("Unable to initialize logger")
		return context.Background()
	}
	return logr.NewContext(context.Background(), logger)
}

func Test_Render_User_Secrets(t *testing.T) {
	ctx := createContext(t)
	renderer := Renderer{}

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
