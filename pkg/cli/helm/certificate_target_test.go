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
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	helm "helm.sh/helm/v4/pkg/action"
	"helm.sh/helm/v4/pkg/chart/common"
	chart "helm.sh/helm/v4/pkg/chart/v2"
	"helm.sh/helm/v4/pkg/chart/v2/loader"
	"helm.sh/helm/v4/pkg/release"
	releasecommon "helm.sh/helm/v4/pkg/release/common"
	releasev1 "helm.sh/helm/v4/pkg/release/v1"
	"helm.sh/helm/v4/pkg/storage/driver"
	corev1 "k8s.io/api/core/v1"
	apiextclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
)

func certificateTestChart(t *testing.T) *chart.Chart {
	t.Helper()
	c, err := loader.Load(filepath.Join("..", "..", "..", "deploy", "Chart"))
	require.NoError(t, err)
	return c
}

func expectRadiusBaseline(t *testing.T, client *MockHelmClient, options ClusterOptions) {
	t.Helper()
	c := options.RadiusReleaseChart
	if c == nil {
		c = certificateTestChart(t)
	}
	client.EXPECT().RunHelmHistory(gomock.Any(), options.Radius.ReleaseName).Return([]*releasev1.Release{{
		Name: options.Radius.ReleaseName, Namespace: options.Radius.Namespace, Version: 1,
		Chart: c, Config: options.RadiusReleaseValues, Info: &releasev1.Info{Status: releasecommon.StatusDeployed},
	}}, nil).MinTimes(1)
}

func TestInstalledChartDefaultsCannotSwitchIssuer(t *testing.T) {
	ca, _, _ := testSigningCA(t, nil, nil, nil, nil)
	old, selected := certificateTestChart(t), certificateTestChart(t)
	old.Values = resolveEffectiveChartValues(old.Values, nil, issuerReleaseValues(ca))
	stored := map[string]any{}
	setValue(stored, true, "global", "rbac", "enabled")
	runInstalledChartSwitchCommands(t, old, selected, stored, "selected-context")
}

func TestExternalProviderChecksActualUpgrade(t *testing.T) {
	c := certificateHelperChart(certificateTestChart(t))
	c.Templates = append(c.Templates, &common.File{Name: "templates/upgrade-only.yaml", Data: []byte(`{{ if .Release.IsUpgrade }}
apiVersion: v1
kind: Pod
metadata: {name: receiver}
spec:
  volumes: [{name: tls, projected: {sources: [{secret: {name: ucp-spiffe}}]}}]
{{ end }}`)})
	opts := NewDefaultClusterOptions()
	opts.Radius.ChartPath = "chart"
	opts.Radius.SetArgs = []string{"global.rbac.enabled=true", "global.rbac.tls.provider=external"}
	mock := NewMockHelmClient(gomock.NewController(t))
	mock.EXPECT().LoadChart("chart").Return(c, nil)
	opts.RadiusReleaseChart = c
	expectRadiusBaseline(t, mock, opts)
	impl := &Impl{Helm: mock, newRotationStore: emptyRotationTestStore, contourCluster: &fakeContourCluster{}}
	require.ErrorContains(t, impl.CheckRadiusCertificates(t.Context(), opts, "selected-context"), "external TLS provider cannot project")
	for _, action := range []string{"upgrade", "reinstall"} {
		t.Run(action, func(t *testing.T) {
			opts := opts
			opts.Contour.Disabled, opts.RadiusInstalled = true, true
			opts.RadiusReleaseValues = authzOnValues
			opts.CertManager.ChartPath = "cert-manager"
			client := NewMockHelmClient(gomock.NewController(t))
			client.EXPECT().LoadChart("chart").Return(c, nil)
			client.EXPECT().LoadChart("cert-manager").Return(&chart.Chart{}, nil).MaxTimes(1)
			expectRadiusBaseline(t, client, opts)
			client.EXPECT().RunHelmGet(gomock.Any(), opts.CertManager.ReleaseName).Return(&releasev1.Release{
				Labels: radManagedByLabels(), Chart: &chart.Chart{Metadata: &chart.Metadata{Version: CertManagerChartDefaultVersion}}, Info: &releasev1.Info{Status: releasecommon.StatusDeployed},
			}, nil).AnyTimes()
			cluster := newStageCluster(t, nil, true)
			impl := cluster.impl(client)
			run := impl.UpgradeRadius
			if action == "reinstall" {
				opts.Radius.Reinstall = true
				client.EXPECT().RunHelmGet(gomock.Any(), "radius").Return(&releasev1.Release{Chart: c, Info: &releasev1.Info{Status: releasecommon.StatusDeployed}}, nil)
				run = impl.InstallRadius
			}
			require.ErrorContains(t, run(t.Context(), opts, "selected-context"), "external TLS provider cannot project")
			require.Empty(t, cluster.clusterWrites())
		})
	}
}

