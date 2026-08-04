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
	"fmt"
	"testing"
	"time"

	k8s "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const (
	// controlPlaneReadyPollInterval is how long to wait between readiness
	// probes while the control plane is recovering.
	controlPlaneReadyPollInterval = 5 * time.Second

	// controlPlaneProbeTimeout bounds a single readiness probe so a hung
	// kube-apiserver cannot consume the caller's entire retry budget in one
	// request.
	controlPlaneProbeTimeout = 10 * time.Second

	// radiusAggregatedAPIPath is the aggregated API discovery path that rad
	// itself requests when it opens a workspace connection (see
	// pkg/cli/workspaces.Connection). Probing the same path exercises the whole
	// chain the deployment depends on: the kube-apiserver, its aggregation
	// layer, and the UCP APIService behind it.
	radiusAggregatedAPIPath = "/apis/api.ucp.dev/v1alpha3"
)

// newControlPlaneReadyWaiter returns a function that blocks until the
// Kubernetes control plane and the Radius aggregated API are both serving
// again, or until ctx is done.
//
// It exists because a fixed sleep is not a useful gate between deployment
// retries. When the kind control plane goes down, every queued retry fails
// instantly against a dead socket and burns the retry budget in seconds while
// the outage lasts minutes. Waiting for readiness instead makes each retry
// attempt meaningful.
//
// It returns nil when no client is available, which disables the gate and
// leaves the retry loop with its delay-only behavior.
func newControlPlaneReadyWaiter(t *testing.T, client k8s.Interface) func(context.Context) error {
	if client == nil {
		return nil
	}

	restClient := client.Discovery().RESTClient()
	if restClient == nil {
		return nil
	}

	return func(ctx context.Context) error {
		return waitForControlPlaneReady(ctx, t, restClient)
	}
}

// waitForControlPlaneReady polls until the control plane is ready or ctx is
// done. The error returned on timeout includes the last probe failure so the
// test log records which component was still unavailable.
func waitForControlPlaneReady(ctx context.Context, t *testing.T, client rest.Interface) error {
	var lastErr error
	for {
		lastErr = checkControlPlaneReady(ctx, client)
		if lastErr == nil {
			return nil
		}

		t.Logf("waiting for the Radius control plane to become ready: %v", lastErr)

		timer := time.NewTimer(controlPlaneReadyPollInterval)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return fmt.Errorf("control plane did not become ready: %w (last probe failure: %v)", ctx.Err(), lastErr)
		}
	}
}

// checkControlPlaneReady performs a single readiness probe.
func checkControlPlaneReady(ctx context.Context, client rest.Interface) error {
	// /readyz reports whether the kube-apiserver has finished starting and its
	// health checks - including the etcd backend check - are passing. It is
	// served to any authenticated caller by the built-in
	// system:public-info-viewer role, so it needs no extra RBAC.
	if err := probe(ctx, client, "/readyz"); err != nil {
		return fmt.Errorf("kube-apiserver is not ready: %w", err)
	}

	// The kube-apiserver can be ready before the aggregated API it proxies to
	// is, and a request to an unavailable APIService fails with 503. Probing it
	// avoids retrying a deployment that is guaranteed to fail.
	if err := probe(ctx, client, radiusAggregatedAPIPath); err != nil {
		return fmt.Errorf("the Radius aggregated API is not reachable: %w", err)
	}

	return nil
}

func probe(ctx context.Context, client rest.Interface, path string) error {
	probeCtx, cancel := context.WithTimeout(ctx, controlPlaneProbeTimeout)
	defer cancel()

	return client.Get().AbsPath(path).Do(probeCtx).Error()
}
