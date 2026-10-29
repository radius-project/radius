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
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/radius-project/radius/pkg/crypto/encryption"
	"github.com/stretchr/testify/require"
	helm "helm.sh/helm/v4/pkg/action"
	chart "helm.sh/helm/v4/pkg/chart/v2"
	"helm.sh/helm/v4/pkg/chart/v2/loader"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	runtimelog "sigs.k8s.io/controller-runtime/pkg/log"
)

func TestEncryptionSecretUpgrade(t *testing.T) {
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		t.Skip("Run make test or set KUBEBUILDER_ASSETS to run the isolated API-server regression.")
	}
	runtimelog.SetLogger(logr.Discard())
	helmBinary, err := exec.LookPath("helm")
	require.NoError(t, err)
	version, err := exec.CommandContext(t.Context(), helmBinary, "version", "--template", "{{.Version}}").Output()
	require.NoError(t, err)
	require.Regexp(t, `^v4\.`, string(version), "This regression requires Helm 4 server-side apply.")

	kubeconfig, client := startTestAPIServer(t)

	chartPath, chart := encryptionSecretChart(t)

	for _, upgrader := range []string{"helm", "rad"} {
		for _, state := range []string{"fresh", "rotated", "legacy"} {
			t.Run(upgrader+"/"+state, func(t *testing.T) {
				namespace := "encryption-" + upgrader + "-" + state
				flags := genericclioptions.NewConfigFlags(false)
				flags.KubeConfig = &kubeconfig
				flags.Namespace = &namespace
				configuration := &helm.Configuration{}
				require.NoError(t, configuration.Init(flags, namespace, helmDriverSecret))
				radiusClient := NewHelmClient()
				_, err := radiusClient.RunHelmInstall(configuration, chart, nil, "radius", namespace, false, time.Minute)
				require.NoError(t, err)

				secrets := client.CoreV1().Secrets(namespace)
				secret, err := secrets.Get(t.Context(), encryption.DefaultEncryptionKeySecretName, metav1.GetOptions{})
				require.NoError(t, err)
				var store encryption.KeyStore
				require.NoError(t, json.Unmarshal(secret.Data[encryption.DefaultEncryptionKeySecretKey], &store))
				require.Equal(t, 1, store.CurrentVersion)
				require.Equal(t, store.Keys["1"].CreatedAt, secret.Annotations["radius.dev/last-rotation"])
				key, err := base64.StdEncoding.DecodeString(store.Keys["1"].Key)
				require.NoError(t, err)
				require.Len(t, key, 32)
				require.True(t, ownsEncryptionRotation(t, secret, FieldManager, metav1.ManagedFieldsOperationApply))

				switch state {
				case "rotated":
					waitForEncryptionTimestampBoundary(t, secret.Annotations["radius.dev/last-rotation"])
					rotatedAt := time.Now().UTC()
					store.CurrentVersion = 2
					store.Keys["2"] = encryption.KeyData{
						Key:       base64.StdEncoding.EncodeToString(make([]byte, 32)),
						Version:   2,
						CreatedAt: rotatedAt.Format(time.RFC3339),
						ExpiresAt: rotatedAt.Add(90 * 24 * time.Hour).Format(time.RFC3339),
					}
					data, err := json.Marshal(store)
					require.NoError(t, err)
					patch, err := json.Marshal(corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{
							"radius.dev/last-rotation": store.Keys["2"].CreatedAt,
						}},
						Data: map[string][]byte{encryption.DefaultEncryptionKeySecretKey: data},
					})
					require.NoError(t, err)
					secret, err = secrets.Patch(t.Context(), secret.Name, types.MergePatchType, patch,
						metav1.PatchOptions{FieldManager: "kubectl-patch"})
					require.NoError(t, err)
					require.True(t, ownsEncryptionRotation(t, secret, "kubectl-patch", metav1.ManagedFieldsOperationUpdate))
				case "legacy":
					secret, err = secrets.Patch(t.Context(), secret.Name, types.MergePatchType,
						[]byte(`{"metadata":{"annotations":{"radius.dev/last-rotation":null}}}`),
						metav1.PatchOptions{FieldManager: "kubectl-patch"})
					require.NoError(t, err)
				}
				uid := secret.UID
				digest := sha256.Sum256(secret.Data[encryption.DefaultEncryptionKeySecretKey])
				rotation, hasRotation := secret.Annotations["radius.dev/last-rotation"]
				if hasRotation {
					// Cross a timestamp boundary so the old template fails deterministically.
					waitForEncryptionTimestampBoundary(t, rotation)
				}

				if upgrader == "helm" {
					command := exec.CommandContext(t.Context(), helmBinary, "upgrade", "radius", chartPath,
						"--kubeconfig", kubeconfig, "--kube-context", "test",
						"--namespace", namespace, "--reuse-values", "--timeout", "1m")
					command.Env = append(os.Environ(), "HELM_DRIVER=secret")
					output, err := command.CombinedOutput()
					require.NoError(t, err, "%s", output)
				} else {
					_, err = radiusClient.RunHelmUpgrade(configuration, chart, nil, "radius", namespace, false, true, time.Minute)
					require.NoError(t, err)
				}

				upgraded, err := secrets.Get(t.Context(), secret.Name, metav1.GetOptions{})
				require.NoError(t, err)
				require.Equal(t, uid, upgraded.UID, "Upgrade must not replace the Secret.")
				require.Equal(t, digest, sha256.Sum256(upgraded.Data[encryption.DefaultEncryptionKeySecretKey]),
					"Upgrade must preserve the exact key-store bytes.")
				upgradedRotation, upgradedHasRotation := upgraded.Annotations["radius.dev/last-rotation"]
				require.Equal(t, hasRotation, upgradedHasRotation)
				require.Equal(t, rotation, upgradedRotation)
				if upgrader == "helm" && hasRotation {
					require.True(t, ownsEncryptionRotation(t, upgraded, filepath.Base(helmBinary), metav1.ManagedFieldsOperationApply),
						"Standalone Helm must apply the unchanged rotation field without forcing conflicts.")
				}
			})
		}
	}
}

