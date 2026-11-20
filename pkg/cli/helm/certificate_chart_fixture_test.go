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
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"helm.sh/helm/v4/pkg/chart/common"
	chartutil "helm.sh/helm/v4/pkg/chart/common/util"
	"helm.sh/helm/v4/pkg/chart/v2/loader"
	"helm.sh/helm/v4/pkg/engine"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
)

func TestCertificateValuesSchema(t *testing.T) {
	t.Parallel()
	chart, err := loader.Load(filepath.Join("..", "..", "..", "deploy", "Chart"))
	require.NoError(t, err)
	tests := []struct {
		name  string
		rbac  any
		valid bool
	}{
		{name: "null is Off", rbac: nil, valid: true},
		{name: "false is Off", rbac: false, valid: true},
		{name: "zero is Off", rbac: int64(0), valid: true},
		{name: "empty string is Off", rbac: "", valid: true},
		{name: "object remains supported", rbac: map[string]any{"enabled": true}, valid: true},
		{name: "true is not an RBAC object", rbac: true},
		{name: "nonzero number is not an RBAC object", rbac: int64(1)},
		{name: "nonempty string is not an RBAC object", rbac: "false"},
		{name: "array is not an RBAC object", rbac: []any{}},
		{name: "issuer reference must still be an object", rbac: map[string]any{
			"certificates": map[string]any{"issuerRef": false},
		}},
		{name: "issuer name must still be a string", rbac: map[string]any{
			"certificates": map[string]any{"issuerRef": map[string]any{"name": false}},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := chartutil.ValidateAgainstSchema(chart, map[string]any{"global": map[string]any{"rbac": tt.rbac}})
			if tt.valid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

// These tests run the real chart's REST-backed lookup against a read-only API
// fixture. They observe issuer readiness, not issuance or approval.
type certificateChartFixture struct {
	objects       []*unstructured.Unstructured
	values        map[string]any
	forbiddenPath string
}

func TestExternalIssuerReadiness(t *testing.T) {
	for _, tt := range []struct {
		name, kind, namespace string
		generation            int64
		want                  string
	}{
		{"namespaced", "Issuer", "radius-system", 1, ""},
		{"cluster scoped", "ClusterIssuer", "", 1, ""},
		{"another namespace", "Issuer", "other", 1, "Issuer is missing"},
		{"stale status", "Issuer", "radius-system", 2, "Ready at its current generation"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := newCertificateChartFixture(t)
			for _, object := range f.objects {
				if object.GetKind() == "Issuer" {
					object.SetKind(tt.kind)
					object.SetGeneration(tt.generation)
					object.SetNamespace(tt.namespace)
				}
			}
			ref := nestedValue(f.values, "global", "rbac", "certificates", "issuerRef").(map[string]any)
			ref["kind"] = tt.kind
			rendered, err := f.renderChart(t)
			if tt.want != "" {
				require.ErrorContains(t, err, tt.want)
				return
			}
			require.NoError(t, err)
			for _, object := range chartObjects(t, rendered) {
				if object.GetKind() == "Certificate" {
					require.Equal(t, ref, nestedValue(object.Object, "spec", "issuerRef"))
				}
			}
		})
	}
}

func newCertificateChartFixture(t *testing.T) *certificateChartFixture {
	t.Helper()
	f := &certificateChartFixture{}
	add := func(apiVersion, kind, namespace, name string, spec map[string]any) *unstructured.Unstructured {
		obj := &unstructured.Unstructured{Object: map[string]any{
			"apiVersion": apiVersion, "kind": kind,
			"metadata": map[string]any{"name": name, "generation": int64(1)},
		}}
		if namespace != "" {
			obj.SetNamespace(namespace)
		}
		if spec != nil {
			obj.Object["spec"] = runtime.DeepCopyJSON(spec)
		}
		f.objects = append(f.objects, obj)
		return obj
	}
	ready := func(obj *unstructured.Unstructured) {
		obj.Object["status"] = map[string]any{"conditions": []any{
			map[string]any{"type": "Ready", "status": "True", "observedGeneration": int64(1)},
		}}
	}
	issuer := map[string]any{"group": "cert-manager.io", "kind": "Issuer", "name": "external-ca"}
	add("v1", "Namespace", "", "radius-system", nil)
	ready(add("cert-manager.io/v1", "Issuer", "radius-system", "external-ca", map[string]any{
		"vault": map[string]any{"server": "https://vault.example"},
	}))
	certificates := map[string]any{
		"mode": "issuerRef", "issuerRef": issuer,
		"caBundle": "-----BEGIN CERTIFICATE-----\ndGVzdA==\n-----END CERTIFICATE-----\n",
	}
	f.values = map[string]any{"de": map[string]any{"internalAuth": map[string]any{"enabled": false}}, "global": map[string]any{"rbac": map[string]any{
		"enabled": true, "certificates": certificates,
	}}}
	return f
}

// renderChart uses the same REST-backed Helm engine as a server-connected install or
// CLI preflight, without installing anything or executing a CertificateRequest.
func (f *certificateChartFixture) renderChart(t *testing.T, extraTemplates ...*common.File) (map[string]string, error) {
	t.Helper()
	server := f.server(t)
	chart, err := loader.Load(filepath.Join("..", "..", "..", "deploy", "Chart"))
	require.NoError(t, err)
	chart.Templates = append(chart.Templates, extraTemplates...)
	values, err := chartutil.CoalesceValues(chart, f.values)
	require.NoError(t, err)
	eng := engine.New(&rest.Config{Host: server.URL})
	caps := common.DefaultCapabilities.Copy()

	return eng.RenderWithContext(t.Context(), chart, common.Values{
		"Values": values, "Capabilities": caps,
		"Release": map[string]any{"Name": "radius", "Namespace": "radius-system", "Service": "Helm"},
	})
}

func (f *certificateChartFixture) server(t *testing.T) *httptest.Server {
	t.Helper()
	resources := map[string]metav1.APIResource{}
	for _, obj := range f.objects {
		plural := strings.ToLower(obj.GetKind()) + "s"
		resources[obj.GetAPIVersion()+"/"+obj.GetKind()] = metav1.APIResource{
			Name: plural, Kind: obj.GetKind(), Namespaced: obj.GetNamespace() != "",
		}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		w.Header().Set("Content-Type", "application/json")
		write := func(value any) {
			require.NoError(t, json.NewEncoder(w).Encode(value))
		}
		if r.URL.Path == f.forbiddenPath {
			w.WriteHeader(http.StatusForbidden)
			write(metav1.Status{TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Status"}, Status: "Failure", Reason: metav1.StatusReasonForbidden, Code: 403, Message: "policy inventory is forbidden"})
			return
		}
		for key, resource := range resources {
			version := strings.TrimSuffix(key, "/"+resource.Kind)
			prefix := "/apis/" + version
			if version == "v1" {
				prefix = "/api/v1"
			}
			if r.URL.Path == prefix {
				list := metav1.APIResourceList{GroupVersion: version}
				for k, item := range resources {
					if strings.TrimSuffix(k, "/"+item.Kind) == version {
						list.APIResources = append(list.APIResources, item)
					}
				}
				write(list)
				return
			}
			if !strings.HasPrefix(r.URL.Path, prefix+"/") {
				continue
			}
			restPath := strings.TrimPrefix(r.URL.Path, prefix+"/")
			namespace := ""
			if strings.HasPrefix(restPath, "namespaces/") && len(strings.Split(restPath, "/")) >= 3 {
				parts := strings.SplitN(restPath, "/", 3)
				namespace, restPath = parts[1], parts[2]
			}
			parts := strings.Split(restPath, "/")
			if parts[0] != resource.Name {
				continue
			}
			items := []any{}
			for _, obj := range f.objects {
				if obj.GetAPIVersion() != version || obj.GetKind() != resource.Kind || (namespace != "" && namespace != obj.GetNamespace()) {
					continue
				}
				if len(parts) == 2 && parts[1] == obj.GetName() {
					write(obj.Object)
					return
				}
				items = append(items, obj.Object)
			}
			if len(parts) == 1 {
				write(map[string]any{"apiVersion": version, "kind": resource.Kind + "List", "items": items})
				return
			}
			break
		}
		w.WriteHeader(http.StatusNotFound)
		write(metav1.Status{TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Status"}, Status: "Failure", Reason: metav1.StatusReasonNotFound, Code: 404})
	}))
	t.Cleanup(server.Close)
	return server
}
