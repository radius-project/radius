// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

// Package userauth wires the experimental user RBAC API into the UCP listener.
package userauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/google/uuid"
	"github.com/radius-project/radius/pkg/authz"
	"github.com/radius-project/radius/pkg/components/database"
	api "github.com/radius-project/radius/pkg/ucp/api/authorization/v20250801preview"
	"github.com/radius-project/radius/pkg/ucp/authorization"
	"github.com/radius-project/radius/pkg/ucp/authzcheck"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
	authv1 "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const settingsPath = "/providers/radius.core/authorizationsettings/default"
const initializePath = "/providers/radius.core/initializeauthorization"

// Options explicitly enables the prototype. Existing installations are unaffected.
type Options struct {
	Enabled bool `yaml:"enabled"`
	// KubernetesIssuer must identify this cluster, not just "kubernetes".
	KubernetesIssuer string `yaml:"kubernetesIssuer"`
	// AcceptIncompleteHistory is a visible POC-only waiver, not a production default.
	AcceptIncompleteHistory bool `yaml:"acceptIncompleteHistory"`
}

// Runtime owns the policy store and cache for one UCP frontend.
type Runtime struct {
	Options       Options
	PathBase      string
	DB            database.Client
	Catalog       *authorization.Catalog
	Store         *authorization.PolicyStore
	Cache         *authorization.PolicyCache
	Kube          kubernetes.Interface
	ComponentMode authz.Mode
}

// New constructs the runtime without creating policy or launching goroutines.
func New(db database.Client, client kubernetes.Interface, options Options, pathBase string, mode authz.Mode) (*Runtime, error) {
	if !strings.HasPrefix(options.KubernetesIssuer, "kubernetes:") || strings.TrimPrefix(options.KubernetesIssuer, "kubernetes:") == "" {
		return nil, fmt.Errorf("userAuthorization.kubernetesIssuer must include a cluster trust domain")
	}
	if client == nil || mode == authz.ModeOff {
		return nil, fmt.Errorf("user RBAC requires Kubernetes and authenticated internal transport")
	}
	catalog, err := authorization.NewCatalog(nil)
	if err != nil {
		return nil, err
	}
	r := &Runtime{Options: options, PathBase: strings.TrimSuffix(pathBase, "/"), DB: db, Catalog: catalog, Kube: client, ComponentMode: mode}
	r.Store, err = authorization.NewPolicyStore(db, catalog, nil, r)
	if err != nil {
		return nil, err
	}
	r.Cache, err = authorization.NewPolicyCache(r.Store, nil)
	if err != nil {
		return nil, err
	}
	return r, nil
}

// Run is joined by the frontend owner when its server stops.
func (r *Runtime) Run(ctx context.Context) error {
	return r.Cache.Run(ctx, func(err error) {
		ucplog.FromContextOrDiscard(ctx).Error(err, "user policy refresh failed")
	})
}

func (r *Runtime) identity(ctx context.Context) (authorization.Identity, error) {
	caller, ok := authzcheck.CallerFromContext(ctx)
	if !ok || caller.Kind != authzcheck.CallerUser || !caller.VerifiedProxy || caller.Name == "" ||
		caller.Name == "system:anonymous" || caller.Name == "system:unauthenticated" {
		return authorization.Identity{}, fmt.Errorf("InvalidAuthenticationInfo: verified Kubernetes principal required")
	}
	typ := api.PrincipalTypeUser
	if strings.HasPrefix(caller.Name, "system:serviceaccount:") {
		typ = api.PrincipalTypeWorkload
	}
	principal := func(typ api.PrincipalType, subject string) api.Principal {
		return api.Principal{Type: to.Ptr(typ), Issuer: to.Ptr(r.Options.KubernetesIssuer), Subject: to.Ptr(subject)}
	}
	id := authorization.Identity{Principal: principal(typ, caller.Name)}
	for _, group := range caller.Groups {
		if group == "" || group == "system:unauthenticated" {
			return authorization.Identity{}, fmt.Errorf("InvalidAuthenticationInfo: anonymous group")
		}
		id.Groups = append(id.Groups, principal(api.PrincipalTypeGroup, group))
	}
	return id, nil
}

func (r *Runtime) bootstrapAllowed(ctx context.Context, identity authorization.Identity) error {
	groups := make([]string, 0, len(identity.Groups))
	for _, g := range identity.Groups {
		groups = append(groups, *g.Subject)
	}
	review, err := r.Kube.AuthorizationV1().SubjectAccessReviews().Create(ctx, &authv1.SubjectAccessReview{
		Spec: authv1.SubjectAccessReviewSpec{
			User: *identity.Principal.Subject, Groups: groups,
			ResourceAttributes: &authv1.ResourceAttributes{Group: "api.ucp.dev", Resource: "authorizationsettings", Name: "default", Verb: "update"},
		},
	}, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("AuthorizationUnavailable: bootstrap SubjectAccessReview: %w", err)
	}
	if !review.Status.Allowed || review.Status.Denied || review.Status.EvaluationError != "" {
		return fmt.Errorf("AuthorizationFailed: Kubernetes bootstrap permission required")
	}
	return nil
}

