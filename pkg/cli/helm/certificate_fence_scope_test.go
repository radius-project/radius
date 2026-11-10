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
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/admission"
	"k8s.io/apiserver/pkg/admission/plugin/policy/generic"
	"k8s.io/apiserver/pkg/admission/plugin/policy/matching"
	"k8s.io/apiserver/pkg/admission/plugin/policy/validating"
	celconfig "k8s.io/apiserver/pkg/apis/cel"
	"k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/client-go/kubernetes/fake"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
)

func TestCandidateTrustFenceResourceScope(t *testing.T) {
	t.Parallel()
	policy, binding := candidateTrustFence()
	require.Nil(t, policy.Spec.ParamKind, "no independently cached mutable trust parameters")
	require.Nil(t, binding.Spec.ParamRef)
	require.Empty(t, policy.Spec.MatchConstraints.NamespaceSelector)
	require.Empty(t, policy.Spec.MatchConstraints.ObjectSelector, "removable labels must not disable the fence")
	matcher := generic.NewPolicyMatcher(matching.NewMatcher(
		corelisters.NewNamespaceLister(cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})), fake.NewClientset()))
	for _, tc := range []struct {
		name, namespace, resource, group, version, subresource string
		match                                                  bool
	}{
		{candidateAPIName, "", "apiservices", candidateAPIKind.Group, "v1", "", true},
		{"v1.unrelated.example", "", "apiservices", candidateAPIKind.Group, "v1", "", false},
		{candidateAPIName, "", "apiservices", "unrelated.example", "v1", "", false},
		{candidateAPIName, "", "secrets", "", "v1", "", false},
		{candidateAPIName, "unexpected", "apiservices", candidateAPIKind.Group, "v1", "", false},
		{candidateAPIName, "", "apiservices", candidateAPIKind.Group, "v1", "status", false},
		{candidateAPIName, "", "apiservices", candidateAPIKind.Group, "v2", "", false},
	} {
		t.Run(tc.name+"/"+tc.group+"/"+tc.resource+"/"+tc.namespace+"/"+tc.version+"/"+tc.subresource, func(t *testing.T) {
			attrs := admission.NewAttributesRecord(nil, nil, candidateAPIKind, tc.namespace, tc.name,
				schema.GroupVersionResource{Group: tc.group, Version: tc.version, Resource: tc.resource},
				tc.subresource, admission.Update, nil, false, nil)
			matches, _, _, err := matcher.DefinitionMatches(attrs, nil, validating.NewValidatingAdmissionPolicyAccessor(policy))
			require.NoError(t, err)
			require.Equal(t, tc.match, matches)
		})
	}
}

func TestCandidateTrustFenceMaterialAndMetadata(t *testing.T) {
	t.Parallel()
	policy, _ := candidateTrustFence()
	validator := candidateFenceValidator(t, policy)
	for _, tc := range []struct {
		name   string
		mutate func(*unstructured.Unstructured)
		deny   bool
	}{
		{"unrelated annotations preserved", func(next *unstructured.Unstructured) {
			annotations := next.GetAnnotations()
			annotations["example.com/unrelated"] = "value"
			next.SetAnnotations(annotations)
		}, false},
		{"cannot disable verification", func(next *unstructured.Unstructured) {
			require.NoError(t, unstructured.SetNestedField(next.Object, true, "spec", "insecureSkipTLSVerify"))
		}, true},
		{"cannot remove bundle", func(next *unstructured.Unstructured) {
			unstructured.RemoveNestedField(next.Object, "spec", "caBundle")
		}, true},
		{"cannot leave empty next pin", func(next *unstructured.Unstructured) {
			annotations := next.GetAnnotations()
			annotations[candidateNext] = ""
			next.SetAnnotations(annotations)
		}, true},
		{"cannot remove annotations", func(next *unstructured.Unstructured) {
			next.SetAnnotations(nil)
		}, true},
		{"status does not require trust mutation", func(next *unstructured.Unstructured) {
			next.Object["status"] = map[string]any{"conditions": []any{map[string]any{"type": "Available", "status": "True"}}}
		}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			old := candidateTrustService([]byte("new"), []byte("new"), nil)
			next := old.DeepCopy()
			tc.mutate(next)
			attrs := admission.NewAttributesRecord(next, old, candidateAPIKind, "", candidateAPIName, candidateAPIResource,
				"", admission.Update, &metav1.UpdateOptions{}, false, &user.DefaultInfo{Name: "rotation-admin"})
			versioned, err := admission.NewVersionedAttributes(attrs, candidateAPIKind, nil)
			require.NoError(t, err)
			result := validator.Validate(t.Context(), candidateAPIResource, versioned, nil, nil, celconfig.RuntimeCELCostBudget, candidateFenceAuthorizer(t))
			require.Len(t, result.Decisions, 1)
			require.NotEqual(t, validating.EvalError, result.Decisions[0].Evaluation, result.Decisions[0].Message)
			require.Equal(t, tc.deny, result.Decisions[0].Action == validating.ActionDeny, result.Decisions[0].Message)
		})
	}
}
