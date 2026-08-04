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
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/test/radcli"
)

func Test_ExecuteWithRetry_SucceedsOnFirstAttempt(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	d := NewDeployExecutor("test.bicep").WithRetry(time.Second, 10*time.Millisecond, func(error) bool { return true })

	err := d.executeWithRetry(context.Background(), t, func() error {
		calls.Add(1)
		return nil
	})

	require.NoError(t, err)
	assert.Equal(t, int32(1), calls.Load())
}

func Test_ExecuteWithRetry_RetriesOnTransientThenSucceeds(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	transientErr := errors.New("ManagedServiceIdentityNotFound")
	d := NewDeployExecutor("test.bicep").WithRetry(time.Second, 10*time.Millisecond, func(err error) bool {
		return err.Error() == "ManagedServiceIdentityNotFound"
	})

	err := d.executeWithRetry(context.Background(), t, func() error {
		n := calls.Add(1)
		if n == 1 {
			return transientErr
		}
		return nil
	})

	require.NoError(t, err)
	assert.Equal(t, int32(2), calls.Load())
}

func Test_ExecuteWithRetry_DoesNotRetryNonTransientError(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	d := NewDeployExecutor("test.bicep").WithRetry(time.Second, 10*time.Millisecond, func(err error) bool {
		return err.Error() == "transient"
	})

	err := d.executeWithRetry(context.Background(), t, func() error {
		calls.Add(1)
		return errors.New("permanent failure")
	})

	require.Error(t, err)
	assert.Equal(t, "permanent failure", err.Error())
	assert.Equal(t, int32(1), calls.Load())
}

func Test_ExecuteWithRetry_StopsWhenBudgetIsExhausted(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	d := NewDeployExecutor("test.bicep").WithRetry(50*time.Millisecond, 10*time.Millisecond, func(error) bool { return true })

	err := d.executeWithRetry(context.Background(), t, func() error {
		calls.Add(1)
		return errors.New("always fails")
	})

	require.Error(t, err)
	assert.Equal(t, "always fails", err.Error())
	// The initial attempt plus at least one retry, and the loop must have
	// stopped once the budget elapsed rather than run forever.
	assert.Greater(t, calls.Load(), int32(1))
}

func Test_ExecuteWithRetry_DefaultDoesNotRetryNonTransientError(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	// NewDeployExecutor enables transient image pull retries by default, but a
	// non-transient error must not be retried.
	d := NewDeployExecutor("test.bicep")

	err := d.executeWithRetry(context.Background(), t, func() error {
		calls.Add(1)
		return errors.New("fails")
	})

	require.Error(t, err)
	assert.Equal(t, int32(1), calls.Load())
}

func Test_ExecuteWithRetry_DefaultRetriesTransientImagePullError(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	// NewDeployExecutor retries transient image pull failures by default, without
	// an explicit WithRetry call.
	d := NewDeployExecutor("test.bicep")
	d.RetryDelay = 10 * time.Millisecond // shorten the delay for the test

	err := d.executeWithRetry(context.Background(), t, func() error {
		n := calls.Add(1)
		if n < 2 {
			return errors.New("Reason: ErrImagePull, net/http: timeout awaiting response headers")
		}
		return nil
	})

	require.NoError(t, err)
	assert.Equal(t, int32(2), calls.Load()) // failed once, succeeded on retry
}

func Test_ExecuteWithRetry_DefaultRetriesTransientConnectionError(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	// NewDeployExecutor retries transient UCP connection resets by default,
	// which occur when the kind control plane restarts and drops every
	// in-flight connection to the API server.
	d := NewDeployExecutor("test.bicep")
	d.RetryDelay = 10 * time.Millisecond // shorten the delay for the test

	err := d.executeWithRetry(context.Background(), t, func() error {
		n := calls.Add(1)
		if n < 2 {
			return errors.New(`command 'rad deploy' had non-zero exit code: exit status 1
Error: Get "https://127.0.0.1:37481/apis/api.ucp.dev/v1alpha3/.../operationStatuses/...": read tcp 127.0.0.1:38764->127.0.0.1:37481: read: connection reset by peer`)
		}
		return nil
	})

	require.NoError(t, err)
	assert.Equal(t, int32(2), calls.Load()) // failed once, succeeded on retry
}

func Test_ExecuteWithRetry_ContextCancelledDuringDelay(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())
	d := NewDeployExecutor("test.bicep").WithRetry(time.Minute, 5*time.Second, func(error) bool { return true })

	// Cancel context immediately after first deploy attempt
	err := d.executeWithRetry(ctx, t, func() error {
		n := calls.Add(1)
		if n == 1 {
			cancel()
			return errors.New("transient")
		}
		return nil
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, int32(1), calls.Load()) // should not have retried
}

