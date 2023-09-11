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

package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/golang/mock/gomock"
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	ctrl "github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	"github.com/radius-project/radius/pkg/armrpc/rest"
	"github.com/radius-project/radius/pkg/armrpc/rpctest"
	"github.com/radius-project/radius/pkg/middleware"
	"github.com/radius-project/radius/pkg/ucp/dataprovider"
	"github.com/radius-project/radius/test/testcontext"
	"github.com/stretchr/testify/require"
)

func Test_NewSubrouter(t *testing.T) {
	tests := []struct {
		name        string
		path        string
		middlewares []func(http.Handler) http.Handler
	}{
		{
			name:        "without middleware",
			path:        "/some-path-base",
			middlewares: chi.Middlewares{},
		},
		{
			name:        "with middleware",
			path:        "/some-path-base",
			middlewares: chi.Middlewares{middleware.NormalizePath},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := chi.NewRouter()
			r := NewSubrouter(p, tc.path, tc.middlewares...)
			r.Get(tc.path, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})
			require.Equal(t, len(tc.middlewares), len(r.Middlewares()))
			rctx := chi.NewRouteContext()
			rctx.Reset()
			ok := r.Match(rctx, "GET", tc.path)
			require.True(t, ok)
		})
	}
}

func Test_RegisterHandler_DeplicatedRoutes(t *testing.T) {
	mctrl := gomock.NewController(t)

	mockSP := dataprovider.NewMockDataStorageProvider(mctrl)
	mockSP.EXPECT().GetStorageClient(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	ctrlOpts := ctrl.Options{
		DataProvider: mockSP,
	}

	p := chi.NewRouter()
	opts := HandlerOptions{
		ParentRouter:      p,
		ResourceType:      "Applications.Test",
		Method:            http.MethodGet,
		ControllerFactory: func(ctrl.Options) (ctrl.Controller, error) { return nil, nil },
		Middlewares:       chi.Middlewares{middleware.NormalizePath},
	}

	ctx := testcontext.New(t)

	err := RegisterHandler(ctx, opts, ctrlOpts)
	require.NoError(t, err)

	err = RegisterHandler(ctx, opts, ctrlOpts)
	require.NoError(t, err, "should not return error if the same route is registered twice")
}

func Test_RegisterHandler(t *testing.T) {
	p := chi.NewRouter()
	tests := []struct {
		name        string
		opts        HandlerOptions
		validMethod []string
		validRoute  []string
		err         error
	}{
		{
			name: "valid route with resource type and method",
			opts: HandlerOptions{
				ParentRouter:      p,
				ResourceType:      "Applications.Test",
				Method:            http.MethodGet,
				ControllerFactory: func(ctrl.Options) (ctrl.Controller, error) { return nil, nil },
				Middlewares:       chi.Middlewares{middleware.NormalizePath},
			},
			validMethod: []string{http.MethodGet},
			validRoute:  []string{"/"},
		}, {
			name: "valid route with with path",
			opts: HandlerOptions{
				ParentRouter:      p,
				Path:              "/test",
				ResourceType:      "Applications.Test",
				Method:            http.MethodGet,
				ControllerFactory: func(ctrl.Options) (ctrl.Controller, error) { return nil, nil },
				Middlewares:       chi.Middlewares{middleware.NormalizePath},
			},
			validMethod: []string{http.MethodGet},
			validRoute:  []string{"/test"},
		},
		{
			name: "valid route with operation type",
			opts: HandlerOptions{
				ParentRouter:      p,
				OperationType:     &v1.OperationType{Type: "Applications.Test", Method: "GET"},
				ControllerFactory: func(ctrl.Options) (ctrl.Controller, error) { return nil, nil },
				Middlewares:       chi.Middlewares{middleware.NormalizePath},
			},
			validMethod: []string{http.MethodGet},
			validRoute:  []string{"/"},
		},
		{
			name: "catch-call routes with opertion type",
			opts: HandlerOptions{
				ParentRouter:      p,
				Path:              "/*",
				OperationType:     &v1.OperationType{Type: "Applications.Test", Method: "PROXY"},
				ControllerFactory: func(ctrl.Options) (ctrl.Controller, error) { return nil, nil },
			},
			validMethod: []string{http.MethodGet, http.MethodPost},
			validRoute:  []string{"/any", "/"},
		},
		{
			name: "invalid operation type by neither setting operationtype nor setting resource type and method",
			opts: HandlerOptions{
				ParentRouter:      p,
				Path:              "/",
				ControllerFactory: func(ctrl.Options) (ctrl.Controller, error) { return nil, nil },
				Middlewares:       chi.Middlewares{middleware.NormalizePath},
			},
			err: ErrInvalidOperationTypeOption,
		},
	}

	mctrl := gomock.NewController(t)

	mockSP := dataprovider.NewMockDataStorageProvider(mctrl)
	mockSP.EXPECT().GetStorageClient(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	ctrlOpts := ctrl.Options{
		DataProvider: mockSP,
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := testcontext.New(t)

			err := RegisterHandler(ctx, tc.opts, ctrlOpts)
			if tc.err != nil {
				require.ErrorContains(t, err, tc.err.Error())
				return
			}

			require.NoError(t, err)

			rctx := chi.NewRouteContext()
			rctx.Reset()

			for i, method := range tc.validMethod {
				ok := p.Match(rctx, method, tc.validRoute[i])
				require.True(t, ok)
			}
		})
	}
}