func TestLegacySelfSignedChart(t *testing.T) {
	legacy := &chart.Chart{Metadata: &chart.Metadata{Name: "radius"}, Values: map[string]any{}}
	setCertificateValues(legacy.Values, map[string]any{"mode": "selfSigned"})
	if path := os.Getenv("A9_PREDECESSOR_CHART"); path != "" {
		var err error
		legacy, err = loader.Load(path)
		require.NoError(t, err)
	}
	current := map[string]any{}
	setValue(current, true, "global", "rbac", "dryRun")
	opts := NewDefaultClusterOptions()
	for _, installed := range []bool{false, true} {
		opts.RadiusInstalled = installed
		target, _, err := (&Impl{}).prepareCertificateTarget(t.Context(), opts, legacy, current, "selected")
		require.NoError(t, err)
		require.Equal(t, "selfSigned", target.Mode)
	}
	opts.Certificates.CASecret = "operator-ca"
	_, _, err := (&Impl{}).prepareCertificateTarget(t.Context(), opts, legacy, current, "selected")
	require.ErrorContains(t, err, "does not support certificate selection")
}

func TestExternalProviderOperatorTargetClusterSecret(t *testing.T) {
	c := certificateTestChart(t)
	opts := NewDefaultClusterOptions()
	opts.Radius.ChartPath = "chart"
	opts.Radius.SetArgs = []string{"global.rbac.enabled=true", "global.rbac.tls.provider=external",
		"global.targetCluster.enabled=true", "global.targetCluster.secretName=platform-spiffe"}
	client := NewMockHelmClient(gomock.NewController(t))
	client.EXPECT().LoadChart("chart").Return(c, nil)
	expectRadiusBaseline(t, client, opts)
	impl := &Impl{Helm: client, newRotationStore: emptyRotationTestStore, contourCluster: &fakeContourCluster{}}
	require.NoError(t, impl.CheckRadiusCertificates(t.Context(), opts, "selected"))
}

func TestExternalProviderReservedReceiver(t *testing.T) {
	// bicep-de is disabled by default, but its serving Secret is still reserved.
	for _, tc := range []struct{ name, manifest, want string }{
		{"certificate", "apiVersion: cert-manager.io/v1\nkind: Certificate\nmetadata: {name: extra}\nspec: {secretName: bicep-de-spiffe}", "cannot issue"},
		{"mount", "apiVersion: v1\nkind: Pod\nmetadata: {name: extra}\nspec: {volumes: [{name: tls, secret: {secretName: bicep-de-spiffe}}]}", "cannot mount"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := certificateHelperChart(certificateTestChart(t))
			c.Templates = append(c.Templates, &common.File{Name: "templates/extra.yaml", Data: []byte(tc.manifest)})
			opts := NewDefaultClusterOptions()
			opts.Radius.ChartPath = "chart"
			opts.Radius.SetArgs = []string{"global.rbac.enabled=true", "global.rbac.tls.provider=external"}
			client := NewMockHelmClient(gomock.NewController(t))
			client.EXPECT().LoadChart("chart").Return(c, nil)
			expectRadiusBaseline(t, client, opts)
			impl := &Impl{Helm: client, newRotationStore: emptyRotationTestStore, contourCluster: &fakeContourCluster{}}
			require.ErrorContains(t, impl.CheckRadiusCertificates(t.Context(), opts, "selected"), tc.want)
		})
	}
}

func byoHistory(t *testing.T, values map[string]any) []*releasev1.Release {
	t.Helper()
	f := newCertificateChartFixture(t)
	f.values = values
	rendered, err := f.renderChart(t)
	require.NoError(t, err)
	manifest := rendered["radius/templates/ucp/apiservice.yaml"] + "\n---\n" + rendered["radius/templates/global/certificates.yaml"]
	previous := certificateHelperChart(certificateTestChart(t))
	previous.Templates = append(previous.Templates, &common.File{Name: "templates/serving.yaml", Data: []byte(manifest)})
	current := cloneChart(previous)
	previous.Metadata.Version = "1.0.0"
	current.Metadata = &chart.Metadata{Name: "radius", Version: "1.1.0"}
	return []*releasev1.Release{
		{Version: 1, Chart: previous, Config: values, Manifest: manifest, Info: &releasev1.Info{Status: releasecommon.StatusSuperseded}},
		{Version: 2, Chart: current, Config: values, Manifest: manifest, Info: &releasev1.Info{Status: releasecommon.StatusDeployed}},
	}
}

