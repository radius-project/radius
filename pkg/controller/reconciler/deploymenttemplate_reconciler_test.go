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

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
	radappiov1alpha3 "github.com/radius-project/radius/pkg/controller/api/radapp.io/v1alpha3"
	sdkclients "github.com/radius-project/radius/pkg/sdk/clients"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/test/testcontext"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	k8sclient "sigs.k8s.io/controller-runtime/pkg/client"
	crconfig "sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

const (
	deploymentTemplateTestWaitDuration            = time.Second * 10
	deploymentTemplateTestWaitInterval            = time.Second * 1
	deploymentTemplateTestControllerDelayInterval = time.Millisecond * 100
)

func SetupDeploymentTemplateTest(t *testing.T) (*mockRadiusClient, *sdkclients.MockResourceDeploymentsClient, k8sclient.Client) {
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
			SkipNameValidation: to.Ptr(true),
		},

		// Suppress metrics in tests to avoid conflicts.
		Metrics: server.Options{
			BindAddress: "0",
		},
	})
	require.NoError(t, err)

	mockRadiusClient := NewMockRadiusClient()
	mockResourceDeploymentsClient := sdkclients.NewMockResourceDeploymentsClient()

	// Set up DeploymentTemplateReconciler.
	err = (&DeploymentTemplateReconciler{
		Client:                    mgr.GetClient(),
		Scheme:                    mgr.GetScheme(),
		EventRecorder:             mgr.GetEventRecorderFor("deploymenttemplate-controller"),
		Radius:                    mockRadiusClient,
		ResourceDeploymentsClient: mockResourceDeploymentsClient,
		DelayInterval:             deploymentTemplateTestControllerDelayInterval,
	}).SetupWithManager(mgr)
	require.NoError(t, err)

	// Set up DeploymentResourceReconciler.
	err = (&DeploymentResourceReconciler{
		Client:                    mgr.GetClient(),
		Scheme:                    mgr.GetScheme(),
		EventRecorder:             mgr.GetEventRecorderFor("deploymentresource-controller"),
		Radius:                    mockRadiusClient,
		ResourceDeploymentsClient: mockResourceDeploymentsClient,
		DelayInterval:             DeploymentResourceTestControllerDelayInterval,
	}).SetupWithManager(mgr)
	require.NoError(t, err)

	go func() {
		err := mgr.Start(ctx)
		require.NoError(t, err)
	}()

	return mockRadiusClient, mockResourceDeploymentsClient, mgr.GetClient()
}

func Test_DeploymentTemplateReconciler_ComputeHash(t *testing.T) {
	testcases := []struct {
		name               string
		deploymentTemplate *radappiov1alpha3.DeploymentTemplate
		expected           string
	}{
		{
			name: "empty",
			deploymentTemplate: &radappiov1alpha3.DeploymentTemplate{
				Spec: radappiov1alpha3.DeploymentTemplateSpec{},
			},
			expected: "bf21a9e8fbc5a3846fb05b4fa0859e0917b2202f",
		},
		{
			name: "simple",
			deploymentTemplate: &radappiov1alpha3.DeploymentTemplate{
				Spec: radappiov1alpha3.DeploymentTemplateSpec{
					Template:       "{}",
					Parameters:     map[string]string{},
					ProviderConfig: "{}",
				},
			},
			expected: "47ee899e74561942ee36a02ffd80be955e251583",
		},
		{
			name: "complex",
			deploymentTemplate: &radappiov1alpha3.DeploymentTemplate{
				Spec: radappiov1alpha3.DeploymentTemplateSpec{
					Template:       `{"resources":[{"type":"Microsoft.Resources/deployments","apiVersion":"2020-06-01","name":"test-deploymenttemplate-basic","properties":{"mode":"Incremental","template":{},"parameters":{}}}]}`,
					Parameters:     map[string]string{"param1": "value1", "param2": "value2"},
					ProviderConfig: `{"AWS":{"type":"aws","value":{"scope":"scope"}}}`,
				},
			},
			expected: "5c83b7122697599db2a47f2d5f7e29f4b9e3c869",
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			hash, err := computeHash(tc.deploymentTemplate)
			require.NoError(t, err)
			require.Equal(t, tc.expected, hash)
		})
	}
}

