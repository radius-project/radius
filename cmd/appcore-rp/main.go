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

	"github.com/project-radius/radius/pkg/radlogger"
	"github.com/project-radius/radius/pkg/telemetry/metrics/metricsservice"
	mh "github.com/project-radius/radius/pkg/telemetry/metrics/metricsservice/hostoptions"
	"github.com/project-radius/radius/pkg/ucp/data"
	"github.com/project-radius/radius/pkg/ucp/dataprovider"
	"github.com/project-radius/radius/pkg/ucp/hosting"

	connector_backend "github.com/project-radius/radius/pkg/connectorrp/backend"
	connector_frontend "github.com/project-radius/radius/pkg/connectorrp/frontend"
)

func newConnectorHosts(configFile string, enableAsyncWorker bool) ([]hosting.Service, *hostoptions.HostOptions) {
	hostings := []hosting.Service{}
	options, err := hostoptions.NewHostOptionsFromEnvironment(configFile)
	if err != nil {
		log.Fatal(err)
	}
	hostings = append(hostings, connector_frontend.NewService(options))
	if enableAsyncWorker {
		hostings = append(hostings, connector_backend.NewService(options))
	}

	return hostings, &options
}

func main() {
	var configFile string
	var enableAsyncWorker bool

	var runConnector bool
	var connectorConfigFile string

	defaultConfig := fmt.Sprintf("radius-%s.yaml", hostoptions.Environment())
	flag.StringVar(&configFile, "config-file", defaultConfig, "The service configuration file.")
	flag.BoolVar(&enableAsyncWorker, "enable-asyncworker", true, "Flag to run async request process worker (for private preview and dev/test purpose).")

	flag.BoolVar(&runConnector, "run-connector", true, "Flag to run Applications.Link RP (for private preview and dev/test purpose).")
	defaultConnectorConfig := fmt.Sprintf("connector-%s.yaml", hostoptions.Environment())
	flag.StringVar(&connectorConfigFile, "connector-config", defaultConnectorConfig, "The service configuration file for Applications.Link.")

	if configFile == "" {
		log.Fatal("config-file is empty.")
	}

	flag.Parse()

	options, err := hostoptions.NewHostOptionsFromEnvironment(configFile)
	if err != nil {
		log.Fatal(err)
	}
	metricOptions := mh.NewHostOptionsFromEnvironment(*options.Config)

	logger, flush, err := radlogger.NewLogger("applications.core")
	if err != nil {
		log.Fatal(err)
	}
	defer flush()

	hostingSvc := []hosting.Service{frontend.NewService(options), metricsservice.NewService(metricOptions)}

	if enableAsyncWorker {
		logger.Info("Enable AsyncRequestProcessWorker.")
		hostingSvc = append(hostingSvc, backend.NewService(options))
	}

	// Configure Applications.Link to run it with Applications.Core RP.
	var connOpts *hostoptions.HostOptions
	if runConnector && connectorConfigFile != "" {
		logger.Info("Run Applications.Link.")
		var connSvcs []hosting.Service
		connSvcs, connOpts = newConnectorHosts(connectorConfigFile, enableAsyncWorker)
		hostingSvc = append(hostingSvc, connSvcs...)
	}

	if options.Config.StorageProvider.Provider == dataprovider.TypeETCD &&
		options.Config.StorageProvider.ETCD.InMemory {
		// For in-memory etcd we need to register another service to manage its lifecycle.
		//
		// The client will be initialized asynchronously.
		logger.Info("Enabled in-memory etcd")
		client := hosting.NewAsyncValue()
		options.Config.StorageProvider.ETCD.Client = client
		if connOpts != nil {
			connOpts.Config.StorageProvider.ETCD.Client = client
		}
		hostingSvc = append(hostingSvc, data.NewEmbeddedETCDService(data.EmbeddedETCDServiceOptions{ClientConfigSink: client}))
	}

	loggerValues := []interface{}{}
	host := &hosting.Host{
		Services: hostingSvc,

		// Values that will be propagated to all loggers
		LoggerValues: loggerValues,
	}

	ctx, cancel := context.WithCancel(logr.NewContext(context.Background(), logger))
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
		os.Exit(0)
	} else {
		panic(err)
	}
}
