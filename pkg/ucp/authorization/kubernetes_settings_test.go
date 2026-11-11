// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

package authorization

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/radius-project/radius/pkg/components/database"
	"github.com/radius-project/radius/pkg/components/database/inmemory"
	api "github.com/radius-project/radius/pkg/ucp/api/authorization/v20250801preview"
	"github.com/stretchr/testify/require"
	authorizationv1 "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
	ktesting "k8s.io/client-go/testing"
)

func settingsReviewClient(t *testing.T, status authorizationv1.SubjectAccessReviewStatus, failure error) *fake.Clientset {
	t.Helper()
	client := fake.NewClientset()
	client.PrependReactor("create", "subjectaccessreviews", func(action ktesting.Action) (bool, runtime.Object, error) {
		review := action.(ktesting.CreateAction).GetObject().(*authorizationv1.SubjectAccessReview)
		require.Equal(t, "alice", review.Spec.User)
		require.Equal(t, "verified-uid", review.Spec.UID)
		require.Equal(t, []string{"operators"}, review.Spec.Groups)
		require.Equal(t, map[string]authorizationv1.ExtraValue{"example.org/tenant": {"a", "b"}}, review.Spec.Extra)
		require.Equal(t, &authorizationv1.ResourceAttributes{
			Verb: "update", Group: "api.ucp.dev", Resource: "authorizationsettings", Name: "default",
		}, review.Spec.ResourceAttributes)
		require.Nil(t, review.Spec.NonResourceAttributes)
		return true, &authorizationv1.SubjectAccessReview{Status: status}, failure
	})
	return client
}

func TestKubernetesSettingsRevocationOnConflict(t *testing.T) {
	client := settingsReviewClient(t, authorizationv1.SubjectAccessReviewStatus{Allowed: true}, nil)
	guard, err := NewKubernetesSettingsGuard(client.AuthorizationV1().SubjectAccessReviews(), "kubernetes:cluster-a", nil)
	require.NoError(t, err)
	db := inmemory.NewClient()
	newTestPolicyStore(t, db, true)
	claims := 0
	wrapped := &policyHookClient{Client: db, before: func(_ context.Context, obj *database.Object) error {
		if saveStage(obj) == "claim" {
			claims++
			client.PrependReactor("create", "subjectaccessreviews", func(ktesting.Action) (bool, runtime.Object, error) {
				return true, &authorizationv1.SubjectAccessReview{Status: authorizationv1.SubjectAccessReviewStatus{Denied: true}}, nil
			})
			return &database.ErrConcurrency{}
		}
		return nil
	}}
	store := newTestPolicyStore(t, wrapped, false)
	store.settings = guard
	actor := settingsActor()
	ctx := context.WithValue(t.Context(), identityContextKey{}, actor.Identity)
	_, err = store.Mutate(ctx, actor, PolicyMutation{
		Target: settingsID, Settings: &api.AuthorizationSettingsProperties{Mode: to.Ptr(api.UserAuthorizationModeDryRun)},
	})
	require.ErrorContains(t, err, "AuthorizationFailed")
	require.Equal(t, 1, claims)
	require.Len(t, client.Actions(), 3)
	snapshot, err := store.LoadPolicy(t.Context(), -1)
	require.NoError(t, err)
	require.Zero(t, snapshot.Generation())
}

func settingsActor() PolicyActor {
	actor := policyActor()
	actor.Identity.KubernetesUID = "verified-uid"
	actor.Identity.KubernetesExtra = map[string][]string{"example.org/tenant": {"a", "b"}}
	actor.Identity.Groups = []api.Principal{testPrincipal(api.PrincipalTypeGroup, "kubernetes:cluster-a", "operators")}
	return actor
}