func Test_DeploymentTemplateReconciler_IsUpToDate(t *testing.T) {
	testcases := []struct {
		name               string
		deploymentTemplate *radappiov1alpha3.DeploymentTemplate
		expected           bool
	}{
		{
			name: "up-to-date",
			deploymentTemplate: &radappiov1alpha3.DeploymentTemplate{
				Spec: radappiov1alpha3.DeploymentTemplateSpec{
					Template:       "{}",
					Parameters:     map[string]string{},
					ProviderConfig: "{}",
				},
				Status: radappiov1alpha3.DeploymentTemplateStatus{
					StatusHash: "47ee899e74561942ee36a02ffd80be955e251583",
				},
			},
			expected: true,
		},
		{
			name: "not-up-to-date",
			deploymentTemplate: &radappiov1alpha3.DeploymentTemplate{
				Spec: radappiov1alpha3.DeploymentTemplateSpec{
					Template:       "{}",
					Parameters:     map[string]string{},
					ProviderConfig: "{}",
				},
				Status: radappiov1alpha3.DeploymentTemplateStatus{
					StatusHash: "incorrecthash",
				},
			},
			expected: false,
		},
		{
			name: "complex-up-to-date",
			deploymentTemplate: &radappiov1alpha3.DeploymentTemplate{
				Spec: radappiov1alpha3.DeploymentTemplateSpec{
					Template:       `{"resources":[{"type":"Microsoft.Resources/deployments","apiVersion":"2020-06-01","name":"test-deploymenttemplate-basic","properties":{"mode":"Incremental","template":{},"parameters":{}}}]}`,
					Parameters:     map[string]string{"param1": "value1", "param2": "value2"},
					ProviderConfig: `{"AWS":{"type":"aws","value":{"scope":"scope"}}}`,
				},
				Status: radappiov1alpha3.DeploymentTemplateStatus{
					StatusHash: "5c83b7122697599db2a47f2d5f7e29f4b9e3c869",
				},
			},
			expected: true,
		},
		{
			name: "complex-not-up-to-date",
			deploymentTemplate: &radappiov1alpha3.DeploymentTemplate{
				Spec: radappiov1alpha3.DeploymentTemplateSpec{
					Template:       `{"resources":[{"type":"Microsoft.Resources/deployments","apiVersion":"2020-06-01","name":"test-deploymenttemplate-basic","properties":{"mode":"Incremental","template":{},"parameters":{}}}]}`,
					Parameters:     map[string]string{"param1": "value1", "param2": "value2"},
					ProviderConfig: `{"AWS":{"type":"aws","value":{"scope":"scope"}}}`,
				},
				Status: radappiov1alpha3.DeploymentTemplateStatus{
					StatusHash: "incorrecthash",
				},
			},
			expected: false,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			isUpToDate := isUpToDate(tc.deploymentTemplate)
			require.Equal(t, tc.expected, isUpToDate)
		})
	}
}

func Test_ParseDeploymentScopeFromProviderConfig(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name           string
		providerConfig string
		wantScope      string
		wantErr        bool
	}{
		{
			name:           "valid: provider with scope",
			providerConfig: `{"deployments":{"type":"deployments","value":{"scope":"deploymentsscope"}}}`,
			wantScope:      "deploymentsscope",
			wantErr:        false,
		},
		{
			name:           "invalid: deployments scope not present",
			providerConfig: `{"radius":{"type":"radius","value":{"scope":"deploymentsscope"}}}`,
			wantErr:        true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			scope, err := ParseDeploymentScopeFromProviderConfig(tc.providerConfig)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantScope, scope)
		})
	}
}

