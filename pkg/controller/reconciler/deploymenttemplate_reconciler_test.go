/*
Copyright 2024 The Radius Authors.

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
	"encoding/json"
	"errors"
	"os"
	"path"
	"testing"
	"time"

	"github.com/radius-project/radius/pkg/cli/clients_new/generated"
	radappiov1alpha3 "github.com/radius-project/radius/pkg/controller/api/radapp.io/v1alpha3"
	sdkclients "github.com/radius-project/radius/pkg/sdk/clients"
	"github.com/radius-project/radius/test/testcontext"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	crconfig "sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

const (
	deploymentTemplateTestWaitDuration            = time.Second * 10
	deploymentTemplateTestWaitInterval            = time.Second * 1
	deploymentTemplateTestControllerDelayInterval = time.Millisecond * 100
)

func SetupDeploymentTemplateTest(t *testing.T) (*mockRadiusClient, client.Client) {
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

	// Set up DeploymentTemplateReconciler.
	err = (&DeploymentTemplateReconciler{
		Client:        mgr.GetClient(),
		Scheme:        mgr.GetScheme(),
		EventRecorder: mgr.GetEventRecorderFor("deploymenttemplate-controller"),
		Radius:        radius,
		DelayInterval: deploymentTemplateTestControllerDelayInterval,
	}).SetupWithManager(mgr)
	require.NoError(t, err)

	// Set up DeploymentResourceReconciler.
	err = (&DeploymentResourceReconciler{
		Client:        mgr.GetClient(),
		Scheme:        mgr.GetScheme(),
		EventRecorder: mgr.GetEventRecorderFor("deploymentresource-controller"),
		Radius:        radius,
		DelayInterval: DeploymentResourceTestControllerDelayInterval,
	}).SetupWithManager(mgr)
	require.NoError(t, err)

	go func() {
		err := mgr.Start(ctx)
		require.NoError(t, err)
	}()

	return radius, mgr.GetClient()
}

func Test_DeploymentTemplateReconciler_Basic(t *testing.T) {
	ctx := testcontext.New(t)
	radius, client := SetupDeploymentTemplateTest(t)

	name := types.NamespacedName{Namespace: "deploymenttemplate-basic", Name: "test-deploymenttemplate-basic"}
	err := client.Create(ctx, &corev1.Namespace{ObjectMeta: ctrl.ObjectMeta{Name: name.Namespace}})
	require.NoError(t, err)

	deploymentTemplate := makeDeploymentTemplate(name, "{}", generateDefaultProviderConfig(), "deploymenttemplate-basic.bicep", map[string]string{})
	err = client.Create(ctx, deploymentTemplate)
	require.NoError(t, err)

	// Wait for the DeploymentTemplate to enter the updating state.
	status := waitForDeploymentTemplateStateUpdating(t, client, name, nil)

	// Verify the provider config is parsed correctly.
	scope, err := ParseDeploymentScopeFromProviderConfig(status.ProviderConfig)
	require.NoError(t, err)
	require.Equal(t, "/planes/radius/local/resourcegroups/default", scope)

	radius.CompleteOperation(status.Operation.ResumeToken, nil)

	// DeploymentTemplate should be ready after the operation completes.
	status = waitForDeploymentTemplateStateReady(t, client, name)
	require.Equal(t, "/planes/radius/local/resourcegroups/default/providers/Microsoft.Resources/deployments/test-deploymenttemplate-basic", status.Resource)

	// Verify that the Radius deployment contains the expected properties.
	expectedProperties := map[string]any{
		"mode":       "Incremental",
		"template":   map[string]any{},
		"parameters": map[string]map[string]string{},
		"providerConfig": sdkclients.ProviderConfig{
			Radius: &sdkclients.Radius{
				Type: "Radius",
				Value: sdkclients.Value{
					Scope: "/planes/radius/local/resourcegroups/default",
				},
			},
			Deployments: &sdkclients.Deployments{
				Type: "Microsoft.Resources",
				Value: sdkclients.Value{
					Scope: "/planes/radius/local/resourcegroups/default",
				},
			},
		},
	}
	resource, err := radius.Resources(scope, "Microsoft.Resources/deployments").Get(ctx, name.Name)
	require.NoError(t, err)
	require.Equal(t, expectedProperties, resource.Properties)

	// Verify that the DeploymentTemplate contains the expected properties.
	require.Equal(t, "{}", status.Template)
	require.Equal(t, "{}", status.Parameters)
	require.Equal(t, string(generateDefaultProviderConfig()), status.ProviderConfig)
	require.Equal(t, "deploymenttemplate-basic.bicep", status.RootFileName)

	// Delete the DeploymentTemplate
	err = client.Delete(ctx, deploymentTemplate)
	require.NoError(t, err)

	// Wait for the DeploymentTemplate to be deleted.
	waitForDeploymentTemplateStateDeleted(t, client, name)
}

func Test_DeploymentTemplateReconciler_FailureRecovery(t *testing.T) {
	// This test tests our ability to recover from failed operations inside Radius.
	//
	// We use the mock client to simulate the failure of update and delete operations
	// and verify that the controller will (eventually) retry these operations.

	ctx := testcontext.New(t)
	radius, client := SetupDeploymentTemplateTest(t)

	name := types.NamespacedName{Namespace: "deploymenttemplate-failurerecovery", Name: "test-deploymenttemplate-failurerecovery"}
	err := client.Create(ctx, &corev1.Namespace{ObjectMeta: ctrl.ObjectMeta{Name: name.Namespace}})
	require.NoError(t, err)

	deploymentTemplate := makeDeploymentTemplate(name, "{}", generateDefaultProviderConfig(), "deploymenttemplate-failurerecovery.bicep", map[string]string{})
	err = client.Create(ctx, deploymentTemplate)
	require.NoError(t, err)

	// Wait for the DeploymentTemplate to enter the updating state.
	status := waitForDeploymentTemplateStateUpdating(t, client, name, nil)

	// Complete the operation, but make it fail.
	operation := status.Operation
	radius.CompleteOperation(status.Operation.ResumeToken, func(state *operationState) {
		state.err = errors.New("failure")

		resource, ok := radius.resources[state.resourceID]
		require.True(t, ok, "failed to find resource")

		resource.Properties["provisioningState"] = "Failed"
		state.value = generated.GenericResourcesClientCreateOrUpdateResponse{GenericResource: resource}
	})

	// DeploymentTemplate should (eventually) start a new provisioning operation
	status = waitForDeploymentTemplateStateUpdating(t, client, name, operation)

	// Complete the operation, successfully this time.
	radius.CompleteOperation(status.Operation.ResumeToken, nil)
	_ = waitForDeploymentTemplateStateReady(t, client, name)

	err = client.Delete(ctx, deploymentTemplate)
	require.NoError(t, err)

	waitForDeploymentTemplateStateDeleted(t, client, name)
}

func Test_DeploymentTemplateReconciler_WithResources(t *testing.T) {
	ctx := testcontext.New(t)
	radius, client := SetupDeploymentTemplateTest(t)

	name := types.NamespacedName{Namespace: "deploymenttemplate-withresources", Name: "test-deploymenttemplate-withresources"}
	err := client.Create(ctx, &corev1.Namespace{ObjectMeta: ctrl.ObjectMeta{Name: name.Namespace}})
	require.NoError(t, err)

	fileContent, err := os.ReadFile(path.Join("testdata", "deploymenttemplate-withresources.json"))
	require.NoError(t, err)
	templateMap := map[string]any{}
	err = json.Unmarshal(fileContent, &templateMap)
	require.NoError(t, err)
	template, err := json.MarshalIndent(templateMap, "", "  ")
	require.NoError(t, err)

	deploymentTemplate := makeDeploymentTemplate(name, string(template), generateDefaultProviderConfig(), "deploymenttemplate-withresources.bicep", map[string]string{})
	err = client.Create(ctx, deploymentTemplate)
	require.NoError(t, err)

	status := waitForDeploymentTemplateStateUpdating(t, client, name, nil)

	// Verify the provider config is parsed correctly.
	scope, err := ParseDeploymentScopeFromProviderConfig(status.ProviderConfig)
	require.NoError(t, err)
	require.Equal(t, "/planes/radius/local/resourcegroups/default", scope)

	radius.CompleteOperation(status.Operation.ResumeToken, func(state *operationState) {
		resource, ok := radius.resources[state.resourceID]
		require.True(t, ok, "failed to find resource")

		resource.Properties["outputResources"] = []any{
			map[string]any{"id": "/planes/radius/local/resourceGroups/default/providers/Applications.Core/environments/env"},
		}
		state.value = generated.GenericResourcesClientCreateOrUpdateResponse{GenericResource: resource}
	})

	// DeploymentTemplate should be ready after the operation completes.
	status = waitForDeploymentTemplateStateReady(t, client, name)
	require.Equal(t, "/planes/radius/local/resourcegroups/default/providers/Microsoft.Resources/deployments/test-deploymenttemplate-withresources", status.Resource)

	// DeploymentTemplate will be waiting for environment to be created.
	createEnvironment(radius, "default", "env")

	dependencyName := types.NamespacedName{Namespace: name.Namespace, Name: "env"}
	dependencyStatus := waitForDeploymentResourceStateReady(t, client, dependencyName)
	require.Equal(t, "/planes/radius/local/resourceGroups/default/providers/Applications.Core/environments/env", dependencyStatus.Id)

	// Verify that the Radius deployment contains the expected properties.
	resource, err := radius.Resources(scope, "Microsoft.Resources/deployments").Get(ctx, name.Name)
	require.NoError(t, err)
	expectedProperties := map[string]any{
		"mode":       "Incremental",
		"template":   templateMap,
		"parameters": map[string]map[string]string{},
		"providerConfig": sdkclients.ProviderConfig{
			Radius: &sdkclients.Radius{
				Type: "Radius",
				Value: sdkclients.Value{
					Scope: "/planes/radius/local/resourcegroups/default",
				},
			},
			Deployments: &sdkclients.Deployments{
				Type: "Microsoft.Resources",
				Value: sdkclients.Value{
					Scope: "/planes/radius/local/resourcegroups/default",
				},
			},
		},
		"outputResources": []any{
			map[string]any{"id": "/planes/radius/local/resourceGroups/default/providers/Applications.Core/environments/env"},
		},
	}
	require.Equal(t, expectedProperties, resource.Properties)

	// Verify that the DeploymentTemplate contains the expected properties.
	require.Equal(t, string(template), status.Template)
	require.Equal(t, "{}", status.Parameters)
	require.Equal(t, string(generateDefaultProviderConfig()), status.ProviderConfig)
	require.Equal(t, "deploymenttemplate-withresources.bicep", status.RootFileName)

	err = client.Delete(ctx, deploymentTemplate)
	require.NoError(t, err)

	waitForDeploymentTemplateStateDeleting(t, client, name, nil)

	dependencyStatus = waitForDeploymentResourceStateDeleting(t, client, dependencyName, nil)

	// Delete the environment.
	deleteEnvironment(radius, "default", "env")

	// Complete the delete operation on the DeploymentResource.
	radius.CompleteOperation(dependencyStatus.Operation.ResumeToken, nil)

	waitForDeploymentResourceDeleted(t, client, dependencyName)
	waitForDeploymentTemplateStateDeleted(t, client, name)
}

func waitForDeploymentTemplateStateUpdating(t *testing.T, client client.Client, name types.NamespacedName, oldOperation *radappiov1alpha3.ResourceOperation) *radappiov1alpha3.DeploymentTemplateStatus {
	ctx := testcontext.New(t)

	logger := t
	status := &radappiov1alpha3.DeploymentTemplateStatus{}
	require.EventuallyWithT(t, func(t *assert.CollectT) {
		logger.Logf("Fetching DeploymentTemplate: %+v", name)
		current := &radappiov1alpha3.DeploymentTemplate{
			Status: radappiov1alpha3.DeploymentTemplateStatus{
				Phrase: radappiov1alpha3.DeploymentTemplatePhrase(radappiov1alpha3.DeploymentResourcePhraseDeleting),
			},
		}
		err := client.Get(ctx, name, current)
		require.NoError(t, err)

		status = &current.Status
		logger.Logf("DeploymentTemplate.Status: %+v", current.Status)
		assert.Equal(t, status.ObservedGeneration, current.Generation, "Status is not updated")

		if assert.Equal(t, radappiov1alpha3.DeploymentTemplatePhraseUpdating, current.Status.Phrase) {
			assert.NotEmpty(t, current.Status.Operation)
			assert.NotEqual(t, oldOperation, current.Status.Operation)
		}

	}, deploymentTemplateTestWaitDuration, deploymentTemplateTestWaitInterval, "failed to enter updating state")

	return status
}

func waitForDeploymentTemplateStateReady(t *testing.T, client client.Client, name types.NamespacedName) *radappiov1alpha3.DeploymentTemplateStatus {
	ctx := testcontext.New(t)

	logger := t
	status := &radappiov1alpha3.DeploymentTemplateStatus{}
	require.EventuallyWithTf(t, func(t *assert.CollectT) {
		logger.Logf("Fetching DeploymentTemplate: %+v", name)
		current := &radappiov1alpha3.DeploymentTemplate{}
		err := client.Get(ctx, name, current)
		require.NoError(t, err)

		status = &current.Status
		logger.Logf("DeploymentTemplate.Status: %+v", current.Status)
		assert.Equal(t, status.ObservedGeneration, current.Generation, "Status is not updated")

		if assert.Equal(t, radappiov1alpha3.DeploymentTemplatePhraseReady, current.Status.Phrase) {
			assert.Empty(t, current.Status.Operation)
		}
	}, deploymentTemplateTestWaitDuration, deploymentTemplateTestWaitInterval, "failed to enter ready state")

	return status
}

func waitForDeploymentTemplateStateDeleting(t *testing.T, client client.Client, name types.NamespacedName, oldOperation *radappiov1alpha3.ResourceOperation) *radappiov1alpha3.DeploymentTemplateStatus {
	ctx := testcontext.New(t)

	logger := t
	status := &radappiov1alpha3.DeploymentTemplateStatus{}
	require.EventuallyWithTf(t, func(t *assert.CollectT) {
		logger.Logf("Fetching DeploymentTemplate: %+v", name)
		current := &radappiov1alpha3.DeploymentTemplate{}
		err := client.Get(ctx, name, current)
		assert.NoError(t, err)

		status = &current.Status
		logger.Logf("DeploymentTemplate.Status: %+v", current.Status)
		assert.Equal(t, status.ObservedGeneration, current.Generation, "Status is not updated")

		assert.Equal(t, radappiov1alpha3.DeploymentTemplatePhraseDeleting, current.Status.Phrase)
	}, deploymentTemplateTestWaitDuration, deploymentTemplateTestWaitInterval, "failed to enter deleting state")

	return status
}

func waitForDeploymentTemplateStateDeleted(t *testing.T, client client.Client, name types.NamespacedName) {
	ctx := testcontext.New(t)

	logger := t
	require.Eventuallyf(t, func() bool {
		logger.Logf("Fetching DeploymentTemplate: %+v", name)
		current := &radappiov1alpha3.DeploymentTemplate{}
		err := client.Get(ctx, name, current)
		if apierrors.IsNotFound(err) {
			return true
		}

		logger.Logf("DeploymentTemplate.Status: %+v", current.Status)
		return false

	}, deploymentTemplateTestWaitDuration, deploymentTemplateTestWaitInterval, "DeploymentTemplate still exists")
}
