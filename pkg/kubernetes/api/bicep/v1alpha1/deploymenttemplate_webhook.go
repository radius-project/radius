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

//+kubebuilder:webhook:path=/validate-bicep-dev-v1alpha1-deploymenttemplate,mutating=false,failurePolicy=fail,sideEffects=None,groups=bicep.dev,resources=deploymenttemplates,verbs=create;update;delete,versions=v1alpha1,name=deploymenttemplate-validator.bicep.dev,admissionReviewVersions={v1,v1beta1}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *DeploymentTemplate) ValidateCreate() error {
	armlog.Info("validate create", "name", r.Name)

	return validateArm(r)
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *DeploymentTemplate) ValidateUpdate(old runtime.Object) error {
	armlog.Info("validate update", "name", r.Name)

	return validateArm(r)
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *DeploymentTemplate) ValidateDelete() error {
	armlog.Info("validate delete", "name", r.Name)

	return nil
}

func validateArm(r *DeploymentTemplate) error {
	template, err := armtemplate.Parse(string(r.Spec.Content.Raw))
	if err != nil {
		return err
	}

	resources, err := armtemplate.Eval(template, armtemplate.TemplateOptions{})
	if err != nil {
		return err
	}

	for _, resource := range resources {
		data, err := json.Marshal(resource.Body)
		if err != nil {
			return err
		}

		validator, err := schema.ValidatorForArmTemplate(resource.Type)
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