// encryptionSecretChart returns a chart with the shipping encryption Secret template, without workloads that
// require a kubelet.
func encryptionSecretChart(t *testing.T) (string, *chart.Chart) {
	t.Helper()
	chartPath := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(chartPath, "templates"), 0700))
	require.NoError(t, os.WriteFile(filepath.Join(chartPath, "Chart.yaml"),
		[]byte("apiVersion: v2\nname: radius\nversion: 0.1.0\n"), 0600))
	for _, name := range []string{"_helpers.tpl", "dynamic-rp/secret.yaml"} {
		content, err := os.ReadFile(filepath.Join("..", "..", "..", "deploy", "Chart", "templates", filepath.FromSlash(name)))
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(chartPath, "templates", filepath.Base(name)), content, 0600))
	}
	loaded, err := loader.Load(chartPath)
	require.NoError(t, err)
	return chartPath, loaded
}

// TestEncryptionSecretUpgradeRotationRace rotates a key after Helm reads the Secret and before it applies it.
// The upgrade must fail instead of writing back the stale key store, even though upgrades force conflicts.
func TestEncryptionSecretUpgradeRotationRace(t *testing.T) {
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		t.Skip("Run make test or set KUBEBUILDER_ASSETS to run the isolated API-server regression.")
	}
	runtimelog.SetLogger(logr.Discard())
	kubeconfig, client := startTestAPIServer(t)
	_, chart := encryptionSecretChart(t)

	for _, serverSideApply := range []bool{true, false} {
		t.Run(fmt.Sprintf("serverSideApply=%t", serverSideApply), func(t *testing.T) {
			namespace := fmt.Sprintf("encryption-race-%t", serverSideApply)
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
			upgradeClient := newUpgradeClient(configuration, namespace, false, true, time.Minute, serverSideApply)
			require.Equal(t, serverSideApply, upgradeClient.ForceConflicts)
			// Helm runs the post-renderer after the template has read the Secret and before it applies the manifest.
			upgradeClient.PostRenderer = postRenderFunc(func() { rotateEncryptionKey(t, secrets) })
			_, err = upgradeClient.Run("radius", chart, map[string]any{})
			require.ErrorContains(t, err, "the object has been modified")

			secret, err := secrets.Get(t.Context(), encryption.DefaultEncryptionKeySecretName, metav1.GetOptions{})
			require.NoError(t, err)
			var store encryption.KeyStore
			require.NoError(t, json.Unmarshal(secret.Data[encryption.DefaultEncryptionKeySecretKey], &store))
			require.Equal(t, 2, store.CurrentVersion, "The upgrade must not remove the rotated key.")
			require.Contains(t, store.Keys, "2")

			// The next upgrade reads the rotated Secret and succeeds.
			_, err = NewHelmClient().RunHelmUpgrade(configuration, chart, nil, "radius", namespace, false, true, time.Minute)
			require.NoError(t, err)
			upgraded, err := secrets.Get(t.Context(), secret.Name, metav1.GetOptions{})
			require.NoError(t, err)
			require.Equal(t, secret.Data, upgraded.Data)

			// A rollback renders the Secret the same way, so it must fail the same way.
			stored, err := configuration.Releases.Get("radius", 1)
			require.NoError(t, err)
			target, err := asRelease(stored)
			require.NoError(t, err)
			rollbackClient := newRollbackClient(configuration, namespace, 1, false, serverSideApply)
			rollbackClient.PostRenderer = postRenderFunc(func() { rotateEncryptionKey(t, secrets) })
			_, err = rollbackClient.Run("radius", target.Chart, target.Config)
			require.ErrorContains(t, err, "the object has been modified")
			rolledBack, err := secrets.Get(t.Context(), secret.Name, metav1.GetOptions{})
			require.NoError(t, err)
			store = encryption.KeyStore{}
			require.NoError(t, json.Unmarshal(rolledBack.Data[encryption.DefaultEncryptionKeySecretKey], &store))
			require.Equal(t, 3, store.CurrentVersion, "The rollback must not remove the rotated key.")
		})
	}
}

