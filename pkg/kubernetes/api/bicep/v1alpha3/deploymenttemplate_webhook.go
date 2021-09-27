// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v1alpha3

import (
	"errors"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// log is for logging in this package.
var armlog = logf.Log.WithName("deploymenttemplate-resource")

func (r *DeploymentTemplate) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

var _ webhook.Validator = &DeploymentTemplate{}

//+kubebuilder:webhook:path=/validate-bicep-dev-v1alpha3-deploymenttemplate,mutating=false,failurePolicy=fail,sideEffects=None,groups=bicep.dev,resources=deploymenttemplates,verbs=create;update;delete,versions=v1alpha3,name=deploymenttemplate-validator.bicep.dev,admissionReviewVersions={v1,v1beta1}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *DeploymentTemplate) ValidateCreate() error {
	armlog.Info("validate create", "name", r.Name)

	return errors.New("Wow")
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *DeploymentTemplate) ValidateUpdate(old runtime.Object) error {
	armlog.Info("validate update", "name", r.Name)

	return nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *DeploymentTemplate) ValidateDelete() error {
	armlog.Info("validate delete", "name", r.Name)

	return nil
}
