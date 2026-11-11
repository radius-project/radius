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
	"sync/atomic"
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
	authorizationv1 "k8s.io/api/authorization/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apiextclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
	"sigs.k8s.io/yaml"
)

// expectCSIDriverSPIFFEPull expects loadChart to pull the pinned csi-driver-spiffe chart. The caller expects LoadChart.
func expectCSIDriverSPIFFEPull(t *testing.T, m *MockHelmClient) {
	m.EXPECT().RunHelmPull(gomock.Any(), csiDriverSPIFFEReleaseName).
		DoAndReturn(func(pullopts []helm.PullOpt, chartRef string) (string, error) {
			pull := helm.NewPull(pullopts...)
			require.Equal(t, csiDriverSPIFFEHelmRepo, pull.RepoURL)
			require.Equal(t, CSIDriverSPIFFEChartDefaultVersion, pull.Version)
			require.NoError(t, os.WriteFile(filepath.Join(pull.DestDir, "Chart.yaml"), []byte("name: "+csiDriverSPIFFEReleaseName+"\nversion: "+CSIDriverSPIFFEChartDefaultVersion), 0644))
			return "Pulled", nil
		})
}

func csiDriverSPIFFEDaemonSet(namespace, release string) *appsv1.DaemonSet {
	return &appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{
		Name:      release + "-driver",
		Namespace: namespace,
		Labels: map[string]string{
			"app.kubernetes.io/name":     "cert-manager-csi-driver-spiffe",
			"app.kubernetes.io/instance": release,
		},
	}}
}

// renamedCSIDriverSPIFFEDaemonSet returns the DaemonSet of a csi-driver-spiffe installed with nameOverride and custom
// labels, so only its pod template shows what it is. volumes and args form that template.
func renamedCSIDriverSPIFFEDaemonSet(namespace, name string, volumes []corev1.Volume, args []string) *appsv1.DaemonSet {
	return &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   namespace,
			Labels:      map[string]string{"app.kubernetes.io/name": name, "app.kubernetes.io/instance": "workload-identity"},
			Annotations: map[string]string{"meta.helm.sh/release-name": "workload-identity", "meta.helm.sh/release-namespace": namespace},
		},
		Spec: appsv1.DaemonSetSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
			Volumes:    volumes,
			Containers: []corev1.Container{{Name: "node-driver-registrar", Args: args}},
		}}},
	}
}

func hostPathVolume(path string) []corev1.Volume {
	return []corev1.Volume{{Name: "plugin-dir", VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: path}}}}
}

func csiDriver(name string, annotations map[string]string) *storagev1.CSIDriver {
	return &storagev1.CSIDriver{ObjectMeta: metav1.ObjectMeta{Name: name, Annotations: annotations}}
}

func spiffePod(namespace, name, driver string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec: corev1.PodSpec{Volumes: []corev1.Volume{{
			Name:         "spiffe",
			VolumeSource: corev1.VolumeSource{CSI: &corev1.CSIVolumeSource{Driver: driver}},
		}}},
	}
}

// sarClient returns a fake client whose SubjectAccessReviews allow the signer names in allowed and records the
// reviews in reviews.
func sarClient(allowed map[string]bool, reviews *[]authorizationv1.SubjectAccessReviewSpec, objects ...runtime.Object) *k8sfake.Clientset {
	client := withValidatingAdmissionPolicyAPI(k8sfake.NewClientset(objects...))
	client.PrependReactor("create", "subjectaccessreviews", func(action k8stesting.Action) (bool, runtime.Object, error) {
		review := action.(k8stesting.CreateAction).GetObject().(*authorizationv1.SubjectAccessReview).DeepCopy()
		if reviews != nil {
			*reviews = append(*reviews, review.Spec)
		}
		review.Status.Allowed = allowed[review.Spec.ResourceAttributes.Name]
		return true, review, nil
	})
	return client
}

// withValidatingAdmissionPolicyAPI makes client's discovery serve ValidatingAdmissionPolicy, as Kubernetes 1.30 and
// later do.
func withValidatingAdmissionPolicyAPI(client *k8sfake.Clientset) *k8sfake.Clientset {
	client.Resources = append(client.Resources, &metav1.APIResourceList{
		GroupVersion: "admissionregistration.k8s.io/v1",
		APIResources: []metav1.APIResource{{Name: "validatingadmissionpolicies", Kind: "ValidatingAdmissionPolicy"}},
	})
	return client
}

func Test_resolveSPIFFESettings(t *testing.T) {
	t.Parallel()

	spiffe := func(values map[string]any) map[string]any {
		return map[string]any{"global": map[string]any{"rbac": map[string]any{"spiffe": values}}}
	}
	defaults := spiffeSettings{TrustDomain: SPIFFETrustDomainDefault, CSIDriverName: SPIFFECSIDriverNameDefault}

	tests := []struct {
		name    string
		stored  map[string]any
		current map[string]any
		want    spiffeSettings
	}{
		{name: "defaults", want: defaults},
		{name: "rbac without spiffe", current: map[string]any{"global": map[string]any{"rbac": map[string]any{"dryRun": true}}}, want: defaults},
		{name: "stored values", stored: spiffe(map[string]any{"trustDomain": "stored.example", "csiDriverName": "stored.csi"}), want: spiffeSettings{TrustDomain: "stored.example", CSIDriverName: "stored.csi"}},
		{
			name:    "current values override stored values",
			stored:  spiffe(map[string]any{"trustDomain": "stored.example", "csiDriverName": "stored.csi"}),
			current: spiffe(map[string]any{"trustDomain": "current.example"}),
			want:    spiffeSettings{TrustDomain: "current.example", CSIDriverName: "stored.csi"},
		},
		{name: "empty strings use defaults", current: spiffe(map[string]any{"trustDomain": "", "csiDriverName": ""}), want: defaults},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, resolveSPIFFESettings(tt.stored, tt.current))
		})
	}
}

// The CLI configures csi-driver-spiffe with the same trust domain and driver name that the Radius chart renders by
// default, so the defaults must match.
func Test_SPIFFEDefaultsMatchChartValues(t *testing.T) {
	t.Parallel()

	content, err := os.ReadFile("../../../deploy/Chart/values.yaml")
	require.NoError(t, err)
	var values struct {
		Global struct {
			RBAC struct {
				SPIFFE struct {
					TrustDomain   string `json:"trustDomain"`
					CSIDriverName string `json:"csiDriverName"`
				} `json:"spiffe"`
			} `json:"rbac"`
		} `json:"global"`
	}
	require.NoError(t, yaml.Unmarshal(content, &values))
	require.Equal(t, SPIFFETrustDomainDefault, values.Global.RBAC.SPIFFE.TrustDomain)
	require.Equal(t, SPIFFECSIDriverNameDefault, values.Global.RBAC.SPIFFE.CSIDriverName)
}

