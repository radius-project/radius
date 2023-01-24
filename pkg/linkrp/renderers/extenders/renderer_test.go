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
	"github.com/project-radius/radius/pkg/linkrp"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/pkg/linkrp/renderers"
	"github.com/project-radius/radius/pkg/rp"
	"github.com/project-radius/radius/pkg/ucp/ucplog"
	"github.com/stretchr/testify/require"
)

func createContext(t *testing.T) context.Context {
	logger, err := ucplog.NewTestLogger(t)
	if err != nil {
		t.Log("Unable to initialize logger")
		return context.Background()
	}
	return logr.NewContext(context.Background(), logger)
}

func Test_Render_Success(t *testing.T) {
	ctx := createContext(t)
	resource := datamodel.Extender{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:   "/subscriptions/testSub/resourceGroups/testGroup/providers/Applications.Link/extenders/test-secret-store",
				Name: "test-secret-store",
				Type: linkrp.ExtendersResourceType,
			},
		},
		Properties: datamodel.ExtenderProperties{
			BasicResourceProperties: rp.BasicResourceProperties{
				Application: "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/applications/testApplication",
				Environment: "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/environments/env0",
			},
			AdditionalProperties: map[string]any{
				"foo": "bar",
			},
			Secrets: map[string]any{
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

func Test_Render_InvalidApplicationID(t *testing.T) {
	ctx := createContext(t)
	resource := datamodel.Extender{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:   "/subscriptions/testSub/resourceGroups/testGroup/providers/Applications.Link/extenders/test-secret-store",
				Name: "test-secret-store",
				Type: linkrp.ExtendersResourceType,
			},
		},
		Properties: datamodel.ExtenderProperties{
			BasicResourceProperties: rp.BasicResourceProperties{
				Application: "invalid-app-id",
				Environment: "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/environments/env0",
			},
			AdditionalProperties: map[string]any{
				"foo": "bar",
			},
			Secrets: map[string]any{
				"secretname": "secretvalue",
			},
		},
	}
	renderer := Renderer{}
	_, err := renderer.Render(ctx, &resource, renderers.RenderOptions{})
	require.Error(t, err)
	require.Equal(t, v1.CodeInvalid, err.(*v1.ErrClientRP).Code)
	require.Equal(t, "failed to parse application from the property: 'invalid-app-id' is not a valid resource id", err.(*v1.ErrClientRP).Message)
}
