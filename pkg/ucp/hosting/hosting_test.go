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

package hosting

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/project-radius/radius/test/testcontext"
	"github.com/stretchr/testify/require"
)

func Test_Host_RequiresServices(t *testing.T) {
	ctx, cancel := testcontext.NewWithDeadline(t, time.Second*5)
	t.Cleanup(cancel)

	host := &Host{
		Services: []Service{},
	}

	err := host.Run(ctx, nil)
	require.Error(t, err)
}

func Test_Host_DetectsDuplicates(t *testing.T) {
	ctx, cancel := testcontext.NewWithDeadline(t, time.Second*5)
	t.Cleanup(cancel)

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
	ctx, cancel := testcontext.NewWithDeadline(t, time.Second*5)
	t.Cleanup(cancel)

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
	ctx, cancel := testcontext.NewWithDeadline(t, time.Second*5)
	t.Cleanup(cancel)

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

// # Function Explanation
//
// NewFuncService creates a new Service with the given name and run function, which takes a context and returns an error if one occurs.
func NewFuncService(name string, run func(context.Context) error) Service {
	return &FuncService{name: name, run: run}
}

type FuncService struct {
	name string
	run  func(ctx context.Context) error
}

// # Function Explanation
//
// Name returns the name of the FuncService instance, or an error if the name is not set.
func (s *FuncService) Name() string {
	return s.name
}

// # Function Explanation
//
// Run takes in a context and calls the run function if it is not nil, otherwise it waits for the context to
// be done and returns nil.
func (s *FuncService) Run(ctx context.Context) error {
	if s.run == nil {
		<-ctx.Done()
		return nil
	}

	return s.run(ctx)
}
