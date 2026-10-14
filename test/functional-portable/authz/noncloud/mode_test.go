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
	"io"
	"os"
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	helm "helm.sh/helm/v4/pkg/action"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	k8s "k8s.io/client-go/kubernetes"

	"github.com/radius-project/radius/pkg/authz"
	"github.com/radius-project/radius/pkg/cli/kubernetes"
	"github.com/radius-project/radius/test"
)

const (
	radiusNamespace   = "radius-system"
	radiusReleaseName = "radius"
	readyTimeout      = 2 * time.Minute
	expectedModeEnv   = "AUTHZ_EXPECTED_MODE"
)

// component is a Radius service that reads the authorization mode at startup.
type component struct {
	deployment string
	container  string
}

var components = []component{
	{deployment: "ucp", container: "ucp"},
	{deployment: "applications-rp", container: "applications-rp"},
	{deployment: "dynamic-rp", container: "dynamic-rp"},
	{deployment: "controller", container: "controller"},
}

var modeLogPattern = regexp.MustCompile(`authz mode=(\w+)`)

// Test_AuthzMode_LoggedByEveryComponent verifies the control plane is up and that every
// Radius component logs the authorization mode derived from the installed Helm values.
func Test_AuthzMode_LoggedByEveryComponent(t *testing.T) {
	options := test.NewTestOptions(t)
	want := expectedInstalledMode(t)
	t.Logf("Helm values select authz mode=%s", want)

	for _, c := range components {
		t.Run(c.deployment, func(t *testing.T) {
			t.Parallel()
			ctx := t.Context()

			requireDeploymentReady(ctx, t, options.K8sClient, c.deployment)

			pods, err := options.K8sClient.CoreV1().Pods(radiusNamespace).List(ctx, metav1.ListOptions{
				LabelSelector: "app.kubernetes.io/name=" + c.deployment,
			})
			require.NoError(t, err)
			require.NotEmpty(t, pods.Items, "no pods found for %s", c.deployment)

			for _, pod := range pods.Items {
				if pod.Status.Phase != corev1.PodRunning {
					continue
				}
				logs := podLogs(ctx, t, options.K8sClient, pod.Name, c.container)
				got, ok := loggedMode(logs)
				startTime := "<unknown>"
				if pod.Status.StartTime != nil {
					startTime = pod.Status.StartTime.Time.Format(time.RFC3339)
				}
				require.Truef(t, ok, "pod %s (started %s) did not log %q; log output may have rotated before the test read it", pod.Name, startTime, "authz mode=<mode>")
				require.Equalf(t, string(want), got, "pod %s logged the wrong authz mode", pod.Name)
			}
		})
	}
}

func Test_ExpectedMode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		values map[string]any
		want   authz.Mode
	}{
		{name: "no global values", values: map[string]any{}, want: authz.ModeOff},
		{name: "no rbac values", values: rbacValues(nil), want: authz.ModeOff},
		{name: "both false", values: rbacValues(map[string]any{"enabled": false, "dryRun": false}), want: authz.ModeOff},
		{name: "dry run", values: rbacValues(map[string]any{"dryRun": true}), want: authz.ModeDryRun},
		{name: "enforce", values: rbacValues(map[string]any{"enabled": true}), want: authz.ModeEnforce},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, expectedMode(tt.values))
		})
	}
}

func Test_LoggedMode(t *testing.T) {
	t.Parallel()

	got, ok := loggedMode(`{"level":"info","msg":"authz mode=dryRun","authz.mode":"dryRun"}`)
	require.True(t, ok)
	require.Equal(t, "dryRun", got)

	_, ok = loggedMode(`{"level":"info","msg":"Loaded options"}`)
	require.False(t, ok)
}

