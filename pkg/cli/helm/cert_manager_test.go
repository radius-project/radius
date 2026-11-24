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
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	helm "helm.sh/helm/v4/pkg/action"
	chart "helm.sh/helm/v4/pkg/chart/v2"
	"helm.sh/helm/v4/pkg/release"
	releasecommon "helm.sh/helm/v4/pkg/release/common"
	releasev1 "helm.sh/helm/v4/pkg/release/v1"
	"helm.sh/helm/v4/pkg/storage/driver"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apiextfake "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/fake"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/radius-project/radius/pkg/authz"
	"github.com/radius-project/radius/test/k8sutil"
)

func certManagerCRD() *apiextensionsv1.CustomResourceDefinition {
	return &apiextensionsv1.CustomResourceDefinition{ObjectMeta: metav1.ObjectMeta{Name: certManagerCRDName}}
}

func certManagerController(namespace, versionLabel, image string) *appsv1.Deployment {
	labels := map[string]string{
		"app.kubernetes.io/name":      "cert-manager",
		"app.kubernetes.io/component": "controller",
	}
	if versionLabel != "" {
		labels["app.kubernetes.io/version"] = versionLabel
	}

	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "cert-manager", Namespace: namespace, Labels: labels},
		Spec: appsv1.DeploymentSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "cert-manager-controller", Image: image}},
		}}},
		Status: appsv1.DeploymentStatus{UpdatedReplicas: 1, AvailableReplicas: 1},
	}
}

// withSidecar puts an istio-proxy container running image before the containers of deployment.
func withSidecar(deployment *appsv1.Deployment, image string) *appsv1.Deployment {
	containers := deployment.Spec.Template.Spec.Containers
	deployment.Spec.Template.Spec.Containers = append([]corev1.Container{{Name: "istio-proxy", Image: image}}, containers...)
	return deployment
}

type fakeCertManagerCluster struct {
	crd        bool
	controller *appsv1.Deployment
	// injectorArgs are the arguments of the cainjector container.
	injectorArgs []string
}

func (c fakeCertManagerCluster) clients() (kubernetes.Interface, apiextclient.Interface) {
	kubeObjects := []runtime.Object{}
	if c.controller != nil {
		kubeObjects = append(kubeObjects, c.controller)
		webhook := certManagerController(c.controller.Namespace, "", "")
		webhook.Name = "cert-manager-webhook"
		webhook.Labels["app.kubernetes.io/name"] = "webhook"
		webhook.Labels["app.kubernetes.io/component"] = "webhook"
		cainjector := certManagerController(c.controller.Namespace, "", "")
		cainjector.Name = "cert-manager-cainjector"
		cainjector.Labels["app.kubernetes.io/name"] = "cainjector"
		cainjector.Labels["app.kubernetes.io/component"] = "cainjector"
		// The cert-manager chart's container name and image.
		cainjector.Spec.Template.Spec.Containers = []corev1.Container{{Name: "cert-manager-cainjector", Image: "quay.io/jetstack/cert-manager-cainjector:v1.21.2", Args: c.injectorArgs}}
		kubeObjects = append(kubeObjects, webhook, cainjector)
	}
	apiextObjects := []runtime.Object{}
	if c.crd {
		apiextObjects = append(apiextObjects, certManagerCRD())
	}

	return k8sfake.NewClientset(kubeObjects...), apiextfake.NewClientset(apiextObjects...)
}

func TestCertManagerClientsKubeconfigLoading(t *testing.T) {
	tests := []struct {
		name        string
		kubeconfig  []string
		contextName string
		wantServer  string
		wantErr     string
	}{
		{name: "default kubeconfig", wantServer: "home"},
		{name: "KUBECONFIG current context", kubeconfig: []string{"env"}, wantServer: "env"},
		{name: "KUBECONFIG replaces matching home context", kubeconfig: []string{"env"}, contextName: "shared", wantServer: "env"},
		{name: "explicit context in later merged file", kubeconfig: []string{"env", "extra"}, contextName: "extra-context", wantServer: "extra"},
		{name: "unknown context", kubeconfig: []string{"env"}, contextName: "missing-context", wantErr: "missing-context"},
		{name: "missing KUBECONFIG does not fall back home", kubeconfig: []string{"missing"}, wantErr: "no usable kubeconfig found"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			servers := map[string]*k8sutil.ForbiddenAPIServer{
				"home":  k8sutil.NewForbiddenAPIServer(t),
				"env":   k8sutil.NewForbiddenAPIServer(t),
				"extra": k8sutil.NewForbiddenAPIServer(t),
			}
			paths := map[string]string{
				"home":    k8sutil.IsolateKubeconfig(t),
				"env":     filepath.Join(t.TempDir(), "env"),
				"extra":   filepath.Join(t.TempDir(), "extra"),
				"missing": filepath.Join(t.TempDir(), "missing"),
			}
			k8sutil.WriteKubeconfig(t, paths["home"], "home-context", map[string]string{
				"home-context": servers["home"].URL,
				"shared":       servers["home"].URL,
			})
			k8sutil.WriteKubeconfig(t, paths["env"], "env-context", map[string]string{
				"env-context": servers["env"].URL,
				"shared":      servers["env"].URL,
			})
			k8sutil.WriteKubeconfig(t, paths["extra"], "extra-context", map[string]string{
				"extra-context": servers["extra"].URL,
			})
			if tt.kubeconfig != nil {
				var files []string
				for _, name := range tt.kubeconfig {
					files = append(files, paths[name])
				}
				t.Setenv("KUBECONFIG", strings.Join(files, string(os.PathListSeparator)))
			}

			kubeClient, apiextClient, err := newCertManagerClients(tt.contextName)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				require.Nil(t, kubeClient)
				require.Nil(t, apiextClient)
			} else {
				require.NoError(t, err)
				_, err = kubeClient.CoreV1().Namespaces().List(t.Context(), metav1.ListOptions{})
				require.True(t, apierrors.IsForbidden(err), "expected fixture rejection, got %v", err)
				_, err = apiextClient.ApiextensionsV1().CustomResourceDefinitions().List(t.Context(), metav1.ListOptions{})
				require.True(t, apierrors.IsForbidden(err), "expected fixture rejection, got %v", err)
			}

			dynamicClient, err := newCertManagerDynamicClient(tt.contextName)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				require.Nil(t, dynamicClient)
			} else {
				require.NoError(t, err)
				_, err = dynamicClient.Resource(certManagerResources[0]).List(t.Context(), metav1.ListOptions{})
				require.True(t, apierrors.IsForbidden(err), "expected fixture rejection, got %v", err)
			}
			for name, server := range servers {
				if name == tt.wantServer {
					require.Equal(t, []string{
						"GET /api/v1/namespaces",
						"GET /apis/apiextensions.k8s.io/v1/customresourcedefinitions",
						"GET /apis/cert-manager.io/v1/certificates",
					}, server.Requests())
				} else {
					require.Empty(t, server.Requests(), "must not contact the %s cluster", name)
				}
			}
		})
	}
}

