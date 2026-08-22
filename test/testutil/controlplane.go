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
	"fmt"
	"testing"
	"time"

	"k8s.io/client-go/rest"
)

const (
	// RadiusAggregatedAPIPath is the aggregated API discovery path that rad itself
	// requests when it opens a workspace connection (see pkg/cli/workspaces.Connection).
	// Probing it exercises the whole chain a deployment depends on: the kube-apiserver,
	// its aggregation layer, and the UCP APIService behind it. A healthy response here
	// implies the kube-apiserver is serving, so no separate /readyz probe is needed.
	RadiusAggregatedAPIPath = "/apis/api.ucp.dev/v1alpha3"

	// ControlPlaneProbeTimeout bounds a single readiness probe so a hung kube-apiserver
	// cannot consume the caller's entire retry budget in one request.
	ControlPlaneProbeTimeout = 10 * time.Second
)

// WaitForControlPlaneReady polls until the Radius aggregated API is serving, or until
// ctx is done. The error returned on timeout includes the last probe failure so the
// test log records what was still unavailable.
//
// A transient 503 right after install is normal: the APIService Available condition is
// owned by the kube-apiserver aggregator, which re-checks the backend on its own
// schedule and can keep reporting a stale unavailable result for several seconds after
// UCP is Ready and serving. Callers must treat that window as retryable.
//
// This reports readiness by returning an error rather than by asserting, so it is safe
// to call from any goroutine. Do not reintroduce require.* assertions here or in a
// caller's polling callback: require.* calls t.FailNow, which calls runtime.Goexit.
// Goexit is not a panic, so recover() cannot observe it, and a goroutine that exits
// that way never reports back to testify's Eventually loop — which then re-arms its
// ticker never, makes exactly one attempt, and blocks until its timeout expires.
func WaitForControlPlaneReady(ctx context.Context, t *testing.T, client rest.Interface, pollInterval time.Duration) error {
	for {
		lastErr := ProbeControlPlane(ctx, client)
		if lastErr == nil {
			return nil
		}

		t.Logf("waiting for the Radius control plane to become ready: %v", lastErr)

		timer := time.NewTimer(pollInterval)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return fmt.Errorf("control plane did not become ready: %w (last probe failure: %v)", ctx.Err(), lastErr)
		}
	}
}

// ProbeControlPlane issues a single bounded request against the Radius aggregated API
// discovery path and reports whether it is serving.
func ProbeControlPlane(ctx context.Context, client rest.Interface) error {
	probeCtx, cancel := context.WithTimeout(ctx, ControlPlaneProbeTimeout)
	defer cancel()

	return client.Get().AbsPath(RadiusAggregatedAPIPath).Do(probeCtx).Error()
}
