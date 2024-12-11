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
	metricsservice "github.com/radius-project/radius/pkg/metrics/service"
	profilerservice "github.com/radius-project/radius/pkg/profiler/service"
	"github.com/radius-project/radius/pkg/trace"
	"github.com/radius-project/radius/pkg/ucp"
	"github.com/radius-project/radius/pkg/ucp/backend"
	"github.com/radius-project/radius/pkg/ucp/frontend/api"
	"github.com/radius-project/radius/pkg/ucp/hosting"
	"github.com/radius-project/radius/pkg/ucp/initializer"
)

// NewServer initializes a host for UCP based on the provided options.
func NewServer(options *ucp.Options) (*hosting.Host, error) {
	hostingServices := []hosting.Service{
		api.NewService(options),
		backend.NewService(options),
	}

	if options.Config.Metrics.Prometheus.Enabled {
		metricOptions := metricsservice.HostOptions{
			Config: &options.Config.Metrics,
		}
		hostingServices = append(hostingServices, metricsservice.NewService(metricOptions))
	}

	if options.Config.Profiler.Enabled {
		profilerOptions := profilerservice.HostOptions{
			Config: &options.Config.Profiler,
		}
		hostingServices = append(hostingServices, profilerservice.NewService(profilerOptions))
	}

	hostingServices = append(hostingServices, &trace.Service{Options: options.Config.Tracing})

	hostingServices = append(hostingServices, initializer.NewService(options))

	return &hosting.Host{
		Services: hostingServices,
	}, nil
}
