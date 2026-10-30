//go:build !windows

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

package helm

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/go-logr/logr"
	ucp_aws "github.com/radius-project/radius/pkg/ucp/aws"
	"github.com/stretchr/testify/require"
	helm "helm.sh/helm/v4/pkg/action"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	runtimelog "sigs.k8s.io/controller-runtime/pkg/log"
)

// TestAWSFingerprintKeySecretUpgrade installs the shipping AWS fingerprint key Secret, then upgrades and rolls back
// with standalone Helm and with rad. Every step must keep the Secret and its key.
func TestAWSFingerprintKeySecretUpgrade(t *testing.T) {
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		t.Skip("Run make test or set KUBEBUILDER_ASSETS to run the isolated API-server regression.")
	}
	runtimelog.SetLogger(logr.Discard())
	helmBinary, err := exec.LookPath("helm")
	require.NoError(t, err)

	kubeconfig, client := startTestAPIServer(t)
	chartPath, chart := shippingTemplatesChart(t, "ucp/aws-fingerprint-key.yaml")

	for _, upgrader := range []string{"helm", "rad"} {
		t.Run(upgrader, func(t *testing.T) {
			namespace := "aws-fingerprint-key-" + upgrader
			flags := genericclioptions.NewConfigFlags(false)
			flags.KubeConfig = &kubeconfig
			flags.Namespace = &namespace
			configuration := &helm.Configuration{}
			require.NoError(t, configuration.Init(flags, namespace, helmDriverSecret))
			radiusClient := NewHelmClient()
			_, err := radiusClient.RunHelmInstall(configuration, chart, nil, "radius", namespace, false, time.Minute)
			require.NoError(t, err)

			secrets := client.CoreV1().Secrets(namespace)
			installed, err := secrets.Get(t.Context(), ucp_aws.FingerprintKeySecretName, metav1.GetOptions{})
			require.NoError(t, err)
			require.Len(t, installed.Data, 1)
			require.Len(t, installed.Data["key"], ucp_aws.MinFingerprintKeyLength)

			helmCommand := func(args ...string) {
				t.Helper()
				args = append(args, "--kubeconfig", kubeconfig, "--kube-context", "test", "--namespace", namespace, "--timeout", "1m")
				command := exec.CommandContext(t.Context(), helmBinary, args...)
				command.Env = append(os.Environ(), "HELM_DRIVER=secret")
				output, err := command.CombinedOutput()
				require.NoError(t, err, "%s", output)
			}
			requireKeptKey := func(step string) {
				t.Helper()
				current, err := secrets.Get(t.Context(), installed.Name, metav1.GetOptions{})
				require.NoError(t, err)
				require.Equal(t, installed.UID, current.UID, "%s must not replace the Secret.", step)
				require.Equal(t, installed.Data, current.Data, "%s must keep the key.", step)
			}

			if upgrader == "helm" {
				helmCommand("upgrade", "radius", chartPath, "--reuse-values")
				requireKeptKey("Upgrade")
				helmCommand("rollback", "radius", "1")
				requireKeptKey("Rollback")
				return
			}

			upgraded, err := radiusClient.RunHelmUpgrade(configuration, chart, nil, "radius", namespace, false, true, time.Minute)
			require.NoError(t, err)
			requireKeptKey("Upgrade")
			require.NoError(t, radiusClient.RunHelmRollback(configuration, "radius", 1, false))
			requireKeptKey("Rollback")

			// The upgrade revision stored the Secret's resourceVersion. After the key is replaced, a rollback to that
			// revision renders against the live Secret, so it succeeds and keeps the replaced key.
			replaced := replaceFingerprintKey(t, secrets)
			require.NoError(t, radiusClient.RunHelmRollback(configuration, "radius", upgraded.Version, false))
			requireFingerprintKey(t, secrets, replaced, "Rollback must keep the live key.")
		})
	}
}