func Test_resolveAuthzMode(t *testing.T) {
	t.Parallel()

	rbac := func(values map[string]any) map[string]any {
		return map[string]any{"global": map[string]any{"rbac": values}}
	}

	tests := []struct {
		name     string
		defaults map[string]any
		stored   map[string]any
		current  map[string]any
		want     authz.Mode
		wantErr  string
	}{
		{name: "defaults", want: authz.ModeOff},
		{name: "chart that enables authorization by default", defaults: rbac(map[string]any{"enabled": true, "dryRun": false}), want: authz.ModeEnforce},
		{name: "stored values override chart defaults", defaults: rbac(map[string]any{"enabled": true}), stored: rbac(map[string]any{"enabled": false}), want: authz.ModeOff},
		{name: "null --set clears chart default", defaults: rbac(map[string]any{"enabled": true}), current: rbac(map[string]any{"enabled": nil}), want: authz.ModeOff},
		{name: "dry run with chart that enforces by default", defaults: rbac(map[string]any{"enabled": true}), current: rbac(map[string]any{"dryRun": true}), wantErr: "cannot be combined"},
		{name: "unrelated values", current: map[string]any{"global": map[string]any{"zipkin": "x"}}, want: authz.ModeOff},
		{name: "dry run from --set", current: rbac(map[string]any{"dryRun": true}), want: authz.ModeDryRun},
		{name: "enforce from --set", current: rbac(map[string]any{"enabled": true}), want: authz.ModeEnforce},
		{name: "dry run from stored release values", stored: rbac(map[string]any{"dryRun": true}), want: authz.ModeDryRun},
		{name: "--set overrides stored", stored: rbac(map[string]any{"dryRun": true}), current: rbac(map[string]any{"dryRun": false}), want: authz.ModeOff},
		{name: "--set enforce with stored dry run disabled", stored: rbac(map[string]any{"dryRun": false}), current: rbac(map[string]any{"enabled": true}), want: authz.ModeEnforce},
		{name: "both enabled", current: rbac(map[string]any{"enabled": true, "dryRun": true}), wantErr: "cannot be combined"},
		{name: "both enabled across stored and --set", stored: rbac(map[string]any{"dryRun": true}), current: rbac(map[string]any{"enabled": true}), wantErr: "cannot be combined"},
		{name: "string values from --set-string", current: rbac(map[string]any{"dryRun": "true"}), want: authz.ModeDryRun},
		{name: "numeric value is truthy like the chart", current: rbac(map[string]any{"enabled": int64(1)}), want: authz.ModeEnforce},
		{name: "zero is falsy like the chart", stored: rbac(map[string]any{"enabled": true}), current: rbac(map[string]any{"enabled": int64(0)}), want: authz.ModeOff},
		{name: "newline-terminated file value is truthy", current: rbac(map[string]any{"enabled": "true\n"}), want: authz.ModeEnforce},
		{name: "null leaf clears stored value", stored: rbac(map[string]any{"dryRun": true}), current: rbac(map[string]any{"dryRun": nil}), want: authz.ModeOff},
		{name: "null rbac clears stored values", stored: rbac(map[string]any{"dryRun": true}), current: map[string]any{"global": map[string]any{"rbac": nil}}, want: authz.ModeOff},
		{name: "null global clears stored values", stored: rbac(map[string]any{"enabled": true}), current: map[string]any{"global": nil}, want: authz.ModeOff},
		{name: "false rbac clears stored values", stored: rbac(map[string]any{"dryRun": true}), current: map[string]any{"global": map[string]any{"rbac": false}}, want: authz.ModeOff},
		{name: "empty rbac map keeps stored values", stored: rbac(map[string]any{"dryRun": true}), current: rbac(map[string]any{}), want: authz.ModeDryRun},
		{name: "empty global map keeps stored values", stored: rbac(map[string]any{"enabled": true}), current: map[string]any{"global": map[string]any{}}, want: authz.ModeEnforce},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			defaults, stored, current := copyValues(tt.defaults), copyValues(tt.stored), copyValues(tt.current)
			got, err := resolveAuthzMode(tt.defaults, tt.stored, tt.current)
			require.Equal(t, defaults, copyValues(tt.defaults), "chart defaults must not be mutated")
			require.Equal(t, stored, copyValues(tt.stored), "stored values must not be mutated")
			require.Equal(t, current, copyValues(tt.current), "current values must not be mutated")
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func Test_resolveAuthzMode_FromCLIArgs(t *testing.T) {
	t.Parallel()

	storedDryRun := map[string]any{"global": map[string]any{"rbac": map[string]any{"dryRun": true}}}
	trueFile := filepath.Join(t.TempDir(), "enabled")
	require.NoError(t, os.WriteFile(trueFile, []byte("true\n"), 0600))

	tests := []struct {
		name    string
		stored  map[string]any
		options RadiusChartOptions
		want    authz.Mode
	}{
		{name: "--set global.rbac=null clears stored dry run", stored: storedDryRun, options: RadiusChartOptions{ChartOptions: ChartOptions{SetArgs: []string{"global.rbac=null"}}}, want: authz.ModeOff},
		{name: "--set global=null clears stored dry run", stored: storedDryRun, options: RadiusChartOptions{ChartOptions: ChartOptions{SetArgs: []string{"global=null"}}}, want: authz.ModeOff},
		{name: "--set global.rbac=false clears stored dry run", stored: storedDryRun, options: RadiusChartOptions{ChartOptions: ChartOptions{SetArgs: []string{"global.rbac=false"}}}, want: authz.ModeOff},
		{name: "--set global.rbac=0 clears stored dry run", stored: storedDryRun, options: RadiusChartOptions{ChartOptions: ChartOptions{SetArgs: []string{"global.rbac=0"}}}, want: authz.ModeOff},
		{name: "--set global.rbac= clears stored dry run", stored: storedDryRun, options: RadiusChartOptions{ChartOptions: ChartOptions{SetArgs: []string{"global.rbac="}}}, want: authz.ModeOff},
		{name: "--set enabled=1 enforces", options: RadiusChartOptions{ChartOptions: ChartOptions{SetArgs: []string{"global.rbac.enabled=1"}}}, want: authz.ModeEnforce},
		{name: "--set-file with trailing newline enforces", options: RadiusChartOptions{ChartOptions: ChartOptions{SetFileArgs: []string{"global.rbac.enabled=" + trueFile}}}, want: authz.ModeEnforce},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			current, err := parseUserValuesFromCLI(&tt.options)
			require.NoError(t, err)
			got, err := resolveAuthzMode(nil, tt.stored, current)
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func Test_detectCertManager(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		cluster       fakeCertManagerCluster
		wantInstalled bool
		wantVersion   string
		wantNamespace string
		wantErr       string
	}{
		{name: "not installed"},
		{
			name:          "version from label",
			cluster:       fakeCertManagerCluster{crd: true, controller: certManagerController("cert-manager", "v1.21.2", "quay.io/jetstack/cert-manager-controller:v1.20.0")},
			wantInstalled: true, wantVersion: "1.21.2", wantNamespace: "cert-manager",
		},
		{
			name:          "version from image tag",
			cluster:       fakeCertManagerCluster{crd: true, controller: certManagerController("security", "", "quay.io/jetstack/cert-manager-controller:v1.20.1")},
			wantInstalled: true, wantVersion: "1.20.1", wantNamespace: "security",
		},
		{
			name:          "version from image tag with digest",
			cluster:       fakeCertManagerCluster{crd: true, controller: certManagerController("cert-manager", "", "registry:5000/cert-manager-controller:v1.21.0@sha256:abc")},
			wantInstalled: true, wantVersion: "1.21.0", wantNamespace: "cert-manager",
		},
		{
			name:          "version from controller image after a sidecar",
			cluster:       fakeCertManagerCluster{crd: true, controller: withSidecar(certManagerController("cert-manager", "", "quay.io/jetstack/cert-manager-controller:v1.21.2"), "docker.io/istio/proxyv2:1.19.0")},
			wantInstalled: true, wantVersion: "1.21.2", wantNamespace: "cert-manager",
		},
		{
			name:          "old controller image after a newer sidecar",
			cluster:       fakeCertManagerCluster{crd: true, controller: withSidecar(certManagerController("cert-manager", "", "quay.io/jetstack/cert-manager-controller:v1.19.0"), "docker.io/istio/proxyv2:1.21.2")},
			wantInstalled: true, wantVersion: "1.19.0", wantNamespace: "cert-manager",
		},
		{
			name: "controller container not identified",
			cluster: fakeCertManagerCluster{crd: true, controller: func() *appsv1.Deployment {
				d := withSidecar(certManagerController("cert-manager", "", "registry.example.com/controller:v1.21.2"), "docker.io/istio/proxyv2:1.21.2")
				d.Spec.Template.Spec.Containers[1].Name = "manager"
				return d
			}()},
			wantErr: "could not determine the cert-manager version",
		},
		{
			name:    "CRD without controller",
			cluster: fakeCertManagerCluster{crd: true},
			wantErr: "no cert-manager controller",
		},
		{
			name:    "controller without CRD",
			cluster: fakeCertManagerCluster{controller: certManagerController("cert-manager", "v1.21.2", "")},
			wantErr: certManagerCRDName,
		},
		{
			name:    "unknown version",
			cluster: fakeCertManagerCluster{crd: true, controller: certManagerController("cert-manager", "", "cert-manager-controller")},
			wantErr: "could not determine the cert-manager version",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			kubeClient, apiextClient := tt.cluster.clients()
			got, err := detectCertManager(t.Context(), kubeClient, apiextClient)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.wantInstalled, got.Installed)
			require.Equal(t, tt.wantVersion, got.Version)
			require.Equal(t, tt.wantNamespace, got.Namespace)
		})
	}
}

func Test_ensureCertManager(t *testing.T) {
	t.Parallel()

	dryRun := []string{"global.rbac.dryRun=true"}
	storedDryRun := map[string]any{"global": map[string]any{"rbac": map[string]any{"dryRun": true}}}
	radOwned := &releasev1.Release{Name: certManagerReleaseName, Labels: radManagedByLabels(), Info: &releasev1.Info{Status: releasecommon.StatusDeployed}, Chart: &chart.Chart{Metadata: &chart.Metadata{Version: CertManagerChartDefaultVersion}}}
	radOwnedFailed := &releasev1.Release{Name: certManagerReleaseName, Labels: radManagedByLabels(), Info: &releasev1.Info{Status: releasecommon.StatusFailed}, Chart: radOwned.Chart}
	healthy := fakeCertManagerCluster{crd: true, controller: certManagerController(CertManagerNamespace, CertManagerChartDefaultVersion, "")}
	userOwned := &releasev1.Release{Name: certManagerReleaseName}

	expectGet := func(rel *releasev1.Release) func(*MockHelmClient) {
		return func(m *MockHelmClient) {
			if rel == nil {
				m.EXPECT().RunHelmGet(gomock.Any(), certManagerReleaseName).Return(nil, driver.ErrReleaseNotFound)
				return
			}
			m.EXPECT().RunHelmGet(gomock.Any(), certManagerReleaseName).Return(rel, nil)
		}
	}
	expectPull := func(t *testing.T, m *MockHelmClient) {
		m.EXPECT().RunHelmPull(gomock.Any(), certManagerReleaseName).
			DoAndReturn(func(pullopts []helm.PullOpt, chartRef string) (string, error) {
				pull := helm.NewPull(pullopts...)
				require.Equal(t, certManagerHelmRepo, pull.RepoURL)
				require.Equal(t, CertManagerChartDefaultVersion, pull.Version)
				require.NoError(t, os.WriteFile(filepath.Join(pull.DestDir, "Chart.yaml"), []byte("name: cert-manager\nversion: "+CertManagerChartDefaultVersion), 0644))
				return "Pulled", nil
			})
		m.EXPECT().LoadChart(gomock.Any()).Return(&chart.Chart{}, nil)
	}
	wantValues := map[string]any{"crds": map[string]any{"enabled": true, "keep": false}}

	tests := []struct {
		name    string
		setArgs []string
		// defaults is the values.yaml of the Radius chart.
		defaults map[string]any
		stored   map[string]any
		upgrade  bool
		cluster  fakeCertManagerCluster
		// upgraded, when set, is the cluster after rad upgrades its cert-manager release.
		upgraded   *fakeCertManagerCluster
		expect     func(*testing.T, *MockHelmClient)
		wantDetect bool
		// wantDetects, when set, is how many times rad detects cert-manager, instead of the once of wantDetect.
		wantDetects  int
		wantErr      string
		wantErrParts []string
	}{
		{
			name:   "mode off makes no cert-manager calls",
			expect: func(*testing.T, *MockHelmClient) {},
		},
		{
			name:    "no cert-manager installs pinned version",
			setArgs: dryRun,
			expect: func(t *testing.T, m *MockHelmClient) {
				expectGet(nil)(m)
				expectPull(t, m)
				m.EXPECT().RunHelmInstall(gomock.Any(), gomock.Any(), wantValues, certManagerReleaseName, CertManagerNamespace, true, DefaultInstallTimeout, radManagedByLabels()).
					Return(&releasev1.Release{}, nil)
			},
			wantDetect: true,
		},
		{
			// A chart that enables authorization by default needs cert-manager while the user values say nothing.
			name:     "chart that enables authorization by default installs pinned version",
			defaults: map[string]any{"global": map[string]any{"rbac": map[string]any{"enabled": true}}},
			expect: func(t *testing.T, m *MockHelmClient) {
				expectGet(nil)(m)
				expectPull(t, m)
				m.EXPECT().RunHelmInstall(gomock.Any(), gomock.Any(), wantValues, certManagerReleaseName, CertManagerNamespace, true, DefaultInstallTimeout, radManagedByLabels()).
					Return(&releasev1.Release{}, nil)
			},
			wantDetect: true,
		},
		{
			name:    "supported user-installed cert-manager is reused",
			setArgs: []string{"global.rbac.enabled=true"},
			cluster: fakeCertManagerCluster{crd: true, controller: certManagerController("security", "v1.20.0", "")},
			expect: func(t *testing.T, m *MockHelmClient) {
				expectGet(nil)(m)
			},
			wantDetect: true,
		},
		{
			name:    "user-owned Helm release is not upgraded",
			setArgs: dryRun,
			upgrade: true,
			cluster: fakeCertManagerCluster{crd: true, controller: certManagerController(CertManagerNamespace, "v1.21.0", "")},
			expect: func(t *testing.T, m *MockHelmClient) {
				expectGet(userOwned)(m)
			},
			wantDetect: true,
		},
		{
			name:    "older than minimum fails",
			setArgs: dryRun,
			cluster: fakeCertManagerCluster{crd: true, controller: certManagerController("cert-manager", "v1.19.6", "")},
			expect: func(t *testing.T, m *MockHelmClient) {
				expectGet(nil)(m)
			},
			wantDetect:   true,
			wantErrParts: []string{"v1.19.6", "v" + CertManagerMinimumVersion},
		},
		{
			name:    "CRD without controller fails",
			setArgs: dryRun,
			cluster: fakeCertManagerCluster{crd: true},
			expect: func(t *testing.T, m *MockHelmClient) {
				expectGet(nil)(m)
			},
			wantDetect: true,
			wantErr:    "no cert-manager controller",
		},
		{
			name:     "rad-owned release is upgraded",
			stored:   storedDryRun,
			upgrade:  true,
			upgraded: &healthy,
			expect: func(t *testing.T, m *MockHelmClient) {
				expectGet(radOwned)(m)
				expectPull(t, m)
				m.EXPECT().RunHelmUpgrade(gomock.Any(), gomock.Any(), wantValues, certManagerReleaseName, CertManagerNamespace, true, true, DefaultInstallTimeout).
					Return(&releasev1.Release{}, nil)
			},
			wantDetect: true,
		},
		{
			// The upgrade reuses the release's values, which can stop cainjector from injecting Radius's CA.
			name:     "rad-owned release upgraded with a restricted cainjector fails",
			stored:   storedDryRun,
			upgrade:  true,
			upgraded: &fakeCertManagerCluster{crd: true, controller: healthy.controller, injectorArgs: []string{"--namespace=security"}},
			expect: func(t *testing.T, m *MockHelmClient) {
				expectGet(radOwned)(m)
				expectPull(t, m)
				m.EXPECT().RunHelmUpgrade(gomock.Any(), gomock.Any(), wantValues, certManagerReleaseName, CertManagerNamespace, true, true, DefaultInstallTimeout).
					Return(&releasev1.Release{}, nil)
			},
			wantDetect: true,
			wantErr:    `is limited to namespace "security" by --namespace`,
		},
		{
			name:    "rad-owned release is reused on install",
			setArgs: dryRun,
			cluster: healthy,
			expect: func(t *testing.T, m *MockHelmClient) {
				expectGet(radOwned)(m)
			},
			wantDetect: true,
		},
		{
			name:     "rad-owned release missing its controller is repaired on install",
			setArgs:  dryRun,
			cluster:  fakeCertManagerCluster{crd: true},
			upgraded: &healthy,
			expect: func(t *testing.T, m *MockHelmClient) {
				expectGet(radOwned)(m)
				expectPull(t, m)
				m.EXPECT().RunHelmUpgrade(gomock.Any(), gomock.Any(), wantValues, certManagerReleaseName, CertManagerNamespace, true, true, DefaultInstallTimeout).
					Return(&releasev1.Release{}, nil)
			},
			wantDetects: 2,
		},
		{
			name:     "failed rad-owned release is repaired on install",
			setArgs:  dryRun,
			cluster:  healthy,
			upgraded: &healthy,
			expect: func(t *testing.T, m *MockHelmClient) {
				expectGet(radOwnedFailed)(m)
				expectPull(t, m)
				m.EXPECT().RunHelmUpgrade(gomock.Any(), gomock.Any(), wantValues, certManagerReleaseName, CertManagerNamespace, true, true, DefaultInstallTimeout).
					Return(&releasev1.Release{}, nil)
			},
			wantDetect: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			mockHelmClient := NewMockHelmClient(ctrl)
			tt.expect(t, mockHelmClient)

			// Record whether rad upgraded the cert-manager release, to serve the cluster after the upgrade.
			upgraded := false
			detectCalls := 0
			impl := &Impl{
				Helm: helmUpgradeObserver{HelmClient: mockHelmClient, upgraded: &upgraded},
				newCertManagerClients: func(kubeContext string) (kubernetes.Interface, apiextclient.Interface, error) {
					detectCalls++
					cluster := tt.cluster
					if upgraded && tt.upgraded != nil {
						cluster = *tt.upgraded
					}
					kubeClient, apiextClient := cluster.clients()
					return kubeClient, apiextClient, nil
				},
			}

			options := NewDefaultClusterOptions()
			options.Radius.SetArgs = tt.setArgs
			err := impl.ensureCertManager(t.Context(), options, "test-context", tt.defaults, tt.stored, cliUserValues(t, options), tt.upgrade)
			switch {
			case tt.wantErr != "":
				require.ErrorContains(t, err, tt.wantErr)
			case len(tt.wantErrParts) > 0:
				require.Error(t, err)
				for _, part := range tt.wantErrParts {
					require.ErrorContains(t, err, part)
				}
			default:
				require.NoError(t, err)
			}

			switch {
			case tt.wantDetects > 0:
				require.Equal(t, tt.wantDetects, detectCalls)
			case tt.wantDetect:
				require.Equal(t, 1, detectCalls)
			default:
				require.Zero(t, detectCalls)
			}
		})
	}
}

