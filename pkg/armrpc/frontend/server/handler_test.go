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
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/armrpc/asyncoperation/statusmanager"
	ctrl "github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	"github.com/radius-project/radius/pkg/armrpc/rest"
	"github.com/radius-project/radius/pkg/armrpc/rpctest"
	"github.com/radius-project/radius/pkg/authz"
	"github.com/radius-project/radius/pkg/components/database/inmemory"
	"github.com/radius-project/radius/pkg/middleware"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
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
	ctrlOpts := ctrl.Options{
		Address:        "localhost:8080",
		ResourceType:   "Applications.Test/testResources",
		DatabaseClient: inmemory.NewClient(),
		StatusManager:  statusmanager.NewMockStatusManager(gomock.NewController(t)),
	}

	p := chi.NewRouter()
	opts := HandlerOptions{
		ParentRouter:      p,
		ResourceType:      "Applications.Test/testResources",
		Method:            http.MethodGet,
		ControllerFactory: func(ctrl.Options) (ctrl.Controller, error) { return nil, nil },
		Middlewares:       chi.Middlewares{middleware.NormalizePath},
	}

	ctx := t.Context()

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
				ResourceType:      "Applications.Test/testResources",
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
				ResourceType:      "Applications.Test/testResources",
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
				ResourceType:      "Applications.Test/testResources",
				OperationType:     &v1.OperationType{Type: "Applications.Test/testResources", Method: "GET"},
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
				ResourceType:      "Applications.Test/testResources",
				OperationType:     &v1.OperationType{Type: "Applications.Test/testResources", Method: "PROXY"},
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

	ctrlOpts := ctrl.Options{
		Address:        "localhost:8080",
		DatabaseClient: inmemory.NewClient(),
		StatusManager:  statusmanager.NewMockStatusManager(gomock.NewController(t)),
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := t.Context()

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
		url:    "/resourcegroups/testrg/providers/applications.core/environments?api-version=2023-10-01-preview",
		method: http.MethodGet,
	}

	req := httptest.NewRequest(handlerTest.method, handlerTest.url, nil)
	responseWriter := httptest.NewRecorder()
	err := &v1.ErrModelConversion{PropertyName: "namespace", ValidValue: "63 characters or less"}
	HandleError(t.Context(), responseWriter, req, err)

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
		url:    "/resourcegroups/testrg/providers/applications.core/environments?api-version=2023-10-01-preview",
		method: http.MethodGet,
	}

	req := httptest.NewRequest(handlerTest.method, handlerTest.url, nil)
	responseWriter := httptest.NewRecorder()
	HandleError(t.Context(), responseWriter, req, v1.ErrInvalidModelConversion)

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
		url:    "/resourcegroups/testrg/providers/applications.core/environments?api-version=2023-10-01-preview",
		method: http.MethodGet,
	}

	req := httptest.NewRequest(handlerTest.method, handlerTest.url, nil)
	responseWriter := httptest.NewRecorder()
	err := errors.New("Internal error")
	HandleError(t.Context(), responseWriter, req, err)

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
	req = req.WithContext(v1.WithARMRequestContext(t.Context(), rCtx))

	handler.ServeHTTP(w, req)

	require.Equal(t, expectedType.String(), rCtx.OperationType.String())
}

type deniedController struct {
	ctrl.Operation[*rpctest.TestResourceDataModel, rpctest.TestResourceDataModel]
	err error
}

func (c *deniedController) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (rest.Response, error) {
	return nil, c.err
}

func Test_HandlerForController_DeniedError(t *testing.T) {
	const identity = "alice@example.com"
	const target = "/planes/radius/local/resourceGroups/rg/providers/Applications.Core/containers/c"

	tests := []struct {
		name           string
		denied         *authz.DeniedError
		wantStatus     int
		wantCode       string
		wantMessage    string
		wantTargetBody string
	}{
		{
			name: "converts to authorization response",
			denied: &authz.DeniedError{
				Code:   v1.CodeOperationNotAssigned,
				Action: "Applications.Core/containers/write",
				Target: target,
			},
			wantStatus:     http.StatusForbidden,
			wantCode:       v1.CodeOperationNotAssigned,
			wantMessage:    "Authorization denied for action 'Applications.Core/containers/write' on target '" + target + "'",
			wantTargetBody: target,
		},
		{
			name: "uses status for code",
			denied: &authz.DeniedError{
				Code:   v1.CodeAuthorizationUnavailable,
				Action: "read",
				Target: target,
			},
			wantStatus:     http.StatusServiceUnavailable,
			wantCode:       v1.CodeAuthorizationUnavailable,
			wantMessage:    "Authorization could not be checked for action 'read' on target '" + target + "'. Try again later.",
			wantTargetBody: target,
		},
		{
			name:        "missing request context falls back to generic denial",
			denied:      &authz.DeniedError{Code: v1.CodeGrantScopeExceeded},
			wantStatus:  http.StatusForbidden,
			wantCode:    v1.CodeGrantScopeExceeded,
			wantMessage: "Authorization denied",
		},
		{
			name:        "unavailable without request context falls back to retry message",
			denied:      &authz.DeniedError{Code: v1.CodeAuthorizationUnavailable},
			wantStatus:  http.StatusServiceUnavailable,
			wantCode:    v1.CodeAuthorizationUnavailable,
			wantMessage: "Authorization could not be checked. Try again later.",
		},
		{
			name:        "empty code defaults to AuthorizationFailed",
			denied:      &authz.DeniedError{},
			wantStatus:  http.StatusForbidden,
			wantCode:    v1.CodeAuthorizationFailed,
			wantMessage: "Authorization denied",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			controller := &deniedController{err: fmt.Errorf("checking access for %s: %w", identity, tt.denied)}
			handler := HandlerForController(controller, v1.OperationType{Type: "Applications.Core/containers", Method: http.MethodPut})

			req := httptest.NewRequest(http.MethodPut, "/", nil)
			req = req.WithContext(v1.WithARMRequestContext(t.Context(), &v1.ARMRequestContext{}))
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)

			require.Equal(t, tt.wantStatus, w.Code)
			require.NotContains(t, w.Body.String(), identity)
			require.NotContains(t, w.Body.String(), "checking access")

			armerr := v1.ErrorResponse{}
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &armerr))
			require.Equal(t, tt.wantCode, armerr.Error.Code)
			require.Equal(t, tt.wantMessage, armerr.Error.Message)
			require.Equal(t, tt.wantTargetBody, armerr.Error.Target)
		})
	}
}
