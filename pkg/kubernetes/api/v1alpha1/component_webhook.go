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
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

//+kubebuilder:webhook:path=/validate-radius-radius-dev-v1alpha1-component,mutating=false,failurePolicy=fail,sideEffects=None,groups=radius.dev,resources=components,verbs=create;update;delete,versions=v1alpha1,name=vcomponent.radius.dev,admissionReviewVersions={v1,v1beta1}

var _ webhook.Validator = &Component{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *Component) ValidateCreate() error {
	componentlog.Info("validate create", "name", r.Name)

	data, err := json.Marshal(r.Spec)
	if err != nil {
		return err
	}
	validator := schema.ComponentValidator()
	if errs := validator.ValidateJSON(data); len(errs) != 0 {
		return errs[0].JSONError
	}

	return nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *Component) ValidateUpdate(old runtime.Object) error {
	componentlog.Info("validate update", "name", r.Name)

	// TODO(user): fill in your validation logic upon object update.
	return nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *Component) ValidateDelete() error {
	componentlog.Info("validate delete", "name", r.Name)

	// TODO(user): fill in your validation logic upon object deletion.
	return nil
}

// func readJSONResource(req *http.Request, obj rest.Resource, id resources.ResourceID) error {
// 	defer req.Body.Close()
// 	data, err := ioutil.ReadAll(req.Body)
// 	if err != nil {
// 		return fmt.Errorf("error reading request body: %w", err)
// 	}
// 	validator, err := schema.ValidatorFor(obj)
// 	if err != nil {
// 		return fmt.Errorf("cannot find validator for %T: %w", obj, err)
// 	}
// 	if errs := validator.ValidateJSON(data); len(errs) != 0 {
// 		return &validationError{
// 			details: errs,
// 		}
// 	}
// 	err = json.Unmarshal(data, obj)
// 	if err != nil {
// 		return fmt.Errorf("error reading %T: %w", obj, err)
// 	}

// 	// Set Resource properties on the resource based on the URL
// 	obj.SetID(id)

// 	return nil
// }
