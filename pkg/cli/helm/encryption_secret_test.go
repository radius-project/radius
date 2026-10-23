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
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/radius-project/radius/pkg/crypto/encryption"
	"github.com/stretchr/testify/require"
	helm "helm.sh/helm/v4/pkg/action"
	"helm.sh/helm/v4/pkg/chart/v2/loader"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
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

	environment := &envtest.Environment{}
	config, err := environment.Start()
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, environment.Stop())
	})
	client, err := kubernetes.NewForConfig(config)
	require.NoError(t, err)
	kubeconfig := filepath.Join(t.TempDir(), "kubeconfig")
	require.NoError(t, clientcmd.WriteToFile(clientcmdapi.Config{
		Clusters: map[string]*clientcmdapi.Cluster{
			"test": {Server: config.Host, CertificateAuthorityData: config.CAData},
		},
		AuthInfos: map[string]*clientcmdapi.AuthInfo{
			"test": {ClientCertificateData: config.CertData, ClientKeyData: config.KeyData},
		},
		Contexts: map[string]*clientcmdapi.Context{
			"test": {Cluster: "test", AuthInfo: "test"},
		},
		CurrentContext: "test",
	}, kubeconfig))

	// Use the shipping template, without workloads that require a kubelet.
	chartPath := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(chartPath, "templates"), 0700))
	require.NoError(t, os.WriteFile(filepath.Join(chartPath, "Chart.yaml"),
		[]byte("apiVersion: v2\nname: radius\nversion: 0.1.0\n"), 0600))
	for _, name := range []string{"_helpers.tpl", "dynamic-rp/secret.yaml"} {
		content, err := os.ReadFile(filepath.Join("..", "..", "..", "deploy", "Chart", "templates", filepath.FromSlash(name)))
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(chartPath, "templates", filepath.Base(name)), content, 0600))
	}
	chart, err := loader.Load(chartPath)
	require.NoError(t, err)

	for _, upgrader := range []string{"helm", "rad"} {
		for _, state := range []string{"fresh", "rotated", "legacy"} {
			t.Run(upgrader+"/"+state, func(t *testing.T) {
				// Keep this test serial: Helm derives its default manager from os.Args[0].
				originalBinary := os.Args[0]
				os.Args[0] = "rad"
				if upgrader == "helm" {
					os.Args[0] = "rad_linux_amd64"
				}
				t.Cleanup(func() { os.Args[0] = originalBinary })
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
				require.True(t, ownsEncryptionRotation(t, secret, os.Args[0], metav1.ManagedFieldsOperationApply))

				switch state {
				case "rotated":
					require.Eventually(t, func() bool {
						return time.Now().UTC().Format(time.RFC3339) != secret.Annotations["radius.dev/last-rotation"]
					}, 3*time.Second, 10*time.Millisecond)
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
					require.Eventually(t, func() bool {
						return time.Now().UTC().Format(time.RFC3339) != rotation
					}, 3*time.Second, 10*time.Millisecond)
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
		require.NoError(t, json.Unmarshal(fields.FieldsV1.Raw, &owned))
		if _, ok := owned.Metadata.Annotations["f:radius.dev/last-rotation"]; ok {
			return true
		}
	}
	return false
}