func TestKubernetesSettingsSubjectAccessReview(t *testing.T) {
	for _, mode := range []api.UserAuthorizationMode{api.UserAuthorizationModeDisabled, api.UserAuthorizationModeDryRun} {
		for _, scenario := range []struct {
			name   string
			status authorizationv1.SubjectAccessReviewStatus
			err    error
			want   string
		}{
			{name: "allowed", status: authorizationv1.SubjectAccessReviewStatus{Allowed: true}},
			{name: "denied", status: authorizationv1.SubjectAccessReviewStatus{Denied: true}, want: "AuthorizationFailed"},
			{name: "no decision", want: "AuthorizationFailed"},
			{name: "contradictory", status: authorizationv1.SubjectAccessReviewStatus{Allowed: true, Denied: true}, want: "AuthorizationFailed"},
			{name: "evaluation failure", status: authorizationv1.SubjectAccessReviewStatus{Allowed: true, EvaluationError: "unavailable"}, want: "AuthorizationUnavailable"},
			{name: "transport failure", err: errors.New("Kubernetes unavailable"), want: "AuthorizationUnavailable"},
		} {
			t.Run(string(mode)+"/"+scenario.name, func(t *testing.T) {
				client := settingsReviewClient(t, scenario.status, scenario.err)
				guard, err := NewKubernetesSettingsGuard(client.AuthorizationV1().SubjectAccessReviews(), "kubernetes:cluster-a", nil)
				require.NoError(t, err)
				store := newTestPolicyStore(t, inmemory.NewClient(), false)
				policy := initialPolicy()
				policy.Assignments = nil
				policy.Settings.Properties.Mode = &mode
				initializer, err := NewPolicyInitializer(store, initializationVerifierFunc(func(context.Context, Policy) error { return nil }))
				require.NoError(t, err)
				require.NoError(t, initializer.Initialize(t.Context(), policy))
				store.settings = guard
				actor := settingsActor()
				ctx := context.WithValue(t.Context(), identityContextKey{}, actor.Identity)
				_, err = store.Mutate(ctx, actor, PolicyMutation{
					Target: settingsID, Settings: &api.AuthorizationSettingsProperties{Mode: to.Ptr(api.UserAuthorizationModeDryRun)},
				})
				if scenario.want != "" {
					require.ErrorContains(t, err, scenario.want)
				} else {
					require.NoError(t, err)
				}
				snapshot, loadErr := store.LoadPolicy(t.Context(), -1)
				require.NoError(t, loadErr)
				saved, loadErr := snapshot.Policy()
				require.NoError(t, loadErr)
				if scenario.want != "" {
					require.Zero(t, saved.Generation)
					require.Empty(t, saved.Assignments)
					require.Equal(t, mode, *saved.Settings.Properties.Mode)
					return
				}
				require.Equal(t, int64(1), saved.Generation)
				require.Len(t, saved.Assignments, 1)
				require.Equal(t, actor.Identity.Principal, *saved.Assignments[0].Properties.Principal)
				require.Equal(t, api.AssignmentManagementKindBootstrap, *saved.Assignments[0].Properties.ManagedBy.Kind)
				require.Zero(t, permanentAdministrators(saved))
				event, err := store.AccessChange(t.Context(), 1)
				require.NoError(t, err)
				require.True(t, *event.Properties.HighImpact)
				require.False(t, *event.Properties.Recovery)
				require.Len(t, event.Properties.PolicyChange.After.RoleAssignments, 1)
				_, err = store.Mutate(ctx, actor, PolicyMutation{
					Target: settingsID, Settings: &api.AuthorizationSettingsProperties{Mode: to.Ptr(api.UserAuthorizationModeDryRun)},
				})
				require.NoError(t, err)
				require.Len(t, client.Actions(), 4, "SAR is repeated even for an idempotent write")
			})
		}
	}
}

