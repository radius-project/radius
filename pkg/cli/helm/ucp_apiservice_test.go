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
	"encoding/base64"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	chart "helm.sh/helm/v4/pkg/chart/v2"
	releasecommon "helm.sh/helm/v4/pkg/release/common"
	releasev1 "helm.sh/helm/v4/pkg/release/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes"
	kubefake "k8s.io/client-go/kubernetes/fake"

	"github.com/radius-project/radius/pkg/authz"
)

// ucpAPIService returns the UCP APIService with caBundle and, if available is not empty, an Available condition.
func ucpAPIService(caBundle string, available string, reason string) *unstructured.Unstructured {
	object := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "apiregistration.k8s.io/v1",
		"kind":       "APIService",
		"metadata": map[string]any{
			"name":   ucpAPIServiceName,
			"labels": map[string]any{helmManagedByLabel: helmManagedByValue},
			"annotations": map[string]any{
				helmReleaseNameAnnotation:      "radius",
				helmReleaseNamespaceAnnotation: RadiusSystemNamespace,
			},
		},
		"spec": map[string]any{
			"group":   "api.ucp.dev",
			"version": "v1alpha3",
			"service": map[string]any{"name": "ucp", "namespace": RadiusSystemNamespace},
		},
	}}
	if caBundle != "" {
		object.Object["spec"].(map[string]any)["caBundle"] = caBundle
	}
	if available != "" {
		object.Object["status"] = map[string]any{"conditions": []any{
			map[string]any{"type": "Available", "status": available, "reason": reason, "message": "test message"},
		}}
	}
	return object
}

// setAnnotation sets annotation key of object to value, keeping its other annotations.
func setAnnotation(object *unstructured.Unstructured, key, value string) {
	annotations := object.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}
	annotations[key] = value
	object.SetAnnotations(annotations)
}

func ucpAPIServiceDynamicClient(objects ...runtime.Object) dynamic.Interface {
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{
		apiServicesResource: "APIServiceList",
	}, objects...)
}

// readyUCPAPIServiceClient returns a client for a cluster whose UCP APIService has a CA bundle and is Available.
func readyUCPAPIServiceClient(string) (dynamic.Interface, error) {
	return ucpAPIServiceDynamicClient(ucpAPIService(base64.StdEncoding.EncodeToString(issuedUCPAPIServerCA()), "True", "Passed")), nil
}

// withoutUCPAPIService makes impl see a cluster without the UCP APIService, so the step that removes trust left by
// authorization on, after Helm switches it off, finds nothing to do.
func withoutUCPAPIService(impl *Impl) *Impl {
	impl.newCertManagerDynamicClient = func(string) (dynamic.Interface, error) { return ucpAPIServiceDynamicClient(), nil }
	impl.newUCPAPIServerTrustClient = func(string) (kubernetes.Interface, error) { return kubefake.NewSimpleClientset(), nil }
	return impl
}

