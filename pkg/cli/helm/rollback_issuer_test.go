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
	helm "helm.sh/helm/v4/pkg/action"
	chart "helm.sh/helm/v4/pkg/chart/v2"
	"helm.sh/helm/v4/pkg/chart/v2/loader"
	"helm.sh/helm/v4/pkg/release"
	releasecommon "helm.sh/helm/v4/pkg/release/common"
	releasev1 "helm.sh/helm/v4/pkg/release/v1"
)

// preCSIClientCertificate is a component client Certificate as Radius charts rendered it before csi-driver-spiffe.
func preCSIClientCertificate(component string) string {
	return clientCertificate(component, "radius-ca-issuer")
}

// clientCertificate is a component client Certificate issued by issuer.
func clientCertificate(component, issuer string) string {
	return fmt.Sprintf(`apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: %[1]s-cert
  namespace: radius-system
spec:
  secretName: %[1]s-mtls
  usages:
    - client auth
  dnsNames:
    - %[1]s.radius-system.svc
  issuerRef:
    group: cert-manager.io
    kind: Issuer
    name: %[2]s
`, component, issuer)
}

const otherIssuerCertificates = `apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: radius-ca
  namespace: radius-system
spec:
  isCA: true
  issuerRef:
    kind: Issuer
    name: radius-selfsigned-bootstrap
---
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: cluster
  namespace: radius-system
spec:
  issuerRef:
    kind: ClusterIssuer
    name: radius-ca-issuer
---
apiVersion: example.com/v1
kind: Certificate
metadata:
  name: other-group
  namespace: radius-system
spec:
  issuerRef:
    name: radius-ca-issuer
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: settings
  namespace: radius-system
`

func Test_unapprovedIssuerCertificates(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name     string
		manifest string
		want     []string
	}{
		{name: "empty manifest"},
		{name: "other issuers and kinds", manifest: otherIssuerCertificates},
		{
			name:     "pre-csi-driver-spiffe client certificates",
			manifest: otherIssuerCertificates + "---\n" + preCSIClientCertificate("ucp") + "---\n" + preCSIClientCertificate("controller"),
			want:     []string{"radius-system/ucp-cert (Issuer radius-ca-issuer)", "radius-system/controller-cert (Issuer radius-ca-issuer)"},
		},
		{
			name:     "legacy radius-mtls-issuer client certificates",
			manifest: otherIssuerCertificates + "---\n" + clientCertificate("ucp", "radius-mtls-issuer"),
			want:     []string{"radius-system/ucp-cert (Issuer radius-mtls-issuer)"},
		},
		{
			name:     "issuerRef without kind or group",
			manifest: "apiVersion: cert-manager.io/v1\nkind: Certificate\nmetadata:\n  name: c\n  namespace: apps\nspec:\n  issuerRef:\n    name: radius-ca-issuer\n",
			want:     []string{"apps/c (Issuer radius-ca-issuer)"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := unapprovedIssuerCertificates(tt.manifest)
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}

	_, err := unapprovedIssuerCertificates("kind: [")
	require.Error(t, err)
}

// Test_unapprovedIssuerCertificates_CurrentChart checks that the current chart can be a rollback target in every
// authorization mode: no Certificate in it requests from an Issuer that the limited default approver does not approve.
func Test_unapprovedIssuerCertificates_CurrentChart(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		mode string
		rbac map[string]any
	}{
		{mode: "off", rbac: map[string]any{}},
		{mode: "dryRun", rbac: map[string]any{"dryRun": true}},
		{mode: "enforce", rbac: map[string]any{"enabled": true}},
	} {
		t.Run(tt.mode, func(t *testing.T) {
			t.Parallel()

			loaded, err := loader.Load("../../../deploy/Chart")
			require.NoError(t, err)
			install := helm.NewInstall(&helm.Configuration{})
			install.DryRunStrategy = helm.DryRunClient
			install.ReleaseName = "radius"
			install.Namespace = "radius-system"
			rendered, err := install.RunWithContext(t.Context(), loaded, map[string]any{"global": map[string]any{"rbac": tt.rbac}})
			require.NoError(t, err)
			accessor, err := release.NewAccessor(rendered)
			require.NoError(t, err)
			if tt.mode != "off" {
				require.Contains(t, accessor.Manifest(), "kind: Certificate")
			}

			got, err := unapprovedIssuerCertificates(accessor.Manifest())
			require.NoError(t, err)
			require.Empty(t, got)
		})
	}
}