// helmUpgradeObserver records in upgraded that rad ran a Helm upgrade.
type helmUpgradeObserver struct {
	HelmClient
	upgraded *bool
}

func (o helmUpgradeObserver) RunHelmUpgrade(helmConf *helm.Configuration, helmChart *chart.Chart, vals map[string]any, releaseName, namespace string, wait bool, reuseValues bool, timeout time.Duration) (*releasev1.Release, error) {
	*o.upgraded = true
	return o.HelmClient.RunHelmUpgrade(helmConf, helmChart, vals, releaseName, namespace, wait, reuseValues, timeout)
}

func Test_ensureCertManager_RBACWarning(t *testing.T) {
	t.Parallel()

	storedDryRun := map[string]any{"global": map[string]any{"rbac": map[string]any{"dryRun": true}}}
	enabledFile := filepath.Join(t.TempDir(), "enabled")
	require.NoError(t, os.WriteFile(enabledFile, []byte("true\n"), 0o600))

	for _, tt := range []struct {
		name        string
		setArgs     []string
		setFileArgs []string
		stored      map[string]any
		upgrade     bool
		wantWarning bool
	}{
		{name: "off by default"},
		{name: "explicitly off", setArgs: []string{"global.rbac.enabled=false,global.rbac.dryRun=false"}},
		{name: "unrelated setting", setArgs: []string{"global.imageTag=edge"}},
		{name: "dry run", setArgs: []string{"global.rbac.dryRun=true"}, wantWarning: true},
		{name: "enforce", setArgs: []string{"global.rbac.enabled=true"}, wantWarning: true},
		{name: "file override", setFileArgs: []string{"global.rbac.enabled=" + enabledFile}, wantWarning: true},
		{name: "stored on reinstall", stored: storedDryRun, wantWarning: true},
		{name: "stored on upgrade", stored: storedDryRun, upgrade: true, wantWarning: true},
		{name: "disable stored dry run", stored: storedDryRun, setArgs: []string{"global.rbac.dryRun=false"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client := NewMockHelmClient(gomock.NewController(t))
			stop := errors.New("stop before managing cert-manager")
			var logs []string
			if tt.wantWarning {
				client.EXPECT().RunHelmGet(gomock.Any(), certManagerReleaseName).
					DoAndReturn(func(*helm.Configuration, string) (*releasev1.Release, error) {
						require.Len(t, logs, 1, "warn before contacting cert-manager")
						return nil, stop
					})
			}
			impl := &Impl{
				Helm: client,
				logger: func(format string, args ...any) {
					logs = append(logs, fmt.Sprintf(format, args...))
				},
			}
			options := NewDefaultClusterOptions()
			options.Radius.SetArgs = tt.setArgs
			options.Radius.SetFileArgs = tt.setFileArgs
			err := impl.ensureCertManager(t.Context(), options, "test-context", nil, tt.stored, cliUserValues(t, options), tt.upgrade)
			if !tt.wantWarning {
				require.NoError(t, err)
				require.Empty(t, logs)
				return
			}
			require.ErrorIs(t, err, stop)
			require.Equal(t, []string{
				"Warning: Radius RBAC is a work in progress. Full functionality is not guaranteed. Do not rely on it for production access control.",
			}, logs)
		})
	}
}

