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

package clientv2

import (
	"net/http"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"github.com/stretchr/testify/require"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) Do(req *http.Request) (*http.Response, error) { return f(req) }

func Test_WithForceDelete(t *testing.T) {
	for _, tt := range []struct {
		method    string
		url       string
		wantForce bool
	}{
		{method: http.MethodDelete, url: "http://localhost/test?api-version=2023-10-01-preview", wantForce: true},
		{method: http.MethodDelete, url: "http://localhost/test?api-version=2023-10-01-preview&force=false"},
		{method: http.MethodGet, url: "http://localhost/test?api-version=2023-10-01-preview"},
	} {
		t.Run(tt.method+" "+tt.url, func(t *testing.T) {
			var captured *http.Request
			opts := WithForceDelete(arm.ClientOptions{})
			pipeline := runtime.NewPipeline("test", "v1.0.0", runtime.PipelineOptions{PerCall: opts.PerCallPolicies}, &policy.ClientOptions{
				Transport: transportFunc(func(req *http.Request) (*http.Response, error) {
					captured = req
					return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}, nil
				}),
			})
			req, err := runtime.NewRequest(t.Context(), tt.method, tt.url)
			require.NoError(t, err)
			_, err = pipeline.Do(req)
			require.NoError(t, err)
			require.Equal(t, "2023-10-01-preview", captured.URL.Query().Get("api-version"))
			require.Equal(t, tt.wantForce, captured.URL.Query().Get("force") == "true")
		})
	}
}

func Test_WithForceDelete_KeepsExistingPolicies(t *testing.T) {
	existing := []policy.Policy{&forceDeletePolicy{}}
	opts := WithForceDelete(arm.ClientOptions{ClientOptions: policy.ClientOptions{PerCallPolicies: existing}})
	require.Len(t, opts.PerCallPolicies, 2)
	require.Len(t, existing, 1)
}
