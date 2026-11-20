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
	"testing"

	"github.com/radius-project/radius/pkg/authz"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	releasecommon "helm.sh/helm/v4/pkg/release/common"
	releasev1 "helm.sh/helm/v4/pkg/release/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func providerHistoryRevision(t *testing.T, revision int, version string, status releasecommon.Status, provider string) *releasev1.Release {
	t.Helper()
	c := certificateTestChart(t)
	c.Metadata.Version = version
	require.NoError(t, unstructured.SetNestedField(c.Values, provider, "global", "rbac", "tls", "provider"))
	return &releasev1.Release{Name: radiusReleaseName, Version: revision, Chart: c, Config: map[string]any{"global": map[string]any{"rbac": map[string]any{"enabled": true}}}, Info: &releasev1.Info{Status: status}}
}

func TestProviderHistoryRollbackBeforeMutation(t *testing.T) {
	for _, form := range []string{"previous", "revision"} {
		for _, old := range []string{"radius", "external"} {
			for _, mode := range []string{"active", "off"} {
				t.Run(form+"/"+old+"/"+mode, func(t *testing.T) {
					next := "external"
					if old == "external" {
						next = "radius"
					}
					target := providerHistoryRevision(t, 1, "0.45.0", releasecommon.StatusSuperseded, next)
					installed := providerHistoryRevision(t, 2, "0.46.0", releasecommon.StatusDeployed, old)
					latest := providerHistoryRevision(t, 3, "0.47.0", releasecommon.StatusPendingUpgrade, next)
					if mode == "off" {
						// Only the saved target overrides distinguish these two charts.
						setValue(target.Chart.Values, old, "global", "rbac", "tls", "provider")
						target.Config = map[string]any{}
						setValue(target.Config, next, "global", "rbac", "tls", "provider")
					} else {
						// Only the deployed revision has a saved provider override.
						setValue(installed.Chart.Values, next, "global", "rbac", "tls", "provider")
						setValue(installed.Config, old, "global", "rbac", "tls", "provider")
					}
					c := newStageCluster(t, nil, false)
					m := NewMockHelmClient(gomock.NewController(t))
					// Any dependency Helm call or RunHelmRollback is an unexpected call.
					m.EXPECT().RunHelmHistory(gomock.Any(), radiusReleaseName).Return([]*releasev1.Release{target, installed, latest}, nil)
					impl := c.impl(m)
					c.kube.ClearActions()
					c.dynamic.ClearActions()
					var err error
					if form == "previous" {
						err = impl.RollbackRadius(t.Context(), "test-context")
					} else {
						err = impl.RollbackRadiusToRevision(t.Context(), "test-context", 1)
					}
					require.Empty(t, c.clusterWrites())
					require.ErrorIs(t, err, errProviderObservationUnavailable)
				})
			}
		}
	}
}

func TestProviderHistoryRollbackPreservesExternal(t *testing.T) {
	old := providerHistoryRevision(t, 2, "0.46.0", releasecommon.StatusDeployed, "radius")
	setValue(old.Config, "external", "global", "rbac", "tls", "provider")
	target := providerHistoryRevision(t, 1, "0.45.0", releasecommon.StatusSuperseded, "external")
	target.Config = nil
	mode, err := rollbackProviderMode([]*releasev1.Release{target, old}, 1)
	require.NoError(t, err)
	require.Equal(t, authz.ModeOff, mode)
}

func TestProviderHistoryRecheckRequiresDeployedRevision(t *testing.T) {
	old := providerHistoryRevision(t, 1, "0.45.0", releasecommon.StatusDeployed, "radius")
	failed := *old
	failed.Info = &releasev1.Info{Status: releasecommon.StatusFailed}
	h := NewMockHelmClient(gomock.NewController(t))
	h.EXPECT().RunHelmHistory(gomock.Any(), radiusReleaseName).Return([]*releasev1.Release{old}, nil)
	h.EXPECT().RunHelmHistory(gomock.Any(), radiusReleaseName).Return([]*releasev1.Release{&failed}, nil)
	opts := NewDefaultClusterOptions()
	opts.RadiusReleaseChart, opts.RadiusReleaseValues = old.Chart, old.Config
	_, plan, _, err := (&Impl{Helm: h}).selectActionCertificates(t.Context(), &opts, old.Chart, nil, map[string]any{}, "", true)
	require.NoError(t, err)
	require.ErrorContains(t, plan.recheck(), "revision changed")
}

func TestProviderHistoryPublicMaintenance(t *testing.T) {
	for _, command := range []string{"reinstall", "upgrade", "preflight"} {
		for _, deployed := range []bool{true, false} {
			name := "no deployed revision"
			if deployed {
				name = "deployed external to radius"
			}
			t.Run(command+"/"+name, func(t *testing.T) {
				c := newStageCluster(t, nil, false)
				h := NewMockHelmClient(gomock.NewController(t))
				i := c.impl(h)
				old := providerHistoryRevision(t, 1, "0.45.0", releasecommon.StatusDeployed, "external")
				latest := providerHistoryRevision(t, 2, "0.46.0", releasecommon.StatusFailed, "radius")
				history := []*releasev1.Release{latest}
				if deployed {
					history = append(history, old)
				}
				opts := NewDefaultClusterOptions()
				opts.Radius.ChartPath, opts.RadiusInstalled, opts.RadiusReleaseValues = "chart", true, old.Config
				h.EXPECT().LoadChart("chart").Return(latest.Chart, nil)
				h.EXPECT().RunHelmHistory(gomock.Any(), radiusReleaseName).Return(history, nil)
				run := i.UpgradeRadius
				switch command {
				case "reinstall":
					opts.Radius.Reinstall, run = true, i.InstallRadius
				case "preflight":
					run = i.CheckRadiusCertificates
				}
				c.kube.ClearActions()
				c.dynamic.ClearActions()
				err := run(t.Context(), opts, "test-context")
				if deployed {
					require.ErrorIs(t, err, errProviderObservationUnavailable)
				} else {
					require.ErrorContains(t, err, "no deployed Radius revision")
				}
				require.Empty(t, c.clusterWrites())
			})
		}
	}
}
