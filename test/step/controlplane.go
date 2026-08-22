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
	"testing"
	"time"

	"github.com/radius-project/radius/test/testutil"
	k8s "k8s.io/client-go/kubernetes"
)

// controlPlaneReadyPollInterval is how long to wait between readiness probes
// while the control plane is recovering.
const controlPlaneReadyPollInterval = 5 * time.Second

// newControlPlaneReadyWaiter returns a function that blocks until the Radius
// aggregated API is serving again, or until ctx is done.
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
		return testutil.WaitForControlPlaneReady(ctx, t, restClient, controlPlaneReadyPollInterval)
	}
}
