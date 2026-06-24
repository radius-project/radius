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
	d := NewDeployExecutor("test.bicep").WithRetry(2, 10*time.Millisecond, func(error) bool { return true })

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
	d := NewDeployExecutor("test.bicep").WithRetry(2, 10*time.Millisecond, func(err error) bool {
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
	d := NewDeployExecutor("test.bicep").WithRetry(2, 10*time.Millisecond, func(err error) bool {
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

func Test_ExecuteWithRetry_ExhaustsAllRetries(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	d := NewDeployExecutor("test.bicep").WithRetry(2, 10*time.Millisecond, func(error) bool { return true })

	err := d.executeWithRetry(context.Background(), t, func() error {
		calls.Add(1)
		return errors.New("always fails")
	})

	require.Error(t, err)
	assert.Equal(t, "always fails", err.Error())
	assert.Equal(t, int32(3), calls.Load()) // 1 initial + 2 retries
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

func Test_ExecuteWithRetry_ContextCancelledDuringDelay(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())
	d := NewDeployExecutor("test.bicep").WithRetry(2, 5*time.Second, func(error) bool { return true })

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
	d.MaxRetries = 3
	d.ShouldRetry = nil // nil predicate

	err := d.executeWithRetry(context.Background(), t, func() error {
		calls.Add(1)
		return errors.New("fails")
	})

	require.Error(t, err)
	assert.Equal(t, int32(1), calls.Load())
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
