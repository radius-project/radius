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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	kubefake "k8s.io/client-go/kubernetes/fake"
)

func TestLegacyBootstrapCannotResumeThroughTokenEntryPoint(t *testing.T) {
	t.Parallel()
	for _, scope := range []rotationScope{rotationComponentsAndAggregation, rotationAggregationOnly} {
		for _, operation := range []string{"install", "upgrade", "rollback", "uninstall"} {
			t.Run(fmt.Sprintf("%s/%s", scope, operation), func(t *testing.T) {
				t.Parallel()
				store, state := bootstrapStoreFixture(t)
				state.Scope = scope
				before, _, err := store.beginBootstrap(t.Context(), state)
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