func TestBYOActionMatchesPreparedTarget(t *testing.T) {
	ca, key, _ := testSigningCA(t, nil, nil, nil, nil)
	other, _, _ := testSigningCA(t, nil, nil, nil, nil)
	for _, tc := range []struct{ action, from, to, want string }{
		{action: "preflight"},
		{action: "upgrade", want: "dispatched"},
		{action: "reinstall", want: "dispatched"},
		{"preflight", "name: external-ca", "name: alternate-ca", "issuer"},
		{"upgrade", "name: external-ca", "name: alternate-ca", "issuer"},
		{"reinstall", "name: external-ca", "name: alternate-ca", "issuer"},
		{"rollback", "name: external-ca", "name: alternate-ca", "issuer"},
		{"preflight", `ca.crt: "` + base64.StdEncoding.EncodeToString(ca), `ca.crt: "` + base64.StdEncoding.EncodeToString(other), "trust"},
		{"preflight", fmt.Sprintf("ca.crt: %q", ca), fmt.Sprintf("ca.crt: %q", other), "selected internal CA"},
		{"preflight", "    name: radius-ca-issuer", "    name: alternate-ca", "selected internal issuer"},
		{"preflight", `secretName: "operator-ca"`, `secretName: "other-ca"`, "signing Secret"},
	} {
		t.Run(tc.action+"/"+tc.want, func(t *testing.T) {
			values := issuerReleaseValues(ca)
			if tc.want == "signing Secret" || tc.want == "selected internal issuer" {
				values = caReleaseValues("operator-ca", ca)
			}

			history := byoHistory(t, values)
			c := history[0].Chart
			if tc.from != "" {
				template := c.Templates[len(c.Templates)-1]
				original := string(template.Data)
				require.Contains(t, original, tc.from)
				template.Data = []byte(strings.Replace(original, tc.from, `{{ if .Release.IsUpgrade }}`+tc.to+`{{ else }}`+tc.from+`{{ end }}`, 1))
			}
			client := NewMockHelmClient(gomock.NewController(t))
			client.EXPECT().RunHelmHistory(gomock.Any(), "radius").Return(history, nil).MinTimes(1)
			cluster := newStageCluster(t, nil, false)
			impl := cluster.impl(client)
			impl.newCertificateClient = func(string) (kubernetes.Interface, error) {
				return fake.NewClientset(caTLSSecret("operator-ca", ca, key)), nil
			}
			opts := NewDefaultClusterOptions()
			opts.Radius.ChartPath, opts.RadiusReleaseValues, opts.Contour.Disabled, opts.RadiusInstalled = "chart", values, true, true
			var err error
			if tc.action == "rollback" {
				err = impl.RollbackRadiusToRevision(t.Context(), "selected", 1)
			} else {
				client.EXPECT().LoadChart("chart").Return(c, nil)
				run := impl.CheckRadiusCertificates
				if tc.action != "preflight" {
					client.EXPECT().MigrateControllerStrategy(gomock.Any(), gomock.Any(), c, gomock.Any(), "radius", RadiusSystemNamespace, true).Return(nil)
					if tc.want == "dispatched" {
						client.EXPECT().RunHelmUpgrade(gomock.Any(), c, gomock.Any(), "radius", RadiusSystemNamespace, gomock.Any(), true, opts.Radius.Timeout).Return(nil, errors.New(tc.want))
					}
					run = impl.UpgradeRadius
					if tc.action == "reinstall" {
						opts.Radius.Reinstall = true
						client.EXPECT().RunHelmGet(gomock.Any(), "radius").Return(history[1], nil).AnyTimes()
						run = impl.InstallRadius
					}
				}
				err = run(t.Context(), opts, "selected")
			}
			if tc.want == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tc.want)
			}
			require.Empty(t, cluster.clusterWrites())
		})
	}
}

func TestPublishedBYOBootstrapChecksSelectedIssuerBeforeApply(t *testing.T) {
	b, _ := initialControllerFixture(t, "issuerRef", "radius")
	manifest := byoHistory(t, issuerReleaseValues(b.target.Planes["aggregation"].CA))[0].Manifest
	require.Contains(t, manifest, "name: external-ca")
	err := b.runPublishedAction(t.Context(), &helm.Configuration{}, func(_ *helm.Configuration, strategy helm.DryRunStrategy) (release.Releaser, error) {
		require.Equal(t, helm.DryRunServer, strategy, "a mismatched issuer must stop Helm before apply")
		return &releasev1.Release{Manifest: manifest}, nil
	})
	require.ErrorContains(t, err, "does not reference the selected")
}

type onRenderCluster struct {
	fakeContourCluster
	before func()
}

func (c *onRenderCluster) RenderConfiguration(context, namespace string) (*helm.Configuration, error) {
	c.before()
	return c.fakeContourCluster.RenderConfiguration(context, namespace)
}

func TestRollbackCertificateObservations(t *testing.T) {
	ca, key, _ := testSigningCA(t, nil, nil, nil, nil)
	for _, revision := range []bool{false, true} {
		for _, change := range []string{"failed attempt", "signer", "history", "unchanged"} {
			t.Run(fmt.Sprintf("revision=%t/%s", revision, change), func(t *testing.T) {
				history := byoHistory(t, caReleaseValues("operator-ca", ca))
				client := NewMockHelmClient(gomock.NewController(t))
				cluster := newStageCluster(t, nil, false)
				impl := cluster.impl(client)
				secret := caTLSSecret("operator-ca", ca, key)
				keys := fake.NewClientset(secret)
				impl.newCertificateClient = func(string) (kubernetes.Interface, error) { return keys, nil }
				want := "signing Secret changed"
				if change == "failed attempt" {
					history[0].Config, history[0].Manifest = nil, ""
					history = append(history, &releasev1.Release{Version: 3, Chart: history[0].Chart, Info: &releasev1.Info{Status: releasecommon.StatusFailed}})
					want = "coordinated CA migration"
				} else {
					impl.contourCluster = &onRenderCluster{before: func() {
						if change == "history" {
							history = append(history, &releasev1.Release{Version: 3, Info: &releasev1.Info{Status: releasecommon.StatusPendingUpgrade}})
						} else if change == "signer" {
							secret.UID = "replacement"
							require.NoError(t, keys.Tracker().Update(corev1.SchemeGroupVersion.WithResource("secrets"), secret, RadiusSystemNamespace))
						}
					}}
					if change == "history" {
						want = "history changed"
					} else if change == "unchanged" {
						want = "dispatched"
						client.EXPECT().RunHelmRollback(gomock.Any(), radiusReleaseName, 1, true).Return(errors.New(want))
					}
				}
				client.EXPECT().RunHelmHistory(gomock.Any(), radiusReleaseName).DoAndReturn(func(*helm.Configuration, string) ([]*releasev1.Release, error) { return history, nil }).MinTimes(1)
				var err error
				if revision {
					err = impl.RollbackRadiusToRevision(t.Context(), "selected", 1)
				} else {
					err = impl.RollbackRadius(t.Context(), "selected")
				}
				require.ErrorContains(t, err, want)
				if change == "failed attempt" {
					require.Equal(t, []string{"create leases", "delete leases"}, cluster.clusterWrites())
					require.Empty(t, keys.Actions())
				} else {
					require.Empty(t, cluster.clusterWrites())
				}
			})
		}
	}
}

