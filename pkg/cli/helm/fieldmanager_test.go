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
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
	helm "helm.sh/helm/v4/pkg/action"
	"helm.sh/helm/v4/pkg/chart/v2/loader"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	corev1ac "k8s.io/client-go/applyconfigurations/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	runtimelog "sigs.k8s.io/controller-runtime/pkg/log"
)

// TestUpgradeTakesOverFields upgrades a value that another manager also owns. Radius must apply as rad and take
// the field over, as client-side apply did with Helm 3, instead of failing with a server-side apply conflict.
func TestUpgradeTakesOverFields(t *testing.T) {
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		t.Skip("Run make test or set KUBEBUILDER_ASSETS to run the isolated API-server regression.")
	}
	runtimelog.SetLogger(logr.Discard())
	kubeconfig, client := startTestAPIServer(t)

	chartPath := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(chartPath, "templates"), 0700))
	require.NoError(t, os.WriteFile(filepath.Join(chartPath, "Chart.yaml"),
		[]byte("apiVersion: v2\nname: radius\nversion: 0.1.0\n"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(chartPath, "values.yaml"), []byte("image: \"0.61\"\n"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(chartPath, "templates", "configmap.yaml"), []byte(
		"apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: field-manager\ndata:\n  image: {{ .Values.image | quote }}\n"), 0600))
	chart, err := loader.Load(chartPath)
	require.NoError(t, err)

	tests := []struct {
		name            string
		serverSideApply bool
		otherManager    string
		otherOperation  metav1.ManagedFieldsOperationType
	}{
		{name: "release asset name", serverSideApply: true, otherManager: "rad_linux_amd64", otherOperation: metav1.ManagedFieldsOperationApply},
		{name: "kubectl edit", serverSideApply: true, otherManager: "kubectl-edit", otherOperation: metav1.ManagedFieldsOperationUpdate},
		{name: "client-side apply release", serverSideApply: false, otherManager: "kubectl-edit", otherOperation: metav1.ManagedFieldsOperationUpdate},
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			namespace := fmt.Sprintf("field-manager-%d", i)
			flags := genericclioptions.NewConfigFlags(false)
			flags.KubeConfig = &kubeconfig
			flags.Namespace = &namespace
			configuration := &helm.Configuration{}
			require.NoError(t, configuration.Init(flags, namespace, helmDriverSecret))

			installClient := newInstallClient(configuration, "radius", namespace, false, time.Minute)
			installClient.ServerSideApply = tt.serverSideApply
			_, err := installClient.Run(chart, map[string]any{})
			require.NoError(t, err)
			require.Equal(t, tt.serverSideApply, usesServerSideApply(configuration, "radius"))

			configMaps := client.CoreV1().ConfigMaps(namespace)
			if tt.otherOperation == metav1.ManagedFieldsOperationApply {
				_, err = configMaps.Apply(t.Context(),
					corev1ac.ConfigMap("field-manager", namespace).WithData(map[string]string{"image": "0.61"}),
					metav1.ApplyOptions{FieldManager: tt.otherManager})
			} else {
				_, err = configMaps.Patch(t.Context(), "field-manager", types.MergePatchType,
					[]byte(`{"data":{"image":"0.61-custom"}}`), metav1.PatchOptions{FieldManager: tt.otherManager})
			}
			require.NoError(t, err)
			configMap, err := configMaps.Get(t.Context(), "field-manager", metav1.GetOptions{})
			require.NoError(t, err)
			require.Contains(t, imageOwners(t, configMap), tt.otherManager)

			_, err = NewHelmClient().RunHelmUpgrade(configuration, chart, map[string]any{"image": "0.62.0"}, "radius", namespace, false, true, time.Minute)
			require.NoError(t, err)

			configMap, err = configMaps.Get(t.Context(), "field-manager", metav1.GetOptions{})
			require.NoError(t, err)
			require.Equal(t, "0.62.0", configMap.Data["image"])
			require.NotContains(t, imageOwners(t, configMap), tt.otherManager)
			if tt.serverSideApply {
				require.Equal(t, []string{FieldManager}, imageOwners(t, configMap))
			}
		})
	}
}

// imageOwners returns the managers that own data.image on the ConfigMap.
func imageOwners(t *testing.T, configMap *corev1.ConfigMap) []string {
	t.Helper()
	var owners []string
	for _, fields := range configMap.ManagedFields {
		if fields.FieldsV1 == nil {
			continue
		}
		var owned struct {
			Data map[string]json.RawMessage `json:"f:data"`
		}
		require.NoError(t, json.Unmarshal(fields.FieldsV1.GetRawBytes(), &owned))
		if _, ok := owned.Data["f:image"]; ok {
			owners = append(owners, fields.Manager)
		}
	}
	return owners
}

// startTestAPIServer starts a private API server and returns a kubeconfig path and a client for it.
func startTestAPIServer(t *testing.T) (string, kubernetes.Interface) {
	t.Helper()

	// A regression must fail on this unusable config, never reach a user's cluster.
	unusedKubeconfig := filepath.Join(t.TempDir(), "unused-kubeconfig")
	require.NoError(t, os.WriteFile(unusedKubeconfig, []byte("invalid kubeconfig"), 0600))
	t.Setenv("KUBECONFIG", unusedKubeconfig)
	t.Setenv("USE_EXISTING_CLUSTER", "true")
	useExistingCluster := false
	environment := &envtest.Environment{UseExistingCluster: &useExistingCluster}
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

	return kubeconfig, client
}