func Test_HandlerErrModelConversion(t *testing.T) {
	var handlerTest = struct {
		url    string
		method string
	}{
		url:    "/resourcegroups/testrg/providers/applications.core/environments?api-version=2022-03-15-privatepreview",
		method: http.MethodGet,
	}

	req := httptest.NewRequest(handlerTest.method, handlerTest.url, nil)
	responseWriter := httptest.NewRecorder()
	err := &v1.ErrModelConversion{PropertyName: "namespace", ValidValue: "63 characters or less"}
	HandleError(context.Background(), responseWriter, req, err)

	bodyBytes, e := io.ReadAll(responseWriter.Body)
	require.NoError(t, e)
	armerr := v1.ErrorResponse{}
	e = json.Unmarshal(bodyBytes, &armerr)
	require.NoError(t, e)
	require.Equal(t, v1.CodeHTTPRequestPayloadAPISpecValidationFailed, armerr.Error.Code)
	require.Equal(t, armerr.Error.Message, "namespace must be 63 characters or less.")
}

func Test_HandlerErrInvalidModelConversion(t *testing.T) {
	var handlerTest = struct {
		url    string
		method string
	}{
		url:    "/resourcegroups/testrg/providers/applications.core/environments?api-version=2022-03-15-privatepreview",
		method: http.MethodGet,
	}

	req := httptest.NewRequest(handlerTest.method, handlerTest.url, nil)
	responseWriter := httptest.NewRecorder()
	HandleError(context.Background(), responseWriter, req, v1.ErrInvalidModelConversion)

	bodyBytes, e := io.ReadAll(responseWriter.Body)
	require.NoError(t, e)
	armerr := v1.ErrorResponse{}
	e = json.Unmarshal(bodyBytes, &armerr)
	require.NoError(t, e)
	require.Equal(t, v1.CodeHTTPRequestPayloadAPISpecValidationFailed, armerr.Error.Code)
	require.Equal(t, armerr.Error.Message, "invalid model conversion")
}

func Test_HandlerErrInternal(t *testing.T) {
	var handlerTest = struct {
		url    string
		method string
	}{
		url:    "/resourcegroups/testrg/providers/applications.core/environments?api-version=2022-03-15-privatepreview",
		method: http.MethodGet,
	}

	req := httptest.NewRequest(handlerTest.method, handlerTest.url, nil)
	responseWriter := httptest.NewRecorder()
	err := errors.New("Internal error")
	HandleError(context.Background(), responseWriter, req, err)

	bodyBytes, e := io.ReadAll(responseWriter.Body)
	require.NoError(t, e)
	armerr := v1.ErrorResponse{}
	e = json.Unmarshal(bodyBytes, &armerr)
	require.NoError(t, e)
	require.Equal(t, v1.CodeInternal, armerr.Error.Code)
	require.Equal(t, armerr.Error.Message, "Internal error")
}

type testAPIController struct {
	ctrl.Operation[*rpctest.TestResourceDataModel, rpctest.TestResourceDataModel]
}

func (e *testAPIController) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (rest.Response, error) {
	return nil, nil
}

func Test_HandlerForController_OperationType(t *testing.T) {
	expectedType := v1.OperationType{Type: "Applications.Compute/virtualMachines", Method: "GET"}

	handler := HandlerForController(&testAPIController{}, expectedType)
	w := httptest.NewRecorder()

	req, err := http.NewRequest(http.MethodGet, "", bytes.NewBuffer([]byte{}))
	require.NoError(t, err)

	rCtx := &v1.ARMRequestContext{}
	req = req.WithContext(v1.WithARMRequestContext(context.Background(), rCtx))

	handler.ServeHTTP(w, req)

	require.Equal(t, expectedType.String(), rCtx.OperationType.String())
}
