// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package rabbitmqv1alpha3

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"github.com/project-radius/radius/pkg/radlogger"
	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/renderers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func createContext(t *testing.T) context.Context {
	logger, err := radlogger.NewTestLogger(t)
	if err != nil {
		t.Log("Unable to initialize logger")
		return context.Background()
	}
	return logr.NewContext(context.Background(), logger)
}

func TestRenderUnmanaged(t *testing.T) {
	ctx := createContext(t)
	renderer := KubernetesRenderer{}

	resource := renderers.RendererResource{
		ApplicationName: "test-app",
		ResourceName:    "test-resource",
		ResourceType:    ResourceType,
		Definition: map[string]interface{}{
			"queue": "cool-queue",
			"secrets": map[string]interface{}{
				"connectionString": "cool-connection-string",
			},
		},
	}

	output, err := renderer.Render(ctx, renderers.RenderOptions{Resource: resource, Dependencies: map[string]renderers.RendererDependency{}})
	assert.NoError(t, err)
	assert.Equal(t, renderers.RendererOutput{
		ComputedValues: map[string]renderers.ComputedValueReference{
			"queue": {
				Value: "cool-queue",
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

func TestInvalidKubernetesMissingQueueName(t *testing.T) {
	renderer := KubernetesRenderer{}

	dependencies := map[string]renderers.RendererDependency{}
	resource := renderers.RendererResource{
		ApplicationName: "test-app",
		ResourceName:    "test-resource",
		ResourceType:    ResourceType,
		Definition: map[string]interface{}{
			"managed": true,
		},
	}

	_, err := renderer.Render(context.Background(), renderers.RenderOptions{Resource: resource, Dependencies: dependencies})
	require.Error(t, err)
	require.Equal(t, "queue name must be specified", err.Error())
}
