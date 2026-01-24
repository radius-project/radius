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

package api

import (
	"context"
	"net/http"
	"testing"

	"github.com/go-chi/chi/v5"
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/armrpc/asyncoperation/statusmanager"
	"github.com/radius-project/radius/pkg/armrpc/hostoptions"
	"github.com/radius-project/radius/pkg/armrpc/rpctest"
	"github.com/radius-project/radius/pkg/components/database/databaseprovider"
	"github.com/radius-project/radius/pkg/components/secret/secretprovider"
	"github.com/radius-project/radius/pkg/ucp"
	"github.com/radius-project/radius/pkg/ucp/frontend/modules"
	"github.com/radius-project/radius/test/testcontext"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func Test_Routes(t *testing.T) {
	pathBase := "/some-path-base"
	tests := []rpctest.HandlerTestSpec{
		{
			OperationType: v1.OperationType{Type: OperationTypeKubernetesOpenAPIV2Doc, Method: v1.OperationGet},
			Method:        http.MethodGet,
			Path:          "/openapi/v2",
			SkipPathBase:  true,
		},
		{
			OperationType: v1.OperationType{Type: OperationTypeKubernetesOpenAPIV3Doc, Method: v1.OperationGet},
			Method:        http.MethodGet,
			Path:          "/openapi/v3",
			SkipPathBase:  true,
		},
		{
			OperationType: v1.OperationType{Type: OperationTypeKubernetesDiscoveryDoc, Method: v1.OperationGet},
			Method:        http.MethodGet,
			Path:          "",
		},
		{
			OperationType: v1.OperationType{Type: OperationTypePlanes, Method: v1.OperationList},
			Method:        http.MethodGet,
			Path:          "/planes",
		},
		{
			// Should be passed to the module.
			Method: http.MethodGet,
			Path:   "/planes/someType",
		},
		{
			// Should be passed to the module.
			Method: http.MethodGet,
			Path:   "/planes/someType/someName",
		},
		{
			// Should be passed to the module.
			Method: http.MethodPost,
			Path:   "/planes/someType/someName/some/other/path",
		},
		{
			// Should be matched by the "unknown plane" route
			Method: http.MethodPost,
			Path:   "/planes/anotherType",
		},
	}

	options := &ucp.Options{
		Config: &ucp.Config{
			Server: hostoptions.ServerOptions{
				Host:     "localhost",
				Port:     8080,
				PathBase: pathBase,
			},
		},
		DatabaseProvider: databaseprovider.FromMemory(),
		SecretProvider:   secretprovider.NewSecretProvider(secretprovider.SecretProviderOptions{Provider: secretprovider.TypeInMemorySecret}),
		StatusManager:    statusmanager.NewMockStatusManager(gomock.NewController(t)),
	}

	rpctest.AssertRouters(t, tests, pathBase, "", func(ctx context.Context) (chi.Router, error) {
		r := chi.NewRouter()
		return r, Register(ctx, r, []modules.Initializer{&testModule{}}, options)
	})
}

func Test_Route_ToModule(t *testing.T) {
	pathBase := "/some-path-base"

	options := &ucp.Options{
		Config: &ucp.Config{
			Server: hostoptions.ServerOptions{
				Host:     "localhost",
				Port:     8080,
				PathBase: pathBase,
			},
		},
		DatabaseProvider: databaseprovider.FromMemory(),
		SecretProvider:   secretprovider.NewSecretProvider(secretprovider.SecretProviderOptions{Provider: secretprovider.TypeInMemorySecret}),
		StatusManager:    statusmanager.NewMockStatusManager(gomock.NewController(t)),
	}

	r := chi.NewRouter()
	err := Register(testcontext.New(t), r, []modules.Initializer{&testModule{}}, options)
	require.NoError(t, err)

	tctx := chi.NewRouteContext()
	tctx.Reset()

	matched := r.Match(tctx, http.MethodGet, pathBase+"/planes/someType/someName/anotherpath")
	require.True(t, matched)
}

func Test_trimProxyPath(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		pathBase string
		expected string
	}{
		{
			name:     "strips path base and preserves remaining path",
			path:     "/apis/api.ucp.dev/v1alpha3/installer/terraform/status",
			pathBase: "/apis/api.ucp.dev/v1alpha3",
			expected: "/installer/terraform/status",
		},
		{
			name:     "returns root when path equals path base",
			path:     "/apis/api.ucp.dev/v1alpha3",
			pathBase: "/apis/api.ucp.dev/v1alpha3",
			expected: "/",
		},
		{
			name:     "ensures leading slash when missing after trim",
			path:     "/apis/api.ucp.dev/v1alpha3installer/terraform/status",
			pathBase: "/apis/api.ucp.dev/v1alpha3",
			expected: "/installer/terraform/status",
		},
		{
			name:     "handles empty path base",
			path:     "/installer/terraform/status",
			pathBase: "",
			expected: "/installer/terraform/status",
		},
		{
			name:     "handles path not starting with path base",
			path:     "/other/path",
			pathBase: "/apis/api.ucp.dev/v1alpha3",
			expected: "/other/path",
		},
		{
			name:     "handles install endpoint",
			path:     "/apis/api.ucp.dev/v1alpha3/installer/terraform/install",
			pathBase: "/apis/api.ucp.dev/v1alpha3",
			expected: "/installer/terraform/install",
		},
		{
			name:     "handles nested paths correctly",
			path:     "/apis/api.ucp.dev/v1alpha3/installer/terraform/versions/1.6.4",
			pathBase: "/apis/api.ucp.dev/v1alpha3",
			expected: "/installer/terraform/versions/1.6.4",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := trimProxyPath(tt.path, tt.pathBase)
			require.Equal(t, tt.expected, result)
		})
	}
}

type testModule struct {
}

func (m *testModule) Initialize(ctx context.Context) (http.Handler, error) {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}), nil
}

func (m *testModule) PlaneType() string {
	return "someType"
}
