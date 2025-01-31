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
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"

	radappiov1alpha3 "github.com/radius-project/radius/pkg/controller/api/radapp.io/v1alpha3"
	"github.com/radius-project/radius/test/testcontext"

	"github.com/stretchr/testify/require"
	admissionv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

const (
	defaultNamespace    = "default"
	validResourceType   = "Applications.Core/extenders"
	invalidResourceType = "invalidType"
	webhookConfigName   = "recipe-webhook-config"
)

// Test_ValidateRecipe_Type tests a recipe with valid and invalid types.
func Test_ValidateRecipe_Type(t *testing.T) {
	ctx := testcontext.New(t)
	radius, client := setupWebhookTest(t)

	// Environment is created.
	createEnvironment(radius, "default", "default")

	t.Run("test recipe for invalid type", func(t *testing.T) {
		recipeName := "test-recipe-invalidtype"
		namespace := types.NamespacedName{Namespace: defaultNamespace, Name: recipeName}
		recipe := makeRecipe(namespace, invalidResourceType)

		err := client.Create(ctx, &corev1.Namespace{ObjectMeta: ctrl.ObjectMeta{Name: namespace.Name}})
		require.NoError(t, err)

		// Webhook is expected to trigger during this call and return an error.
		err = client.Create(ctx, recipe)
		require.True(t, apierrors.IsInvalid(err))

		// Convert the error to a *apierrors.StatusError to get the status code
		statusError, ok := err.(*apierrors.StatusError)
		require.True(t, ok)

		// Check for expected status code
		require.Equal(t, int32(http.StatusUnprocessableEntity), statusError.ErrStatus.Code)
	})

	t.Run("test recipe for valid type", func(t *testing.T) {
		recipeName := "test-recipe-validtype"
		namespace := types.NamespacedName{Namespace: defaultNamespace, Name: recipeName}
		recipe := makeRecipe(namespace, validResourceType)

		err := client.Create(ctx, &corev1.Namespace{ObjectMeta: ctrl.ObjectMeta{Name: namespace.Name}})
		require.NoError(t, err)

		err = client.Create(ctx, recipe)
		require.NoError(t, err)

		// Recipe will be waiting for extender to complete provisioning.
		status := waitForRecipeStateUpdating(t, client, namespace, nil)

		radius.CompleteOperation(status.Operation.ResumeToken, nil)
		_, err = radius.Resources(status.Scope, validResourceType).Get(ctx, namespace.Name)
		require.NoError(t, err)

		err = client.Delete(ctx, recipe)
		require.NoError(t, err)

		// Deletion of the resource is in progress.
		status = waitForRecipeStateDeleting(t, client, namespace, nil)
		radius.CompleteOperation(status.Operation.ResumeToken, nil)

		// Now deleting of the deployment object can complete.
		waitForRecipeDeleted(t, client, namespace)
	})

	t.Run("test recipe update from valid to invalid type", func(t *testing.T) {
		// Create a recipe with a valid type
		recipeName := "test-recipe-update"
		namespace := types.NamespacedName{Namespace: defaultNamespace, Name: recipeName}
		recipe := makeRecipe(namespace, validResourceType)

		err := client.Create(ctx, &corev1.Namespace{ObjectMeta: ctrl.ObjectMeta{Name: namespace.Name}})
		require.NoError(t, err)

		err = client.Create(ctx, recipe)
		require.NoError(t, err)

		// Recipe will be waiting for extender to complete provisioning.
		status := waitForRecipeStateUpdating(t, client, namespace, nil)

		radius.CompleteOperation(status.Operation.ResumeToken, nil)
		_, err = radius.Resources(status.Scope, validResourceType).Get(ctx, namespace.Name)
		require.NoError(t, err)

		// Using RetryOnConflict to avoid catching a conflict error when updating the recipe.
		err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
			// Retrieve the latest version of the recipe
			recipe = &radappiov1alpha3.Recipe{}
			if err := client.Get(ctx, namespace, recipe); err != nil {
				return err
			}

			// Update the recipe to have an invalid type
			recipe.Spec.Type = invalidResourceType
			return client.Update(ctx, recipe)
		})
		// The webhook should reject the update and return an Invalid error
		require.True(t, apierrors.IsInvalid(err))

		// Convert the error to a *apierrors.StatusError to get the status code
		statusError, ok := err.(*apierrors.StatusError)
		require.True(t, ok)

		// Check for expected status code
		require.Equal(t, int32(http.StatusUnprocessableEntity), statusError.ErrStatus.Code)

		err = client.Delete(ctx, recipe)
		require.NoError(t, err)

		// Deletion of the resource is in progress.
		status = waitForRecipeStateDeleting(t, client, namespace, nil)
		radius.CompleteOperation(status.Operation.ResumeToken, nil)

		// Now deleting of the deployment object can complete.
		waitForRecipeDeleted(t, client, namespace)
	})

	// NOTE: We are updating the FailurePolicy of the webhook to Ignore after running webhook tests.
	// This is to ensure that the webhook does not interfere with other tests in the reconciler package.
	// This approach may be updated in the future.
	failurePolicy := admissionv1.Ignore
	updateWebhookFailurePolicy(t, webhookConfigName, &failurePolicy)
}

