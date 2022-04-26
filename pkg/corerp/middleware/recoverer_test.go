// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/require"
)

func TestRecoverer(t *testing.T) {
	const testPathBase = "/base"
	w := httptest.NewRecorder()
	r := mux.NewRouter()
	r.Path(testPathBase + "/subscriptions/{subscriptionID}/resourcegroups/{resourceGroup}/providers/{providerName}/{resourceType}/{resourceName}").Methods(http.MethodPut).HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			// panic !!!
			panic("panic test")
		})

	handler := Recoverer(r)

	testUrl := testPathBase + "/subscriptions/00001b53-0000-0000-0000-00006235a42c/resourcegroups/radius-test-rg/providers/Applications.Core/environments/env0"

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPut, testUrl, nil)
	handler.ServeHTTP(w, req)

	parsed := w.Body.String()
	require.Equal(t, 500, w.Result().StatusCode)
	require.NotEmpty(t, parsed)
}
