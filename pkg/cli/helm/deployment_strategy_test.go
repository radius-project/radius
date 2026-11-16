//go:build !windows

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

package helm

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
	helm "helm.sh/helm/v4/pkg/action"
	chart "helm.sh/helm/v4/pkg/chart/v2"
	"helm.sh/helm/v4/pkg/chart/v2/loader"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	runtimelog "sigs.k8s.io/controller-runtime/pkg/log"
)

// TestControllerRecreateStrategy moves the shipping controller Deployment between a chart without a strategy, which
// the API server defaults to RollingUpdate, and the current chart's Recreate strategy. Server-side apply can't
// remove the defaulted rollingUpdate field because no manager owns it, so without rad clearing it the API server
// rejects the Recreate Deployment.
func TestControllerRecreateStrategy(t *testing.T) {
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		t.Skip("Run make test or set KUBEBUILDER_ASSETS to run the isolated API-server regression.")
	}
	runtimelog.SetLogger(logr.Discard())
	kubeconfig, client := startTestAPIServer(t)
	recreate, rollingUpdate := controllerDeploymentCharts(t)

	tests := []struct {
		name string
		// steps are the charts applied after installing the first one. A positive integer rolls back to that
		// revision.
		install *chart.Chart
		steps   []any
		want    appsv1.DeploymentStrategyType
	}{
		// v0.62.0 and earlier releases have no strategy.
		{name: "upgrade", install: rollingUpdate, steps: []any{recreate}, want: appsv1.RecreateDeploymentStrategyType},
		// Rolling back to an earlier version restores the default strategy.
		{name: "rollback to earlier version", install: rollingUpdate, steps: []any{recreate, 1}, want: appsv1.RollingUpdateDeploymentStrategyType},
		// A downgrade to an earlier chart restores the default strategy, so rolling back again must clear it.
		{name: "rollback after downgrade", install: recreate, steps: []any{rollingUpdate, 1}, want: appsv1.RecreateDeploymentStrategyType},
		{name: "downgrade", install: recreate, steps: []any{rollingUpdate}, want: appsv1.RollingUpdateDeploymentStrategyType},
	}
	for i, tt := range tests {
		for _, serverSideApply := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s/serverSideApply=%t", tt.name, serverSideApply), func(t *testing.T) {
				namespace := fmt.Sprintf("strategy-%d-%t", i, serverSideApply)
				flags := genericclioptions.NewConfigFlags(false)
				flags.KubeConfig = &kubeconfig
				flags.Namespace = &namespace
				configuration := &helm.Configuration{}
				require.NoError(t, configuration.Init(flags, namespace, helmDriverSecret))
				installClient := newInstallClient(configuration, "radius", namespace, false, time.Minute)
				installClient.ServerSideApply = serverSideApply
				_, err := installClient.Run(tt.install, map[string]any{})
				require.NoError(t, err)
				require.Equal(t, serverSideApply, usesServerSideApply(configuration, "radius"))

				radiusClient := NewHelmClient()
				for _, step := range tt.steps {
					switch step := step.(type) {
					case *chart.Chart:
						_, err = radiusClient.RunHelmUpgrade(configuration, step, nil, "radius", namespace, false, true, time.Minute)
					case int:
						err = radiusClient.RunHelmRollback(configuration, "radius", step, false)
					}
					require.NoError(t, err)
				}

				deployment, err := client.AppsV1().Deployments(namespace).Get(t.Context(), "controller", metav1.GetOptions{})
				require.NoError(t, err)
				for _, entry := range deployment.ManagedFields {
					require.NotEqual(t, strategyFieldManager, entry.Manager)
				}
				strategy := deployment.Spec.Strategy
				require.Equal(t, tt.want, strategy.Type)
				if tt.want == appsv1.RecreateDeploymentStrategyType {
					require.Nil(t, strategy.RollingUpdate)
				} else {
					require.NotNil(t, strategy.RollingUpdate)
				}
			})
		}
	}
}

// controllerDeploymentCharts returns charts with the shipping controller Deployment and with the same Deployment
// without a strategy, as v0.62.0 and earlier releases shipped it.
func controllerDeploymentCharts(t *testing.T) (*chart.Chart, *chart.Chart) {
	t.Helper()
	recreatePath, recreate := shippingTemplatesChart(t, "_helpers.tpl", "controller/deployment.yaml")
	values, err := os.ReadFile(filepath.Join("..", "..", "..", "deploy", "Chart", "values.yaml"))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(recreatePath, "values.yaml"), values, 0600))
	recreate, err = loader.Load(recreatePath)
	require.NoError(t, err)

	template, err := os.ReadFile(filepath.Join(recreatePath, "templates", "deployment.yaml"))
	require.NoError(t, err)
	strategy := "  strategy:\n    type: Recreate\n    rollingUpdate: null\n"
	require.Contains(t, string(template), strategy)
	require.NoError(t, os.WriteFile(filepath.Join(recreatePath, "templates", "deployment.yaml"),
		[]byte(strings.Replace(string(template), strategy, "", 1)), 0600))
	rollingUpdate, err := loader.Load(recreatePath)
	require.NoError(t, err)
	return recreate, rollingUpdate
}
