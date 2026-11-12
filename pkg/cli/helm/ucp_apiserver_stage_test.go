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
	"encoding/base64"
	"errors"
	"io"
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
	kubefakehelm "helm.sh/helm/v4/pkg/kube/fake"
	"helm.sh/helm/v4/pkg/release"
	releasecommon "helm.sh/helm/v4/pkg/release/common"
	releasev1 "helm.sh/helm/v4/pkg/release/v1"
	"helm.sh/helm/v4/pkg/storage"
	"helm.sh/helm/v4/pkg/storage/driver"
	apiextclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/dynamic"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes"
	kubefake "k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"
)

var (
	stagedCertificatesResource = schema.GroupVersionResource{Group: "cert-manager.io", Version: "v1", Resource: "certificates"}
	stagedIssuersResource      = schema.GroupVersionResource{Group: "cert-manager.io", Version: "v1", Resource: "issuers"}
)

// stagedCAObject is an object of the radius-apiserver-ca chain that rad must apply before Helm reissues ucp-cert.
type stagedCAObject struct {
	resource schema.GroupVersionResource
	kind     string
	name     string
}

var stagedCAObjects = []stagedCAObject{
	{resource: stagedIssuersResource, kind: "Issuer", name: RadiusBootstrapIssuerName},
	{resource: stagedCertificatesResource, kind: "Certificate", name: ucpAPIServerCASecretName},
	{resource: stagedIssuersResource, kind: "Issuer", name: RadiusAPIServerIssuerName},
}

// stageCluster is a cluster where UCP serves a certificate from the Helm-generated ucp-ca, and where cert-manager
// issues radius-apiserver-ca when its Certificate is created, if issueCA is set.
type stageCluster struct {
	kube        *kubefake.Clientset
	dynamic     *dynamicfake.FakeDynamicClient
	ucpCA       testCA
	apiServerCA testCA
	issueCA     bool
}

func newStageCluster(t *testing.T, apiService *unstructured.Unstructured, issueCA bool) *stageCluster {
	t.Helper()
	c := &stageCluster{
		kube:        kubefake.NewSimpleClientset(),
		ucpCA:       newTestCA(t, "ucp-ca"),
		apiServerCA: newTestCA(t, "radius-apiserver-ca"),
		issueCA:     issueCA,
	}
	objects := []runtime.Object{}
	if apiService != nil {
		objects = append(objects, apiService)
	}
	c.dynamic = dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{
		apiServicesResource:        "APIServiceList",
		stagedCertificatesResource: "CertificateList",
		stagedIssuersResource:      "IssuerList",
	}, objects...)
	issue := func(action clienttesting.Action) (bool, runtime.Object, error) {
		// UpdateAction has the same method set as CreateAction, so this matches both verbs.
		name := action.(clienttesting.CreateAction).GetObject().(*unstructured.Unstructured).GetName()
		if name == ucpAPIServerCASecretName && c.issueCA {
			c.issueAPIServerCA(t)
		}
		return false, nil, nil
	}
	c.dynamic.PrependReactor("create", "certificates", issue)
	c.dynamic.PrependReactor("update", "certificates", issue)
	return c
}

// offAPIService is the UCP APIService that a chart with authorization off renders: its caBundle holds ucp-ca, and
// the ca.crt of ucp-cert also holds the leaf, which rad must not trust.
func (c *stageCluster) offAPIService(t *testing.T) *unstructured.Unstructured {
	t.Helper()
	bundle := append(append([]byte{}, c.ucpCA.pem...), issueUCPCertificate(t, c.ucpCA)...)
	return ucpAPIService(base64.StdEncoding.EncodeToString(bundle), "True", "Passed")
}

func (c *stageCluster) issueAPIServerCA(t *testing.T) {
	t.Helper()
	_, err := c.kube.CoreV1().Secrets(RadiusSystemNamespace).Create(t.Context(), apiServerCASecret(c.apiServerCA), metav1.CreateOptions{})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		require.NoError(t, err)
	}
}