// TestAWSFingerprintKeySecretReplaceRace replaces the key after Helm reads the Secret and before it applies it. The
// upgrade and rollback must fail instead of writing back the key they read, even though upgrades force conflicts.
func TestAWSFingerprintKeySecretReplaceRace(t *testing.T) {
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		t.Skip("Run make test or set KUBEBUILDER_ASSETS to run the isolated API-server regression.")
	}
	runtimelog.SetLogger(logr.Discard())
	kubeconfig, client := startTestAPIServer(t)
	_, chart := shippingTemplatesChart(t, "ucp/aws-fingerprint-key.yaml")

	for _, serverSideApply := range []bool{true, false} {
		t.Run(fmt.Sprintf("serverSideApply=%t", serverSideApply), func(t *testing.T) {
			namespace := fmt.Sprintf("aws-fingerprint-key-race-%t", serverSideApply)
			flags := genericclioptions.NewConfigFlags(false)
			flags.KubeConfig = &kubeconfig
			flags.Namespace = &namespace
			configuration := &helm.Configuration{}
			require.NoError(t, configuration.Init(flags, namespace, helmDriverSecret))
			installClient := newInstallClient(configuration, "radius", namespace, false, time.Minute)
			installClient.ServerSideApply = serverSideApply
			_, err := installClient.Run(chart, map[string]any{})
			require.NoError(t, err)
			require.Equal(t, serverSideApply, usesServerSideApply(configuration, "radius"))

			secrets := client.CoreV1().Secrets(namespace)
			var replaced []byte
			upgradeClient := newUpgradeClient(configuration, namespace, false, true, time.Minute, serverSideApply)
			require.Equal(t, serverSideApply, upgradeClient.ForceConflicts)
			// Helm runs the post-renderer after the template has read the Secret and before it applies the manifest.
			upgradeClient.PostRenderer = postRenderFunc(func() { replaced = replaceFingerprintKey(t, secrets) })
			_, err = upgradeClient.Run("radius", chart, map[string]any{})
			require.ErrorContains(t, err, "the object has been modified")
			requireFingerprintKey(t, secrets, replaced, "The upgrade must not overwrite the replaced key.")

			// The next upgrade reads the replaced Secret and succeeds.
			_, err = NewHelmClient().RunHelmUpgrade(configuration, chart, nil, "radius", namespace, false, true, time.Minute)
			require.NoError(t, err)
			requireFingerprintKey(t, secrets, replaced, "The upgrade must keep the replaced key.")

			// A rollback renders the Secret the same way, so it must fail the same way.
			stored, err := configuration.Releases.Get("radius", 1)
			require.NoError(t, err)
			target, err := asRelease(stored)
			require.NoError(t, err)
			rollbackClient := newRollbackClient(configuration, namespace, 1, false, serverSideApply)
			rollbackClient.PostRenderer = postRenderFunc(func() { replaced = replaceFingerprintKey(t, secrets) })
			_, err = rollbackClient.Run("radius", target.Chart, target.Config)
			require.ErrorContains(t, err, "the object has been modified")
			requireFingerprintKey(t, secrets, replaced, "The rollback must not overwrite the replaced key.")
		})
	}
}

// replaceFingerprintKey replaces the key with kubectl patch, the way an operator would, and returns the new key.
func replaceFingerprintKey(t *testing.T, secrets typedcorev1.SecretInterface) []byte {
	t.Helper()
	key := make([]byte, ucp_aws.MinFingerprintKeyLength)
	_, err := rand.Read(key)
	require.NoError(t, err)
	patch, err := json.Marshal(corev1.Secret{Data: map[string][]byte{"key": key}})
	require.NoError(t, err)
	_, err = secrets.Patch(t.Context(), ucp_aws.FingerprintKeySecretName, types.MergePatchType, patch,
		metav1.PatchOptions{FieldManager: "kubectl-patch"})
	require.NoError(t, err)
	return key
}

func requireFingerprintKey(t *testing.T, secrets typedcorev1.SecretInterface, want []byte, msg string) {
	t.Helper()
	secret, err := secrets.Get(t.Context(), ucp_aws.FingerprintKeySecretName, metav1.GetOptions{})
	require.NoError(t, err)
	require.True(t, bytes.Equal(want, secret.Data["key"]), msg)
}
