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
var resourcelog = logf.Log.WithName("resource")

func (r *Resource) SetupWebhookWithManager(mgr ctrl.Manager) error {
	resourcelog.Info("setup webhook", "name", r.Name)
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

//+kubebuilder:webhook:path=/validate-radius-dev-v1alpha1-resource,mutating=false,failurePolicy=fail,sideEffects=None,groups=radius.dev,resources=resources,verbs=create;update;delete,versions=v1alpha1,name=resource-validation.radius.dev,admissionReviewVersions={v1,v1beta1}

var _ webhook.Validator = &Resource{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *Resource) ValidateCreate() error {
	resourcelog.Info("validate create", "name", r.Name)

	return validate(r)
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *Resource) ValidateUpdate(old runtime.Object) error {
	resourcelog.Info("validate update", "name", r.Name)

	return validate(r)
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *Resource) ValidateDelete() error {
	resourcelog.Info("validate delete", "name", r.Name)

	return nil
}

func validate(r *Resource) error {

	// // HACK: currently we expect kind and hierarchy to be empty
	// // when doing json schema validation as the model doesn't quite fit
	// specCopy := r.Spec.DeepCopy()
	// // specCopy.Hierarchy = nil
	// // specCopy.Kind = ""
	// hackedJson := map[string]interface{}{
	// 	// "kind":       r.Spec.Kind,
	// 	"properties": specCopy,
	// }

	// data, err := json.Marshal(hackedJson)
	// if err != nil {
	// 	return err
	// }

	// // k8s model mirrors the resource properties in the schema,
	// // except kind and hierarchy, which we validate separately.

	// validator := schema.GetResourceValidator()
	// resourcelog.Info("json payload", "json", string(data))

	// if errs := validator.ValidateJSON(data); len(errs) != 0 {
	// 	return &schema.AggregateValidationError{
	// 		Details: errs,
	// 	}
	// }
	return nil
}
