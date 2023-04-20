// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package validator

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/project-radius/radius/swagger"
	"github.com/stretchr/testify/require"
)

func TestFindParam(t *testing.T) {
	l, err := LoadSpec(context.Background(), "applications.core", swagger.SpecFiles, "/{rootScope:.*}", "rootScope")
	require.NoError(t, err)
	v, ok := l.GetValidator("applications.core/environments", "2023-04-15-preview")
	require.True(t, ok)
	validator := v.(*validator)

	w := httptest.NewRecorder()
	r := mux.NewRouter()
	req, _ := http.NewRequest(http.MethodPut, armIDUrl, nil)

	router := r.PathPrefix("/{rootScope:.*}").Subrouter()
	router.Path(envRoute).Methods(http.MethodPut).HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		param, err := validator.findParam(r)
		require.NoError(t, err)
		require.NotNil(t, param)
		require.Equal(t, 1, len(validator.paramCache))

		w.WriteHeader(http.StatusAccepted)
	})

	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusAccepted, w.Result().StatusCode)
}

func TestToRouteParams(t *testing.T) {
	v := validator{
		rootScopeParam: "rootScope",
	}
	req, _ := http.NewRequest("", "http://radius/test", nil)
	ps := v.toRouteParams(req)
	require.Equal(t, 0, len(ps))

	req, _ = http.NewRequest("", "http://radius/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/applications.core/environments/env0?api-version=2023-04-15-preview", nil)
	ps = v.toRouteParams(req)
	require.Equal(t, 2, len(ps))
	require.Equal(t, "rootScope", ps[0].Name)
	require.Equal(t, "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg", ps[0].Value)
	require.Equal(t, "api-version", ps[1].Name)
	require.Equal(t, "2023-04-15-preview", ps[1].Value)
}
