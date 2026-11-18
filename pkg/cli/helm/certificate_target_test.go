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
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/radius-project/radius/test/k8sutil"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"helm.sh/helm/v4/pkg/chart/common"
	valuesutil "helm.sh/helm/v4/pkg/chart/common/util"
	chart "helm.sh/helm/v4/pkg/chart/v2"
	"helm.sh/helm/v4/pkg/chart/v2/loader"
	"helm.sh/helm/v4/pkg/engine"
	releasecommon "helm.sh/helm/v4/pkg/release/common"
	releasev1 "helm.sh/helm/v4/pkg/release/v1"
	corev1 "k8s.io/api/core/v1"
	apiextclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
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
	}}, nil)
}

func TestInstalledChartDefaultsCannotSwitchIssuer(t *testing.T) {
	fixture := newIssuerPolicyFixture(t)
	server := fixture.server(t)
	kubeContext := useTargetKubeconfig(t, server.URL, k8sutil.NewForbiddenAPIServer(t).URL, true)
	ca, _, _ := testSigningCA(t, nil, nil, nil, nil)
	certificateValues(fixture.values)["caBundle"] = string(ca)
	old, selected := certificateTestChart(t), certificateTestChart(t)
	old.Values = valuesutil.CoalesceTables(copyValues(fixture.values), old.Values)
	stored := map[string]any{}
	setValue(stored, true, "global", "rbac", "enabled")

	// Render both complete charts using identical stored overrides. The issuer change
	// is in the actual UCP Certificate, not just in a synthetic values comparison.
	renderer := engine.New(&rest.Config{Host: server.URL})
	caps := common.DefaultCapabilities.Copy()
	caps.APIVersions = append(caps.APIVersions, admissionPolicyAPI)
	for c, issuer := range map[*chart.Chart]string{old: "external-ca", selected: "radius-apiserver-issuer"} {
		values, err := valuesutil.CoalesceValues(c, stored)
		require.NoError(t, err)
		rendered, err := renderer.RenderWithContext(t.Context(), c, common.Values{
			"Values": values, "Capabilities": caps,
			"Release": map[string]any{"Name": radiusReleaseName, "Namespace": RadiusSystemNamespace, "Service": "Helm"},
		})
		require.NoError(t, err)
		require.Contains(t, rendered[c.Name()+"/templates/ucp/apiservice.yaml"], "name: "+issuer)
	}

	runInstalledChartSwitchCommands(t, old, selected, stored, kubeContext)
}

func TestExternalChartCannotRequireManagedVolume(t *testing.T) {
	for _, command := range []string{"install", "upgrade", "preflight-only", "rollback"} {
		t.Run(command, func(t *testing.T) {
			selected := certificateTestChart(t)
			setValue(selected.Values, true, "global", "rbac", "enabled")
			setValue(selected.Values, "external", "global", "rbac", "tls", "provider")
			match := `{{- with include "radius.authz.spiffe.volume" . | trim }}{{ . | nindent 8 }}{{- end }}`
			// The historical manifest and the offline install render both omit the
			// volume in the upgrade-only cases. Check what Helm actually reapplies.
			volume := "- name: local-serving\n          csi:\n            driver: spiffe.csi.radapp.io"
			if command != "install" {
				volume = "{{- if .Release.IsUpgrade }}\n        " + volume + "\n        {{- end }}"
			}
			for _, file := range selected.Templates {
				if file.Name == "templates/ucp/deployment.yaml" {
					require.Contains(t, string(file.Data), match)
					file.Data = []byte(strings.Replace(string(file.Data), match, volume, 1))
				}
			}
			options := NewDefaultClusterOptions()
			options.Radius.ChartPath = "chart"
			history := []*releasev1.Release{{
				Name: radiusReleaseName, Namespace: RadiusSystemNamespace, Version: 1, Chart: selected,
				Info: &releasev1.Info{Status: releasecommon.StatusDeployed},
			}}
			h := NewMockHelmClient(gomock.NewController(t))
			i := &Impl{Helm: h, contourCluster: &fakeContourCluster{capabilities: capabilitiesWith(admissionPolicyAPI)}}
			var err error
			if command == "rollback" {
				err = i.reconcileRollbackDependencies(t.Context(), "test", history, 1)
			} else {
				i.newCSIDriverSPIFFEClient = func(string) (kubernetes.Interface, error) { return fake.NewClientset(), nil }
				h.EXPECT().LoadChart("chart").Return(selected, nil)
				if command != "install" {
					h.EXPECT().RunHelmHistory(gomock.Any(), radiusReleaseName).Return(history, nil)
				}
				run := i.InstallRadius
				switch command {
				case "preflight-only":
					run = i.CheckRadiusCertificates
				case "upgrade":
					run = i.UpgradeRadius
				}
				err = run(t.Context(), options, "test")
			}
			require.ErrorContains(t, err, `requires managed CSI driver "spiffe.csi.radapp.io"`)
		})
	}
}

