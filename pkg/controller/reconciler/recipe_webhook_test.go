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
	"strings"
	"testing"
	"time"

	radappiov1alpha3 "github.com/radius-project/radius/pkg/controller/api/radapp.io/v1alpha3"
	portableresources "github.com/radius-project/radius/pkg/rp/portableresources"
	"github.com/radius-project/radius/test/testcontext"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

const (
	defaultNamespace    = "default"
	validResourceType   = "Applications.Core/extenders"
	invalidResourceType = "Applications.Core/invalidType"
)

// Test_ValidateRecipe_Type tests a recipe with valid and invalid types.
func Test_ValidateRecipe_Type(t *testing.T) {
	ctx := testcontext.New(t)
	radius, client := setupWebhookTest(t)

	// Environment is created.
	createEnvironment(radius, "default")

	t.Run("test recipe for invalid type", func(t *testing.T) {
		recipeName := "test-recipe-invalidtype"
		namespace := types.NamespacedName{Namespace: defaultNamespace, Name: recipeName}
		recipe := makeRecipe(namespace, invalidResourceType)
		expectedError := fmt.Sprintf("Recipe.radapp.io \"%s\" not found", recipeName)

		err := client.Create(ctx, &corev1.Namespace{ObjectMeta: ctrl.ObjectMeta{Name: namespace.Name}})
		require.NoError(t, err)

		// We do not check for error here since error may be captured after this initial call is completed.
		_ = client.Create(ctx, recipe)

		current := &radappiov1alpha3.Recipe{}
		require.Eventually(t, func() bool {
			if err := client.Get(ctx, namespace, current); err != nil {
				require.EqualError(t, err, expectedError)
				return true
			}
			return false
		}, time.Second*10, time.Millisecond*200)

		err = client.Delete(ctx, recipe)
		require.Error(t, err, expectedError)
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

			if tr.function == "create" {
				_, err = recipe.ValidateCreate(ctx, recipe)
			} else if tr.function == "update" {
				_, err = recipe.ValidateUpdate(ctx, nil, recipe)
			} else {
				_, err = recipe.ValidateDelete(ctx, recipe)
			}

			if tr.wantErr {
				validResourceTypes := strings.Join(portableresources.GetValidPortableResourceTypes(), ", ")
				expectedError := fmt.Sprintf("invalid resource type %s in recipe %s. allowed values are: %s", tr.typeName, tr.recipeName, validResourceTypes)
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
		LeaderElection:     false,
		MetricsBindAddress: "0",
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

	err = (&radappiov1alpha3.Recipe{}).SetupWebhookWithManager(mgr)
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

	return radius, mgr.GetClient()
}
