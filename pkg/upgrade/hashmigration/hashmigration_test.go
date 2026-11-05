/*
Copyright 2026 The Radius Authors.

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

package hashmigration

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

type recordingReporter struct {
	mu       sync.Mutex
	statuses map[string][]Status
	err      error

	// failures is the number of reports that fail before reports succeed.
	failures int
}

func (r *recordingReporter) Report(_ context.Context, key string, status Status) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.statuses == nil {
		r.statuses = map[string][]Status{}
	}
	r.statuses[key] = append(r.statuses[key], status)
	if r.failures > 0 {
		r.failures--
		return errors.New("the ConfigMap is unavailable")
	}
	return r.err
}

func (r *recordingReporter) last(key string) Status {
	r.mu.Lock()
	defer r.mu.Unlock()
	values := r.statuses[key]
	return values[len(values)-1]
}

func Test_Service_RunPass_States(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name  string
		step  StepFunc
		state State
		done  bool
	}{
		{name: "complete", step: StepFunc{StepName: "s", Fn: func(context.Context) (Result, error) { return Result{Migrated: 2, Conflicts: 1, Skipped: 1}, nil }}, state: StateComplete, done: true},
		{name: "not applicable", step: StepFunc{StepName: "s", Fn: func(context.Context) (Result, error) { return Result{NotApplicable: true}, nil }}, state: StateNotApplicable, done: true},
		{name: "remaining", step: StepFunc{StepName: "s", Fn: func(context.Context) (Result, error) { return Result{Migrated: 1, Remaining: 3}, nil }}, state: StateInProgress},
		{name: "error", step: StepFunc{StepName: "s", Fn: func(context.Context) (Result, error) { return Result{Migrated: 1}, errors.New("boom") }}, state: StateFailed},
		{name: "panic", step: StepFunc{StepName: "s", Fn: func(context.Context) (Result, error) { panic("bad") }}, state: StateFailed},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			reporter := &recordingReporter{}
			service := NewService(Options{Component: "test", Steps: []Step{tt.step}, Reporter: reporter})
			service.now = func() time.Time { return time.Date(2026, 11, 3, 1, 2, 3, 0, time.FixedZone("x", 3600)) }

			require.Equal(t, tt.done, service.RunPass(t.Context()))
			status := reporter.last(StatusKey("test", "s"))
			require.Equal(t, tt.state, status.State)
			require.Equal(t, tt.done, status.Done())
			require.Equal(t, time.Date(2026, 11, 3, 0, 2, 3, 0, time.UTC), status.Updated)
			require.NotEmpty(t, status.Version)
			if tt.state == StateFailed {
				require.NotEmpty(t, status.Error)
			} else {
				require.Empty(t, status.Error)
			}
		})
	}
}

func Test_Service_RunPass_ContinuesAfterFailures(t *testing.T) {
	t.Parallel()
	reporter := &recordingReporter{err: errors.New("unavailable")}
	ran := []string{}
	service := NewService(Options{Component: "c", Reporter: reporter, Steps: []Step{
		StepFunc{StepName: "first", Fn: func(context.Context) (Result, error) { ran = append(ran, "first"); return Result{}, errors.New("x") }},
		StepFunc{StepName: "second", Fn: func(context.Context) (Result, error) { ran = append(ran, "second"); return Result{}, nil }},
	}})

	require.False(t, service.RunPass(t.Context()))
	require.Equal(t, []string{"first", "second"}, ran, "a failed step or report must not stop later steps")
	require.Equal(t, StateComplete, reporter.last("c.second").State)
}

func Test_Service_Run_RetriesUntilDone(t *testing.T) {
	t.Parallel()
	attempts := 0
	reporter := &recordingReporter{}
	service := NewService(Options{Component: "c", Reporter: reporter, RetryInterval: time.Millisecond, MaxRetryInterval: 2 * time.Millisecond,
		Steps: []Step{StepFunc{StepName: "s", Fn: func(context.Context) (Result, error) {
			attempts++
			if attempts < 3 {
				return Result{Remaining: 1}, nil
			}
			return Result{Migrated: 1}, nil
		}}}})

	require.NoError(t, service.Run(t.Context()))
	require.Equal(t, 3, attempts)
	require.Equal(t, StateComplete, reporter.last("c.s").State)
	require.Len(t, reporter.statuses["c.s"], 3)
}

func Test_Service_RunPass_AccumulatesMigratedValues(t *testing.T) {
	t.Parallel()
	results := []Result{
		{Migrated: 2, Deduplicated: 1, Remaining: 3, Conflicts: 1, Message: "busy"},
		{Migrated: 3, Skipped: 1},
	}
	reporter := &recordingReporter{}
	service := NewService(Options{Component: "c", Reporter: reporter, Steps: []Step{StepFunc{StepName: "s", Fn: func(context.Context) (Result, error) {
		result := results[0]
		results = results[1:]
		return result, nil
	}}}})

	require.False(t, service.RunPass(t.Context()))
	require.True(t, service.RunPass(t.Context()))

	// Migrated values accumulate across passes; the remaining work, conflicts, and skipped values are those of the last pass.
	status := reporter.last("c.s")
	require.Equal(t, StateComplete, status.State)
	require.Equal(t, Result{Migrated: 5, Deduplicated: 1, Skipped: 1}, status.Result)
}

func Test_Service_RunPass_RecordsFinalStatus(t *testing.T) {
	t.Parallel()
	runs := map[string]int{}
	reporter := &recordingReporter{failures: 1}
	service := NewService(Options{Component: "c", Reporter: reporter, Steps: []Step{
		StepFunc{StepName: "done", Fn: func(context.Context) (Result, error) { runs["done"]++; return Result{Migrated: 1}, nil }},
		StepFunc{StepName: "other", Fn: func(context.Context) (Result, error) { runs["other"]++; return Result{}, nil }},
	}})

	// The final status of the first step is not recorded, so the migration is not complete yet.
	require.False(t, service.RunPass(t.Context()))
	require.Equal(t, map[string]int{"done": 1, "other": 1}, runs)

	// The next pass records the status without running the steps again.
	require.True(t, service.RunPass(t.Context()))
	require.Equal(t, map[string]int{"done": 1, "other": 1}, runs, "a step that is done must not run again")
	require.Len(t, reporter.statuses["c.done"], 2)
	require.Len(t, reporter.statuses["c.other"], 1)
	status := reporter.last("c.done")
	require.Equal(t, StateComplete, status.State)
	require.Equal(t, 1, status.Migrated)

	require.True(t, service.RunPass(t.Context()))
	require.Len(t, reporter.statuses["c.done"], 2, "a recorded final status is not recorded again")
}

func Test_Service_Run_RetriesUntilStatusIsRecorded(t *testing.T) {
	t.Parallel()
	reporter := &recordingReporter{failures: 2}
	service := NewService(Options{Component: "c", Reporter: reporter, RetryInterval: time.Millisecond, MaxRetryInterval: 2 * time.Millisecond,
		Steps: []Step{StepFunc{StepName: "s", Fn: func(context.Context) (Result, error) { return Result{Migrated: 1}, nil }}}})

	require.NoError(t, service.Run(t.Context()))
	require.Len(t, reporter.statuses["c.s"], 3)
	require.Equal(t, StateComplete, reporter.last("c.s").State)
}

// funcExclusion is an Exclusion that runs fn with the context it is given, or fails.
type funcExclusion struct {
	err  error
	runs int
}

func (e *funcExclusion) Run(ctx context.Context, fn func(context.Context)) error {
	e.runs++
	if e.err != nil {
		return e.err
	}
	fn(ctx)
	return nil
}

func Test_Service_Exclusion(t *testing.T) {
	t.Parallel()

	newService := func(exclusion Exclusion, observed *[]bool) *Service {
		return NewService(Options{Component: "c", Exclusion: exclusion, RetryInterval: time.Millisecond, MaxRetryInterval: time.Millisecond,
			Steps: []Step{StepFunc{StepName: "s", Fn: func(ctx context.Context) (Result, error) {
				*observed = append(*observed, IsExclusive(ctx))
				return Result{}, nil
			}}}})
	}

	t.Run("steps run exclusively", func(t *testing.T) {
		t.Parallel()
		observed := []bool{}
		exclusion := &funcExclusion{}
		require.NoError(t, newService(exclusion, &observed).Run(t.Context()))
		require.Equal(t, []bool{true}, observed)
		require.Equal(t, 1, exclusion.runs)
	})

	t.Run("without exclusion", func(t *testing.T) {
		t.Parallel()
		observed := []bool{}
		require.NoError(t, newService(nil, &observed).Run(t.Context()))
		require.Equal(t, []bool{false}, observed)
	})

	t.Run("exclusion fails", func(t *testing.T) {
		t.Parallel()
		observed := []bool{}
		exclusion := &funcExclusion{err: errors.New("invalid configuration")}
		ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
		defer cancel()
		require.ErrorIs(t, newService(exclusion, &observed).Run(ctx), context.DeadlineExceeded)
		require.Empty(t, observed, "steps must not run without the exclusion")
		require.Greater(t, exclusion.runs, 1, "the exclusion is retried")
	})
}

func Test_Service_Run_StopsOnCancellation(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	service := NewService(Options{Component: "c", RetryInterval: time.Hour, Steps: []Step{StepFunc{StepName: "s", Fn: func(context.Context) (Result, error) {
		cancel()
		return Result{Remaining: 1}, nil
	}}}})

	require.ErrorIs(t, service.Run(ctx), context.Canceled)

	delayed := NewService(Options{Component: "c", InitialDelay: time.Hour})
	cancelled, stop := context.WithCancel(t.Context())
	stop()
	require.ErrorIs(t, delayed.Run(cancelled), context.Canceled)
	require.Equal(t, "c sha256 migration", delayed.Name())
}

func Test_ConfigMapReporter(t *testing.T) {
	t.Parallel()
	client := fake.NewClientset()
	reporter := NewConfigMapReporter(client, "radius-system")
	ctx := t.Context()

	status, err := ReadStatus(ctx, client, "radius-system")
	require.NoError(t, err)
	require.Empty(t, status)

	first := Status{Result: Result{Migrated: 3, Conflicts: 1}, State: StateComplete, Version: "edge", Updated: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)}
	require.NoError(t, reporter.Report(ctx, "ucp.datastore", first))
	second := Status{Result: Result{Remaining: 2}, State: StateInProgress, Version: "edge", Updated: first.Updated}
	require.NoError(t, reporter.Report(ctx, "applications-rp.terraformState", second))

	configMap, err := client.CoreV1().ConfigMaps("radius-system").Get(ctx, ConfigMapName, metav1.GetOptions{})
	require.NoError(t, err)
	require.Equal(t, "radius", configMap.Labels["app.kubernetes.io/part-of"])
	require.JSONEq(t, `{"migrated":3,"conflicts":1,"remaining":0,"state":"Complete","version":"edge","updated":"2026-01-02T03:04:05Z"}`, configMap.Data["ucp.datastore"])

	status, err = ReadStatus(ctx, client, "radius-system")
	require.NoError(t, err)
	require.Equal(t, map[string]Status{"ucp.datastore": first, "applications-rp.terraformState": second}, status)

	first.State = StateInProgress
	require.NoError(t, reporter.Report(ctx, "ucp.datastore", first))
	status, err = ReadStatus(ctx, client, "radius-system")
	require.NoError(t, err)
	require.Equal(t, StateInProgress, status["ucp.datastore"].State)
	require.Equal(t, second, status["applications-rp.terraformState"], "a component must not overwrite another component's key")
}

func Test_ReadStatus_Invalid(t *testing.T) {
	t.Parallel()
	client := fake.NewClientset(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: ConfigMapName, Namespace: "radius-system"},
		Data:       map[string]string{"ucp.datastore": "not json"},
	})

	_, err := ReadStatus(t.Context(), client, "radius-system")
	require.ErrorContains(t, err, `invalid status "ucp.datastore"`)
}

func Test_Result_Add(t *testing.T) {
	t.Parallel()
	result := Result{Migrated: 1, Deduplicated: 2, Remaining: 3, Conflicts: 4, Skipped: 5}
	result.Add(Result{Migrated: 10, Deduplicated: 20, Remaining: 30, Conflicts: 40, Skipped: 50, NotApplicable: true, Message: "ignored"})
	require.Equal(t, Result{Migrated: 11, Deduplicated: 22, Remaining: 33, Conflicts: 44, Skipped: 55}, result)
}