// Test_Webhook_ValidateFunctions tests webhook functions ValidateCreate, ValidateUpdate, and ValidateDelete
// for a recipe with valid and invalid resource types.
func Test_Webhook_ValidateFunctions(t *testing.T) {
	tests := []struct {
		name       string
		recipeName string
		typeName   string
		function   string
		wantErr    bool
	}{
		{
			name:       "create recipe with valid type",
			recipeName: "create-recipe-validtype",
			typeName:   validResourceType,
			function:   "create",
			wantErr:    false,
		},
		{
			name:       "create recipe with invalid type",
			recipeName: "create-recipe-invalidtype",
			typeName:   invalidResourceType,
			function:   "create",
			wantErr:    true,
		},
		{
			name:       "update recipe with valid type",
			recipeName: "update-recipe-validtype",
			typeName:   validResourceType,
			function:   "update",
			wantErr:    false,
		},
		{
			name:       "update recipe with invalid type",
			recipeName: "update-recipe-invalidtype",
			typeName:   invalidResourceType,
			function:   "update",
			wantErr:    true,
		},
		{
			name:       "delete recipe with valid type",
			recipeName: "delete-recipe-validtype",
			typeName:   validResourceType,
			function:   "delete",
			wantErr:    false,
		},
		{
			name:       "delete recipe with invalid type",
			recipeName: "delete-recipe-invalidtype",
			typeName:   invalidResourceType,
			function:   "delete",
			wantErr:    false,
		},
	}
	for _, tr := range tests {
		t.Run(tr.name, func(t *testing.T) {
			ctx := testcontext.New(t)
			var err error
			namespace := types.NamespacedName{Namespace: defaultNamespace, Name: tr.recipeName}
			recipe := makeRecipe(namespace, tr.typeName)
			recipeWebhook := &RecipeWebhook{}

			if tr.function == "create" {
				_, err = recipeWebhook.ValidateCreate(ctx, recipe)
			} else if tr.function == "update" {
				_, err = recipeWebhook.ValidateUpdate(ctx, nil, recipe)
			} else {
				_, err = recipeWebhook.ValidateDelete(ctx, recipe)
			}

			if tr.wantErr {
				expectedError := fmt.Sprintf("Recipe.radapp.io \"%s\" is invalid: spec.type: Invalid value: \"%s\": must be in the format 'ResourceProvider.Namespace/resourceType'", tr.recipeName, tr.typeName)
				require.True(t, apierrors.IsInvalid(err))
				require.EqualError(t, err, expectedError)

			} else {
				require.NoError(t, err)
			}
		})
	}
}

