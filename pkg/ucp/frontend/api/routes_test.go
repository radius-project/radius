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
	"github.com/golang/mock/gomock"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/armrpc/rpctest"
	"github.com/project-radius/radius/pkg/ucp/dataprovider"
	"github.com/project-radius/radius/pkg/ucp/frontend/modules"
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
			OperationType: v1.OperationType{Type: OperationTypePlanesByType, Method: v1.OperationList},
			Method:        http.MethodGet,
			Path:          "/planes/someType",
		},
		{
			OperationType: v1.OperationType{Type: OperationTypePlanesByType, Method: v1.OperationGet},
			Method:        http.MethodGet,
			Path:          "/planes/someType/someName",
		},
		{
			OperationType: v1.OperationType{Type: OperationTypePlanesByType, Method: v1.OperationPut},
			Method:        http.MethodPut,
			Path:          "/planes/someType/someName",
		},
		{
			OperationType: v1.OperationType{Type: OperationTypePlanesByType, Method: v1.OperationDelete},
			Method:        http.MethodDelete,
			Path:          "/planes/someType/someName",
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

	rpctest.AssertRouters(t, tests, pathBase, "", func(ctx context.Context) (chi.Router, error) {
		r := chi.NewRouter()
		return r, Register(ctx, r, nil, options)
	})
}
