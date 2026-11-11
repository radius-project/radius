// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

package authorization

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/radius-project/radius/pkg/components/database"
	"github.com/radius-project/radius/pkg/components/database/inmemory"
	api "github.com/radius-project/radius/pkg/ucp/api/authorization/v20250801preview"
	"github.com/stretchr/testify/require"
	authenticationv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
	ktesting "k8s.io/client-go/testing"
)

func TestReviewInstallationPrincipal(t *testing.T) {
	for _, scenario := range []struct {
		name, user, group string
		groups            []string
		failure           error
		want              string
	}{
		{name: "user", user: "Alice", groups: []string{"operators"}},
		{name: "verified group", user: "Alice", groups: []string{"operators"}, group: "operators"},
		{name: "unverified group", user: "Alice", groups: []string{"operators"}, group: "admins", want: "not a verified membership"},
		{name: "anonymous", user: "system:anonymous", want: "anonymous principal"},
		{name: "missing identity", want: "authenticated human"},
		{name: "workload", user: "system:serviceaccount:radius-system:ci", want: "authenticated human"},
		{name: "unauthenticated", user: "Alice", groups: []string{"system:unauthenticated"}, want: "authenticated human"},
		{name: "review failure", failure: errors.New("review forbidden"), want: "review forbidden"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			client := fake.NewClientset()
			client.PrependReactor("create", "selfsubjectreviews", func(action ktesting.Action) (bool, runtime.Object, error) {
				review := action.(ktesting.CreateAction).GetObject().(*authenticationv1.SelfSubjectReview)
				require.Empty(t, review.Status.UserInfo, "the installer cannot assert its reviewed identity")
				return true, &authenticationv1.SelfSubjectReview{Status: authenticationv1.SelfSubjectReviewStatus{
					UserInfo: authenticationv1.UserInfo{Username: scenario.user, Groups: scenario.groups},
				}}, scenario.failure
			})
			principal, err := ReviewInstallationPrincipal(t.Context(), client.AuthenticationV1().SelfSubjectReviews(), "kubernetes:cluster-a", scenario.group)
			if scenario.want != "" {
				require.ErrorContains(t, err, scenario.want)
				require.Empty(t, principal)
				return
			}
			require.NoError(t, err)
			require.Equal(t, "kubernetes:cluster-a", *principal.Issuer)
			if scenario.group == "" {
				require.Equal(t, api.PrincipalTypeUser, *principal.Type)
				require.Equal(t, scenario.user, *principal.Subject)
			} else {
				require.Equal(t, api.PrincipalTypeGroup, *principal.Type)
				require.Equal(t, scenario.group, *principal.Subject)
			}
		})
	}
}

func TestReviewInstallationPrincipalRESTBoundary(t *testing.T) {
	for _, status := range []int{http.StatusCreated, http.StatusForbidden} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, http.MethodPost, r.Method)
				require.Equal(t, "/apis/authentication.k8s.io/v1/selfsubjectreviews", r.URL.Path)
				require.Empty(t, r.Header.Get("Impersonate-User"))
				var review authenticationv1.SelfSubjectReview
				require.NoError(t, json.NewDecoder(r.Body).Decode(&review))
				require.Empty(t, review.Status.UserInfo)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(status)
				if status != http.StatusCreated {
					require.NoError(t, json.NewEncoder(w).Encode(metav1.Status{
						TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Status"},
						Status:   "Failure", Code: int32(status), Reason: metav1.StatusReasonForbidden,
					}))
					return
				}
				require.NoError(t, json.NewEncoder(w).Encode(authenticationv1.SelfSubjectReview{
					TypeMeta: metav1.TypeMeta{APIVersion: "authentication.k8s.io/v1", Kind: "SelfSubjectReview"},
					Status: authenticationv1.SelfSubjectReviewStatus{
						UserInfo: authenticationv1.UserInfo{Username: "Alice", Groups: []string{"operators"}},
					},
				}))
			}))
			defer server.Close()
			client, err := kubernetes.NewForConfig(&rest.Config{
				Host: server.URL, ContentConfig: rest.ContentConfig{ContentType: runtime.ContentTypeJSON},
			})
			require.NoError(t, err)
			principal, err := ReviewInstallationPrincipal(t.Context(), client.AuthenticationV1().SelfSubjectReviews(), "kubernetes:cluster-a", "operators")
			if status == http.StatusCreated {
				require.NoError(t, err)
				require.Equal(t, api.PrincipalTypeGroup, *principal.Type)
				require.Equal(t, "operators", *principal.Subject)
			} else {
				require.ErrorContains(t, err, "review Kubernetes installation identity")
				require.Empty(t, principal)
			}
		})
	}
}