func Test_DeploymentTemplateReconciler_Basic(t *testing.T) {
	// This test tests the basic functionality of the DeploymentTemplate controller.
	// It creates a DeploymentTemplate (with an empty template field),
	// waits for it to be ready, and then deletes it.
	//
	// This is the same structure as all of the following tests.

	ctx := testcontext.New(t)

	// Set up the test.
	_, mockDeploymentClient, k8sClient := SetupDeploymentTemplateTest(t)
	testNamespace := "deploymenttemplate-basic"
	testName := "test-deploymenttemplate-basic"
	template := "{}"
	parameters := map[string]string{}
	providerConfig, err := sdkclients.NewDefaultProviderConfig(testNamespace).String()
	require.NoError(t, err)

	// Create k8s namespace for the test.
	namespacedName := types.NamespacedName{Namespace: testNamespace, Name: testName}
	err = k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: ctrl.ObjectMeta{Name: testNamespace}})
	require.NoError(t, err)

	// Create the DeploymentTemplate resource.
	deploymentTemplate := makeDeploymentTemplate(namespacedName, template, providerConfig, parameters)
	err = k8sClient.Create(ctx, deploymentTemplate)
	require.NoError(t, err)

	// Wait for the DeploymentTemplate to enter the Updating state.
	status := waitForDeploymentTemplateStateUpdating(t, k8sClient, namespacedName, nil)

	// Complete the operation.
	mockDeploymentClient.CompleteOperation(status.Operation.ResumeToken, nil)

	// DeploymentTemplate should be Ready after the operation completes.
	status = waitForDeploymentTemplateStateReady(t, k8sClient, namespacedName)

	// Verify that the DeploymentTemplate desired state contains the expected properties.
	expectedDeploymentTemplateSpec := &radappiov1alpha3.DeploymentTemplate{
		Spec: radappiov1alpha3.DeploymentTemplateSpec{
			Template:       template,
			Parameters:     parameters,
			ProviderConfig: providerConfig,
		},
	}
	expectedStatusHash, err := computeHash(expectedDeploymentTemplateSpec)
	require.NoError(t, err)
	require.Equal(t, expectedStatusHash, status.StatusHash)

	// Trigger deletion of the DeploymentTemplate.
	err = k8sClient.Delete(ctx, deploymentTemplate)
	require.NoError(t, err)

	// Wait for the DeploymentTemplate to be deleted.
	waitForDeploymentTemplateStateDeleted(t, k8sClient, namespacedName)
}

func Test_DeploymentTemplateReconciler_FailureRecovery(t *testing.T) {
	// This test tests our ability to recover from failed operations inside Radius.
	//
	// We use the mock client to simulate the failure of update and delete operations
	// and verify that the controller will (eventually) retry these operations.

	ctx := testcontext.New(t)

	// Set up the test.
	_, mockDeploymentClient, k8sClient := SetupDeploymentTemplateTest(t)
	testNamespace := "deploymenttemplate-failurerecovery"
	testName := "test-deploymenttemplate-failurerecovery"
	template := "{}"
	parameters := map[string]string{}
	providerConfig, err := sdkclients.NewDefaultProviderConfig(testNamespace).String()
	require.NoError(t, err)

	// Create k8s namespace for the test.
	namespacedName := types.NamespacedName{Namespace: testNamespace, Name: testName}
	err = k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: ctrl.ObjectMeta{Name: testNamespace}})
	require.NoError(t, err)

	// Create the DeploymentTemplate resource.
	deploymentTemplate := makeDeploymentTemplate(namespacedName, template, providerConfig, parameters)
	err = k8sClient.Create(ctx, deploymentTemplate)
	require.NoError(t, err)

	// Wait for the DeploymentTemplate to enter the Updating state.
	status := waitForDeploymentTemplateStateUpdating(t, k8sClient, namespacedName, nil)

	// Complete the operation, but make it fail.
	operation := status.Operation
	mockDeploymentClient.CompleteOperation(status.Operation.ResumeToken, func(state *sdkclients.OperationState) {
		state.Err = errors.New("failure")

		resource, ok := mockDeploymentClient.GetResource(state.ResourceID)
		require.True(t, ok, "failed to find resource")

		resource.Properties.ProvisioningState = to.Ptr(armresources.ProvisioningStateFailed)
		state.Value = sdkclients.ClientCreateOrUpdateResponse{DeploymentExtended: armresources.DeploymentExtended{Properties: resource.Properties}}
	})

	// DeploymentTemplate should (eventually) start a new provisioning operation
	status = waitForDeploymentTemplateStateUpdating(t, k8sClient, namespacedName, operation)

	// Complete the operation, successfully this time.
	mockDeploymentClient.CompleteOperation(status.Operation.ResumeToken, nil)
	status = waitForDeploymentTemplateStateReady(t, k8sClient, namespacedName)

	// Verify that the DeploymentTemplate desired state contains the expected properties.
	expectedDeploymentTemplateSpec := &radappiov1alpha3.DeploymentTemplate{
		Spec: radappiov1alpha3.DeploymentTemplateSpec{
			Template:       template,
			Parameters:     parameters,
			ProviderConfig: providerConfig,
		},
	}
	expectedStatusHash, err := computeHash(expectedDeploymentTemplateSpec)
	require.NoError(t, err)
	require.Equal(t, expectedStatusHash, status.StatusHash)

	// Trigger deletion of the DeploymentTemplate.
	err = k8sClient.Delete(ctx, deploymentTemplate)
	require.NoError(t, err)

	// Wait for the DeploymentTemplate to be deleted.
	waitForDeploymentTemplateStateDeleted(t, k8sClient, namespacedName)
}

