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
	"time"

	"github.com/radius-project/radius/pkg/authz"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	chart "helm.sh/helm/v4/pkg/chart/v2"
	releasecommon "helm.sh/helm/v4/pkg/release/common"
	releasev1 "helm.sh/helm/v4/pkg/release/v1"
	coordinationv1 "k8s.io/api/coordination/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	kubefake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func TestClearAggregationInjection(t *testing.T) {
	t.Parallel()
	for _, scenario := range []string{"success", "foreign", "changed API", "lease loss", "canceled before patch"} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			api := &unstructured.Unstructured{Object: map[string]any{
				"apiVersion": "apiregistration.k8s.io/v1", "kind": "APIService",
				"metadata": map[string]any{"name": ucpAPIServiceName, "uid": "api-one", "resourceVersion": "1",
					"labels": map[string]any{helmManagedByLabel: helmManagedByValue},
					"annotations": map[string]any{
						helmReleaseNameAnnotation: radiusReleaseName, helmReleaseNamespaceAnnotation: RadiusSystemNamespace,
						injectCAFromSecretAnnotation: "radius-system/old-signing-secret", "cert-manager.io/inject-apiserver-ca": "true", "keep": "unchanged",
					}},
				"spec": map[string]any{"caBundle": "not-modified-by-annotation-cleanup"},
			}}
			if scenario == "foreign" {
				api.SetLabels(nil)
			}
			kube := kubefake.NewClientset()
			client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme(), api)
			store := &rotationStore{kube: kube, dynamic: client, namespace: RadiusSystemNamespace, release: radiusReleaseName, now: time.Now}
			kube.PrependReactor("create", "leases", func(action k8stesting.Action) (bool, runtime.Object, error) {
				lease := action.(k8stesting.CreateAction).GetObject().(*coordinationv1.Lease)
				lease.UID, lease.ResourceVersion = "lease-one", "1"
				return false, nil, nil
			})
			gets := 0
			kube.PrependReactor("get", "leases", func(action k8stesting.Action) (bool, runtime.Object, error) {
				gets++
				if gets >= 4 {
					if scenario == "canceled before patch" {
						cancel()
					}
					if scenario == "lease loss" {
						return true, &coordinationv1.Lease{}, nil
					}
				}
				return false, nil, nil
			})
			patches := 0
			client.PrependReactor("patch", "apiservices", func(action k8stesting.Action) (bool, runtime.Object, error) {
				patches++
				if scenario == "changed API" {
					changed := api.DeepCopy()
					changed.SetResourceVersion("2")
					require.NoError(t, client.Tracker().Update(apiServicesResource, changed, ""))
				}
				return false, nil, nil
			})
			impl := &Impl{newRotationStore: func(string, string) (*rotationStore, error) { return store, nil }}
			err := impl.clearAggregationInjection(ctx, "", RadiusSystemNamespace, radiusReleaseName)
			live, readErr := client.Resource(apiServicesResource).Get(t.Context(), api.GetName(), metav1.GetOptions{})
			require.NoError(t, readErr)
			if scenario == "success" {
				require.NoError(t, err)
				require.Equal(t, "unchanged", live.GetAnnotations()["keep"])
				require.Empty(t, live.GetAnnotations()[injectCAFromSecretAnnotation])
				require.Empty(t, live.GetAnnotations()["cert-manager.io/inject-apiserver-ca"])
				require.Equal(t, 1, patches)
			} else {
				require.Error(t, err)
				require.Equal(t, api.GetAnnotations(), live.GetAnnotations())
				if scenario != "changed API" {
					require.Zero(t, patches)
				}
				if scenario == "canceled before patch" {
					require.ErrorIs(t, err, context.Canceled)
				}
			}
			require.Equal(t, api.Object["spec"], live.Object["spec"])
			for _, action := range kube.Actions() {
				if action.GetResource().Resource == "secrets" {
					require.Equal(t, "get", action.GetVerb(), "Off cleanup must not mutate Secrets")
					require.Equal(t, ucpAPIServerTrustSecretName, action.(k8stesting.GetAction).GetName(),
						"only the permanent public source may be inspected for orphaned journal markers")
				}
			}
		})
	}
}

func TestAggregationStageCanceledBeforeReadsOrRender(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	for _, mode := range []authz.Mode{authz.ModeOff, authz.ModeDryRun, authz.ModeEnforce} {
		err := (&Impl{}).stageUCPAPIServerCA(ctx, "", RadiusSystemNamespace, radiusReleaseName, mode,
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
	require.NoError(t, settleUCPAPIServerCA(t.Context(), "", kube, client, RadiusSystemNamespace))
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
	render := func(context.Context) (string, bool, error) {
		t.Fatal("Off preflight must not render or clean up annotations")
		return "", false, nil
	}
	require.NoError(t, (&Impl{}).checkAggregationBeforeDependencies(t.Context(), options, "", nil, render))
	options.Radius.SetArgs = []string{"global.rbac.dryRun=true"}
	err := (&Impl{}).checkAggregationBeforeDependencies(t.Context(), options, "", nil,
		func(context.Context) (string, bool, error) { return "", true, nil })
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
