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
	"testing"
	"time"

	"github.com/radius-project/radius/pkg/authz"
	"github.com/stretchr/testify/require"
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
				require.NotEqual(t, "secrets", action.GetResource().Resource, "Off cleanup must not read or delete signing/public Secrets")
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
