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
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	helm "helm.sh/helm/v4/pkg/action"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/clientcmd"
	runtimelog "sigs.k8s.io/controller-runtime/pkg/log"
)

func TestRotationHelmTrustOwnership(t *testing.T) {
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		t.Skip("Set KUBEBUILDER_ASSETS for isolated CA rotation Helm tests.")
	}
	runtimelog.SetLogger(logr.Discard())
	for _, ssa := range []bool{false, true} {
		t.Run(fmt.Sprintf("serverSideApply=%t", ssa), func(t *testing.T) {
			kubeconfig, kube := startTestAPIServer(t)
			namespace := RadiusSystemNamespace
			flags := genericclioptions.NewConfigFlags(false)
			flags.KubeConfig, flags.Namespace = &kubeconfig, &namespace
			configuration := &helm.Configuration{}
			require.NoError(t, configuration.Init(flags, namespace, helmDriverSecret))
			_, chart := shippingTemplatesChart(t, "_helpers.tpl", "_rotation.tpl", "_aggregation-trust.tpl", "ucp/apiservice.yaml")
			install := newInstallClient(configuration, radiusReleaseName, namespace, false, time.Minute)
			install.ServerSideApply = ssa
			_, err := install.Run(chart, map[string]any{})
			require.NoError(t, err)
			config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
			require.NoError(t, err)
			client, err := dynamic.NewForConfig(config)
			require.NoError(t, err)
			store := &rotationStore{kube: kube, dynamic: client, namespace: namespace, release: radiusReleaseName, now: time.Now}
			require.NoError(t, withIdentityOperation(t.Context(), kube, namespace, func(operation *identityOperation) error {
				store.guardWrite = func(ctx context.Context) error {
					return operation.checkProviderJournal(ctx, radiusReleaseName, nil)
				}
				legacy, err := kube.CoreV1().Secrets(namespace).Get(t.Context(), "ucp-cert", metav1.GetOptions{})
				require.NoError(t, err)

				// A render constructed before the journal existed cannot overwrite a
				// completed rotation, including when Helm forces SSA conflicts.
				next, _, _ := testSigningCA(t, nil, nil, nil, nil)
				upgrade := newUpgradeClient(configuration, namespace, false, true, time.Minute, ssa)
				upgrade.PostRenderer = postRenderFunc(func() {
					state, err := store.begin(t.Context(), uuid.NewString(), next)
					require.NoError(t, err)
					require.NoError(t, store.reconcileTrust(t.Context(), state.ID))
					for _, phase := range rotationPhases[1:] {
						advanceRotationTest(t, store, phase)
					}
				})
				_, err = upgrade.Run(radiusReleaseName, chart, map[string]any{})
				require.ErrorContains(t, err, "the object has been modified")
				api, err := client.Resource(apiServiceResource).Get(t.Context(), radiusAPIService, metav1.GetOptions{})
				require.NoError(t, err)
				bundle, err := apiServiceBundle(api)
				require.NoError(t, err)
				require.Equal(t, next, bundle)

				upgrade = newUpgradeClient(configuration, namespace, false, true, time.Minute, ssa)
				_, err = upgrade.Run(radiusReleaseName, chart, map[string]any{})
				require.NoError(t, err, "a fresh render preserves the completed public trust")
				api, err = client.Resource(apiServiceResource).Get(t.Context(), radiusAPIService, metav1.GetOptions{})
				require.NoError(t, err)
				bundle, err = apiServiceBundle(api)
				require.NoError(t, err)
				require.Equal(t, next, bundle)
				current, err := kube.CoreV1().Secrets(namespace).Get(t.Context(), "ucp-cert", metav1.GetOptions{})
				require.NoError(t, err)
				require.Equal(t, legacy.Data, current.Data, "trust publication must not replace the legacy serving key")

				third, _, _ := testSigningCA(t, nil, nil, nil, nil)
				_, err = store.begin(t.Context(), uuid.NewString(), third)
				require.NoError(t, err)
				upgrade = newUpgradeClient(configuration, namespace, false, true, time.Minute, ssa)
				_, err = upgrade.Run(radiusReleaseName, chart, map[string]any{})
				require.ErrorContains(t, err, "rotation is unfinished")
				return nil
			}))
		})
	}
}
