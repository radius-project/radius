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
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"helm.sh/helm/v4/pkg/chart/common"
	chart "helm.sh/helm/v4/pkg/chart/v2"
	releasecommon "helm.sh/helm/v4/pkg/release/common"
	releasev1 "helm.sh/helm/v4/pkg/release/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

func TestProviderHelmRecordingAPIServer(t *testing.T) {
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		t.Skip("Set KUBEBUILDER_ASSETS for isolated provider Helm recording tests.")
	}
	path, kube := startTestAPIServer(t)
	for _, scenario := range []string{"complete", "owned pending", "interrupted", "foreign pending"} {
		t.Run(scenario, func(t *testing.T) {
			namespace := "provider-" + uuid.NewString()[:8]
			_, err := kube.CoreV1().Namespaces().Create(t.Context(), &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: namespace},
			}, metav1.CreateOptions{})
			require.NoError(t, err)
			config, err := initHelmConfig(&genericclioptions.ConfigFlags{KubeConfig: &path, Namespace: &namespace})
			require.NoError(t, err)
			selected := &chart.Chart{Metadata: &chart.Metadata{Name: radiusReleaseName, Version: "1.0.0", APIVersion: "v2"},
				Values: map[string]any{}, Templates: []*common.File{{Name: "templates/config.yaml", Data: []byte(`
apiVersion: v1
kind: ConfigMap
metadata:
  name: {{ .Values.name }}
  namespace: {{ .Release.Namespace }}
data:
  config: target
`)}}}
			create := func(name, data string) *corev1.ConfigMap {
				t.Helper()
				object, err := kube.CoreV1().ConfigMaps(namespace).Create(t.Context(), &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace,
						Labels: map[string]string{"app.kubernetes.io/managed-by": "Helm"},
						Annotations: map[string]string{"meta.helm.sh/release-name": radiusReleaseName,
							"meta.helm.sh/release-namespace": namespace}},
					Data: map[string]string{"config": data},
				}, metav1.CreateOptions{})
				require.NoError(t, err)
				return object
			}
			create("source-config", "source")
			target := create("prepared-config", "target")
			source := &releasev1.Release{Name: radiusReleaseName, Namespace: namespace, Version: 1,
				Chart: selected, Config: map[string]any{"name": "source-config"},
				Info:     &releasev1.Info{Status: releasecommon.StatusDeployed},
				Manifest: fmt.Sprintf("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: source-config\n  namespace: %s\ndata:\n  config: source\n", namespace)}
			require.NoError(t, config.Releases.Create(source))
			values := map[string]any{"name": target.Name}
			intent, err := newProviderHelmIntent(source, values, []string{target.Name})
			require.NoError(t, err)
			plan := providerTransitionPlan{ID: uuid.NewString(), Namespace: namespace, Release: radiusReleaseName, Helm: intent}
			if scenario == "owned pending" || scenario == "foreign pending" {
				label := plan.ID
				if scenario == "foreign pending" {
					label = uuid.NewString()
				}
				require.NoError(t, config.Releases.Create(&releasev1.Release{
					Name: radiusReleaseName, Namespace: namespace, Version: 2, Chart: selected, Config: copyValues(values),
					Info:   &releasev1.Info{Status: releasecommon.StatusPendingUpgrade},
					Labels: map[string]string{providerTransitionMarker: label},
				}))
			}
			err = withIdentityOperation(t.Context(), kube, namespace, func(operation *identityOperation) error {
				writes := 0
				interrupt := scenario == "interrupted"
				fence := func(ctx context.Context) error {
					writes++
					if interrupt && writes == 3 {
						return errors.New("injected final-write interruption")
					}
					return operation.check(ctx)
				}
				err := recordProviderRelease(t.Context(), config, plan, selected, values, 20*time.Second, fence)
				if scenario == "foreign pending" {
					require.ErrorContains(t, err, "unrelated Helm operation")
					require.Zero(t, writes)
					return nil
				}
				if scenario == "interrupted" {
					require.Error(t, err)
					interrupt = false
					require.NoError(t, recordProviderRelease(t.Context(), config, plan, selected, values, 20*time.Second, fence))
				} else {
					require.NoError(t, err)
				}
				require.Greater(t, writes, 3, "the release and resource writes must all pass the live transaction fence")
				_, latest, err := providerReleaseHistory(config, plan)
				require.NoError(t, err)
				require.Equal(t, releasecommon.StatusDeployed, latest.Info.Status)
				version := latest.Version
				require.NoError(t, recordProviderRelease(t.Context(), config, plan, selected, values, 20*time.Second, fence))
				_, latest, err = providerReleaseHistory(config, plan)
				require.NoError(t, err)
				require.Equal(t, version, latest.Version, "completed recording must not add another Helm revision")
				current, err := kube.CoreV1().ConfigMaps(namespace).Get(t.Context(), target.Name, metav1.GetOptions{})
				require.NoError(t, err)
				require.Equal(t, target.UID, current.UID)
				require.Equal(t, target.Data, current.Data)
				_, err = kube.CoreV1().ConfigMaps(namespace).Get(t.Context(), "source-config", metav1.GetOptions{})
				require.True(t, apierrors.IsNotFound(err), "recording recovery must tolerate Helm pruning source-only configuration")
				return nil
			})
			require.NoError(t, err)
		})
	}
}
