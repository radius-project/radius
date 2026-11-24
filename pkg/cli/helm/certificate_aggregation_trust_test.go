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
	"encoding/json"
	"fmt"
	"testing"

	"github.com/radius-project/radius/pkg/authz"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	chart "helm.sh/helm/v4/pkg/chart/v2"
	releasecommon "helm.sh/helm/v4/pkg/release/common"
	releasev1 "helm.sh/helm/v4/pkg/release/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	kubefake "k8s.io/client-go/kubernetes/fake"
)

func TestClearAggregationInjection(t *testing.T) {
	t.Parallel()
	for _, scenario := range []string{"no journal", initialCertificateJournal, rotationJournalName, identityProviderJournal, "orphaned source", "namespace intent"} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()
			store := bootstrapStoreFixture(t)
			kube := store.kube.(*kubefake.Clientset)
			client := store.dynamic.(*dynamicfake.FakeDynamicClient)
			ca := newTestCA(t, "radius-apiserver-ca")
			api := permanentTrustAPIService(ca.pem)
			require.NoError(t, client.Tracker().Add(api))
			namespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: store.namespace}}
			switch scenario {
			case "no journal":
			case "namespace intent":
				namespace.Annotations = map[string]string{initialCertificateIntent: "recorded"}
			case "orphaned source":
				source := permanentPublicTrustSecret(ca.pem)
				source.Annotations[initialCertificateIntent] = "lost-journal"
				require.NoError(t, kube.Tracker().Add(source))
			default:
				require.NoError(t, kube.Tracker().Add(&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{Name: scenario, Namespace: store.namespace},
					Data:       map[string]string{rotationJournalKey: `{"phase":"complete"}`},
				}))
			}
			require.NoError(t, kube.Tracker().Add(namespace))
			impl := &Impl{newRotationStore: func(string, string) (*rotationStore, error) { return store, nil }}
			err := impl.clearAggregationInjection(t.Context(), "", RadiusSystemNamespace, radiusReleaseName)
			live, readErr := client.Resource(apiServicesResource).Get(t.Context(), api.GetName(), metav1.GetOptions{})
			require.NoError(t, readErr)
			if scenario == "no journal" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
			require.Equal(t, api, live, "pre-Helm exclusion must never remove injection or change trust")
			for _, action := range append(kube.Actions(), client.Actions()...) {
				require.Equal(t, "get", action.GetVerb(), "preflight must not acquire a Lease or mutate resources")
			}
		})
	}
}

func TestAggregationStageCanceledBeforeReadsOrRender(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	for _, mode := range []authz.Mode{authz.ModeOff, authz.ModeDryRun, authz.ModeEnforce} {
		err := (&Impl{}).stageUCPAPIServerCA(ctx, "", RadiusSystemNamespace, radiusReleaseName, mode, mode,
			func(context.Context) (string, bool, error) {
				t.Fatal("canceled staging must not render or mutate")
				return "", false, nil
			})
		require.ErrorIs(t, err, context.Canceled)
	}
}

func TestRenderedAggregationTrustSkipsNonSecrets(t *testing.T) {
	t.Parallel()
	ca, _, _ := testSigningCA(t, nil, nil, nil, nil)
	source := permanentPublicTrustSecret(ca)
	source.Kind, source.APIVersion = "Secret", "v1"
	data, err := json.Marshal(source)
	require.NoError(t, err)
	manifest := "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: config\ndata:\n  config.yaml: not base64!\n---\n" + string(data)
	bundle, err := renderedAggregationTrust(manifest, RadiusSystemNamespace)
	require.NoError(t, err)
	require.Equal(t, ca, bundle)
	_, err = renderedAggregationTrust(manifest+"\n---\n"+string(data), RadiusSystemNamespace)
	require.ErrorContains(t, err, "ambiguous")
}

func TestSettleAggregationRetainsExactPermanentSource(t *testing.T) {
	t.Parallel()
	ca, _, _ := testSigningCA(t, nil, nil, nil, nil)
	source, api := permanentPublicTrustSecret(ca), permanentTrustAPIService(ca)
	kube := kubefake.NewClientset(source)
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme(), api)
	require.NoError(t, settleUCPAPIServerCA(t.Context(), "", kube, client, RadiusSystemNamespace, ""))
	live, err := kube.CoreV1().Secrets(RadiusSystemNamespace).Get(t.Context(), source.Name, metav1.GetOptions{})
	require.NoError(t, err)
	require.Equal(t, source, live)
	observed, err := client.Resource(apiServicesResource).Get(t.Context(), api.GetName(), metav1.GetOptions{})
	require.NoError(t, err)
	require.Equal(t, api, observed)
	require.GreaterOrEqual(t, len(client.Actions()), ucpAPIServerCASettleChecks)
	for _, action := range append(kube.Actions(), client.Actions()...) {
		require.Equal(t, "get", action.GetVerb(), "convergence must not rewrite or delete permanent trust")
	}
}

func TestAggregationPreflightDoesNotCleanUpOff(t *testing.T) {
	t.Parallel()
	options := NewDefaultClusterOptions()
	require.NoError(t, (&Impl{newRotationStore: emptyRotationTestStore}).checkBeforeCertificateDependencies(t.Context(), options, "", nil, &certificateTarget{mode: authz.ModeOff}))
	err := (&Impl{}).checkBeforeCertificateDependencies(t.Context(), options, "", nil, &certificateTarget{Mode: "selfSigned", mode: authz.ModeDryRun})
	require.ErrorIs(t, err, errCertificateObserverUnavailable)
}

func TestRollbackUnsupportedAggregationStopsBeforeDependencies(t *testing.T) {
	t.Parallel()
	for _, explicit := range []bool{false, true} {
		t.Run(fmt.Sprint(explicit), func(t *testing.T) {
			t.Parallel()
			client := NewMockHelmClient(gomock.NewController(t))
			client.EXPECT().RunHelmHistory(gomock.Any(), radiusReleaseName).Return([]*releasev1.Release{
				{Version: 1, Config: authzOnValues, Chart: &chart.Chart{Metadata: &chart.Metadata{Version: "0.45.0"}},
					Info: &releasev1.Info{Status: releasecommon.StatusSuperseded}},
				{Version: 2, Config: map[string]any{}, Chart: &chart.Chart{Metadata: &chart.Metadata{Version: "0.46.0"}},
					Info: &releasev1.Info{Status: releasecommon.StatusDeployed}},
			}, nil)
			impl := &Impl{Helm: client, newRotationStore: emptyRotationTestStore}
			var err error
			if explicit {
				err = impl.RollbackRadiusToRevision(t.Context(), "test-context", 1)
			} else {
				err = impl.RollbackRadius(t.Context(), "test-context")
			}
			require.ErrorIs(t, err, errCertificateObserverUnavailable)
		})
	}
}
