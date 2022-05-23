// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package hosting

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

func Test_Host_RequiresServices(t *testing.T) {
	ctx, cancel := context.WithDeadline(createContext(t), time.Now().Add(time.Second*5))
	defer cancel()

	host := &Host{
		Services: []Service{},
	}

	err := host.Run(ctx, nil)
	require.Error(t, err)
}

func Test_Host_DetectsDuplicates(t *testing.T) {
	ctx, cancel := context.WithDeadline(createContext(t), time.Now().Add(time.Second*5))
	defer cancel()

	host := &Host{
		Services: []Service{
			NewFuncService("A", func(c context.Context) error { return nil }),
			NewFuncService("A", func(c context.Context) error { return nil }),
		},
	}

	err := host.Run(ctx, nil)
	require.Error(t, err)
}

func Test_Host_RunMultipleServices_HandlesExit(t *testing.T) {
	ctx, cancel := context.WithDeadline(createContext(t), time.Now().Add(time.Second*5))
	defer cancel()

	started := make(chan struct{})

	host := &Host{
		Services: []Service{
			// Different types of exit
			NewFuncService("A", func(c context.Context) error {
				// Early exit
				started <- struct{}{}
				return nil
			}),
			NewFuncService("B", func(c context.Context) error {
				// Graceful exit during shutdown
				started <- struct{}{}
				<-c.Done()
				return nil
			}),
			NewFuncService("C", func(c context.Context) error {
				// Cancellation
				started <- struct{}{}
				<-c.Done()
				return c.Err()
			}),
			NewFuncService("D", func(c context.Context) error {
				// Early-exit error
				started <- struct{}{}
				return errors.New("error from D")
			}),
			NewFuncService("E", func(c context.Context) error {
				// Shutdown error
				started <- struct{}{}
				<-c.Done()
				return errors.New("error from E")
			}),
			NewFuncService("F", func(c context.Context) error {
				// Panic
				started <- struct{}{}
				<-c.Done()
				panic("oh my!")
			}),
		},
	}

	serviceErrors := make(chan LifecycleMessage, len(host.Services))
	stopped := make(chan error)

	// Run the host
	go func() {
		err := host.Run(ctx, serviceErrors)
		stopped <- err
		close(stopped)
		close(started)
	}()

	// Wait for all services to start
	for i := 0; i < len(host.Services); i++ {
		<-started
	}

	// Should have an error from D
	message := <-serviceErrors
	require.Equal(t, "D", message.Name)
	require.Error(t, message.Err)

	// Trigger shutdown - it's not considered a timeout in this case.
	cancel()
	err := <-stopped
	require.NoError(t, err)

	// Could be E or F (order is random)
	message = <-serviceErrors
	require.Regexp(t, "[EF]", message.Name)
	require.Error(t, message.Err)

	message = <-serviceErrors
	require.Regexp(t, "[EF]", message.Name)
	require.Error(t, message.Err)
}

func Test_Host_RunMultipleServices_ShutdownTimeout(t *testing.T) {
	ctx, cancel := context.WithDeadline(createContext(t), time.Now().Add(time.Second*5))
	defer cancel()

	started := make(chan struct{})

	host := &Host{
		Services: []Service{
			NewFuncService("A", func(c context.Context) error {
				// Does not terminate
				started <- struct{}{}
				<-c.Done()
				time.Sleep(time.Second * 30)
				return nil
			}),
			NewFuncService("B", func(c context.Context) error {
				// Does not terminate
				started <- struct{}{}
				<-c.Done()
				time.Sleep(time.Second * 30)
				return nil
			}),
		},
		TimeoutFunc: func() {
			// Allow a timeout to occur immediately after shutdown.
		},
	}

	serviceErrors := make(chan LifecycleMessage, len(host.Services))
	stopped := make(chan error)

	// Run the host
	go func() {
		err := host.Run(ctx, serviceErrors)
		stopped <- err
		close(stopped)
		close(started)
	}()

	// Wait for all services to start
	for i := 0; i < len(host.Services); i++ {
		<-started
	}

	// Trigger shutdown - it's not considered a timeout in this case.
	cancel()
	err := <-stopped
	require.Error(t, err)
	require.Equal(t, "shutdown timeout reached while the following services are still running: A, B", err.Error())
}

func NewFuncService(name string, run func(context.Context) error) Service {
	return &FuncService{name: name, run: run}
}

type FuncService struct {
	name string
	run  func(ctx context.Context) error
}

func (s *FuncService) Name() string {
	return s.name
}

func (s *FuncService) Run(ctx context.Context) error {
	if s.run == nil {
		<-ctx.Done()
		return nil
	}

	return s.run(ctx)
}

func createContext(t *testing.T) context.Context {
	return logr.NewContext(context.Background(), zapr.NewLogger(zaptest.NewLogger(t)))
}