func Test_ExecuteWithRetry_NilShouldRetryDisablesRetries(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	d := NewDeployExecutor("test.bicep")
	d.ShouldRetry = nil // nil predicate

	err := d.executeWithRetry(context.Background(), t, func() error {
		calls.Add(1)
		return errors.New("fails")
	})

	require.Error(t, err)
	assert.Equal(t, int32(1), calls.Load())
}

func Test_ExecuteWithRetry_ZeroBudgetDisablesRetries(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	d := NewDeployExecutor("test.bicep")
	d.RetryDelay = 0
	d.RetryBudget = 0
	d.ShouldRetry = func(error) bool { return true }

	err := d.executeWithRetry(context.Background(), t, func() error {
		calls.Add(1)
		return errors.New("transient")
	})

	require.Error(t, err)
	assert.Equal(t, int32(1), calls.Load())
}

func Test_ExecuteWithRetry_WaitsForControlPlaneBeforeRetrying(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	var waits atomic.Int32

	d := NewDeployExecutor("test.bicep")
	d.RetryDelay = time.Millisecond
	d.ShouldRetry = func(error) bool { return true }
	// The gate stands in for a control plane that is down for the first two
	// probes: the retry must not be attempted until it reports ready.
	d.waitForReady = func(ctx context.Context) error {
		if waits.Add(1) < 3 {
			return errors.New("kube-apiserver is not ready")
		}
		return nil
	}

	err := d.executeWithRetry(context.Background(), t, func() error {
		n := calls.Add(1)
		if n == 1 {
			return errors.New("connection reset by peer")
		}
		return nil
	})

	// A failing gate ends the loop with the deployment error rather than
	// retrying against an unavailable control plane.
	require.Error(t, err)
	assert.Equal(t, int32(1), calls.Load())
	assert.Equal(t, int32(1), waits.Load())
}

func Test_ExecuteWithRetry_RetriesOnceControlPlaneIsReady(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	var waits atomic.Int32

	d := NewDeployExecutor("test.bicep")
	d.RetryDelay = time.Millisecond
	d.ShouldRetry = func(error) bool { return true }
	d.waitForReady = func(ctx context.Context) error {
		waits.Add(1)
		return nil
	}

	err := d.executeWithRetry(context.Background(), t, func() error {
		n := calls.Add(1)
		if n == 1 {
			return errors.New("connection reset by peer")
		}
		return nil
	})

	require.NoError(t, err)
	assert.Equal(t, int32(2), calls.Load())
	assert.Equal(t, int32(1), waits.Load())
}

func Test_ExecuteWithRetry_BudgetBoundsTheReadinessWait(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32

	d := NewDeployExecutor("test.bicep")
	d.RetryDelay = 0
	d.RetryBudget = 50 * time.Millisecond
	d.ShouldRetry = func(error) bool { return true }
	// A control plane that never recovers must not block past the budget.
	d.waitForReady = func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	}

	start := time.Now()
	err := d.executeWithRetry(context.Background(), t, func() error {
		calls.Add(1)
		return errors.New("connection reset by peer")
	})

	require.Error(t, err)
	assert.Equal(t, "connection reset by peer", err.Error())
	assert.Equal(t, int32(1), calls.Load())
	assert.Less(t, time.Since(start), 5*time.Second)
}

func Test_EffectiveRetryBudget_CappedByTestDeadline(t *testing.T) {
	t.Parallel()
	deadline, ok := t.Deadline()
	if !ok {
		t.Skip("go test was run without -timeout, so there is no deadline to cap against")
	}

	// A budget far larger than the time the test binary has left must be cut
	// down, so a broken cluster produces per-test failures rather than an
	// opaque `go test -timeout` panic.
	d := NewDeployExecutor("test.bicep")
	d.RetryBudget = time.Hour

	budget := d.effectiveRetryBudget(t)

	assert.Less(t, budget, time.Until(deadline))
	assert.Positive(t, budget)
}

func Test_EffectiveRetryBudget_UnchangedWhenItFitsTheDeadline(t *testing.T) {
	t.Parallel()
	d := NewDeployExecutor("test.bicep")
	d.RetryBudget = time.Millisecond

	assert.Equal(t, time.Millisecond, d.effectiveRetryBudget(t))
}

