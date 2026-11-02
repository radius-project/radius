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
	"helm.sh/helm/v4/pkg/chart/v2/loader"
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
