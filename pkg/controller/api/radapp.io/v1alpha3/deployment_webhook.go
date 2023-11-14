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

	"github.com/radius-project/radius/pkg/ucp/ucplog"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
)

// SetupWebhookWithManager sets up a webhook for the built in Deployment resource with the given controller manager.
// It creates a new webhook managed by the controller manager, registers the built in Deployment resource with the webhook,
// sets the validator for the webhook to the Deployment instance, and completes the webhook setup.
func (d *BuiltInDeployment) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&appsv1.Deployment{}).
		WithDefaulter(d).
		Complete()
}

// +kubebuilder:webhook:path=/mutate-radapp-io-v1alpha3-builtindeployment,mutating=true,failurePolicy=fail,sideEffects=None,groups=radapp.io,resources=deployments,verbs=create;update,versions=v1alpha3,name=deployment-webhook.radapp.io,sideEffects=None,admissionReviewVersions=v1

// BuiltInDeployment is a type that implements the mutating webhook function for the type Deployment.
type BuiltInDeployment struct{}

// Default mutates the built in Deployment object.
func (a *BuiltInDeployment) Default(ctx context.Context, obj runtime.Object) error {
	logger := ucplog.FromContextOrDiscard(ctx)
	_, ok := obj.(*appsv1.Deployment)
	if !ok {
		return fmt.Errorf("expected a built in Deployment but got a %T", obj)
	}

	// TODO: Placeholder implementation to be updated.
	logger.Info("Update Deployment")

	return nil
}