// ValidateSettingsChange implements the store's trusted settings guard.
func (r *Runtime) ValidateSettingsChange(ctx context.Context, actor authorization.PolicyActor, before, after api.AuthorizationSettingsProperties) error {
	if after.Mode == nil {
		return fmt.Errorf("Invalid: mode is required")
	}
	if *before.Mode != api.UserAuthorizationModeEnforce {
		if err := r.bootstrapAllowed(ctx, actor.Identity); err != nil {
			return err
		}
	}
	if *after.Mode == api.UserAuthorizationModeEnforce && r.ComponentMode != authz.ModeEnforce {
		return fmt.Errorf("ComponentProtocolNotEnforced: internal authorization must be enforce")
	}
	if *after.Mode == api.UserAuthorizationModeEnforce && *before.Mode != api.UserAuthorizationModeEnforce {
		if !r.Options.AcceptIncompleteHistory {
			return fmt.Errorf("DryRunHistoryIncomplete: prototype has no durable 24-hour history; explicit installation waiver required")
		}
		ucplog.FromContextOrDiscard(ctx).Info("user RBAC history waiver used", "highImpact", true, "acceptIncompleteHistory", true, "principal", *actor.Identity.Principal.Subject)
	}
	if *before.Mode != api.UserAuthorizationModeDisabled && *after.Mode == api.UserAuthorizationModeDisabled {
		return fmt.Errorf("AuthorizationFailed: returning to Disabled requires recovery (not implemented in this POC)")
	}
	return nil
}

type initializationVerifier struct {
	runtime  *Runtime
	identity authorization.Identity
}

func (v initializationVerifier) VerifyPolicyInitialization(ctx context.Context, policy authorization.Policy) error {
	if err := v.runtime.bootstrapAllowed(ctx, v.identity); err != nil {
		return err
	}
	if policy.Settings.Properties.Mode == nil || *policy.Settings.Properties.Mode != api.UserAuthorizationModeDisabled || len(policy.Assignments) != 1 {
		return fmt.Errorf("invalid initial bootstrap policy")
	}
	return nil
}

func (r *Runtime) initialize(w http.ResponseWriter, req *http.Request, identity authorization.Identity) {
	initializer, err := authorization.NewPolicyInitializer(r.Store, initializationVerifier{r, identity})
	if err != nil {
		fail(w, err)
		return
	}
	assignmentID := "/providers/Radius.Core/roleAssignments/bootstrap-" + uuid.NewString()
	creator := *identity.Principal.Subject
	policy := authorization.Policy{
		Settings: api.AuthorizationSettingsResource{ID: to.Ptr(settingsPath), Properties: &api.AuthorizationSettingsProperties{Mode: to.Ptr(api.UserAuthorizationModeDisabled), Generation: to.Ptr(int64(0))}},
		Assignments: []api.RoleAssignmentResource{{
			ID: to.Ptr(assignmentID),
			Properties: &api.RoleAssignmentProperties{
				Principal: &identity.Principal, RoleDefinitionID: to.Ptr("/providers/Radius.Core/roleDefinitions/radius-administrator"),
				Scope: to.Ptr("/"), CreatedBy: &creator,
				ManagedBy: &api.AssignmentManagement{Kind: to.Ptr(api.AssignmentManagementKindBootstrap)},
			},
		}},
	}
	if err := initializer.Initialize(req.Context(), policy); err != nil {
		fail(w, err)
		return
	}
	if err := r.Cache.Refresh(req.Context()); err != nil {
		fail(w, err)
		return
	}
	ucplog.FromContextOrDiscard(req.Context()).Info("user RBAC explicitly initialized", "principal", creator, "highImpact", true)
	write(w, http.StatusCreated, policy.Settings)
}

func write(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		// A disconnected client cannot be notified through the response.
		ucplog.FromContextOrDiscard(context.Background()).Error(err, "write user authorization response")
	}
}

func fail(w http.ResponseWriter, err error) {
	status, code := http.StatusBadRequest, "Invalid"
	var conflict *database.ErrConcurrency
	switch {
	case errors.Is(err, authorization.ErrAuthorizationUnavailable):
		status, code = http.StatusServiceUnavailable, "AuthorizationUnavailable"
	case errors.Is(err, &database.ErrNotFound{}):
		status, code = http.StatusNotFound, "NotFound"
	case errors.As(err, &conflict), errors.Is(err, authorization.ErrPolicyConflict):
		status, code = http.StatusConflict, "Conflict"
	default:
		for _, c := range []string{"InvalidAuthenticationInfo", "AuthorizationFailed", "LastAdministratorProtected", "PrivilegeEscalationDenied", "ManagedByConflict", "BuiltInRoleImmutable", "ScopeNotAssignable", "RoleAssignmentImmutable", "RoleDefinitionInUse", "ComponentProtocolNotEnforced", "DryRunHistoryIncomplete", "AuthorizationIndeterminate", "AuthorizationUnavailable"} {
			if strings.Contains(err.Error(), c) {
				code = c
				status = http.StatusForbidden
				if c == "InvalidAuthenticationInfo" {
					status = http.StatusUnauthorized
				} else if c == "AuthorizationUnavailable" || c == "AuthorizationIndeterminate" {
					status = http.StatusServiceUnavailable
				}
				break
			}
		}
	}
	write(w, status, map[string]any{"error": map[string]string{"code": code, "message": err.Error()}})
}
