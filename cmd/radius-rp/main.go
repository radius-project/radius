// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/go-logr/logr"
	"github.com/project-radius/radius/pkg/armrpc/hostoptions"
	"github.com/project-radius/radius/pkg/corerp/backend"
	"github.com/project-radius/radius/pkg/corerp/frontend"
	metricsservice "github.com/project-radius/radius/pkg/metrics/service"
	profilerservice "github.com/project-radius/radius/pkg/profiler/service"
	"github.com/project-radius/radius/pkg/trace"

	"github.com/project-radius/radius/pkg/logging"
	"github.com/project-radius/radius/pkg/ucp/data"
	"github.com/project-radius/radius/pkg/ucp/dataprovider"
	"github.com/project-radius/radius/pkg/ucp/hosting"
	"github.com/project-radius/radius/pkg/ucp/ucplog"
	etcdclient "go.etcd.io/etcd/client/v3"
)

const serviceName = "Radius resource provider"

func main() {
	var configFile string

	defaultConfig := fmt.Sprintf("radius-%s.yaml", hostoptions.Environment())
	flag.StringVar(&configFile, "config-file", defaultConfig, "The service configuration file.")

	if configFile == "" {
		log.Fatal("config-file is empty.") //nolint:forbidigo // this is OK inside the main function.
	}

	flag.Parse()

	options, err := hostoptions.NewHostOptionsFromEnvironment(configFile)
	if err != nil {
		log.Fatal(err) //nolint:forbidigo // this is OK inside the main function.
	}
	hostingSvc := []hosting.Service{frontend.NewService(options)}

	logger, flush, err := ucplog.NewLogger(logging.AppCoreLoggerName, &options.Config.Logging)
	if err != nil {
		log.Fatal(err) //nolint:forbidigo // this is OK inside the main function.
	}
	defer flush()

	logger.Info("Enable AsyncRequestProcessWorker.")
	hostingSvc = append(hostingSvc, backend.NewService(options))

	metricOptions := metricsservice.NewHostOptionsFromEnvironment(*options.Config)
	metricOptions.Config.ServiceName = serviceName
	if metricOptions.Config.Prometheus.Enabled {
		hostingSvc = append(hostingSvc, metricsservice.NewService(metricOptions))
	}

	profilerOptions := profilerservice.NewHostOptionsFromEnvironment(*options.Config)
	if profilerOptions.Config.Enabled {
		hostingSvc = append(hostingSvc, profilerservice.NewService(profilerOptions))
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
		hostingSvc = append(hostingSvc, data.NewEmbeddedETCDService(data.EmbeddedETCDServiceOptions{ClientConfigSink: client}))
	}

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
