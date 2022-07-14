// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package extenders

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/connectorrp/datamodel"
	"github.com/project-radius/radius/pkg/connectorrp/renderers"
	"github.com/project-radius/radius/pkg/radlogger"
	"github.com/project-radius/radius/pkg/rp"
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
	resource := datamodel.Extender{
		TrackedResource: v1.TrackedResource{
			ID:   "/subscriptions/testSub/resourceGroups/testGroup/providers/Applications.Connector/daprSecretStores/test-secret-store",
			Name: "test-secret-store",
			Type: "Applications.Connector/daprSecretStores",
		},
		Properties: datamodel.ExtenderProperties{
			ExtenderResponseProperties: datamodel.ExtenderResponseProperties{
				Application: "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/applications/testApplication",
				Environment: "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/environments/env0",
				AdditionalProperties: map[string]interface{}{
					"foo": "bar",
				},
			},
			Secrets: map[string]interface{}{
				"secretname": "secretvalue",
			},
		},
	}
	renderer := Renderer{}
	result, err := renderer.Render(ctx, &resource, renderers.RenderOptions{})
	require.NoError(t, err)

	require.Equal(t, 0, len(result.Resources))

	expected := map[string]renderers.ComputedValueReference{
		"foo": {Value: "bar"},
	}
	require.Equal(t, expected, result.ComputedValues)

	expectedSecrets := map[string]rp.SecretValueReference{
		"secretname": {
			Value: "secretvalue",
		},
	}
	require.Equal(t, expectedSecrets, result.SecretValues)
}
