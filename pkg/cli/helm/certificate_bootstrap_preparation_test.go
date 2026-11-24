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
	"go.uber.org/mock/gomock"
	chart "helm.sh/helm/v4/pkg/chart/v2"
	"helm.sh/helm/v4/pkg/chart/v2/loader"
	"helm.sh/helm/v4/pkg/storage/driver"
	apiextclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/client-go/kubernetes"
	kubefake "k8s.io/client-go/kubernetes/fake"
)

func TestBootstrapInstallRejectsStaticValuesBeforeWrites(t *testing.T) {
	for _, tt := range []struct{ override, want string }{
		{"global.rbac.podSecurityLevel=typo", "global.rbac.podSecurityLevel"},
		{"networkPolicies.enabled=true", "networkPolicies.controlPlaneCIDRs"},
		{"global.prometheus.port=3000", "conflicts with another Radius listener"},
	} {
		t.Run(tt.override, func(t *testing.T) {
			store := bootstrapStoreFixture(t)
			opts := NewDefaultClusterOptions()
			opts.Radius.ChartPath, opts.CertManager.ChartPath, opts.Contour.Disabled = "chart", "cert-manager", true
			opts.Radius.SetArgs = []string{"global.rbac.enabled=true", "global.rbac.tls.provider=external", tt.override}
			client := NewMockHelmClient(gomock.NewController(t))
			client.EXPECT().LoadChart("chart").Return(certificateTestChart(t), nil)
			client.EXPECT().RunHelmGet(gomock.Any(), certManagerReleaseName).Return(nil, driver.ErrReleaseNotFound).AnyTimes()
			client.EXPECT().LoadChart("cert-manager").Return(&chart.Chart{}, nil).AnyTimes()
			client.EXPECT().RunHelmInstall(gomock.Any(), gomock.Any(), gomock.Any(), certManagerReleaseName,
				gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
			impl := &Impl{Helm: client, newRotationStore: func(string, string) (*rotationStore, error) { return store, nil },
				newCertManagerClients: func(string) (kubernetes.Interface, apiextclient.Interface, error) {
					k, e := (fakeCertManagerCluster{}).clients()
					return k, e, nil
				},
			}
			err := impl.InstallRadius(t.Context(), opts, "test")
			require.ErrorContains(t, err, tt.want)
			for _, action := range store.kube.(*kubefake.Clientset).Actions() {
				require.Contains(t, []string{"get", "list"}, action.GetVerb(), "%s %s", action.GetVerb(), action.GetResource())
			}
		})
	}
}

func TestBootstrapPreparationDoesNotWriteOrRequestObserverCredentials(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"selfSigned", "caSecret", "issuerRef"} {
		for _, provider := range []string{"radius", "external"} {
			t.Run(fmt.Sprintf("%s/%s", mode, provider), func(t *testing.T) {
				t.Parallel()
				store := bootstrapStoreFixture(t)
				selected, err := loader.Load("../../../deploy/Chart")
				require.NoError(t, err)
				values := map[string]any{"global": map[string]any{"rbac": map[string]any{
					"enabled": true, "tls": map[string]any{"provider": provider},
					"certificates": map[string]any{"mode": mode},
				}}}
				impl := &Impl{newRotationStore: func(string, string) (*rotationStore, error) { return store, nil }}
				target := &certificateTarget{Mode: mode, Provider: provider}
				installer, err := impl.prepareAggregationBootstrap(t.Context(), NewDefaultClusterOptions(), selected, nil, values, "", nil, target)
				require.NoError(t, err)
				require.NotNil(t, installer)
				require.Same(t, target, installer.target)
				require.Equal(t, "preparing", installer.state.Phase)
				for _, action := range store.kube.(*kubefake.Clientset).Actions() {
					require.Contains(t, []string{"get", "list"}, action.GetVerb())
				}
			})
		}
	}
}