func Test_IsTransientImagePullError(t *testing.T) {
	// imagePullError mirrors how rad surfaces a transient image pull failure:
	// the ErrImagePull/timeout cause only appears inside a deeply nested
	// details[].message field, while the top-level code/message returned by
	// CLIError.Error() is the generic "DeploymentFailed". This is the failure
	// observed for Test_RabbitMQ_Manual when ghcr.io is slow to respond.
	imagePullError := &radcli.CLIError{
		ErrorResponse: apiv1.ErrorResponse{
			Error: &apiv1.ErrorDetails{
				Code:    "DeploymentFailed",
				Message: "At least one resource deployment operation failed.",
				Details: []*apiv1.ErrorDetails{
					{Code: "OK"},
					{
						Code:    "ResourceDeploymentFailure",
						Message: "Failed",
						Details: []*apiv1.ErrorDetails{
							{
								Code:    "Internal",
								Message: `Container state is 'Waiting' Reason: ErrImagePull, Message: rpc error: code = DeadlineExceeded desc = failed to pull and unpack image "ghcr.io/radius-project/mirror/rabbitmq:3.10": failed to copy: httpReadSeeker: failed open: failed to do request: Get "https://ghcr.io/v2/radius-project/mirror/rabbitmq/manifests/sha256:0c60": net/http: timeout awaiting response headers`,
							},
						},
					},
				},
			},
		},
	}

	// nonTransientError mirrors a permanent failure (an unsupported resource
	// type) that should not be retried.
	nonTransientError := &radcli.CLIError{
		ErrorResponse: apiv1.ErrorResponse{
			Error: &apiv1.ErrorDetails{
				Code:    "DeploymentFailed",
				Message: "the resource type is not supported",
			},
		},
	}

	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{name: "nil error", err: nil, expected: false},
		{name: "nested ErrImagePull timeout", err: imagePullError, expected: true},
		{name: "plain ImagePullBackOff string", err: errors.New("pod is stuck in ImagePullBackOff"), expected: true},
		{name: "registry response header timeout", err: errors.New("failed to do request: net/http: timeout awaiting response headers"), expected: true},
		{name: "containerd pull-and-unpack failure", err: errors.New(`failed to pull and unpack image "ghcr.io/radius-project/mirror/rabbitmq:3.10"`), expected: true},
		{name: "non-transient CLIError", err: nonTransientError, expected: false},
		{name: "unrelated error", err: errors.New("connection refused"), expected: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.expected, IsTransientImagePullError(tc.err))
		})
	}
}

func Test_IsTransientConnectionError(t *testing.T) {
	// nonTransientError mirrors a permanent structured deployment failure, which
	// must not be treated as a transient connection reset.
	nonTransientError := &radcli.CLIError{
		ErrorResponse: apiv1.ErrorResponse{
			Error: &apiv1.ErrorDetails{
				Code:    "DeploymentFailed",
				Message: "the resource type is not supported",
			},
		},
	}

	// markerBearingStructuredError mirrors a genuine (non-transient) structured
	// ARM deployment failure whose flattened message happens to contain a
	// connection marker. The concrete-type guard must keep this from being
	// misclassified as a retryable transport failure.
	markerBearingStructuredError := &radcli.CLIError{
		ErrorResponse: apiv1.ErrorResponse{
			Error: &apiv1.ErrorDetails{
				Code:    "DeploymentFailed",
				Message: "recipe execution failed: unexpected EOF while parsing module output",
			},
		},
	}

	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{name: "nil error", err: nil, expected: false},
		{
			name:     "apiserver connection reset",
			err:      errors.New(`Get "https://127.0.0.1:37481/.../operationStatuses/...": read tcp 127.0.0.1:38764->127.0.0.1:37481: read: connection reset by peer`),
			expected: true,
		},
		{
			name:     "clean EOF mid-response",
			err:      errors.New(`Get "https://127.0.0.1:37481/.../deployments/rad-deploy-...": EOF`),
			expected: true,
		},
		{name: "log-stream unexpected EOF", err: errors.New("error streaming logs: unexpected EOF"), expected: true},
		{name: "broken pipe on write", err: errors.New("write tcp 127.0.0.1:38764->127.0.0.1:37481: write: broken pipe"), expected: true},
		{name: "non-transient structured failure", err: nonTransientError, expected: false},
		{name: "structured failure containing a marker", err: markerBearingStructuredError, expected: false},
		{name: "unrelated error", err: errors.New("connection refused"), expected: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.expected, IsTransientConnectionError(tc.err))
		})
	}
}

func Test_IsTransientDeployError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{name: "nil error", err: nil, expected: false},
		{name: "transient image pull", err: errors.New("pod is stuck in ImagePullBackOff"), expected: true},
		{name: "transient connection reset", err: errors.New("read: connection reset by peer"), expected: true},
		{name: "non-transient failure", err: errors.New("the resource type is not supported"), expected: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.expected, IsTransientDeployError(tc.err))
		})
	}
}
