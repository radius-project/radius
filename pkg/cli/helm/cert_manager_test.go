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

type fakeCertManagerCluster struct {
	crd        bool
	controller *appsv1.Deployment
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
		name    string
		stored  map[string]any
		current map[string]any
		want    authz.Mode
		wantErr string
	}{
		{name: "defaults", want: authz.ModeOff},
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

			stored, current := copyValues(tt.stored), copyValues(tt.current)
			got, err := resolveAuthzMode(tt.stored, tt.current)
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
			got, err := resolveAuthzMode(tt.stored, current)
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
	wantValues := buildCertManagerValues(RadiusSystemNamespace)
	radOwned := &releasev1.Release{Name: certManagerReleaseName, Labels: radManagedByLabels(), Info: &releasev1.Info{Status: releasecommon.StatusDeployed}, Chart: &chart.Chart{Metadata: &chart.Metadata{Version: CertManagerChartDefaultVersion}}, Config: wantValues}
	radOwnedFailed := &releasev1.Release{Name: certManagerReleaseName, Labels: radManagedByLabels(), Info: &releasev1.Info{Status: releasecommon.StatusFailed}, Chart: radOwned.Chart}
	// An older rad installed cert-manager with only the CRD values, so its default approver may approve every Issuer.
	radOwnedUnconfigured := &releasev1.Release{Name: certManagerReleaseName, Labels: radManagedByLabels(), Info: &releasev1.Info{Status: releasecommon.StatusDeployed}, Chart: radOwned.Chart, Config: map[string]any{"crds": map[string]any{"enabled": true, "keep": false}}}
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

	tests := []struct {
		name         string
		setArgs      []string
		stored       map[string]any
		upgrade      bool
		cluster      fakeCertManagerCluster
		expect       func(*testing.T, *MockHelmClient)
		wantDetect   bool
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
			name:    "rad-owned release is upgraded",
			stored:  storedDryRun,
			upgrade: true,
			expect: func(t *testing.T, m *MockHelmClient) {
				expectGet(radOwned)(m)
				expectPull(t, m)
				m.EXPECT().RunHelmUpgrade(gomock.Any(), gomock.Any(), wantValues, certManagerReleaseName, CertManagerNamespace, true, true, DefaultInstallTimeout).
					Return(&releasev1.Release{}, nil)
			},
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
			name:    "rad-owned release without approveSignerNames is reconfigured on install",
			setArgs: dryRun,
			cluster: healthy,
			expect: func(t *testing.T, m *MockHelmClient) {
				expectGet(radOwnedUnconfigured)(m)
				expectPull(t, m)
				m.EXPECT().RunHelmUpgrade(gomock.Any(), gomock.Any(), wantValues, certManagerReleaseName, CertManagerNamespace, true, true, DefaultInstallTimeout).
					Return(&releasev1.Release{}, nil)
			},
			wantDetect: true,
		},
		{
			name:    "rad-owned release missing its controller is repaired on install",
			setArgs: dryRun,
			cluster: fakeCertManagerCluster{crd: true},
			expect: func(t *testing.T, m *MockHelmClient) {
				expectGet(radOwned)(m)
				expectPull(t, m)
				m.EXPECT().RunHelmUpgrade(gomock.Any(), gomock.Any(), wantValues, certManagerReleaseName, CertManagerNamespace, true, true, DefaultInstallTimeout).
					Return(&releasev1.Release{}, nil)
			},
			wantDetect: true,
		},
		{
			name:    "failed rad-owned release is repaired on install",
			setArgs: dryRun,
			cluster: healthy,
			expect: func(t *testing.T, m *MockHelmClient) {
				expectGet(radOwnedFailed)(m)
				expectPull(t, m)
				m.EXPECT().RunHelmUpgrade(gomock.Any(), gomock.Any(), wantValues, certManagerReleaseName, CertManagerNamespace, true, true, DefaultInstallTimeout).
					Return(&releasev1.Release{}, nil)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			mockHelmClient := NewMockHelmClient(ctrl)
			tt.expect(t, mockHelmClient)

			detectCalls := 0
			impl := &Impl{
				Helm: mockHelmClient,
				newCertManagerClients: func(kubeContext string) (kubernetes.Interface, apiextclient.Interface, error) {
					detectCalls++
					kubeClient, apiextClient := tt.cluster.clients()
					return kubeClient, apiextClient, nil
				},
			}

			options := NewDefaultClusterOptions()
			options.Radius.SetArgs = tt.setArgs
			err := impl.ensureCertManager(t.Context(), options, "test-context", tt.stored, tt.upgrade)
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

			if tt.wantDetect {
				require.Equal(t, 1, detectCalls)
			} else {
				require.Zero(t, detectCalls)
			}
		})
	}
}

