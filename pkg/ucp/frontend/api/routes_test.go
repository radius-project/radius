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
	"github.com/radius-project/radius/pkg/armrpc/rpctest"
	"github.com/radius-project/radius/pkg/ucp/databaseprovider"
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

	options := modules.Options{
		Address:          "localhost",
		PathBase:         pathBase,
		DatabaseProvider: databaseprovider.FromMemory(),
		StatusManager:    statusmanager.NewMockStatusManager(gomock.NewController(t)),
	}

	rpctest.AssertRouters(t, tests, pathBase, "", func(ctx context.Context) (chi.Router, error) {
		r := chi.NewRouter()
		return r, Register(ctx, r, []modules.Initializer{&testModule{}}, options)
	})
}

func Test_Route_ToModule(t *testing.T) {
	pathBase := "/some-path-base"

	options := modules.Options{
		Address:          "localhost",
		PathBase:         pathBase,
		DatabaseProvider: databaseprovider.FromMemory(),
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
