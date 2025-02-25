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

package reconciler

import (
	"context"
	"fmt"
	"strings"

	radappiov1alpha3 "github.com/radius-project/radius/pkg/controller/api/radapp.io/v1alpha3"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// SetupWebhookWithManager sets up the webhook for the Recipe type with the provided manager.
// It configures the webhook to watch for changes on the Recipe resource and uses the provided validator.
// Returns an error if there was a problem setting up the webhook.
func (r *RecipeWebhook) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&radappiov1alpha3.Recipe{}).
		WithValidator(r).
		Complete()
}

// RecipeWebhook implements the validating webhook functions for the Recipe type.
type RecipeWebhook struct{}

// ValidateCreate validates the creation of a Recipe object.
func (r *RecipeWebhook) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	logger := ucplog.FromContextOrDiscard(ctx)

	recipe, ok := obj.(*radappiov1alpha3.Recipe)
	if !ok {
		return nil, fmt.Errorf("expected a Recipe but got a %T", obj)
	}

	logger.Info("Validating Create Recipe %s", recipe.Name)
	return r.validateRecipeType(ctx, recipe)
}

// ValidateUpdate validates the update of a Recipe object.
func (r *RecipeWebhook) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	logger := ucplog.FromContextOrDiscard(ctx)

	recipe, ok := newObj.(*radappiov1alpha3.Recipe)
	if !ok {
		return nil, fmt.Errorf("expected a Recipe but got a %T", newObj)
	}

	logger.Info("Validating Update Recipe %s", recipe.Name)
	return r.validateRecipeType(ctx, recipe)
}

// ValidateDelete validates the deletion of a Recipe object.
func (r *RecipeWebhook) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	logger := ucplog.FromContextOrDiscard(ctx)
	logger.Info("Validating Delete Recipe")

	_, ok := obj.(*radappiov1alpha3.Recipe)
	if !ok {
		return nil, fmt.Errorf("expected a Recipe but got a %T", obj)
	}

	// currently there is no validation when deleting Recipe
	return nil, nil
}

// validateRecipeType validates Recipe object.
func (r *RecipeWebhook) validateRecipeType(ctx context.Context, recipe *radappiov1alpha3.Recipe) (admission.Warnings, error) {
	logger := ucplog.FromContextOrDiscard(ctx)
	var errList field.ErrorList
	flPath := field.NewPath("spec").Child("type")

	logger.Info("Validating Recipe Type %s in Recipe %s", recipe.Spec.Type, recipe.Name)
	if recipe.Spec.Type == "" || strings.Count(recipe.Spec.Type, "/") != 1 {
		errList = append(errList, field.Invalid(flPath, recipe.Spec.Type, "must be in the format 'ResourceProvider.Namespace/resourceType'"))

		return nil, apierrors.NewInvalid(
			schema.GroupKind{Group: "radapp.io", Kind: "Recipe"},
			recipe.Name,
			errList)
	}

	return nil, nil
}
