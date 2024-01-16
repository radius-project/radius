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

	"github.com/radius-project/radius/pkg/ucp/ucplog"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
)

// SetupWebhookWithManager sets up a webhook for the built in Deployment resource with the given manager.
// It configures the webhook to watch for changes in the appsv1.Deployment resource type and applies the provided defaulter.
// Returns an error if there was a problem setting up the webhook.
func (d *DeploymentWebhook) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&appsv1.Deployment{}).
		WithDefaulter(d).
		Complete()
}

// DeploymentWebhook implements the mutating webhook function for the type Deployment.
type DeploymentWebhook struct{}

// Default mutates the built in Deployment object.
func (a *DeploymentWebhook) Default(ctx context.Context, obj runtime.Object) error {
	logger := ucplog.FromContextOrDiscard(ctx)

	deployment, ok := obj.(*appsv1.Deployment)
	if !ok {
		return fmt.Errorf("expected a built in Deployment but got a %T", obj)
	}

	if annotationValue, exists := deployment.ObjectMeta.Annotations[AnnotationRadiusEnabled]; exists && annotationValue == "true" {

		logger.Info("Pausing Deployment", "deploymentName", deployment.Name)
		deployment.Spec.Paused = true
	}

	return nil
}
