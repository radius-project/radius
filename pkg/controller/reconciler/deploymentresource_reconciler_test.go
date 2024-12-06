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
	"fmt"
	"testing"
	"time"

	radappiov1alpha3 "github.com/radius-project/radius/pkg/controller/api/radapp.io/v1alpha3"
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
	DeploymentResourceTestWaitDuration            = time.Second * 10
	DeploymentResourceTestWaitInterval            = time.Second * 1
	DeploymentResourceTestControllerDelayInterval = time.Millisecond * 100

	TestDeploymentResourceNamespace           = "deploymentresource-basic"
	TestDeploymentResourceName                = "test-deploymentresource"
	TestDeploymentResourceRadiusResourceGroup = "default-deploymentresource-basic"
)

var (
	TestDeploymentResourceScope = fmt.Sprintf("/planes/radius/local/resourcegroups/%s", TestDeploymentResourceRadiusResourceGroup)
	TestDeploymentResourceID    = fmt.Sprintf("%s/providers/Microsoft.Resources/deployments/%s", TestDeploymentResourceScope, TestDeploymentResourceName)
)

func SetupDeploymentResourceTest(t *testing.T) (*mockRadiusClient, client.Client) {
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

func Test_DeploymentResourceReconciler_Basic(t *testing.T) {
	ctx := testcontext.New(t)
	_, client := SetupDeploymentResourceTest(t)

	name := types.NamespacedName{Namespace: TestDeploymentResourceNamespace, Name: TestDeploymentResourceName}
	err := client.Create(ctx, &corev1.Namespace{ObjectMeta: ctrl.ObjectMeta{Name: name.Namespace}})
	require.NoError(t, err)

	deployment := makeDeploymentResource(name, TestDeploymentResourceID)
	err = client.Create(ctx, deployment)
	require.NoError(t, err)

	// Deployment will update after operation completes
	status := waitForDeploymentResourceStateReady(t, client, name)
	require.Equal(t, TestDeploymentResourceID, status.Id)

	err = client.Delete(ctx, deployment)
	require.NoError(t, err)

	// Now deleting of the DeploymentResource object can complete.
	waitForDeploymentResourceDeleted(t, client, name)
}

func waitForDeploymentResourceStateReady(t *testing.T, client client.Client, name types.NamespacedName) *radappiov1alpha3.DeploymentResourceStatus {
	ctx := testcontext.New(t)

	logger := t
	status := &radappiov1alpha3.DeploymentResourceStatus{}
	require.EventuallyWithTf(t, func(t *assert.CollectT) {
		logger.Logf("Fetching DeploymentResource: %+v", name)
		current := &radappiov1alpha3.DeploymentResource{}
		err := client.Get(ctx, name, current)
		require.NoError(t, err)

		status = &current.Status
		logger.Logf("DeploymentResource.Status: %+v", current.Status)
		if assert.Equal(t, radappiov1alpha3.DeploymentResourcePhraseReady, current.Status.Phrase) {
			assert.Empty(t, current.Status.Operation)
		}
	}, DeploymentResourceTestWaitDuration, DeploymentResourceTestWaitInterval, "failed to enter ready state")

	return status
}

func waitForDeploymentResourceStateDeleting(t *testing.T, client client.Client, name types.NamespacedName, oldOperation *radappiov1alpha3.ResourceOperation) *radappiov1alpha3.DeploymentResourceStatus {
	ctx := testcontext.New(t)

	logger := t
	status := &radappiov1alpha3.DeploymentResourceStatus{}
	require.EventuallyWithTf(t, func(t *assert.CollectT) {
		logger.Logf("Fetching DeploymentResource: %+v", name)
		current := &radappiov1alpha3.DeploymentResource{}
		err := client.Get(ctx, name, current)
		assert.NoError(t, err)

		status = &current.Status
		logger.Logf("DeploymentResource.Status: %+v", current.Status)
		assert.Equal(t, status.ObservedGeneration, current.Generation, "Status is not updated")

		if assert.Equal(t, radappiov1alpha3.DeploymentResourcePhraseDeleting, current.Status.Phrase) {
			assert.NotEmpty(t, current.Status.Operation)
			assert.NotEqual(t, oldOperation, current.Status.Operation)
		}
	}, DeploymentResourceTestWaitDuration, DeploymentResourceTestWaitInterval, "failed to enter deleting state")

	return status
}

func waitForDeploymentResourceDeleted(t *testing.T, client client.Client, name types.NamespacedName) {
	ctx := testcontext.New(t)

	logger := t
	require.Eventuallyf(t, func() bool {
		logger.Logf("Fetching DeploymentResource: %+v", name)
		current := &radappiov1alpha3.DeploymentResource{}
		err := client.Get(ctx, name, current)
		if apierrors.IsNotFound(err) {
			return true
		}

		logger.Logf("DeploymentResource.Status: %+v", current.Status)
		return false

	}, DeploymentResourceTestWaitDuration, DeploymentResourceTestWaitInterval, "DeploymentResource still exists")
}