func TestExternalIssuerDependencyParity(t *testing.T) {
	ca, _, _ := testSigningCA(t, nil, nil, nil, nil)
	for _, unsupported := range []bool{false, true} {
		for _, preflight := range []bool{false, true} {
			t.Run(fmt.Sprintf("unsupported=%t/preflight=%t", unsupported, preflight), func(t *testing.T) {
				values := issuerReleaseValues(ca)
				history := byoHistory(t, values)
				opts := NewDefaultClusterOptions()
				opts.Radius.ChartPath, opts.RadiusReleaseValues, opts.Contour.Disabled = "chart", values, true
				client := NewMockHelmClient(gomock.NewController(t))
				client.EXPECT().LoadChart("chart").Return(history[1].Chart, nil)
				client.EXPECT().RunHelmHistory(gomock.Any(), "radius").Return(history, nil).MinTimes(1)
				version, want := CertManagerChartDefaultVersion, "cainjector"
				if unsupported {
					version, want = "v1.19.0", "older than the minimum"
				}
				kube, ext := (fakeCertManagerCluster{crd: true, controller: certManagerController("security", version, "")}).clients()
				require.NoError(t, kube.AppsV1().Deployments("security").Delete(t.Context(), "cert-manager-cainjector", metav1.DeleteOptions{}))
				keys := kube.(*fake.Clientset)
				keys.ClearActions()
				impl := &Impl{Helm: client, newRotationStore: emptyRotationTestStore, contourCluster: &fakeContourCluster{}, newCertManagerClients: func(string) (kubernetes.Interface, apiextclient.Interface, error) { return kube, ext, nil }}
				run := impl.UpgradeRadius
				if preflight {
					run = impl.CheckRadiusCertificates
				}
				require.ErrorContains(t, run(t.Context(), opts, "selected"), want)
				for _, action := range keys.Actions() {
					require.Contains(t, []string{"list", "get"}, action.GetVerb())
				}
			})
		}
	}
}

func TestPreflightRendersSelfSignedUpgrade(t *testing.T) {
	c := certificateHelperChart(certificateTestChart(t))
	c.Templates = append(c.Templates, &common.File{Name: "templates/upgrade.yaml", Data: []byte(`{{ if .Release.IsUpgrade }}{{ fail "invalid upgrade manifest" }}{{ end }}`)})
	opts := NewDefaultClusterOptions()
	opts.Radius.ChartPath = "chart"
	client := NewMockHelmClient(gomock.NewController(t))
	client.EXPECT().LoadChart("chart").Return(c, nil)
	expectRadiusBaseline(t, client, opts)
	impl := &Impl{Helm: client, newRotationStore: emptyRotationTestStore, contourCluster: &fakeContourCluster{}}
	require.ErrorContains(t, impl.CheckRadiusCertificates(t.Context(), opts, "selected-context"), "invalid upgrade manifest")
}

func TestCertificateTargetUsesChartTrustDomain(t *testing.T) {
	for _, tt := range []struct {
		name, wantErr string
		value         any
	}{
		{name: "absent"},
		{name: "empty", value: "", wantErr: "trustDomain must be a SPIFFE trust domain name"},
		{name: "number", value: 42, wantErr: "trustDomain must be a string"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			current := map[string]any{}
			setValue(current, true, "global", "rbac", "enabled")
			if tt.value != nil {
				setValue(current, tt.value, "global", "rbac", "spiffe", "trustDomain")
			}
			target, _, err := (&Impl{}).prepareCertificateTarget(t.Context(), NewDefaultClusterOptions(), certificateTestChart(t), current, "context")
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				require.Nil(t, target)
				return
			}
			require.NoError(t, err)
			require.Equal(t, "radius.internal", target.TrustDomain)
		})
	}
}

