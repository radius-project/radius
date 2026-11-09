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
	"testing"
	"time"

	"github.com/radius-project/radius/pkg/cli/clierrors"
	"github.com/stretchr/testify/require"
	"helm.sh/helm/v4/pkg/chart/v2/loader"
	releasecommon "helm.sh/helm/v4/pkg/release/common"
	releasev1 "helm.sh/helm/v4/pkg/release/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestCertManagerPendingReleaseWithHelmStorage(t *testing.T) {
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		t.Skip("Run make test or set KUBEBUILDER_ASSETS to run the isolated API-server regression.")
	}
	kubeconfig, kubeClient := startTestAPIServer(t)
	t.Setenv("KUBECONFIG", kubeconfig)
	for _, namespace := range []string{CertManagerNamespace, "other-workload"} {
		_, err := kubeClient.CoreV1().Namespaces().Create(t.Context(), &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: namespace},
		}, metav1.CreateOptions{})
		require.NoError(t, err)
	}
	foreignSecrets := kubeClient.CoreV1().Secrets("other-workload")
	foreign, err := foreignSecrets.Create(t.Context(), &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: "application-certificate", Labels: map[string]string{"cert-manager.io/certificate-name": "application"},
		},
		Data: map[string][]byte{"test-marker": []byte("preserve foreign certificate material")},
	}, metav1.CreateOptions{})
	require.NoError(t, err)

	chartPath := writeStubRadiusChart(t)
	chart, err := loader.Load(chartPath)
	require.NoError(t, err)
	helmClient := NewHelmClient()
	configuration := newTestHelmConfiguration(t, kubeconfig, CertManagerNamespace)

	for _, status := range []releasecommon.Status{
		releasecommon.StatusPendingInstall, releasecommon.StatusPendingUpgrade, releasecommon.StatusPendingRollback,
	} {
		for _, owned := range []bool{false, true} {
			for index, command := range []string{"install", "install --reinstall", "upgrade"} {
				t.Run(fmt.Sprintf("%s/owned=%t/%s", status, owned, command), func(t *testing.T) {
					name := fmt.Sprintf("cm-%s-%t-%d", status, owned, index)
					pending := &releasev1.Release{
						Name: name, Namespace: CertManagerNamespace, Version: 1,
						Chart: chart, Info: &releasev1.Info{Status: status},
					}
					if owned {
						pending.Labels = radManagedByLabels()
					}
					if status != releasecommon.StatusPendingInstall {
						deployed := *pending
						deployed.Info = &releasev1.Info{Status: releasecommon.StatusDeployed}
						require.NoError(t, configuration.Releases.Create(&deployed))
						pending.Version = 2
					}
					require.NoError(t, configuration.Releases.Create(pending))

					// Real Helm refuses the pending release without distinguishing an
					// active operation from an interrupted one.
					_, err := helmClient.RunHelmUpgrade(configuration, chart, nil, name, CertManagerNamespace, false, true, time.Minute)
					require.ErrorContains(t, err, "another operation (install/upgrade/rollback) is in progress")
					before, err := helmClient.RunHelmHistory(configuration, name)
					require.NoError(t, err)
					secrets := kubeClient.CoreV1().Secrets(CertManagerNamespace)
					metadataBefore, err := secrets.List(t.Context(), metav1.ListOptions{})
					require.NoError(t, err)

					options := NewDefaultClusterOptions()
					options.Contour.Disabled = true
					options.Radius.ChartPath = filepath.Join("..", "..", "..", "deploy", "Chart")
					options.CertManager.ChartPath = chartPath
					options.CertManager.ReleaseName = name
					options.Radius.SetArgs = []string{"global.rbac.dryRun=true"}
					impl := &Impl{Helm: helmClient}
					if command == "upgrade" {
						err = impl.UpgradeRadius(t.Context(), options, testKubeContext)
					} else {
						options.Radius.Reinstall = command == "install --reinstall"
						// Fresh enabled installs now enter the bootstrap driver
						// before this dependency gate. Exercise the gate against
						// real Helm storage without pretending the stub chart
						// implements configured-CA bootstrap.
						err = impl.ensureCertManager(t.Context(), options, testKubeContext, nil, options.Radius.Reinstall)
					}
					require.True(t, clierrors.IsFriendlyError(err), "%v", err)
					require.ErrorContains(t, err, string(status))
					require.ErrorContains(t, err, `helm --kube-context "test" -n cert-manager status `+name)
					after, err := helmClient.RunHelmHistory(configuration, name)
					require.NoError(t, err)
					require.Equal(t, before, after, "no rollback or release metadata change")
					metadataAfter, err := secrets.List(t.Context(), metav1.ListOptions{})
					require.NoError(t, err)
					require.Equal(t, metadataBefore.Items, metadataAfter.Items)
					stillForeign, err := foreignSecrets.Get(t.Context(), foreign.Name, metav1.GetOptions{})
					require.NoError(t, err)
					require.Equal(t, foreign, stillForeign)
					requireNoRelease(t, kubeconfig, radiusReleaseName)
				})
			}
		}
	}
}
