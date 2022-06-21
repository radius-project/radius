// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package validator

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/project-radius/radius/swagger"
	"github.com/stretchr/testify/require"
)

func TestFindParam(t *testing.T) {
	l := NewLoader("applications.core", swagger.SpecFiles)
	err := l.LoadSpec()
	require.NoError(t, err)
	v, ok := l.GetValidator("applications.core/environments", "2022-03-15-privatepreview")
	require.True(t, ok)
	validator := v.(*validator)

	w := httptest.NewRecorder()
	r := mux.NewRouter()
	req, _ := http.NewRequest(http.MethodPut, baseUrl, nil)
	r.Path(envRoute).Methods(http.MethodPut).HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

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
	v := validator{}
	req, _ := http.NewRequest("", "http://radius/test", nil)
	ps := v.toRouteParams(req)
	require.Equal(t, 0, len(ps))

	req, _ = http.NewRequest("", "http://radius/test?api-version=2022-03-15-privatepreview", nil)
	ps = v.toRouteParams(req)
	require.Equal(t, 1, len(ps))
}