func Test_Helm_InstallRadius_CertManagerTooOld_StopsBeforeRadius(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mockHelmClient := NewMockHelmClient(ctrl)
	mockHelmClient.EXPECT().RunHelmGet(gomock.Any(), certManagerReleaseName).Return(nil, driver.ErrReleaseNotFound)
	mockHelmClient.EXPECT().LoadChart("radius-chart").Return(&chart.Chart{}, nil)

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
				err := impl.ensureCertManager(t.Context(), options, "test-context", nil, upgrade)
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
			err := impl.ensureCertManager(t.Context(), options, "test-context", nil, false)
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
				require.Error(t, checkCertManagerReady(t.Context(), k, "security", RadiusSystemNamespace))
			})
		}
	}
}

func Test_checkCertManagerReady_CAInjector(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{name: "chart defaults", args: []string{"--v=2", "--leader-election-namespace=kube-system"}},
		{name: "injection explicitly enabled", args: []string{"--enable-apiservices-injectable=true", "--enable-certificates-data-source"}},
		{name: "limited to the Radius namespace", args: []string{"--namespace", RadiusSystemNamespace}},
		{name: "APIService injection disabled", args: []string{"--enable-apiservices-injectable=false"}, wantErr: "sets --enable-apiservices-injectable=false"},
		{name: "Certificate data source disabled", args: []string{"--enable-certificates-data-source=0"}, wantErr: "sets --enable-certificates-data-source=0"},
		{name: "limited to another namespace", args: []string{"--namespace=security"}, wantErr: `is limited to namespace "security" by --namespace`},
		{name: "limited to another namespace with a separate value", args: []string{"--namespace", "security"}, wantErr: `is limited to namespace "security" by --namespace`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			k, _ := (fakeCertManagerCluster{controller: certManagerController("security", "v1.21.2", "")}).clients()
			deployments := k.AppsV1().Deployments("security")
			deployment, err := deployments.Get(t.Context(), "cert-manager-cainjector", metav1.GetOptions{})
			require.NoError(t, err)
			deployment.Spec.Template.Spec.Containers[0].Args = tt.args
			_, err = deployments.Update(t.Context(), deployment, metav1.UpdateOptions{})
			require.NoError(t, err)

			err = checkCertManagerReady(t.Context(), k, "security", RadiusSystemNamespace)
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, "cert-manager cainjector Deployment security/cert-manager-cainjector "+tt.wantErr)
			require.ErrorContains(t, err, "inject the CA of UCP's API server certificate into the Radius APIService")
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
			err = impl.ensureCertManager(t.Context(), options, "test-context", nil, version == "v1.22.1")
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
			mockHelmClient.EXPECT().RunHelmGet(gomock.Any(), csiDriverSPIFFEReleaseName).Return(nil, driver.ErrReleaseNotFound)
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
	client.EXPECT().RunHelmGet(gomock.Any(), csiDriverSPIFFEReleaseName).Return(nil, driver.ErrReleaseNotFound)
	client.EXPECT().RunHelmGet(gomock.Any(), certManagerReleaseName).Return(nil, errors.New("release storage unavailable"))
	var logs []string
	impl := &Impl{
		Helm:                        client,
		removeDefaultContourGateway: (&defaultContourGatewayCalls{}).remove,
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