// helmReissuesUCPCertificate does what Helm and cert-manager do once Helm applies the chart with authorization on:
// cert-manager issues radius-apiserver-ca, reissues ucp-cert from radius-apiserver-issuer, and UCP rolls out.
func (c *stageCluster) helmReissuesUCPCertificate(t *testing.T) {
	t.Helper()
	c.issueAPIServerCA(t)
	secrets := c.kube.CoreV1().Secrets(RadiusSystemNamespace)
	_, err := secrets.Create(t.Context(), tlsSecret(ucpAPIServerTLSSecretName, issueUCPCertificate(t, c.apiServerCA), c.apiServerCA.pem), metav1.CreateOptions{})
	require.NoError(t, err)
	_, err = c.kube.AppsV1().Deployments(RadiusSystemNamespace).Create(t.Context(), ucpDeployment(true), metav1.CreateOptions{})
	require.NoError(t, err)
}

// cainjectorCASource makes the APIService look like one cert-manager already maintains from radius-apiserver-ca.
func (c *stageCluster) cainjectorAPIService() *unstructured.Unstructured {
	apiService := ucpAPIService(base64.StdEncoding.EncodeToString(c.apiServerCA.pem), "True", "Passed")
	apiService.SetAnnotations(map[string]string{injectCAFromSecretAnnotation: ucpAPIServerCASource(RadiusSystemNamespace)})
	return apiService
}

func (c *stageCluster) impl(client HelmClient, certManagerControllerSA string) *Impl {
	return &Impl{
		Helm:           client,
		logger:         func(string, ...any) {},
		contourCluster: &fakeContourCluster{},
		newCertManagerClients: func(string) (kubernetes.Interface, apiextclient.Interface, error) {
			kubeClient, apiextClient := fakeCertManagerCluster{crd: true, controller: certManagerController(CertManagerNamespace, CertManagerChartDefaultVersion, "")}.clients()
			return kubeClient, apiextClient, nil
		},
		newCSIDriverSPIFFEClient: func(string) (kubernetes.Interface, error) {
			controller := certManagerControllerWithServiceAccount(certManagerController("security", CertManagerChartDefaultVersion, ""), certManagerControllerSA)
			return sarClient(nil, nil, controller), nil
		},
		newCertManagerDynamicClient: func(string) (dynamic.Interface, error) { return c.dynamic, nil },
		newUCPAPIServerTrustClient:  func(string) (kubernetes.Interface, error) { return c.kube, nil },
	}
}

// certManagerWrites returns the create, update and patch requests rad sent for cert-manager objects.
func (c *stageCluster) certManagerWrites() []string {
	writes := []string{}
	for _, action := range c.dynamic.Actions() {
		if action.GetResource().Group != "cert-manager.io" || action.GetVerb() == "get" || action.GetVerb() == "list" || action.GetVerb() == "watch" {
			continue
		}
		writes = append(writes, action.GetVerb()+" "+action.GetResource().Resource)
	}
	return writes
}

// requireStagedCA checks, before Helm changes the release, that rad applied the radius-apiserver-ca chain as the
// chart renders it, owned by the Radius release so Helm adopts it, and that the API server trusts both ucp-ca, which
// the running UCP pods serve, and radius-apiserver-ca, which UCP serves once Helm reissues ucp-cert.
func (c *stageCluster) requireStagedCA(t *testing.T, rendered map[string]map[string]any) {
	t.Helper()
	for _, object := range stagedCAObjects {
		staged, err := c.dynamic.Resource(object.resource).Namespace(RadiusSystemNamespace).Get(t.Context(), object.name, metav1.GetOptions{})
		require.NoError(t, err, "rad must apply %s %s before Helm reissues ucp-cert", object.kind, object.name)
		require.Equal(t, "Helm", staged.GetLabels()["app.kubernetes.io/managed-by"], "%s %s", object.kind, object.name)
		require.Equal(t, "radius", staged.GetAnnotations()["meta.helm.sh/release-name"], "%s %s", object.kind, object.name)
		require.Equal(t, RadiusSystemNamespace, staged.GetAnnotations()["meta.helm.sh/release-namespace"], "%s %s", object.kind, object.name)
		require.Equal(t, rendered[object.kind+"/"+object.name], staged.Object["spec"], "%s %s must match the rendered chart", object.kind, object.name)
		for key, value := range rendered[object.kind+"/"+object.name+"#labels"] {
			require.Equal(t, value, staged.GetLabels()[key], "%s %s label %s", object.kind, object.name, key)
		}
	}
	require.Equal(t, []string{"ucp-ca", "radius-apiserver-ca"}, trustedSubjects(t, c.kube))
	require.Equal(t, []string{"ucp-ca", "radius-apiserver-ca"}, apiServiceCABundle(t, c.dynamic))
}

