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

package authz

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8s "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/radius-project/radius/pkg/authz"
	"github.com/radius-project/radius/pkg/azure/tokencredentials"
	corerp "github.com/radius-project/radius/pkg/corerp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/sdk"
	"github.com/radius-project/radius/test/rp"
)

// Test_PodSecurity_EnvironmentNamespace creates an Applications.Core environment, so Radius
// creates its namespace, and checks the Pod Security labels and their effect on a privileged
// pod: no labels in off mode, a warning in dryRun, and a rejection in enforce.
func Test_PodSecurity_EnvironmentNamespace(t *testing.T) {
	options := rp.NewRPTestOptions(t)
	ctx := t.Context()

	values := installedValues(t)
	mode := expectedInstalledMode(t)
	level := expectedPodSecurityLevel(values)
	t.Logf("Helm values select authz mode=%s podSecurityLevel=%s", mode, level)

	suffix := time.Now().UnixNano()
	envName := fmt.Sprintf("authz-psa-%d", suffix)
	namespace := envName

	connection, err := options.Workspace.Connect(ctx)
	require.NoError(t, err)
	envClient, err := corerp.NewEnvironmentsClient(&tokencredentials.AnonymousCredential{}, sdk.NewClientOptions(connection))
	require.NoError(t, err)
	// The generated client rejects a leading '/' in the scope.
	scope := strings.TrimPrefix(options.Workspace.Scope, "/")

	t.Cleanup(func() {
		ctx := context.Background() //nolint:usetesting // t.Context() is cancelled before cleanup runs.
		_, _ = envClient.Delete(ctx, scope, envName, nil)
		_ = options.K8sClient.CoreV1().Namespaces().Delete(ctx, namespace, metav1.DeleteOptions{})
	})

	_, err = envClient.CreateOrUpdate(ctx, scope, envName, corerp.EnvironmentResource{
		Location: new("global"),
		Properties: &corerp.EnvironmentProperties{
			Compute: &corerp.KubernetesCompute{Kind: new("kubernetes"), Namespace: new(namespace)},
		},
	}, nil)
	require.NoError(t, err, "failed to create environment %s", envName)

	ns, err := options.K8sClient.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
	require.NoError(t, err, "Radius did not create namespace %s", namespace)
	require.Equal(t, authz.Options{Mode: mode, PodSecurityLevel: level}.PodSecurityLabels(namespace), podSecurityLabels(ns.Labels))

	warnings := &warningRecorder{}
	cfg := rest.CopyConfig(options.K8sConfig)
	cfg.WarningHandler = warnings
	client, err := k8s.NewForConfig(cfg)
	require.NoError(t, err)

	// A server-side dry run runs admission without creating the pod.
	_, err = client.CoreV1().Pods(namespace).Create(ctx, privilegedPod(), metav1.CreateOptions{DryRun: []string{metav1.DryRunAll}})

	switch mode {
	case authz.ModeOff:
		require.NoError(t, err)
		require.Empty(t, warnings.podSecurity())
	case authz.ModeDryRun:
		require.NoError(t, err, "dryRun must not reject privileged pods")
		require.NotEmpty(t, warnings.podSecurity(), "expected a Pod Security warning for a privileged pod")
	case authz.ModeEnforce:
		require.Error(t, err)
		require.True(t, apierrors.IsForbidden(err), "expected Forbidden, got %v", err)
		require.Contains(t, err.Error(), "violates PodSecurity")
	}
}

func Test_ExpectedPodSecurityLevel(t *testing.T) {
	t.Parallel()

	require.Equal(t, authz.PodSecurityLevelBaseline, expectedPodSecurityLevel(map[string]any{}))
	require.Equal(t, authz.PodSecurityLevelBaseline, expectedPodSecurityLevel(rbacValues(map[string]any{"podSecurityLevel": ""})))
	require.Equal(t, authz.PodSecurityLevelRestricted, expectedPodSecurityLevel(rbacValues(map[string]any{"podSecurityLevel": "restricted"})))
}

// expectedPodSecurityLevel mirrors the chart's radius.authz.podSecurityLevel helper.
func expectedPodSecurityLevel(values map[string]any) authz.PodSecurityLevel {
	global, _ := values["global"].(map[string]any)
	rbac, _ := global["rbac"].(map[string]any)
	level, _ := rbac["podSecurityLevel"].(string)
	if level == "" {
		return authz.PodSecurityLevelBaseline
	}
	return authz.PodSecurityLevel(level)
}

// podSecurityLabels returns the Pod Security Admission labels in labels, or nil when there are none.
func podSecurityLabels(labels map[string]string) map[string]string {
	var result map[string]string
	for k, v := range labels {
		if strings.HasPrefix(k, "pod-security.kubernetes.io/") {
			if result == nil {
				result = map[string]string{}
			}
			result[k] = v
		}
	}
	return result
}

func privilegedPod() *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "authz-privileged"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{
				Name:            "privileged",
				Image:           "busybox",
				SecurityContext: &corev1.SecurityContext{Privileged: new(true)},
			}},
		},
	}
}

// warningRecorder records warnings returned by the Kubernetes API server.
type warningRecorder struct {
	mu       sync.Mutex
	warnings []string
}

func (w *warningRecorder) HandleWarningHeader(_ int, _ string, text string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.warnings = append(w.warnings, text)
}

func (w *warningRecorder) podSecurity() []string {
	return w.matching("PodSecurity")
}

// matching returns the recorded warnings that contain substr.
func (w *warningRecorder) matching(substr string) []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	var result []string
	for _, warning := range w.warnings {
		if strings.Contains(warning, substr) {
			result = append(result, warning)
		}
	}
	return result
}