func Test_buildCSIDriverSPIFFEValues(t *testing.T) {
	t.Parallel()

	got := buildCSIDriverSPIFFEValues("custom-ns", spiffeSettings{TrustDomain: "example.org", CSIDriverName: "spiffe.example.org"})
	require.Equal(t, map[string]any{
		"app": map[string]any{
			"trustDomain":                "example.org",
			"name":                       "spiffe.example.org",
			"certificateRequestDuration": "24h",
			"issuer":                     map[string]any{"name": "radius-ca-issuer", "kind": "Issuer", "group": "cert-manager.io"},
			"approver": map[string]any{
				"signerName":           "issuers.cert-manager.io/custom-ns.radius-ca-issuer",
				"autoApproveNonSPIFFE": false,
			},
		},
	}, got)
}

func Test_certManagerApproverDisabled(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
		want bool
	}{
		{name: "no args"},
		{name: "default controllers", args: []string{"--controllers=*"}},
		{name: "approver disabled", args: []string{"--v=2", "--controllers=*,-certificaterequests-approver"}, want: true},
		{name: "approver disabled with separate value", args: []string{"--controllers", "*,-certificaterequests-approver"}, want: true},
		{name: "other controller disabled", args: []string{"--controllers=*,-certificaterequests-issuer-acme"}},
		{name: "trailing --controllers flag", args: []string{"--controllers"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			deployment := certManagerController(CertManagerNamespace, "", "")
			deployment.Spec.Template.Spec.Containers[0].Args = tt.args
			require.Equal(t, tt.want, certManagerApproverDisabled(deployment))
		})
	}
}

func Test_checkCertManagerDefaultApprover(t *testing.T) {
	t.Parallel()

	const exact = "issuers.cert-manager.io/radius-system.radius-ca-issuer"
	const wildcard = "issuers.cert-manager.io/*"

	controller := func(mutate func(*appsv1.Deployment)) *appsv1.Deployment {
		deployment := certManagerController("security", "v1.21.2", "")
		deployment.Labels["app.kubernetes.io/instance"] = "cm"
		deployment.Spec.Template.Spec.ServiceAccountName = "cert-manager"
		if mutate != nil {
			mutate(deployment)
		}
		return deployment
	}

	tests := []struct {
		name        string
		objects     []runtime.Object
		allowed     map[string]bool
		wantReviews []string
		wantErr     string
		wantCommand string
	}{
		{name: "no cert-manager"},
		{
			name:        "approver cannot approve the Radius issuer",
			objects:     []runtime.Object{controller(nil)},
			wantReviews: []string{wildcard, exact},
		},
		{
			name:        "wildcard approval",
			objects:     []runtime.Object{controller(nil)},
			allowed:     map[string]bool{wildcard: true},
			wantReviews: []string{wildcard},
			wantErr:     "cert-manager's default approver (system:serviceaccount:security:cert-manager) can approve CertificateRequests for the Radius CA issuer signer " + exact,
			wantCommand: "helm --kube-context \"test-context\" -n security upgrade cm cert-manager --repo https://charts.jetstack.io --version v1.21.2 --reuse-values --set \"approveSignerNames={issuers.cert-manager.io/radius-system.radius-selfsigned-bootstrap,issuers.cert-manager.io/radius-system.radius-mtls-issuer,clusterissuers.cert-manager.io/*}\"",
		},
		{
			name: "exact approval without version or release labels",
			objects: []runtime.Object{controller(func(d *appsv1.Deployment) {
				d.Labels = map[string]string{"app.kubernetes.io/name": "cert-manager", "app.kubernetes.io/component": "controller"}
			})},
			allowed:     map[string]bool{exact: true},
			wantReviews: []string{wildcard, exact},
			wantCommand: "helm --kube-context \"test-context\" -n security upgrade cert-manager cert-manager --repo https://charts.jetstack.io --reuse-values --set \"approveSignerNames={issuers.cert-manager.io/radius-system.radius-selfsigned-bootstrap,issuers.cert-manager.io/radius-system.radius-mtls-issuer,clusterissuers.cert-manager.io/*}\"",
		},
		{
			name: "disabled approver is not reviewed",
			objects: []runtime.Object{controller(func(d *appsv1.Deployment) {
				d.Spec.Template.Spec.Containers[0].Args = []string{"--controllers=*,-certificaterequests-approver"}
			})},
			allowed: map[string]bool{wildcard: true, exact: true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var reviews []authorizationv1.SubjectAccessReviewSpec
			client := sarClient(tt.allowed, &reviews, tt.objects...)

			err := checkCertManagerDefaultApprover(t.Context(), client, RadiusSystemNamespace, "test-context")
			if tt.wantCommand == "" {
				require.NoError(t, err)
			} else {
				var conflict *defaultApproverConflictError
				require.ErrorAs(t, err, &conflict)
				require.Equal(t, tt.wantCommand, conflict.Command)
				require.Contains(t, err.Error(), tt.wantCommand)
				require.Contains(t, err.Error(), tt.wantErr)
			}

			var names []string
			for _, review := range reviews {
				require.Equal(t, "system:serviceaccount:security:cert-manager", review.User)
				require.ElementsMatch(t, []string{"system:serviceaccounts", "system:serviceaccounts:security", "system:authenticated"}, review.Groups)
				require.Equal(t, authorizationv1.ResourceAttributes{Group: "cert-manager.io", Resource: "signers", Verb: "approve", Name: review.ResourceAttributes.Name}, *review.ResourceAttributes)
				names = append(names, review.ResourceAttributes.Name)
			}
			require.Equal(t, tt.wantReviews, names)
		})
	}
}

func Test_checkCertManagerDefaultApprover_DefaultServiceAccount(t *testing.T) {
	t.Parallel()
	var reviews []authorizationv1.SubjectAccessReviewSpec
	client := sarClient(nil, &reviews, certManagerController(CertManagerNamespace, "", ""))
	require.NoError(t, checkCertManagerDefaultApprover(t.Context(), client, RadiusSystemNamespace, ""))
	require.NotEmpty(t, reviews)
	require.Equal(t, "system:serviceaccount:cert-manager:default", reviews[0].User)
}

func Test_checkCertManagerDefaultApprover_ReviewError(t *testing.T) {
	t.Parallel()
	client := k8sfake.NewClientset(certManagerController(CertManagerNamespace, "", ""))
	client.PrependReactor("create", "subjectaccessreviews", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("forbidden")
	})
	err := checkCertManagerDefaultApprover(t.Context(), client, RadiusSystemNamespace, "")
	require.ErrorContains(t, err, "could not check cert-manager's default approver: forbidden")
	var conflict *defaultApproverConflictError
	require.False(t, errors.As(err, &conflict))
}

