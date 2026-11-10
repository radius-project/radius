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
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
)

func TestRotationJournalDecoding(t *testing.T) {
	tests := []struct{ name, data, want string }{
		{"duplicate", `{"phase":"publish","phase":"retire"}`, "duplicate"},
		{"duplicate escaped key", `{"phase":"publish","\u0070hase":"retire"}`, "duplicate"},
		{"duplicate nested plan", `{"plan":{"targetMode":"selfSigned","targetMode":"caSecret"}}`, "duplicate"},
		{"duplicate workload", `{"plan":{"consumers":[{"name":"ucp","name":"controller"}]}}`, "duplicate"},
		{"nested null", `{"plan":{"consumers":[null]}}`, "null"},
		{"unknown", `{"unexpected":true}`, "schema"},
		{"null", `{"phase":null}`, "null"},
		{"array", `[]`, "one object"},
		{"trailing", `{"phase":"publish"}{}`, "trailing"},
		{"truncated", `{"phase":`, "invalid"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := decodeRotationState(tt.data)
			require.ErrorContains(t, err, tt.want)
		})
	}
	now := time.Now().UTC()
	old, _, _ := testSigningCA(t, nil, nil, nil, nil)
	next, _, _ := testSigningCA(t, nil, nil, nil, nil)
	state := rotationState{SchemaVersion: 1, ID: uuid.NewString(), Namespace: RadiusSystemNamespace, Release: radiusReleaseName,
		APIServiceUID: types.UID(uuid.NewString()), Phase: rotationPublish, CreatedAt: now, OldCA: old, NewCA: next}
	data, err := json.Marshal(state)
	require.NoError(t, err)
	loaded, err := decodeRotationState(string(data))
	require.NoError(t, err)
	require.Equal(t, state, loaded)
	require.NoError(t, loaded.validate(now))
	loaded.PreviousRevision = uuid.NewString() + "/publish"
	require.ErrorContains(t, loaded.validate(now), "previous")
	loaded.PreviousRevision = ""
	loaded.Recovery = true
	loaded.PriorRevisions = []string{uuid.NewString() + "/issue"}
	require.ErrorContains(t, loaded.validate(now), "predecessor")
	loaded.Recovery, loaded.PriorRevisions = false, nil
	loaded.ID = uuid.Nil.String()
	require.ErrorContains(t, loaded.validate(now), "identity")
}

func TestRotationRejectsCompetingAPIServiceOwnership(t *testing.T) {
	store := &rotationStore{namespace: RadiusSystemNamespace, release: radiusReleaseName}
	base := &unstructured.Unstructured{Object: map[string]any{
		"metadata": map[string]any{
			"name": radiusAPIService, "uid": uuid.NewString(), "resourceVersion": "1",
			"annotations": map[string]any{"meta.helm.sh/release-name": radiusReleaseName, "meta.helm.sh/release-namespace": RadiusSystemNamespace},
		},
		"spec": map[string]any{"service": map[string]any{"name": "ucp", "namespace": RadiusSystemNamespace}},
	}}
	require.NoError(t, store.checkAPIService(base))
	tests := []struct {
		name   string
		change func(*unstructured.Unstructured)
		want   string
	}{
		{"certificate injection", func(api *unstructured.Unstructured) {
			annotations := api.GetAnnotations()
			annotations["cert-manager.io/inject-ca-from"] = ""
			api.SetAnnotations(annotations)
		}, "injection configured"},
		{"secret injection", func(api *unstructured.Unstructured) {
			annotations := api.GetAnnotations()
			annotations["cert-manager.io/inject-ca-from-secret"] = "radius-system/ca"
			api.SetAnnotations(annotations)
		}, "injection configured"},
		{"API-server injection", func(api *unstructured.Unstructured) {
			annotations := api.GetAnnotations()
			annotations["cert-manager.io/inject-apiserver-ca"] = "true"
			api.SetAnnotations(annotations)
		}, "injection configured"},
		{"removed annotation with injector history", func(api *unstructured.Unstructured) {
			api.SetManagedFields([]metav1.ManagedFieldsEntry{{Manager: "cert-manager-cainjector"}})
		}, "injector write history"},
		{"another trust owner", func(api *unstructured.Unstructured) {
			annotations := api.GetAnnotations()
			annotations[rotationOwner] = "someone-else"
			api.SetAnnotations(annotations)
		}, "another owner"},
		{"another Helm release", func(api *unstructured.Unstructured) {
			annotations := api.GetAnnotations()
			annotations["meta.helm.sh/release-name"] = "someone-else"
			api.SetAnnotations(annotations)
		}, "not owned"},
		{"terminating", func(api *unstructured.Unstructured) { now := metav1.Now(); api.SetDeletionTimestamp(&now) }, "terminating"},
		{"missing version", func(api *unstructured.Unstructured) { api.SetResourceVersion("") }, "identity"},
		{"insecure", func(api *unstructured.Unstructured) {
			require.NoError(t, unstructured.SetNestedField(api.Object, true, "spec", "insecureSkipTLSVerify"))
		}, "verify"},
		{"wrong service", func(api *unstructured.Unstructured) {
			require.NoError(t, unstructured.SetNestedField(api.Object, "different", "spec", "service", "name"))
		}, "unexpected service"},
	}
	for _, tt := range tests {
		t.Run(strings.ReplaceAll(tt.name, " ", "_"), func(t *testing.T) {
			api := base.DeepCopy()
			tt.change(api)
			require.ErrorContains(t, store.checkAPIService(api), tt.want)
		})
	}
}
