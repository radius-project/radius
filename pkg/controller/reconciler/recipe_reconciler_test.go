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
	"errors"
	"testing"

	"github.com/radius-project/radius/pkg/cli/clients_new/generated"
	"github.com/radius-project/radius/test/testcontext"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	crconfig "sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

func SetupRecipeTest(t *testing.T) (*mockRadiusClient, client.Client) {
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
		Controller: crconfig.Controller{
			SkipNameValidation: boolPtr(true),
		},

		// Suppress metrics in tests to avoid conflicts.
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

	go func() {
		err := mgr.Start(ctx)
		require.NoError(t, err)
	}()

	return radius, mgr.GetClient()
}

func Test_RecipeReconciler_WithoutSecret(t *testing.T) {
	ctx := testcontext.New(t)
	radius, client := SetupRecipeTest(t)

	name := types.NamespacedName{Namespace: "recipe-without-secret", Name: "test-recipe-withoutsecret"}
	err := client.Create(ctx, &corev1.Namespace{ObjectMeta: ctrl.ObjectMeta{Name: name.Namespace}})
	require.NoError(t, err)

	recipe := makeRecipe(name, "Applications.Core/extenders")
	err = client.Create(ctx, recipe)
	require.NoError(t, err)

	// Recipe will be waiting for environment to be created.
	createEnvironment(radius, "default", "default")

	// Recipe will be waiting for extender to complete provisioning.
	status := waitForRecipeStateUpdating(t, client, name, nil)
	require.Equal(t, "/planes/radius/local/resourcegroups/default-recipe-without-secret", status.Scope)
	require.Equal(t, "/planes/radius/local/resourceGroups/default/providers/Applications.Core/environments/default", status.Environment)
	require.Equal(t, "/planes/radius/local/resourcegroups/default-recipe-without-secret/providers/Applications.Core/applications/recipe-without-secret", status.Application)

	radius.CompleteOperation(status.Operation.ResumeToken, nil)

	// Recipe will update after operation completes
	status = waitForRecipeStateReady(t, client, name)
	require.Equal(t, "/planes/radius/local/resourcegroups/default-recipe-without-secret/providers/Applications.Core/extenders/test-recipe-withoutsecret", status.Resource)

	extender, err := radius.Resources(status.Scope, "Applications.Core/extenders").Get(ctx, name.Name)
	require.NoError(t, err)
	require.Equal(t, "recipe", extender.Properties["resourceProvisioning"])

	err = client.Delete(ctx, recipe)
	require.NoError(t, err)

	// Deletion of the recipe is in progress.
	status = waitForRecipeStateDeleting(t, client, name, nil)
	radius.CompleteOperation(status.Operation.ResumeToken, nil)

	// Now deleting of the deployment object can complete.
	waitForRecipeDeleted(t, client, name)
}