func TestKubernetesSettingsRejectsUnverifiedIdentityAndRollout(t *testing.T) {
	for _, scenario := range []string{"missing identity", "forged groups", "foreign issuer", "group caller", "workload caller", "foreign group", "Enforce"} {
		t.Run(scenario, func(t *testing.T) {
			client := fake.NewClientset()
			guard, err := NewKubernetesSettingsGuard(client.AuthorizationV1().SubjectAccessReviews(), "kubernetes:cluster-a", nil)
			require.NoError(t, err)
			actor := settingsActor()
			ctx := t.Context()
			switch scenario {
			case "forged groups":
				ctx = context.WithValue(ctx, identityContextKey{}, actor.Identity)
				actor.Identity.Groups = nil
			case "foreign issuer":
				actor.Identity.Principal.Issuer = to.Ptr("kubernetes:other-cluster")
			case "group caller":
				actor.Identity.Principal.Type = to.Ptr(api.PrincipalTypeGroup)
			case "workload caller":
				actor.Identity.Principal.Type = to.Ptr(api.PrincipalTypeWorkload)
			case "foreign group":
				actor.Identity.Groups[0].Issuer = to.Ptr("kubernetes:other-cluster")
			}
			if scenario != "missing identity" && scenario != "forged groups" {
				ctx = context.WithValue(ctx, identityContextKey{}, actor.Identity)
			}
			before := api.AuthorizationSettingsProperties{Mode: to.Ptr(api.UserAuthorizationModeDisabled)}
			after := api.AuthorizationSettingsProperties{Mode: to.Ptr(api.UserAuthorizationModeDryRun)}
			if scenario == "Enforce" {
				before.Mode = to.Ptr(api.UserAuthorizationModeEnforce)
			}
			require.Error(t, guard.ValidateSettingsChange(ctx, actor, before, after))
			require.Empty(t, client.Actions())
		})
	}
}

func TestKubernetesSettingsRESTBoundary(t *testing.T) {
	for _, scenario := range []struct {
		name    string
		status  int
		allowed bool
		want    string
	}{
		{name: "authorized", status: http.StatusCreated, allowed: true},
		{name: "caller denied", status: http.StatusCreated, want: "AuthorizationFailed"},
		{name: "UCP cannot submit SAR", status: http.StatusForbidden, want: "AuthorizationUnavailable"},
		{name: "API server unavailable", status: http.StatusServiceUnavailable, want: "AuthorizationUnavailable"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			actor := settingsActor()
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				require.Equal(t, http.MethodPost, r.Method)
				require.Equal(t, "/apis/authorization.k8s.io/v1/subjectaccessreviews", r.URL.Path)
				require.Empty(t, r.Header.Get("Impersonate-User"))
				var review authorizationv1.SubjectAccessReview
				require.NoError(t, json.NewDecoder(r.Body).Decode(&review))
				require.Equal(t, *actor.Identity.Principal.Subject, review.Spec.User)
				require.Equal(t, actor.Identity.KubernetesUID, review.Spec.UID)
				require.Equal(t, []string{"operators"}, review.Spec.Groups)
				require.Equal(t, authorizationv1.ExtraValue{"a", "b"}, review.Spec.Extra["example.org/tenant"])
				require.Equal(t, &authorizationv1.ResourceAttributes{
					Group: "api.ucp.dev", Resource: "authorizationsettings", Name: "default", Verb: "update",
				}, review.Spec.ResourceAttributes)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(scenario.status)
				if scenario.status != http.StatusCreated {
					require.NoError(t, json.NewEncoder(w).Encode(metav1.Status{
						TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Status"},
						Status:   "Failure", Code: int32(scenario.status), Reason: metav1.StatusReasonForbidden,
					}))
					return
				}
				require.NoError(t, json.NewEncoder(w).Encode(authorizationv1.SubjectAccessReview{
					TypeMeta: metav1.TypeMeta{APIVersion: "authorization.k8s.io/v1", Kind: "SubjectAccessReview"},
					Status:   authorizationv1.SubjectAccessReviewStatus{Allowed: scenario.allowed},
				}))
			}))
			defer server.Close()
			client, err := kubernetes.NewForConfig(&rest.Config{
				Host: server.URL, ContentConfig: rest.ContentConfig{ContentType: runtime.ContentTypeJSON},
			})
			require.NoError(t, err)
			guard, err := NewKubernetesSettingsGuard(client.AuthorizationV1().SubjectAccessReviews(), "kubernetes:cluster-a", nil)
			require.NoError(t, err)
			ctx := context.WithValue(t.Context(), identityContextKey{}, actor.Identity)
			err = guard.ValidateSettingsChange(ctx, actor,
				api.AuthorizationSettingsProperties{Mode: to.Ptr(api.UserAuthorizationModeDisabled)},
				api.AuthorizationSettingsProperties{Mode: to.Ptr(api.UserAuthorizationModeDryRun)})
			if scenario.want == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, scenario.want)
			}
			require.Equal(t, 1, requests)
		})
	}
}
