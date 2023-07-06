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
	"github.com/stretchr/testify/require"
)

func TestARMRequestCtx(t *testing.T) {

	outOfBoundsTopParamError := v1.ErrorDetails{
		Code:    v1.CodeInvalid,
		Message: fmt.Sprintf("unexpected error: %v", v1.ErrTopQueryParamOutOfBounds),
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
					rpcCtx := v1.ARMRequestContextFromContext(r.Context())
					_, _ = w.Write([]byte(rpcCtx.ResourceID.ScopeSegments()[0].Name))
				})

			handler := ARMRequestCtx(testPathBase, v1.LocationGlobal)(r)

			testUrl := testPathBase + tt.url

			req, err := http.NewRequestWithContext(context.Background(), http.MethodPut, testUrl, nil)
			require.NoError(t, err)
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

func Test_ARMRequestCtx_with_empty_location_causes_panic(t *testing.T) {
	require.Panics(t, func() {
		ARMRequestCtx("/some/base/path", "") // Empty location
	})
}
