// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

package controller

import (
	"context"
	"testing"

	"github.com/radius-project/radius/pkg/dynamicrp/datamodel"
	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/recipes/engine"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestDelegatingEngineUsesDesiredSnapshot(t *testing.T) {
	t.Parallel()
	mock := engine.NewMockEngine(gomock.NewController(t))
	incoming := []recipes.IncomingReference{{ID: "route", Revision: "r1"}}
	desired := map[string]any{"image": "example"}
	output := &recipes.RecipeOutput{DelegatedOutputs: []recipes.DelegatedOutput{{ID: "route", Revision: "r1"}}}
	mock.EXPECT().Execute(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, options engine.ExecuteOptions) (*recipes.RecipeOutput, error) {
			require.Equal(t, incoming, options.Recipe.IncomingReferences)
			require.Equal(t, desired, options.Recipe.Properties)
			require.Equal(t, []string{"owned"}, options.PreviousState)
			return output, nil
		})
	wrapper := &delegatingEngine{Engine: mock, incoming: incoming, desired: desired}
	actual, err := wrapper.Execute(t.Context(), engine.ExecuteOptions{
		BaseOptions:   engine.BaseOptions{Recipe: recipes.ResourceMetadata{Properties: map[string]any{"status": "not-input"}}},
		PreviousState: []string{"owned"},
	})
	require.NoError(t, err)
	require.Same(t, output, actual)
	require.Same(t, output, wrapper.output)
}

func TestDelegatingEngineDeleteProvidesEmptyReferences(t *testing.T) {
	t.Parallel()
	mock := engine.NewMockEngine(gomock.NewController(t))
	mock.EXPECT().Delete(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, options engine.DeleteOptions) error {
			require.NotNil(t, options.Recipe.IncomingReferences)
			require.Empty(t, options.Recipe.IncomingReferences)
			require.Equal(t, map[string]any{"image": "example"}, options.Recipe.Properties)
			return nil
		})
	wrapper := &delegatingEngine{
		Engine: mock, incoming: []recipes.IncomingReference{},
		desired: map[string]any{"image": "example"},
	}
	require.NoError(t, wrapper.Delete(t.Context(), engine.DeleteOptions{}))
}

func TestProjectDelegatedValues(t *testing.T) {
	t.Parallel()
	resourceSchema := map[string]any{"type": "object", "properties": map[string]any{
		"image":    map[string]any{"type": "string"},
		"endpoint": map[string]any{"type": "integer", "readOnly": true},
		"status":   map[string]any{"type": "object", "readOnly": true},
	}}
	for _, test := range []struct {
		name   string
		values map[string]any
		valid  bool
	}{
		{"computed", map[string]any{"endpoint": 8080}, true},
		{"desired overwrite", map[string]any{"image": "bad"}, false},
		{"unknown", map[string]any{"foreign": 4}, false},
		{"framework state", map[string]any{"status": map[string]any{}}, false},
		{"invalid value type", map[string]any{"endpoint": "not-an-integer"}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			resource := &datamodel.DynamicResource{Properties: map[string]any{"image": "keep"}}
			err := projectDelegatedValues(t.Context(), resource, resourceSchema, test.values)
			if test.valid {
				require.NoError(t, err)
				require.Equal(t, 8080, resource.Properties["endpoint"])
			} else {
				require.Error(t, err)
				require.NotContains(t, resource.Properties, "endpoint")
			}
			require.Equal(t, "keep", resource.Properties["image"])
		})
	}
}