func (c *stageCluster) requireNotStaged(t *testing.T) {
	t.Helper()
	require.Empty(t, c.certManagerWrites(), "rad must not stage cert-manager objects")
	requireNoTrustSecret(t, c.kube)
}

func loadStageTestChart(t *testing.T) *chart.Chart {
	t.Helper()
	c, err := loader.Load("../../../deploy/Chart")
	require.NoError(t, err)
	c.Files = slices.DeleteFunc(c.Files, func(file *common.File) bool {
		return strings.HasPrefix(file.Name, "crds/")
	})
	return c
}

// renderStageTestManifest renders the Radius chart with values as a fresh install does.
func renderStageTestManifest(t *testing.T, values map[string]any) string {
	t.Helper()
	install := helm.NewInstall(newTestRenderConfiguration(nil))
	install.DryRunStrategy = helm.DryRunServer
	install.ReleaseName = "radius"
	install.Namespace = RadiusSystemNamespace
	rendered, err := install.RunWithContext(t.Context(), loadStageTestChart(t), values)
	require.NoError(t, err)
	accessor, err := release.NewAccessor(rendered)
	require.NoError(t, err)
	return accessor.Manifest()
}

// renderedStagedCA returns the spec, by kind/name, and the labels, by kind/name#labels, of each object of the
// radius-apiserver-ca chain in manifest.
func renderedStagedCA(t *testing.T, manifest string) map[string]map[string]any {
	t.Helper()
	result := map[string]map[string]any{}
	decoder := yaml.NewYAMLOrJSONDecoder(strings.NewReader(manifest), 4096)
	for {
		object := &unstructured.Unstructured{}
		err := decoder.Decode(&object.Object)
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)
		for _, staged := range stagedCAObjects {
			if object.GetKind() == staged.kind && object.GetName() == staged.name {
				result[staged.kind+"/"+staged.name] = object.Object["spec"].(map[string]any)
				labels := map[string]any{}
				for key, value := range object.GetLabels() {
					labels[key] = value
				}
				result[staged.kind+"/"+staged.name+"#labels"] = labels
			}
		}
	}
	for _, staged := range stagedCAObjects {
		require.Contains(t, result, staged.kind+"/"+staged.name)
	}
	return result
}

var authzOnValues = map[string]any{"global": map[string]any{"rbac": map[string]any{"enabled": true}}}

// stageUpgrade is rad upgrade kubernetes, or rad install kubernetes --reinstall, from a deployed release with values
// deployed, with Helm storage in memory.
type stageUpgrade struct {
	configuration *helm.Configuration
	helmClient    HelmClient
	command       string
}

func newStageUpgrade(t *testing.T, deployed map[string]any, command string) *stageUpgrade {
	t.Helper()
	u := &stageUpgrade{
		configuration: &helm.Configuration{
			Releases:     storage.Init(driver.NewMemory()),
			KubeClient:   &kubefakehelm.PrintingKubeClient{Out: io.Discard},
			Capabilities: common.DefaultCapabilities,
		},
		helmClient: NewHelmClient(),
		command:    command,
	}
	if deployed != nil {
		_, err := u.helmClient.RunHelmInstall(u.configuration, loadStageTestChart(t), deployed, "radius", RadiusSystemNamespace, false, DefaultInstallTimeout, nil)
		require.NoError(t, err)
	}
	return u
}

