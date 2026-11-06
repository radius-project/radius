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

	"github.com/stretchr/testify/require"
	admissionv1 "k8s.io/api/admissionregistration/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/admission"
	admissioncel "k8s.io/apiserver/pkg/admission/plugin/cel"
	"k8s.io/apiserver/pkg/admission/plugin/policy/generic"
	"k8s.io/apiserver/pkg/admission/plugin/policy/matching"
	"k8s.io/apiserver/pkg/admission/plugin/policy/validating"
	celconfig "k8s.io/apiserver/pkg/apis/cel"
	"k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/apiserver/pkg/authorization/authorizer"
	"k8s.io/apiserver/pkg/cel/environment"
	"k8s.io/client-go/kubernetes/fake"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/utils/ptr"
)

const (
	candidateFenceName = "radius-aggregation-trust-fence"
	candidateAPIName   = "v1alpha3.api.ucp.dev"
	candidateCurrent   = "radius.io/aggregation-trust-current"
	candidateNext      = "radius.io/aggregation-trust-next"
)

var candidateAPIKind = schema.GroupVersionKind{Group: "apiregistration.k8s.io", Version: "v1", Kind: "APIService"}
var candidateAPIResource = schema.GroupVersionResource{Group: "apiregistration.k8s.io", Version: "v1", Resource: "apiservices"}

// This policy is deliberately test-only. Installing it, choosing its operator
// permissions and establishing activation on every writable API server are not
// supported product operations. A locally active policy is not an HA barrier.
func candidateTrustFence() (*admissionv1.ValidatingAdmissionPolicy, *admissionv1.ValidatingAdmissionPolicyBinding) {
	policy := &admissionv1.ValidatingAdmissionPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: candidateFenceName},
		Spec: admissionv1.ValidatingAdmissionPolicySpec{
			FailurePolicy: ptr.To(admissionv1.Fail),
			MatchConstraints: &admissionv1.MatchResources{
				MatchPolicy: ptr.To(admissionv1.Exact), NamespaceSelector: &metav1.LabelSelector{}, ObjectSelector: &metav1.LabelSelector{},
				ResourceRules: []admissionv1.NamedRuleWithOperations{{
					ResourceNames: []string{candidateAPIName},
					RuleWithOperations: admissionv1.RuleWithOperations{
						Operations: []admissionv1.OperationType{admissionv1.Create, admissionv1.Update, admissionv1.Delete},
						Rule: admissionv1.Rule{APIGroups: []string{candidateAPIKind.Group}, APIVersions: []string{"v1"},
							Resources: []string{"apiservices"}, Scope: ptr.To(admissionv1.ClusterScope)},
					},
				}},
			},
			Validations: []admissionv1.Validation{{
				Expression: candidateFenceExpression, Message: "APIService trust must use its protected allowed CA bundles",
				Reason: ptr.To(metav1.StatusReasonForbidden),
			}},
		},
	}
	binding := &admissionv1.ValidatingAdmissionPolicyBinding{
		ObjectMeta: metav1.ObjectMeta{Name: candidateFenceName},
		Spec: admissionv1.ValidatingAdmissionPolicyBindingSpec{
			PolicyName: candidateFenceName, ValidationActions: []admissionv1.ValidationAction{admissionv1.Deny},
		},
	}
	return policy, binding
}

// The authorization capability is a candidate, not an installed role. Anyone
// allowed to change this policy can already weaken its protection.
const candidateFenceAuthority = `authorizer.group('admissionregistration.k8s.io').
resource('validatingadmissionpolicies').name('radius-aggregation-trust-fence').
check('update').allowed()`

