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
	"github.com/radius-project/radius/pkg/components/database/databaseprovider"
	metricsservice "github.com/radius-project/radius/pkg/metrics/service"
	profilerservice "github.com/radius-project/radius/pkg/profiler/service"
	"github.com/radius-project/radius/pkg/trace"
	"github.com/radius-project/radius/pkg/ucp"
	"github.com/radius-project/radius/pkg/ucp/backend"
	"github.com/radius-project/radius/pkg/ucp/data"
	"github.com/radius-project/radius/pkg/ucp/frontend/api"
	"github.com/radius-project/radius/pkg/ucp/hosting"
)

// NewServer creates a new hosting.Host instance with services for API, EmbeddedETCD, Metrics, Profiler and Backend (if
// enabled) based on the given Options.
func NewServer(options *ucp.Options) (*hosting.Host, error) {
	hostingServices := []hosting.Service{
		api.NewService(options),
		backend.NewService(options),
	}

	if options.Config.Database.Provider == databaseprovider.TypeETCD &&
		options.Config.Database.ETCD.InMemory {
		hostingServices = append(hostingServices, data.NewEmbeddedETCDService(data.EmbeddedETCDServiceOptions{ClientConfigSink: options.Config.Database.ETCD.Client}))
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

	return &hosting.Host{
		Services: hostingServices,
	}, nil
}