func TestInitializeKubernetesInstallation(t *testing.T) {
	for _, scenario := range []struct {
		name  string
		mode  api.UserAuthorizationMode
		admin []api.Principal
		want  string
	}{
		{name: "existing-install Disabled", mode: api.UserAuthorizationModeDisabled},
		{name: "new-install user", mode: api.UserAuthorizationModeDryRun, admin: []api.Principal{testIdentity().Principal}},
		{name: "new-install group", mode: api.UserAuthorizationModeDryRun, admin: []api.Principal{testPrincipal(api.PrincipalTypeGroup, "kubernetes:cluster-a", "operators")}},
		{name: "missing administrator", mode: api.UserAuthorizationModeDryRun, want: "administrators are required"},
		{name: "Disabled does not grant access", mode: api.UserAuthorizationModeDisabled, admin: []api.Principal{testIdentity().Principal}, want: "must not create"},
		{name: "foreign issuer", mode: api.UserAuthorizationModeDryRun, admin: []api.Principal{testPrincipal(api.PrincipalTypeUser, "kubernetes:other-cluster", "Alice")}, want: "configured Kubernetes issuer"},
		{name: "workload", mode: api.UserAuthorizationModeDryRun, admin: []api.Principal{testPrincipal(api.PrincipalTypeWorkload, "kubernetes:cluster-a", "ci")}, want: "user or group"},
		{name: "forged workload user", mode: api.UserAuthorizationModeDryRun, admin: []api.Principal{testPrincipal(api.PrincipalTypeUser, "kubernetes:cluster-a", "system:serviceaccount:radius-system:ci")}, want: "user or group"},
		{name: "no rollout bypass", mode: api.UserAuthorizationModeEnforce, admin: []api.Principal{testIdentity().Principal}, want: "AuthorizationUnavailable"},
		{name: "unknown mode", mode: api.UserAuthorizationMode("unknown"), want: "invalid installation"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			store := newTestPolicyStore(t, inmemory.NewClient(), false)
			err := InitializeKubernetesInstallation(t.Context(), store, "kubernetes:cluster-a", InstallationOptions{
				Mode: scenario.mode, Administrators: scenario.admin,
			})
			if scenario.want != "" {
				require.ErrorContains(t, err, scenario.want)
				_, err = store.LoadPolicy(t.Context(), -1)
				require.ErrorIs(t, err, ErrAuthorizationUnavailable)
				return
			}
			require.NoError(t, err)
			snapshot, err := store.LoadPolicy(t.Context(), -1)
			require.NoError(t, err)
			policy, err := snapshot.Policy()
			require.NoError(t, err)
			require.Zero(t, policy.Generation)
			require.Equal(t, scenario.mode, *policy.Settings.Properties.Mode)
			require.Len(t, policy.Assignments, len(scenario.admin))
			require.Zero(t, permanentAdministrators(policy))
			for i, assignment := range policy.Assignments {
				require.Equal(t, api.AssignmentManagementKindBootstrap, *assignment.Properties.ManagedBy.Kind)
				require.Equal(t, scenario.admin[i], *assignment.Properties.Principal)
				require.Equal(t, "/", *assignment.Properties.Scope)
				require.Equal(t, builtInID("radius-administrator"), *assignment.Properties.RoleDefinitionID)
			}
			// A later startup configuration can neither replace initial access
			// nor reset a persisted mode.
			require.NoError(t, InitializeKubernetesInstallation(t.Context(), store, "", InstallationOptions{Mode: api.UserAuthorizationModeEnforce}))
			unchanged, err := store.LoadPolicy(t.Context(), -1)
			require.NoError(t, err)
			after, err := unchanged.Policy()
			require.NoError(t, err)
			require.Equal(t, policy, after)
		})
	}
}