func Test_checkForeignCSIDriverSPIFFE(t *testing.T) {
	t.Parallel()

	options := NewDefaultClusterOptions().CSIDriverSPIFFE.ChartOptions
	ownAnnotations := map[string]string{"meta.helm.sh/release-name": csiDriverSPIFFEReleaseName, "meta.helm.sh/release-namespace": RadiusSystemNamespace}
	tests := []struct {
		name    string
		objects []runtime.Object
		listErr error
		getErr  error
		wantErr string
	}{
		{name: "none"},
		{name: "Radius's own release", objects: []runtime.Object{
			csiDriverSPIFFEDaemonSet(RadiusSystemNamespace, csiDriverSPIFFEReleaseName),
			csiDriver(SPIFFECSIDriverNameDefault, ownAnnotations),
		}},
		{
			name: "Radius's own release found by annotations",
			objects: []runtime.Object{func() *appsv1.DaemonSet {
				ds := renamedCSIDriverSPIFFEDaemonSet(RadiusSystemNamespace, "renamed", hostPathVolume(csiDriverSPIFFEPluginDir), nil)
				ds.Annotations = ownAnnotations
				return ds
			}()},
		},
		{
			name:    "cert-manager csi-driver uses another plugin path",
			objects: []runtime.Object{renamedCSIDriverSPIFFEDaemonSet("cert-manager", "cert-manager-csi-driver", hostPathVolume("/var/lib/kubelet/plugins/cert-manager-csi-driver"), []string{"--kubelet-registration-path=/var/lib/kubelet/plugins/cert-manager-csi-driver/csi.sock"})},
		},
		{
			name:    "plugin path prefix is not a match",
			objects: []runtime.Object{renamedCSIDriverSPIFFEDaemonSet("other", "other", hostPathVolume(csiDriverSPIFFEPluginDir+"-2"), []string{"--kubelet-registration-path=" + csiDriverSPIFFEPluginDir + "-2/csi.sock"})},
		},
		{
			name:    "another namespace",
			objects: []runtime.Object{csiDriverSPIFFEDaemonSet("cert-manager", "spiffe")},
			wantErr: "another csi-driver-spiffe (DaemonSet cert-manager/spiffe-driver) runs on it. Radius installs its own csi-driver-spiffe, limited to the Radius issuer, and a cluster can run only one because each registers the same kubelet plugin path (/var/lib/kubelet/plugins/cert-manager-csi-driver-spiffe). Remove the other installation, for example with helm --kube-context \"test-context\" -n cert-manager uninstall spiffe, and then run the command again.",
		},
		{
			name:    "another release in the Radius namespace",
			objects: []runtime.Object{csiDriverSPIFFEDaemonSet(RadiusSystemNamespace, "other")},
			wantErr: "uninstall other",
		},
		{
			name:    "renamed installation found by its plugin hostPath",
			objects: []runtime.Object{renamedCSIDriverSPIFFEDaemonSet("identity", "workload-identity-node", hostPathVolume(csiDriverSPIFFEPluginDir+"/"), nil)},
			wantErr: "another csi-driver-spiffe (DaemonSet identity/workload-identity-node) runs on it.",
		},
		{
			name:    "renamed installation found by its registration path",
			objects: []runtime.Object{renamedCSIDriverSPIFFEDaemonSet("identity", "workload-identity-node", nil, []string{"--csi-address=/plugin/csi.sock", "--kubelet-registration-path=" + csiDriverSPIFFEPluginDir + "/csi.sock"})},
			wantErr: "helm --kube-context \"test-context\" -n identity uninstall workload-identity",
		},
		{
			name:    "renamed installation found by a separate registration path argument",
			objects: []runtime.Object{renamedCSIDriverSPIFFEDaemonSet("identity", "workload-identity-node", nil, []string{"--kubelet-registration-path", csiDriverSPIFFEPluginDir + "/csi.sock"})},
			wantErr: "DaemonSet identity/workload-identity-node",
		},
		{
			name:    "CSIDriver name taken by another release",
			objects: []runtime.Object{csiDriver(SPIFFECSIDriverNameDefault, map[string]string{"meta.helm.sh/release-name": "other", "meta.helm.sh/release-namespace": "other"})},
			wantErr: "CSIDriver \"spiffe.csi.radapp.io\" already exists and Radius's csi-driver-spiffe release did not create it. Remove it, for example with kubectl --context \"test-context\" delete csidriver spiffe.csi.radapp.io, or set global.rbac.spiffe.csiDriverName to another name",
		},
		{
			name:    "CSIDriver without Helm annotations",
			objects: []runtime.Object{csiDriver(SPIFFECSIDriverNameDefault, nil)},
			wantErr: "CSIDriver \"spiffe.csi.radapp.io\" already exists",
		},
		{name: "unrelated CSIDriver", objects: []runtime.Object{csiDriver("csi.cert-manager.io", nil)}},
		{name: "list error", listErr: errors.New("boom"), wantErr: "failed to list DaemonSets to find csi-driver-spiffe installations: boom"},
		{name: "CSIDriver get error", getErr: errors.New("boom"), wantErr: "failed to check CSIDriver spiffe.csi.radapp.io: boom"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			client := k8sfake.NewClientset(tt.objects...)
			if tt.listErr != nil {
				client.PrependReactor("list", "daemonsets", func(k8stesting.Action) (bool, runtime.Object, error) {
					return true, nil, tt.listErr
				})
			}
			if tt.getErr != nil {
				client.PrependReactor("get", "csidrivers", func(k8stesting.Action) (bool, runtime.Object, error) {
					return true, nil, tt.getErr
				})
			}
			err := checkForeignCSIDriverSPIFFE(t.Context(), client, options, SPIFFECSIDriverNameDefault, "test-context")
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func Test_planAndApplyCSIDriverSPIFFE(t *testing.T) {
	t.Parallel()

	dryRun := []string{"global.rbac.dryRun=true"}
	wantValues := buildCSIDriverSPIFFEValues(RadiusSystemNamespace, spiffeSettings{TrustDomain: SPIFFETrustDomainDefault, CSIDriverName: SPIFFECSIDriverNameDefault})
	owned := func(status releasecommon.Status, config map[string]any) *releasev1.Release {
		return &releasev1.Release{Name: csiDriverSPIFFEReleaseName, Labels: radManagedByLabels(), Info: &releasev1.Info{Status: status}, Config: config}
	}
	expectGet := func(rel *releasev1.Release) func(*MockHelmClient) {
		return func(m *MockHelmClient) {
			if rel == nil {
				m.EXPECT().RunHelmGet(gomock.Any(), csiDriverSPIFFEReleaseName).Return(nil, driver.ErrReleaseNotFound)
				return
			}
			m.EXPECT().RunHelmGet(gomock.Any(), csiDriverSPIFFEReleaseName).Return(rel, nil)
		}
	}
	expectInstall := func(values map[string]any) func(*testing.T, *MockHelmClient) {
		return func(t *testing.T, m *MockHelmClient) {
			expectGet(nil)(m)
			expectCSIDriverSPIFFEPull(t, m)
			m.EXPECT().LoadChart(gomock.Any()).Return(&chart.Chart{}, nil)
			m.EXPECT().RunHelmInstall(gomock.Any(), gomock.Any(), values, csiDriverSPIFFEReleaseName, RadiusSystemNamespace, true, DefaultInstallTimeout, radManagedByLabels()).
				Return(&releasev1.Release{}, nil)
		}
	}
	expectUpgrade := func(rel *releasev1.Release) func(*testing.T, *MockHelmClient) {
		return func(t *testing.T, m *MockHelmClient) {
			expectGet(rel)(m)
			expectCSIDriverSPIFFEPull(t, m)
			m.EXPECT().LoadChart(gomock.Any()).Return(&chart.Chart{}, nil)
			m.EXPECT().RunHelmUpgrade(gomock.Any(), gomock.Any(), wantValues, csiDriverSPIFFEReleaseName, RadiusSystemNamespace, true, false, DefaultInstallTimeout).
				Return(&releasev1.Release{}, nil)
		}
	}
	approver := certManagerController(CertManagerNamespace, CertManagerChartDefaultVersion, "")
	disabledApprover := certManagerController(CertManagerNamespace, CertManagerChartDefaultVersion, "")
	disabledApprover.Spec.Template.Spec.Containers[0].Args = []string{"--controllers=*,-certificaterequests-approver"}

	tests := []struct {
		name       string
		setArgs    []string
		stored     map[string]any
		upgrade    bool
		objects    []runtime.Object
		allowed    map[string]bool
		expect     func(*testing.T, *MockHelmClient)
		wantClient bool
		noVAPAPI   bool
		wantErr    string
		wantLog    string
		// wantRequesters are the cert-manager controller users returned for the Radius chart.
		wantRequesters []string
	}{
		{name: "mode off makes no calls", expect: func(*testing.T, *MockHelmClient) {}},
		{
			name:       "installs when absent",
			setArgs:    dryRun,
			expect:     expectInstall(wantValues),
			wantClient: true,
		},
		{
			name:    "installs with the chart's SPIFFE values",
			setArgs: []string{"global.rbac.enabled=true", "global.rbac.spiffe.trustDomain=example.org"},
			stored:  map[string]any{"global": map[string]any{"rbac": map[string]any{"spiffe": map[string]any{"csiDriverName": "spiffe.example.org"}}}},
			expect: expectInstall(buildCSIDriverSPIFFEValues(RadiusSystemNamespace,
				spiffeSettings{TrustDomain: "example.org", CSIDriverName: "spiffe.example.org"})),
			wantClient: true,
		},
		{
			name:       "mode from stored values",
			stored:     map[string]any{"global": map[string]any{"rbac": map[string]any{"enabled": true}}},
			expect:     expectInstall(wantValues),
			wantClient: true,
		},
		{
			name:    "user-owned release fails",
			setArgs: dryRun,
			expect: func(_ *testing.T, m *MockHelmClient) {
				expectGet(&releasev1.Release{Name: csiDriverSPIFFEReleaseName})(m)
			},
			wantErr: "Helm release \"cert-manager-csi-driver-spiffe\" in namespace \"radius-system\" was not installed by rad. Radius installs and manages its own csi-driver-spiffe. Remove that release, for example with helm --kube-context \"test-context\" -n radius-system uninstall cert-manager-csi-driver-spiffe",
		},
		{
			name:    "renamed foreign installation fails before installing",
			setArgs: dryRun,
			objects: []runtime.Object{renamedCSIDriverSPIFFEDaemonSet("identity", "workload-identity-node", hostPathVolume(csiDriverSPIFFEPluginDir), nil)},
			expect: func(_ *testing.T, m *MockHelmClient) {
				expectGet(nil)(m)
			},
			wantClient: true,
			wantErr:    "another csi-driver-spiffe (DaemonSet identity/workload-identity-node)",
		},
		{
			name:    "foreign installation fails before installing",
			setArgs: dryRun,
			objects: []runtime.Object{csiDriverSPIFFEDaemonSet("cert-manager", "csi-driver-spiffe")},
			expect: func(_ *testing.T, m *MockHelmClient) {
				expectGet(nil)(m)
			},
			wantClient: true,
			wantErr:    "another csi-driver-spiffe (DaemonSet cert-manager/csi-driver-spiffe-driver)",
		},
		{
			name:    "default approver conflict fails before installing",
			setArgs: dryRun,
			objects: []runtime.Object{approver},
			allowed: map[string]bool{"issuers.cert-manager.io/*": true},
			expect: func(_ *testing.T, m *MockHelmClient) {
				expectGet(nil)(m)
			},
			wantClient: true,
			wantErr:    "Radius stopped before it installed or upgraded csi-driver-spiffe and Radius because cert-manager's default approver (system:serviceaccount:cert-manager:default) can approve CertificateRequests for the Radius CA issuer signer issuers.cert-manager.io/radius-system.radius-ca-issuer. Only Radius's csi-driver-spiffe approver may approve them. Limit the default approver to other signers, for example with helm --kube-context \"test-context\" -n cert-manager upgrade cert-manager cert-manager --repo https://charts.jetstack.io --version " + CertManagerChartDefaultVersion + " " + "--reuse-values --set \"approveSignerNames={issuers.cert-manager.io/radius-system.radius-selfsigned-bootstrap,issuers.cert-manager.io/radius-system.radius-mtls-issuer,clusterissuers.cert-manager.io/*}\"" + ".",
		},
		{
			name:    "default approver conflict on upgrade fails before upgrading",
			setArgs: dryRun,
			upgrade: true,
			objects: []runtime.Object{approver},
			allowed: map[string]bool{radiusCAIssuerSignerName(RadiusSystemNamespace): true},
			expect: func(_ *testing.T, m *MockHelmClient) {
				expectGet(owned(releasecommon.StatusDeployed, wantValues))(m)
			},
			wantClient: true,
			wantErr:    "Limit the default approver to other signers",
		},
		{
			name:     "cluster without ValidatingAdmissionPolicy fails before installing",
			setArgs:  dryRun,
			noVAPAPI: true,
			expect: func(_ *testing.T, m *MockHelmClient) {
				expectGet(nil)(m)
			},
			wantClient: true,
			wantErr:    "Radius stopped before changing the cluster because the cluster does not serve ValidatingAdmissionPolicy (admissionregistration.k8s.io/v1).",
		},
		{
			name:    "returns each cert-manager controller's user",
			setArgs: dryRun,
			objects: []runtime.Object{
				approver,
				certManagerControllerWithServiceAccount(certManagerController("security", CertManagerChartDefaultVersion, ""), "cert-manager"),
				certManagerControllerWithServiceAccount(certManagerController("other", CertManagerChartDefaultVersion, ""), "controller"),
			},
			allowed: map[string]bool{
				issuerSignerName(RadiusSystemNamespace, RadiusBootstrapIssuerName): true,
				issuerSignerName(RadiusSystemNamespace, RadiusMTLSIssuerName):      true,
			},
			expect:     expectInstall(wantValues),
			wantClient: true,
			wantRequesters: []string{
				"system:serviceaccount:cert-manager:default",
				"system:serviceaccount:other:controller",
				"system:serviceaccount:security:cert-manager",
			},
		},
		{
			name:    "approver limited to the Radius bootstrap and mTLS issuers installs quietly",
			setArgs: dryRun,
			objects: []runtime.Object{approver},
			allowed: map[string]bool{
				issuerSignerName(RadiusSystemNamespace, RadiusBootstrapIssuerName): true,
				issuerSignerName(RadiusSystemNamespace, RadiusMTLSIssuerName):      true,
			},
			expect:         expectInstall(wantValues),
			wantClient:     true,
			wantRequesters: []string{"system:serviceaccount:cert-manager:default"},
		},
		{
			name:           "approver that cannot approve the mTLS issuer is a warning",
			setArgs:        dryRun,
			objects:        []runtime.Object{approver},
			allowed:        map[string]bool{issuerSignerName(RadiusSystemNamespace, RadiusBootstrapIssuerName): true},
			expect:         expectInstall(wantValues),
			wantClient:     true,
			wantLog:        "Warning: cert-manager's default approver cannot approve CertificateRequests for issuers.cert-manager.io/radius-system.radius-mtls-issuer. The Radius CA and the component certificates are issued only if another approver approves them.",
			wantRequesters: []string{"system:serviceaccount:cert-manager:default"},
		},
		{
			name:           "disabled approver is a warning",
			setArgs:        dryRun,
			objects:        []runtime.Object{disabledApprover},
			expect:         expectInstall(wantValues),
			wantClient:     true,
			wantLog:        "Warning: cert-manager's default approver cannot approve CertificateRequests for issuers.cert-manager.io/radius-system.radius-selfsigned-bootstrap, issuers.cert-manager.io/radius-system.radius-mtls-issuer. The Radius CA and the component certificates are issued only if another approver approves them.",
			wantRequesters: []string{"system:serviceaccount:cert-manager:default"},
		},
		{
			name:       "unchanged deployed release is reused",
			setArgs:    dryRun,
			expect:     func(_ *testing.T, m *MockHelmClient) { expectGet(owned(releasecommon.StatusDeployed, wantValues))(m) },
			wantClient: true,
		},
		{
			name:       "deployed release is upgraded on upgrade",
			setArgs:    dryRun,
			upgrade:    true,
			expect:     expectUpgrade(owned(releasecommon.StatusDeployed, wantValues)),
			wantClient: true,
		},
		{
			name:       "changed values are upgraded",
			setArgs:    dryRun,
			expect:     expectUpgrade(owned(releasecommon.StatusDeployed, map[string]any{"app": map[string]any{"trustDomain": "old.example"}})),
			wantClient: true,
		},
		{
			name:       "failed release is repaired",
			setArgs:    dryRun,
			expect:     expectUpgrade(owned(releasecommon.StatusFailed, wantValues)),
			wantClient: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			mockHelmClient := NewMockHelmClient(ctrl)
			tt.expect(t, mockHelmClient)

			clientCalls := 0
			var logs []string
			impl := &Impl{
				Helm: mockHelmClient,
				newCSIDriverSPIFFEClient: func(kubeContext string) (kubernetes.Interface, error) {
					require.Equal(t, "test-context", kubeContext)
					clientCalls++
					client := sarClient(tt.allowed, nil, tt.objects...)
					if tt.noVAPAPI {
						client.Resources = nil
					}
					return client, nil
				},
				logger: func(format string, v ...any) { logs = append(logs, fmt.Sprintf(format, v...)) },
			}

			options := NewDefaultClusterOptions()
			options.Radius.SetArgs = tt.setArgs
			var requesters []string
			plan, err := impl.planCSIDriverSPIFFE(t.Context(), options, "test-context", tt.stored)
			if err == nil {
				requesters, err = impl.applyCSIDriverSPIFFE(t.Context(), plan, "test-context", tt.upgrade)
			}
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tt.wantRequesters, requesters)
			if tt.wantClient {
				require.Equal(t, 1, clientCalls)
			} else {
				require.Zero(t, clientCalls)
			}
			if tt.wantLog == "" {
				require.Empty(t, logs)
			} else {
				require.Equal(t, []string{tt.wantLog}, logs)
			}
		})
	}
}

func Test_planCSIDriverSPIFFE_ClientError(t *testing.T) {
	t.Parallel()
	mockHelmClient := NewMockHelmClient(gomock.NewController(t))
	mockHelmClient.EXPECT().RunHelmGet(gomock.Any(), csiDriverSPIFFEReleaseName).Return(nil, driver.ErrReleaseNotFound)
	impl := &Impl{
		Helm:                     mockHelmClient,
		newCSIDriverSPIFFEClient: func(string) (kubernetes.Interface, error) { return nil, errors.New("no kubeconfig") },
	}
	options := NewDefaultClusterOptions()
	options.Radius.SetArgs = []string{"global.rbac.dryRun=true"}
	plan, err := impl.planCSIDriverSPIFFE(t.Context(), options, "test-context", nil)
	require.EqualError(t, err, "failed to create Kubernetes clients: no kubeconfig")
	require.Nil(t, plan)
}

func certManagerControllerWithServiceAccount(controller *appsv1.Deployment, serviceAccount string) *appsv1.Deployment {
	controller.Spec.Template.Spec.ServiceAccountName = serviceAccount
	return controller
}

func Test_uninstallCSIDriverSPIFFE(t *testing.T) {
	t.Parallel()

	owned := &releasev1.Release{Name: csiDriverSPIFFEReleaseName, Labels: radManagedByLabels()}
	tests := []struct {
		name          string
		release       *releasev1.Release
		getErr        error
		objects       []runtime.Object
		clientErr     error
		listErr       error
		wantUninstall bool
		wantKept      bool
		wantLog       string
	}{
		{name: "not installed", getErr: driver.ErrReleaseNotFound},
		{name: "user-owned is kept", release: &releasev1.Release{Name: csiDriverSPIFFEReleaseName}},
		{name: "rad-owned is removed", release: owned, wantUninstall: true},
		{
			name:          "pods of other drivers do not block",
			release:       owned,
			objects:       []runtime.Object{spiffePod("default", "app", "csi.cert-manager.io"), &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "plain", Namespace: "default"}}},
			wantUninstall: true,
		},
		{
			name:     "pods that still use the driver keep it",
			release:  owned,
			objects:  []runtime.Object{spiffePod(RadiusSystemNamespace, "ucp-0", SPIFFECSIDriverNameDefault)},
			wantKept: true,
			wantLog:  "Keeping csi-driver-spiffe and cert-manager: 1 Pods still mount volumes from CSI driver spiffe.csi.radapp.io (e.g. radius-system/ucp-0). Rerun the uninstall after they are gone.",
		},
		{
			name: "configured driver name is used",
			release: &releasev1.Release{Name: csiDriverSPIFFEReleaseName, Labels: radManagedByLabels(),
				Config: map[string]any{"app": map[string]any{"name": "spiffe.example.org"}}},
			objects:  []runtime.Object{spiffePod(RadiusSystemNamespace, "ucp-0", "spiffe.example.org")},
			wantKept: true,
			wantLog:  "Keeping csi-driver-spiffe and cert-manager: 1 Pods still mount volumes from CSI driver spiffe.example.org",
		},
		{name: "get error keeps the release", getErr: errors.New("boom"), wantKept: true, wantLog: "Keeping csi-driver-spiffe and cert-manager: could not check whether rad installed it: failed to run helm get for release cert-manager-csi-driver-spiffe in namespace radius-system (context: test-context): boom"},
		{name: "client error keeps the release", release: owned, clientErr: errors.New("no kubeconfig"), wantKept: true, wantLog: "Keeping csi-driver-spiffe and cert-manager: could not check for Pods that use csi-driver-spiffe: no kubeconfig"},
		{name: "list error keeps the release", release: owned, listErr: errors.New("forbidden"), wantKept: true, wantLog: "Keeping csi-driver-spiffe and cert-manager: could not check for Pods that use csi-driver-spiffe: forbidden"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			mockHelmClient := NewMockHelmClient(gomock.NewController(t))
			mockHelmClient.EXPECT().RunHelmGet(gomock.Any(), csiDriverSPIFFEReleaseName).Return(tt.release, tt.getErr)
			if tt.wantUninstall {
				mockHelmClient.EXPECT().RunHelmUninstall(gomock.Any(), csiDriverSPIFFEReleaseName, RadiusSystemNamespace, true).
					Return(&release.UninstallReleaseResponse{}, nil)
			}
			var logs []string
			impl := &Impl{
				Helm:   mockHelmClient,
				logger: func(format string, v ...any) { logs = append(logs, fmt.Sprintf(format, v...)) },
				newCSIDriverSPIFFEClient: func(kubeContext string) (kubernetes.Interface, error) {
					require.Equal(t, "test-context", kubeContext)
					if tt.clientErr != nil {
						return nil, tt.clientErr
					}
					client := k8sfake.NewClientset(tt.objects...)
					if tt.listErr != nil {
						client.PrependReactor("list", "pods", func(k8stesting.Action) (bool, runtime.Object, error) {
							return true, nil, tt.listErr
						})
					}
					return client, nil
				},
				csiDriverSPIFFEPodWaitTimeout:  50 * time.Millisecond,
				csiDriverSPIFFEPodPollInterval: 5 * time.Millisecond,
			}
			err := impl.uninstallCSIDriverSPIFFE(t.Context(), NewDefaultClusterOptions(), "test-context")
			if tt.wantKept {
				var kept *CSIDriverSPIFFEKeptError
				require.ErrorAs(t, err, &kept)
				require.Equal(t, RadiusSystemNamespace, kept.Namespace)
				require.Equal(t, len(tt.objects) > 0, len(kept.Pods) > 0)
				require.Equal(t, len(tt.objects) == 0, kept.Reason != "")
			} else {
				require.NoError(t, err)
			}
			if tt.wantLog == "" {
				require.Empty(t, logs)
				return
			}
			require.Len(t, logs, 1)
			require.Contains(t, logs[0], tt.wantLog)
		})
	}
}

