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
	"bytes"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
	helm "helm.sh/helm/v4/pkg/action"
	"helm.sh/helm/v4/pkg/chart/common"
	chart "helm.sh/helm/v4/pkg/chart/v2"
	"helm.sh/helm/v4/pkg/chart/v2/loader"
	releasecommon "helm.sh/helm/v4/pkg/release/common"
	releasev1 "helm.sh/helm/v4/pkg/release/v1"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	runtimelog "sigs.k8s.io/controller-runtime/pkg/log"
)

func TestControllerStrategyMigration(t *testing.T) {
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		t.Skip("Run make test or set KUBEBUILDER_ASSETS to run the isolated API-server regression.")
	}
	runtimelog.SetLogger(logr.Discard())
	kubeconfig, client := startTestAPIServer(t)
	shipping, err := loader.Load("../../../deploy/Chart")
	require.NoError(t, err)
	path, desired := shippingTemplatesChart(t, "_helpers.tpl", "controller/deployment.yaml")
	desired.Values, desired.Metadata = shipping.Values, shipping.Metadata
	previous, err := loader.Load(path)
	require.NoError(t, err)
	previous.Values, previous.Metadata = shipping.Values, shipping.Metadata
	for _, file := range previous.Templates {
		if file.Name == "templates/deployment.yaml" {
			start := bytes.Index(file.Data, []byte("  strategy:\n"))
			end := bytes.Index(file.Data, []byte("  selector:\n"))
			require.GreaterOrEqual(t, start, 0)
			require.Greater(t, end, start)
			file.Data = append(bytes.Clone(file.Data[:start]), file.Data[end:]...)
		}
	}

	for index, tt := range []struct {
		name        string
		ssa         bool
		failedFirst bool
		fresh       bool
	}{
		{name: "SSA", ssa: true},
		{name: "CSA"},
		{name: "retry failed SSA upgrade", ssa: true, failedFirst: true},
		{name: "fresh SSA installation", ssa: true, fresh: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			namespace := fmt.Sprintf("controller-migration-%d", index)
			flags := genericclioptions.NewConfigFlags(false)
			flags.KubeConfig, flags.Namespace = &kubeconfig, &namespace
			configuration := &helm.Configuration{}
			require.NoError(t, configuration.Init(flags, namespace, helmDriverSecret))
			helmClient := NewHelmClient()
			if tt.fresh {
				require.NoError(t, helmClient.MigrateControllerStrategy(t.Context(), configuration, desired, nil, "radius", namespace, true))
			}
			install := newInstallClient(configuration, "radius", namespace, false, time.Minute)
			install.ServerSideApply = tt.ssa
			initialChart := previous
			if tt.fresh {
				initialChart = desired
			}
			_, err := install.Run(cloneChart(initialChart), nil)
			require.NoError(t, err)
			deployments := client.AppsV1().Deployments(namespace)
			before, err := deployments.Get(t.Context(), "controller", metav1.GetOptions{})
			require.NoError(t, err)
			if !tt.fresh {
				require.Equal(t, appsv1.RollingUpdateDeploymentStrategyType, before.Spec.Strategy.Type)
				require.NotNil(t, before.Spec.Strategy.RollingUpdate)
			}
			if tt.failedFirst {
				_, err = helmClient.RunHelmUpgrade(configuration, cloneChart(desired), nil, "radius", namespace, false, true, time.Minute)
				require.ErrorContains(t, err, "may not be specified when strategy `type` is 'Recreate'")
			}
			history, err := helmClient.RunHelmHistory(configuration, "radius")
			require.NoError(t, err)
			require.NoError(t, helmClient.MigrateControllerStrategy(t.Context(), configuration, desired, nil, "radius", namespace, true))
			migrated, err := deployments.Get(t.Context(), "controller", metav1.GetOptions{})
			require.NoError(t, err)
			require.Equal(t, before.UID, migrated.UID)
			require.Equal(t, before.Spec.Template, migrated.Spec.Template)
			require.Equal(t, before.Spec.Replicas, migrated.Spec.Replicas)
			require.Equal(t, appsv1.RecreateDeploymentStrategyType, migrated.Spec.Strategy.Type)
			require.Nil(t, migrated.Spec.Strategy.RollingUpdate)
			require.NoError(t, helmClient.MigrateControllerStrategy(t.Context(), configuration, desired, nil, "radius", namespace, true))
			again, err := deployments.Get(t.Context(), "controller", metav1.GetOptions{})
			require.NoError(t, err)
			require.Equal(t, migrated.ResourceVersion, again.ResourceVersion, "migration must be idempotent")
			afterHistory, err := helmClient.RunHelmHistory(configuration, "radius")
			require.NoError(t, err)
			require.Equal(t, history, afterHistory, "rendering and migration must not change Helm history")
			upgraded, err := helmClient.RunHelmUpgrade(configuration, cloneChart(desired), nil, "radius", namespace, false, true, time.Minute)
			require.NoError(t, err)
			require.Equal(t, releasecommon.StatusDeployed, upgraded.Info.Status)
			after, err := deployments.Get(t.Context(), "controller", metav1.GetOptions{})
			require.NoError(t, err)
			require.Equal(t, before.UID, after.UID)
			require.Equal(t, appsv1.RecreateDeploymentStrategyType, after.Spec.Strategy.Type)
			require.Nil(t, after.Spec.Strategy.RollingUpdate)
		})
	}
}

