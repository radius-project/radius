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

package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/go-logr/logr"
	"github.com/spf13/pflag"
	etcdclient "go.etcd.io/etcd/client/v3"
	runtimelog "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/radius-project/radius/pkg/armrpc/builder"
	"github.com/radius-project/radius/pkg/armrpc/hostoptions"
	metricsservice "github.com/radius-project/radius/pkg/metrics/service"
	profilerservice "github.com/radius-project/radius/pkg/profiler/service"
	"github.com/radius-project/radius/pkg/recipes/controllerconfig"
	"github.com/radius-project/radius/pkg/server"
	"github.com/radius-project/radius/pkg/trace"

	pr_backend "github.com/radius-project/radius/pkg/portableresources/backend"
	pr_frontend "github.com/radius-project/radius/pkg/portableresources/frontend"
	"github.com/radius-project/radius/pkg/ucp/data"
	"github.com/radius-project/radius/pkg/ucp/dataprovider"
	"github.com/radius-project/radius/pkg/ucp/hosting"
	"github.com/radius-project/radius/pkg/ucp/ucplog"

	corerp_setup "github.com/radius-project/radius/pkg/corerp/setup"
	messagingrp_setup "github.com/radius-project/radius/pkg/messagingrp/setup"
)

const serviceName = "radius"

func main() {
	var configFile string
	defaultConfig := fmt.Sprintf("radius-%s.yaml", hostoptions.Environment())
	pflag.StringVar(&configFile, "config-file", defaultConfig, "The service configuration file.")
	if configFile == "" {
		log.Fatal("config-file is empty.") //nolint:forbidigo // this is OK inside the main function.
	}

	var portableResourceConfigFile string
	defaultPortableRsConfig := fmt.Sprintf("portableresource-%s.yaml", hostoptions.Environment())
	pflag.StringVar(&portableResourceConfigFile, "portableresource-config", defaultPortableRsConfig, "The service configuration file for portable resource providers.")

	pflag.Parse()

	options, err := hostoptions.NewHostOptionsFromEnvironment(configFile)
	if err != nil {
		log.Fatal(err) //nolint:forbidigo // this is OK inside the main function.
	}

	hostingSvc := []hosting.Service{}

	metricOptions := metricsservice.NewHostOptionsFromEnvironment(*options.Config)
	metricOptions.Config.ServiceName = serviceName
	if metricOptions.Config.Prometheus.Enabled {
		hostingSvc = append(hostingSvc, metricsservice.NewService(metricOptions))
	}

	profilerOptions := profilerservice.NewHostOptionsFromEnvironment(*options.Config)
	if profilerOptions.Config.Enabled {
		hostingSvc = append(hostingSvc, profilerservice.NewService(profilerOptions))
	}

	logger, flush, err := ucplog.NewLogger(serviceName, &options.Config.Logging)
	if err != nil {
		log.Fatal(err) //nolint:forbidigo // this is OK inside the main function.
	}
	defer flush()

	// Must set the logger before using controller-runtime.
	runtimelog.SetLogger(logger)

	// Load portable resource config.
	prOptions, err := hostoptions.NewHostOptionsFromEnvironment(portableResourceConfigFile)
	if err != nil {
		log.Fatal(err) //nolint:forbidigo // this is OK inside the main function.
	}

	if options.Config.StorageProvider.Provider == dataprovider.TypeETCD &&
		options.Config.StorageProvider.ETCD.InMemory {
		// For in-memory etcd we need to register another service to manage its lifecycle.
		//
		// The client will be initialized asynchronously.
		logger.Info("Enabled in-memory etcd")
		client := hosting.NewAsyncValue[etcdclient.Client]()
		options.Config.StorageProvider.ETCD.Client = client
		options.Config.SecretProvider.ETCD.Client = client

		// Portable resource options
		prOptions.Config.StorageProvider.ETCD.Client = client
		prOptions.Config.SecretProvider.ETCD.Client = client

		hostingSvc = append(hostingSvc, data.NewEmbeddedETCDService(data.EmbeddedETCDServiceOptions{ClientConfigSink: client}))
	}

	builders, err := builders(options)
	if err != nil {
		log.Fatal(err) //nolint:forbidigo // this is OK inside the main function.
	}

	hostingSvc = append(
		hostingSvc,
		server.NewAPIService(options, builders),
		server.NewAsyncWorker(options, builders),

		// Configure Portable Resources to run it with Applications.Core RP.
		//
		// This is temporary until we migrate these resources to use the new registration model.
		pr_frontend.NewService(prOptions),
		pr_backend.NewService(prOptions),
	)

	loggerValues := []any{}
	host := &hosting.Host{
		Services: hostingSvc,

		// Values that will be propagated to all loggers
		LoggerValues: loggerValues,
	}

	ctx, cancel := context.WithCancel(logr.NewContext(context.Background(), logger))

	tracerOpts := options.Config.TracerProvider
	tracerOpts.ServiceName = serviceName
	shutdown, err := trace.InitTracer(tracerOpts)
	if err != nil {
		log.Fatal(err) //nolint:forbidigo // this is OK inside the main function.
	}
	defer func() {
		if err := shutdown(ctx); err != nil {
			log.Printf("failed to shutdown TracerProvider: %v\n", err)
		}

	}()
	stopped, serviceErrors := host.RunAsync(ctx)

	exitCh := make(chan os.Signal, 2)
	signal.Notify(exitCh, os.Interrupt, syscall.SIGTERM)

	select {
	// Shutdown triggered
	case <-exitCh:
		logger.Info("Shutting down....")
		cancel()

	// A service terminated with a failure. Shut down
	case <-serviceErrors:
		logger.Info("Error occurred - shutting down....")
		cancel()
	}

	// Finished shutting down. An error returned here is a failure to terminate
	// gracefully, so just crash if that happens.
	err = <-stopped
	if err == nil {
		os.Exit(0) //nolint:forbidigo // this is OK inside the main function.
	} else {
		panic(err)
	}
}

func builders(options hostoptions.HostOptions) ([]builder.Builder, error) {
	config, err := controllerconfig.New(options)
	if err != nil {
		return nil, err
	}

	return []builder.Builder{
		corerp_setup.SetupNamespace(config).GenerateBuilder(),
		messagingrp_setup.SetupNamespace(config).GenerateBuilder(),
		// Add resource provider builders...
	}, nil
}
