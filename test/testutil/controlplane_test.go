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

package testutil

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// newTestRESTClient returns a REST client pointed at a test HTTP server that
// serves the given handler.
func newTestRESTClient(t *testing.T, handler http.Handler) rest.Interface {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	client, err := kubernetes.NewForConfig(&rest.Config{Host: server.URL})
	require.NoError(t, err)

	return client.Discovery().RESTClient()
}

func Test_WaitForControlPlaneReady_ReturnsWhenAggregatedAPIServes(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	var paths []string
	client := newTestRESTClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.Path)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))

	require.NoError(t, WaitForControlPlaneReady(t.Context(), t, client, time.Millisecond))

	// A single probe of the aggregated path is enough: it cannot succeed unless
	// the kube-apiserver and its aggregation layer are both serving.
	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, []string{RadiusAggregatedAPIPath}, paths)
}

func Test_WaitForControlPlaneReady_PollsUntilHealthy(t *testing.T) {
	t.Parallel()
	var requests atomic.Int32
	// The kube-apiserver can be up while the UCP APIService behind the
	// aggregation layer is still unavailable, which surfaces as a 503. The gate
	// must keep polling rather than let a caller proceed in that window.
	client := newTestRESTClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requests.Add(1) < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	require.NoError(t, WaitForControlPlaneReady(ctx, t, client, time.Millisecond))
	assert.Equal(t, int32(3), requests.Load())
}

func Test_WaitForControlPlaneReady_ReportsLastProbeFailureOnTimeout(t *testing.T) {
	t.Parallel()
	client := newTestRESTClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))

	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()

	err := WaitForControlPlaneReady(ctx, t, client, time.Millisecond)
	require.Error(t, err)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Contains(t, err.Error(), "last probe failure")
}

// Test_WaitForControlPlaneReady_DoesNotGoexit guards the property that makes this
// helper safe to call from a polling goroutine: it must report failure by returning
// an error, never by calling t.FailNow (directly or through require.*). A helper that
// exits via runtime.Goexit silently kills its caller's goroutine, which is what broke
// the upgrade test's readiness retry loop.
func Test_WaitForControlPlaneReady_DoesNotGoexit(t *testing.T) {
	t.Parallel()
	client := newTestRESTClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))

	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()

	returned := make(chan error, 1)
	go func() {
		// A Goexit would skip this send and leave the channel empty forever.
		defer close(returned)
		err := WaitForControlPlaneReady(ctx, t, client, time.Millisecond)
		returned <- err
	}()

	select {
	case err, ok := <-returned:
		require.True(t, ok, "helper exited via runtime.Goexit instead of returning")
		require.Error(t, err)
	case <-time.After(30 * time.Second):
		t.Fatal("helper never returned")
	}
}