// mock returns a Helm client that reads Radius from the in-memory storage and installs csi-driver-spiffe. It
// expects upgrades times Radius upgrades (installs for a fresh install) and calls beforeHelm before each.
func (u *stageUpgrade) mock(t *testing.T, upgrades int, beforeHelm func()) *MockHelmClient {
	t.Helper()
	client := NewMockHelmClient(gomock.NewController(t))
	client.EXPECT().RunHelmGet(gomock.Any(), gomock.Any()).AnyTimes().
		DoAndReturn(func(_ *helm.Configuration, name string) (*releasev1.Release, error) {
			return u.helmClient.RunHelmGet(u.configuration, name)
		})
	client.EXPECT().RunHelmHistory(gomock.Any(), gomock.Any()).AnyTimes().
		DoAndReturn(func(_ *helm.Configuration, name string) ([]*releasev1.Release, error) {
			return u.helmClient.RunHelmHistory(u.configuration, name)
		})
	client.EXPECT().LoadChart(gomock.Any()).AnyTimes().
		DoAndReturn(func(string) (*chart.Chart, error) { return loadStageTestChart(t), nil })
	expectCSIDriverSPIFFEPull(t, client)
	client.EXPECT().RunHelmInstall(gomock.Any(), gomock.Any(), gomock.Any(), csiDriverSPIFFEReleaseName, RadiusSystemNamespace, true, DefaultInstallTimeout, radManagedByLabels()).
		AnyTimes().Return(&releasev1.Release{}, nil)
	if u.command == "install" {
		client.EXPECT().RunHelmInstall(gomock.Any(), gomock.Any(), gomock.Any(), "radius", RadiusSystemNamespace, gomock.Any(), gomock.Any(), gomock.Any()).Times(upgrades).
			DoAndReturn(func(_ *helm.Configuration, c *chart.Chart, vals map[string]any, name, namespace string, _ bool, timeout time.Duration, labels map[string]string) (*releasev1.Release, error) {
				beforeHelm()
				return u.helmClient.RunHelmInstall(u.configuration, c, vals, name, namespace, false, timeout, labels)
			})
		return client
	}
	client.EXPECT().RunHelmUpgrade(gomock.Any(), gomock.Any(), gomock.Any(), "radius", gomock.Any(), gomock.Any(), true, gomock.Any()).Times(upgrades).
		DoAndReturn(func(_ *helm.Configuration, c *chart.Chart, vals map[string]any, name, namespace string, _ bool, reuse bool, timeout time.Duration) (*releasev1.Release, error) {
			beforeHelm()
			return u.helmClient.RunHelmUpgrade(u.configuration, c, vals, name, namespace, false, reuse, timeout)
		})
	return client
}

func (u *stageUpgrade) run(t *testing.T, impl *Impl, setArgs []string) error {
	t.Helper()
	state, err := impl.CheckRadiusInstall("test-context")
	require.NoError(t, err)
	options := NewDefaultClusterOptions()
	options.Radius.ChartPath = "chart"
	options.Radius.SetArgs = setArgs
	options.Contour.Disabled = true
	options.RadiusReleaseValues = state.RadiusValues
	switch u.command {
	case "upgrade":
		return impl.UpgradeRadius(t.Context(), options, "test-context")
	case "install --reinstall":
		options.Radius.Reinstall = true
		return impl.InstallRadius(t.Context(), options, "test-context")
	default:
		return impl.InstallRadius(t.Context(), options, "test-context")
	}
}

// (a) and (c): when an upgrade turns authorization on, rad stages radius-apiserver-ca, as the chart renders it and
// owned by the Radius release, and makes the API server trust it together with ucp-ca before Helm reissues ucp-cert.
func Test_UpgradeRadius_StagesUCPAPIServerCABeforeHelm(t *testing.T) {
	rendered := renderedStagedCA(t, renderStageTestManifest(t, authzOnValues))
	for _, command := range []string{"upgrade", "install --reinstall"} {
		t.Run(command, func(t *testing.T) {
			shortenUCPAPIServiceWait(t)
			u := newStageUpgrade(t, map[string]any{}, command)
			c := newStageCluster(t, nil, true)
			require.NoError(t, c.dynamic.Tracker().Add(c.offAPIService(t)))

			helmCalls := 0
			client := u.mock(t, 1, func() {
				helmCalls++
				c.requireStagedCA(t, rendered)
				c.helmReissuesUCPCertificate(t)
			})

			require.NoError(t, u.run(t, c.impl(client, "cert-manager"), []string{"global.rbac.enabled=true"}))
			require.Equal(t, 1, helmCalls)
			// After Helm, rad removes ucp-ca once UCP serves the new certificate.
			require.Equal(t, ucpAPIServerCASource(RadiusSystemNamespace), apiServiceCASource(t, c.dynamic))
			require.Equal(t, []string{"radius-apiserver-ca"}, apiServiceCABundle(t, c.dynamic))
			requireNoTrustSecret(t, c.kube)
		})
	}
}

