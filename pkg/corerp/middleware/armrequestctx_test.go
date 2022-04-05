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
	"github.com/project-radius/radius/pkg/corerp/servicecontext"
	"github.com/stretchr/testify/assert"
)

func TestARMRequestCtx(t *testing.T) {
	const testPrefix = "/prefix"
	w := httptest.NewRecorder()
	r := mux.NewRouter()
	r.Path(testPrefix + "/subscriptions/{subscriptionID}/resourcegroups/{resourceGroup}/providers/{providerName}/{resourceType}/{resourceName}").Methods(http.MethodPut).HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			rpcCtx := servicecontext.ARMRequestContextFromContext(r.Context())

			_, _ = w.Write([]byte(rpcCtx.ResourceID.SubscriptionID))
		})

	handler := ARMRequestCtx(testPrefix)(r)

	testUrl := testPrefix + "/subscriptions/00001b53-0000-0000-0000-00006235a42c/resourcegroups/radius-test-rg/providers/Applications.Core/environments/env0"

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPut, testUrl, nil)
	handler.ServeHTTP(w, req)

	parsed := w.Body.String()
	assert.Equal(t, "00001b53-0000-0000-0000-00006235a42c", parsed)
}
