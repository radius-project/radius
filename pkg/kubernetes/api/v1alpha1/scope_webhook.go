// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v1alpha1

import (
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// log is for logging in this package.
var scopelog = logf.Log.WithName("scope-resource")

func (r *Scope) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

//+kubebuilder:webhook:path=/mutate-radius-radius-dev-v1alpha1-scope,mutating=true,failurePolicy=fail,sideEffects=None,groups=applications.radius.dev,resources=scopes,verbs=create;update,versions=v1alpha1,name=mscope.kb.io,admissionReviewVersions={v1,v1beta1}

var _ webhook.Defaulter = &Scope{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *Scope) Default() {
	scopelog.Info("default", "name", r.Name)

	// TODO(user): fill in your defaulting logic.
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
//+kubebuilder:webhook:path=/validate-radius-radius-dev-v1alpha1-scope,mutating=false,failurePolicy=fail,sideEffects=None,groups=applications.radius.dev,resources=scopes,verbs=create;update,versions=v1alpha1,name=vscope.kb.io,admissionReviewVersions={v1,v1beta1}

var _ webhook.Validator = &Scope{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *Scope) ValidateCreate() error {
	scopelog.Info("validate create", "name", r.Name)

	// TODO(user): fill in your validation logic upon object creation.
	return nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *Scope) ValidateUpdate(old runtime.Object) error {
	scopelog.Info("validate update", "name", r.Name)

	// TODO(user): fill in your validation logic upon object update.
	return nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *Scope) ValidateDelete() error {
	scopelog.Info("validate delete", "name", r.Name)

	// TODO(user): fill in your validation logic upon object deletion.
	return nil
}
