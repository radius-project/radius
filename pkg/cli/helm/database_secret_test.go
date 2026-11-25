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
	"os"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"helm.sh/helm/v4/pkg/chart/common"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestDatabasePasswordAcrossEngineRollout(t *testing.T) {
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		t.Skip("Set KUBEBUILDER_ASSETS to run the isolated API-server regression.")
	}
	const namespace = "radius-test"
	kubeconfig, client := startTestAPIServer(t)
	configuration := helmTestConfiguration(t, kubeconfig, namespace)
	chart := loadStageTestChart(t)
	chart.Templates = slices.DeleteFunc(chart.Templates, func(file *common.File) bool {
		return file.Name != "templates/_helpers.tpl" && file.Name != "templates/database/configmaps.yaml" &&
			file.Name != "templates/ucp/deployment.yaml"
	})
	rad := NewHelmClient()
	values := map[string]any{
		"database": map[string]any{"enabled": true},
		"global":   map[string]any{"rbac": map[string]any{"enabled": true}},
	}
	_, err := rad.RunHelmInstall(configuration, chart, values, "radius", namespace, false, time.Minute, nil)
	require.NoError(t, err)

	startupConfig := func() (string, []byte) {
		t.Helper()
		deployment, err := client.AppsV1().Deployments(namespace).Get(t.Context(), "ucp", metav1.GetOptions{})
		require.NoError(t, err)
		for _, env := range deployment.Spec.Template.Spec.Containers[0].Env {
			if env.Name == "POSTGRES_PASSWORD" {
				ref := env.ValueFrom.SecretKeyRef
				secret, err := client.CoreV1().Secrets(namespace).Get(t.Context(), ref.Name, metav1.GetOptions{})
				require.NoError(t, err)
				return deployment.Spec.Template.Annotations["checksum/authz-config"], secret.Data[ref.Key]
			}
		}
		t.Fatal("UCP has no POSTGRES_PASSWORD")
		return "", nil
	}
	initialChecksum, initialPassword := startupConfig()
	require.NotEmpty(t, initialChecksum)
	require.Len(t, initialPassword, 16)
	_, err = rad.RunHelmUpgrade(configuration, chart, map[string]any{
		"de": map[string]any{"tag": "test-fixture", "internalAuth": map[string]any{"enabled": true}},
	}, "radius", namespace, false, true, time.Minute)
	require.NoError(t, err)
	upgradedChecksum, upgradedPassword := startupConfig()
	require.NotEqual(t, initialChecksum, upgradedChecksum)
	require.True(t, bytes.Equal(initialPassword, upgradedPassword), "upgraded UCP must receive the password used to initialize the database")

	require.NoError(t, rad.RunHelmRollback(configuration, "radius", 1, false))
	rollbackChecksum, rollbackPassword := startupConfig()
	require.Equal(t, initialChecksum, rollbackChecksum)
	require.True(t, bytes.Equal(initialPassword, rollbackPassword), "rolled-back UCP must receive the original password")
}