func Test_Helm_InstallRadius_CertManagerTooOld_StopsBeforeRadius(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mockHelmClient := NewMockHelmClient(ctrl)
	mockHelmClient.EXPECT().RunHelmGet(gomock.Any(), certManagerReleaseName).Return(nil, driver.ErrReleaseNotFound)
	mockHelmClient.EXPECT().LoadChart("radius-chart").Return(certificateTestChart(t), nil)

	impl := &Impl{
		Helm: mockHelmClient,
		newCertManagerClients: func(string) (kubernetes.Interface, apiextclient.Interface, error) {
			kubeClient, apiextClient := fakeCertManagerCluster{crd: true, controller: certManagerController("cert-manager", "v1.18.0", "")}.clients()
			return kubeClient, apiextClient, nil
		},
	}

	options := NewDefaultClusterOptions()
	options.Radius.ChartPath = "radius-chart"
	options.Contour.Disabled = true
	options.Radius.SetArgs = []string{"global.rbac.dryRun=true"}
	err := impl.InstallRadius(t.Context(), options, "test-context")
	require.ErrorContains(t, err, "v"+CertManagerMinimumVersion)
}

func Test_CertManagerPendingRelease_StopsBeforeChanges(t *testing.T) {
	k8sutil.WriteKubeconfig(t, k8sutil.IsolateKubeconfig(t), "prod west", map[string]string{
		"prod west": "https://prod.example.test",
	})

	for _, status := range []releasecommon.Status{
		releasecommon.StatusPendingInstall, releasecommon.StatusPendingUpgrade, releasecommon.StatusPendingRollback,
	} {
		for _, owned := range []bool{false, true} {
			for _, command := range []string{"install", "install --reinstall", "upgrade"} {
				for _, kubeContext := range []string{"prod west", ""} {
					t.Run(fmt.Sprintf("%s/owned=%t/%s/context=%s", status, owned, command, kubeContext), func(t *testing.T) {
						t.Parallel()
						options := NewDefaultClusterOptions()
						options.Radius.ChartPath = "radius-chart"
						options.Contour.Disabled = true
						options.CertManager.ReleaseName = "shared-cert-manager"
						options.CertManager.Namespace = "security"
						if command == "install" {
							options.Radius.SetArgs = []string{"global.rbac.enabled=true"}
						} else {
							options.RadiusReleaseValues = map[string]any{"global": map[string]any{"rbac": map[string]any{"dryRun": true}}}
							options.Radius.Reinstall = command == "install --reinstall"
						}
						release := &releasev1.Release{
							Name: options.CertManager.ReleaseName, Namespace: options.CertManager.Namespace,
							Version: 3, Info: &releasev1.Info{Status: status},
							Chart: &chart.Chart{Metadata: &chart.Metadata{Version: CertManagerChartDefaultVersion}},
						}
						if owned {
							release.Labels = radManagedByLabels()
						}
						client := NewMockHelmClient(gomock.NewController(t))
						if command != "install" {
							expectRadiusBaseline(t, client, options)
						}
						client.EXPECT().LoadChart(options.Radius.ChartPath).Return(certificateTestChart(t), nil)
						client.EXPECT().RunHelmGet(gomock.Any(), options.CertManager.ReleaseName).Return(release, nil)
						impl := &Impl{
							Helm: client,
							newCertManagerClients: func(string) (kubernetes.Interface, apiextclient.Interface, error) {
								t.Error("pending release must stop before Kubernetes detection or readiness checks")
								return nil, nil, errors.New("unexpected Kubernetes detection")
							},
						}
						var err error
						if command == "upgrade" {
							err = impl.UpgradeRadius(t.Context(), options, kubeContext)
						} else {
							err = impl.InstallRadius(t.Context(), options, kubeContext)
						}
						require.Error(t, err)
						for _, part := range []string{
							`"shared-cert-manager"`, `"security"`, "revision 3", string(status),
							"may still be running", "confirm that no Helm operation is active",
							"Do not uninstall cert-manager or delete its CRDs or Helm release Secrets",
							"other workloads", "certificates",
						} {
							require.ErrorContains(t, err, part)
						}
						helmPrefix := `helm --kube-context "prod west"`
						kubectlPrefix := `kubectl --context "prod west"`
						require.ErrorContains(t, err, `context "prod west"`)
						require.ErrorContains(t, err, helmPrefix+" -n security status shared-cert-manager")
						require.ErrorContains(t, err, helmPrefix+" -n security history shared-cert-manager")
						require.ErrorContains(t, err, kubectlPrefix+" -n security get deployments,jobs,pods")
						require.ErrorContains(t, err, kubectlPrefix+" get certificates,certificaterequests,issuers,clusterissuers,orders.acme.cert-manager.io,challenges.acme.cert-manager.io -A")
						if status == releasecommon.StatusPendingInstall {
							require.ErrorContains(t, err, "no successful revision to roll back to")
							require.NotContains(t, err.Error(), "rollback shared-cert-manager")
						} else {
							require.ErrorContains(t, err, helmPrefix+" -n security rollback shared-cert-manager REVISION --wait")
							require.ErrorContains(t, err, "replace REVISION with a known-good revision from the history")
						}
						if !owned {
							require.ErrorContains(t, err, "not owned by rad")
							require.ErrorContains(t, err, "installation's operator")
						}
						require.Equal(t, status, release.Info.Status)
						require.Equal(t, 3, release.Version)
					})
				}
			}
		}
	}
}

