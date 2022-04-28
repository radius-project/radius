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
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/go-logr/logr"
	"github.com/project-radius/radius/pkg/corerp/frontend"
	"github.com/project-radius/radius/pkg/corerp/hostoptions"
	"github.com/project-radius/radius/pkg/hosting"
	"github.com/project-radius/radius/pkg/radlogger"
	"go.opentelemetry.io/otel/exporters/metric/prometheus"
	"go.opentelemetry.io/otel/metric/global"
	export "go.opentelemetry.io/otel/sdk/export/metric"
	"go.opentelemetry.io/otel/sdk/metric/aggregator/histogram"
	controller "go.opentelemetry.io/otel/sdk/metric/controller/basic"
	processor "go.opentelemetry.io/otel/sdk/metric/processor/basic"
	selector "go.opentelemetry.io/otel/sdk/metric/selector/simple"
)

func initMeter(port int, endpoint string) {
	promConfig := prometheus.Config{}
	c := controller.New(
		processor.New(
			selector.NewWithHistogramDistribution(
				histogram.WithExplicitBoundaries(promConfig.DefaultHistogramBoundaries),
			),
			export.CumulativeExportKindSelector(),
		),
	)
	exporter, err := prometheus.NewExporter(promConfig, c)
	if err != nil {
		log.Panicf("failed to initialize prometheus exporter %v", err)
	}

	global.SetMeterProvider(exporter.MeterProvider())

	http.HandleFunc(endpoint, exporter.ServeHTTP)
	concatenatedPort := ":" + strconv.Itoa(port)
	go func() {
		_ = http.ListenAndServe(concatenatedPort, nil)
	}()
	log.Printf("Prometheus server running on %s", concatenatedPort)
}

func main() {
	var configFile string

	defaultConfig := fmt.Sprintf("radius-%s.yaml", hostoptions.Environment())
	flag.StringVar(&configFile, "config-file", defaultConfig, "The service configuration file.")
	if configFile == "" {
		log.Fatal("config-file is empty.")
	}

	flag.Parse()

	options, err := hostoptions.NewHostOptionsFromEnvironment(configFile)
	if err != nil {
		log.Fatal(err)
	}

	logger, flush, err := radlogger.NewLogger("applications.core")
	if err != nil {
		log.Fatal(err)
	}
	defer flush()

	loggerValues := []interface{}{}
	host := &hosting.Host{
		Services: []hosting.Service{
			frontend.NewService(options),
		},

		// Values that will be propagated to all loggers
		LoggerValues: loggerValues,
	}
	initMeter(options.Config.Metrics.Port, options.Config.Metrics.Endpoint)

	// Create a channel to handle the shutdown
	exitCh := make(chan os.Signal, 1)
	signal.Notify(exitCh, syscall.SIGINT, syscall.SIGTERM)

	ctx, cancel := context.WithCancel(logr.NewContext(context.Background(), logger))
	stopped, serviceErrors := host.RunAsync(ctx)

	for {
		select {

		// Shutdown triggered
		case <-exitCh:
			fmt.Println("Shutting down....")
			cancel()

		// A service terminated with a failure. Shut down
		case <-serviceErrors:
			fmt.Println("Shutting down....")
			cancel()

		// Finished shutting down. An error returned here is a failure to terminate
		// gracefully, so just crash if that happens.
		case err := <-stopped:
			if err == nil {
				os.Exit(0)
			} else {
				panic(err)
			}
		}
	}
}
