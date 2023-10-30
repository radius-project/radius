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
	"context"
	"fmt"
	"strings"

	portableresources "github.com/radius-project/radius/pkg/rp/portableresources"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// SetupWebhookWithManager sets up a webhook for the Recipe resource with the given controller manager.
// It creates a new webhook managed by the controller manager, registers the Recipe resource with the webhook,
// sets the validator for the webhook to the Recipe instance, and completes the webhook setup.
func (r *Recipe) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		WithValidator(r).
		Complete()
}

// +kubebuilder:webhook:path=/validate-radapp-io-v1alpha3-recipe,mutating=false,failurePolicy=fail,sideEffects=None,groups=radapp.io,resources=recipe,verbs=create;update,versions=v1alpha3,name=recipe-webhook.radapp.io,sideEffects=None,admissionReviewVersions=v1

// ValidateCreate validates the creation of a Recipe object.
func (r *Recipe) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	logger := ucplog.FromContextOrDiscard(ctx)

	recipe, ok := obj.(*Recipe)
	if !ok {
		return nil, fmt.Errorf("expected a Recipe but got a %T", obj)
	}

	logger.Info("Validating Create Recipe %s", recipe.Name)
	return recipe.validateRecipeType(ctx)
}

// ValidateUpdate validates the update of a Recipe object.
func (r *Recipe) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	logger := ucplog.FromContextOrDiscard(ctx)

	recipe, ok := newObj.(*Recipe)
	if !ok {
		return nil, fmt.Errorf("expected a Recipe but got a %T", newObj)
	}

	logger.Info("Validating Update Recipe %s", recipe.Name)
	return recipe.validateRecipeType(ctx)
}

// ValidateDelete validates the deletion of a Recipe object.
func (r *Recipe) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	logger := ucplog.FromContextOrDiscard(ctx)
	logger.Info("Validating Delete Recipe")

	_, ok := obj.(*Recipe)
	if !ok {
		return nil, fmt.Errorf("expected a Recipe but got a %T", obj)
	}

	// currently there is no validation when deleting Recipe
	return nil, nil
}

// validateRecipeType validates Resource Type to be created by Recipe.
func (r *Recipe) validateRecipeType(ctx context.Context) (admission.Warnings, error) {
	logger := ucplog.FromContextOrDiscard(ctx)
	validResourceTypes := strings.Join(portableresources.GetValidPortableResourceTypes(), ", ")

	logger.Info("Validating Recipe Type %s in Recipe %s", r.Spec.Type, r.Name)
	if !portableresources.IsValidPortableResourceType(r.Spec.Type) {
		return nil, fmt.Errorf("invalid resource type %s in recipe %s. allowed values are: %s", r.Spec.Type, r.Name, validResourceTypes)
	}

	return nil, nil
}
