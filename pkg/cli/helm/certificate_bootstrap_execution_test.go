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
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	kubefake "k8s.io/client-go/kubernetes/fake"
)

func TestLegacyBootstrapCannotResumeThroughTokenEntryPoint(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{`{"version":1,"phase":"complete"}`, `{"version":1,"phase":"preparing"}`, "invalid", ""} {
		for _, operation := range []string{"install", "upgrade", "rollback", "uninstall"} {
			t.Run(fmt.Sprintf("%s/%s", raw, operation), func(t *testing.T) {
				t.Parallel()
				store := bootstrapStoreFixture(t)
				before, err := store.kube.CoreV1().ConfigMaps(store.namespace).Create(t.Context(), &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{Name: aggregationBootstrapName, Namespace: store.namespace},
					Data:       map[string]string{rotationJournalKey: raw},
				}, metav1.CreateOptions{})
				require.NoError(t, err)
				kube := store.kube.(*kubefake.Clientset)
				kube.ClearActions()
				client := store.dynamic.(*dynamicfake.FakeDynamicClient)
				client.ClearActions()
				impl := &Impl{newRotationStore: func(string, string) (*rotationStore, error) { return store, nil }}
				err = impl.guardCertificateRotation(t.Context(), "", store.namespace, operation, nil)
				require.ErrorContains(t, err, "matching pre-token release")
				after, err := kube.CoreV1().ConfigMaps(store.namespace).Get(t.Context(), aggregationBootstrapName, metav1.GetOptions{})
				require.NoError(t, err)
				require.Equal(t, before, after, "a token-mode retry must preserve the old recovery record exactly")
				for _, action := range kube.Actions() {
					require.Contains(t, []string{"get", "list"}, action.GetVerb())
				}

				require.Empty(t, client.Actions())
			})
		}
	}
}

func TestLegacyBootstrapNamespaceIntent(t *testing.T) {
	for _, raw := range []string{"", "invalid", `{"version":1}`} {
		t.Run(raw, func(t *testing.T) {
			store := bootstrapStoreFixture(t)
			_, err := store.kube.CoreV1().Namespaces().Create(t.Context(), &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: store.namespace, Annotations: map[string]string{bootstrapNamespaceIntent: raw}},
			}, metav1.CreateOptions{})
			require.NoError(t, err)
			impl := &Impl{newRotationStore: func(string, string) (*rotationStore, error) { return store, nil }}
			require.ErrorContains(t, impl.guardCertificateRotation(t.Context(), "", store.namespace, "upgrade", nil), "matching pre-token release")
		})
	}
}
