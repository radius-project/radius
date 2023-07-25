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

package hosting

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/project-radius/radius/test/testcontext"
	"github.com/stretchr/testify/require"
)

const TestTimeout = time.Minute * 5

type testValue struct{}

type testResult struct {
	Value *testValue
	Err   error
}

// Used to synchronize the test "workers" for our tests.
type synchronizer struct {
	Value *AsyncValue[testValue]

	WorkerCount      int
	workersStarted   *sync.WaitGroup
	workersCompleted *sync.WaitGroup

	Done    chan error
	results chan testResult
}

// # Function Explanation
//
// NewSynchronizer creates a new synchronizer struct with a given worker count, initializes the waitgroups and results
// channel, and returns the synchronizer.
func NewSynchronizer(workerCount int) *synchronizer {
	s := &synchronizer{
		Value:            NewAsyncValue[testValue](),
		WorkerCount:      workerCount,
		workersStarted:   &sync.WaitGroup{},
		workersCompleted: &sync.WaitGroup{},
		results:          make(chan testResult, workerCount),
	}

	s.workersStarted.Add(workerCount)
	s.workersCompleted.Add(workerCount)

	return s
}

// # Function Explanation
//
// Start launches a number of workers to get a value from a context and returns a test result with the value retrieved
// and any errors that occurred. If the test times out before the workers complete, an error is returned.
func (s *synchronizer) Start(ctx context.Context, t *testing.T) {
	for i := 0; i < s.WorkerCount; i++ {
		go func() {
			s.workersStarted.Done()

			value, err := s.Value.Get(ctx)
			s.results <- testResult{Value: value, Err: err}
			s.workersCompleted.Done()
		}()
	}

	started := make(chan struct{})

	go func() {
		s.workersStarted.Wait()
		started <- struct{}{}
		close(started)
	}()

	select {
	case <-started:
		return
	case <-ctx.Done():
		require.Fail(t, "test timed out without completing")
		return
	}
}

// # Function Explanation
//
// WaitForWorkersCompleted creates a channel to wait for workers to complete and returns a channel of test
// results or an error if the test times out.
func (s *synchronizer) WaitForWorkersCompleted(ctx context.Context, t *testing.T) <-chan testResult {
	completed := make(chan struct{})

	go func() {
		s.workersCompleted.Wait()
		completed <- struct{}{}
		close(completed)

		// TRICKY: this is the best place to close the results channel.
		close(s.results)
	}()

	select {
	case <-completed:
		return s.results
	case <-ctx.Done():
		require.Fail(t, "test timed out without completing")
		return nil
	}
}

func Test_Get_NoBlockingWhenValueSet_Value(t *testing.T) {
	ctx, cancel := testcontext.NewWithDeadline(t, TestTimeout)
	t.Cleanup(cancel)

	asyncValue := NewAsyncValue[testValue]()

	value := &testValue{}
	asyncValue.Put(value)

	got, err := asyncValue.Get(ctx)
	require.Equal(t, value, got)
	require.NoError(t, err)
}

func Test_Get_NoBlockingWhenValueSet_Err(t *testing.T) {
	ctx, cancel := testcontext.NewWithDeadline(t, TestTimeout)
	t.Cleanup(cancel)

	asyncValue := NewAsyncValue[testValue]()

	err := errors.New("OH noes...")
	asyncValue.PutErr(err)

	got, goterr := asyncValue.Get(ctx)
	require.Nil(t, got)
	require.ErrorIs(t, err, goterr)
}

func Test_Get_BlocksUntil_ValueSet(t *testing.T) {
	ctx, cancel := testcontext.NewWithDeadline(t, TestTimeout)
	t.Cleanup(cancel)

	s := NewSynchronizer(10)
	s.Start(ctx, t)

	value := &testValue{}
	s.Value.Put(value)

	results := s.WaitForWorkersCompleted(ctx, t)

	// Verify results
	count := 0
	for result := range results {
		count++
		require.Equal(t, testResult{Value: value}, result)
	}
	require.Equal(t, s.WorkerCount, count)
}

func Test_Get_BlocksUntil_ErrSet(t *testing.T) {
	ctx, cancel := testcontext.NewWithDeadline(t, TestTimeout)
	t.Cleanup(cancel)

	s := NewSynchronizer(10)
	s.Start(ctx, t)

	err := errors.New("OH noes...")
	s.Value.PutErr(err)

	results := s.WaitForWorkersCompleted(ctx, t)

	// Verify results
	count := 0
	for result := range results {
		count++
		require.Equal(t, testResult{Err: err}, result)
	}
	require.Equal(t, s.WorkerCount, count)
}

func Test_Get_BlocksUntil_Canceled(t *testing.T) {
	// We need two contexts. We want to cancel the work done by the workers.
	workerContext, workerCancel := testcontext.NewWithCancel(t)
	ctx, cancel := testcontext.NewWithDeadline(t, TestTimeout)
	t.Cleanup(cancel)

	s := NewSynchronizer(10)
	s.Start(workerContext, t)

	workerCancel()

	results := s.WaitForWorkersCompleted(ctx, t)

	// Verify results
	count := 0
	for result := range results {
		count++
		require.Error(t, result.Err) // Workers see a wrapped error, not the exact error from the context.
	}
	require.Equal(t, s.WorkerCount, count)
}
