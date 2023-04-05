// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package hosting

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/project-radius/radius/pkg/ucp/util"
)

const ShutdownTimeout = time.Second * 10

// Service is an abstraction for a long-running subsystem of the RP.
type Service interface {
	// Name returns the name of the service.
	Name() string

	// Run runs the service as a blocking operation.
	Run(ctx context.Context) error
}

// Host manages the lifetimes and starting of Services.
type Host struct {
	// Slice of services to run. Started in order.
	Services []Service

	// LoggerValues is key-value-pairs passed to .WithValues to initialize the logger for the host.
	LoggerValues []any

	// TimeoutFunc allows you to control the timeout behavior for testing
	TimeoutFunc func()
}

// LifecycleMessage is a message returned when a service terminates.
type LifecycleMessage struct {
	Name string
	Err  error
}

func (host *Host) RunAsync(ctx context.Context) (<-chan error, <-chan LifecycleMessage) {
	stopped := make(chan error, 1)
	serviceErrors := make(chan LifecycleMessage, len(host.Services))

	go func() {
		fmt.Println(fmt.Sprintf("@@@@@@ Before calling host.Run in %s, goroutineCount: %v", util.GetCaller(), runtime.NumGoroutine()))
		err := host.Run(ctx, serviceErrors)
		fmt.Println(fmt.Sprintf("@@@@@@ After calling host.Run in %s, goroutineCount: %v", util.GetCaller(), runtime.NumGoroutine()))
		stopped <- err
		close(stopped)
	}()

	return stopped, serviceErrors
}

// Run launches and runs as a blocking call all services until graceful shutdown or timeout occurs.
func (host *Host) Run(ctx context.Context, serviceErrors chan<- LifecycleMessage) error {
	if serviceErrors != nil {
		defer close(serviceErrors)
	}

	if len(host.Services) == 0 {
		return errors.New("at least one service is required")
	}

	logger := logr.FromContextOrDiscard(ctx)
	logger = logger.WithValues(host.LoggerValues...)
	ctx = logr.NewContext(ctx, logger)

	messages := make(chan LifecycleMessage, len(host.Services))
	defer close(messages)

	// Track running services so we know when they all stop.
	running := map[string]bool{}

	// Detect duplicate names before we launch any work.
	for _, service := range host.Services {
		_, ok := running[service.Name()]
		if ok {
			return fmt.Errorf("detected duplicate service %s", service.Name())
		}

		// Record that this service was started. We're guaranteed to get
		// a message about its lifecycle and that's where we remove it.
		//
		// NOTE: DO NOT access this inside a goroutine.
		running[service.Name()] = true
	}

	for i := range host.Services {
		service := host.Services[i]
		logger.Info(fmt.Sprintf("Starting %s", service.Name()))

		// Error reporting is asynchronous. We don't early exit on first error.
		go func() {
			// Handle a panic from the service
			defer func() {
				value := recover()
				if value != nil {
					// Log here to force the original call stack to be logged.
					err := fmt.Errorf("service %s panicked: %v", service.Name(), value)
					logger.Error(err, "recovered from panic")
					messages <- LifecycleMessage{Name: service.Name(), Err: err}
				}
			}()

			err := host.runService(ctx, service, messages)
			messages <- LifecycleMessage{Name: service.Name(), Err: err}
		}()
	}

	// Handle shutdown timeouts.
	timeout := make(chan struct{}, 1)
	go func() {
		<-ctx.Done()
		if host.TimeoutFunc != nil {
			// Override to control timeout behavior for testing
			host.TimeoutFunc()
		} else {
			time.Sleep(ShutdownTimeout)
		}

		timeout <- struct{}{}
		close(timeout)
	}()

	logger.Info("Started all services", "count", len(host.Services))

	// Now that all services are running we just need to wait for all services to stop, or for a timeout
	// to occur
	for len(running) > 0 {
		select {
		case message := <-messages:
			// Remove from running table
			delete(running, message.Name)

			if message.Err != nil {
				logger.Error(message.Err, fmt.Sprintf("Service %s terminated with error: %v", message.Name, message.Err))

				// Report error to client
				if serviceErrors != nil {
					serviceErrors <- message
				}
			} else {
				logger.Info(fmt.Sprintf("Service %s terminated gracefully", message.Name))
			}

		case <-timeout:
			names := []string{}
			for k := range running {
				names = append(names, k)
			}
			sort.Strings(names)

			err := fmt.Errorf("shutdown timeout reached while the following services are still running: %s", strings.Join(names, ", "))
			logger.Error(err, "Shutdown timeout reached")
			return err
		}
	}

	return nil
}

func (host *Host) runService(ctx context.Context, service Service, messages chan<- LifecycleMessage) error {
	// Create a new logger and context for the service to use.
	logger := logr.FromContextOrDiscard(ctx)
	logger = logger.WithName(service.Name())
	ctx = logr.NewContext(ctx, logger)

	logger.Info(fmt.Sprintf("@@@@@@ Before calling service.Run for %s in %s, goroutineCount: %v", service.Name(), util.GetCaller(), runtime.NumGoroutine()))
	err := service.Run(ctx)
	logger.Info(fmt.Sprintf("@@@@@@ After calling service.Run for %s in %s, goroutineCount: %v", service.Name(), util.GetCaller(), runtime.NumGoroutine()))

	// Suppress a cancellation-related error. That's a graceful exit.
	if err == ctx.Err() {
		return nil
	} else if err != nil {
		return err
	}

	return nil
}
