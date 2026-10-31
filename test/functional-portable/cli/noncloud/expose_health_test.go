/*
Copyright 2026 The Radius Authors.

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

package resource_test

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func Test_GetHealthEndpointOnLocalPort_HungServer(t *testing.T) {
	t.Parallel()

	// A server that accepts the request and never answers.
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(server.Close)
	t.Cleanup(func() { close(release) })
	port := server.Listener.Addr().(*net.TCPAddr).Port

	result := make(chan error, 1)
	go func() {
		result <- getHealthEndpointOnLocalPort(t.Context(), t, 0, port, 100*time.Millisecond)
	}()

	select {
	case err := <-result:
		require.Error(t, err)
	case <-time.After(10 * time.Second):
		require.Fail(t, "health check did not time out on a server that never answers")
	}
}

func Test_ExposeAndCheckHealth_RetriesTakenPort(t *testing.T) {
	t.Parallel()

	ports := []int{1001, 1002}
	var tried []int
	getPort := func() (int, error) {
		port := ports[len(tried)]
		tried = append(tried, port)
		return port, nil
	}
	expose := func(ctx context.Context, port int) (string, error) {
		if port == 1001 {
			return "Error: failed to port-forward: unable to listen on any of the requested ports: [{1001 3000}]", context.DeadlineExceeded
		}
		<-ctx.Done()
		return "", context.Canceled
	}
	checkHealth := func(ctx context.Context, port int) error {
		if port == 1001 {
			// Nothing answers on a port rad could not listen on.
			<-ctx.Done()
			return ctx.Err()
		}
		return nil
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		exposeAndCheckHealth(t.Context(), t, exposeAttempts, getPort, expose, checkHealth)
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		require.Fail(t, "health check kept waiting after rad exited")
	}
	require.Equal(t, []int{1001, 1002}, tried)
}
