// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/go-logr/logr"
	"github.com/project-radius/radius/pkg/health"
	"github.com/project-radius/radius/pkg/radlogger"
	"github.com/project-radius/radius/pkg/radrp/backend/healthlistener"
	"github.com/project-radius/radius/pkg/radrp/frontend"
	"github.com/project-radius/radius/pkg/radrp/hostoptions"
	"github.com/project-radius/radius/pkg/ucp/hosting"
)

func main() {
	options, err := hostoptions.NewHostOptionsFromEnvironment()
	if err != nil {
		log.Println(err.Error())
		os.Exit(1)
	}

	logger, flush, err := radlogger.NewLogger("rad-rp")
	if err != nil {
		log.Println(err.Error())
		os.Exit(1)
	}
	defer flush()

	loggerValues := []interface{}{}
	if options.Arm != nil {
		loggerValues = []interface{}{
			radlogger.LogFieldRPIdentifier, options.RPIdentifier,
		}
	}

	host := &hosting.Host{
		Services: []hosting.Service{
			frontend.NewService(frontend.NewServiceOptions(options)),
			healthlistener.NewService(healthlistener.NewServiceOptions(options)),
			health.NewService(health.NewServiceOptions(options)),
		},

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