func Test_ExpectedTestMode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		helmMode authz.Mode
		env      string
		want     authz.Mode
		wantErr  string
	}{
		{name: "unset uses helm mode", helmMode: authz.ModeOff, want: authz.ModeOff},
		{name: "env matches helm mode", helmMode: authz.ModeDryRun, env: "dryRun", want: authz.ModeDryRun},
		{name: "env may expect off", helmMode: authz.ModeOff, env: "off", want: authz.ModeOff},
		{name: "invalid env mode", helmMode: authz.ModeDryRun, env: "audit", wantErr: "invalid authorization mode"},
		{name: "env mismatches helm mode", helmMode: authz.ModeOff, env: "dryRun", wantErr: "does not match Helm values"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := expectedTestMode(tt.helmMode, tt.env)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func rbacValues(rbac map[string]any) map[string]any {
	global := map[string]any{}
	if rbac != nil {
		global["rbac"] = rbac
	}
	return map[string]any{"global": global}
}

// expectedInstalledMode reads the installed Radius Helm release and returns the authorization
// mode the test must observe.
func expectedInstalledMode(t *testing.T) authz.Mode {
	t.Helper()

	contextName, err := kubernetes.GetContextFromConfigFileIfExists("", "")
	require.NoError(t, err)

	flags := genericclioptions.NewConfigFlags(false)
	namespace := radiusNamespace
	flags.Namespace = &namespace
	if contextName != "" {
		flags.Context = &contextName
	}

	cfg := &helm.Configuration{}
	require.NoError(t, cfg.Init(flags, radiusNamespace, "secret"))

	get := helm.NewGetValues(cfg)
	get.AllValues = true
	values, err := get.Run(radiusReleaseName)
	require.NoError(t, err, "failed to read Helm values for release %q", radiusReleaseName)

	helmMode := expectedMode(values)
	want, err := expectedTestMode(helmMode, os.Getenv(expectedModeEnv))
	require.NoError(t, err)
	return want
}

// expectedMode mirrors the chart's radius.authz.mode helper.
func expectedMode(values map[string]any) authz.Mode {
	global, _ := values["global"].(map[string]any)
	rbac, _ := global["rbac"].(map[string]any)
	if dryRun, _ := rbac["dryRun"].(bool); dryRun {
		return authz.ModeDryRun
	}
	if enabled, _ := rbac["enabled"].(bool); enabled {
		return authz.ModeEnforce
	}
	return authz.ModeOff
}

func expectedTestMode(helmMode authz.Mode, expected string) (authz.Mode, error) {
	if expected == "" {
		return helmMode, nil
	}

	mode, err := authz.ParseMode(expected)
	if err != nil {
		return "", fmt.Errorf("%s: %w", expectedModeEnv, err)
	}
	if mode != helmMode {
		return "", fmt.Errorf("%s=%s does not match Helm values selecting authz mode=%s", expectedModeEnv, mode, helmMode)
	}
	return mode, nil
}

func loggedMode(logs string) (string, bool) {
	match := modeLogPattern.FindStringSubmatch(logs)
	if match == nil {
		return "", false
	}
	return match[1], true
}

func requireDeploymentReady(ctx context.Context, t *testing.T, client k8s.Interface, name string) {
	t.Helper()

	err := wait.PollUntilContextTimeout(ctx, 2*time.Second, readyTimeout, true, func(ctx context.Context) (bool, error) {
		deployment, err := client.AppsV1().Deployments(radiusNamespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		return deployment.Status.ReadyReplicas > 0, nil
	})
	require.NoError(t, err, "deployment %s/%s is not ready", radiusNamespace, name)
}

func podLogs(ctx context.Context, t *testing.T, client k8s.Interface, pod, container string) string {
	t.Helper()

	stream, err := client.CoreV1().Pods(radiusNamespace).GetLogs(pod, &corev1.PodLogOptions{Container: container}).Stream(ctx)
	require.NoError(t, err, "failed to read logs for %s/%s", pod, container)
	defer stream.Close()

	data, err := io.ReadAll(stream)
	require.NoError(t, err)
	return string(data)
}
