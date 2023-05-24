/*
Copyright 2023 The Radius Authors.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package daprinvokehttproutes

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/linkrp"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/pkg/linkrp/renderers"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
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
	renderer := Renderer{}

	resource := datamodel.DaprInvokeHttpRoute{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:   "/subscriptions/testSub/resourceGroups/testGroup/providers/Applications.Link/daprInvokeHttpRoutes/test.http.route",
				Name: "test.http.route",
				Type: linkrp.DaprInvokeHttpRoutesResourceType,
			},
		},
		Properties: datamodel.DaprInvokeHttpRouteProperties{
			BasicResourceProperties: rpv1.BasicResourceProperties{
				Application: "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/applications/testApplication",
				Environment: "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/environments/env0",
			},
			AppId: "test-appId",
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

func Test_Render_InvalidApplicationID(t *testing.T) {
	ctx := createContext(t)
	renderer := Renderer{}

	resource := datamodel.DaprInvokeHttpRoute{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:   "/subscriptions/testSub/resourceGroups/testGroup/providers/Applications.Link/daprInvokeHttpRoutes/test-http-route",
				Name: "test-http-route",
				Type: linkrp.DaprInvokeHttpRoutesResourceType,
			},
		},
		Properties: datamodel.DaprInvokeHttpRouteProperties{
			BasicResourceProperties: rpv1.BasicResourceProperties{
				Application: "invalid-app-id",
				Environment: "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/environments/env0",
			},
			AppId: "test-appId",
		},
	}
	_, err := renderer.Render(ctx, &resource, renderers.RenderOptions{})
	require.Error(t, err)
	require.Equal(t, v1.CodeInvalid, err.(*v1.ErrClientRP).Code)
	require.Equal(t, "failed to parse application from the property: 'invalid-app-id' is not a valid resource id", err.(*v1.ErrClientRP).Message)
}
