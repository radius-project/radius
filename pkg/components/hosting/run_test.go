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

	"github.com/radius-project/radius/test/testcontext"
	"github.com/stretchr/testify/require"
)

func Test_RunWithInterrupts_ServiceFailure_PropagatesError(t *testing.T) {
	ctx, cancel := testcontext.NewWithDeadline(t, time.Second*10)
	t.Cleanup(cancel)

	expectedErr := errors.New("service failed")

	host := &Host{
		Services: []Service{
			NewFuncService("failing", func(c context.Context) error {
				return expectedErr
			}),
		},
	}

	err := RunWithInterrupts(ctx, host)
	require.Error(t, err)
	require.Equal(t, expectedErr, err)
}

func Test_RunWithInterrupts_ServiceFailure_WithLongRunningService(t *testing.T) {
	ctx, cancel := testcontext.NewWithDeadline(t, time.Second*10)
	t.Cleanup(cancel)

	expectedErr := errors.New("initialization failed")

	host := &Host{
		Services: []Service{
			// A long-running service (like an HTTP server) that blocks until cancelled
			NewFuncService("server", func(c context.Context) error {
				<-c.Done()
				return nil
			}),
			// A service that fails immediately (like the initializer)
			NewFuncService("initializer", func(c context.Context) error {
				return expectedErr
			}),
		},
	}

	err := RunWithInterrupts(ctx, host)
	require.Error(t, err)
	require.Equal(t, expectedErr, err)
}

func Test_RunWithInterrupts_AllServicesComplete_NoError(t *testing.T) {
	ctx, cancel := testcontext.NewWithDeadline(t, time.Second*10)
	t.Cleanup(cancel)

	host := &Host{
		Services: []Service{
			NewFuncService("quick", func(c context.Context) error {
				return nil
			}),
		},
	}

	// When all services complete without error, the serviceErrors channel is closed
	// by host.Run without sending any messages. This causes RunWithInterrupts to
	// receive the zero-value (ok=false) and exit cleanly.
	err := RunWithInterrupts(ctx, host)
	require.NoError(t, err)
}

func Test_RunWithInterrupts_ShutdownTimeout_ReturnsError(t *testing.T) {
	ctx, cancel := testcontext.NewWithDeadline(t, time.Second*10)
	t.Cleanup(cancel)

	host := &Host{
		Services: []Service{
			// A service that fails to trigger shutdown
			NewFuncService("failing", func(c context.Context) error {
				return errors.New("boom")
			}),
			// A service that never terminates - will cause shutdown timeout
			NewFuncService("stuck", func(c context.Context) error {
				<-c.Done()
				time.Sleep(time.Second * 30)
				return nil
			}),
		},
		TimeoutFunc: func() {
			// Allow timeout to occur immediately
		},
	}

	err := RunWithInterrupts(ctx, host)
	// Shutdown timeout error from host.Run takes priority
	require.Error(t, err)
	require.Contains(t, err.Error(), "shutdown timeout reached")
}
