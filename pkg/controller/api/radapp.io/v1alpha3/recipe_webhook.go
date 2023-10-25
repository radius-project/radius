/*
Copyright 2023.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha3

import (
	"fmt"

	portableresources "github.com/radius-project/radius/pkg/rp/portableresources"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// log is for logging in this package.
var recipejoblog = logf.Log.WithName("recipe-resource")

func (r *Recipe) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

// +kubebuilder:webhook:path=/validate-radapp-io-v1alpha3-recipe,mutating=false,failurePolicy=fail,sideEffects=None,groups=radapp.io,resources=recipe,verbs=create;update,versions=v1alpha3,name=recipe-webhook.radapp.io,sideEffects=None,admissionReviewVersions=v1
var _ webhook.Validator = &Recipe{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *Recipe) ValidateCreate() (admission.Warnings, error) {
	recipejoblog.Info("validate create", "name", r.Name)

	return nil, r.validateRecipeType()
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *Recipe) ValidateUpdate(old runtime.Object) (admission.Warnings, error) {
	recipejoblog.Info("validate update", "name", r.Name)

	return nil, r.validateRecipeType()
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *Recipe) ValidateDelete() (admission.Warnings, error) {
	recipejoblog.Info("validate delete", "name", r.Name)

	return nil, nil
}

// validateRecipeType validates Resource Type to be created.
func (r *Recipe) validateRecipeType() error {
	var errList field.ErrorList
	flPath := field.NewPath("spec").Child("type")

	if !portableresources.IsValidPortableResourceType(r.Spec.Type) {
		errList = append(errList, field.Invalid(flPath, r.Spec.Type, fmt.Sprintf("invalid resource type %q in recipe", r.Spec.Type)))
		return apierrors.NewInvalid(
			schema.GroupKind{Group: "radapp.io", Kind: "Recipe"},
			r.Name,
			errList)

	}

	return nil
}