func Test_RecipeReconciler_ChangeEnvironmentAndApplication(t *testing.T) {
	ctx := testcontext.New(t)
	radius, client := SetupRecipeTest(t)

	name := types.NamespacedName{Namespace: "recipe-change-envapp", Name: "test-recipe-change-envapp"}
	err := client.Create(ctx, &corev1.Namespace{ObjectMeta: ctrl.ObjectMeta{Name: name.Namespace}})
	require.NoError(t, err)

	recipe := makeRecipe(name, "Applications.Core/extenders")
	err = client.Create(ctx, recipe)
	require.NoError(t, err)

	// Recipe will be waiting for environment to be created.
	createEnvironment(radius, "default", "default")

	// Recipe will be waiting for extender to complete provisioning.
	status := waitForRecipeStateUpdating(t, client, name, nil)
	require.Equal(t, "/planes/radius/local/resourcegroups/default-recipe-change-envapp", status.Scope)
	require.Equal(t, "/planes/radius/local/resourceGroups/default/providers/Applications.Core/environments/default", status.Environment)
	require.Equal(t, "/planes/radius/local/resourcegroups/default-recipe-change-envapp/providers/Applications.Core/applications/recipe-change-envapp", status.Application)

	radius.CompleteOperation(status.Operation.ResumeToken, nil)

	// Recipe will update after operation completes
	status = waitForRecipeStateReady(t, client, name)
	require.Equal(t, "/planes/radius/local/resourcegroups/default-recipe-change-envapp/providers/Applications.Core/extenders/test-recipe-change-envapp", status.Resource)

	createEnvironment(radius, "new-environment", "new-environment")

	// Now update the recipe to change the environment and application.
	err = client.Get(ctx, name, recipe)
	require.NoError(t, err)

	recipe.Spec.Environment = "new-environment"
	recipe.Spec.Application = "new-application"

	err = client.Update(ctx, recipe)
	require.NoError(t, err)

	// Now the recipe will delete and re-create the resource.

	// Deletion of the resource is in progress.
	status = waitForRecipeStateDeleting(t, client, name, nil)
	radius.CompleteOperation(status.Operation.ResumeToken, nil)

	// Resource should be gone.
	_, err = radius.Resources(status.Scope, "Applications.Core/extenders").Get(ctx, name.Name)
	require.Error(t, err)

	// Recipe will be waiting for extender to complete provisioning.
	status = waitForRecipeStateUpdating(t, client, name, nil)
	require.Equal(t, "/planes/radius/local/resourcegroups/new-environment-new-application", status.Scope)
	require.Equal(t, "/planes/radius/local/resourceGroups/new-environment/providers/Applications.Core/environments/new-environment", status.Environment)
	require.Equal(t, "/planes/radius/local/resourcegroups/new-environment-new-application/providers/Applications.Core/applications/new-application", status.Application)
	radius.CompleteOperation(status.Operation.ResumeToken, nil)

	// Recipe will update after operation completes
	status = waitForRecipeStateReady(t, client, name)
	require.Equal(t, "/planes/radius/local/resourcegroups/new-environment-new-application/providers/Applications.Core/extenders/test-recipe-change-envapp", status.Resource)

	// Now delete the recipe.
	err = client.Delete(ctx, recipe)
	require.NoError(t, err)

	// Deletion of the resource is in progress.
	status = waitForRecipeStateDeleting(t, client, name, nil)
	radius.CompleteOperation(status.Operation.ResumeToken, nil)

	// Now deleting of the deployment object can complete.
	waitForRecipeDeleted(t, client, name)
}

func Test_RecipeReconciler_FailureRecovery(t *testing.T) {
	// This test tests our ability to recover from failed operations inside Radius.
	//
	// We use the mock client to simulate the failure of update and delete operations
	// and verify that the controller will (eventually) retry these operations.

	ctx := testcontext.New(t)
	radius, client := SetupRecipeTest(t)

	name := types.NamespacedName{Namespace: "recipe-failure-recovery", Name: "test-recipe-failure-recovery"}
	err := client.Create(ctx, &corev1.Namespace{ObjectMeta: ctrl.ObjectMeta{Name: name.Namespace}})
	require.NoError(t, err)

	recipe := makeRecipe(name, "Applications.Core/extenders")
	err = client.Create(ctx, recipe)
	require.NoError(t, err)

	// Recipe will be waiting for environment to be created.
	createEnvironment(radius, "default", "default")

	// Recipe will be waiting for extender to complete provisioning.
	status := waitForRecipeStateUpdating(t, client, name, nil)

	// Complete the operation, but make it fail.
	operation := status.Operation
	radius.CompleteOperation(status.Operation.ResumeToken, func(state *operationState) {
		state.err = errors.New("oops")

		resource, ok := radius.resources[state.resourceID]
		require.True(t, ok, "failed to find resource")

		resource.Properties["provisioningState"] = "Failed"
		state.value = generated.GenericResourcesClientCreateOrUpdateResponse{GenericResource: resource}
	})

	// Recipe should (eventually) start a new provisioning operation
	status = waitForRecipeStateUpdating(t, client, name, operation)

	// Complete the operation, successfully this time.
	radius.CompleteOperation(status.Operation.ResumeToken, nil)
	_ = waitForRecipeStateReady(t, client, name)

	err = client.Delete(ctx, recipe)
	require.NoError(t, err)

	// Deletion of the recipe is in progress.
	status = waitForRecipeStateDeleting(t, client, name, nil)

	// Complete the operation, but make it fail.
	operation = status.Operation
	radius.CompleteOperation(status.Operation.ResumeToken, func(state *operationState) {
		state.err = errors.New("oops")

		resource, ok := radius.resources[state.resourceID]
		require.True(t, ok, "failed to find resource")

		resource.Properties["provisioningState"] = "Failed"
	})

	// Recipe should (eventually) start a new provisioning operation
	status = waitForRecipeStateDeleting(t, client, name, operation)

	// Complete the operation, successfully this time.
	radius.CompleteOperation(status.Operation.ResumeToken, nil)

	// Now deleting of the deployment object can complete.
	waitForRecipeDeleted(t, client, name)
}