const candidateFenceExpression = `
(
  (oldObject == null || !has(oldObject.metadata.annotations) ||
   (!('radius.io/aggregation-trust-current' in oldObject.metadata.annotations) &&
    !('radius.io/aggregation-trust-next' in oldObject.metadata.annotations))) &&
  (object == null || !has(object.metadata.annotations) ||
   (!('radius.io/aggregation-trust-current' in object.metadata.annotations) &&
    !('radius.io/aggregation-trust-next' in object.metadata.annotations)))
) ||
(
  object == null ? (` + candidateFenceAuthority + `) :
  (
    has(object.metadata.annotations) &&
    'radius.io/aggregation-trust-current' in object.metadata.annotations &&
    size(object.metadata.annotations['radius.io/aggregation-trust-current']) > 0 &&
    (!('radius.io/aggregation-trust-next' in object.metadata.annotations) ||
     size(object.metadata.annotations['radius.io/aggregation-trust-next']) > 0) &&
    has(object.spec.caBundle) && size(object.spec.caBundle) > 0 &&
    (!has(object.spec.insecureSkipTLSVerify) || !object.spec.insecureSkipTLSVerify) &&
    (
      object.spec.caBundle == object.metadata.annotations['radius.io/aggregation-trust-current'] ||
      ('radius.io/aggregation-trust-next' in object.metadata.annotations &&
       object.spec.caBundle == object.metadata.annotations['radius.io/aggregation-trust-next'])
    ) &&
    (
      (
        oldObject != null && has(oldObject.metadata.annotations) &&
        'radius.io/aggregation-trust-current' in oldObject.metadata.annotations &&
        object.metadata.annotations['radius.io/aggregation-trust-current'] ==
          oldObject.metadata.annotations['radius.io/aggregation-trust-current'] &&
        ('radius.io/aggregation-trust-next' in object.metadata.annotations) ==
          ('radius.io/aggregation-trust-next' in oldObject.metadata.annotations) &&
        (!('radius.io/aggregation-trust-next' in object.metadata.annotations) ||
         object.metadata.annotations['radius.io/aggregation-trust-next'] ==
           oldObject.metadata.annotations['radius.io/aggregation-trust-next'])
      ) || (` + candidateFenceAuthority + `)
    )
  )
)`

func candidateFenceValidator(t *testing.T, policy *admissionv1.ValidatingAdmissionPolicy) validating.Validator {
	t.Helper()
	compiler := admissioncel.NewConditionCompiler(environment.MustBaseEnvSet(environment.DefaultCompatibilityVersion()))
	options := admissioncel.OptionalVariableDeclarations{HasAuthorizer: true}
	expressions := make([]admissioncel.ExpressionAccessor, len(policy.Spec.Validations))
	for i, validation := range policy.Spec.Validations {
		expressions[i] = &validating.ValidationCondition{
			Expression: validation.Expression, Message: validation.Message, Reason: validation.Reason,
		}
	}
	filter := compiler.CompileCondition(expressions, options, environment.StoredExpressions)
	require.Empty(t, filter.CompilationErrors())
	empty := compiler.CompileCondition(nil, options, environment.StoredExpressions)
	return validating.NewValidator(filter, nil, empty, empty, policy.Spec.FailurePolicy, nil)
}

func candidateTrustService(bundle, current, next []byte) *unstructured.Unstructured {
	annotations := map[string]string{}
	if current != nil {
		annotations[candidateCurrent] = base64.StdEncoding.EncodeToString(current)
	}
	if next != nil {
		annotations[candidateNext] = base64.StdEncoding.EncodeToString(next)
	}
	object := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": candidateAPIKind.GroupVersion().String(), "kind": candidateAPIKind.Kind,
		"metadata": map[string]any{"name": candidateAPIName},
		"spec": map[string]any{
			"group": "api.ucp.dev", "version": "v1alpha3", "groupPriorityMinimum": int64(1000), "versionPriority": int64(15),
			"service":  map[string]any{"namespace": RadiusSystemNamespace, "name": "ucp"},
			"caBundle": base64.StdEncoding.EncodeToString(bundle),
		},
	}}
	object.SetAnnotations(annotations)
	return object
}

