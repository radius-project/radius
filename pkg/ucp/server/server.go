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
	"github.com/radius-project/radius/pkg/components/hosting"
	"github.com/radius-project/radius/pkg/components/metrics/metricsservice"
	"github.com/radius-project/radius/pkg/components/profiler/profilerservice"
	"github.com/radius-project/radius/pkg/components/trace/traceservice"
	"github.com/radius-project/radius/pkg/ucp"
	"github.com/radius-project/radius/pkg/ucp/backend"
	"github.com/radius-project/radius/pkg/ucp/frontend/api"
	"github.com/radius-project/radius/pkg/ucp/initializer"
)

// NewServer initializes a host for UCP based on the provided options.
func NewServer(options *ucp.Options) (*hosting.Host, error) {
	services := []hosting.Service{
		api.NewService(options),
		backend.NewService(options),
	}

	if options.Config.Metrics.Enabled {
		services = append(services, &metricsservice.Service{Options: &options.Config.Metrics})
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