// The driver must stay until the Radius Pods that mount its volumes are gone, because the kubelet calls it to unmount
// them. Helm's uninstall of Radius returns before those Pods finish terminating.
func Test_uninstallCSIDriverSPIFFE_WaitsForPods(t *testing.T) {
	t.Parallel()

	client := k8sfake.NewClientset(
		spiffePod(RadiusSystemNamespace, "ucp-0", SPIFFECSIDriverNameDefault),
		spiffePod(RadiusSystemNamespace, "controller-0", SPIFFECSIDriverNameDefault),
	)
	// Each Pod terminates after a few polls, like Pods that the garbage collector and kubelet remove after Helm returns.
	var lists atomic.Int32
	client.PrependReactor("list", "pods", func(k8stesting.Action) (bool, runtime.Object, error) {
		switch lists.Add(1) {
		case 3:
			require.NoError(t, client.Tracker().Delete(corev1.SchemeGroupVersion.WithResource("pods"), RadiusSystemNamespace, "ucp-0"))
		case 5:
			require.NoError(t, client.Tracker().Delete(corev1.SchemeGroupVersion.WithResource("pods"), RadiusSystemNamespace, "controller-0"))
		}
		return false, nil, nil
	})

	mockHelmClient := NewMockHelmClient(gomock.NewController(t))
	mockHelmClient.EXPECT().RunHelmGet(gomock.Any(), csiDriverSPIFFEReleaseName).
		Return(&releasev1.Release{Name: csiDriverSPIFFEReleaseName, Labels: radManagedByLabels()}, nil)
	mockHelmClient.EXPECT().RunHelmUninstall(gomock.Any(), csiDriverSPIFFEReleaseName, RadiusSystemNamespace, true).
		DoAndReturn(func(*helm.Configuration, string, string, bool) (*release.UninstallReleaseResponse, error) {
			pods, err := client.CoreV1().Pods(metav1.NamespaceAll).List(t.Context(), metav1.ListOptions{})
			require.NoError(t, err)
			require.Empty(t, pods.Items, "csi-driver-spiffe was removed while Pods still used it")
			return &release.UninstallReleaseResponse{}, nil
		})

	var logs []string
	impl := &Impl{
		Helm:                           mockHelmClient,
		logger:                         func(format string, v ...any) { logs = append(logs, fmt.Sprintf(format, v...)) },
		newCSIDriverSPIFFEClient:       func(string) (kubernetes.Interface, error) { return client, nil },
		csiDriverSPIFFEPodWaitTimeout:  10 * time.Second,
		csiDriverSPIFFEPodPollInterval: time.Millisecond,
	}
	require.NoError(t, impl.uninstallCSIDriverSPIFFE(t.Context(), NewDefaultClusterOptions(), "test-context"))
	require.Empty(t, logs)
	require.GreaterOrEqual(t, lists.Load(), int32(5))
}

