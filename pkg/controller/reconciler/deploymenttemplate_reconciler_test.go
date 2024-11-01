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
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/radius-project/radius/pkg/cli/clients_new/generated"
	radappiov1alpha3 "github.com/radius-project/radius/pkg/controller/api/radapp.io/v1alpha3"
	"github.com/radius-project/radius/test/testcontext"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	DeploymentTemplateTestWaitDuration            = time.Second * 10
	DeploymentTemplateTestWaitInterval            = time.Second * 1
	DeploymentTemplateTestControllerDelayInterval = time.Millisecond * 100

	TestDeploymentTemplateNamespace           = "DeploymentTemplate-basic"
	TestDeploymentTemplateName                = "test-DeploymentTemplate"
	TestDeploymentTemplateRadiusResourceGroup = "default-DeploymentTemplate-basic"
)

var (
	TestDeploymentTemplateScope = fmt.Sprintf("/planes/radius/local/resourcegroups/%s", TestDeploymentTemplateRadiusResourceGroup)
	TestDeploymentTemplateID    = fmt.Sprintf("%s/providers/Microsoft.Resources/deployments/%s", TestDeploymentTemplateScope, TestDeploymentTemplateName)
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
	})
	require.NoError(t, err)

	radius := NewMockRadiusClient()
	err = (&DeploymentTemplateReconciler{
		Client:        mgr.GetClient(),
		Scheme:        mgr.GetScheme(),
		EventRecorder: mgr.GetEventRecorderFor("DeploymentTemplate-controller"),
		Radius:        radius,
		DelayInterval: DeploymentTemplateTestControllerDelayInterval,
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

	name := types.NamespacedName{Namespace: "DeploymentTemplate-basic", Name: "test-DeploymentTemplate"}
	err := client.Create(ctx, &corev1.Namespace{ObjectMeta: ctrl.ObjectMeta{Name: name.Namespace}})
	require.NoError(t, err)

	deployment := makeDeploymentTemplate(name, map[string]any{})
	err = client.Create(ctx, deployment)
	require.NoError(t, err)

	// Deployment will be waiting for environment to be created.
	createEnvironment(radius, "default")

	// Deployment will be waiting for template to complete provisioning.
	status := waitForDeploymentTemplateStateUpdating(t, client, name, nil)

	scope, err := ParseDeploymentScopeFromProviderConfig(status.ProviderConfig)
	require.NoError(t, err)
	require.Equal(t, "/planes/radius/local/resourcegroups/default-DeploymentTemplate-basic", scope)

	radius.CompleteOperation(status.Operation.ResumeToken, nil)

	// Deployment will update after operation completes
	status = waitForDeploymentTemplateStateReady(t, client, name)
	require.Equal(t, "/planes/radius/local/resourcegroups/default-DeploymentTemplate-basic/providers/Microsoft.Resources/deployments/test-DeploymentTemplate", status.Resource)

	resource, err := radius.Resources(scope, "Microsoft.Resources/deployments").Get(ctx, name.Name)
	require.NoError(t, err)

	expectedProperties := map[string]any{
		"mode":       "Incremental",
		"parameters": map[string]map[string]any{},
		"providerConfig": map[string]any{
			"deployments": map[string]any{
				"type": "Microsoft.Resources",
				"value": map[string]any{
					"scope": "/planes/radius/local/resourcegroups/default-DeploymentTemplate-basic",
				},
			},
			"radius": map[string]any{
				"type": "Radius",
				"value": map[string]any{
					"scope": "/planes/radius/local/resourcegroups/default-DeploymentTemplate-basic",
				},
			},
		}, "template": map[string]any{},
	}
	require.Equal(t, expectedProperties, resource.Properties)

	err = client.Delete(ctx, deployment)
	require.NoError(t, err)

	// Deletion of the DeploymentTemplate is in progress.
	status = waitForDeploymentTemplateStateDeleting(t, client, name, nil)
	radius.CompleteOperation(status.Operation.ResumeToken, nil)

	// Now deleting of the DeploymentTemplate object can complete.
	waitForDeploymentTemplateDeleted(t, client, name)
}

func Test_DeploymentTemplateReconciler_FailureRecovery(t *testing.T) {
	// This test tests our ability to recover from failed operations inside Radius.
	//
	// We use the mock client to simulate the failure of update and delete operations
	// and verify that the controller will (eventually) retry these operations.

	ctx := testcontext.New(t)
	radius, client := SetupDeploymentTemplateTest(t)

	name := types.NamespacedName{Namespace: "DeploymentTemplate-failure-recovery", Name: "test-DeploymentTemplate-failure-recovery"}
	err := client.Create(ctx, &corev1.Namespace{ObjectMeta: ctrl.ObjectMeta{Name: name.Namespace}})
	require.NoError(t, err)

	deployment := makeDeploymentTemplate(name, map[string]any{})
	err = client.Create(ctx, deployment)
	require.NoError(t, err)

	// Deployment will be waiting for environment to be created.
	createEnvironment(radius, "default")

	// Deployment will be waiting for template to complete provisioning.
	status := waitForDeploymentTemplateStateUpdating(t, client, name, nil)

	// Complete the operation, but make it fail.
	operation := status.Operation
	radius.CompleteOperation(status.Operation.ResumeToken, func(state *operationState) {
		state.err = errors.New("oops")

		resource, ok := radius.resources[state.resourceID]
		require.True(t, ok, "failed to find resource")

		resource.Properties["provisioningState"] = "Failed"
		state.value = generated.GenericResourcesClientCreateOrUpdateResponse{GenericResource: resource}
	})

	// Deployment should (eventually) start a new provisioning operation
	status = waitForDeploymentTemplateStateUpdating(t, client, name, operation)

	// Complete the operation, successfully this time.
	radius.CompleteOperation(status.Operation.ResumeToken, nil)
	_ = waitForDeploymentTemplateStateReady(t, client, name)

	err = client.Delete(ctx, deployment)
	require.NoError(t, err)

	// Deletion of the deployment is in progress.
	status = waitForDeploymentTemplateStateDeleting(t, client, name, nil)

	// Complete the operation, but make it fail.
	operation = status.Operation
	radius.CompleteOperation(status.Operation.ResumeToken, func(state *operationState) {
		state.err = errors.New("oops")

		resource, ok := radius.resources[state.resourceID]
		require.True(t, ok, "failed to find resource")

		resource.Properties["provisioningState"] = "Failed"
	})

	// Deployment should (eventually) start a new deletion operation
	status = waitForDeploymentTemplateStateDeleting(t, client, name, operation)

	// Complete the operation, successfully this time.
	radius.CompleteOperation(status.Operation.ResumeToken, nil)

	// Now deleting of the deployment object can complete.
	waitForDeploymentTemplateDeleted(t, client, name)
}

func waitForDeploymentTemplateStateUpdating(t *testing.T, client client.Client, name types.NamespacedName, oldOperation *radappiov1alpha3.ResourceOperation) *radappiov1alpha3.DeploymentTemplateStatus {
	ctx := testcontext.New(t)

	logger := t
	status := &radappiov1alpha3.DeploymentTemplateStatus{}
	require.EventuallyWithT(t, func(t *assert.CollectT) {
		logger.Logf("Fetching DeploymentTemplate: %+v", name)
		current := &radappiov1alpha3.DeploymentTemplate{}
		err := client.Get(ctx, name, current)
		require.NoError(t, err)

		status = &current.Status
		logger.Logf("DeploymentTemplate.Status: %+v", current.Status)
		assert.Equal(t, status.ObservedGeneration, current.Generation, "Status is not updated")

		if assert.Equal(t, radappiov1alpha3.DeploymentTemplatePhraseUpdating, current.Status.Phrase) {
			assert.NotEmpty(t, current.Status.Operation)
			assert.NotEqual(t, oldOperation, current.Status.Operation)
		}

	}, DeploymentTemplateTestWaitDuration, DeploymentTemplateTestWaitInterval, "failed to enter updating state")

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
	}, DeploymentTemplateTestWaitDuration, DeploymentTemplateTestWaitInterval, "failed to enter updating state")

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

		if assert.Equal(t, radappiov1alpha3.DeploymentTemplatePhraseDeleting, current.Status.Phrase) {
			assert.NotEmpty(t, current.Status.Operation)
			assert.NotEqual(t, oldOperation, current.Status.Operation)
		}
	}, DeploymentTemplateTestWaitDuration, DeploymentTemplateTestWaitInterval, "failed to enter deleting state")

	return status
}

func waitForDeploymentTemplateDeleted(t *testing.T, client client.Client, name types.NamespacedName) {
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

	}, DeploymentTemplateTestWaitDuration, DeploymentTemplateTestWaitInterval, "DeploymentTemplate still exists")
}