func Test_CertManagerPendingRelease_PinsImplicitContext(t *testing.T) {
	configPath := k8sutil.IsolateKubeconfig(t)
	servers := map[string]string{
		"prod west": "https://prod.example.test",
		"decoy":     "https://decoy.example.test",
	}
	k8sutil.WriteKubeconfig(t, configPath, "prod west", servers)
	client := NewMockHelmClient(gomock.NewController(t))
	client.EXPECT().RunHelmGet(gomock.Any(), certManagerReleaseName).
		DoAndReturn(func(config *helm.Configuration, _ string) (*releasev1.Release, error) {
			checked, err := config.RESTClientGetter.ToRESTConfig()
			require.NoError(t, err)
			require.Equal(t, servers["prod west"], checked.Host)
			k8sutil.WriteKubeconfig(t, configPath, "decoy", servers)
			return &releasev1.Release{Version: 2, Info: &releasev1.Info{Status: releasecommon.StatusPendingUpgrade}}, nil
		})
	options := NewDefaultClusterOptions()
	options.Radius.SetArgs = []string{"global.rbac.enabled=true"}
	err := (&Impl{Helm: client}).ensureCertManager(t.Context(), options, "", nil, nil, cliUserValues(t, options), true)
	require.ErrorContains(t, err, `helm --kube-context "prod west" -n cert-manager rollback cert-manager REVISION --wait`)
	require.ErrorContains(t, err, `kubectl --context "prod west"`)
	require.NotContains(t, err.Error(), "decoy")
	require.Equal(t, servers["decoy"], helmRESTConfig(t, "").Host, "ambient context really changed")
	require.Equal(t, servers["prod west"], helmRESTConfig(t, "prod west").Host, "suggested context still selects the inspected cluster")
}

func Test_CertManagerPendingRelease_ContextResolution(t *testing.T) {
	for _, invalid := range []bool{false, true} {
		t.Run(fmt.Sprintf("invalid=%t", invalid), func(t *testing.T) {
			configPath := k8sutil.IsolateKubeconfig(t)
			client := NewMockHelmClient(gomock.NewController(t))
			if invalid {
				require.NoError(t, os.MkdirAll(filepath.Dir(configPath), 0700))
				require.NoError(t, os.WriteFile(configPath, []byte("invalid: ["), 0600))
			} else {
				client.EXPECT().RunHelmGet(gomock.Any(), certManagerReleaseName).
					Return(&releasev1.Release{Info: &releasev1.Info{Status: releasecommon.StatusPendingUpgrade}}, nil)
			}
			options := NewDefaultClusterOptions()
			options.Radius.SetArgs = []string{"global.rbac.enabled=true"}
			err := (&Impl{Helm: client}).ensureCertManager(t.Context(), options, "", nil, nil, cliUserValues(t, options), true)
			if invalid {
				require.ErrorContains(t, err, "failed to resolve cert-manager kubeconfig context")
			} else {
				require.ErrorContains(t, err, "No named kubeconfig context was selected")
				require.ErrorContains(t, err, "explicitly select and verify the cluster that rad inspected")
			}
		})
	}
}