func TestRollbackChecksCertManagerBeforeSigningSecret(t *testing.T) {
	ca, _, _ := testSigningCA(t, nil, nil, nil, nil)
	values := caReleaseValues("operator-ca", ca)
	previous, current := certificateTestChart(t), certificateTestChart(t)
	previous.Metadata.Version, current.Metadata.Version = "1.0.0", "1.1.0"
	const manifest = "apiVersion: v1\nkind: Secret\nmetadata: {name: radius-apiserver-trust}"
	history := []*releasev1.Release{
		{Version: 1, Chart: previous, Config: values, Manifest: manifest, Info: &releasev1.Info{Status: releasecommon.StatusSuperseded}},
		{Version: 2, Chart: current, Config: values, Manifest: manifest, Info: &releasev1.Info{Status: releasecommon.StatusDeployed}},
	}
	for _, revision := range []bool{false, true} {
		t.Run(fmt.Sprintf("by-revision=%t", revision), func(t *testing.T) {
			m := NewMockHelmClient(gomock.NewController(t))
			m.EXPECT().RunHelmHistory(gomock.Any(), radiusReleaseName).Return(history, nil)
			cluster := newStageCluster(t, nil, false)
			impl := cluster.impl(m)
			impl.newCertManagerClients = func(k string) (kubernetes.Interface, apiextclient.Interface, error) {
				require.Equal(t, "selected", k)
				kube, ext := (&fakeCertManagerCluster{}).clients()
				return kube, ext, nil
			}
			keys := fake.NewClientset()
			impl.newCertificateClient = func(string) (kubernetes.Interface, error) { return keys, nil }
			var err error
			if revision {
				err = impl.RollbackRadiusToRevision(t.Context(), "selected", 1)
			} else {
				err = impl.RollbackRadius(t.Context(), "selected")
			}
			require.ErrorContains(t, err, "cert-manager is not installed")
			require.Empty(t, keys.Actions(), "no signing Secret access or import before readiness")
			require.Empty(t, cluster.clusterWrites())
		})
	}
}

func TestCertificateActionRenderOrdering(t *testing.T) {
	for _, action := range []string{"reinstall", "upgrade", "reset"} {
		for _, invalid := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/invalid-provider=%t", action, invalid), func(t *testing.T) {
				c := certificateHelperChart(certificateTestChart(t))
				c.Templates = append(c.Templates, &common.File{Name: "templates/action.yaml", Data: []byte(
					`{{ if .Release.IsUpgrade }}{{ fail "rendered upgrade" }}{{ else }}{{ fail "rendered install" }}{{ end }}`)})
				options := NewDefaultClusterOptions()
				options.Contour.Disabled = true
				options.RadiusInstalled = true
				options.RadiusReleaseValues = authzOnValues
				options.Radius.Reinstall = action == "reinstall"
				options.ResetValues = action == "reset"
				options.Radius.ChartPath = "chart"
				provider := "external"
				if invalid {
					provider = "unknown"
				}
				options.Radius.SetArgs = []string{"global.rbac.enabled=true", "global.rbac.tls.provider=" + provider}
				client := NewMockHelmClient(gomock.NewController(t))
				client.EXPECT().LoadChart("chart").Return(c, nil)
				expectRadiusBaseline(t, client, options)
				if !invalid {
					client.EXPECT().RunHelmGet(gomock.Any(), options.CertManager.ReleaseName).Return(nil, driver.ErrReleaseNotFound)
					if action == "reinstall" {
						client.EXPECT().RunHelmGet(gomock.Any(), "radius").Return(&releasev1.Release{Chart: c, Info: &releasev1.Info{Status: releasecommon.StatusDeployed}}, nil)
					}
				}
				impl := newStageCluster(t, nil, true).impl(client)
				if action == "fresh" && !invalid {
					options.CertManager.ChartPath = "cert-manager"
					client.EXPECT().LoadChart("cert-manager").Return(&chart.Chart{}, nil)
					installed := false
					client.EXPECT().RunHelmInstall(gomock.Any(), gomock.Any(), gomock.Any(), certManagerReleaseName, CertManagerNamespace, true, DefaultInstallTimeout, radManagedByLabels()).DoAndReturn(func(*helm.Configuration, *chart.Chart, map[string]any, string, string, bool, time.Duration, map[string]string) (*releasev1.Release, error) {
						installed = true
						return &releasev1.Release{}, nil
					})
					impl.newCertManagerClients = func(string) (kubernetes.Interface, apiextclient.Interface, error) {
						kube, ext := (fakeCertManagerCluster{}).clients()
						return kube, ext, nil
					}
					impl.contourCluster = &onRenderCluster{before: func() { require.True(t, installed, "fresh lookups need cert-manager installed before rendering") }}
				}
				run := impl.UpgradeRadius
				if options.Radius.Reinstall {
					run = impl.InstallRadius
				}
				err := run(t.Context(), options, "test-context")
				switch {
				case invalid:
					require.ErrorContains(t, err, "provider")
				default:
					require.ErrorContains(t, err, "rendered upgrade")
				}
			})
		}
	}
}

