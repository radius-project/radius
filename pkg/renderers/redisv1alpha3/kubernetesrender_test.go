// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package redisv1alpha3

import (
	"context"
	"testing"

	"github.com/Azure/go-autorest/autorest/to"
	"github.com/go-logr/logr"
	"github.com/project-radius/radius/pkg/radlogger"
	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/renderers"
	"github.com/stretchr/testify/require"
	"gotest.tools/assert"
)

func createContext(t *testing.T) context.Context {
	logger, err := radlogger.NewTestLogger(t)
	if err != nil {
		t.Log("Unable to initialize logger")
		return context.Background()
	}
	return logr.NewContext(context.Background(), logger)
}

func TestRenderRedis(t *testing.T) {
	ctx := createContext(t)
	renderer := KubernetesRenderer{}

	input := renderers.RendererResource{
		ApplicationName: "test-app",
		ResourceName:    "test-redis",
		ResourceType:    ResourceType,
		Definition: map[string]interface{}{
			"host": "hello.com",
			"port": 1234,
			"secrets": map[string]interface{}{
				"connectionString": "***",
				"password":         "***",
			},
		},
	}
	output, err := renderer.Render(ctx, renderers.RenderOptions{
		Resource:     input,
		Dependencies: map[string]renderers.RendererDependency{},
	})
	require.NoError(t, err)

	expected := renderers.RendererOutput{
		ComputedValues: map[string]renderers.ComputedValueReference{
			"host": {
				Value: to.StringPtr("hello.com"),
			},
			"port": {
				Value: to.Int32Ptr(1234),
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
			"connectionString": {
				LocalID:       outputresource.LocalIDScrapedSecret,
				ValueSelector: "connectionString",
			},
		},
	}
	assert.DeepEqual(t, expected, output)
}
