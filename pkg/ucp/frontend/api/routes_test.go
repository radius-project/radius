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
	"fmt"
	"net/http"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/golang/mock/gomock"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/ucp/dataprovider"
	"github.com/project-radius/radius/pkg/ucp/frontend/modules"
	"github.com/stretchr/testify/require"
)

func Test_Routes(t *testing.T) {
	pathBase := "/some-path-base"
	tests := []struct {
		method       string
		path         string
		name         string
		skipPathBase bool
	}{
		{
			name:         v1.OperationType{Type: OperationTypeKubernetesOpenAPIV2Doc, Method: v1.OperationGet}.String(),
			method:       http.MethodGet,
			path:         "/openapi/v2",
			skipPathBase: true,
		},
		{
			name:   v1.OperationType{Type: OperationTypeKubernetesDiscoveryDoc, Method: v1.OperationGet}.String(),
			method: http.MethodGet,
			path:   "",
		},
		{
			name:   v1.OperationType{Type: OperationTypePlanes, Method: v1.OperationList}.String(),
			method: http.MethodGet,
			path:   "/planes",
		},
		{
			name:   v1.OperationType{Type: OperationTypePlanesByType, Method: v1.OperationList}.String(),
			method: http.MethodGet,
			path:   "/planes/someType",
		},
		{
			name:   v1.OperationType{Type: OperationTypePlanesByType, Method: v1.OperationGet}.String(),
			method: http.MethodGet,
			path:   "/planes/someType/someName",
		},
		{
			name:   v1.OperationType{Type: OperationTypePlanesByType, Method: v1.OperationPut}.String(),
			method: http.MethodPut,
			path:   "/planes/someType/someName",
		},
		{
			name:   v1.OperationType{Type: OperationTypePlanesByType, Method: v1.OperationDelete}.String(),
			method: http.MethodDelete,
			path:   "/planes/someType/someName",
		},
	}

	ctrl := gomock.NewController(t)
	dataProvider := dataprovider.NewMockDataStorageProvider(ctrl)
	dataProvider.EXPECT().GetStorageClient(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()

	options := modules.Options{
		Address:      "localhost",
		PathBase:     pathBase,
		DataProvider: dataProvider,
	}

	router := chi.NewRouter()
	ctx := context.Background()
	err := Register(ctx, router, nil, options)
	require.NoError(t, err)

	for _, test := range tests {
		t.Run(fmt.Sprintf("%s_%s", test.method, test.path), func(t *testing.T) {
			p := pathBase + test.path

			if test.skipPathBase {
				p = test.path
			}

			tctx := chi.NewRouteContext()
			tctx.Reset()

			result := router.Match(tctx, test.method, p)
			require.Truef(t, result, "no route found for %s %s", test.method, p)
		})
	}

	t.Run("all named routes are tested", func(t *testing.T) {
		err := chi.Walk(router, func(method string, route string, handler http.Handler, middlewares ...func(http.Handler) http.Handler) error {
			t.Logf("%s %s", method, route)
			return nil
		})
		require.NoError(t, err)
	})
}
