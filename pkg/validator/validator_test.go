// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package validator

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/project-radius/radius/swagger"
	"github.com/stretchr/testify/require"
)

var envRequestBody = `{
	"location": "West US",
	"properties": {
		"compute": {
			"kind": "kubernetes",
			"resourceId": "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup/providers/Microsoft.ContainerService/managedClusters/radiusTestCluster"
		}
	
}
`

func TestValidateRequest(t *testing.T) {
	l := NewLoader("applications.core", swagger.SpecFiles)
	err := l.LoadSpec()
	require.NoError(t, err)

	w := httptest.NewRecorder()
	r := mux.NewRouter()
	r.Path("/{rootScope:.*}/providers/applications.core/environments/{environmentName}").
		Queries("api-version", "{api-version}").
		Methods(http.MethodPut).
		HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				v, ok := l.GetValidator("environments", "2022-03-15-privatepreview")
				require.True(t, ok)
				errs := v.ValidateRequest(r)
				require.Equal(t, 0, len(errs))
				body, _ := json.Marshal(errs)
				_, _ = w.Write(body)
			})

	req, _ := http.NewRequestWithContext(
		context.Background(),
		http.MethodPut,
		"http://localhost:8080/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/applications.core/environments/env0?api-version=2022-03-15-privatepreview",
		bytes.NewBufferString(envRequestBody))

	r.ServeHTTP(w, req)
}
