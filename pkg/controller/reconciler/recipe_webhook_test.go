/*
Copyright 2023 The KEDA Authors

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
	"testing"
	"time"

	radappiov1alpha3 "github.com/radius-project/radius/pkg/controller/api/radapp.io/v1alpha3"
	"github.com/radius-project/radius/test/testcontext"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

func Test_ValidateRecipe_InvalidType(t *testing.T) {
	ctx := testcontext.New(t)

	recipeName := "test-recipe-invalidtype"
	defaultNamespace := "default"
	namespace := types.NamespacedName{Namespace: defaultNamespace, Name: recipeName}
	recipe := makeTestRecipe(namespace, "Applications.Core/InvalidType")

	expectedError := fmt.Sprintf("Recipe.radapp.io \"%s\" not found", recipeName)
	radius, client := setupWebhookTest(t)

	err := client.Create(ctx, &corev1.Namespace{ObjectMeta: ctrl.ObjectMeta{Name: namespace.Name}})
	require.NoError(t, err)

	// We do not check for error here since error may be captured after this initial call is completed.
	_ = client.Create(ctx, recipe)

	// Recipe will be waiting for environment to be created.
	createEnvironment(radius, "default")

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
}

func Test_ValidateRecipe_ValidType(t *testing.T) {
	ctx := testcontext.New(t)
	recipeName := "test-recipe-validtype"
	defaultNamespace := "default"
	namespace := types.NamespacedName{Namespace: defaultNamespace, Name: recipeName}
	recipe := makeTestRecipe(namespace, "Applications.Core/extenders")

	radius, client := setupWebhookTest(t)

	err := client.Create(ctx, &corev1.Namespace{ObjectMeta: ctrl.ObjectMeta{Name: namespace.Name}})
	require.NoError(t, err)

	err = client.Create(ctx, recipe)
	require.NoError(t, err)

	// Recipe will be waiting for environment to be created.
	createEnvironment(radius, "default")

	// Recipe will be waiting for extender to complete provisioning.
	status := waitForRecipeStateUpdating(t, client, namespace, nil)

	radius.CompleteOperation(status.Operation.ResumeToken, nil)
	_, err = radius.Resources(status.Scope, "Applications.Core/extenders").Get(ctx, namespace.Name)
	require.NoError(t, err)

	err = client.Delete(ctx, recipe)
	require.NoError(t, err)

	// Deletion of the resource is in progress.
	status = waitForRecipeStateDeleting(t, client, namespace, nil)
	radius.CompleteOperation(status.Operation.ResumeToken, nil)

	// Now deleting of the deployment object can complete.
	waitForRecipeDeleted(t, client, namespace)
}

func makeTestRecipe(name types.NamespacedName, resourceType string) *radappiov1alpha3.Recipe {
	return &radappiov1alpha3.Recipe{
		ObjectMeta: ctrl.ObjectMeta{
			Namespace: name.Namespace,
			Name:      name.Name,
			Annotations: map[string]string{
				"radapp.io/enabled": "true",
			},
		},
		Spec: radappiov1alpha3.RecipeSpec{
			Type: resourceType,
		},
	}
}

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
			Host:    webhookInstallOptions.LocalServingHost,
			Port:    webhookInstallOptions.LocalServingPort,
			CertDir: webhookInstallOptions.LocalServingCertDir,
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
	addrPort := fmt.Sprintf("%s:%d", webhookInstallOptions.LocalServingHost, webhookInstallOptions.LocalServingPort)
	err = waitForConnection(addrPort, 10*time.Second, 5)
	require.NoError(t, err)

	return radius, mgr.GetClient()
}

// waitForConnection waits for a secure connection to the given address and port.
func waitForConnection(addrPort string, timeout time.Duration, maxAttempts int) error {
	dialer := &net.Dialer{Timeout: timeout}

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		conn, err := tls.DialWithDialer(dialer, "tcp", addrPort, &tls.Config{InsecureSkipVerify: true})
		if err == nil {
			conn.Close()
			return nil
		}

		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			return err
		}

		time.Sleep(1 * time.Second)
	}

	return fmt.Errorf("failed to connect after %d attempts", maxAttempts)
}