func Test_ensureCertManager_ReleaseLookupErrorDoesNotRepair(t *testing.T) {
	t.Parallel()
	for _, upgrade := range []bool{false, true} {
		t.Run(fmt.Sprintf("upgrade=%t", upgrade), func(t *testing.T) {
			t.Parallel()
			failure := apierrors.NewForbidden(schema.GroupResource{Resource: "secrets"}, "", errors.New("denied"))
			client := NewMockHelmClient(gomock.NewController(t))
			client.EXPECT().RunHelmGet(gomock.Any(), certManagerReleaseName).Return(nil, failure)
			impl := &Impl{Helm: client}
			options := NewDefaultClusterOptions()
			options.Radius.SetArgs = []string{"global.rbac.dryRun=true"}
			err := impl.ensureCertManager(t.Context(), options, "test-context", nil, nil, cliUserValues(t, options), upgrade)
			require.ErrorIs(t, err, failure)
			require.ErrorContains(t, err, "failed to run helm get")
			require.NotContains(t, err.Error(), "rollback")
		})
	}
}

func Test_ensureCertManager_NewerOwnedRelease(t *testing.T) {
	t.Parallel()
	for _, upgrade := range []bool{false, true} {
		for _, healthy := range []bool{false, true} {
			t.Run(fmt.Sprintf("upgrade=%t/healthy=%t", upgrade, healthy), func(t *testing.T) {
				t.Parallel()
				client := NewMockHelmClient(gomock.NewController(t))
				status := releasecommon.StatusFailed
				if healthy {
					status = releasecommon.StatusDeployed
				}
				client.EXPECT().RunHelmGet(gomock.Any(), certManagerReleaseName).Return(&releasev1.Release{
					Name: certManagerReleaseName, Labels: radManagedByLabels(),
					Chart: &chart.Chart{Metadata: &chart.Metadata{Version: "v1.22.1"}},
					Info:  &releasev1.Info{Status: status},
				}, nil)
				impl := &Impl{Helm: client, newCertManagerClients: func(string) (kubernetes.Interface, apiextclient.Interface, error) {
					k, a := (fakeCertManagerCluster{crd: true, controller: certManagerController(CertManagerNamespace, "v1.22.1", "")}).clients()
					return k, a, nil
				}}
				options := NewDefaultClusterOptions()
				options.Radius.SetArgs = []string{"global.rbac.dryRun=true"}
				err := impl.ensureCertManager(t.Context(), options, "test-context", nil, nil, cliUserValues(t, options), upgrade)
				if healthy {
					require.NoError(t, err)
				} else {
					require.ErrorContains(t, err, "rad will not downgrade")
				}
			})
		}
	}
}

func Test_ensureCertManager_DetectionFailureDoesNotRepair(t *testing.T) {
	t.Parallel()
	for _, failure := range []error{
		apierrors.NewForbidden(schema.GroupResource{Resource: "deployments"}, "", errors.New("denied")),
		apierrors.NewServiceUnavailable("temporarily unavailable"),
	} {
		t.Run(failure.Error(), func(t *testing.T) {
			t.Parallel()
			client := NewMockHelmClient(gomock.NewController(t))
			client.EXPECT().RunHelmGet(gomock.Any(), certManagerReleaseName).Return(&releasev1.Release{
				Name: certManagerReleaseName, Labels: radManagedByLabels(),
				Chart: &chart.Chart{Metadata: &chart.Metadata{Version: CertManagerChartDefaultVersion}},
				Info:  &releasev1.Info{Status: releasecommon.StatusDeployed},
			}, nil)
			impl := &Impl{Helm: client, newCertManagerClients: func(string) (kubernetes.Interface, apiextclient.Interface, error) {
				k, a := (fakeCertManagerCluster{crd: true, controller: certManagerController(CertManagerNamespace, CertManagerChartDefaultVersion, "")}).clients()
				k.(*k8sfake.Clientset).PrependReactor("list", "deployments", func(k8stesting.Action) (bool, runtime.Object, error) {
					return true, nil, failure
				})
				return k, a, nil
			}}
			options := NewDefaultClusterOptions()
			options.Radius.SetArgs = []string{"global.rbac.dryRun=true"}
			err := impl.ensureCertManager(t.Context(), options, "test-context", nil, nil, cliUserValues(t, options), false)
			require.ErrorIs(t, err, failure)
		})
	}
}

func Test_checkCertManagerReady(t *testing.T) {
	t.Parallel()
	for _, component := range []string{"controller", "webhook", "cainjector"} {
		for _, failure := range []string{"missing", "scaled down", "unavailable", "stale rollout", "old replicas", "forbidden"} {
			t.Run(component+"/"+failure, func(t *testing.T) {
				t.Parallel()
				k, _ := (fakeCertManagerCluster{controller: certManagerController("security", "v1.21.2", "")}).clients()
				name := "cert-manager"
				if component != "controller" {
					name += "-" + component
				}
				deployments := k.AppsV1().Deployments("security")
				deployment, err := deployments.Get(t.Context(), name, metav1.GetOptions{})
				require.NoError(t, err)
				switch failure {
				case "missing":
					require.NoError(t, deployments.Delete(t.Context(), name, metav1.DeleteOptions{}))
				case "scaled down":
					deployment.Spec.Replicas = new(int32(0))
				case "unavailable":
					deployment.Status.AvailableReplicas = 0
				case "stale rollout":
					deployment.Generation = 2
					deployment.Status.ObservedGeneration = 1
				case "old replicas":
					deployment.Status.UpdatedReplicas = 0
				case "forbidden":
					k.(*k8sfake.Clientset).PrependReactor("list", "deployments", func(k8stesting.Action) (bool, runtime.Object, error) {
						return true, nil, apierrors.NewForbidden(schema.GroupResource{Resource: "deployments"}, "", errors.New("denied"))
					})
				}
				if failure != "missing" {
					_, err = deployments.Update(t.Context(), deployment, metav1.UpdateOptions{})
					require.NoError(t, err)
				}
				require.Error(t, checkCertManagerReady(t.Context(), k, "security", RadiusSystemNamespace, t.Logf))
			})
		}
	}
}

