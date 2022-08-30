// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package servicecontext

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/stretchr/testify/assert"
)

func TestARMRequestCtx(t *testing.T) {

	outOfBoundsTopParamError := v1.ErrorDetails{
		Code:    v1.CodeInvalid,
		Message: fmt.Sprintf("unexpected error: %v", ErrTopQueryParamOutOfBounds),
	}

	invalidTopParamError := v1.ErrorDetails{
		Code:    v1.CodeInvalid,
		Message: "unexpected error: strconv.Atoi: parsing \"xyz\": invalid syntax",
	}

	armRequestCtxTestCases := []struct {
		desc string
		url  string
		code int
		ok   bool
		body string
		err  v1.ErrorDetails
	}{
		{
			"get-env-success",
			"/subscriptions/00001b53-0000-0000-0000-00006235a42c/resourcegroups/radius-test-rg/providers/Applications.Core/environments/env0",
			http.StatusOK,
			true,
			"00001b53-0000-0000-0000-00006235a42c",
			v1.ErrorDetails{},
		},
		{
			"out-of-bounds-top-query-param",
			"/subscriptions/00001b53-0000-0000-0000-00006235a42c/resourcegroups/radius-test-rg/providers/Applications.Core/environments?top=10000",
			http.StatusBadRequest,
			false,
			"",
			outOfBoundsTopParamError,
		},
		{
			"bad-top-query-param",
			"/subscriptions/00001b53-0000-0000-0000-00006235a42c/resourcegroups/radius-test-rg/providers/Applications.Core/environments?top=xyz",
			http.StatusBadRequest,
			false,
			"",
			invalidTopParamError,
		},
	}

	for _, tt := range armRequestCtxTestCases {
		t.Run(tt.desc, func(t *testing.T) {
			const testPathBase = "/base"
			w := httptest.NewRecorder()
			r := mux.NewRouter()
			r.Path(testPathBase + "/subscriptions/{subscriptionID}/resourcegroups/{resourceGroup}/providers/{providerName}/{resourceType}/{resourceName}").Methods(http.MethodPut).HandlerFunc(
				func(w http.ResponseWriter, r *http.Request) {
					rpcCtx := ARMRequestContextFromContext(r.Context())
					_, _ = w.Write([]byte(rpcCtx.ResourceID.ScopeSegments()[0].Name))
				})

			handler := ARMRequestCtx(testPathBase)(r)

			testUrl := testPathBase + tt.url

			req, _ := http.NewRequestWithContext(context.Background(), http.MethodPut, testUrl, nil)
			handler.ServeHTTP(w, req)

			assert.Equal(t, tt.code, w.Code)

			if !tt.ok {
				errResp := &v1.ErrorResponse{}
				_ = json.Unmarshal(w.Body.Bytes(), errResp)
				assert.Equal(t, tt.err, errResp.Error)
			} else {
				assert.Equal(t, tt.body, w.Body.String())
			}
		})
	}
}
