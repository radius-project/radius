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
var deploymentlog = logf.Log.WithName("deployment-resource")

func (r *Deployment) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

//+kubebuilder:webhook:path=/mutate-radius-radius-dev-v1alpha1-deployment,mutating=true,failurePolicy=fail,sideEffects=None,groups=radius.dev,resources=deployments,verbs=create;update,versions=v1alpha1,name=mdeployment.kb.io,admissionReviewVersions={v1,v1beta1}

var _ webhook.Defaulter = &Deployment{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *Deployment) Default() {
	deploymentlog.Info("default", "name", r.Name)

	// TODO(user): fill in your defaulting logic.
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
//+kubebuilder:webhook:path=/validate-radius-radius-dev-v1alpha1-deployment,mutating=false,failurePolicy=fail,sideEffects=None,groups=radius.dev,resources=deployments,verbs=create;update,versions=v1alpha1,name=vdeployment.kb.io,admissionReviewVersions={v1,v1beta1}

var _ webhook.Validator = &Deployment{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *Deployment) ValidateCreate() error {
	deploymentlog.Info("validate create", "name", r.Name)

	// TODO(user): fill in your validation logic upon object creation.
	return nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *Deployment) ValidateUpdate(old runtime.Object) error {
	deploymentlog.Info("validate update", "name", r.Name)

	// TODO(user): fill in your validation logic upon object update.
	return nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *Deployment) ValidateDelete() error {
	deploymentlog.Info("validate delete", "name", r.Name)

	// TODO(user): fill in your validation logic upon object deletion.
	return nil
}