func Test_checkCertManagerReady_CAInjector(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		args    []string
		sidecar []string
		// mirrored renames the cainjector container and mirrors its image, so only its command identifies it.
		mirrored bool
		// command adds flags after the cainjector executable in the command of the mirrored container.
		command []string
		// unidentified renames the cainjector container and mirrors its image without a command that identifies it.
		unidentified bool
		// env is the environment of the cainjector container, which Kubernetes expands in $(NAME) references of its
		// args.
		env         []corev1.EnvVar
		wantErr     string
		wantWarning bool
		// wantNamespaceWarning is set when rad cannot resolve the namespace flag and warns instead of checking it.
		wantNamespaceWarning bool
	}{
		{name: "chart defaults", args: []string{"--v=2", "--leader-election-namespace=kube-system"}},
		{name: "injection explicitly enabled", args: []string{"--enable-apiservices-injectable=true", "--enable-certificates-data-source"}},
		{name: "limited to the Radius namespace", args: []string{"--namespace", RadiusSystemNamespace}},
		{name: "APIService injection disabled", args: []string{"--enable-apiservices-injectable=false"}, wantErr: "sets --enable-apiservices-injectable=false"},
		// Radius injects from a Secret, not a Certificate.
		{name: "Certificate data source disabled", args: []string{"--enable-certificates-data-source=false"}},
		{name: "ignores other namespaces", args: []string{"--ignore-namespaces=kube-system,security"}},
		{name: "ignores the Radius namespace", args: []string{"--ignore-namespaces", "kube-system," + RadiusSystemNamespace}, wantErr: `ignores namespace "radius-system" by --ignore-namespaces`},
		{name: "limited to another namespace", args: []string{"--namespace=security"}, wantErr: `is limited to namespace "security" by --namespace`},
		{name: "limited to another namespace with a separate value", args: []string{"--namespace", "security"}, wantErr: `is limited to namespace "security" by --namespace`},
		// Only the cainjector container's flags configure injection; a sidecar's flags of the same name do not.
		{name: "sidecar limited to another namespace", sidecar: []string{"--namespace=security", "--enable-apiservices-injectable=false"}},
		{name: "restricted injector beside a sidecar", args: []string{"--namespace=security"}, sidecar: []string{"--v=2"}, wantErr: `is limited to namespace "security" by --namespace`},
		{name: "renamed mirrored injector beside a restricted sidecar", mirrored: true, sidecar: []string{"--namespace=security"}},
		// With nothing to identify cainjector among several containers, rad reads no container's flags and warns.
		{name: "unidentified injector beside a restricted sidecar", unidentified: true, sidecar: []string{"--namespace=security"}, wantWarning: true},
		{name: "unidentified restricted injector beside a sidecar", unidentified: true, args: []string{"--namespace=security"}, sidecar: []string{"--v=2"}, wantWarning: true},
		// A Deployment with one container runs cainjector in it.
		{name: "unidentified lone restricted injector", unidentified: true, args: []string{"--namespace=security"}, wantErr: `is limited to namespace "security" by --namespace`},
		{name: "renamed mirrored injector with APIService injection disabled in its command", mirrored: true, command: []string{"--enable-apiservices-injectable=false"},
			sidecar: []string{"--v=2"}, wantErr: "sets --enable-apiservices-injectable=false"},
		{name: "renamed mirrored injector restricted by its command and args", mirrored: true, command: []string{"--namespace"}, args: []string{"security"},
			wantErr: `is limited to namespace "security" by --namespace`},
		{name: "renamed mirrored restricted injector beside a sidecar", mirrored: true, args: []string{"--namespace=security"}, sidecar: []string{"--v=2"}, wantErr: `is limited to namespace "security" by --namespace`},
		// Kubernetes expands $(NAME) references to the container's environment in its args.
		{name: "namespace from the environment is the Radius namespace", args: []string{"--namespace=$(WATCH_NAMESPACE)"},
			env: []corev1.EnvVar{{Name: "WATCH_NAMESPACE", Value: RadiusSystemNamespace}}},
		{name: "namespace from the environment is another namespace", args: []string{"--namespace", "$(WATCH_NAMESPACE)"},
			env: []corev1.EnvVar{{Name: "WATCH_NAMESPACE", Value: "security"}}, wantErr: `is limited to namespace "security" by --namespace`},
		{name: "namespace from the pod namespace", args: []string{"--namespace=$(POD_NAMESPACE)"},
			env:     []corev1.EnvVar{{Name: "POD_NAMESPACE", ValueFrom: &corev1.EnvVarSource{FieldRef: &corev1.ObjectFieldSelector{FieldPath: "metadata.namespace"}}}},
			wantErr: `is limited to namespace "security" by --namespace`},
		{name: "namespace from an environment variable that refers to another", args: []string{"--namespace=$(WATCH_NAMESPACE)"},
			env: []corev1.EnvVar{{Name: "BASE", Value: "secur"}, {Name: "WATCH_NAMESPACE", Value: "$(BASE)ity"}}, wantErr: `is limited to namespace "security" by --namespace`},
		{name: "escaped reference is literal", args: []string{"--namespace=$$(WATCH_NAMESPACE)"},
			env: []corev1.EnvVar{{Name: "WATCH_NAMESPACE", Value: RadiusSystemNamespace}}, wantErr: `is limited to namespace "$(WATCH_NAMESPACE)" by --namespace`},
		{name: "ignored namespaces from the environment", args: []string{"--ignore-namespaces=$(IGNORED)"},
			env: []corev1.EnvVar{{Name: "IGNORED", Value: "kube-system," + RadiusSystemNamespace}}, wantErr: `ignores namespace "radius-system" by --ignore-namespaces`},
		{name: "APIService injection disabled by a flag from the environment", args: []string{"$(INJECTABLE)"},
			env: []corev1.EnvVar{{Name: "INJECTABLE", Value: "--enable-apiservices-injectable=false"}}, wantErr: "sets --enable-apiservices-injectable=false"},
		// A value that only the kubelet can resolve leaves the namespace unknown, so rad warns rather than refusing.
		{name: "namespace from a Secret", args: []string{"--namespace=$(WATCH_NAMESPACE)"},
			env:                  []corev1.EnvVar{{Name: "WATCH_NAMESPACE", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{Key: "namespace"}}}},
			wantNamespaceWarning: true},
		{name: "namespace from an undefined variable", args: []string{"--namespace=$(WATCH_NAMESPACE)"}, wantNamespaceWarning: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			k, _ := (fakeCertManagerCluster{controller: certManagerController("security", "v1.21.2", "")}).clients()
			deployments := k.AppsV1().Deployments("security")
			deployment, err := deployments.Get(t.Context(), "cert-manager-cainjector", metav1.GetOptions{})
			require.NoError(t, err)
			deployment.Spec.Template.Spec.Containers[0].Args = tt.args
			deployment.Spec.Template.Spec.Containers[0].Env = tt.env
			if tt.mirrored {
				injector := &deployment.Spec.Template.Spec.Containers[0]
				injector.Name = "injector"
				injector.Image = "registry.example.com/platform/injector:v1"
				injector.Command = append([]string{"/usr/local/bin/cainjector"}, tt.command...)
			}
			if tt.unidentified {
				injector := &deployment.Spec.Template.Spec.Containers[0]
				injector.Name = "injector"
				injector.Image = "registry.example.com/platform/injector:v1"
				injector.Command = []string{"/usr/local/bin/injector"}
			}
			if tt.sidecar != nil {
				// A sidecar listed first, like the injected proxies of some service meshes.
				deployment.Spec.Template.Spec.Containers = append([]corev1.Container{{Name: "proxy", Image: "example.com/mesh/proxy:v1", Args: tt.sidecar}}, deployment.Spec.Template.Spec.Containers...)
			}
			_, err = deployments.Update(t.Context(), deployment, metav1.UpdateOptions{})
			require.NoError(t, err)

			var warnings []string
			err = checkCertManagerReady(t.Context(), k, "security", RadiusSystemNamespace, func(format string, v ...any) {
				warnings = append(warnings, fmt.Sprintf(format, v...))
			})
			if tt.wantWarning {
				require.Equal(t, []string{`Warning: rad could not tell which container of cert-manager cainjector Deployment security/cert-manager-cainjector ("proxy", "injector") runs cainjector, ` +
					`so it did not check that cainjector can inject the CA certificate in Secret radius-system/radius-apiserver-ca into the Radius APIService`}, warnings)
			} else if tt.wantNamespaceWarning {
				require.Equal(t, []string{`Warning: rad could not resolve the --namespace value "$(WATCH_NAMESPACE)" of cert-manager cainjector container "cert-manager-cainjector", ` +
					`so it did not check that cainjector watches namespace "radius-system"`}, warnings)
			} else {
				require.Empty(t, warnings)
			}
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, "cert-manager cainjector Deployment security/cert-manager-cainjector "+tt.wantErr)
			require.ErrorContains(t, err, "inject the CA certificate in Secret radius-system/radius-apiserver-ca into the Radius APIService")
		})
	}
}