func Test_CSIDriverSPIFFEInstalledByRad(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		release *releasev1.Release
		getErr  error
		want    bool
		wantErr bool
	}{
		{name: "not installed", getErr: driver.ErrReleaseNotFound},
		{name: "user-owned", release: &releasev1.Release{Name: csiDriverSPIFFEReleaseName}},
		{name: "rad-owned", release: &releasev1.Release{Name: csiDriverSPIFFEReleaseName, Labels: radManagedByLabels()}, want: true},
		{name: "error", getErr: errors.New("boom"), wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			mockHelmClient := NewMockHelmClient(gomock.NewController(t))
			mockHelmClient.EXPECT().RunHelmGet(gomock.Any(), csiDriverSPIFFEReleaseName).Return(tt.release, tt.getErr)
			got, err := (&Impl{Helm: mockHelmClient}).CSIDriverSPIFFEInstalledByRad("test-context")
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

// UninstallRadius removes csi-driver-spiffe after Radius and its Pods, so the driver can unmount the Radius Pods'
// volumes, and before cert-manager, whose CRDs the driver uses.
func Test_UninstallRadius_RemovesCSIDriverSPIFFEAfterRadius(t *testing.T) {
	t.Parallel()

	client := NewMockHelmClient(gomock.NewController(t))
	gomock.InOrder(
		client.EXPECT().RunHelmUninstall(gomock.Any(), radiusReleaseName, RadiusSystemNamespace, true).Return(&release.UninstallReleaseResponse{}, nil),
		client.EXPECT().RunHelmUninstall(gomock.Any(), contourReleaseName, RadiusSystemNamespace, true).Return(&release.UninstallReleaseResponse{}, nil),
		client.EXPECT().RunHelmGet(gomock.Any(), csiDriverSPIFFEReleaseName).
			Return(&releasev1.Release{Name: csiDriverSPIFFEReleaseName, Labels: radManagedByLabels()}, nil),
		client.EXPECT().RunHelmUninstall(gomock.Any(), csiDriverSPIFFEReleaseName, RadiusSystemNamespace, true).Return(&release.UninstallReleaseResponse{}, nil),
		client.EXPECT().RunHelmGet(gomock.Any(), certManagerReleaseName).Return(nil, driver.ErrReleaseNotFound),
	)

	impl := &Impl{
		Helm:                        client,
		removeDefaultContourGateway: (&defaultContourGatewayCalls{}).remove,
		newCSIDriverSPIFFEClient:    func(string) (kubernetes.Interface, error) { return k8sfake.NewClientset(), nil },
	}
	require.NoError(t, impl.UninstallRadius(t.Context(), NewDefaultClusterOptions(), "test-context"))
}

func Test_UninstallCertManager_RemovesCSIDriverSPIFFEFirst(t *testing.T) {
	t.Parallel()

	client := NewMockHelmClient(gomock.NewController(t))
	gomock.InOrder(
		client.EXPECT().RunHelmGet(gomock.Any(), csiDriverSPIFFEReleaseName).
			Return(&releasev1.Release{Name: csiDriverSPIFFEReleaseName, Labels: radManagedByLabels()}, nil),
		client.EXPECT().RunHelmUninstall(gomock.Any(), csiDriverSPIFFEReleaseName, RadiusSystemNamespace, true).Return(&release.UninstallReleaseResponse{}, nil),
		client.EXPECT().RunHelmGet(gomock.Any(), certManagerReleaseName).Return(nil, driver.ErrReleaseNotFound),
	)

	impl := &Impl{Helm: client, newCSIDriverSPIFFEClient: func(string) (kubernetes.Interface, error) { return k8sfake.NewClientset(), nil }}
	require.NoError(t, impl.UninstallCertManager(t.Context(), NewDefaultClusterOptions(), "test-context"))
}

// A foreign csi-driver-spiffe stops install and upgrade before they touch cert-manager or any release.
func Test_ForeignCSIDriverSPIFFE_FailsBeforeChangingCluster(t *testing.T) {
	t.Parallel()

	for _, command := range []string{"install", "upgrade"} {
		t.Run(command, func(t *testing.T) {
			t.Parallel()

			client := NewMockHelmClient(gomock.NewController(t))
			client.EXPECT().LoadChart(gomock.Any()).Return(&chart.Chart{Metadata: &chart.Metadata{Name: "radius", Version: "0.1.0"}}, nil)
			client.EXPECT().RunHelmGet(gomock.Any(), csiDriverSPIFFEReleaseName).Return(nil, driver.ErrReleaseNotFound)

			impl := &Impl{
				Helm: client,
				newCertManagerClients: func(string) (kubernetes.Interface, apiextclient.Interface, error) {
					t.Fatal("cert-manager was checked after the csi-driver-spiffe check failed")
					return nil, nil, nil
				},
				newCSIDriverSPIFFEClient: func(string) (kubernetes.Interface, error) {
					return k8sfake.NewClientset(renamedCSIDriverSPIFFEDaemonSet("identity", "workload-identity-node", hostPathVolume(csiDriverSPIFFEPluginDir), nil)), nil
				},
			}
			options := NewDefaultClusterOptions()
			options.Radius.ChartPath = "chart"
			options.Radius.SetArgs = []string{"global.rbac.dryRun=true"}
			options.Contour.Disabled = true

			var err error
			if command == "install" {
				err = impl.InstallRadius(t.Context(), options, "test-context")
			} else {
				err = impl.UpgradeRadius(t.Context(), options, "test-context")
			}
			require.ErrorContains(t, err, "Radius stopped before changing the cluster because another csi-driver-spiffe (DaemonSet identity/workload-identity-node) runs on it.")
		})
	}
}

// When Pods still use csi-driver-spiffe, uninstall keeps it and cert-manager, whose CRDs and controller it needs.
func Test_UninstallRadius_KeepsCertManagerWhenCSIDriverSPIFFEIsKept(t *testing.T) {
	t.Parallel()

	client := NewMockHelmClient(gomock.NewController(t))
	gomock.InOrder(
		client.EXPECT().RunHelmUninstall(gomock.Any(), radiusReleaseName, RadiusSystemNamespace, true).Return(&release.UninstallReleaseResponse{}, nil),
		client.EXPECT().RunHelmUninstall(gomock.Any(), contourReleaseName, RadiusSystemNamespace, true).Return(&release.UninstallReleaseResponse{}, nil),
		client.EXPECT().RunHelmGet(gomock.Any(), csiDriverSPIFFEReleaseName).
			Return(&releasev1.Release{Name: csiDriverSPIFFEReleaseName, Labels: radManagedByLabels()}, nil),
	)

	var logs []string
	impl := &Impl{
		Helm:                        client,
		removeDefaultContourGateway: (&defaultContourGatewayCalls{}).remove,
		logger:                      func(format string, v ...any) { logs = append(logs, fmt.Sprintf(format, v...)) },
		newCSIDriverSPIFFEClient: func(string) (kubernetes.Interface, error) {
			return k8sfake.NewClientset(spiffePod(RadiusSystemNamespace, "ucp-0", SPIFFECSIDriverNameDefault)), nil
		},
		csiDriverSPIFFEPodWaitTimeout:  20 * time.Millisecond,
		csiDriverSPIFFEPodPollInterval: 5 * time.Millisecond,
	}
	err := impl.UninstallRadius(t.Context(), NewDefaultClusterOptions(), "test-context")
	var kept *CSIDriverSPIFFEKeptError
	require.ErrorAs(t, err, &kept)
	require.Equal(t, []string{"radius-system/ucp-0"}, kept.Pods)
	require.Len(t, logs, 1)
	require.Contains(t, logs[0], "Keeping csi-driver-spiffe and cert-manager")
}

// Helm rolls back only the Radius release, so rollback first makes csi-driver-spiffe match the target revision's
// authorization settings, or stops before changing anything when it cannot.
func Test_RollbackRadius_ReconcilesCSIDriverSPIFFE(t *testing.T) {
	t.Parallel()

	rbac := func(rbac map[string]any) map[string]any { return map[string]any{"global": map[string]any{"rbac": rbac}} }
	targetValues := rbac(map[string]any{"enabled": true, "spiffe": map[string]any{"trustDomain": "rollback.example"}})
	installedValues := buildCSIDriverSPIFFEValues(RadiusSystemNamespace, spiffeSettings{TrustDomain: SPIFFETrustDomainDefault, CSIDriverName: SPIFFECSIDriverNameDefault})
	wantValues := buildCSIDriverSPIFFEValues(RadiusSystemNamespace, spiffeSettings{TrustDomain: "rollback.example", CSIDriverName: SPIFFECSIDriverNameDefault})
	installed := &releasev1.Release{Name: csiDriverSPIFFEReleaseName, Labels: radManagedByLabels(), Info: &releasev1.Info{Status: releasecommon.StatusDeployed}, Config: installedValues}
	certManager := &releasev1.Release{Name: certManagerReleaseName, Labels: radManagedByLabels(), Info: &releasev1.Info{Status: releasecommon.StatusDeployed},
		Chart: &chart.Chart{Metadata: &chart.Metadata{Version: CertManagerChartDefaultVersion}}, Config: buildCertManagerValues(RadiusSystemNamespace)}
	history := func(target map[string]any) []*releasev1.Release {
		return []*releasev1.Release{
			{Version: 1, Config: target, Chart: &chart.Chart{Metadata: &chart.Metadata{Version: "0.45.0"}}, Info: &releasev1.Info{Status: releasecommon.StatusSuperseded}},
			{Version: 2, Config: rbac(map[string]any{"enabled": true}), Chart: &chart.Chart{Metadata: &chart.Metadata{Version: "0.46.0"}}, Info: &releasev1.Info{Status: releasecommon.StatusDeployed}},
		}
	}
	allowed := map[string]bool{
		issuerSignerName(RadiusSystemNamespace, RadiusBootstrapIssuerName): true,
		issuerSignerName(RadiusSystemNamespace, RadiusMTLSIssuerName):      true,
	}

	tests := []struct {
		name       string
		toRevision bool
		target     map[string]any
		objects    []runtime.Object
		expect     func(*testing.T, *MockHelmClient)
		wantErr    string
	}{
		{
			name:   "different trust domain upgrades the driver before rolling back",
			target: targetValues,
			expect: func(t *testing.T, m *MockHelmClient) {
				gomock.InOrder(
					m.EXPECT().RunHelmGet(gomock.Any(), csiDriverSPIFFEReleaseName).Return(installed, nil),
					m.EXPECT().RunHelmGet(gomock.Any(), certManagerReleaseName).Return(certManager, nil),
					m.EXPECT().RunHelmUpgrade(gomock.Any(), gomock.Any(), wantValues, csiDriverSPIFFEReleaseName, RadiusSystemNamespace, true, false, DefaultInstallTimeout).
						Return(&releasev1.Release{}, nil),
					m.EXPECT().RunHelmRollback(gomock.Any(), radiusReleaseName, 1, true).Return(nil),
				)
				expectCSIDriverSPIFFEPull(t, m)
				m.EXPECT().LoadChart(gomock.Any()).Return(&chart.Chart{}, nil)
			},
		},
		{
			name:       "rollback to a revision also reconciles the driver",
			toRevision: true,
			target:     targetValues,
			expect: func(t *testing.T, m *MockHelmClient) {
				gomock.InOrder(
					m.EXPECT().RunHelmGet(gomock.Any(), csiDriverSPIFFEReleaseName).Return(installed, nil),
					m.EXPECT().RunHelmGet(gomock.Any(), certManagerReleaseName).Return(certManager, nil),
					m.EXPECT().RunHelmUpgrade(gomock.Any(), gomock.Any(), wantValues, csiDriverSPIFFEReleaseName, RadiusSystemNamespace, true, false, DefaultInstallTimeout).
						Return(&releasev1.Release{}, nil),
					m.EXPECT().RunHelmRollback(gomock.Any(), radiusReleaseName, 1, true).Return(nil),
				)
				expectCSIDriverSPIFFEPull(t, m)
				m.EXPECT().LoadChart(gomock.Any()).Return(&chart.Chart{}, nil)
			},
		},
		{
			name:   "matching driver is kept",
			target: rbac(map[string]any{"enabled": true}),
			expect: func(_ *testing.T, m *MockHelmClient) {
				gomock.InOrder(
					m.EXPECT().RunHelmGet(gomock.Any(), csiDriverSPIFFEReleaseName).Return(installed, nil),
					m.EXPECT().RunHelmGet(gomock.Any(), certManagerReleaseName).Return(certManager, nil),
					m.EXPECT().RunHelmRollback(gomock.Any(), radiusReleaseName, 1, true).Return(nil),
				)
			},
		},
		{
			name:   "authorization off in the target changes nothing else",
			target: nil,
			expect: func(_ *testing.T, m *MockHelmClient) {
				m.EXPECT().RunHelmRollback(gomock.Any(), radiusReleaseName, 1, true).Return(nil)
			},
		},
		{
			name:    "foreign driver stops before rolling back",
			target:  targetValues,
			objects: []runtime.Object{renamedCSIDriverSPIFFEDaemonSet("identity", "workload-identity-node", hostPathVolume(csiDriverSPIFFEPluginDir), nil)},
			expect: func(_ *testing.T, m *MockHelmClient) {
				m.EXPECT().RunHelmGet(gomock.Any(), csiDriverSPIFFEReleaseName).Return(nil, driver.ErrReleaseNotFound)
			},
			wantErr: "Radius stopped before changing the cluster because another csi-driver-spiffe (DaemonSet identity/workload-identity-node) runs on it.",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			m := NewMockHelmClient(gomock.NewController(t))
			m.EXPECT().RunHelmHistory(gomock.Any(), radiusReleaseName).Return(history(tt.target), nil)
			tt.expect(t, m)

			objects := append([]runtime.Object{certManagerController(CertManagerNamespace, CertManagerChartDefaultVersion, "")}, tt.objects...)
			impl := &Impl{
				Helm:   m,
				logger: func(string, ...any) {},
				newCertManagerClients: func(string) (kubernetes.Interface, apiextclient.Interface, error) {
					kubeClient, apiextClient := fakeCertManagerCluster{crd: true, controller: certManagerController(CertManagerNamespace, CertManagerChartDefaultVersion, "")}.clients()
					return kubeClient, apiextClient, nil
				},
				newCSIDriverSPIFFEClient: func(string) (kubernetes.Interface, error) { return sarClient(allowed, nil, objects...), nil },
			}
			var err error
			if tt.toRevision {
				err = impl.RollbackRadiusToRevision(t.Context(), "test-context", 1)
			} else {
				err = impl.RollbackRadius(t.Context(), "test-context")
			}
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
		})
	}
}
