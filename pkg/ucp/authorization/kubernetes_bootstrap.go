// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

package authorization

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/radius-project/radius/pkg/components/database"
	api "github.com/radius-project/radius/pkg/ucp/api/authorization/v20250801preview"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
	authenticationv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	typedauthenticationv1 "k8s.io/client-go/kubernetes/typed/authentication/v1"
)

// InstallationOptions are trusted installation input, not a public request.
// Installers obtain administrators using ReviewInstallationPrincipal before
// publishing Helm values. Existing policy always takes precedence at startup.
type InstallationOptions struct {
	Mode           api.UserAuthorizationMode `yaml:"mode"`
	Administrators []api.Principal           `yaml:"administrators"`
}

// ReviewInstallationPrincipal obtains the installer's identity directly from
// Kubernetes. A requested group must occur in that verified identity; it cannot
// be supplied as an asserted membership. No generic Kubernetes access is treated
// as Radius authority.
func ReviewInstallationPrincipal(ctx context.Context, reviews typedauthenticationv1.SelfSubjectReviewInterface, issuer, group string) (api.Principal, error) {
	if reviews == nil || !validKubernetesIssuer(issuer) {
		return api.Principal{}, fmt.Errorf("installation identity requires a SelfSubjectReview client and installation-qualified issuer")
	}
	review, err := reviews.Create(ctx, &authenticationv1.SelfSubjectReview{}, metav1.CreateOptions{})
	if err != nil {
		return api.Principal{}, fmt.Errorf("review Kubernetes installation identity: %w", err)
	}
	if review == nil {
		return api.Principal{}, fmt.Errorf("Kubernetes installation identity review returned no identity")
	}
	info := review.Status.UserInfo
	if strings.TrimSpace(info.Username) == "" || strings.HasPrefix(info.Username, "system:serviceaccount:") ||
		slices.Contains(info.Groups, "system:unauthenticated") {
		return api.Principal{}, fmt.Errorf("installation bootstrap requires an authenticated human identity")
	}
	principal := api.Principal{Type: to.Ptr(api.PrincipalTypeUser), Issuer: to.Ptr(issuer), Subject: to.Ptr(info.Username)}
	if _, err := key(&principal); err != nil {
		return api.Principal{}, err
	}
	if group != "" {
		if !slices.Contains(info.Groups, group) {
			return api.Principal{}, fmt.Errorf("installation bootstrap group is not a verified membership")
		}
		principal.Type, principal.Subject = to.Ptr(api.PrincipalTypeGroup), to.Ptr(group)
	}
	if err := validateManagementPrincipal(&principal); err != nil {
		return api.Principal{}, err
	}
	return principal, nil
}

func validKubernetesIssuer(issuer string) bool {
	return strings.HasPrefix(issuer, "kubernetes:") &&
		strings.TrimSpace(strings.TrimPrefix(issuer, "kubernetes:")) != "" &&
		!strings.ContainsAny(issuer, " \t\r\n")
}

type installationVerifier struct{ expected Policy }

func (v installationVerifier) VerifyPolicyInitialization(_ context.Context, policy Policy) error {
	if !reflect.DeepEqual(policy, v.expected) {
		return fmt.Errorf("installation policy does not match trusted startup configuration")
	}
	return nil
}

// InitializeKubernetesInstallation runs only at the trusted startup boundary.
// The Helm-installed configuration is authoritative for the first publication,
// never a flag or body on a public API. It does not repair or replace policy on
// restart, upgrade, corruption, or an arbitrary backend error.
func InitializeKubernetesInstallation(ctx context.Context, store *PolicyStore, issuer string, options InstallationOptions) error {
	if store == nil {
		return fmt.Errorf("installation policy store is required")
	}
	var anchor accessVersion
	if _, err := store.read(ctx, accessVersionID, &anchor); err == nil {
		_, err = store.LoadPolicy(ctx, -1)
		return err
	} else {
		var missing *database.ErrNotFound
		if !errors.As(err, &missing) {
			return err
		}
	}
	if !validKubernetesIssuer(issuer) {
		return fmt.Errorf("installation bootstrap requires the configured Kubernetes issuer")
	}
	switch options.Mode {
	case api.UserAuthorizationModeDisabled:
		if len(options.Administrators) != 0 {
			return fmt.Errorf("Disabled initialization must not create administrator assignments")
		}
	case api.UserAuthorizationModeDryRun:
		if len(options.Administrators) == 0 {
			return fmt.Errorf("installation bootstrap administrators are required")
		}
	case api.UserAuthorizationModeEnforce:
		return fmt.Errorf("%w: enforcement rollout guard is not installed", ErrAuthorizationUnavailable)
	default:
		return fmt.Errorf("invalid installation authorization mode")
	}
	administrators, err := clonePolicyValue(options.Administrators)
	if err != nil {
		return err
	}
	for _, principal := range administrators {
		if err := validateManagementPrincipal(&principal); err != nil {
			return err
		}
		if *principal.Issuer != issuer || *principal.Type != api.PrincipalTypeUser && *principal.Type != api.PrincipalTypeGroup ||
			strings.HasPrefix(*principal.Subject, "system:serviceaccount:") {
			return fmt.Errorf("installation administrator must be a user or group in the configured Kubernetes issuer")
		}
	}
	initial := Policy{Settings: api.AuthorizationSettingsResource{
		ID: to.Ptr(settingsID), Properties: &api.AuthorizationSettingsProperties{Mode: &options.Mode, Generation: to.Ptr(int64(0))},
	}}
	if len(administrators) != 0 {
		actor := PolicyActor{Identity: Identity{Principal: administrators[0]}}
		if err := addBootstrapAdministrators(&initial, administrators, actor); err != nil {
			return err
		}
		for i := range initial.Assignments {
			initial.Assignments[i].Properties.CreatedBy = to.Ptr("installation")
		}
	}
	expected, err := clonePolicyValue(initial)
	if err != nil {
		return err
	}
	initializer, err := NewPolicyInitializer(store, installationVerifier{expected: expected})
	if err != nil {
		return err
	}
	if err := initializer.Initialize(ctx, initial); err != nil {
		var conflict *database.ErrConcurrency
		if !errors.As(err, &conflict) {
			return err
		}
		// Another initializer won the create-only publication. Read its complete
		// policy, never overwrite it with this instance's startup configuration.
		_, err = store.LoadPolicy(ctx, -1)
		return err
	}
	ucplog.FromContextOrDiscard(ctx).WithName("radius.audit").Info("User authorization installation initialized",
		"generation", 0, "mode", options.Mode, "bootstrapAdministrators", administrators)
	return nil
}
