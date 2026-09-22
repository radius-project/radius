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

package resource_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
	utilexec "k8s.io/client-go/util/exec"
)

func redisTestPod() *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "redis-pod", Namespace: "test",
			Labels: map[string]string{"app": "redis", "radapp.io/application": "test-app", "resource": "redis-service"},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{Name: "redis", State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}},
				{Name: "redis-monitor", State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{ExitCode: 1}}},
			},
		},
	}
}

func TestWaitForRedis(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name        string
		mutate      func(*corev1.Pod)
		output      string
		pingErr     error
		wantErr     string
		wantPing    bool
		wantTimeout bool
	}{
		{name: "PONG despite monitor failure", output: "PONG\n", wantPing: true},
		{name: "Running alone is insufficient", output: "", wantErr: `PING response ""`, wantPing: true, wantTimeout: true},
		{name: "Redis error response is not readiness", output: "NOAUTH Authentication required.", wantErr: "NOAUTH", wantPing: true, wantTimeout: true},
		{name: "connection refused", pingErr: utilexec.CodeExitError{Err: errors.New("connection refused"), Code: 1}, wantErr: "connection refused", wantPing: true, wantTimeout: true},
		{name: "exec forbidden", pingErr: apierrors.NewForbidden(schema.GroupResource{Resource: "pods"}, "redis-pod", errors.New("exec forbidden")), wantErr: "exec forbidden", wantPing: true},
		{name: "missing redis-cli", pingErr: utilexec.CodeExitError{Err: errors.New("command not found"), Code: 127}, wantErr: "command not found", wantPing: true},
		{name: "PONG with execution error is rejected", output: "PONG", pingErr: errors.New("stream failed"), wantErr: "stream failed", wantPing: true},
		{name: "Redis not started", mutate: func(p *corev1.Pod) { p.Status.ContainerStatuses = nil }, wantErr: "has not started", wantTimeout: true},
		{name: "failed pod", mutate: func(p *corev1.Pod) { p.Status.Phase = corev1.PodFailed }, wantErr: "phase Failed"},
		{name: "terminating pod", mutate: func(p *corev1.Pod) { now := metav1.Now(); p.DeletionTimestamp = &now }, wantErr: "terminating"},
		{name: "wrong application", mutate: func(p *corev1.Pod) { p.Labels["radapp.io/application"] = "other" }, wantErr: "no Redis pod found", wantTimeout: true},
		{name: "wrong namespace", mutate: func(p *corev1.Pod) { p.Namespace = "other" }, wantErr: "no Redis pod found", wantTimeout: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			pod := redisTestPod()
			if tt.mutate != nil {
				tt.mutate(pod)
			}
			client := fake.NewClientset(pod)
			ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
			defer cancel()
			calls := 0
			err := waitForRedis(ctx, client, "test", "test-app", func(ctx context.Context, selected corev1.Pod) (string, error) {
				calls++
				require.Equal(t, pod.Name, selected.Name)
				deadline, ok := ctx.Deadline()
				require.True(t, ok)
				require.LessOrEqual(t, time.Until(deadline), 5*time.Second)
				return tt.output, tt.pingErr
			})
			if tt.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tt.wantErr)
			}
			if tt.wantTimeout {
				require.ErrorIs(t, err, context.DeadlineExceeded)
			}
			require.Equal(t, tt.wantPing, calls > 0)
		})
	}
}

func TestWaitForRedisRetriesUntilPong(t *testing.T) {
	t.Parallel()
	client := fake.NewClientset(redisTestPod())
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	calls := 0
	err := waitForRedis(ctx, client, "test", "test-app", func(context.Context, corev1.Pod) (string, error) {
		calls++
		if calls == 1 {
			return "", utilexec.CodeExitError{Err: errors.New("connection refused"), Code: 1}
		}
		return "PONG", nil
	})
	require.NoError(t, err)
	require.Equal(t, 2, calls)
}

func TestWaitForRedisRejectsAmbiguousPods(t *testing.T) {
	t.Parallel()
	first, second := redisTestPod(), redisTestPod()
	second.Name = "another-redis"
	err := waitForRedis(t.Context(), fake.NewClientset(first, second), "test", "test-app", func(context.Context, corev1.Pod) (string, error) {
		t.Fatal("must not select an arbitrary Redis pod")
		return "", nil
	})
	require.ErrorContains(t, err, "expected one Redis pod, found 2")
}

func TestWaitForRedisListError(t *testing.T) {
	t.Parallel()
	client := fake.NewClientset()
	listErr := errors.New("API unavailable")
	client.PrependReactor("list", "pods", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, listErr
	})
	err := waitForRedis(t.Context(), client, "test", "test-app", nil)
	require.ErrorIs(t, err, listErr)
}

func TestWaitForRedisCanceled(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	client := fake.NewClientset(redisTestPod())
	err := waitForRedis(ctx, client, "test", "test-app", nil)
	require.ErrorIs(t, err, context.Canceled)
	require.Empty(t, client.Actions())
}

func TestWaitForRedisCancelsInFlightPing(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	called := false
	err := waitForRedis(ctx, fake.NewClientset(redisTestPod()), "test", "test-app", func(attemptCtx context.Context, _ corev1.Pod) (string, error) {
		called = true
		cancel()
		<-attemptCtx.Done()
		return "", attemptCtx.Err()
	})
	require.True(t, called)
	require.ErrorIs(t, err, context.Canceled)
}
