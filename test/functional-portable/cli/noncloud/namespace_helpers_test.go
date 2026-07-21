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

package resource_test

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/radius-project/radius/pkg/cli/kubernetes"
	"github.com/radius-project/radius/test/radcli"
	"github.com/radius-project/radius/test/rp"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8s "k8s.io/client-go/kubernetes"
)

func newCLIWithoutDefaultEnvironment(t *testing.T, options rp.RPTestOptions) *radcli.CLI {
	t.Helper()

	cwd, err := os.Getwd()
	require.NoError(t, err)

	tempConfigFile, err := os.CreateTemp(cwd, "rad-test-config-*.yaml")
	require.NoError(t, err, "Failed to create temp config file")
	t.Cleanup(func() {
		_ = os.Remove(tempConfigFile.Name())
	})

	configYAML := fmt.Sprintf(`workspaces:
        default: test-workspace
        items:
          test-workspace:
            connection:
              kind: %v
              context: %v
        `, options.Workspace.Connection["kind"], options.Workspace.Connection["context"])

	_, err = tempConfigFile.WriteString(configYAML)
	require.NoError(t, err, "Failed to write config file")
	err = tempConfigFile.Close()
	require.NoError(t, err, "Failed to close config file")

	return radcli.NewCLI(t, tempConfigFile.Name())
}

func createKubernetesNamespace(ctx context.Context, t *testing.T, client k8s.Interface, namespace string) {
	t.Helper()

	require.NoError(t, kubernetes.EnsureNamespace(ctx, client, namespace), "failed to create namespace %s", namespace)
}

func deleteKubernetesNamespace(ctx context.Context, t *testing.T, client k8s.Interface, namespace string) {
	t.Helper()

	err := client.CoreV1().Namespaces().Delete(ctx, namespace, metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		t.Logf("Warning: Failed to delete namespace %s: %v", namespace, err)
	}
}
