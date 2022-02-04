// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package genericv1alpha3

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"github.com/project-radius/radius/pkg/radlogger"
	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/renderers"
	"github.com/project-radius/radius/pkg/resourcekinds"
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

func Test_Render_Success(t *testing.T) {
	ctx := createContext(t)

	dependencies := map[string]renderers.RendererDependency{}
	resource := renderers.RendererResource{
		ApplicationName: "test-app",
		ResourceName:    "test-resource",
		ResourceType:    ResourceType,
		Definition: map[string]interface{}{
			"properties": map[string]interface{}{
				"foo": "bar",
			},
			"secrets": map[string]interface{}{
				"secretname": "secretvalue",
			},
		},
	}

	renderer := AzureRenderer{}
	result, err := renderer.Render(ctx, renderers.RenderOptions{Resource: resource, Dependencies: dependencies})
	require.NoError(t, err)

	require.Equal(t, 1, len(result.Resources))
	require.Equal(t, outputresource.LocalIDGeneric, result.Resources[0].LocalID)
	require.Equal(t, resourcekinds.Generic, result.Resources[0].ResourceKind)

	expected := map[string]renderers.ComputedValueReference{
		"foo": {Value: "bar"},
	}
	require.Equal(t, expected, result.ComputedValues)

	expectedSecrets := map[string]renderers.SecretValueReference{
		"secretname": {
			Value: "secretvalue",
		},
	}
	require.Equal(t, expectedSecrets, result.SecretValues)
}
