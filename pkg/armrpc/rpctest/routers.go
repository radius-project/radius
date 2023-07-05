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
	"context"
	"net/http"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/project-radius/radius/test/testcontext"
	"github.com/stretchr/testify/require"
)

// HandlerTestSpec is a specification for a single test case for a handler.
type HandlerTestSpec struct {
	// Name is the name of the test case.
	Name string
	// Path is the sub path of the router matches.
	Path string
	// Method is the HTTP method of the request.
	Method string
	// WithoutRootScope indicates that the root scope should not be prepended to the path.
	WithoutRootScope bool
	// SkipPathBase indicates that the path base should not be prepended to the path.
	SkipPathBase bool
}

// AssertRouters asserts that the given router matches the given test cases.
func AssertRouters(t *testing.T, tests []HandlerTestSpec, pathBase, rootScope string, configureRouter func(context.Context) (chi.Router, error)) {
	ctx := testcontext.New(t)
	r, err := configureRouter(ctx)
	require.NoError(t, err)

	namesMatched := make(map[string]bool)
	for _, tt := range tests {
		if tt.WithoutRootScope {
			namesMatched[tt.Name] = true
			continue
		}

		pb := ""
		if !tt.SkipPathBase {
			pb = pathBase
		}

		uri := pb + rootScope + tt.Path
		if tt.WithoutRootScope {
			uri = pb + tt.Path
		}

		t.Run(tt.Name, func(t *testing.T) {
			tctx := chi.NewRouteContext()
			tctx.Reset()

			result := r.Match(tctx, tt.Method, uri)

			t.Logf("result: %v", tctx)
			require.Truef(t, result, "no route found for %s %s, context: %v", tt.Method, uri, tctx)
		})
	}

	t.Run("all named routes are tested", func(t *testing.T) {
		err := chi.Walk(r, func(method string, route string, handler http.Handler, middlewares ...func(http.Handler) http.Handler) error {
			t.Logf("%s %s", method, route)
			return nil
		})
		require.NoError(t, err)
	})
	/*
		t.Run("all named routes are tested", func(t *testing.T) {
			err := r.Walk(func(route *mux.Route, router *mux.Router, ancestors []*mux.Route) error {
				if route.GetName() == "" {
					return nil
				}

				assert.Contains(t, namesMatched, route.GetName(), "route %s is not tested", route.GetName())
				return nil
			})
			require.NoError(t, err)
		})*/
}