func TestSteadyBYORechecksSigningSecret(t *testing.T) {
	ca, key, _ := testSigningCA(t, nil, nil, nil, nil)
	opts := NewDefaultClusterOptions()
	opts.RadiusReleaseValues = caReleaseValues("operator-ca", ca)
	rendered := byoHistory(t, opts.RadiusReleaseValues)[0]
	selected, manifest := rendered.Chart, rendered.Manifest
	opts.Radius.ChartPath, opts.Contour.Disabled = "chart", true
	for _, boundary := range []string{"before migration", "before staging"} {
		for _, change := range []string{"signing Secret", "Radius revision"} {
			t.Run(boundary+"/"+change, func(t *testing.T) {
				rev := &releasev1.Release{Version: 1, Chart: selected, Config: opts.RadiusReleaseValues, Manifest: manifest, Info: &releasev1.Info{Status: releasecommon.StatusDeployed}}
				client := NewMockHelmClient(gomock.NewController(t))
				client.EXPECT().LoadChart("chart").Return(selected, nil)
				client.EXPECT().RunHelmHistory(gomock.Any(), radiusReleaseName).Return([]*releasev1.Release{rev}, nil).MinTimes(1)
				secret := caTLSSecret("operator-ca", ca, key)
				kube := fake.NewClientset(secret)
				mutate := func() {
					if change == "Radius revision" {
						rev.Version++
						return
					}
					secret.Data[corev1.TLSPrivateKeyKey] = nil
					_, err := kube.CoreV1().Secrets(opts.Radius.Namespace).Update(t.Context(), secret, metav1.UpdateOptions{})
					require.NoError(t, err)
				}
				client.EXPECT().RunHelmGet(gomock.Any(), opts.CertManager.ReleaseName).DoAndReturn(func(*helm.Configuration, string) (*releasev1.Release, error) {
					if boundary == "before migration" {
						mutate()
					}
					return nil, driver.ErrReleaseNotFound
				})
				if boundary == "before staging" {
					expectControllerStrategyMigration(client, true).DoAndReturn(func(context.Context, *helm.Configuration, *chart.Chart, map[string]any, string, string, bool) error {
						mutate()
						return nil
					})
				}
				cluster := newStageCluster(t, nil, true)
				impl := cluster.impl(client)
				impl.newCertificateClient = func(k string) (kubernetes.Interface, error) { require.Equal(t, "selected", k); return kube, nil }
				require.ErrorContains(t, impl.UpgradeRadius(t.Context(), opts, "selected"), change+" changed")
				requireOnlyCertificateGuardReads(t, cluster)
			})
		}
	}
}

func TestMigrationFailureWithImportedCA(t *testing.T) {
	ca, key, cert := testSigningCA(t, nil, nil, nil, nil)
	digest := sha256.Sum256(cert.Raw)
	name := fmt.Sprintf("radius-imported-ca-%x", digest[:16])
	options := NewDefaultClusterOptions()
	dir := t.TempDir()
	options.Certificates = CertificateOptions{CACert: filepath.Join(dir, "ca.pem"), CAKey: filepath.Join(dir, "key.pem")}
	require.NoError(t, os.WriteFile(options.Certificates.CACert, ca, 0600))
	require.NoError(t, os.WriteFile(options.Certificates.CAKey, key, 0600))
	options.Radius.ChartPath, options.Contour.Disabled = "chart", true
	options.RadiusReleaseValues = caReleaseValues(name, ca)
	c := certificateTestChart(t)
	revision := &releasev1.Release{Version: 1, Chart: c, Config: options.RadiusReleaseValues,
		Info: &releasev1.Info{Status: releasecommon.StatusDeployed}, Manifest: `apiVersion: v1
kind: Secret
metadata: {name: radius-apiserver-trust}`}
	client := NewMockHelmClient(gomock.NewController(t))
	client.EXPECT().LoadChart("chart").Return(c, nil)
	client.EXPECT().RunHelmHistory(gomock.Any(), "radius").Return([]*releasev1.Release{revision}, nil).MinTimes(1)
	client.EXPECT().RunHelmGet(gomock.Any(), options.CertManager.ReleaseName).Return(nil, driver.ErrReleaseNotFound)
	failure := errors.New("migration denied")
	expectControllerStrategyMigration(client, true).Return(failure)
	cluster := newStageCluster(t, nil, true)
	impl := cluster.impl(client)
	signing := fake.NewClientset(caTLSSecret(name, ca, key))
	impl.newCertificateClient = func(string) (kubernetes.Interface, error) { return signing, nil }
	require.ErrorIs(t, impl.UpgradeRadius(t.Context(), options, "test-context"), failure)
	require.NotEmpty(t, signing.Actions())
	for _, action := range signing.Actions() {
		require.Equal(t, "get", action.GetVerb())
	}
	requireOnlyCertificateGuardReads(t, cluster)
}

func requireOnlyCertificateGuardReads(t *testing.T, cluster *stageCluster) {
	t.Helper()
	for _, action := range append(cluster.kube.Actions(), cluster.dynamic.Actions()...) {
		require.Contains(t, []string{"get", "list"}, action.GetVerb())
	}
}

func runInstalledChartSwitchCommands(t *testing.T, old, selected *chart.Chart, stored map[string]any, kubeContext string) {
	t.Helper()
	for _, command := range []string{"reinstall", "upgrade", "preflight-only"} {
		t.Run(command, func(t *testing.T) {
			client := NewMockHelmClient(gomock.NewController(t))
			client.EXPECT().LoadChart("chart").Return(selected, nil)
			client.EXPECT().RunHelmHistory(gomock.Any(), radiusReleaseName).Return([]*releasev1.Release{
				{Chart: selected, Version: 2, Info: &releasev1.Info{Status: releasecommon.StatusFailed}},
				{Chart: old, Config: stored, Version: 1, Info: &releasev1.Info{Status: releasecommon.StatusDeployed}},
			}, nil)
			i := &Impl{Helm: client, newRotationStore: emptyRotationTestStore}
			opts := NewDefaultClusterOptions()
			opts.Radius.ChartPath, opts.RadiusInstalled, opts.RadiusReleaseValues = "chart", true, stored
			run := i.UpgradeRadius
			switch command {
			case "reinstall":
				opts.Radius.Reinstall, run = true, i.InstallRadius
			case "preflight-only":
				run = i.CheckRadiusCertificates
			}
			require.ErrorContains(t, run(t.Context(), opts, kubeContext), "coordinated CA migration")
		})
	}
}

