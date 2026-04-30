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

func Test_ExecuteWithRetry_NoRetriesWithoutConfig(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	d := NewDeployExecutor("test.bicep") // no WithRetry

	err := d.executeWithRetry(context.Background(), t, func() error {
		calls.Add(1)
		return errors.New("fails")
	})

	require.Error(t, err)
	assert.Equal(t, int32(1), calls.Load())
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