// TestEncryptionSecretRollback rolls back across upgrades that wrote the encryption Secret. The rollback must
// succeed and keep the live keys, whether or not a key was rotated after the target revision.
func TestEncryptionSecretRollback(t *testing.T) {
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		t.Skip("Run make test or set KUBEBUILDER_ASSETS to run the isolated API-server regression.")
	}
	runtimelog.SetLogger(logr.Discard())
	kubeconfig, client := startTestAPIServer(t)
	_, chart := encryptionSecretChart(t)

	tests := []struct {
		name string
		// rotateAfterTarget rotates a key after revision A instead of before it.
		rotateAfterTarget bool
	}{
		// Install, rotate, upgrade to A, upgrade to B, roll back to A. A's upgrade wrote the Secret, so the
		// resourceVersion stored in A's manifest is stale even though the keys haven't changed since.
		{name: "keys unchanged since target"},
		// Install, upgrade to A, upgrade to B, rotate, roll back to A. The rollback must keep the rotated key.
		{name: "key rotated since target", rotateAfterTarget: true},
	}
	for _, tt := range tests {
		for _, serverSideApply := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s/serverSideApply=%t", tt.name, serverSideApply), func(t *testing.T) {
				namespace := fmt.Sprintf("encryption-rollback-%t-%t", tt.rotateAfterTarget, serverSideApply)
				flags := genericclioptions.NewConfigFlags(false)
				flags.KubeConfig = &kubeconfig
				flags.Namespace = &namespace
				configuration := &helm.Configuration{}
				require.NoError(t, configuration.Init(flags, namespace, helmDriverSecret))
				installClient := newInstallClient(configuration, "radius", namespace, false, time.Minute)
				installClient.ServerSideApply = serverSideApply
				_, err := installClient.Run(chart, map[string]any{})
				require.NoError(t, err)

				secrets := client.CoreV1().Secrets(namespace)
				radiusClient := NewHelmClient()
				if !tt.rotateAfterTarget {
					rotateEncryptionKey(t, secrets)
				}
				revisionA, err := radiusClient.RunHelmUpgrade(configuration, chart, nil, "radius", namespace, false, true, time.Minute)
				require.NoError(t, err)
				_, err = radiusClient.RunHelmUpgrade(configuration, chart, nil, "radius", namespace, false, true, time.Minute)
				require.NoError(t, err)
				if tt.rotateAfterTarget {
					rotateEncryptionKey(t, secrets)
				}
				before, err := secrets.Get(t.Context(), encryption.DefaultEncryptionKeySecretName, metav1.GetOptions{})
				require.NoError(t, err)

				require.NoError(t, radiusClient.RunHelmRollback(configuration, "radius", revisionA.Version, false))

				after, err := secrets.Get(t.Context(), before.Name, metav1.GetOptions{})
				require.NoError(t, err)
				require.Equal(t, before.UID, after.UID)
				require.Equal(t, before.Data, after.Data, "Rollback must keep the live key store.")
				require.Equal(t, before.Annotations["radius.dev/last-rotation"], after.Annotations["radius.dev/last-rotation"])
				var store encryption.KeyStore
				require.NoError(t, json.Unmarshal(after.Data[encryption.DefaultEncryptionKeySecretKey], &store))
				require.Equal(t, 2, store.CurrentVersion)

				latest, err := radiusClient.RunHelmGet(configuration, "radius")
				require.NoError(t, err)
				require.Equal(t, fmt.Sprintf("Rollback to %d", revisionA.Version), latest.Info.Description)
			})
		}
	}
}

