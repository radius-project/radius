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
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	releasev1 "helm.sh/helm/v4/pkg/release/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/radius-project/radius/pkg/authz"
)

// ucpAPIService returns the UCP APIService with caBundle and, if available is not empty, an Available condition.
func ucpAPIService(caBundle string, available string, reason string) *unstructured.Unstructured {
	object := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "apiregistration.k8s.io/v1",
		"kind":       "APIService",
		"metadata":   map[string]any{"name": ucpAPIServiceName},
		"spec":       map[string]any{"group": "api.ucp.dev", "version": "v1alpha3"},
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

func ucpAPIServiceDynamicClient(objects ...runtime.Object) dynamic.Interface {
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{
		apiServicesResource: "APIServiceList",
	}, objects...)
}

// readyUCPAPIServiceClient returns a client for a cluster whose UCP APIService has a CA bundle and is Available.
func readyUCPAPIServiceClient(string) (dynamic.Interface, error) {
	return ucpAPIServiceDynamicClient(ucpAPIService(base64.StdEncoding.EncodeToString(issuedUCPAPIServerCA()), "True", "Passed")), nil
}

func Test_waitForUCPAPIService(t *testing.T) {
	timeout, interval := ucpAPIServiceTimeout, ucpAPIServicePollInterval
	ucpAPIServiceTimeout, ucpAPIServicePollInterval = 50*time.Millisecond, 10*time.Millisecond
	t.Cleanup(func() { ucpAPIServiceTimeout, ucpAPIServicePollInterval = timeout, interval })

	// With authorization off the chart sets the CA bundle, so rad does not read the cluster.
	off := &Impl{newCertManagerDynamicClient: func(string) (dynamic.Interface, error) { return nil, errors.New("no cluster") }}
	require.NoError(t, off.waitForUCPAPIService(t.Context(), "test-context", RadiusSystemNamespace, authz.ModeOff))

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
				"kubectl --context \"test-context\" -n radius-system get secret radius-apiserver-ca -o yaml",
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
	// Authorization-on rollback cannot discover trust from a signing Secret.
	err := impl.waitForRolledBackUCPAPIService(t.Context(), "test-context", RadiusSystemNamespace, history, 2)
	require.ErrorContains(t, err, "cannot read permanent aggregation trust")
}
