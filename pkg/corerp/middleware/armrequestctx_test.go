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

	armRequestCtxTestCases := []struct {
		desc string
		url  string
		code int
		body string
	}{
		{
			"get-env-success",
			"/subscriptions/00001b53-0000-0000-0000-00006235a42c/resourcegroups/radius-test-rg/providers/Applications.Core/environments/env0",
			http.StatusOK,
			"00001b53-0000-0000-0000-00006235a42c",
		},
		{
			"bad-top-query-param",
			"/subscriptions/00001b53-0000-0000-0000-00006235a42c/resourcegroups/radius-test-rg/providers/Applications.Core/environments?top=10000",
			http.StatusBadRequest,
			servicecontext.ErrTopQueryParamOutOfBounds.Error() + "\n",
		},
	}

	for _, tt := range armRequestCtxTestCases {
		t.Run(tt.desc, func(t *testing.T) {
			const testPathBase = "/base"
			w := httptest.NewRecorder()
			r := mux.NewRouter()
			r.Path(testPathBase + "/subscriptions/{subscriptionID}/resourcegroups/{resourceGroup}/providers/{providerName}/{resourceType}/{resourceName}").Methods(http.MethodPut).HandlerFunc(
				func(w http.ResponseWriter, r *http.Request) {
					rpcCtx := servicecontext.ARMRequestContextFromContext(r.Context())
					_, _ = w.Write([]byte(rpcCtx.ResourceID.SubscriptionID))
				})

			handler := ARMRequestCtx(testPathBase)(r)

			testUrl := testPathBase + tt.url

			req, _ := http.NewRequestWithContext(context.Background(), http.MethodPut, testUrl, nil)
			handler.ServeHTTP(w, req)

			assert.Equal(t, tt.body, w.Body.String())
			assert.Equal(t, tt.code, w.Code)
		})
	}
}