type postRenderFunc func()

func (f postRenderFunc) Run(manifests *bytes.Buffer) (*bytes.Buffer, error) {
	f()
	return manifests, nil
}

// rotateEncryptionKey adds a new current key the way the rotation CronJob does, with kubectl patch.
func rotateEncryptionKey(t *testing.T, secrets typedcorev1.SecretInterface) {
	t.Helper()
	secret, err := secrets.Get(t.Context(), encryption.DefaultEncryptionKeySecretName, metav1.GetOptions{})
	require.NoError(t, err)
	var store encryption.KeyStore
	require.NoError(t, json.Unmarshal(secret.Data[encryption.DefaultEncryptionKeySecretKey], &store))
	rotatedAt := time.Now().UTC()
	store.CurrentVersion++
	store.Keys[strconv.Itoa(store.CurrentVersion)] = encryption.KeyData{
		Key:       base64.StdEncoding.EncodeToString(make([]byte, 32)),
		Version:   store.CurrentVersion,
		CreatedAt: rotatedAt.Format(time.RFC3339),
		ExpiresAt: rotatedAt.Add(90 * 24 * time.Hour).Format(time.RFC3339),
	}
	data, err := json.Marshal(store)
	require.NoError(t, err)
	patch, err := json.Marshal(corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{"radius.dev/last-rotation": rotatedAt.Format(time.RFC3339)}},
		Data:       map[string][]byte{encryption.DefaultEncryptionKeySecretKey: data},
	})
	require.NoError(t, err)
	_, err = secrets.Patch(t.Context(), secret.Name, types.MergePatchType, patch, metav1.PatchOptions{FieldManager: "kubectl-patch"})
	require.NoError(t, err)
}

func waitForEncryptionTimestampBoundary(t *testing.T, timestamp string) {
	t.Helper()
	instant, err := time.Parse(time.RFC3339, timestamp)
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		return time.Now().Truncate(time.Second).After(instant.Truncate(time.Second))
	}, 3*time.Second, 10*time.Millisecond)
}

func ownsEncryptionRotation(t *testing.T, secret *corev1.Secret, manager string, operation metav1.ManagedFieldsOperationType) bool {
	t.Helper()
	for _, fields := range secret.ManagedFields {
		if fields.Manager != manager || fields.Operation != operation || fields.FieldsV1 == nil {
			continue
		}
		var owned struct {
			Metadata struct {
				Annotations map[string]json.RawMessage `json:"f:annotations"`
			} `json:"f:metadata"`
		}
		require.NoError(t, json.Unmarshal(fields.FieldsV1.GetRawBytes(), &owned))
		if _, ok := owned.Metadata.Annotations["f:radius.dev/last-rotation"]; ok {
			return true
		}
	}
	return false
}