func TestExternalChartUsesFinalRequesters(t *testing.T) {
	for _, command := range []string{"preflight-only", "install", "upgrade", "reinstall", "fresh-cert-manager"} {
		t.Run(command, func(t *testing.T) {
			selected := certificateTestChart(t)
			setValue(selected.Values, true, "global", "rbac", "enabled")
			setValue(selected.Values, "external", "global", "rbac", "tls", "provider")
			selected.Templates = append(selected.Templates, &common.File{Name: "templates/requester-workload.yaml", Data: []byte(`
{{- if or (has "system:serviceaccount:security:custom" .Values.global.rbac.certManager.requesters) (has "system:serviceaccount:cert-manager:cert-manager" .Values.global.rbac.certManager.requesters) }}
apiVersion: apps/v1
kind: Deployment
metadata: {name: requester-workload}
spec:
  template:
    spec:
      volumes:
      - name: local-serving
        csi: {driver: spiffe.csi.radapp.io}
{{- end }}`)})
			ctrl := certManagerControllerWithServiceAccount(certManagerController("security", CertManagerChartDefaultVersion, ""), "custom")
			kube := sarClient(nil, nil, ctrl)
			if command == "fresh-cert-manager" {
				kube = sarClient(nil, nil)
			}
			h := NewMockHelmClient(gomock.NewController(t))
			h.EXPECT().LoadChart("chart").Return(selected, nil)
			i := &Impl{Helm: h, contourCluster: &fakeContourCluster{capabilities: capabilitiesWith(admissionPolicyAPI)},
				newCSIDriverSPIFFEClient: func(string) (kubernetes.Interface, error) { return kube, nil }}
			options := NewDefaultClusterOptions()
			options.Radius.ChartPath, options.Contour.Disabled = "chart", true
			run := i.InstallRadius
			if command != "install" && command != "fresh-cert-manager" {
				options.RadiusInstalled, options.RadiusReleaseChart = true, selected
				expectRadiusBaseline(t, h, options)
				switch command {
				case "preflight-only":
					run = i.CheckRadiusCertificates
				case "upgrade":
					run = i.UpgradeRadius
				case "reinstall":
					options.Radius.Reinstall = true
				}
			}
			require.ErrorContains(t, run(t.Context(), options, "test"), `requires managed CSI driver "spiffe.csi.radapp.io"`)
		})
	}
}

func TestChangedRequestersStopBeforeDependencyApply(t *testing.T) {
	ctrl := certManagerControllerWithServiceAccount(certManagerController("security", CertManagerChartDefaultVersion, ""), "changed")
	plan := &csiDriverSPIFFEPlan{
		kubeClient: sarClient(nil, nil, ctrl),
		requesters: []string{"system:serviceaccount:security:custom"},
	}
	_, err := (&Impl{}).applyCSIDriverSPIFFE(t.Context(), plan, "test", true)
	require.ErrorContains(t, err, "requesters differ from the validated Radius values")
}

