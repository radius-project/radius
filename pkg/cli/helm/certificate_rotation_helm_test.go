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
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	helm "helm.sh/helm/v4/pkg/action"
	"helm.sh/helm/v4/pkg/chart/v2/loader"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"path/filepath"
)

func TestLegacyRotationHelmPreservesRecoveryJournal(t *testing.T) {
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		t.Skip("Set KUBEBUILDER_ASSETS for isolated CA rotation Helm tests.")
	}
	for _, ssa := range []bool{false, true} {
		t.Run(fmt.Sprintf("serverSideApply=%t", ssa), func(t *testing.T) {
			store, config, _ := newPlannedRotationTest(t)
			namespace := store.namespace
			kubeconfig := filepath.Join(t.TempDir(), "kubeconfig")
			require.NoError(t, clientcmd.WriteToFile(clientcmdapi.Config{
				Clusters:  map[string]*clientcmdapi.Cluster{"test": {Server: config.Host, CertificateAuthorityData: config.CAData}},
				AuthInfos: map[string]*clientcmdapi.AuthInfo{"test": {ClientCertificateData: config.CertData, ClientKeyData: config.KeyData}},
				Contexts:  map[string]*clientcmdapi.Context{"test": {Cluster: "test", AuthInfo: "test"}}, CurrentContext: "test",
			}, kubeconfig))
			flags := genericclioptions.NewConfigFlags(false)
			flags.KubeConfig, flags.Namespace = &kubeconfig, &namespace
			configuration := &helm.Configuration{}
			require.NoError(t, configuration.Init(flags, namespace, helmDriverSecret))
			selected, err := loader.Load("../../../deploy/Chart")
			require.NoError(t, err)
			templates := selected.Templates[:0]
			for _, file := range selected.Templates {
				if strings.HasSuffix(file.Name, ".tpl") || file.Name == "templates/global/certificates.yaml" {
					templates = append(templates, file)
				}
			}
			selected.Templates = templates
			for _, complete := range []bool{false, true} {
				if complete {
					for _, phase := range rotationPhases[1:] {
						advanceRotationTest(t, store, phase)
					}
				}
				journal, _, err := store.read(t.Context())
				require.NoError(t, err)
				before, err := store.dynamic.Resource(apiServiceResource).Get(t.Context(), radiusAPIService, metav1.GetOptions{})
				require.NoError(t, err)
				install := newInstallClient(configuration, radiusReleaseName, namespace, false, time.Minute)
				install.ServerSideApply = ssa
				_, err = install.Run(selected, map[string]any{})
				require.ErrorContains(t, err, "Legacy or foreign CA journal")
				after, err := store.dynamic.Resource(apiServiceResource).Get(t.Context(), radiusAPIService, metav1.GetOptions{})
				require.NoError(t, err)
				require.Equal(t, before.Object["spec"], after.Object["spec"])
				require.Equal(t, before.GetAnnotations(), after.GetAnnotations())
				retained, _, err := store.read(t.Context())
				require.NoError(t, err)
				require.Equal(t, journal, retained)
			}
		})
	}
}