func TestInstallationStartupErrorsAndConcurrentPublication(t *testing.T) {
	t.Run("read failure is not an absent anchor", func(t *testing.T) {
		db := &runtimeDatabase{Client: inmemory.NewClient()}
		failure := errors.New("database unreachable")
		db.setGet(func(context.Context, string) error { return failure })
		store := newTestPolicyStore(t, db, false)
		require.ErrorIs(t, InitializeKubernetesInstallation(t.Context(), store, "kubernetes:cluster-a",
			InstallationOptions{Mode: api.UserAuthorizationModeDisabled}), failure)
		db.setGet(nil)
		_, err := store.LoadPolicy(t.Context(), -1)
		require.ErrorIs(t, err, ErrAuthorizationUnavailable)
	})
	t.Run("publication failure is not success", func(t *testing.T) {
		failure := errors.New("publication unavailable")
		store := newTestPolicyStore(t, &policyHookClient{Client: inmemory.NewClient(), before: func(_ context.Context, obj *database.Object) error {
			if obj.ID == accessVersionID {
				return failure
			}
			return nil
		}}, false)
		require.ErrorIs(t, InitializeKubernetesInstallation(t.Context(), store, "kubernetes:cluster-a",
			InstallationOptions{Mode: api.UserAuthorizationModeDisabled}), failure)
		_, err := store.LoadPolicy(t.Context(), -1)
		require.ErrorIs(t, err, ErrAuthorizationUnavailable)
	})
	t.Run("corrupt existing anchor is never replaced", func(t *testing.T) {
		db := inmemory.NewClient()
		require.NoError(t, db.Save(t.Context(), &database.Object{
			Metadata: database.Metadata{ID: accessVersionID},
			Data:     map[string]any{"generation": 0, "root": "/missing-policy-root"},
		}))
		before, err := db.Get(t.Context(), accessVersionID)
		require.NoError(t, err)
		store := newTestPolicyStore(t, db, false)
		require.Error(t, InitializeKubernetesInstallation(t.Context(), store, "kubernetes:cluster-a",
			InstallationOptions{Mode: api.UserAuthorizationModeDisabled}))
		after, err := db.Get(t.Context(), accessVersionID)
		require.NoError(t, err)
		require.Equal(t, before.ETag, after.ETag)
		require.Equal(t, before.Data, after.Data)
	})
	t.Run("concurrent startup does not overwrite", func(t *testing.T) {
		arrived := make(chan struct{}, 2)
		release := make(chan struct{})
		db := &policyHookClient{Client: inmemory.NewClient(), before: func(ctx context.Context, obj *database.Object) error {
			if obj.ID == accessVersionID {
				arrived <- struct{}{}
				select {
				case <-release:
				case <-ctx.Done():
					return ctx.Err()
				}
			}
			return nil
		}}
		store := newTestPolicyStore(t, db, false)
		errors := make(chan error, 2)
		var workers sync.WaitGroup
		for _, name := range []string{"alice", "bob"} {
			workers.Go(func() {
				errors <- InitializeKubernetesInstallation(t.Context(), store, "kubernetes:cluster-a", InstallationOptions{
					Mode:           api.UserAuthorizationModeDryRun,
					Administrators: []api.Principal{testPrincipal(api.PrincipalTypeUser, "kubernetes:cluster-a", name)},
				})
			})
		}
		<-arrived
		<-arrived
		close(release)
		workers.Wait()
		require.NoError(t, <-errors)
		require.NoError(t, <-errors)
		snapshot, err := store.LoadPolicy(t.Context(), -1)
		require.NoError(t, err)
		policy, err := snapshot.Policy()
		require.NoError(t, err)
		require.Len(t, policy.Assignments, 1)
		require.Contains(t, []string{"alice", "bob"}, *policy.Assignments[0].Properties.Principal.Subject)
		require.Equal(t, to.Ptr(int64(0)), policy.Settings.Properties.Generation)
	})
}
