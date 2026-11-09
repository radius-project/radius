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
	"errors"
	"strings"
	"testing"

	"github.com/radius-project/radius/pkg/authz/mtls"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	kubefake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func TestBootstrapPartialInstallPins(t *testing.T) {
	t.Parallel()
	for _, scenario := range []string{
		"normal", "partial", "lost pin write", "changed spec", "foreign marker",
		"foreign owner", "replaced", "deleted", "drift after pin", "incomplete manifest",
	} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()
			s, state := bootstrapStoreFixture(t)
			ctx := t.Context()
			state.Mode, state.Scope = "caSecret", rotationAggregationOnly
			state.CA, _, _ = testSigningCA(t, nil, nil, nil, nil)
			state.CAPinnedAt = s.now().UTC()
			state.Signer, state.SignerUID = "operator-ca", "signer-uid"
			journal, _, err := s.beginBootstrap(ctx, state)
			require.NoError(t, err)
			next := state
			next.Phase, next.ProofPort = bootstrapInstalling, 9090
			next.InstallValuesSHA256 = strings.Repeat("c", 64)
			next.TrustSHA256, _, err = mtls.TrustBundleDigest(next.CA, next.CAPinnedAt)
			require.NoError(t, err)
			_, err = s.updateBootstrap(ctx, journal, state, next)
			require.NoError(t, err)
			state = next

			client := s.dynamic.(*dynamicfake.FakeDynamicClient)
			var documents []string
			var first *unstructured.Unstructured
			for index, identity := range []struct {
				resource schema.GroupVersionResource
				kind     string
				name     string
			}{
				{apiServiceResource, "APIService", radiusAPIService},
				{schema.GroupVersionResource{Version: "v1", Resource: "services"}, "Service", "ucp"},
				{schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}, "Deployment", "ucp"},
			} {
				object := &unstructured.Unstructured{Object: map[string]any{
					"apiVersion": identity.resource.GroupVersion().String(), "kind": identity.kind,
					"spec": map[string]any{"fixture": "original"},
				}}
				object.SetName(identity.name)
				if identity.resource != apiServiceResource {
					object.SetNamespace(s.namespace)
				}
				object.SetAnnotations(map[string]string{
					aggregationBootstrapOwner: state.ID, "meta.helm.sh/release-name": s.release,
					"meta.helm.sh/release-namespace": s.namespace,
				})
				data, err := json.Marshal(object)
				require.NoError(t, err)
				documents = append(documents, string(data))
				object.SetUID(types.UID(identity.kind + "-uid"))
				object.SetResourceVersion("1")
				if index == 0 {
					first = object
					switch scenario {
					case "changed spec":
						object.Object["spec"] = map[string]any{"fixture": "changed"}
					case "foreign marker":
						annotations := object.GetAnnotations()
						annotations[aggregationBootstrapOwner] = "foreign"
						object.SetAnnotations(annotations)
					case "foreign owner":
						object.SetOwnerReferences([]metav1.OwnerReference{{APIVersion: "v1", Kind: "Secret", Name: "foreign", UID: "foreign"}})
					}
				}
				if scenario != "partial" || index != 2 {
					require.NoError(t, client.Tracker().Add(object))
				}
			}
			manifest := strings.Join(documents, "\n---\n")
			if scenario == "incomplete manifest" {
				manifest = strings.Join(documents[:2], "\n---\n")
			}
			lost := scenario == "lost pin write"
			s.kube.(*kubefake.Clientset).PrependReactor("update", "configmaps", func(k8stesting.Action) (bool, runtime.Object, error) {
				if lost {
					return true, nil, errors.New("lost bootstrap pin write")
				}
				return false, nil, nil
			})
			err = s.checkBootstrapInstall(ctx, state.ID, manifest, false)
			switch scenario {
			case "changed spec", "foreign marker", "foreign owner", "incomplete manifest":
				require.Error(t, err)
				return
			case "lost pin write":
				require.ErrorContains(t, err, "lost bootstrap pin write")
				lost = false
				require.NoError(t, s.checkBootstrapInstall(ctx, state.ID, manifest, false))
			default:
				require.NoError(t, err)
			}
			switch scenario {
			case "replaced":
				first.SetUID("replacement")
				require.NoError(t, client.Tracker().Update(apiServiceResource, first, ""))
			case "deleted":
				require.NoError(t, client.Tracker().Delete(apiServiceResource, "", first.GetName()))
			case "drift after pin":
				require.NoError(t, unstructured.SetNestedField(first.Object, "added", "spec", "unexpected"))
				require.NoError(t, client.Tracker().Update(apiServiceResource, first, ""))
			}
			err = s.checkBootstrapInstall(ctx, state.ID, manifest, true)
			if scenario == "partial" || scenario == "replaced" || scenario == "deleted" || scenario == "drift after pin" {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				_, pinned, err := s.readBootstrap(ctx)
				require.NoError(t, err)
				require.Len(t, pinned.Resources, 3)
			}
		})
	}
}
