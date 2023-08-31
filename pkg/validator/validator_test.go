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

package validator

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/go-openapi/runtime/middleware"
	"github.com/stretchr/testify/require"

	"github.com/radius-project/radius/swagger"
)

func Test_FindParam(t *testing.T) {
	l, err := LoadSpec(context.Background(), "applications.core", swagger.SpecFiles, []string{"/subscriptions/{subscriptionID}/resourceGroups/{rgName}"}, "rootScope")
	require.NoError(t, err)
	v, ok := l.GetValidator("applications.core/environments", "2022-03-15-privatepreview")
	require.True(t, ok)
	validator := v.(*validator)

	w := httptest.NewRecorder()
	r := chi.NewRouter()
	req, err := http.NewRequest(http.MethodPut, armResourceGroupScopedResourceURL, nil)
	require.NoError(t, err)

	r.MethodFunc(http.MethodPut, "/subscriptions/{subscriptionID}/resourceGroups/{rgName}"+environmentResourceRoute, func(w http.ResponseWriter, r *http.Request) {
		param, err := validator.findParam(r)
		require.NoError(t, err)
		require.NotNil(t, param)
		require.Equal(t, 1, len(validator.paramCache))

		w.WriteHeader(http.StatusAccepted)
	})

	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusAccepted, w.Result().StatusCode)
}

func Test_ToRouteParams(t *testing.T) {
	v := validator{
		rootScopeParam: "rootScope",
	}

	t.Run("non-match", func(t *testing.T) {
		// double slash is an invalid resource ID '//test'
		req, err := http.NewRequest("", "http://radius//test", nil)
		require.NoError(t, err)
		ps := v.toRouteParams(req)
		require.Empty(t, ps)
	})

	t.Run("azure subscription path", func(t *testing.T) {
		req, err := http.NewRequest("", "http://radius/subscriptions/00000000-0000-0000-0000-000000000000/providers/applications.core/environments?api-version=2022-03-15-privatepreview", nil)
		require.NoError(t, err)
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("subscriptionID", "00000000-0000-0000-0000-000000000000")
		rctx.URLParams.Add("*", "/providers/applications.core/environments")
		ps := v.toRouteParams(req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx)))

		expected := middleware.RouteParams{
			{Name: "rootScope", Value: "/subscriptions/00000000-0000-0000-0000-000000000000"},
			{Name: "api-version", Value: "2022-03-15-privatepreview"},
			{Name: "subscriptionID", Value: "00000000-0000-0000-0000-000000000000"},
		}
		require.Equal(t, expected, ps)
	})

	t.Run("azure resource-group path", func(t *testing.T) {
		req, err := http.NewRequest("", "http://radius/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/applications.core/environments/env0?api-version=2022-03-15-privatepreview", nil)
		require.NoError(t, err)
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("subscriptionID", "00000000-0000-0000-0000-000000000000")
		rctx.URLParams.Add("*", "/providers/applications.core/environments/env0")
		rctx.URLParams.Add("environmentName", "env0")
		ps := v.toRouteParams(req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx)))

		expected := middleware.RouteParams{
			{Name: "rootScope", Value: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg"},
			{Name: "api-version", Value: "2022-03-15-privatepreview"},
			{Name: "subscriptionID", Value: "00000000-0000-0000-0000-000000000000"},
			{Name: "environmentName", Value: "env0"},
		}
		require.Equal(t, expected, ps)
	})

	t.Run("ucp plane path", func(t *testing.T) {
		req, err := http.NewRequest("", "http://radius/planes/radius/local/providers/applications.core/environments?api-version=2022-03-15-privatepreview", nil)
		require.NoError(t, err)
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("planeType", "radius")
		rctx.URLParams.Add("planeName", "local")
		rctx.URLParams.Add("*", "/providers/applications.core/environments")
		ps := v.toRouteParams(req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx)))

		expected := middleware.RouteParams{
			{Name: "rootScope", Value: "/planes/radius/local"},
			{Name: "api-version", Value: "2022-03-15-privatepreview"},
			{Name: "planeType", Value: "radius"},
			{Name: "planeName", Value: "local"},
		}
		require.Equal(t, expected, ps)
	})

	t.Run("ucp resource-group path", func(t *testing.T) {
		req, err := http.NewRequest("", "http://radius/planes/radius/local/resourceGroups/radius-test-rg/providers/applications.core/environments/env0?api-version=2022-03-15-privatepreview", nil)
		require.NoError(t, err)
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("planeType", "radius")
		rctx.URLParams.Add("planeName", "local")
		rctx.URLParams.Add("*", "/providers/applications.core/environments")
		rctx.URLParams.Add("environmentName", "env0")
		ps := v.toRouteParams(req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx)))

		expected := middleware.RouteParams{
			{Name: "rootScope", Value: "/planes/radius/local/resourceGroups/radius-test-rg"},
			{Name: "api-version", Value: "2022-03-15-privatepreview"},
			{Name: "planeType", Value: "radius"},
			{Name: "planeName", Value: "local"},
			{Name: "environmentName", Value: "env0"},
		}
		require.Equal(t, expected, ps)
	})
}