func TestControllerStrategyMigrationUsesRenderedUpgrade(t *testing.T) {
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		t.Skip("Run make test or set KUBEBUILDER_ASSETS to run the isolated API-server regression.")
	}
	runtimelog.SetLogger(logr.Discard())
	kubeconfig, client := startTestAPIServer(t)
	previous := &chart.Chart{
		Metadata: &chart.Metadata{APIVersion: "v2", Name: "radius", Version: "0.1.0"},
		Templates: []*common.File{{Name: "templates/controller.yaml", Data: []byte(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: controller
spec:
  selector:
    matchLabels: {app: controller}
  template:
    metadata:
      labels: {app: controller}
    spec:
      containers:
        - name: controller
          image: example.invalid/controller:test
`)}},
	}
	desired := cloneChart(previous)
	template := *previous.Templates[0]
	desired.Templates = []*common.File{&template}
	desired.Templates[0].Data = bytes.Replace(desired.Templates[0].Data, []byte("spec:\n"), []byte(`spec:
  {{- if .Values.reject }}{{ fail "render rejected" }}{{ end }}
  {{- if and .Release.IsUpgrade (gt .Release.Revision 1) .Values.recreate (lookup "apps/v1" "Deployment" .Release.Namespace "controller") }}
  strategy:
    type: Recreate
    rollingUpdate: null
  {{- end }}
`), 1)
	for index, tt := range []struct {
		name      string
		reuse     bool
		values    map[string]any
		failed    bool
		wantError string
		want      appsv1.DeploymentStrategyType
	}{
		{name: "stored value and lookup", reuse: true, want: appsv1.RecreateDeploymentStrategyType},
		{name: "override", reuse: true, values: map[string]any{"recreate": false}, want: appsv1.RollingUpdateDeploymentStrategyType},
		{name: "reset values", want: appsv1.RollingUpdateDeploymentStrategyType},
		{name: "failed revision uses deployed values", reuse: true, failed: true, want: appsv1.RecreateDeploymentStrategyType},
		{name: "render error leaves controller unchanged", reuse: true, values: map[string]any{"reject": true}, wantError: "render rejected", want: appsv1.RollingUpdateDeploymentStrategyType},
	} {
		t.Run(tt.name, func(t *testing.T) {
			namespace := fmt.Sprintf("rendered-migration-%d", index)
			flags := genericclioptions.NewConfigFlags(false)
			flags.KubeConfig, flags.Namespace = &kubeconfig, &namespace
			configuration := &helm.Configuration{}
			require.NoError(t, configuration.Init(flags, namespace, helmDriverSecret))
			helmClient := NewHelmClient()
			installed, err := helmClient.RunHelmInstall(configuration, cloneChart(previous), map[string]any{"recreate": true}, "radius", namespace, false, time.Minute, nil)
			require.NoError(t, err)
			if tt.failed {
				failed := *installed
				failed.Version = 2
				failed.Info = &releasev1.Info{Status: releasecommon.StatusFailed}
				failed.Config = map[string]any{"recreate": false}
				require.NoError(t, configuration.Releases.Create(&failed))
			}
			err = helmClient.MigrateControllerStrategy(t.Context(), configuration, desired, tt.values, "radius", namespace, tt.reuse)
			if tt.wantError != "" {
				require.ErrorContains(t, err, tt.wantError)
			} else {
				require.NoError(t, err)
			}
			deployment, err := client.AppsV1().Deployments(namespace).Get(t.Context(), "controller", metav1.GetOptions{})
			require.NoError(t, err)
			require.Equal(t, tt.want, deployment.Spec.Strategy.Type)
		})
	}
}

// TestControllerStrategyMigrationWhileStopped verifies the zero-replica-guarded
// patch documented for migrating existing Flux templates.
func TestControllerStrategyMigrationWhileStopped(t *testing.T) {
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		t.Skip("Run make test or set KUBEBUILDER_ASSETS to run the isolated API-server regression.")
	}
	runtimelog.SetLogger(logr.Discard())
	kubeconfig, client := startTestAPIServer(t)
	shipping, err := loader.Load("../../../deploy/Chart")
	require.NoError(t, err)
	path, desired := shippingTemplatesChart(t, "_helpers.tpl", "controller/deployment.yaml")
	desired.Values, desired.Metadata = shipping.Values, shipping.Metadata
	previous, err := loader.Load(path)
	require.NoError(t, err)
	previous.Values, previous.Metadata = shipping.Values, shipping.Metadata
	for _, file := range previous.Templates {
		if file.Name == "templates/deployment.yaml" {
			start := bytes.Index(file.Data, []byte("  strategy:\n"))
			end := bytes.Index(file.Data, []byte("  selector:\n"))
			require.GreaterOrEqual(t, start, 0)
			require.Greater(t, end, start)
			// The released chart omitted strategy; the API supplies RollingUpdate.
			file.Data = append(file.Data[:start], file.Data[end:]...)
		}
	}

	for _, serverSideApply := range []bool{true, false} {
		t.Run(fmt.Sprintf("serverSideApply=%t", serverSideApply), func(t *testing.T) {
			namespace := fmt.Sprintf("controller-upgrade-%t", serverSideApply)
			flags := genericclioptions.NewConfigFlags(false)
			flags.KubeConfig, flags.Namespace = &kubeconfig, &namespace
			configuration := &helm.Configuration{}
			require.NoError(t, configuration.Init(flags, namespace, helmDriverSecret))
			install := newInstallClient(configuration, "radius", namespace, false, time.Minute)
			install.ServerSideApply = serverSideApply
			_, err := install.Run(previous, nil)
			require.NoError(t, err)
			deployments := client.AppsV1().Deployments(namespace)
			before, err := deployments.Get(t.Context(), "controller", metav1.GetOptions{})
			require.NoError(t, err)
			require.NotNil(t, before.Spec.Strategy.RollingUpdate)

			migration := []byte(`[{"op":"test","path":"/spec/replicas","value":0},{"op":"add","path":"/spec/strategy","value":{"type":"Recreate"}}]`)
			_, err = deployments.Patch(t.Context(), "controller", types.JSONPatchType, migration, metav1.PatchOptions{})
			require.Error(t, err, "migration must refuse to change the strategy before the controller is stopped")
			_, err = deployments.Patch(t.Context(), "controller", types.MergePatchType,
				[]byte(`{"spec":{"replicas":0}}`), metav1.PatchOptions{})
			require.NoError(t, err)
			for range 2 {
				stopped, err := deployments.Patch(t.Context(), "controller", types.JSONPatchType, migration, metav1.PatchOptions{})
				require.NoError(t, err)
				require.Zero(t, *stopped.Spec.Replicas, "strategy migration must not restart the old controller")
				require.Equal(t, before.UID, stopped.UID)
			}
			_, err = newUpgradeClient(configuration, namespace, false, true, time.Minute, serverSideApply).Run("radius", desired, nil)
			require.NoError(t, err)
			after, err := deployments.Get(t.Context(), "controller", metav1.GetOptions{})
			require.NoError(t, err)
			require.Equal(t, before.UID, after.UID)
			require.Equal(t, appsv1.RecreateDeploymentStrategyType, after.Spec.Strategy.Type)
			require.Nil(t, after.Spec.Strategy.RollingUpdate)
		})
	}
}
