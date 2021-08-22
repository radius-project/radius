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
var armlog = logf.Log.WithName("arm-resource")

func (r *Arm) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
//+kubebuilder:webhook:path=/validate-radius-radius-dev-v1alpha1-deployment,mutating=false,failurePolicy=fail,sideEffects=None,groups=radius.dev,resources=deployments,verbs=create;update;delete,versions=v1alpha1,name=vdeployment.radius.dev,admissionReviewVersions={v1,v1beta1}

var _ webhook.Validator = &Deployment{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *Arm) ValidateCreate() error {
	deploymentlog.Info("validate create", "name", r.Name)

	// TODO(user): fill in your validation logic upon object creation.
	return nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *Arm) ValidateUpdate(old runtime.Object) error {
	deploymentlog.Info("validate update", "name", r.Name)

	// TODO(user): fill in your validation logic upon object update.
	return nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *Arm) ValidateDelete() error {
	deploymentlog.Info("validate delete", "name", r.Name)

	// TODO(user): fill in your validation logic upon object deletion.
	return nil
}