func TestCertificateTargetProfiles(t *testing.T) {
	for _, tt := range []struct {
		name, provider, domain string
		de                     bool
		edit                   func(*x509.Certificate)
		fail                   bool
	}{
		{name: "both profiles constrained", provider: "radius"},
		{name: "explicit domain", provider: "radius", domain: "radius.internal"},
		{name: "external only DNS", provider: "external", edit: func(c *x509.Certificate) { c.PermittedURIDomains = []string{"other.example"} }},
		{name: "URI exclusion", provider: "radius", fail: true, edit: func(c *x509.Certificate) { c.ExcludedURIDomains = []string{"radius.internal"} }},
		{name: "URI wrong domain", provider: "radius", fail: true, edit: func(c *x509.Certificate) { c.PermittedURIDomains = []string{"other.example"} }},
		{name: "DNS wrong namespace", provider: "radius", fail: true, edit: func(c *x509.Certificate) { c.PermittedDNSDomains = []string{"ucp.other.svc"} }},
		{name: "external DNS exclusion", provider: "external", fail: true, edit: func(c *x509.Certificate) { c.ExcludedDNSDomains = []string{"ucp.radius-system.svc"} }},
		{name: "custom domain with DE receiver", provider: "radius", domain: "custom.example", de: true},
		{name: "custom domain incompatible", provider: "radius", domain: "custom.example", fail: true, edit: func(c *x509.Certificate) { c.PermittedURIDomains = []string{"radius.internal"} }},
		{name: "receiver server usage only", provider: "radius", edit: func(c *x509.Certificate) { c.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth} }},
		{name: "external server usage only", provider: "external", edit: func(c *x509.Certificate) { c.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth} }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			domain := tt.domain
			if domain == "" {
				domain = "radius.internal"
			}
			ca, key, _ := testSigningCA(t, nil, func(c *x509.Certificate) {
				c.MaxPathLenZero = true
				c.PermittedDNSDomainsCritical = true
				c.PermittedDNSDomains = []string{"ucp.radius-system.svc"}
				c.PermittedURIDomains = []string{domain}
				if tt.edit != nil {
					tt.edit(c)
				}
			}, nil, nil)
			client := fake.NewClientset(&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "operator-ca", Namespace: RadiusSystemNamespace, UID: "ca-uid", ResourceVersion: "17"},
				Type:       corev1.SecretTypeTLS, Data: map[string][]byte{corev1.TLSCertKey: ca, corev1.TLSPrivateKeyKey: key},
			})
			options := NewDefaultClusterOptions()
			current := caReleaseValues("operator-ca", ca)
			rbac := current["global"].(map[string]any)["rbac"].(map[string]any)
			rbac["tls"] = map[string]any{"provider": tt.provider}
			rbac["spiffe"] = map[string]any{"trustDomain": domain}
			current["de"] = map[string]any{"image": "test-engine", "tag": "test", "internalAuth": map[string]any{"enabled": tt.de}}
			impl := &Impl{newCertificateClient: func(string) (kubernetes.Interface, error) { return client, nil }}
			target, plan, err := impl.prepareCertificateTarget(t.Context(), options, certificateTestChart(t), current, "context")
			if tt.fail {
				require.ErrorContains(t, err, "cannot support the selected URI/DNS")
				require.Nil(t, target)
			} else {
				require.NoError(t, err)
				require.Equal(t, domain, target.TrustDomain)
				require.Nil(t, plan.secret)
				require.Equal(t, "ca-uid", string(target.SigningUID))
				require.Equal(t, "17", target.SigningVersion)
				require.Equal(t, "radius-apiserver-issuer", target.Planes["aggregation"].Issuer.Name)
				require.NotEmpty(t, target.Planes["aggregation"].CA)
				if tt.provider == "external" {
					require.Len(t, target.Planes, 1)
					require.Empty(t, target.Receivers)
				} else {
					require.Equal(t, "radius-ca-issuer", target.Planes["internal"].Issuer.Name)
					require.Contains(t, target.Receivers, "ucp")
					require.NotContains(t, target.Receivers, "controller")
					require.Equal(t, tt.de, slices.Contains(target.Receivers, "bicep-de"))
				}
				public, err := json.Marshal(target)
				require.NoError(t, err)
				require.NotContains(t, string(public), "PRIVATE KEY")
			}
			for _, action := range client.Actions() {
				require.Equal(t, "get", action.GetVerb())
			}
		})
	}
}

