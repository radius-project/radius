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

package k8sutil

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// NewNamespaceAPIServer exposes namespace discovery, reads, creates, and patches over HTTP.
// Separate servers can share a fake client to model different addresses for the same cluster.
func NewNamespaceAPIServer(t *testing.T, kubeClient client.Client) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var response any
		switch r.URL.Path {
		case "/api":
			response = &metav1.APIVersions{TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "APIVersions"}, Versions: []string{"v1"}}
		case "/apis":
			response = &metav1.APIGroupList{TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "APIGroupList"}}
		case "/api/v1":
			response = &metav1.APIResourceList{
				TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "APIResourceList"}, GroupVersion: "v1",
				APIResources: []metav1.APIResource{{Name: "namespaces", SingularName: "namespace", Kind: "Namespace", Verbs: metav1.Verbs{"get", "create", "patch"}}},
			}
		default:
			if r.URL.Path != "/api/v1/namespaces" && !strings.HasPrefix(r.URL.Path, "/api/v1/namespaces/") {
				http.NotFound(w, r)
				return
			}
			ns := &corev1.Namespace{TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Namespace"}}
			name := strings.TrimPrefix(r.URL.Path, "/api/v1/namespaces/")
			var err error
			switch r.Method {
			case http.MethodGet:
				err = kubeClient.Get(r.Context(), client.ObjectKey{Name: name}, ns)
			case http.MethodPost:
				var data []byte
				data, err = io.ReadAll(r.Body)
				if err == nil {
					_, _, err = clientgoscheme.Codecs.UniversalDeserializer().Decode(data, nil, ns)
				}
				if err == nil {
					err = kubeClient.Create(r.Context(), ns)
				}
			case http.MethodPatch:
				ns.Name = name
				var patch json.RawMessage
				err = json.NewDecoder(r.Body).Decode(&patch)
				if err == nil {
					err = kubeClient.Patch(r.Context(), ns, client.RawPatch(types.MergePatchType, patch))
				}
			default:
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			if err != nil {
				statusErr, ok := err.(apierrors.APIStatus)
				if !ok {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				status := statusErr.Status()
				w.WriteHeader(int(status.Code))
				response = &status
			} else {
				response = ns
			}
		}
		if err := json.NewEncoder(w).Encode(response); err != nil {
			t.Errorf("encoding fake Kubernetes response: %v", err)
		}
	}))
	t.Cleanup(server.Close)
	return server
}
