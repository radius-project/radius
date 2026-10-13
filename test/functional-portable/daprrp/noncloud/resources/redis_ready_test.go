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
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/radius-project/radius/test/rp"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/remotecommand"
	utilexec "k8s.io/client-go/util/exec"
)

func verifyRedisReady(namespace, application string) func(context.Context, *testing.T, rp.RPTest) {
	return func(ctx context.Context, t *testing.T, ct rp.RPTest) {
		t.Helper()
		t.Logf("Waiting up to 3 minutes for Redis in %s to respond to PING through its service", namespace)
		err := waitForRedis(ctx, ct.Options.K8sClient, namespace, application, func(ctx context.Context, pod corev1.Pod) (string, error) {
			return pingRedis(ctx, ct.Options, pod)
		})
		require.NoError(t, err)
		t.Logf("Redis in %s responded with PONG; the consumer can now be deployed", namespace)
	}
}

func waitForRedis(ctx context.Context, client kubernetes.Interface, namespace, application string, ping func(context.Context, corev1.Pod) (string, error)) error {
	selector := labels.Set{"app": "redis", "radapp.io/application": application}.AsSelector().String()
	lastObservation := "no Redis pod observed"
	err := wait.PollUntilContextTimeout(ctx, time.Second, 3*time.Minute, true, func(ctx context.Context) (bool, error) {
		// PollUntilContextTimeout invokes its immediate callback even for an already canceled context.
		if err := ctx.Err(); err != nil {
			return false, err
		}
		pods, err := client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
		if err != nil {
			return false, fmt.Errorf("listing Redis pods: %w", err)
		}
		if len(pods.Items) == 0 {
			lastObservation = "no Redis pod found"
			return false, nil
		}
		if len(pods.Items) != 1 {
			return false, fmt.Errorf("expected one Redis pod, found %d", len(pods.Items))
		}
		pod := pods.Items[0]
		if pod.DeletionTimestamp != nil || pod.Status.Phase == corev1.PodFailed || pod.Status.Phase == corev1.PodSucceeded {
			return false, fmt.Errorf("Redis pod %s is terminating or has phase %s", pod.Name, pod.Status.Phase)
		}
		lastObservation = fmt.Sprintf("pod %s phase %s: Redis container has not started", pod.Name, pod.Status.Phase)
		for _, status := range pod.Status.ContainerStatuses {
			// Do not gate on the redis-monitor container or confuse Running with Redis readiness.
			if status.Name != "redis" || status.State.Running == nil {
				continue
			}
			attemptCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			output, err := ping(attemptCtx, pod)
			cancel()
			lastObservation = fmt.Sprintf("pod %s PING response %q, error: %v", pod.Name, output, err)
			if err != nil {
				var exitErr utilexec.ExitError
				if (errors.As(err, &exitErr) && exitErr.ExitStatus() == 1) ||
					errors.Is(err, context.DeadlineExceeded) || apierrors.IsNotFound(err) {
					return false, nil
				}
				return false, fmt.Errorf("executing Redis PING: %w", err)
			}
			if strings.TrimSpace(output) != "PONG" {
				return false, nil
			}
			return true, nil
		}
		return false, nil
	})
	if err != nil {
		return fmt.Errorf("waiting for Redis for application %s in namespace %s: %w; last observation: %s", application, namespace, err, lastObservation)
	}
	return nil
}

func pingRedis(ctx context.Context, opts rp.RPTestOptions, pod corev1.Pod) (string, error) {
	// The shared Redis module gives its pod and Service the same resource label.
	service := pod.Labels["resource"]
	if service == "" {
		return "", fmt.Errorf("Redis pod %s has no resource label identifying its service", pod.Name)
	}
	req := opts.K8sClient.CoreV1().RESTClient().Post().
		Resource("pods").Namespace(pod.Namespace).Name(pod.Name).SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: "redis",
			Command:   []string{"redis-cli", "--raw", "-h", service + "." + pod.Namespace + ".svc.cluster.local", "PING"},
			Stdout:    true,
			Stderr:    true,
		}, scheme.ParameterCodec)
	executor, err := remotecommand.NewSPDYExecutor(opts.K8sConfig, http.MethodPost, req.URL())
	if err != nil {
		return "", fmt.Errorf("creating Redis exec stream: %w", err)
	}
	var stdout, stderr bytes.Buffer
	err = executor.StreamWithContext(ctx, remotecommand.StreamOptions{Stdout: &stdout, Stderr: &stderr})
	if err != nil {
		return stdout.String(), fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}
