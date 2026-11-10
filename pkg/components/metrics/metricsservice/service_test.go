/*
Copyright 2026 The Radius Authors.

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

package metricsservice

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/radius-project/radius/pkg/authz/mtls"
	"github.com/stretchr/testify/require"
)

func TestMetricsProofRouting(t *testing.T) {
	t.Parallel()
	metrics := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	for _, enabled := range []bool{false, true} {
		service := &Service{Options: &Options{Prometheus: &PrometheusOptions{Path: "/metrics"}}}
		if enabled {
			// POST exercises routing without attempting to read TLS material.
			service.StartupProof = &mtls.StartupProof{}
		}

		handler, err := service.handler(metrics)
		require.NoError(t, err)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/metrics", nil))
		require.Equal(t, http.StatusNoContent, response.Code)
		response = httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, mtls.StartupProofPath, nil))
		if enabled {
			require.Equal(t, http.StatusMethodNotAllowed, response.Code)
			require.Equal(t, "no-store", response.Header().Get("Cache-Control"))
		} else {
			require.Equal(t, http.StatusNotFound, response.Code)
		}
		service.Options.Prometheus.Path = mtls.StartupProofPath
		_, err = service.handler(metrics)
		if enabled {
			require.ErrorContains(t, err, "conflicts")
		} else {
			require.NoError(t, err)
		}
	}

}

func TestMetricsAggregationProofRouting(t *testing.T) {
	t.Parallel()
	for _, component := range []bool{false, true} {
		service := &Service{Options: &Options{Prometheus: &PrometheusOptions{Path: "/metrics"}},
			AggregationProof: &mtls.AggregationProof{}}
		if component {
			service.StartupProof = &mtls.StartupProof{}
		}
		handler, err := service.handler(http.NotFoundHandler())
		require.NoError(t, err)
		for _, path := range []string{mtls.AggregationProofPath, mtls.StartupProofPath} {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, path, nil))
			if path == mtls.AggregationProofPath || component {
				require.Equal(t, http.StatusMethodNotAllowed, response.Code)
				require.Equal(t, "no-store", response.Header().Get("Cache-Control"))
			} else {
				require.Equal(t, http.StatusNotFound, response.Code)
			}
		}
		service.Options.Prometheus.Path = mtls.AggregationProofPath
		_, err = service.handler(http.NotFoundHandler())
		require.ErrorContains(t, err, "conflicts")
	}
}
