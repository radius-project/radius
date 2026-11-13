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
	"helm.sh/helm/v4/pkg/chart/v2/loader"
	kubefake "k8s.io/client-go/kubernetes/fake"
)

func TestBootstrapPreparationRejectsUnsupportedObservationBeforeWrites(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"selfSigned", "caSecret", "issuerRef"} {
		for _, provider := range []string{"radius", "external"} {
			t.Run(fmt.Sprintf("%s/%s", mode, provider), func(t *testing.T) {
				t.Parallel()
				store, _ := bootstrapStoreFixture(t)
				selected, err := loader.Load("../../../deploy/Chart")
				require.NoError(t, err)
				values := map[string]any{"global": map[string]any{"rbac": map[string]any{
					"enabled": true, "tls": map[string]any{"provider": provider},
					"certificates": map[string]any{"mode": mode},
				}}}
				impl := &Impl{newRotationStore: func(string, string) (*rotationStore, error) { return store, nil }}
				installer, err := impl.prepareAggregationBootstrap(t.Context(), NewDefaultClusterOptions(), selected, nil, values, "", nil)
				require.ErrorIs(t, err, errCertificateObserverUnavailable)
				require.Nil(t, installer)
				for _, action := range store.kube.(*kubefake.Clientset).Actions() {
					require.Contains(t, []string{"get", "list"}, action.GetVerb())
				}
			})
		}
	}
}