func Test_DeploymentTemplateReconciler_WithResources(t *testing.T) {
	// This test tests the ability to handle deployments of
	// resources created by the DeploymentTemplate.

	ctx := testcontext.New(t)

	// Set up the test.
	_, mockDeploymentClient, k8sClient := SetupDeploymentTemplateTest(t)
	testNamespace := "deploymenttemplate-withresources"
	testName := "test-deploymenttemplate-withresources"
	template := readFileIntoTemplate(t, "deploymenttemplate-withresources.json")
	parameters := map[string]string{}
	providerConfig, err := sdkclients.NewDefaultProviderConfig(testNamespace).String()
	require.NoError(t, err)

	// Create k8s namespace for the test.
	namespacedName := types.NamespacedName{Namespace: testNamespace, Name: testName}
	err = k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: ctrl.ObjectMeta{Name: testNamespace}})
	require.NoError(t, err)

	// Create the DeploymentTemplate resource.
	deploymentTemplate := makeDeploymentTemplate(namespacedName, template, providerConfig, parameters)
	err = k8sClient.Create(ctx, deploymentTemplate)
	require.NoError(t, err)

	// Wait for the DeploymentTemplate to enter the Updating state.
	status := waitForDeploymentTemplateStateUpdating(t, k8sClient, namespacedName, nil)

	// Complete the operation.
	mockDeploymentClient.CompleteOperation(status.Operation.ResumeToken, func(state *sdkclients.OperationState) {
		resource, ok := mockDeploymentClient.GetResource(state.ResourceID)
		require.True(t, ok, "failed to find resource")

		resource.Properties.OutputResources = []*armresources.ResourceReference{
			{ID: to.Ptr("/planes/radius/local/resourceGroups/deploymenttemplate-update/providers/Applications.Core/environments/deploymenttemplate-withresources-env")},
		}
		state.Value = sdkclients.ClientCreateOrUpdateResponse{DeploymentExtended: armresources.DeploymentExtended{Properties: resource.Properties}}
	})

	// DeploymentTemplate should be ready after the operation completes.
	status = waitForDeploymentTemplateStateReady(t, k8sClient, namespacedName)

	// The dependencies (DeploymentResource resources) should be created.
	dependencyName := types.NamespacedName{Namespace: namespacedName.Namespace, Name: "deploymenttemplate-withresources-env"}
	dependencyStatus := waitForDeploymentResourceStateReady(t, k8sClient, dependencyName)
	require.Equal(t, "/planes/radius/local/resourceGroups/deploymenttemplate-update/providers/Applications.Core/environments/deploymenttemplate-withresources-env", dependencyStatus.Id)

	// Verify that the DeploymentTemplate desired state contains the expected properties.
	expectedDeploymentTemplateSpec := &radappiov1alpha3.DeploymentTemplate{
		Spec: radappiov1alpha3.DeploymentTemplateSpec{
			Template:       template,
			Parameters:     parameters,
			ProviderConfig: providerConfig,
		},
	}
	expectedStatusHash, err := computeHash(expectedDeploymentTemplateSpec)
	require.NoError(t, err)
	require.Equal(t, expectedStatusHash, status.StatusHash)

	// Trigger deletion of the DeploymentTemplate.
	err = k8sClient.Delete(ctx, deploymentTemplate)
	require.NoError(t, err)

	// The DeploymentTemplate should be in the deleting state.
	waitForDeploymentTemplateStateDeleting(t, k8sClient, namespacedName)

	// Get the status of the dependency (DeploymentResource resource).
	dependencyStatus = waitForDeploymentResourceStateDeleting(t, k8sClient, dependencyName, nil)

	// Complete the delete operation on the DeploymentResource.
	mockDeploymentClient.CompleteOperation(dependencyStatus.Operation.ResumeToken, nil)

	// Wait for the DeploymentTemplate to be deleted.
	waitForDeploymentResourceDeleted(t, k8sClient, dependencyName)
	waitForDeploymentTemplateStateDeleted(t, k8sClient, namespacedName)
}

