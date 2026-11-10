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
	"fmt"
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
		{name: "policy reference must still be typed", rbac: map[string]any{
			"certificates": map[string]any{"requestPolicy": map[string]any{
				"certManager": map[string]any{"namespace": false},
			}},
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
// fixture. They do not execute cert-manager, approver-policy, or admission.
type issuerPolicyFixture struct {
	objects       []*unstructured.Unstructured
	values        map[string]any
	forbiddenPath string
}

func (f *issuerPolicyFixture) object(kind, name string) *unstructured.Unstructured {
	for _, obj := range f.objects {
		if obj.GetKind() == kind && obj.GetName() == name {
			return obj
		}
	}
	return nil
}

func (f *issuerPolicyFixture) set(t *testing.T, kind, name string, value any, fields ...string) {
	t.Helper()
	obj := f.object(kind, name)
	require.NotNil(t, obj)
	require.NoError(t, unstructured.SetNestedField(obj.Object, value, fields...))
}

func (f *issuerPolicyFixture) container(kind, name string) map[string]any {
	spec := f.object(kind, name).Object["spec"].(map[string]any)
	if kind == "Deployment" {
		spec = spec["template"].(map[string]any)["spec"].(map[string]any)
	}
	return spec["containers"].([]any)[0].(map[string]any)
}

func (f *issuerPolicyFixture) grantComponentCertificateMutation(subject map[string]any, verb string) {
	role := f.object("ClusterRole", "radius-policy-use").DeepCopy()
	role.SetName("component-certificates")
	role.Object["rules"] = []any{map[string]any{
		"apiGroups": []any{"cert-manager.io"}, "resources": []any{"certificates"}, "verbs": []any{verb},
	}}
	binding := f.object("ClusterRoleBinding", "radius-policy-use").DeepCopy()
	binding.SetName("component-certificates")
	binding.Object["roleRef"].(map[string]any)["name"] = role.GetName()
	binding.Object["subjects"] = []any{subject}
	f.objects = append(f.objects, role, binding)
}

func (f *issuerPolicyFixture) useClusterIssuer(t *testing.T) {
	t.Helper()
	issuer := f.object("Issuer", "external-ca")
	issuer.SetKind("ClusterIssuer")
	issuer.SetNamespace("")
	certs := f.values["global"].(map[string]any)["rbac"].(map[string]any)["certificates"].(map[string]any)
	certs["issuerRef"].(map[string]any)["kind"] = "ClusterIssuer"
	for _, obj := range f.objects {
		if obj.GetKind() == "CertificateRequestPolicy" {
			require.NoError(t, unstructured.SetNestedField(obj.Object, "ClusterIssuer", "spec", "selector", "issuerRef", "kind"))
		}
	}
	f.object("ClusterRole", "approver").Object["rules"].([]any)[0].(map[string]any)["resourceNames"] = []any{"clusterissuers.cert-manager.io/external-ca"}
}

func newIssuerPolicyFixture(t *testing.T) *issuerPolicyFixture {
	t.Helper()
	f := &issuerPolicyFixture{}
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
		"requestPolicy": map[string]any{
			"certManager":    map[string]any{"namespace": "cert-manager", "deployment": "cert-manager"},
			"approverPolicy": map[string]any{"namespace": "cert-manager", "deployment": "cert-manager-approver-policy"},
			"webhook":        map[string]any{"namespace": "cert-manager", "service": "cert-manager-webhook", "configuration": "cert-manager-webhook"},
		},
	}
	f.values = map[string]any{"global": map[string]any{"rbac": map[string]any{
		"enabled": true, "certificates": certificates,
	}}}
	for _, c := range []struct {
		name, image string
		args        []any
	}{
		{"cert-manager", "quay.io/jetstack/cert-manager-controller:v1.21.2", []any{"--controllers=-certificaterequests-approver"}},
		{"cert-manager-approver-policy", "quay.io/jetstack/cert-manager-approver-policy:v0.29.0", []any{"--webhook-port=10250"}},
	} {
		containers := []any{map[string]any{"name": c.name, "image": c.image, "args": c.args}}
		podSpec := map[string]any{"serviceAccountName": c.name, "containers": containers}
		d := add("apps/v1", "Deployment", "cert-manager", c.name, map[string]any{
			"replicas": int64(1), "selector": map[string]any{"matchLabels": map[string]any{"app": c.name}},
			"template": map[string]any{"spec": podSpec},
		})
		d.Object["status"] = map[string]any{
			"observedGeneration": int64(1), "updatedReplicas": int64(1), "readyReplicas": int64(1), "replicas": int64(1),
		}
		p := add("v1", "Pod", "cert-manager", c.name+"-pod", runtime.DeepCopyJSON(podSpec))
		p.SetLabels(map[string]string{"app": c.name})
		ready(p)
		p.Object["status"].(map[string]any)["phase"] = "Running"
	}
	webhookPod := add("v1", "Pod", "cert-manager", "cert-manager-webhook-pod", map[string]any{
		"containers": []any{map[string]any{"image": "quay.io/jetstack/cert-manager-webhook:v1.21.2"}},
	})
	ready(webhookPod)
	webhookPod.Object["status"].(map[string]any)["phase"] = "Running"
	webhookPod.Object["status"].(map[string]any)["podIP"] = "127.0.0.1"
	e := add("v1", "Endpoints", "cert-manager", "cert-manager-webhook", nil)
	e.Object["subsets"] = []any{map[string]any{"addresses": []any{
		map[string]any{"ip": "127.0.0.1", "targetRef": map[string]any{
			"kind": "Pod", "namespace": "cert-manager", "name": "cert-manager-webhook-pod",
		}},
	}}}
	for _, kind := range []string{"MutatingWebhookConfiguration", "ValidatingWebhookConfiguration"} {
		operations, resources := []any{"CREATE"}, []any{"certificaterequests"}
		path := "/mutate"
		if kind == "ValidatingWebhookConfiguration" {
			operations, resources = []any{"CREATE", "UPDATE"}, []any{"*/*"}
			path = "/validate"
		}
		obj := add("admissionregistration.k8s.io/v1", kind, "", "cert-manager-webhook", nil)
		obj.Object["webhooks"] = []any{map[string]any{
			"name": "webhook.cert-manager.io", "failurePolicy": "Fail",
			"clientConfig": map[string]any{"caBundle": "dGVzdA==", "service": map[string]any{"namespace": "cert-manager", "name": "cert-manager-webhook", "path": path}},
			"rules": []any{map[string]any{"apiGroups": []any{"cert-manager.io"}, "apiVersions": []any{"v1"},
				"operations": operations, "resources": resources}},
		}}
	}
	names := []any{}
	for _, component := range []string{"ucp", "applications-rp", "dynamic-rp", "controller", "bicep-de"} {
		name := "radius-radius-system-" + component
		names = append(names, name)
		ready(add("policy.cert-manager.io/v1alpha1", "CertificateRequestPolicy", "", name, map[string]any{
			"selector": map[string]any{"issuerRef": issuer, "namespace": map[string]any{"matchNames": []any{"radius-system"}}},
			"allowed": map[string]any{
				"dnsNames": map[string]any{"values": []any{component + ".radius-system.svc"}, "required": true,
					"validations": []any{map[string]any{"rule": fmt.Sprintf(
						"cr.username == 'system:serviceaccount:cert-manager:cert-manager' && cr.namespace == 'radius-system' && cr.name.startsWith('%s-cert-')", component)}}},
				"isCA": false, "usages": []any{"digital signature", "server auth", "client auth"},
			},
			"constraints": map[string]any{"minDuration": "24h", "maxDuration": "24h",
				"privateKey": map[string]any{"algorithm": "ECDSA", "minSize": int64(256), "maxSize": int64(256)}},
		}))
	}
	role := add("rbac.authorization.k8s.io/v1", "ClusterRole", "", "radius-policy-use", nil)
	role.Object["rules"] = []any{map[string]any{
		"apiGroups": []any{"policy.cert-manager.io"}, "resources": []any{"certificaterequestpolicies"},
		"verbs": []any{"use"}, "resourceNames": names,
	}}
	approvalRole := add("rbac.authorization.k8s.io/v1", "ClusterRole", "", "approver", nil)
	approvalRole.Object["rules"] = []any{
		map[string]any{"apiGroups": []any{"cert-manager.io"}, "resources": []any{"signers"}, "verbs": []any{"approve"}, "resourceNames": []any{"issuers.cert-manager.io/radius-system.external-ca"}},
		map[string]any{"apiGroups": []any{"cert-manager.io"}, "resources": []any{"certificaterequests/status"}, "verbs": []any{"patch"}},
		map[string]any{"apiGroups": []any{"cert-manager.io"}, "resources": []any{"certificaterequests"}, "verbs": []any{"list", "watch"}},
		map[string]any{"apiGroups": []any{"policy.cert-manager.io"}, "resources": []any{"certificaterequestpolicies"}, "verbs": []any{"list", "watch"}},
		map[string]any{"apiGroups": []any{"policy.cert-manager.io"}, "resources": []any{"certificaterequestpolicies/status"}, "verbs": []any{"patch"}},
		map[string]any{"apiGroups": []any{"authorization.k8s.io"}, "resources": []any{"subjectaccessreviews"}, "verbs": []any{"create"}},
		map[string]any{"apiGroups": []any{"rbac.authorization.k8s.io"}, "resources": []any{"roles", "clusterroles", "rolebindings", "clusterrolebindings"}, "verbs": []any{"list", "watch"}},
		map[string]any{"apiGroups": []any{""}, "resources": []any{"namespaces"}, "verbs": []any{"list", "watch"}},
	}
	for name, sa := range map[string]string{"radius-policy-use": "cert-manager", "approver": "cert-manager-approver-policy"} {
		b := add("rbac.authorization.k8s.io/v1", "ClusterRoleBinding", "", name, nil)
		b.Object["roleRef"] = map[string]any{"apiGroup": "rbac.authorization.k8s.io", "kind": "ClusterRole", "name": name}
		b.Object["subjects"] = []any{map[string]any{"kind": "ServiceAccount", "name": sa, "namespace": "cert-manager"}}
	}
	return f
}

// render uses the same REST-backed Helm engine as a server-connected install or
// CLI preflight, without installing anything or executing a CertificateRequest.
func (f *issuerPolicyFixture) render(t *testing.T) (string, error) {
	t.Helper()
	resources := map[string]metav1.APIResource{}
	for _, obj := range f.objects {
		plural := strings.ToLower(obj.GetKind()) + "s"
		switch obj.GetKind() {
		case "Endpoints":
			plural = "endpoints"
		case "CertificateRequestPolicy":
			plural = "certificaterequestpolicies"
		}
		resources[obj.GetAPIVersion()+"/"+obj.GetKind()] = metav1.APIResource{
			Name: plural, Kind: obj.GetKind(), Namespaced: obj.GetNamespace() != "",
		}
	}
	// Discovery still advertises resources when an absence is under test.
	for kind, resource := range map[string]string{"CertificateRequestPolicy": "certificaterequestpolicies", "Role": "roles", "RoleBinding": "rolebindings"} {
		version := "rbac.authorization.k8s.io/v1"
		if kind == "CertificateRequestPolicy" {
			version = "policy.cert-manager.io/v1alpha1"
		}
		resources[version+"/"+kind] = metav1.APIResource{Name: resource, Kind: kind, Namespaced: kind != "CertificateRequestPolicy"}
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
	chart, err := loader.Load(filepath.Join("..", "..", "..", "deploy", "Chart"))
	require.NoError(t, err)
	templates := chart.Templates[:0]
	for _, file := range chart.Templates {
		if strings.HasSuffix(file.Name, ".tpl") || file.Name == "templates/global/certificates.yaml" {
			templates = append(templates, file)
		}
	}
	chart.Templates = templates
	eng := engine.New(&rest.Config{Host: server.URL})
	rendered, err := eng.Render(chart, common.Values{
		"Values": f.values, "Release": map[string]any{"Name": "radius", "Namespace": "radius-system"},
	})
	return rendered["radius/templates/global/certificates.yaml"], err
}

func TestIssuerPolicyChart(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		change       func(*testing.T, *issuerPolicyFixture)
		err          string
		noComponents bool
	}{
		{name: "supported namespaced issuer"},
		{name: "issuer API group unsupported", change: func(t *testing.T, f *issuerPolicyFixture) {
			f.values["global"].(map[string]any)["rbac"].(map[string]any)["certificates"].(map[string]any)["issuerRef"].(map[string]any)["group"] = "external.example"
		}, err: "issuerRef.group=cert-manager.io"},
		{name: "issuer private-key Secret reference prohibited", change: func(t *testing.T, f *issuerPolicyFixture) {
			f.values["global"].(map[string]any)["rbac"].(map[string]any)["certificates"].(map[string]any)["caSecret"] = "private-key"
		}, err: "issuerRef must not specify caSecret"},
		{name: "public trust cannot include parent chain", change: func(t *testing.T, f *issuerPolicyFixture) {
			certs := f.values["global"].(map[string]any)["rbac"].(map[string]any)["certificates"].(map[string]any)
			certs["caBundle"] = certs["caBundle"].(string) + certs["caBundle"].(string)
		}, err: "exactly one public dedicated CA certificate"},
		{name: "public trust cannot include private key", change: func(t *testing.T, f *issuerPolicyFixture) {
			certs := f.values["global"].(map[string]any)["rbac"].(map[string]any)["certificates"].(map[string]any)
			certs["caBundle"] = certs["caBundle"].(string) + "-----BEGIN PRIVATE KEY-----\ndGVzdA==\n-----END PRIVATE KEY-----\n"
		}, err: "exactly one public dedicated CA certificate"},
		{name: "supported cluster issuer", change: func(t *testing.T, f *issuerPolicyFixture) {
			f.useClusterIssuer(t)
		}},
		{name: "policy use through namespace RoleBinding", change: func(t *testing.T, f *issuerPolicyFixture) {
			b := f.object("ClusterRoleBinding", "radius-policy-use")
			b.SetKind("RoleBinding")
			b.SetNamespace("radius-system")
		}},
		{name: "external inventory tolerates dormant bounded component policies", noComponents: true, change: func(t *testing.T, f *issuerPolicyFixture) {
			f.values["global"].(map[string]any)["rbac"].(map[string]any)["mtls"] = map[string]any{"provider": "external"}
		}},
		{name: "external inventory requires only UCP policy", noComponents: true, change: func(t *testing.T, f *issuerPolicyFixture) {
			f.values["global"].(map[string]any)["rbac"].(map[string]any)["mtls"] = map[string]any{"provider": "external"}
			var objects []*unstructured.Unstructured
			for _, obj := range f.objects {
				if obj.GetKind() != "CertificateRequestPolicy" || obj.GetName() == "radius-radius-system-ucp" {
					objects = append(objects, obj)
				}
			}
			f.objects = objects
		}},
		{name: "API canonical duration serialization", change: func(t *testing.T, f *issuerPolicyFixture) {
			f.set(t, "CertificateRequestPolicy", "radius-radius-system-ucp", "24h0m0s", "spec", "constraints", "minDuration")
			f.set(t, "CertificateRequestPolicy", "radius-radius-system-ucp", "24h0m0s", "spec", "constraints", "maxDuration")
		}},
		{name: "official default validating namespace selector", change: func(t *testing.T, f *issuerPolicyFixture) {
			w := f.object("ValidatingWebhookConfiguration", "cert-manager-webhook").Object["webhooks"].([]any)[0].(map[string]any)
			w["namespaceSelector"] = map[string]any{"matchExpressions": []any{
				map[string]any{"key": "cert-manager.io/disable-validation", "operator": "NotIn", "values": []any{"true"}},
			}}
		}},
		{name: "namespace disables official validating webhook", change: func(t *testing.T, f *issuerPolicyFixture) {
			f.object("Namespace", "radius-system").SetLabels(map[string]string{"cert-manager.io/disable-validation": "true"})
			w := f.object("ValidatingWebhookConfiguration", "cert-manager-webhook").Object["webhooks"].([]any)[0].(map[string]any)
			w["namespaceSelector"] = map[string]any{"matchExpressions": []any{
				map[string]any{"key": "cert-manager.io/disable-validation", "operator": "NotIn", "values": []any{"true"}},
			}}
		}, err: "must fail closed for the Radius namespace"},
		{name: "offline or missing issuer", change: func(t *testing.T, f *issuerPolicyFixture) {
			f.object("Issuer", "external-ca").SetName("different")
		}, err: "configured existing Issuer/ClusterIssuer is missing"},
		{name: "forbidden policy inspection", change: func(t *testing.T, f *issuerPolicyFixture) {
			f.forbiddenPath = "/apis/policy.cert-manager.io/v1alpha1/certificaterequestpolicies"
		}, err: "policy inventory is forbidden"},
		{name: "stale issuer", change: func(t *testing.T, f *issuerPolicyFixture) {
			f.object("Issuer", "external-ca").SetGeneration(2)
		}, err: "must be Ready at its current generation"},
		{name: "default approver running", change: func(t *testing.T, f *issuerPolicyFixture) {
			f.container("Deployment", "cert-manager")["args"] = []any{"--v=2"}
		}, err: "default auto-approval must be explicitly disabled"},
		{name: "duplicate controller flag", change: func(t *testing.T, f *issuerPolicyFixture) {
			f.container("Deployment", "cert-manager")["args"] = []any{"--controllers=-certificaterequests-approver", "--controllers=*"}
		}, err: "without duplicate controller flags"},
		{name: "unsupported config-file override", change: func(t *testing.T, f *issuerPolicyFixture) {
			f.container("Deployment", "cert-manager")["args"] = []any{"--controllers=-certificaterequests-approver", "--config=/config.yaml"}
		}, err: "unsupported certManager argument"},
		{name: "stale controller rollout", change: func(t *testing.T, f *issuerPolicyFixture) {
			f.object("Deployment", "cert-manager").SetGeneration(2)
		}, err: "complete current Ready rollout"},
		{name: "zero controller replicas", change: func(t *testing.T, f *issuerPolicyFixture) {
			f.set(t, "Deployment", "cert-manager", int64(0), "spec", "replicas")
		}, err: "complete current Ready rollout"},
		{name: "ready pod still runs the old default approver", change: func(t *testing.T, f *issuerPolicyFixture) {
			f.container("Pod", "cert-manager-pod")["args"] = []any{"--controllers=*"}
		}, err: "every certManager pod must be Running/Ready"},
		{name: "policy controller absent", change: func(t *testing.T, f *issuerPolicyFixture) {
			f.object("Deployment", "cert-manager-approver-policy").SetName("different")
		}, err: "approverPolicy Deployment cert-manager/cert-manager-approver-policy is missing"},
		{name: "policy controller not ready", change: func(t *testing.T, f *issuerPolicyFixture) {
			f.set(t, "Pod", "cert-manager-approver-policy-pod", "Pending", "status", "phase")
		}, err: "every approverPolicy pod must be Running/Ready"},
		{name: "unreviewed policy controller version", change: func(t *testing.T, f *issuerPolicyFixture) {
			f.container("Deployment", "cert-manager-approver-policy")["image"] = "quay.io/jetstack/cert-manager-approver-policy:v0.30.0"
		}, err: "requires the supported cert-manager-approver-policy:v0.29.0"},
		{name: "policy missing", change: func(t *testing.T, f *issuerPolicyFixture) {
			f.object("CertificateRequestPolicy", "radius-radius-system-ucp").SetName("wrong-name")
		}, err: "not an exact supported Radius policy"},
		{name: "stale policy generation", change: func(t *testing.T, f *issuerPolicyFixture) {
			f.object("CertificateRequestPolicy", "radius-radius-system-ucp").SetGeneration(2)
		}, err: "must be Ready at its current generation"},
		{name: "wrong issuer policy", change: func(t *testing.T, f *issuerPolicyFixture) {
			f.set(t, "CertificateRequestPolicy", "radius-radius-system-ucp", "other-ca", "spec", "selector", "issuerRef", "name")
		}, err: "install all expected Ready Radius"},
		{name: "wrong namespace policy", change: func(t *testing.T, f *issuerPolicyFixture) {
			f.set(t, "CertificateRequestPolicy", "radius-radius-system-ucp", []any{"other"}, "spec", "selector", "namespace", "matchNames")
		}, err: "not an exact supported Radius policy"},
		{name: "wildcard DNS policy", change: func(t *testing.T, f *issuerPolicyFixture) {
			f.set(t, "CertificateRequestPolicy", "radius-radius-system-ucp", []any{"*.radius-system.svc"}, "spec", "allowed", "dnsNames", "values")
		}, err: "not an exact supported Radius policy"},
		{name: "optional DNS identity", change: func(t *testing.T, f *issuerPolicyFixture) {
			f.set(t, "CertificateRequestPolicy", "radius-radius-system-ucp", false, "spec", "allowed", "dnsNames", "required")
		}, err: "not an exact supported Radius policy"},
		{name: "additional identity SAN", change: func(t *testing.T, f *issuerPolicyFixture) {
			f.set(t, "CertificateRequestPolicy", "radius-radius-system-ucp", []any{"ucp.radius-system.svc", "other.radius-system.svc"}, "spec", "allowed", "dnsNames", "values")
		}, err: "not an exact supported Radius policy"},
		{name: "unrestricted usages", change: func(t *testing.T, f *issuerPolicyFixture) {
			f.set(t, "CertificateRequestPolicy", "radius-radius-system-ucp", []any{"any"}, "spec", "allowed", "usages")
		}, err: "not an exact supported Radius policy"},
		{name: "missing policy constraints", change: func(t *testing.T, f *issuerPolicyFixture) {
			unstructured.RemoveNestedField(f.object("CertificateRequestPolicy", "radius-radius-system-ucp").Object, "spec", "constraints")
		}, err: "not an exact supported Radius policy"},
		{name: "wrong requestor identity", change: func(t *testing.T, f *issuerPolicyFixture) {
			f.set(t, "CertificateRequestPolicy", "radius-radius-system-ucp", []any{map[string]any{"rule": "true"}}, "spec", "allowed", "dnsNames", "validations")
		}, err: "not an exact supported Radius policy"},
		{name: "CA issuance allowed", change: func(t *testing.T, f *issuerPolicyFixture) {
			f.set(t, "CertificateRequestPolicy", "radius-radius-system-ucp", true, "spec", "allowed", "isCA")
		}, err: "not an exact supported Radius policy"},
		{name: "duration widened", change: func(t *testing.T, f *issuerPolicyFixture) {
			f.set(t, "CertificateRequestPolicy", "radius-radius-system-ucp", "48h", "spec", "constraints", "maxDuration")
		}, err: "not an exact supported Radius policy"},
		{name: "key bound widened", change: func(t *testing.T, f *issuerPolicyFixture) {
			f.set(t, "CertificateRequestPolicy", "radius-radius-system-ucp", int64(384), "spec", "constraints", "privateKey", "maxSize")
		}, err: "not an exact supported Radius policy"},
		{name: "additional OR policy even without Ready", change: func(t *testing.T, f *issuerPolicyFixture) {
			p := f.object("CertificateRequestPolicy", "radius-radius-system-ucp").DeepCopy()
			p.SetName("broad")
			p.Object["spec"] = map[string]any{"selector": map[string]any{"issuerRef": map[string]any{}}}
			delete(p.Object, "status")
			f.objects = append(f.objects, p)
		}, err: "overlapping policy broad"},
		{name: "same issuer cross-namespace policy can mint a Radius SAN", change: func(t *testing.T, f *issuerPolicyFixture) {
			f.useClusterIssuer(t)
			p := f.object("CertificateRequestPolicy", "radius-radius-system-ucp").DeepCopy()
			p.SetName("other-namespace")
			require.NoError(t, unstructured.SetNestedField(p.Object, []any{"other-team"}, "spec", "selector", "namespace", "matchNames"))
			// A request namespace is not encoded in the certificate. Allowing
			// the Radius SAN here would still impersonate UCP to a receiver.
			f.objects = append(f.objects, p)
		}, err: "overlapping policy other-namespace"},
		{name: "disjoint issuer policy is not evaluated", change: func(t *testing.T, f *issuerPolicyFixture) {
			p := f.object("CertificateRequestPolicy", "radius-radius-system-ucp").DeepCopy()
			p.SetName("different-issuer")
			p.Object["spec"] = map[string]any{"selector": map[string]any{"issuerRef": map[string]any{"name": "unrelated-ca"}}}
			f.objects = append(f.objects, p)
		}},
		{name: "requestor policy-use binding missing", change: func(t *testing.T, f *issuerPolicyFixture) {
			f.object("ClusterRoleBinding", "radius-policy-use").Object["subjects"] = []any{}
		}, err: "actual cert-manager controller ServiceAccount lacks policy use"},
		{name: "broad policy-use group binding", change: func(t *testing.T, f *issuerPolicyFixture) {
			f.object("ClusterRoleBinding", "radius-policy-use").Object["subjects"] = []any{map[string]any{"kind": "Group", "name": "system:authenticated"}}
		}, err: "grants Radius policy use to a subject other than"},
		{name: "missing one policy-use resourceName", change: func(t *testing.T, f *issuerPolicyFixture) {
			f.object("ClusterRole", "radius-policy-use").Object["rules"].([]any)[0].(map[string]any)["resourceNames"] = []any{"radius-radius-system-ucp"}
		}, err: "lacks policy use"},
		{name: "RBAC resourceNames star is literal not wildcard", change: func(t *testing.T, f *issuerPolicyFixture) {
			f.object("ClusterRole", "radius-policy-use").Object["rules"].([]any)[0].(map[string]any)["resourceNames"] = []any{"*"}
		}, err: "lacks policy use"},
		{name: "namespace-only signer approval cannot work", change: func(t *testing.T, f *issuerPolicyFixture) {
			b := f.object("ClusterRoleBinding", "approver")
			b.SetKind("RoleBinding")
			b.SetNamespace("radius-system")
		}, err: "lacks required cluster-wide signer approval"},
		{name: "approver cannot create subject access review", change: func(t *testing.T, f *issuerPolicyFixture) {
			f.object("ClusterRole", "approver").Object["rules"].([]any)[5].(map[string]any)["verbs"] = []any{"get"}
		}, err: "lacks required cluster-wide signer approval"},
		{name: "component can create Certificate", change: func(t *testing.T, f *issuerPolicyFixture) {
			f.grantComponentCertificateMutation(map[string]any{"kind": "ServiceAccount", "namespace": "radius-system", "name": "controller"}, "create")
		}, err: "component controller must not create or modify"},
		{name: "component group can create Certificate", change: func(t *testing.T, f *issuerPolicyFixture) {
			f.grantComponentCertificateMutation(map[string]any{"kind": "Group", "name": "system:serviceaccounts:radius-system"}, "create")
		}, err: "must not create or modify certificate"},
		{name: "component user can patch Certificate", change: func(t *testing.T, f *issuerPolicyFixture) {
			f.grantComponentCertificateMutation(map[string]any{"kind": "User", "name": "system:serviceaccount:radius-system:controller"}, "patch")
		}, err: "component controller must not create or modify"},
		{name: "another subject can approve outside policy", change: func(t *testing.T, f *issuerPolicyFixture) {
			b := f.object("ClusterRoleBinding", "approver").DeepCopy()
			b.SetName("default-approver")
			b.Object["subjects"] = []any{map[string]any{"kind": "ServiceAccount", "namespace": "cert-manager", "name": "cert-manager"}}
			f.objects = append(f.objects, b)
		}, err: "including the default approver"},
		{name: "mutating webhook excludes namespace", change: func(t *testing.T, f *issuerPolicyFixture) {
			f.object("MutatingWebhookConfiguration", "cert-manager-webhook").Object["webhooks"].([]any)[0].(map[string]any)["namespaceSelector"] = map[string]any{"matchLabels": map[string]any{"skip": "true"}}
		}, err: "must fail closed for the Radius namespace"},
		{name: "validating webhook fails open", change: func(t *testing.T, f *issuerPolicyFixture) {
			f.object("ValidatingWebhookConfiguration", "cert-manager-webhook").Object["webhooks"].([]any)[0].(map[string]any)["failurePolicy"] = "Ignore"
		}, err: "must fail closed for the Radius namespace"},
		{name: "webhook no ready backend", change: func(t *testing.T, f *issuerPolicyFixture) {
			f.object("Endpoints", "cert-manager-webhook").Object["subsets"] = []any{}
		}, err: "has no Ready endpoints"},
		{name: "webhook rule does not cover status", change: func(t *testing.T, f *issuerPolicyFixture) {
			w := f.object("ValidatingWebhookConfiguration", "cert-manager-webhook").Object["webhooks"].([]any)[0].(map[string]any)
			w["rules"].([]any)[0].(map[string]any)["resources"] = []any{"certificaterequests"}
		}, err: "must protect CertificateRequest creation and status updates"},
		{name: "webhook cluster-only scope excludes requests", change: func(t *testing.T, f *issuerPolicyFixture) {
			w := f.object("ValidatingWebhookConfiguration", "cert-manager-webhook").Object["webhooks"].([]any)[0].(map[string]any)
			w["rules"].([]any)[0].(map[string]any)["scope"] = "Cluster"
		}, err: "must protect CertificateRequest creation and status updates"},
		{name: "webhook endpoint routing mismatch", change: func(t *testing.T, f *issuerPolicyFixture) {
			f.set(t, "Pod", "cert-manager-webhook-pod", "127.0.0.2", "status", "podIP")
		}, err: "endpoint address must match"},
		{name: "webhook unverified configuration file", change: func(t *testing.T, f *issuerPolicyFixture) {
			f.container("Pod", "cert-manager-webhook-pod")["args"] = []any{"--config=/config.yaml"}
		}, err: "unsupported cert-manager webhook argument"},
		{name: "webhook excludes selected objects", change: func(t *testing.T, f *issuerPolicyFixture) {
			f.object("MutatingWebhookConfiguration", "cert-manager-webhook").Object["webhooks"].([]any)[0].(map[string]any)["objectSelector"] = map[string]any{"matchLabels": map[string]any{"protected": "true"}}
		}, err: "must fail closed for the Radius namespace"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newIssuerPolicyFixture(t)
			if tt.change != nil {
				tt.change(t, f)
			}
			rendered, err := f.render(t)
			if tt.err != "" {
				require.ErrorContains(t, err, tt.err)
				return
			}
			require.NoError(t, err)
			count := 5
			if tt.noComponents {
				count = 0
			}
			require.Equal(t, count, strings.Count(rendered, "kind: Certificate\n"))
			if !tt.noComponents {
				require.Contains(t, rendered, "name: external-ca")
			}
			require.Contains(t, rendered, "name: radius-mtls-ca")
			require.NotContains(t, rendered, "\nkind: Issuer\n")
			require.NotContains(t, rendered, "kind: Secret\n")
			require.NotContains(t, rendered, "radius-ca-issuer")
		})
	}
}
