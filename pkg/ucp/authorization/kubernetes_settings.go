// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

package authorization

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	api "github.com/radius-project/radius/pkg/ucp/api/authorization/v20250801preview"
	authorizationv1 "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	typedauthorizationv1 "k8s.io/client-go/kubernetes/typed/authorization/v1"
)

// KubernetesSettingsGuard authorizes bootstrap settings changes using the
// authenticated caller, never UCP's own Kubernetes access. Rollout prerequisites
// are delegated separately; without that dependency Enforce transitions fail.
type KubernetesSettingsGuard struct {
	reviews typedauthorizationv1.SubjectAccessReviewInterface
	issuer  string
	rollout SettingsChangeGuard
}

// NewKubernetesSettingsGuard requires a real Kubernetes SAR client and the same
// installation issuer as the authenticating proxy boundary. A nil rollout guard
// permits only Disabled/DryRun changes, not production enforcement transitions.
func NewKubernetesSettingsGuard(reviews typedauthorizationv1.SubjectAccessReviewInterface, issuer string, rollout SettingsChangeGuard) (*KubernetesSettingsGuard, error) {
	if reviews == nil || !strings.HasPrefix(issuer, "kubernetes:") ||
		strings.TrimSpace(strings.TrimPrefix(issuer, "kubernetes:")) == "" ||
		strings.ContainsAny(issuer, " \t\r\n") {
		return nil, fmt.Errorf("Kubernetes settings require a SAR client and installation-qualified issuer")
	}
	return &KubernetesSettingsGuard{reviews: reviews, issuer: issuer, rollout: rollout}, nil
}

func (g *KubernetesSettingsGuard) authorize(ctx context.Context, actor PolicyActor) error {
	identity, verified := IdentityFromContext(ctx)
	if !verified || !reflect.DeepEqual(identity, actor.Identity) {
		return &policyAccessError{permission: "Kubernetes bootstrap authorization", target: settingsID, source: "Kubernetes"}
	}
	principal, err := key(&identity.Principal)
	if err != nil {
		return err
	}
	if principal.issuer != g.issuer || principal.typ != api.PrincipalTypeUser {
		return &policyAccessError{permission: "Kubernetes human bootstrap identity", target: settingsID, source: "Kubernetes"}
	}
	spec := authorizationv1.SubjectAccessReviewSpec{
		User: principal.subject, UID: identity.KubernetesUID,
		Extra: make(map[string]authorizationv1.ExtraValue, len(identity.KubernetesExtra)),
		ResourceAttributes: &authorizationv1.ResourceAttributes{
			Verb: "update", Group: "api.ucp.dev", Resource: "authorizationsettings", Name: "default",
		},
	}
	for _, group := range identity.Groups {
		k, err := key(&group)
		if err != nil || k.typ != api.PrincipalTypeGroup || k.issuer != g.issuer {
			return fmt.Errorf("invalid verified Kubernetes group identity")
		}
		spec.Groups = append(spec.Groups, k.subject)
	}
	for name, values := range identity.KubernetesExtra {
		spec.Extra[name] = append(authorizationv1.ExtraValue{}, values...)
	}
	review, err := g.reviews.Create(ctx, &authorizationv1.SubjectAccessReview{Spec: spec}, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("%w: Kubernetes bootstrap SubjectAccessReview: %w", ErrAuthorizationUnavailable, err)
	}
	if review == nil || review.Status.EvaluationError != "" {
		return fmt.Errorf("%w: Kubernetes bootstrap SubjectAccessReview was indeterminate", ErrAuthorizationUnavailable)
	}
	if !review.Status.Allowed || review.Status.Denied {
		return &policyAccessError{permission: "Kubernetes update api.ucp.dev/authorizationsettings/default", target: settingsID, source: "Kubernetes"}
	}
	return nil
}

// ValidateSettingsChange repeats the Kubernetes authorization on every store
// retry, including unchanged PUTs. Generic access to the aggregated API does not
// grant bootstrap authority.
func (g *KubernetesSettingsGuard) ValidateSettingsChange(ctx context.Context, actor PolicyActor, before, after api.AuthorizationSettingsProperties) error {
	if before.Mode == nil || after.Mode == nil {
		return fmt.Errorf("authorization mode is required")
	}
	switch *before.Mode {
	case api.UserAuthorizationModeDisabled, api.UserAuthorizationModeDryRun:
		if err := g.authorize(ctx, actor); err != nil {
			return err
		}
	case api.UserAuthorizationModeEnforce:
	default:
		return fmt.Errorf("invalid current authorization mode")
	}
	switch *after.Mode {
	case api.UserAuthorizationModeDisabled, api.UserAuthorizationModeDryRun, api.UserAuthorizationModeEnforce:
	default:
		return fmt.Errorf("invalid requested authorization mode")
	}
	if *before.Mode == api.UserAuthorizationModeEnforce || *after.Mode == api.UserAuthorizationModeEnforce {
		if g.rollout == nil {
			return fmt.Errorf("%w: enforcement rollout guard is not installed", ErrAuthorizationUnavailable)
		}
		return g.rollout.ValidateSettingsChange(ctx, actor, before, after)
	}
	return nil
}

// BootstrapAdministrators supplies only the Kubernetes-authorized caller. Group
// claims are SAR inputs, not permission to install every group as an admin.
func (g *KubernetesSettingsGuard) BootstrapAdministrators(ctx context.Context, actor PolicyActor, before, after api.AuthorizationSettingsProperties) ([]api.Principal, error) {
	if err := g.authorize(ctx, actor); err != nil {
		return nil, err
	}
	if after.Mode == nil || *after.Mode == api.UserAuthorizationModeDisabled {
		return nil, nil
	}
	return []api.Principal{actor.Identity.Principal}, nil
}