// rollbackHistory returns a history whose revision 1, the rollback target, has targetManifest and targetValues.
func rollbackHistory(targetManifest string, targetValues map[string]any) []*releasev1.Release {
	return []*releasev1.Release{
		{Version: 1, Manifest: targetManifest, Config: targetValues, Chart: &chart.Chart{Metadata: &chart.Metadata{Version: "0.45.0"}}, Info: &releasev1.Info{Status: releasecommon.StatusSuperseded}},
		{Version: 2, Chart: &chart.Chart{Metadata: &chart.Metadata{Version: "0.46.0"}}, Info: &releasev1.Info{Status: releasecommon.StatusDeployed}},
	}
}

func Test_Helm_Rollback_RefusesUnapprovedIssuerCertificates(t *testing.T) {
	t.Parallel()

	type rollbackCase struct {
		name     string
		issuer   string
		rollback func(impl *Impl, t *testing.T) error
	}
	var cases []rollbackCase
	for _, issuer := range []string{"radius-ca-issuer", "radius-mtls-issuer"} {
		cases = append(cases,
			rollbackCase{name: "previous version from " + issuer, issuer: issuer, rollback: func(impl *Impl, t *testing.T) error {
				return impl.RollbackRadius(t.Context(), "test-context")
			}},
			rollbackCase{name: "revision from " + issuer, issuer: issuer, rollback: func(impl *Impl, t *testing.T) error {
				return impl.RollbackRadiusToRevision(t.Context(), "test-context", 1)
			}},
		)
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)
			mockHelmClient := NewMockHelmClient(ctrl)
			mockHelmClient.EXPECT().
				RunHelmHistory(gomock.AssignableToTypeOf(&helm.Configuration{}), "radius").
				Return(rollbackHistory(clientCertificate("applications-rp", tt.issuer), map[string]any{"global": map[string]any{"rbac": map[string]any{"enabled": true}}}), nil)
			// The refusal comes before rollback reconciles cert-manager and csi-driver-spiffe for the target's
			// authorization settings, which reads and upgrades their releases, and before Helm rolls back Radius.
			mockHelmClient.EXPECT().RunHelmGet(gomock.Any(), gomock.Any()).Times(0)
			mockHelmClient.EXPECT().RunHelmUpgrade(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
			mockHelmClient.EXPECT().RunHelmInstall(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
			mockHelmClient.EXPECT().LoadChart(gomock.Any()).Times(0)
			mockHelmClient.EXPECT().RunHelmRollback(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

			err := tt.rollback(&Impl{Helm: mockHelmClient, newRotationStore: emptyRotationTestStore}, t)
			require.ErrorContains(t, err, "Radius cannot roll back to revision 1 because it issues these component client Certificates: radius-system/applications-rp-cert (Issuer "+tt.issuer+")")
			require.ErrorContains(t, err, "rad rollback kubernetes --list-revisions")
		})
	}
}

func Test_Helm_Rollback_AllowsTargetWithoutUnapprovedIssuerCertificates(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mockHelmClient := NewMockHelmClient(ctrl)
	mockHelmClient.EXPECT().
		RunHelmHistory(gomock.AssignableToTypeOf(&helm.Configuration{}), "radius").
		Return(rollbackHistory(otherIssuerCertificates, nil), nil)
	mockHelmClient.EXPECT().
		RunHelmRollback(gomock.AssignableToTypeOf(&helm.Configuration{}), "radius", 1, true).
		Return(nil)

	require.NoError(t, (&Impl{Helm: mockHelmClient, newRotationStore: emptyRotationTestStore}).RollbackRadiusToRevision(t.Context(), "test-context", 1))
}