func TestCertificateTargetParentConstraints(t *testing.T) {
	parentKey, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	require.NoError(t, err)
	parentPEM, _, parent := testSigningCA(t, parentKey, func(c *x509.Certificate) {
		c.PermittedURIDomains = []string{"wrong.example"}
		c.PermittedDNSDomainsCritical = true
	}, nil, nil)
	ca, key, _ := testSigningCA(t, nil, func(c *x509.Certificate) {
		c.Subject.CommonName = "dedicated-intermediate"
	}, parent, parentKey)
	chain := append(append([]byte{}, ca...), parentPEM...)
	for _, source := range []string{"stored values", "Secret flag", "file import"} {
		t.Run(source, func(t *testing.T) {
			options := NewDefaultClusterOptions()
			current := caReleaseValues("operator-ca", ca)
			client := fake.NewClientset(caTLSSecret("operator-ca", chain, key))
			if source != "stored values" {
				current = map[string]any{"global": map[string]any{"rbac": map[string]any{"enabled": true}}}
				options.Certificates.CASecret = "operator-ca"
			}
			if source == "file import" {
				directory := t.TempDir()
				certPath, keyPath := filepath.Join(directory, "ca.pem"), filepath.Join(directory, "key.pem")
				require.NoError(t, os.WriteFile(certPath, chain, 0600))
				require.NoError(t, os.WriteFile(keyPath, key, 0600))
				options.Certificates = CertificateOptions{CACert: certPath, CAKey: keyPath}
				client = fake.NewClientset()
			}
			impl := &Impl{newCertificateClient: func(string) (kubernetes.Interface, error) { return client, nil }}
			target, plan, err := impl.prepareCertificateTarget(t.Context(), options, certificateTestChart(t), current, "context")
			require.ErrorContains(t, err, "CA certificate 2 cannot support")
			require.Nil(t, target)
			require.Nil(t, plan)
			for _, action := range client.Actions() {
				require.Equal(t, "get", action.GetVerb())
			}
		})
	}
}

func TestCertificateTargetTransitionAndLease(t *testing.T) {
	ca, key, _ := testSigningCA(t, nil, nil, nil, nil)
	directory := t.TempDir()
	certPath, keyPath := filepath.Join(directory, "ca.pem"), filepath.Join(directory, "key.pem")
	require.NoError(t, os.WriteFile(certPath, ca, 0600))
	require.NoError(t, os.WriteFile(keyPath, key, 0600))
	options := NewDefaultClusterOptions()
	options.RadiusInstalled = true
	options.RadiusReleaseValues = map[string]any{"global": map[string]any{"rbac": map[string]any{"enabled": true}}}
	options.Certificates = CertificateOptions{CACert: certPath, CAKey: keyPath}
	client := fake.NewClientset()
	impl := &Impl{newCertificateClient: func(string) (kubernetes.Interface, error) { return client, nil }}
	target, plan, err := impl.prepareCertificateTarget(t.Context(), options, certificateTestChart(t), map[string]any{}, "context")
	require.NoError(t, err)
	require.Equal(t, "caSecret", target.Mode)
	require.NotNil(t, plan.secret)
	require.ErrorContains(t, unchangedCertificateSelection(options.RadiusReleaseValues, target.Values), "mode changes")
	require.ErrorContains(t, impl.applyCertificatePlan(t.Context(), plan), "Lease write guard")
	for _, action := range client.Actions() {
		require.Equal(t, "get", action.GetVerb())
	}
	plan.beforeWrite = func(context.Context) error { return errors.New("lease lost") }
	require.ErrorContains(t, impl.applyCertificatePlan(t.Context(), plan), "lease lost")
	for _, action := range client.Actions() {
		require.Equal(t, "get", action.GetVerb())
	}
	options.Certificates = CertificateOptions{}
	options.RadiusReleaseValues = caReleaseValues("previous-ca", ca)
	current := map[string]any{"global": map[string]any{"rbac": map[string]any{"certificates": map[string]any{"mode": "selfSigned"}}}}
	target, plan, err = impl.prepareCertificateTarget(t.Context(), options, certificateTestChart(t), current, "context")
	require.NoError(t, err)
	require.Nil(t, plan)
	require.Equal(t, "selfSigned", target.Mode)
	require.True(t, target.Planes["internal"].Unresolved)
	require.True(t, target.Planes["aggregation"].Unresolved)
	require.Empty(t, certificateValues(target.Values)["caBundle"])
	require.ErrorContains(t, unchangedCertificateSelection(options.RadiusReleaseValues, current), "mode changes")
}

func TestCertificateTargetResetValues(t *testing.T) {
	options := NewDefaultClusterOptions()
	options.RadiusReleaseValues = map[string]any{"global": map[string]any{"rbac": map[string]any{"dryRun": true, "tls": map[string]any{"provider": "external"}}}}
	selected := certificateTestChart(t)
	impl := &Impl{}

	target, _, err := impl.prepareCertificateTarget(t.Context(), options, selected, map[string]any{}, "context")
	require.NoError(t, err)
	require.Equal(t, "external", target.Provider, "omitted settings must survive an ordinary upgrade")

	options.ResetValues = true
	current := map[string]any{"global": map[string]any{"rbac": map[string]any{"enabled": true}}}
	target, _, err = impl.prepareCertificateTarget(t.Context(), options, selected, current, "context")
	require.NoError(t, err)
	require.Equal(t, "radius", target.Provider, "reset must use the selected chart's provider default")
	require.Equal(t, false, nestedValue(target.Values, "global", "rbac", "dryRun"))
}
