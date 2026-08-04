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

func Test_CheckControlPlaneReady_ReadyWhenBothProbesSucceed(t *testing.T) {
	t.Parallel()
	var paths []string
	client := newTestRESTClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		w.WriteHeader(http.StatusOK)
	}))

	require.NoError(t, checkControlPlaneReady(context.Background(), client))
	assert.Equal(t, []string{"/readyz", radiusAggregatedAPIPath}, paths)
}

func Test_CheckControlPlaneReady_NotReadyWhenAPIServerIsUnhealthy(t *testing.T) {
	t.Parallel()
	client := newTestRESTClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))

	err := checkControlPlaneReady(context.Background(), client)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "kube-apiserver is not ready")
}

func Test_CheckControlPlaneReady_NotReadyWhenAggregatedAPIIsUnavailable(t *testing.T) {
	t.Parallel()
	// The kube-apiserver can be healthy while the UCP APIService behind the
	// aggregation layer is still unavailable, which the apiserver reports as a
	// 503. Retrying a deployment in that window cannot succeed.
	client := newTestRESTClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == radiusAggregatedAPIPath {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))

	err := checkControlPlaneReady(context.Background(), client)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "the Radius aggregated API is not reachable")
}

func Test_WaitForControlPlaneReady_ReturnsOnceHealthy(t *testing.T) {
	t.Parallel()
	var requests atomic.Int32
	client := newTestRESTClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Fail the first /readyz probe so the wait loop has to poll at least
		// twice before succeeding.
		if r.URL.Path == "/readyz" && requests.Add(1) == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	require.NoError(t, waitForControlPlaneReady(ctx, t, client))
	assert.Greater(t, requests.Load(), int32(1))
}

func Test_WaitForControlPlaneReady_ReportsLastProbeFailureOnTimeout(t *testing.T) {
	t.Parallel()
	client := newTestRESTClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := waitForControlPlaneReady(ctx, t, client)
	require.Error(t, err)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Contains(t, err.Error(), "kube-apiserver is not ready")
}

func Test_NewControlPlaneReadyWaiter_NilClientDisablesGate(t *testing.T) {
	t.Parallel()
	assert.Nil(t, newControlPlaneReadyWaiter(t, nil))
}
