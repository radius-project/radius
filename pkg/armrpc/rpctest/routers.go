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

package rpctest

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/test/testcontext"
	"github.com/stretchr/testify/require"
)

// HandlerTestSpec is a specification for a single test case for a handler.
type HandlerTestSpec struct {
	// OperationType is the operation type of the request.
	OperationType v1.OperationType
	// Path is the sub path of the router matches.
	Path string
	// Method is the HTTP method of the request.
	Method string
	// WithoutRootScope indicates that the root scope should not be prepended to the path.
	WithoutRootScope bool
	// SkipPathBase indicates that the path base should not be prepended to the path.
	SkipPathBase bool
	// SkipOperationTypeValidation indicates that the operation type should not be validated.
	SkipOperationTypeValidation bool
}

// AssertRouters asserts that the given router matches the given test cases.
func AssertRouters(t *testing.T, tests []HandlerTestSpec, pathBase, rootScope string, configureRouter func(context.Context) (chi.Router, error)) {
	ctx := testcontext.New(t)
	r, err := configureRouter(ctx)
	require.NoError(t, err)

	t.Log("Available routes:")
	err = chi.Walk(r, func(method string, route string, handler http.Handler, middlewares ...func(http.Handler) http.Handler) error {
		t.Logf("Method: %s, Path: %s, Middlewares: %+v", method, route, middlewares)
		return nil
	})
	require.NoError(t, err)

	for _, tt := range tests {
		pb := ""
		if !tt.SkipPathBase {
			pb = pathBase
		}

		uri := pb + rootScope + tt.Path
		if tt.WithoutRootScope {
			uri = pb + tt.Path
		}

		t.Run(tt.Method+"|"+tt.Path, func(t *testing.T) {
			tctx := chi.NewRouteContext()
			tctx.Reset()

			matched := r.Match(tctx, tt.Method, uri)
			require.True(t, matched)
			found := "Found"
			if !matched {
				found = "not found"
			}

			t.Logf("%s for %s %s, matched routes: %v", found, tt.Method, uri, strings.Join(tctx.RoutePatterns, "|"))

			if tt.SkipOperationTypeValidation {
				return
			}
		})
	}
}

// AssertRequests asserts that the restful APIs matches the routes and its operation type matches the given test cases.
// This is working only for test controllers. If you want to validate the routes for the real controllers, use AssertRouters.
func AssertRequests(t *testing.T, tests []HandlerTestSpec, pathBase, rootScope string, configureRouter func(context.Context) (chi.Router, error)) {
	ctx := testcontext.New(t)
	r, err := configureRouter(ctx)
	require.NoError(t, err)

	t.Log("Available routes:")
	err = chi.Walk(r, func(method string, route string, handler http.Handler, middlewares ...func(http.Handler) http.Handler) error {
		t.Logf("Method: %s, Path: %s, Middlewares: %+v", method, route, middlewares)
		return nil
	})
	require.NoError(t, err)

	for _, tt := range tests {
		pb := ""
		if !tt.SkipPathBase {
			pb = pathBase
		}

		uri := pb + rootScope + tt.Path
		if tt.WithoutRootScope {
			uri = pb + tt.Path
		}

		t.Run(tt.Method+"|"+tt.Path, func(t *testing.T) {
			w := httptest.NewRecorder()

			// It will not validate body.
			req, err := http.NewRequest(tt.Method, uri, bytes.NewBuffer([]byte{}))
			require.NoError(t, err)

			rCtx := &v1.ARMRequestContext{}
			req = req.WithContext(v1.WithARMRequestContext(context.Background(), rCtx))

			r.ServeHTTP(w, req)
			require.NotEqual(t, 404, w.Result().StatusCode)
			require.Equal(t, tt.OperationType.String(), rCtx.OperationType.String(), "operation type not found: %s %s %s", uri, tt.Method, rCtx.OperationType.String())
		})
	}
}
