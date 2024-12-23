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

// Package testhost provides a host for running any Radius control-plane component
// as an in-memory server for testing purposes.
//
// This package should be wrapped in a test package specific to the component under test.
// The wrapping design allows for component-specific depenendendencies to be defined without
// polluting the shared code.
package testhost

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/radius-project/radius/pkg/ucp/hosting"
	"github.com/radius-project/radius/test/testcontext"
	"github.com/stretchr/testify/require"
)

// StartHost starts a new test host for the given hosting.Host and returns a TestHost instance.
// The TestHost will have its lifecycle managed by the test context, and will be shut down when the test
// completes.
func StartHost(t *testing.T, host *hosting.Host, baseURL string) *TestHost {
	ctx, cancel := context.WithCancel(testcontext.New(t))
	errs, messages := host.RunAsync(ctx)

	go func() {
		for msg := range messages {
			t.Logf("Message: %s", msg)
		}
	}()

	th := &TestHost{
		baseURL:     baseURL,
		host:        host,
		messages:    messages,
		cancel:      cancel,
		stoppedChan: errs,
		t:           t,
	}
	t.Cleanup(th.Close)

	// Wait for the server to start listening on the port.
	require.Eventuallyf(t, func() bool {
		u, err := url.Parse(baseURL)
		if err != nil {
			panic("Invalid URL: " + baseURL)
		}

		conn, err := net.Dial("tcp", net.JoinHostPort(u.Hostname(), u.Port()))
		if err != nil {
			t.Logf("Waiting for server to start listening on port: %v", err)
			return false
		}
		defer conn.Close()

		return true
	}, time.Second*5, time.Millisecond*20, "server did not start listening on port")

	return th
}

// TestHost is a test server for any Radius control-plane component. Do not construct this type directly, use the Start function.
type TestHost struct {
	// baseURL is the base URL of the server, including the path base.
	baseURL string

	// host is the hosting process running the component.
	host *hosting.Host

	// messages is the channel that will receive lifecycle messages from the host.
	messages <-chan hosting.LifecycleMessage

	// cancel is the function to call to stop the server.
	cancel context.CancelFunc

	// stoppedChan is the channel that will be closed when the server has stopped.
	stoppedChan <-chan error

	// shutdown is used to ensure that Close is only called once.
	shutdown sync.Once

	// t is the testing.T instance to use for assertions.
	t *testing.T
}

// Close shuts down the server and will block until shutdown completes.
func (th *TestHost) Close() {
	// We're being picking about resource cleanup here, because unless we are picky we hit scalability
	// problems in tests pretty quickly.
	th.shutdown.Do(func() {
		// Shut down the host.
		th.cancel()

		if th.stoppedChan != nil {
			<-th.stoppedChan // host stopped
		}
	})
}

// BaseURL returns the base URL of the server, including the path base.
//
// This should be used as a URL prefix for all requests to the server.
func (th *TestHost) BaseURL() string {
	return th.baseURL
}

// Client returns the HTTP client to use to make requests to the server.
func (th *TestHost) Client() *http.Client {
	return http.DefaultClient
}

// T returns the testing.T instance associated with the test host.
func (th *TestHost) T() *testing.T {
	return th.t
}

// AllocateFreePort chooses a random port for use in tests.
func AllocateFreePort(t *testing.T) int {
	listener, err := net.Listen("tcp", ":0")
	require.NoError(t, err, "failed to allocate port")

	port := listener.Addr().(*net.TCPAddr).Port

	err = listener.Close()
	require.NoError(t, err, "failed to close listener")

	return port
}
