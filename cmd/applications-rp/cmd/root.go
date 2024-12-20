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

package cmd

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/spf13/cobra"
	runtimelog "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/radius-project/radius/pkg/armrpc/builder"
	"github.com/radius-project/radius/pkg/armrpc/hostoptions"
	"github.com/radius-project/radius/pkg/components/metrics/metricsservice"
	"github.com/radius-project/radius/pkg/components/profiler/profilerservice"
	"github.com/radius-project/radius/pkg/components/trace/traceservice"
	"github.com/radius-project/radius/pkg/recipes/controllerconfig"
	"github.com/radius-project/radius/pkg/server"

	"github.com/radius-project/radius/pkg/components/hosting"
	"github.com/radius-project/radius/pkg/ucp/ucplog"

	corerp_setup "github.com/radius-project/radius/pkg/corerp/setup"
	daprrp_setup "github.com/radius-project/radius/pkg/daprrp/setup"
	dsrp_setup "github.com/radius-project/radius/pkg/datastoresrp/setup"
	msgrp_setup "github.com/radius-project/radius/pkg/messagingrp/setup"
)

const serviceName = "radius"

var rootCmd = &cobra.Command{
	Use:   "applications-rp",
	Short: "Applications.* Resource Provider Server",
	Long:  `Server process for the Applications.* Resource Provider (applications-rp).`,
	RunE: func(cmd *cobra.Command, args []string) error {
		configFilePath := cmd.Flag("config-file").Value.String()
		options, err := hostoptions.NewHostOptionsFromEnvironment(configFilePath)
		if err != nil {
			return err
		}

		logger, flush, err := ucplog.NewLogger(serviceName, &options.Config.Logging)
		if err != nil {
			return err
		}
		defer flush()

		// Must set the logger before using controller-runtime.
		runtimelog.SetLogger(logger)

		services := []hosting.Service{}
		if options.Config.MetricsProvider.Enabled {
			services = append(services, &metricsservice.Service{Options: &options.Config.MetricsProvider})
		}

		if options.Config.ProfilerProvider.Enabled {
			services = append(services, &profilerservice.Service{Options: &options.Config.ProfilerProvider})
		}

		if options.Config.TracerProvider.Enabled {
			services = append(services, &traceservice.Service{Options: &options.Config.TracerProvider})
		}

		builders, err := builders(options)
		if err != nil {
			return err
		}

		services = append(
			services,
			server.NewAPIService(options, builders),
			server.NewAsyncWorker(options, builders),
		)

		host := &hosting.Host{
			Services: services,
		}

		// Make the logger available to the services.
		ctx := logr.NewContext(context.Background(), logger)

		// Make the hosting configuration available to the services.
		ctx = hostoptions.WithContext(ctx, options.Config)

		return hosting.RunWithInterrupts(ctx, host)
	},
}

func Execute() {
	// Let users override the configuration via `--config-file`.
	rootCmd.Flags().String("config-file", fmt.Sprintf("applications-rp-%s.yaml", hostoptions.Environment()), "The service configuration file.")
	cobra.CheckErr(rootCmd.ExecuteContext(context.Background()))
}

func builders(options hostoptions.HostOptions) ([]builder.Builder, error) {
	config, err := controllerconfig.New(options)
	if err != nil {
		return nil, err
	}

	return []builder.Builder{
		corerp_setup.SetupNamespace(config).GenerateBuilder(),
		daprrp_setup.SetupNamespace(config).GenerateBuilder(),
		msgrp_setup.SetupNamespace(config).GenerateBuilder(),
		dsrp_setup.SetupNamespace(config).GenerateBuilder(),
		// Add resource provider builders...
	}, nil
}