func candidateFenceAuthorizer(t *testing.T) authorizer.UnconditionalAuthorizer {
	t.Helper()
	return authorizer.AuthorizerFunc(func(_ context.Context, a authorizer.Attributes) (authorizer.Decision, string, error) {
		require.True(t, a.IsResourceRequest())
		require.Equal(t, "admissionregistration.k8s.io", a.GetAPIGroup())
		require.Equal(t, "validatingadmissionpolicies", a.GetResource())
		require.Equal(t, candidateFenceName, a.GetName())
		require.Empty(t, a.GetNamespace())
		require.Equal(t, "update", a.GetVerb())
		switch a.GetUser().GetName() {
		case "rotation-admin":
			return authorizer.DecisionAllow, "", nil
		case "authorization-error":
			return authorizer.DecisionNoOpinion, "", errors.New("authorization unavailable")
		default:
			return authorizer.DecisionDeny, "", nil
		}
	})
}

func candidateFenceAttributes(old, next *unstructured.Unstructured, caller string) admission.Attributes {
	var before, after runtime.Object
	if old != nil {
		before = old
	}
	if next != nil {
		after = next
	}
	op := admission.Update
	if old == nil {
		op = admission.Create
	} else if next == nil {
		op = admission.Delete
	}
	return admission.NewAttributesRecord(after, before, candidateAPIKind, "", candidateAPIName,
		candidateAPIResource, "", op, nil, false, &user.DefaultInfo{Name: caller})
}