func Test_DeploymentTemplateReconciler_Update(t *testing.T) {
	// This test tests our ability to update a DeploymentTemplate.
	// We create a DeploymentTemplate, update it, and verify that the Radius resource is updated accordingly.

	ctx := testcontext.New(t)

	// Set up the test.
	_, mockDeploymentClient, k8sClient := SetupDeploymentTemplateTest(t)
	testNamespace := "deploymenttemplate-update"
	testName := "test-deploymenttemplate-update"
	template := readFileIntoTemplate(t, "deploymenttemplate-update-1.json")
	parameters := map[string]string{}
	providerConfig, err := sdkclients.NewDefaultProviderConfig(testNamespace).String()
	require.NoError(t, err)

	// Create k8s namespace for the test.
	namespacedName := types.NamespacedName{Namespace: testNamespace, Name: testName}
	err = k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: ctrl.ObjectMeta{Name: testNamespace}})
	require.NoError(t, err)

	// Create the DeploymentTemplate resource.
	deploymentTemplate := makeDeploymentTemplate(namespacedName, template, providerConfig, parameters)
	err = k8sClient.Create(ctx, deploymentTemplate)
	require.NoError(t, err)

	// Wait for the DeploymentTemplate to enter the Updating state.
	status := waitForDeploymentTemplateStateUpdating(t, k8sClient, namespacedName, nil)

	// Complete the operation.
	mockDeploymentClient.CompleteOperation(status.Operation.ResumeToken, func(state *sdkclients.OperationState) {
		resource, ok := mockDeploymentClient.GetResource(state.ResourceID)
		require.True(t, ok, "failed to find resource")

		resource.Properties.OutputResources = []*armresources.ResourceReference{
			{ID: to.Ptr("/planes/radius/local/resourceGroups/deploymenttemplate-update/providers/Applications.Core/environments/deploymenttemplate-update-env")},
		}
		state.Value = sdkclients.ClientCreateOrUpdateResponse{DeploymentExtended: armresources.DeploymentExtended{Properties: resource.Properties}}
	})

	// DeploymentTemplate should be ready after the operation completes.
	status = waitForDeploymentTemplateStateReady(t, k8sClient, namespacedName)

	// The dependencies (DeploymentResource resources) should be created.
	dependencyName := types.NamespacedName{Namespace: namespacedName.Namespace, Name: "deploymenttemplate-update-env"}
	dependencyStatus := waitForDeploymentResourceStateReady(t, k8sClient, dependencyName)
	require.Equal(t, "/planes/radius/local/resourceGroups/deploymenttemplate-update/providers/Applications.Core/environments/deploymenttemplate-update-env", dependencyStatus.Id)

	// Verify that the DeploymentTemplate desired state contains the expected properties.
	expectedDeploymentTemplateSpec := &radappiov1alpha3.DeploymentTemplate{
		Spec: radappiov1alpha3.DeploymentTemplateSpec{
			Template:       template,
			Parameters:     parameters,
			ProviderConfig: providerConfig,
		},
	}
	expectedStatusHash, err := computeHash(expectedDeploymentTemplateSpec)
	require.NoError(t, err)
	require.Equal(t, expectedStatusHash, status.StatusHash)

	// Now, we will re-deploy the DeploymentTemplate with a new template.

	// Get the DeploymentTemplate resource.
	newDeploymentTemplate := radappiov1alpha3.DeploymentTemplate{}
	err = k8sClient.Get(ctx, namespacedName, &newDeploymentTemplate)
	require.NoError(t, err)

	// Update the template field on the DeploymentTemplate.
	template = readFileIntoTemplate(t, "deploymenttemplate-update-2.json")
	newDeploymentTemplate.Spec.Template = string(template)

	// Update the DeploymentTemplate resource.
	err = k8sClient.Update(ctx, &newDeploymentTemplate)
	require.NoError(t, err)

	// Now, the DeploymentTemplate should re-enter the Updating state.
	status = waitForDeploymentTemplateStateUpdating(t, k8sClient, namespacedName, nil)

	// Complete the operation again.
	mockDeploymentClient.CompleteOperation(status.Operation.ResumeToken, func(state *sdkclients.OperationState) {
		resource, ok := mockDeploymentClient.GetResource(state.ResourceID)
		require.True(t, ok, "failed to find resource")

		resource.Properties.OutputResources = []*armresources.ResourceReference{
			{ID: to.Ptr("/planes/radius/local/resourceGroups/deploymenttemplate-update/providers/Applications.Core/environments/deploymenttemplate-update-env")},
		}
		state.Value = sdkclients.ClientCreateOrUpdateResponse{DeploymentExtended: armresources.DeploymentExtended{Properties: resource.Properties}}
	})

	// DeploymentTemplate should be Ready again after the operation completes.
	status = waitForDeploymentTemplateStateReady(t, k8sClient, namespacedName)

	// The dependencies (DeploymentResource resources) should also be Ready.
	dependencyName = types.NamespacedName{Namespace: namespacedName.Namespace, Name: "deploymenttemplate-update-env"}
	dependencyStatus = waitForDeploymentResourceStateReady(t, k8sClient, dependencyName)
	require.Equal(t, "/planes/radius/local/resourceGroups/deploymenttemplate-update/providers/Applications.Core/environments/deploymenttemplate-update-env", dependencyStatus.Id)

	// Verify that the DeploymentTemplate contains the expected properties.
	expectedDeploymentTemplateSpec = &radappiov1alpha3.DeploymentTemplate{
		Spec: radappiov1alpha3.DeploymentTemplateSpec{
			Template:       template,
			Parameters:     parameters,
			ProviderConfig: providerConfig,
		},
	}
	expectedStatusHash, err = computeHash(expectedDeploymentTemplateSpec)
	require.NoError(t, err)
	require.Equal(t, expectedStatusHash, status.StatusHash)

	// Trigger deletion of the DeploymentTemplate.
	err = k8sClient.Delete(ctx, deploymentTemplate)
	require.NoError(t, err)

	// The DeploymentTemplate should be in the deleting state.
	waitForDeploymentTemplateStateDeleting(t, k8sClient, namespacedName)

	// Get the status of the dependency (DeploymentResource resource).
	dependencyStatus = waitForDeploymentResourceStateDeleting(t, k8sClient, dependencyName, nil)

	// Complete the delete operation on the DeploymentResource.
	mockDeploymentClient.CompleteOperation(dependencyStatus.Operation.ResumeToken, nil)

	// Wait for the DeploymentTemplate to be deleted.
	waitForDeploymentResourceDeleted(t, k8sClient, dependencyName)
	waitForDeploymentTemplateStateDeleted(t, k8sClient, namespacedName)
}

