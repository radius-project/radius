// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/Azure/radius/pkg/health"
	"github.com/Azure/radius/pkg/hosting"
	"github.com/Azure/radius/pkg/radlogger"
	"github.com/Azure/radius/pkg/radrp/backend/healthlistener"
	"github.com/Azure/radius/pkg/radrp/frontend"
	"github.com/Azure/radius/pkg/radrp/hostoptions"
	"github.com/go-logr/logr"
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

	host := &hosting.Host{
		Services: []hosting.Service{
			frontend.NewService(frontend.NewServiceOptions(options)),
			healthlistener.NewService(healthlistener.NewServiceOptions(options)),
			health.NewService(health.NewServiceOptions(options)),
		},

		// Values that will be propagated to all loggers
		LoggerValues: []interface{}{
			radlogger.LogFieldResourceGroup, options.Arm.ResourceGroup,
			radlogger.LogFieldSubscriptionID, options.Arm.SubscriptionID,
		},
	}

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