func Test_waitForUCPAPIService(t *testing.T) {
	timeout, interval := ucpAPIServiceTimeout, ucpAPIServicePollInterval
	ucpAPIServiceTimeout, ucpAPIServicePollInterval = 50*time.Millisecond, 10*time.Millisecond
	t.Cleanup(func() { ucpAPIServiceTimeout, ucpAPIServicePollInterval = timeout, interval })

	// With authorization off the chart sets the CA bundle; rad only removes trust that authorization on left, which
	// Test_ReconcileUCPOffAPIServerTrust covers, and fails if it cannot read the cluster.
	off := &Impl{
		newCertManagerDynamicClient: func(string) (dynamic.Interface, error) { return ucpAPIServiceDynamicClient(), nil },
		newUCPAPIServerTrustClient:  func(string) (kubernetes.Interface, error) { return kubefake.NewSimpleClientset(), nil },
	}
	require.NoError(t, off.waitForUCPAPIService(t.Context(), "test-context", RadiusSystemNamespace, authz.ModeOff, nil))
	unreachable := &Impl{
		newCertManagerDynamicClient: func(string) (dynamic.Interface, error) { return nil, errors.New("no cluster") },
		newUCPAPIServerTrustClient:  func(string) (kubernetes.Interface, error) { return kubefake.NewSimpleClientset(), nil },
	}
	require.ErrorContains(t, unreachable.waitForUCPAPIService(t.Context(), "test-context", RadiusSystemNamespace, authz.ModeOff, nil), "no cluster")

	tests := []struct {
		name    string
		objects []runtime.Object
		wantErr []string
	}{
		{name: "ready", objects: []runtime.Object{ucpAPIService("Y2E=", "True", "Passed")}},
		{
			name:    "missing",
			wantErr: []string{"it does not exist", "kubectl --context \"test-context\" get apiservice v1alpha3.api.ucp.dev -o yaml"},
		},
		{
			// The controller and webhook are healthy, but nothing injects the CA, for example because the cainjector
			// is down.
			name:    "no CA injected",
			objects: []runtime.Object{ucpAPIService("", "False", "FailedDiscoveryCheck")},
			wantErr: []string{
				"it has no CA bundle",
				"ca.crt of Secret radius-system/radius-apiserver-ca",
				"kubectl --context \"test-context\" -n radius-system get secret radius-apiserver-ca -o jsonpath='{.data.ca\\.crt}'",
				"kubectl --context \"test-context\" get pods -A -l app.kubernetes.io/component=cainjector",
			},
		},
		{
			name:    "not available",
			objects: []runtime.Object{ucpAPIService("Y2E=", "False", "FailedDiscoveryCheck")},
			wantErr: []string{"it is not Available (FailedDiscoveryCheck: test message)", "ucp pods are ready"},
		},
		{
			name:    "no conditions",
			objects: []runtime.Object{ucpAPIService("Y2E=", "", "")},
			wantErr: []string{"has not reported whether it is Available"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			impl := &Impl{newCertManagerDynamicClient: func(string) (dynamic.Interface, error) {
				return ucpAPIServiceDynamicClient(tt.objects...), nil
			}}

			err := impl.waitForAvailableUCPAPIService(t.Context(), "test-context", RadiusSystemNamespace)
			if len(tt.wantErr) == 0 {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			// The Secret holds the CA's private key; diagnostics must never print the whole Secret.
			require.NotContains(t, err.Error(), "-o yaml\n")
			require.NotContains(t, err.Error(), "get secret "+ucpAPIServerCASecretName+" -o yaml")
			for _, want := range tt.wantErr {
				require.Contains(t, err.Error(), want)
			}
		})
	}
}

func Test_waitForRolledBackUCPAPIService(t *testing.T) {
	timeout, interval := ucpAPIServiceTimeout, ucpAPIServicePollInterval
	ucpAPIServiceTimeout, ucpAPIServicePollInterval = 50*time.Millisecond, 10*time.Millisecond
	t.Cleanup(func() { ucpAPIServiceTimeout, ucpAPIServicePollInterval = timeout, interval })

	history := []*releasev1.Release{
		{Version: 1, Config: map[string]any{}},
		{Version: 2, Config: map[string]any{"global": map[string]any{"rbac": map[string]any{"enabled": true}}}},
	}
	impl := &Impl{
		newUCPAPIServerTrustClient: uninstallTrustClient,
		newCertManagerDynamicClient: func(string) (dynamic.Interface, error) {
			return ucpAPIServiceDynamicClient(ucpAPIService("", "", "")), nil
		},
	}

	// Rolling back to a revision with authorization off does not wait.
	require.NoError(t, impl.waitForRolledBackUCPAPIService(t.Context(), "test-context", RadiusSystemNamespace, history, 1))
	// Rolling back to a revision with authorization on waits for the API server CA.
	err := impl.waitForRolledBackUCPAPIService(t.Context(), "test-context", RadiusSystemNamespace, history, 2)
	require.ErrorContains(t, err, "Secret radius-system/radius-apiserver-ca is not usable")
}

// A chart before radius-apiserver-ca accepts the values of authorization on but serves UCP from the Secret ucp-cert of
// authorization off, so rad waits as for authorization off rather than for radius-apiserver-ca.
func Test_waitForInstalledUCPAPIService_ChartWithoutAPIServerCA(t *testing.T) {
	timeout, interval := ucpAPIServiceTimeout, ucpAPIServicePollInterval
	ucpAPIServiceTimeout, ucpAPIServicePollInterval = 50*time.Millisecond, 10*time.Millisecond
	t.Cleanup(func() { ucpAPIServiceTimeout, ucpAPIServicePollInterval = timeout, interval })
	impl := &Impl{
		newUCPAPIServerTrustClient: uninstallTrustClient,
		newCertManagerDynamicClient: func(string) (dynamic.Interface, error) {
			return ucpAPIServiceDynamicClient(ucpAPIService("", "", "")), nil
		},
	}
	enabled := map[string]any{"global": map[string]any{"rbac": map[string]any{"enabled": true}}}
	served := func(kind string) *releasev1.Release {
		apiVersion := "v1"
		if kind == "Certificate" {
			apiVersion = "cert-manager.io/v1"
		}
		return &releasev1.Release{Version: 1, Manifest: "apiVersion: " + apiVersion + "\nkind: " + kind + "\nmetadata:\n  name: ucp-cert\n"}
	}

	require.NoError(t, impl.waitForInstalledUCPAPIService(t.Context(), "test-context", RadiusSystemNamespace, served("Secret"), nil, nil, enabled))
	require.ErrorContains(t, impl.waitForInstalledUCPAPIService(t.Context(), "test-context", RadiusSystemNamespace, served("Certificate"), nil, nil, enabled), "Secret radius-system/radius-apiserver-ca is not usable")

	// A chart that enables authorization by default serves UCP from the Certificate ucp-cert while the user values say
	// nothing, so rad waits for radius-apiserver-ca rather than reporting success without it.
	require.ErrorContains(t, impl.waitForInstalledUCPAPIService(t.Context(), "test-context", RadiusSystemNamespace, served("Certificate"), enabled, nil, nil), "Secret radius-system/radius-apiserver-ca is not usable")
	require.ErrorContains(t, impl.waitForInstalledUCPAPIService(t.Context(), "test-context", RadiusSystemNamespace, served("Certificate"), nil, nil, nil), "Secret radius-system/radius-apiserver-ca is not usable")
}

// With a chart that enables authorization by default, rad stages radius-apiserver-ca from the rendered chart before
// Helm, although the user values say nothing about authorization.
func Test_stageInstalledUCPAPIServerCA_ChartDefaults(t *testing.T) {
	impl := &Impl{
		newUCPAPIServerTrustClient: uninstallTrustClient,
		newCertManagerDynamicClient: func(string) (dynamic.Interface, error) {
			return ucpAPIServiceDynamicClient(ucpAPIService("", "", "")), nil
		},
	}
	errRendered := errors.New("rendered")
	render := func(context.Context) (string, bool, error) { return "", false, errRendered }
	options := NewDefaultClusterOptions()

	require.NoError(t, impl.stageInstalledUCPAPIServerCA(t.Context(), options, "test-context", nil, nil, nil, nil, false, render))
	defaults := map[string]any{"global": map[string]any{"rbac": map[string]any{"enabled": true}}}
	require.ErrorIs(t, impl.stageInstalledUCPAPIServerCA(t.Context(), options, "test-context", nil, defaults, nil, nil, false, render), errRendered)
}

func Test_deployedAuthzMode(t *testing.T) {
	const certificate = "apiVersion: cert-manager.io/v1\nkind: Certificate\nmetadata:\n  name: ucp-cert\n"
	const secret = "apiVersion: v1\nkind: Secret\nmetadata:\n  name: ucp-cert\n"
	enforce := map[string]any{"global": map[string]any{"rbac": map[string]any{"enabled": true, "mode": "enforce"}}}
	revision := func(version int, status releasecommon.Status, config map[string]any, manifest string) *releasev1.Release {
		return &releasev1.Release{Version: version, Config: config, Manifest: manifest, Info: &releasev1.Info{Status: status}}
	}

	tests := []struct {
		name    string
		history []*releasev1.Release
		want    authz.Mode
		wantErr string
	}{
		{name: "no history", want: authz.ModeOff},
		{name: "deployed on", history: []*releasev1.Release{revision(1, releasecommon.StatusDeployed, enforce, certificate)}, want: authz.ModeEnforce},
		{name: "deployed off", history: []*releasev1.Release{revision(1, releasecommon.StatusDeployed, map[string]any{}, secret)}, want: authz.ModeOff},
		{
			name:    "failed revision above a deployed one is unknown",
			history: []*releasev1.Release{revision(2, releasecommon.StatusFailed, map[string]any{}, secret), revision(1, releasecommon.StatusDeployed, enforce, certificate)},
			want:    "",
		},
		{name: "chart that ignores its authorization values", history: []*releasev1.Release{revision(1, releasecommon.StatusDeployed, enforce, secret)}, want: authz.ModeOff},
		// The manifest decides between on and off; a chart that renders the Certificate serves authorization on.
		{name: "Certificate with authorization off in values", history: []*releasev1.Release{revision(1, releasecommon.StatusDeployed, map[string]any{}, certificate)}, want: authz.ModeEnforce},
		{
			name: "chart that enables authorization by default",
			history: []*releasev1.Release{{
				Version: 1, Config: map[string]any{}, Manifest: certificate, Info: &releasev1.Info{Status: releasecommon.StatusDeployed},
				Chart: &chart.Chart{Values: map[string]any{"global": map[string]any{"rbac": map[string]any{"enabled": false, "dryRun": true}}}},
			}},
			want: authz.ModeDryRun,
		},
		{
			name: "invalid values",
			history: []*releasev1.Release{{
				Version: 1, Config: map[string]any{"global": map[string]any{"rbac": map[string]any{"dryRun": true}}}, Manifest: certificate, Info: &releasev1.Info{Status: releasecommon.StatusDeployed},
				Chart: &chart.Chart{Values: map[string]any{"global": map[string]any{"rbac": map[string]any{"enabled": true}}}},
			}},
			wantErr: "cannot be combined",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := deployedAuthzMode(tt.history)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}
