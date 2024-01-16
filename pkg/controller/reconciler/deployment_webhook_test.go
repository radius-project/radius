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
	"testing"
	"time"

	"github.com/radius-project/radius/test/testcontext"
	"github.com/stretchr/testify/require"
	admissionv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
)

// Test_DeploymentPaused tests for expected deployment being paused and  with valid and invalid types.
func Test_DeploymentPaused(t *testing.T) {
	ctx := testcontext.New(t)
	radius, client := setupWebhookTest(t)

	// Environment is created.
	createEnvironment(radius, "default")

	t.Run("test deployment is paused ", func(t *testing.T) {
		name := types.NamespacedName{Namespace: "deployment-paused", Name: "test-deployment-paused"}
		err := client.Create(ctx, &corev1.Namespace{ObjectMeta: ctrl.ObjectMeta{Name: name.Namespace}})
		require.NoError(t, err)

		deployment := makeDeployment(name)
		deployment.Annotations[AnnotationRadiusEnabled] = "true"
		err = client.Create(ctx, deployment)
		require.NoError(t, err)

		// Webhook is expected to trigger during this call and pause the deployment.
		// Wait for the webhook to process the deployment
		time.Sleep(time.Second * 2)

		// Get the deployment
		actualDeployment := &appsv1.Deployment{}
		err = client.Get(ctx, name, actualDeployment)
		require.NoError(t, err)

		// Check if the deployment is paused
		require.True(t, actualDeployment.Spec.Paused)

		err = client.Delete(ctx, deployment)
		require.NoError(t, err)

		// Now deleting of the deployment object can complete.
		waitForDeploymentDeleted(t, client, name)
	})

	t.Run("test deployment is not paused ", func(t *testing.T) {
		name := types.NamespacedName{Namespace: "deployment-not-paused", Name: "test-deployment-not-paused"}
		err := client.Create(ctx, &corev1.Namespace{ObjectMeta: ctrl.ObjectMeta{Name: name.Namespace}})
		require.NoError(t, err)

		deployment := makeDeployment(name)
		err = client.Create(ctx, deployment)
		require.NoError(t, err)

		// Webhook is expected to trigger during this call and skip the deployment.
		// Wait for the webhook to process the deployment
		time.Sleep(time.Second * 2)

		// Get the deployment
		actualDeployment := &appsv1.Deployment{}
		err = client.Get(ctx, name, actualDeployment)
		require.NoError(t, err)

		// Check if the deployment is paused
		require.False(t, actualDeployment.Spec.Paused)

		err = client.Delete(ctx, deployment)
		require.NoError(t, err)

		// Now deleting of the deployment object can complete.
		waitForDeploymentDeleted(t, client, name)
	})

	// NOTE: We are updating the FailurePolicy of the webhook to Ignore after running webhook tests.
	// This is to ensure that the webhook does not interfere with other tests in the reconciler package.
	// This approach may be updated in the future.
	failurePolicy := admissionv1.Ignore
	updateWebhookFailurePolicy(t, webhookConfigName, &failurePolicy)
}

// Test_DeploymentWebhook_Mutate is a unit test function that tests the Mutate method of the DeploymentWebhook struct.
// It verifies the behavior of the Mutate method by running multiple test cases with different input values.
// Each test case represents a scenario where a deployment is created with different annotations, and the expected result is checked.
// Test_DeploymentWebhook_Mutate tests webhook functions for DeploymentWebhook
// for a recipe with valid and invalid resource types.
func Test_DeploymentWebhook_Mutate(t *testing.T) {
	tests := []struct {
		name                   string
		deploymentName         string
		radappAnnotation       string
		expectPausedDeployment bool
	}{
		{
			name:                   "create deployment radapp enabled",
			deploymentName:         "create-deployment-radapp-enabled",
			radappAnnotation:       "true",
			expectPausedDeployment: true,
		},
		{
			name:                   "create deployment radapp disabled",
			deploymentName:         "create-deployment-radapp-disabled",
			radappAnnotation:       "false",
			expectPausedDeployment: false,
		},
		{
			name:                   "create deployment no annotation",
			deploymentName:         "create-deployment-no-annotation",
			radappAnnotation:       "missing",
			expectPausedDeployment: false,
		},
	}
	for _, tr := range tests {
		t.Run(tr.name, func(t *testing.T) {
			ctx := testcontext.New(t)
			var err error
			namespace := types.NamespacedName{Namespace: defaultNamespace, Name: tr.deploymentName}
			deployment := makeDeployment(namespace)
			if tr.radappAnnotation != "missing" {
				deployment.Annotations[AnnotationRadiusEnabled] = tr.radappAnnotation
			}

			deploymentWebhook := &DeploymentWebhook{}
			err = deploymentWebhook.Default(ctx, deployment)
			require.NoError(t, err)

			// Check if the deployment is paused
			require.Equal(t, tr.expectPausedDeployment, deployment.Spec.Paused)
		})
	}
}