// (d): an upgrade that keeps authorization on, and a fresh install, have no earlier CA to overlap with, so rad stages
// nothing before Helm.
func Test_UpgradeRadius_DoesNotStageUCPAPIServerCA(t *testing.T) {
	t.Run("on to on", func(t *testing.T) {
		shortenUCPAPIServiceWait(t)
		u := newStageUpgrade(t, authzOnValues, "upgrade")
		c := newStageCluster(t, nil, true)
		c.issueAPIServerCA(t)
		require.NoError(t, c.dynamic.Tracker().Add(c.cainjectorAPIService()))
		writesBefore := 0
		client := u.mock(t, 1, func() {
			writesBefore = len(c.certManagerWrites())
			requireNoTrustSecret(t, c.kube)
			c.helmReissuesUCPCertificate(t)
		})

		require.NoError(t, u.run(t, c.impl(client, "cert-manager"), nil))
		require.Zero(t, writesBefore)
		c.requireNotStaged(t)
	})

	t.Run("fresh install", func(t *testing.T) {
		shortenUCPAPIServiceWait(t)
		u := newStageUpgrade(t, nil, "install")
		c := newStageCluster(t, nil, true)
		client := u.mock(t, 1, func() {
			c.requireNotStaged(t)
			// The chart creates the APIService, which cainjector fills from radius-apiserver-ca.
			require.NoError(t, c.dynamic.Tracker().Add(c.cainjectorAPIService()))
			c.helmReissuesUCPCertificate(t)
		})

		require.NoError(t, u.run(t, c.impl(client, "cert-manager"), []string{"global.rbac.enabled=true"}))
		c.requireNotStaged(t)
	})
}

// (e): when cert-manager does not issue radius-apiserver-ca in time, rad stops before Helm, so UCP keeps serving
// ucp-cert from ucp-ca, which the API server still trusts. Running the command again after cert-manager issues the CA
// finishes the stage and the upgrade.
func Test_UpgradeRadius_StageUCPAPIServerCATimeout(t *testing.T) {
	shortenUCPAPIServiceWait(t)
	rendered := renderedStagedCA(t, renderStageTestManifest(t, authzOnValues))
	u := newStageUpgrade(t, map[string]any{}, "upgrade")
	c := newStageCluster(t, nil, false)
	offAPIService := c.offAPIService(t)
	require.NoError(t, c.dynamic.Tracker().Add(offAPIService))

	err := u.run(t, c.impl(u.mock(t, 0, func() {}), "cert-manager"), []string{"global.rbac.enabled=true"})
	require.ErrorContains(t, err, "Secret radius-system/radius-apiserver-ca is not usable")
	require.ErrorContains(t, err, `kubectl --context "test-context" -n radius-system describe certificate radius-apiserver-ca`)
	apiService, err := c.dynamic.Resource(apiServicesResource).Get(t.Context(), ucpAPIServiceName, metav1.GetOptions{})
	require.NoError(t, err)
	require.Equal(t, offAPIService.Object["spec"], apiService.Object["spec"], "the API server must keep trusting the CA UCP serves")
	deployed, err := u.helmClient.RunHelmGet(u.configuration, "radius")
	require.NoError(t, err)
	require.Equal(t, 1, deployed.Version, "rad must not call Helm")

	// cert-manager issues the CA after rad gave up; the rerun adopts the partial stage.
	c.issueCA = true
	c.issueAPIServerCA(t)
	helmCalls := 0
	client := u.mock(t, 1, func() {
		helmCalls++
		c.requireStagedCA(t, rendered)
		c.helmReissuesUCPCertificate(t)
	})
	require.NoError(t, u.run(t, c.impl(client, "cert-manager"), []string{"global.rbac.enabled=true"}))
	require.Equal(t, 1, helmCalls)
	require.Equal(t, []string{"radius-apiserver-ca"}, apiServiceCABundle(t, c.dynamic))
}