func Test_RecipeReconciler_WithSecret(t *testing.T) {
	ctx := testcontext.New(t)
	radius, client := SetupRecipeTest(t)

	name := types.NamespacedName{Namespace: "recipe-withsecret", Name: "test-recipe-withsecret"}
	err := client.Create(ctx, &corev1.Namespace{ObjectMeta: ctrl.ObjectMeta{Name: name.Namespace}})
	require.NoError(t, err)

	recipe := makeRecipe(name, "Applications.Core/extenders")
	recipe.Spec.SecretName = name.Name

	err = client.Create(ctx, recipe)
	require.NoError(t, err)

	// Recipe will be waiting for environment to be created.
	createEnvironment(radius, "default", "default")

	// Recipe will be waiting for extender to complete provisioning.
	status := waitForRecipeStateUpdating(t, client, name, nil)

	// Update the resource with computed values as part of completing the operation.
	radius.CompleteOperation(status.Operation.ResumeToken, func(state *operationState) {
		resource, ok := radius.resources[state.resourceID]
		require.True(t, ok, "failed to find resource")

		resource.Properties["a-value"] = "a"
		resource.Properties["secrets"] = map[string]string{
			"b-secret": "b",
		}
		state.value = generated.GenericResourcesClientCreateOrUpdateResponse{GenericResource: resource}
	})

	// Recipe will update after operation completes
	status = waitForRecipeStateReady(t, client, name)

	secret := corev1.Secret{}
	err = client.Get(ctx, name, &secret)
	require.NoError(t, err)

	expectedData := map[string][]byte{
		"a-value":  []byte("a"),
		"b-secret": []byte("b"),
	}

	require.Equal(t, expectedData, secret.Data)

	extender, err := radius.Resources(status.Scope, "Applications.Core/extenders").Get(ctx, name.Name)
	require.NoError(t, err)
	require.Equal(t, "recipe", extender.Properties["resourceProvisioning"])

	// Now we'll change the secret name.
	err = client.Get(ctx, name, recipe)
	require.NoError(t, err)

	recipe.Spec.SecretName = "new-secret-name"
	err = client.Update(ctx, recipe)
	require.NoError(t, err)

	// Recipe will update after operation completes
	_ = waitForRecipeStateReady(t, client, name)

	// The old secret should be deleted
	old := corev1.Secret{}
	err = client.Get(ctx, name, &old)
	require.Error(t, err)
	require.True(t, apierrors.IsNotFound(err))

	secret = corev1.Secret{}
	err = client.Get(ctx, types.NamespacedName{Namespace: name.Namespace, Name: "new-secret-name"}, &secret)
	require.NoError(t, err)
	require.Equal(t, expectedData, secret.Data)

	// Now we'll delete the recipe.
	err = client.Delete(ctx, recipe)
	require.NoError(t, err)

	// Deletion of the recipe is in progress.
	status = waitForRecipeStateDeleting(t, client, name, nil)
	radius.CompleteOperation(status.Operation.ResumeToken, nil)

	// Now deleting of the deployment object can complete.
	waitForRecipeDeleted(t, client, name)

	err = client.Get(ctx, name, &secret)
	require.Error(t, err)
	require.True(t, apierrors.IsNotFound(err))
}
