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

package server

import (
	"github.com/radius-project/radius/pkg/authz/mtls"
	"github.com/radius-project/radius/pkg/components/hosting"
	"github.com/radius-project/radius/pkg/components/metrics/metricsservice"
	"github.com/radius-project/radius/pkg/components/profiler/profilerservice"
	"github.com/radius-project/radius/pkg/components/trace/traceservice"
	"github.com/radius-project/radius/pkg/dynamicrp"
	"github.com/radius-project/radius/pkg/dynamicrp/backend"
	"github.com/radius-project/radius/pkg/dynamicrp/frontend"
)

// NewServer initializes a host for UCP based on the provided options.
func NewServer(options *dynamicrp.Options) (*hosting.Host, error) {
	metrics := options.Config.Metrics
	proof, err := mtls.ProcessStartupProof(options.Config.Authorization,
		metrics.Enabled && metrics.Prometheus != nil && metrics.Prometheus.Port > 0 &&
			metrics.Prometheus.Port <= 65535 && metrics.Prometheus.Path != mtls.StartupProofPath)
	if err != nil {
		return nil, err
	}
	services := []hosting.Service{}

	// Metrics is provided via a service.
	if options.Config.Metrics.Enabled {
		services = append(services, &metricsservice.Service{Options: &options.Config.Metrics, StartupProof: proof})
	}

	// Profiling is provided via a service.
	if options.Config.Profiler.Enabled {
		services = append(services, &profilerservice.Service{Options: &options.Config.Profiler})
	}

	// Tracing is provided via a service.
	if options.Config.Tracing.Enabled {
		services = append(services, &traceservice.Service{Options: &options.Config.Tracing})
	}

	services = append(services, frontend.NewService(options))
	services = append(services, backend.NewService(options))

	return &hosting.Host{
		Services: services,
	}, nil
}