// (b): a rollback from a revision with authorization off to one with it on stages radius-apiserver-ca from the
// target revision's manifest before Helm rolls back, after the rollback issuer guard and the dependency reconcile.
func Test_RollbackRadius_StagesUCPAPIServerCABeforeHelm(t *testing.T) {
	manifest := renderStageTestManifest(t, authzOnValues)
	rendered := renderedStagedCA(t, manifest)
	installed := &releasev1.Release{Name: csiDriverSPIFFEReleaseName, Labels: radManagedByLabels(), Info: &releasev1.Info{Status: releasecommon.StatusDeployed},
		Config: buildCSIDriverSPIFFEValues(RadiusSystemNamespace, spiffeSettings{TrustDomain: SPIFFETrustDomainDefault, CSIDriverName: SPIFFECSIDriverNameDefault})}
	certManager := &releasev1.Release{Name: certManagerReleaseName, Labels: radManagedByLabels(), Info: &releasev1.Info{Status: releasecommon.StatusDeployed},
		Chart: &chart.Chart{Metadata: &chart.Metadata{Version: CertManagerChartDefaultVersion}}, Config: buildCertManagerValues(RadiusSystemNamespace)}
	allowed := map[string]bool{
		issuerSignerName(RadiusSystemNamespace, RadiusBootstrapIssuerName): true,
		issuerSignerName(RadiusSystemNamespace, RadiusAPIServerIssuerName): true,
	}
	history := func(targetManifest string) []*releasev1.Release {
		return []*releasev1.Release{
			{Version: 1, Config: authzOnValues, Manifest: targetManifest, Chart: &chart.Chart{Metadata: &chart.Metadata{Version: "0.45.0"}}, Info: &releasev1.Info{Status: releasecommon.StatusSuperseded}},
			{Version: 2, Config: map[string]any{}, Chart: &chart.Chart{Metadata: &chart.Metadata{Version: "0.46.0"}}, Info: &releasev1.Info{Status: releasecommon.StatusDeployed}},
		}
	}
	impl := func(c *stageCluster, m *MockHelmClient) *Impl {
		i := c.impl(m, "cert-manager")
		i.newCSIDriverSPIFFEClient = func(string) (kubernetes.Interface, error) {
			return sarClient(allowed, nil, certManagerController(CertManagerNamespace, CertManagerChartDefaultVersion, "")), nil
		}
		return i
	}

	for _, toRevision := range []bool{false, true} {
		name := "RollbackRadius"
		if toRevision {
			name = "RollbackRadiusToRevision"
		}
		t.Run(name, func(t *testing.T) {
			shortenUCPAPIServiceWait(t)
			c := newStageCluster(t, nil, true)
			require.NoError(t, c.dynamic.Tracker().Add(c.offAPIService(t)))
			m := NewMockHelmClient(gomock.NewController(t))
			m.EXPECT().RunHelmHistory(gomock.Any(), radiusReleaseName).Return(history(manifest), nil)
			rollbacks := 0
			gomock.InOrder(
				m.EXPECT().RunHelmGet(gomock.Any(), csiDriverSPIFFEReleaseName).Return(installed, nil),
				m.EXPECT().RunHelmGet(gomock.Any(), certManagerReleaseName).Return(certManager, nil),
				m.EXPECT().RunHelmRollback(gomock.Any(), radiusReleaseName, 1, true).DoAndReturn(func(*helm.Configuration, string, int, bool) error {
					rollbacks++
					c.requireStagedCA(t, rendered)
					c.helmReissuesUCPCertificate(t)
					return nil
				}),
			)

			var err error
			if toRevision {
				err = impl(c, m).RollbackRadiusToRevision(t.Context(), "test-context", 1)
			} else {
				err = impl(c, m).RollbackRadius(t.Context(), "test-context")
			}
			require.NoError(t, err)
			require.Equal(t, 1, rollbacks)
			require.Equal(t, []string{"radius-apiserver-ca"}, apiServiceCABundle(t, c.dynamic))
			requireNoTrustSecret(t, c.kube)
		})
	}

	// The issuer guard refuses the target before rad stages anything.
	t.Run("refused target stages nothing", func(t *testing.T) {
		shortenUCPAPIServiceWait(t)
		c := newStageCluster(t, nil, true)
		require.NoError(t, c.dynamic.Tracker().Add(c.offAPIService(t)))
		legacy := manifest + "\n---\n" + `apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: ucp-mtls
  namespace: radius-system
spec:
  secretName: ucp-mtls
  issuerRef:
    name: radius-mtls-issuer
    kind: Issuer
`
		m := NewMockHelmClient(gomock.NewController(t))
		m.EXPECT().RunHelmHistory(gomock.Any(), radiusReleaseName).Return(history(legacy), nil)

		err := impl(c, m).RollbackRadiusToRevision(t.Context(), "test-context", 1)
		require.ErrorContains(t, err, "Radius cannot roll back to revision 1")
		require.Empty(t, c.dynamic.Actions(), "a refused rollback must not touch the cluster")
		requireNoTrustSecret(t, c.kube)
	})
}
