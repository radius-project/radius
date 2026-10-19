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
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	helm "helm.sh/helm/v4/pkg/action"
	chart "helm.sh/helm/v4/pkg/chart/v2"
	"helm.sh/helm/v4/pkg/release"
	releasev1 "helm.sh/helm/v4/pkg/release/v1"
	"helm.sh/helm/v4/pkg/storage/driver"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apiextfake "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/fake"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	k8sfake "k8s.io/client-go/kubernetes/fake"

	"github.com/radius-project/radius/pkg/authz"
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
	}
	apiextObjects := []runtime.Object{}
	if c.crd {
		apiextObjects = append(apiextObjects, certManagerCRD())
	}

	return k8sfake.NewClientset(kubeObjects...), apiextfake.NewClientset(apiextObjects...)
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := resolveAuthzMode(tt.stored, tt.current)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
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
	radOwned := &releasev1.Release{Name: certManagerReleaseName, Labels: radManagedByLabels()}
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
	wantValues := map[string]any{"crds": map[string]any{"enabled": true}}

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
			expect: func(t *testing.T, m *MockHelmClient) {
				expectGet(radOwned)(m)
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

	impl := &Impl{
		Helm: mockHelmClient,
		newCertManagerClients: func(string) (kubernetes.Interface, apiextclient.Interface, error) {
			kubeClient, apiextClient := fakeCertManagerCluster{crd: true, controller: certManagerController("cert-manager", "v1.18.0", "")}.clients()
			return kubeClient, apiextClient, nil
		},
	}

	options := NewDefaultClusterOptions()
	options.Radius.SetArgs = []string{"global.rbac.dryRun=true"}
	err := impl.InstallRadius(t.Context(), options, "test-context")
	require.ErrorContains(t, err, "v"+CertManagerMinimumVersion)
}

func Test_uninstallCertManager(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		release       *releasev1.Release
		getErr        error
		wantUninstall bool
		wantErr       bool
	}{
		{name: "not installed", getErr: driver.ErrReleaseNotFound},
		{name: "user-owned is kept", release: &releasev1.Release{Name: certManagerReleaseName}},
		{name: "rad-owned is removed", release: &releasev1.Release{Name: certManagerReleaseName, Labels: radManagedByLabels()}, wantUninstall: true},
		{name: "get error", getErr: errors.New("boom"), wantErr: true},
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

			impl := &Impl{Helm: mockHelmClient}
			err := impl.uninstallCertManager(NewDefaultClusterOptions(), "test-context")
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}
