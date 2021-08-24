// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v1alpha1

import (
	"encoding/json"
	"fmt"

	"github.com/Azure/radius/pkg/cli/armtemplate"
	"github.com/Azure/radius/pkg/radrp/schema"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
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
//+kubebuilder:webhook:path=/validate-radius-dev-v1alpha1-arm,mutating=false,failurePolicy=fail,sideEffects=None,groups=radius.dev,resources=arms,verbs=create;update;delete,versions=v1alpha1,name=varm.radius.dev,admissionReviewVersions={v1,v1beta1}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *Arm) ValidateCreate() error {
	armlog.Info("validate create", "name", r.Name)

	template, err := armtemplate.Parse(r.Spec.Content)
	if err != nil {
		return err
	}

	resources, err := armtemplate.Eval(template, armtemplate.TemplateOptions{})
	if err != nil {
		return err
	}

	for _, resource := range resources {
		data, err := json.Marshal(resource)
		if err != nil {
			return err
		}

		validator, err := schema.ValidatorFor(resource)
		if err != nil {
			return fmt.Errorf("cannot find validator for %T: %w", resource, err)
		}
		if errs := validator.ValidateJSON(data); len(errs) != 0 {
			return &schema.AggregateValidationError{
				Details: errs,
			}
		}
	}
	return nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *Arm) ValidateUpdate(old runtime.Object) error {
	armlog.Info("validate update", "name", r.Name)

	// TODO(user): fill in your validation logic upon object update.
	return nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *Arm) ValidateDelete() error {
	armlog.Info("validate delete", "name", r.Name)

	// TODO(user): fill in your validation logic upon object deletion.
	return nil
}