func TestCandidateTrustFenceLifecycle(t *testing.T) {
	t.Parallel()
	old, _, _ := testSigningCA(t, nil, nil, nil, nil)
	newCA, _, _ := testSigningCA(t, nil, nil, nil, nil)
	third, _, _ := testSigningCA(t, nil, nil, nil, nil)
	overlap := append(append([]byte{}, old...), newCA...)
	laterOverlap := append(append([]byte{}, newCA...), third...)
	policy, binding := candidateTrustFence()
	matcher := generic.NewPolicyMatcher(matching.NewMatcher(
		corelisters.NewNamespaceLister(cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})), fake.NewClientset()))
	dispatcher := validating.NewDispatcher(candidateFenceAuthorizer(t), matcher)
	hooks := []validating.PolicyHook{{Policy: policy, Bindings: []*admissionv1.ValidatingAdmissionPolicyBinding{binding},
		Evaluator: candidateFenceValidator(t, policy)}}
	for _, tc := range []struct {
		name, caller string
		old, next    *unstructured.Unstructured
		deny         bool
	}{
		{"legacy Off create", "injector", nil, candidateTrustService(old, nil, nil), false},
		{"legacy Off update", "injector", candidateTrustService(old, nil, nil), candidateTrustService(old, nil, nil), false},
		{"authorized arm", "rotation-admin", candidateTrustService(old, nil, nil), candidateTrustService(old, old, nil), false},
		{"unauthorized arm", "injector", candidateTrustService(old, nil, nil), candidateTrustService(old, old, nil), true},
		{"unauthorized protected create", "injector", nil, candidateTrustService(old, old, nil), true},
		{"authorize overlap", "rotation-admin", candidateTrustService(old, old, nil), candidateTrustService(old, old, overlap), false},
		{"injector publishes overlap", "injector", candidateTrustService(old, old, overlap), candidateTrustService(overlap, old, overlap), false},
		{"lock overlap", "rotation-admin", candidateTrustService(overlap, old, overlap), candidateTrustService(overlap, overlap, nil), false},
		{"queued old-only rejected", "injector", candidateTrustService(overlap, overlap, nil), candidateTrustService(old, overlap, nil), true},
		{"atomic retirement", "rotation-admin", candidateTrustService(overlap, overlap, nil), candidateTrustService(newCA, newCA, nil), false},
		{"queued overlap rejected", "injector", candidateTrustService(newCA, newCA, nil), candidateTrustService(overlap, newCA, nil), true},
		{"no change retry", "injector", candidateTrustService(newCA, newCA, nil), candidateTrustService(newCA, newCA, nil), false},
		{"injector cannot widen allowance", "injector", candidateTrustService(newCA, newCA, nil), candidateTrustService(overlap, overlap, nil), true},
		{"future overlap authorized", "rotation-admin", candidateTrustService(newCA, newCA, nil), candidateTrustService(newCA, newCA, laterOverlap), false},
		{"future overlap injected", "injector", candidateTrustService(newCA, newCA, laterOverlap), candidateTrustService(laterOverlap, newCA, laterOverlap), false},
		{"retired root remains rejected", "injector", candidateTrustService(newCA, newCA, laterOverlap), candidateTrustService(overlap, newCA, laterOverlap), true},
		{"cannot unarm", "rotation-admin", candidateTrustService(newCA, newCA, nil), candidateTrustService(newCA, nil, nil), true},
		{"orphan next pin rejected", "rotation-admin", candidateTrustService(newCA, nil, nil), candidateTrustService(newCA, nil, overlap), true},
		{"empty pin rejected", "rotation-admin", candidateTrustService(newCA, newCA, nil), candidateTrustService(newCA, []byte{}, nil), true},
		{"uninstall allowed", "rotation-admin", candidateTrustService(newCA, newCA, nil), nil, false},
		{"injector deletion rejected", "injector", candidateTrustService(newCA, newCA, nil), nil, true},
		{"authorization unavailable", "authorization-error", candidateTrustService(newCA, newCA, nil), candidateTrustService(newCA, newCA, laterOverlap), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			attrs := candidateFenceAttributes(tc.old, tc.next, tc.caller)
			err := dispatcher.Dispatch(t.Context(), attrs, nil, hooks)
			if tc.deny {
				require.True(t, apierrors.IsForbidden(err), "expected forbidden, got %v", err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestCandidateTrustFenceMissingHooksDoNotProtect(t *testing.T) {
	t.Parallel()
	attrs := candidateFenceAttributes(candidateTrustService([]byte("new"), []byte("new"), nil),
		candidateTrustService([]byte("old+new"), []byte("new"), nil), "injector")
	dispatcher := validating.NewDispatcher(nil, nil)
	require.NoError(t, dispatcher.Dispatch(t.Context(), attrs, nil, []validating.PolicyHook{}),
		"Fail cannot deny a policy that this API server has not loaded")
	policy, _ := candidateTrustFence()
	matcher := generic.NewPolicyMatcher(matching.NewMatcher(
		corelisters.NewNamespaceLister(cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})), fake.NewClientset()))
	dispatcher = validating.NewDispatcher(candidateFenceAuthorizer(t), matcher)
	require.NoError(t, dispatcher.Dispatch(t.Context(), attrs, nil, []validating.PolicyHook{{
		Policy: policy, Evaluator: candidateFenceValidator(t, policy),
	}}), "a policy without a locally loaded binding does not enforce its validation")
}

func TestCandidateTrustFenceFailureIsDenied(t *testing.T) {
	t.Parallel()
	policy, _ := candidateTrustFence()
	policy.Spec.Validations[0].Expression = "object.spec.missing == 'value'"
	validator := candidateFenceValidator(t, policy)
	attrs := candidateFenceAttributes(nil, candidateTrustService([]byte("new"), []byte("new"), nil), "injector")
	versioned, err := admission.NewVersionedAttributes(attrs, candidateAPIKind, nil)
	require.NoError(t, err)
	result := validator.Validate(t.Context(), candidateAPIResource, versioned, nil, nil, celconfig.RuntimeCELCostBudget, nil)
	require.Len(t, result.Decisions, 1)
	require.Equal(t, validating.EvalError, result.Decisions[0].Evaluation)
	require.Equal(t, validating.ActionDeny, result.Decisions[0].Action)
}
