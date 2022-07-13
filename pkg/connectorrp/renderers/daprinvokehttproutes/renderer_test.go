// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package daprinvokehttproutes

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/connectorrp/datamodel"
	"github.com/project-radius/radius/pkg/connectorrp/renderers"
	"github.com/project-radius/radius/pkg/radlogger"
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
	renderer := Renderer{}

	resource := datamodel.DaprInvokeHttpRoute{
		TrackedResource: v1.TrackedResource{
			ID:   "/subscriptions/testSub/resourceGroups/testGroup/providers/Applications.Connector/daprInvokeHttpRoutes/test-http-route",
			Name: "test-http-route",
			Type: "Applications.Connector/daprInvokeHttpRoutes",
		},
		Properties: datamodel.DaprInvokeHttpRouteProperties{
			Application: "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/applications/testApplication",
			Environment: "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/environments/env0",
			AppId:       "test-appId",
		},
	}
	output, err := renderer.Render(ctx, &resource, renderers.RenderOptions{})
	require.NoError(t, err)
	require.NoError(t, err)

	require.Empty(t, output.Resources)

	expectedComputedValues := map[string]renderers.ComputedValueReference{
		"appId": {
			Value: "test-appId",
		},
	}
	require.Equal(t, expectedComputedValues, output.ComputedValues)

	require.Empty(t, output.SecretValues)
}