func Test_ensureCertManager_UnreadyWebhookIsNotReused(t *testing.T) {
	t.Parallel()
	for _, version := range []string{"external", CertManagerChartDefaultVersion, "v1.22.1"} {
		t.Run(version, func(t *testing.T) {
			t.Parallel()
			client := NewMockHelmClient(gomock.NewController(t))
			var rel *releasev1.Release
			if version != "external" {
				rel = &releasev1.Release{
					Name: certManagerReleaseName, Labels: radManagedByLabels(),
					Chart: &chart.Chart{Metadata: &chart.Metadata{Version: version}},
					Info:  &releasev1.Info{Status: releasecommon.StatusDeployed},
				}
			}
			client.EXPECT().RunHelmGet(gomock.Any(), certManagerReleaseName).Return(rel, nil)
			k, a := (fakeCertManagerCluster{crd: true, controller: certManagerController(CertManagerNamespace, "v1.22.1", "")}).clients()
			webhooks := k.AppsV1().Deployments(CertManagerNamespace)
			webhook, err := webhooks.Get(t.Context(), "cert-manager-webhook", metav1.GetOptions{})
			require.NoError(t, err)
			webhook.Status.AvailableReplicas = 0
			_, err = webhooks.Update(t.Context(), webhook, metav1.UpdateOptions{})
			require.NoError(t, err)
			impl := &Impl{Helm: client, newCertManagerClients: func(string) (kubernetes.Interface, apiextclient.Interface, error) {
				return k, a, nil
			}}
			options := NewDefaultClusterOptions()
			options.Radius.SetArgs = []string{"global.rbac.dryRun=true"}
			err = impl.ensureCertManager(t.Context(), options, "test-context", nil, nil, cliUserValues(t, options), version == "v1.22.1")
			require.ErrorContains(t, err, "cert-manager webhook Deployment")
			require.ErrorContains(t, err, "is not ready")
		})
	}
}

func certManagerResource(kind, namespace, name string) *unstructured.Unstructured {
	u := &unstructured.Unstructured{}
	u.SetAPIVersion("cert-manager.io/v1")
	if kind == "Order" || kind == "Challenge" {
		u.SetAPIVersion("acme.cert-manager.io/v1")
	}
	u.SetKind(kind)
	u.SetNamespace(namespace)
	u.SetName(name)
	return u
}

func fakeCertManagerDynamicClient(listErr error, objects ...runtime.Object) *dynamicfake.FakeDynamicClient {
	listKinds := map[schema.GroupVersionResource]string{}
	for _, r := range certManagerResources {
		listKinds[r] = r.Resource + "List"
	}
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), listKinds, objects...)
	if listErr != nil {
		client.PrependReactor("list", "*", func(k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, listErr
		})
	}
	return client
}

func Test_UninstallCertManager(t *testing.T) {
	t.Parallel()

	radOwned := &releasev1.Release{Name: certManagerReleaseName, Labels: radManagedByLabels()}

	tests := []struct {
		name          string
		release       *releasev1.Release
		getErr        error
		objects       []runtime.Object
		listErr       error
		wantUninstall bool
		wantLog       string
		wantErr       bool
	}{
		{name: "not installed", getErr: driver.ErrReleaseNotFound},
		{name: "user-owned is kept", release: &releasev1.Release{Name: certManagerReleaseName}},
		{name: "get error keeps cert-manager without blocking purge", getErr: errors.New("boom"), wantLog: "Keeping cert-manager: could not check whether rad installed it:"},
		{name: "missing resource types permit removal", release: radOwned, listErr: apierrors.NewNotFound(schema.GroupResource{Group: "cert-manager.io", Resource: "certificates"}, ""), wantUninstall: true},
		{name: "unmapped resource types permit removal", release: radOwned, listErr: &meta.NoResourceMatchError{PartialResource: certManagerResources[0]}, wantUninstall: true},
		{
			name:          "rad-owned without foreign resources is removed",
			release:       radOwned,
			objects:       []runtime.Object{certManagerResource("Certificate", RadiusSystemNamespace, "radius-cert")},
			wantUninstall: true,
		},
		{
			name:    "rad-owned with a foreign Certificate is kept",
			release: radOwned,
			objects: []runtime.Object{
				certManagerResource("Certificate", RadiusSystemNamespace, "radius-cert"),
				certManagerResource("Certificate", "app", "web-tls"),
			},
			wantLog: "Keeping cert-manager: other workloads use it (1 resources, e.g. app/Certificate/web-tls)",
		},
		{
			name:    "rad-owned with a ClusterIssuer is kept",
			release: radOwned,
			objects: []runtime.Object{certManagerResource("ClusterIssuer", "", "letsencrypt")},
			wantLog: "Keeping cert-manager: other workloads use it (1 resources, e.g. ClusterIssuer/letsencrypt)",
		},
		{
			name:    "foreign ACME resources keep cert-manager",
			release: radOwned,
			objects: []runtime.Object{certManagerResource("Order", "app", "renewal"), certManagerResource("Challenge", "app", "challenge")},
			wantLog: "Keeping cert-manager: other workloads use it (2 resources",
		},
		{
			name:    "list error keeps cert-manager",
			release: radOwned,
			listErr: errors.New("forbidden"),
			wantLog: "Keeping cert-manager: could not check whether other workloads use it:",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			mockHelmClient := NewMockHelmClient(ctrl)
			mockHelmClient.EXPECT().RunHelmGet(gomock.Any(), certManagerReleaseName).Return(tt.release, tt.getErr)
			if tt.wantUninstall {
				mockHelmClient.EXPECT().RunHelmUninstall(gomock.Any(), certManagerReleaseName, CertManagerNamespace, true).
					Return(&release.UninstallReleaseResponse{}, nil)
			}

			var logs []string
			impl := &Impl{
				Helm: mockHelmClient,
				newCertManagerDynamicClient: func(string) (dynamic.Interface, error) {
					return fakeCertManagerDynamicClient(tt.listErr, tt.objects...), nil
				},
				logger: func(format string, v ...any) { logs = append(logs, fmt.Sprintf(format, v...)) },
			}
			err := impl.UninstallCertManager(t.Context(), NewDefaultClusterOptions(), "test-context")
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			if tt.wantLog == "" {
				require.Empty(t, logs)
				return
			}
			require.Len(t, logs, 1)
			require.True(t, strings.HasPrefix(logs[0], tt.wantLog), logs[0])
		})
	}
}

func Test_UninstallRadius_CertManagerOwnershipErrorDoesNotBlockCleanup(t *testing.T) {
	t.Parallel()
	client := NewMockHelmClient(gomock.NewController(t))
	for _, name := range []string{radiusReleaseName, contourReleaseName} {
		client.EXPECT().RunHelmUninstall(gomock.Any(), name, RadiusSystemNamespace, true).Return(&release.UninstallReleaseResponse{}, nil)
	}
	client.EXPECT().RunHelmGet(gomock.Any(), certManagerReleaseName).Return(nil, errors.New("release storage unavailable"))
	var logs []string
	impl := &Impl{
		Helm:                        client,
		removeDefaultContourGateway: (&defaultContourGatewayCalls{}).remove,
		newUCPAPIServerTrustClient:  uninstallTrustClient,
		newCertManagerDynamicClient: uninstallStageClient,
		logger:                      func(format string, v ...any) { logs = append(logs, fmt.Sprintf(format, v...)) },
	}
	require.NoError(t, impl.UninstallRadius(t.Context(), NewDefaultClusterOptions(), "test-context"))
	require.Len(t, logs, 1)
	require.Contains(t, logs[0], "Keeping cert-manager: could not check whether rad installed it")
}

func Test_foreignCertManagerResources_MissingTypeDoesNotHideOtherTypes(t *testing.T) {
	t.Parallel()
	client := fakeCertManagerDynamicClient(nil, certManagerResource("Issuer", "app", "issuer"))
	client.PrependReactor("list", "certificates", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewNotFound(schema.GroupResource{Group: "cert-manager.io", Resource: "certificates"}, "")
	})
	foreign, err := foreignCertManagerResources(t.Context(), client, RadiusSystemNamespace)
	require.NoError(t, err)
	require.Equal(t, []string{"app/Issuer/issuer"}, foreign)
}

// cliUserValues returns the values that prepareRadiusChart reads from the command line options.
func cliUserValues(t *testing.T, options ClusterOptions) map[string]any {
	t.Helper()
	values, err := parseUserValuesFromCLI(&options.Radius)
	require.NoError(t, err)
	return values
}
