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
	"context"

	"github.com/radius-project/radius/pkg/authz/mtls"
	"github.com/radius-project/radius/pkg/components/hosting"
	"github.com/radius-project/radius/pkg/components/metrics/metricsservice"
	"github.com/radius-project/radius/pkg/components/profiler/profilerservice"
	"github.com/radius-project/radius/pkg/components/trace/traceservice"
	"github.com/radius-project/radius/pkg/ucp"
	"github.com/radius-project/radius/pkg/ucp/backend"
	"github.com/radius-project/radius/pkg/ucp/frontend/api"
	ucp_aws_frontend "github.com/radius-project/radius/pkg/ucp/frontend/aws"
	"github.com/radius-project/radius/pkg/ucp/initializer"
)

// NewServer initializes TLS evidence and shared AWS clients before creating the UCP host.
func NewServer(ctx context.Context, options *ucp.Options) (*hosting.Host, error) {
	metrics := options.Config.Metrics
	proof, err := mtls.ProcessStartupProof(options.Config.Authorization,
		metrics.Enabled && metrics.Prometheus != nil && metrics.Prometheus.Port > 0 &&
			metrics.Prometheus.Port <= 65535 && metrics.Prometheus.Path != mtls.StartupProofPath)
	if err != nil {
		return nil, err
	}
	frontend := api.NewService(options)
	aggregationProof, err := frontend.PrepareAggregationProof(
		metrics.Enabled && metrics.Prometheus != nil && metrics.Prometheus.Port > 0 &&
			metrics.Prometheus.Port <= 65535 && metrics.Prometheus.Path != mtls.AggregationProofPath)
	if err != nil {
		return nil, err
	}
	if err := ucp_aws_frontend.InitializeClients(ctx, options); err != nil {
		return nil, err
	}

	services := []hosting.Service{
		frontend,
		backend.NewService(options),
	}

	if options.Config.Metrics.Enabled {
		services = append(services, &metricsservice.Service{Options: &options.Config.Metrics, StartupProof: proof, AggregationProof: aggregationProof})
	}

	if options.Config.Profiler.Enabled {
		services = append(services, &profilerservice.Service{Options: &options.Config.Profiler})
	}

	if options.Config.Tracing.Enabled {
		services = append(services, &traceservice.Service{Options: &options.Config.Tracing})
	}

	services = append(services, initializer.NewService(options))

	return &hosting.Host{
		Services: services,
	}, nil
}
