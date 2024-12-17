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
	"time"

	"github.com/radius-project/radius/pkg/components/database/databaseprovider"
	"github.com/radius-project/radius/pkg/dynamicrp"
	"github.com/radius-project/radius/pkg/dynamicrp/backend"
	"github.com/radius-project/radius/pkg/dynamicrp/frontend"
	metricsservice "github.com/radius-project/radius/pkg/metrics/service"
	profilerservice "github.com/radius-project/radius/pkg/profiler/service"
	"github.com/radius-project/radius/pkg/trace"
	"github.com/radius-project/radius/pkg/ucp/data"
	"github.com/radius-project/radius/pkg/ucp/hosting"
)

const (
	HTTPServerStopTimeout = time.Second * 10
	ServiceName           = "dynamic-rp"
)

const UCPProviderName = "System.Resources"

// NewServer creates a new hosting.Host instance with services for API, EmbeddedETCD, Metrics, Profiler and Backend (if
// enabled) based on the given Options.
func NewServer(options *dynamicrp.Options) (*hosting.Host, error) {
	services := []hosting.Service{}

	// In-memory ETCD requires a service running in the process.
	if options.Config.Database.Provider == databaseprovider.TypeETCD &&
		options.Config.Database.ETCD.InMemory {
		services = append(services, data.NewEmbeddedETCDService(data.EmbeddedETCDServiceOptions{ClientConfigSink: options.Config.Database.ETCD.Client}))
	}

	// Metrics is provided via a service.
	if options.Config.Metrics.Prometheus.Enabled {
		services = append(services, metricsservice.NewService(metricsservice.HostOptions{
			Config: &options.Config.Metrics,
		}))
	}

	// Profiling is provided via a service.
	if options.Config.Profiler.Enabled {
		services = append(services, profilerservice.NewService(profilerservice.HostOptions{
			Config: &options.Config.Profiler,
		}))
	}

	// Tracing is provided via a service.
	if options.Config.Tracing.ServiceName != "" {
		services = append(services, &trace.Service{Options: options.Config.Tracing})
	}

	services = append(services, frontend.NewService(options))
	services = append(services, backend.NewService(options))

	return &hosting.Host{
		Services: services,
	}, nil
}
