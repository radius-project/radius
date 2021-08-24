// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v1alpha1

import (
	"encoding/json"

	"github.com/Azure/radius/pkg/radrp/schema"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// log is for logging in this package.
var componentlog = logf.Log.WithName("component-resource")

func (r *Component) SetupWebhookWithManager(mgr ctrl.Manager) error {
	componentlog.Info("setup webhook", "name", r.Name)
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

//+kubebuilder:webhook:path=/validate-radius-dev-v1alpha1-component,mutating=false,failurePolicy=fail,sideEffects=None,groups=radius.dev,resources=components,verbs=create;update;delete,versions=v1alpha1,name=component-validation.radius.dev,admissionReviewVersions={v1,v1beta1}

var _ webhook.Validator = &Component{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *Component) ValidateCreate() error {
	componentlog.Info("validate create", "name", r.Name)

	return validate(r)
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *Component) ValidateUpdate(old runtime.Object) error {
	componentlog.Info("validate update", "name", r.Name)

	return validate(r)
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *Component) ValidateDelete() error {
	componentlog.Info("validate delete", "name", r.Name)

	// TODO(user): fill in your validation logic upon object deletion.
	return validate(r)
}

func validate(r *Component) error {

	// HACK: currently we expect kind and hierarchy to be empty
	// when doing json schema validation as the model doesn't quite fit
	specCopy := r.Spec.DeepCopy()
	specCopy.Hierarchy = nil
	specCopy.Kind = ""
	hackedJson := map[string]interface{}{
		"kind":       specCopy.Kind,
		"properties": specCopy,
	}

	data, err := json.Marshal(hackedJson)
	if err != nil {
		return err
	}

	// k8s model mirrors the component properties in the schema,
	// except kind and hierarchy, which we validate separately.

	validator := schema.GetComponentValidator()
	componentlog.Info("json payload", "json", string(data))

	if errs := validator.ValidateJSON(data); len(errs) != 0 {
		return &schema.AggregateValidationError{
			Details: errs,
		}
	}
	return nil
}
