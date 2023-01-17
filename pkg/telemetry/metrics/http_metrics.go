// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package metrics

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/project-radius/radius/pkg/telemetry/metrics/provider"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.opentelemetry.io/otel/metric"
)

type httpMetrics struct {
	requestCounter prometheus.Counter

	meter metric.Meter
}

func NewHTTPMetrics(providerName string) *httpMetrics {
	pme, err := provider.GetPrometheusExporter()
	if err != nil {
		panic(err)
	}

	meter := pme.MeterProvider.Meter("radius")

	requestCounter := promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "radius",
		Subsystem: strings.Replace(strings.ToLower(providerName), ".", "_", -1),
		Name:      "request_count_total",
		Help:      "The total number of requests received by " + providerName,
	})

	return &httpMetrics{
		requestCounter: requestCounter,
		meter:          meter,
	}
}

func (h *httpMetrics) HTTPMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Println("HTTPMiddleware")
			h.requestCounter.Inc()
			next.ServeHTTP(w, r)
		})
	}
}
