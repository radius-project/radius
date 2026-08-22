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

package step

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/radius-project/radius/test/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// The polling, timeout, and last-failure reporting behavior of the readiness gate is
// covered by testutil.WaitForControlPlaneReady's own tests. These tests cover the
// wiring in this package: which client the gate probes and when it is disabled.

// newTestKubernetesClient returns a Kubernetes client pointed at a test HTTP server
// that serves the given handler.
func newTestKubernetesClient(t *testing.T, handler http.Handler) kubernetes.Interface {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	client, err := kubernetes.NewForConfig(&rest.Config{Host: server.URL})
	require.NoError(t, err)

	return client
}

func Test_NewControlPlaneReadyWaiter_ProbesAggregatedAPI(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	var paths []string
	client := newTestKubernetesClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.Path)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))

	waiter := newControlPlaneReadyWaiter(t, client)
	require.NotNil(t, waiter)

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	require.NoError(t, waiter(ctx))

	// A single probe of the aggregated path is enough: it cannot succeed unless
	// the kube-apiserver and its aggregation layer are both serving.
	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, []string{testutil.RadiusAggregatedAPIPath}, paths)
}

func Test_NewControlPlaneReadyWaiter_ReturnsErrorWhenAggregatedAPIStaysUnavailable(t *testing.T) {
	t.Parallel()
	// The kube-apiserver can be up while the UCP APIService behind the aggregation
	// layer is still unavailable, which surfaces as a 503. The gate must report that
	// as an error rather than let a retry proceed.
	client := newTestKubernetesClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))

	waiter := newControlPlaneReadyWaiter(t, client)
	require.NotNil(t, waiter)

	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()

	err := waiter(ctx)
	require.Error(t, err)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Contains(t, err.Error(), "last probe failure")
}

func Test_NewControlPlaneReadyWaiter_NilClientDisablesGate(t *testing.T) {
	t.Parallel()
	assert.Nil(t, newControlPlaneReadyWaiter(t, nil))
}