func Test_DeploymentTemplateReconciler_OutputResources(t *testing.T) {
	// This test tests the ability to perform diff detection on
	// the OutputResources field of the DeploymentTemplate.
	// We create a DeploymentTemplate with some resources,
	// update the DeploymentTemplate to remove some resources,
	// and verify that the diff is correct.

	ctx := testcontext.New(t)

	// Set up the test.
	_, mockDeploymentClient, k8sClient := SetupDeploymentTemplateTest(t)
	testNamespace := "deploymenttemplate-outputresources"
	testName := "test-deploymenttemplate-outputresources"
	template := readFileIntoTemplate(t, "deploymenttemplate-outputresources-1.json")
	parameters := map[string]string{}
	providerConfig, err := sdkclients.NewDefaultProviderConfig(testNamespace).String()
	require.NoError(t, err)

	// Create k8s namespace for the test.
	namespacedName := types.NamespacedName{Namespace: testNamespace, Name: testName}
	err = k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: ctrl.ObjectMeta{Name: testNamespace}})
	require.NoError(t, err)

	// Create the DeploymentTemplate resource.
	deploymentTemplate := makeDeploymentTemplate(namespacedName, template, providerConfig, parameters)
	err = k8sClient.Create(ctx, deploymentTemplate)
	require.NoError(t, err)

	// Wait for the DeploymentTemplate to enter the Updating state.
	status := waitForDeploymentTemplateStateUpdating(t, k8sClient, namespacedName, nil)

	// Complete the operation.
	mockDeploymentClient.CompleteOperation(status.Operation.ResumeToken, func(state *sdkclients.OperationState) {
		resource, ok := mockDeploymentClient.GetResource(state.ResourceID)
		require.True(t, ok, "failed to find resource")

		resource.Properties.OutputResources = []*armresources.ResourceReference{
			{ID: to.Ptr("/planes/radius/local/resourceGroups/deploymenttemplate-outputresources/providers/Applications.Core/environments/deploymenttemplate-outputresources-environment")},
			{ID: to.Ptr("/planes/radius/local/resourceGroups/deploymenttemplate-outputresources/providers/Applications.Core/applications/deploymenttemplate-outputresources-application")},
			{ID: to.Ptr("/planes/radius/local/resourceGroups/deploymenttemplate-outputresources/providers/Applications.Core/containers/deploymenttemplate-outputresources-container")},
		}
		state.Value = sdkclients.ClientCreateOrUpdateResponse{DeploymentExtended: armresources.DeploymentExtended{Properties: resource.Properties}}
	})

	// DeploymentTemplate should be ready after the operation completes.
	status = waitForDeploymentTemplateStateReady(t, k8sClient, namespacedName)

	// The dependencies (DeploymentResource resources) should be created.
	dependencies := []struct {
		resourceID     string
		namespacedName types.NamespacedName
	}{
		{
			resourceID:     "/planes/radius/local/resourceGroups/deploymenttemplate-outputresources/providers/Applications.Core/environments/deploymenttemplate-outputresources-environment",
			namespacedName: types.NamespacedName{Namespace: namespacedName.Namespace, Name: "deploymenttemplate-outputresources-environment"},
		},
		{
			resourceID:     "/planes/radius/local/resourceGroups/deploymenttemplate-outputresources/providers/Applications.Core/applications/deploymenttemplate-outputresources-application",
			namespacedName: types.NamespacedName{Namespace: namespacedName.Namespace, Name: "deploymenttemplate-outputresources-application"},
		},
		{
			resourceID:     "/planes/radius/local/resourceGroups/deploymenttemplate-outputresources/providers/Applications.Core/containers/deploymenttemplate-outputresources-container",
			namespacedName: types.NamespacedName{Namespace: namespacedName.Namespace, Name: "deploymenttemplate-outputresources-container"},
		},
	}
	for _, dependency := range dependencies {
		dependencyStatus := waitForDeploymentResourceStateReady(t, k8sClient, dependency.namespacedName)
		require.Equal(t, dependency.resourceID, dependencyStatus.Id)
	}

	// Verify that the DeploymentTemplate desired state contains the expected properties.
	expectedDeploymentTemplateSpec := &radappiov1alpha3.DeploymentTemplate{
		Spec: radappiov1alpha3.DeploymentTemplateSpec{
			Template:       template,
			Parameters:     parameters,
			ProviderConfig: providerConfig,
		},
	}
	expectedStatusHash, err := computeHash(expectedDeploymentTemplateSpec)
	require.NoError(t, err)
	require.Equal(t, expectedStatusHash, status.StatusHash)

	// Now, we will re-deploy the DeploymentTemplate with a new template.
	// This template has the container resource removed.

	// Get the DeploymentTemplate resource.
	newDeploymentTemplate := radappiov1alpha3.DeploymentTemplate{}
	err = k8sClient.Get(ctx, namespacedName, &newDeploymentTemplate)
	require.NoError(t, err)

	// Update the template field on the DeploymentTemplate.
	template = readFileIntoTemplate(t, "deploymenttemplate-outputresources-2.json")
	newDeploymentTemplate.Spec.Template = string(template)

	// Update the DeploymentTemplate resource.
	err = k8sClient.Update(ctx, &newDeploymentTemplate)
	require.NoError(t, err)

	// Now, the DeploymentTemplate should re-enter the Updating state.
	status = waitForDeploymentTemplateStateUpdating(t, k8sClient, namespacedName, nil)

	// Complete the operation again, with a different set of output resources.
	mockDeploymentClient.CompleteOperation(status.Operation.ResumeToken, func(state *sdkclients.OperationState) {
		resource, ok := mockDeploymentClient.GetResource(state.ResourceID)
		require.True(t, ok, "failed to find resource")

		resource.Properties.OutputResources = []*armresources.ResourceReference{
			{ID: to.Ptr("/planes/radius/local/resourceGroups/deploymenttemplate-outputresources/providers/Applications.Core/environments/deploymenttemplate-outputresources-environment")},
			{ID: to.Ptr("/planes/radius/local/resourceGroups/deploymenttemplate-outputresources/providers/Applications.Core/applications/deploymenttemplate-outputresources-application")},
		}
		state.Value = sdkclients.ClientCreateOrUpdateResponse{DeploymentExtended: armresources.DeploymentExtended{Properties: resource.Properties}}
	})

	// Complete the delete operation on the container resource.
	dependencyName := types.NamespacedName{Namespace: namespacedName.Namespace, Name: "deploymenttemplate-outputresources-container"}
	dependencyStatus := waitForDeploymentResourceStateDeleting(t, k8sClient, dependencyName, nil)
	mockDeploymentClient.CompleteOperation(dependencyStatus.Operation.ResumeToken, nil)

	// DeploymentTemplate should be Ready again after the operation completes.
	status = waitForDeploymentTemplateStateReady(t, k8sClient, namespacedName)

	// The dependencies (DeploymentResource resources) should be in the Ready state.
	dependencies = []struct {
		resourceID     string
		namespacedName types.NamespacedName
	}{
		{
			resourceID:     "/planes/radius/local/resourceGroups/deploymenttemplate-outputresources/providers/Applications.Core/environments/deploymenttemplate-outputresources-environment",
			namespacedName: types.NamespacedName{Namespace: namespacedName.Namespace, Name: "deploymenttemplate-outputresources-environment"},
		},
		{
			resourceID:     "/planes/radius/local/resourceGroups/deploymenttemplate-outputresources/providers/Applications.Core/applications/deploymenttemplate-outputresources-application",
			namespacedName: types.NamespacedName{Namespace: namespacedName.Namespace, Name: "deploymenttemplate-outputresources-application"},
		},
	}
	for _, dependency := range dependencies {
		dependencyStatus := waitForDeploymentResourceStateReady(t, k8sClient, dependency.namespacedName)
		require.Equal(t, dependency.resourceID, dependencyStatus.Id)
	}

	// Assert that the container resource has been deleted.
	dependencyName = types.NamespacedName{Namespace: namespacedName.Namespace, Name: "deploymenttemplate-outputresources-container"}
	waitForDeploymentResourceDeleted(t, k8sClient, dependencyName)

	// Check the cluster for the container resource, it should not exist.
	err = k8sClient.Get(ctx, dependencyName, &radappiov1alpha3.DeploymentResource{})
	require.True(t, apierrors.IsNotFound(err), "expected DeploymentResource to be deleted")

	// Verify that the DeploymentTemplate contains the expected properties.
	expectedDeploymentTemplateSpec = &radappiov1alpha3.DeploymentTemplate{
		Spec: radappiov1alpha3.DeploymentTemplateSpec{
			Template:       template,
			Parameters:     map[string]string{},
			ProviderConfig: providerConfig,
		},
	}
	expectedStatusHash, err = computeHash(expectedDeploymentTemplateSpec)
	require.NoError(t, err)
	require.Equal(t, expectedStatusHash, status.StatusHash)

	// Trigger deletion of the DeploymentTemplate.
	err = k8sClient.Delete(ctx, deploymentTemplate)
	require.NoError(t, err)

	// The DeploymentTemplate should be in the deleting state.
	waitForDeploymentTemplateStateDeleting(t, k8sClient, namespacedName)

	// Wait for all of the dependencies (DeploymentResource resources) to be deleted.
	for _, dependency := range dependencies {
		dependencyStatus := waitForDeploymentResourceStateDeleting(t, k8sClient, dependency.namespacedName, nil)
		mockDeploymentClient.CompleteOperation(dependencyStatus.Operation.ResumeToken, nil)
		waitForDeploymentResourceDeleted(t, k8sClient, dependency.namespacedName)
	}

	// Wait for the DeploymentTemplate to be deleted.
	waitForDeploymentTemplateStateDeleted(t, k8sClient, namespacedName)
}

func waitForDeploymentTemplateStateUpdating(t *testing.T, client k8sclient.Client, name types.NamespacedName, oldOperation *radappiov1alpha3.ResourceOperation) *radappiov1alpha3.DeploymentTemplateStatus {
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

func waitForDeploymentTemplateStateReady(t *testing.T, client k8sclient.Client, name types.NamespacedName) *radappiov1alpha3.DeploymentTemplateStatus {
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

func waitForDeploymentTemplateStateDeleting(t *testing.T, client k8sclient.Client, name types.NamespacedName) *radappiov1alpha3.DeploymentTemplateStatus {
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

func waitForDeploymentTemplateStateDeleted(t *testing.T, client k8sclient.Client, name types.NamespacedName) {
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

func readFileIntoTemplate(t *testing.T, filename string) string {
	fileContent, err := os.ReadFile(path.Join("testdata", filename))
	require.NoError(t, err)
	templateMap := map[string]any{}
	err = json.Unmarshal(fileContent, &templateMap)
	require.NoError(t, err)
	template, err := json.MarshalIndent(templateMap, "", "  ")
	require.NoError(t, err)
	return string(template)
}