// setupWebhookTest sets up a webhook test environment.
func setupWebhookTest(t *testing.T) (*mockRadiusClient, client.Client) {
	SkipWithoutEnvironment(t)

	// For debugging, you can set uncomment this to see logs from the controller. This will cause tests to fail
	// because the logging will continue after the test completes.
	//
	// Add runtimelog "sigs.k8s.io/controller-runtime/pkg/log" to imports.
	//
	// runtimelog.SetLogger(ucplog.FromContextOrDiscard(testcontext.New(t)))

	// Shut down the manager when the test exits.
	ctx, cancel := testcontext.NewWithCancel(t)
	t.Cleanup(cancel)

	mgr, err := ctrl.NewManager(config, ctrl.Options{
		Scheme: scheme,
		WebhookServer: webhook.NewServer(webhook.Options{
			Host:    testOptions.LocalServingHost,
			Port:    testOptions.LocalServingPort,
			CertDir: testOptions.LocalServingCertDir,
		}),
		LeaderElection: false,
		Metrics: server.Options{
			BindAddress: "0",
		},
	})
	require.NoError(t, err)

	radius := NewMockRadiusClient()
	err = (&RecipeReconciler{
		Client:        mgr.GetClient(),
		Scheme:        mgr.GetScheme(),
		EventRecorder: mgr.GetEventRecorderFor("recipe-controller"),
		Radius:        radius,
		DelayInterval: recipeTestControllerDelayInterval,
	}).SetupWithManager(mgr)
	require.NoError(t, err)

	err = (&RecipeWebhook{}).SetupWebhookWithManager(mgr)
	require.NoError(t, err)

	go func() {
		err := mgr.Start(ctx)
		require.NoError(t, err)
	}()

	// wait for the webhook server to get ready
	var dialErr error
	dialer := &net.Dialer{Timeout: time.Second}
	addrPort := fmt.Sprintf("%s:%d", testOptions.LocalServingHost, testOptions.LocalServingPort)
	require.Eventuallyf(t, func() bool {
		if conn, err := tls.DialWithDialer(dialer, "tcp", addrPort, &tls.Config{InsecureSkipVerify: true}); err == nil {
			conn.Close()
			return true
		} else if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			dialErr = err
			return false
		}

		return false

	}, time.Second*5, time.Millisecond*200, "Failed to connect: %v", dialErr)

	// NOTE: The default FailurePolicy of webhook is set to Ignore (main_test.go).
	// We are updating the FailurePolicy of the webhook to Fail before running webhook tests to ensure the webhook will return an error when validation fails.
	// This approach may be updated in the future.
	failurePolicy := admissionv1.Fail
	updateWebhookFailurePolicy(t, webhookConfigName, &failurePolicy)

	return radius, mgr.GetClient()
}

// updateWebhookFailurePolicy updates the failure policy of a ValidatingWebhookConfiguration object.
// The function retrieves the ValidatingWebhookConfiguration object for the given webhookConfigName,
// updates its failure policy with the provided webhookfailurePolicy value, and then updates the object in the Kubernetes cluster.
// If any error occurs during the retrieval or update process, the function fails the test.
func updateWebhookFailurePolicy(t *testing.T, webhookConfigName string, webhookfailurePolicy *admissionv1.FailurePolicyType) {
	SkipWithoutEnvironment(t)

	// Shut down the manager when the test exits.
	ctx, cancel := testcontext.NewWithCancel(t)
	t.Cleanup(cancel)

	// Define the object key (name)
	key := client.ObjectKey{
		Name: webhookConfigName, // "recipe-webhook-config",
	}

	// Create a ValidatingWebhookConfiguration object to receive the data
	webhook := &admissionv1.ValidatingWebhookConfiguration{}

	// Get the ValidatingWebhookConfiguration
	k8sClient, err := client.New(config, client.Options{})
	require.NoError(t, err)

	err = k8sClient.Get(ctx, key, webhook)
	require.NoError(t, err)

	// Update the failure policy of the webhook
	webhook.Webhooks[0].FailurePolicy = webhookfailurePolicy

	// Update the ValidatingWebhookConfiguration
	err = k8sClient.Update(ctx, webhook)
	require.NoError(t, err)
}