func TestExternalPreflightRejectsStandalonePod(t *testing.T) {
	const pod = `
apiVersion: v1
kind: Pod
metadata: {name: serving}
spec:
  containers: [{name: app, image: example.invalid/app}]
  volumes: [{name: local-serving, csi: {driver: spiffe.csi.radapp.io}}]
`
	for _, conditional := range []bool{false, true} {
		t.Run(fmt.Sprintf("requesterConditional=%t", conditional), func(t *testing.T) {
			selected := certificateTestChart(t)
			setValue(selected.Values, true, "global", "rbac", "enabled")
			setValue(selected.Values, "external", "global", "rbac", "tls", "provider")
			template := pod
			if conditional {
				template = `{{ if has "system:serviceaccount:security:custom" .Values.global.rbac.certManager.requesters }}` + pod + `{{ end }}`
			}
			selected.Templates = append(selected.Templates, &common.File{Name: "templates/serving-pod.yaml", Data: []byte(template)})
			options := NewDefaultClusterOptions()
			options.Radius.ChartPath, options.RadiusReleaseChart = "chart", selected
			h := NewMockHelmClient(gomock.NewController(t))
			h.EXPECT().LoadChart("chart").Return(selected, nil)
			expectRadiusBaseline(t, h, options)
			ctrl := certManagerControllerWithServiceAccount(certManagerController("security", CertManagerChartDefaultVersion, ""), "custom")
			kube := sarClient(nil, nil, ctrl)
			i := &Impl{Helm: h, contourCluster: &fakeContourCluster{capabilities: capabilitiesWith(admissionPolicyAPI)},
				newCSIDriverSPIFFEClient: func(string) (kubernetes.Interface, error) { return kube, nil }}
			require.ErrorContains(t, i.CheckRadiusCertificates(t.Context(), options, "test"), `requires managed CSI driver "spiffe.csi.radapp.io"`)
		})
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
			i := &Impl{Helm: client}
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
		emptyDomain            bool
		edit                   func(*x509.Certificate)
		fail                   bool
	}{
		{name: "both profiles constrained", provider: "radius"},
		{name: "empty domain uses chart fallback", provider: "radius", emptyDomain: true},
		{name: "external only DNS", provider: "external", edit: func(c *x509.Certificate) { c.PermittedURIDomains = []string{"other.example"} }},
		{name: "URI exclusion", provider: "radius", fail: true, edit: func(c *x509.Certificate) { c.ExcludedURIDomains = []string{"radius.internal"} }},
		{name: "URI wrong domain", provider: "radius", fail: true, edit: func(c *x509.Certificate) { c.PermittedURIDomains = []string{"other.example"} }},
		{name: "DNS wrong namespace", provider: "radius", fail: true, edit: func(c *x509.Certificate) { c.PermittedDNSDomains = []string{"ucp.other.svc"} }},
		{name: "external DNS exclusion", provider: "external", fail: true, edit: func(c *x509.Certificate) { c.ExcludedDNSDomains = []string{"ucp.radius-system.svc"} }},
		{name: "custom domain with DE receiver", provider: "radius", domain: "custom.example", de: true},
		{name: "custom domain incompatible", provider: "radius", domain: "custom.example", fail: true, edit: func(c *x509.Certificate) { c.PermittedURIDomains = []string{"radius.internal"} }},
		{name: "CSI requires client usage despite token caller", provider: "radius", fail: true, edit: func(c *x509.Certificate) { c.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth} }},
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
			if tt.emptyDomain {
				rbac["spiffe"] = map[string]any{"trustDomain": ""}
			}
			current["de"] = map[string]any{"internalAuth": map[string]any{"enabled": tt.de}}
			impl := &Impl{newCertificateClient: func(string) (kubernetes.Interface, error) { return client, nil }}
			target, plan, err := impl.prepareCertificateTarget(t.Context(), options, certificateTestChart(t), current, "context")
			if tt.fail {
				require.ErrorContains(t, err, "cannot support the selected URI/DNS")
				require.Nil(t, target)
			} else {
				require.Equal(t, domain, target.TrustDomain)
				require.NoError(t, err)
				require.Nil(t, plan.secret)
				require.Equal(t, "ca-uid", string(target.SigningUID))
				require.Equal(t, "17", target.SigningVersion)
				require.Equal(t, RadiusAPIServerIssuerName, target.Planes["aggregation"].Issuer.Name)
				require.NotEmpty(t, target.Planes["aggregation"].CA)
				if tt.provider == "external" {
					require.Len(t, target.Planes, 1)
					require.Empty(t, target.Receivers)
				} else {
					require.Equal(t, RadiusCAIssuerName, target.Planes["internal"].Issuer.Name)
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

func TestCertificateTargetPolicyRenderer(t *testing.T) {
	fixture := newIssuerPolicyFixture(t)
	fixture.objects = append(fixture.objects, &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "cert-manager.io/v1", "kind": "Issuer",
		"metadata": map[string]any{"name": RadiusCAIssuerName, "namespace": RadiusSystemNamespace},
		"spec":     map[string]any{"ca": map[string]any{"secretName": "radius-ca"}},
	}})
	server := fixture.server(t)
	decoy := k8sutil.NewForbiddenAPIServer(t)
	kubeContext := useTargetKubeconfig(t, server.URL, decoy.URL, false)
	ca, _, _ := testSigningCA(t, nil, nil, nil, nil)
	certificateValues(fixture.values)["caBundle"] = string(ca)
	options := NewDefaultClusterOptions()
	options.RadiusInstalled = true
	options.RadiusReleaseValues = map[string]any{"global": map[string]any{"rbac": map[string]any{"enabled": true}}}
	selected := certificateTestChart(t)
	history := []*releasev1.Release{{
		Name: "radius", Namespace: RadiusSystemNamespace, Version: 1, Config: copyValues(options.RadiusReleaseValues),
		Chart: selected, Info: &releasev1.Info{Status: releasecommon.StatusDeployed},
	}}
	impl := &Impl{}
	require.ErrorContains(t, unchangedCertificateSelection(options.RadiusReleaseValues, fixture.values), "mode changes")
	require.ErrorContains(t, impl.renderCertificatePolicy(t.Context(), options, selected, fixture.values, kubeContext, history), "certificate lifecycle prerequisite")
	require.NoError(t, impl.renderCertificatePolicy(t.Context(), options, certificatePolicyChart(selected), fixture.values, kubeContext, history))
	history[0].Config["changed"] = true
	require.ErrorContains(t, impl.renderCertificatePolicy(t.Context(), options, certificatePolicyChart(selected), fixture.values, kubeContext, history), "changed since preflight")
	require.Empty(t, decoy.Requests())
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

func TestCASecretLivePreflight(t *testing.T) {
	for _, provider := range []string{"radius", "external"} {
		for _, invalidInjector := range []bool{false, true} {
			t.Run(provider+fmt.Sprint("/invalidInjector=", invalidInjector), func(t *testing.T) {
				ca, key, _ := testSigningCA(t, nil, nil, nil, nil)
				fixture := newIssuerPolicyFixture(t)
				rbac := fixture.values["global"].(map[string]any)["rbac"].(map[string]any)
				rbac["tls"] = map[string]any{"provider": provider}
				rbac["certificates"] = map[string]any{"mode": "caSecret", "caSecret": "operator-ca", "caBundle": string(ca)}
				server := fixture.server(t)
				decoy := k8sutil.NewForbiddenAPIServer(t)
				kubeContext := useTargetKubeconfig(t, server.URL, decoy.URL, false)
				d, rs, pod := injectorFixture("cm-cainjector")
				if invalidInjector {
					d.Spec.Template.Spec.Containers[0].Args = []string{"--config=/opaque.yaml"}
					pod.Spec = *d.Spec.Template.Spec.DeepCopy()
				}
				controller := certManagerController("security", "v1.21.2", "")
				client := fake.NewClientset(d, rs, pod, controller, caTLSSecret("operator-ca", ca, key))
				_, extensions := (fakeCertManagerCluster{crd: true}).clients()
				impl := &Impl{
					newCertificateClient:  func(string) (kubernetes.Interface, error) { return client, nil },
					newCertManagerClients: func(string) (kubernetes.Interface, apiextclient.Interface, error) { return client, extensions, nil },
				}
				options := NewDefaultClusterOptions()
				current := copyValues(fixture.values)
				selected := certificateTestChart(t)
				target, _, err := impl.prepareSelectedCertificates(t.Context(), options, selected, current, kubeContext, nil)
				require.NoError(t, err)
				require.ErrorContains(t, target.checkTransition(target.mode), "maintaining global.rbac.certificates.mode=caSecret")
				err = impl.checkIssuerPolicy(t.Context(), options, selected, current, kubeContext, nil)
				if invalidInjector {
					require.ErrorContains(t, err, "not an inspectable")
				} else {
					require.NoError(t, err)
					helmClient := NewMockHelmClient(gomock.NewController(t))
					helmClient.EXPECT().LoadChart("local-chart").Return(selected, nil)
					impl.Helm = helmClient
					options.Radius.ChartPath = "local-chart"
					options.Radius.SetArgs = []string{"global.rbac.enabled=true", "global.rbac.tls.provider=" + provider}
					options.Certificates.CASecret = "operator-ca"
					err = impl.InstallRadius(t.Context(), options, kubeContext)
					require.ErrorContains(t, err, "mode=caSecret requires the certificate lifecycle")
				}
				for _, action := range client.Actions() {
					require.Contains(t, []string{"get", "list"}, action.GetVerb())
				}
				require.Empty(t, decoy.Requests())
			})
		}
	}
}
