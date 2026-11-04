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
	"errors"

	"k8s.io/client-go/kubernetes"

	"github.com/radius-project/radius/pkg/components/hosting"
	"github.com/radius-project/radius/pkg/components/metrics/metricsservice"
	"github.com/radius-project/radius/pkg/components/profiler/profilerservice"
	"github.com/radius-project/radius/pkg/components/trace/traceservice"
	"github.com/radius-project/radius/pkg/dynamicrp"
	"github.com/radius-project/radius/pkg/dynamicrp/backend"
	"github.com/radius-project/radius/pkg/dynamicrp/frontend"
	"github.com/radius-project/radius/pkg/recipes/terraform/config/backends"
	"github.com/radius-project/radius/pkg/upgrade/hashmigration"
)

// NewServer initializes a host for UCP based on the provided options.
func NewServer(options *dynamicrp.Options) (*hosting.Host, error) {
	services := []hosting.Service{}

	// Metrics is provided via a service.
	if options.Config.Metrics.Enabled {
		services = append(services, &metricsservice.Service{Options: &options.Config.Metrics})
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

	if !options.Config.SHA256Migration.Disabled {
		services = append(services, newHashMigrationService(options))
	}

	return &hosting.Host{
		Services: services,
	}, nil
}

// hashMigrationComponent is the component name dynamic-rp records its SHA-256 migration status under.
const hashMigrationComponent = "dynamic-rp"

// newHashMigrationService creates the service that migrates the SHA-1-derived values dynamic-rp owns: the Terraform
// states of its resources.
func newHashMigrationService(options *dynamicrp.Options) *hashmigration.Service {
	client := func() (kubernetes.Interface, error) {
		if options.KubernetesProvider == nil {
			return nil, errors.New("the Kubernetes client is not available")
		}

		return options.KubernetesProvider.ClientGoClient()
	}

	var reporter hashmigration.Reporter
	if kubernetesClient, err := client(); err == nil {
		reporter = hashmigration.NewConfigMapReporter(kubernetesClient, hashmigration.DefaultNamespace)
	}

	return hashmigration.NewService(hashmigration.Options{
		Component:    hashMigrationComponent,
		Steps:        []hashmigration.Step{backends.NewStateMigrationStep(client, options.DatabaseProvider.GetClient)},
		Reporter:     reporter,
		InitialDelay: hashmigration.DefaultInitialDelay,
	})
}
