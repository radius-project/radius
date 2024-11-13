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
	"fmt"
	"testing"
	"time"

	"github.com/radius-project/radius/pkg/cli/clients_new/generated"
	radappiov1alpha3 "github.com/radius-project/radius/pkg/controller/api/radapp.io/v1alpha3"
	"github.com/radius-project/radius/pkg/corerp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/kubernetes"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/test/testcontext"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	crconfig "sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

const (
	deploymentTestWaitDuration            = time.Second * 10
	deploymentTestWaitInterval            = time.Second * 1
	deploymentTestControllerDelayInterval = time.Millisecond * 100
)

func SetupDeploymentTest(t *testing.T) (*mockRadiusClient, client.Client) {
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
	err = (&DeploymentReconciler{
		Client:        mgr.GetClient(),
		Scheme:        mgr.GetScheme(),
		EventRecorder: mgr.GetEventRecorderFor("deployment-controller"),
		Radius:        radius,
		DelayInterval: deploymentTestControllerDelayInterval,
	}).SetupWithManager(mgr)
	require.NoError(t, err)

	go func() {
		err := mgr.Start(ctx)
		require.NoError(t, err)
	}()

	return radius, mgr.GetClient()
}

// Creates a deployment with Radius enabled.
//
// Then exercises the cleanup path by deleting the deployment.
func Test_DeploymentReconciler_RadiusEnabled_ThenDeploymentDeleted(t *testing.T) {
	ctx := testcontext.New(t)
	radius, client := SetupDeploymentTest(t)

	name := types.NamespacedName{Namespace: "deployment-enabled-deleted", Name: "test-deployment-enabled-deleted"}
	err := client.Create(ctx, &corev1.Namespace{ObjectMeta: ctrl.ObjectMeta{Name: name.Namespace}})
	require.NoError(t, err)

	deployment := makeDeployment(name)
	deployment.Annotations[AnnotationRadiusEnabled] = "true"
	err = client.Create(ctx, deployment)
	require.NoError(t, err)

	// Deployment will be waiting for environment to be created.
	createEnvironment(radius, "default")

	// Deployment will be waiting for container to complete deployment.
	annotations := waitForStateUpdating(t, client, name)

	radius.CompleteOperation(annotations.Status.Operation.ResumeToken, nil)

	// Deployment will update after operation completes
	annotations = waitForStateReady(t, client, name)

	err = client.Get(ctx, name, deployment)
	require.NoError(t, err)

	// We should not have created a secret reference since there are no connections.
	require.False(t, removeSecretReference(deployment, deployment.Name+"-connections"))

	container, err := radius.Containers(annotations.Status.Scope).Get(ctx, deployment.Name, nil)
	require.NoError(t, err)
	require.Equal(t, "manual", string(*container.Properties.ResourceProvisioning))
	require.Equal(t, []*v20231001preview.ResourceReference{{ID: to.Ptr("/planes/kubernetes/local/namespaces/deployment-enabled-deleted/providers/apps/Deployment/" + deployment.Name)}}, container.Properties.Resources)

	err = client.Delete(ctx, deployment)
	require.NoError(t, err)

	// Deletion of the container is in progress.
	annotations = waitForStateDeleting(t, client, name)
	radius.CompleteOperation(annotations.Status.Operation.ResumeToken, nil)

	// Now deleting of the deployment object can complete.
	waitForDeploymentDeleted(t, client, name)
}

func Test_DeploymentReconciler_ChangeEnvironmentAndApplication(t *testing.T) {
	ctx := testcontext.New(t)
	radius, client := SetupDeploymentTest(t)

	name := types.NamespacedName{Namespace: "deployment-change-envapp", Name: "test-deployment-change-envapp"}
	err := client.Create(ctx, &corev1.Namespace{ObjectMeta: ctrl.ObjectMeta{Name: name.Namespace}})
	require.NoError(t, err)

	deployment := makeDeployment(name)
	deployment.Annotations[AnnotationRadiusEnabled] = "true"
	err = client.Create(ctx, deployment)
	require.NoError(t, err)

	// Deployment will be waiting for environment to be created.
	createEnvironment(radius, "default")

	// Deployment will be waiting for container to complete deployment.
	annotations := waitForStateUpdating(t, client, name)
	require.Equal(t, "/planes/radius/local/resourcegroups/default-deployment-change-envapp", annotations.Status.Scope)
	require.Equal(t, "/planes/radius/local/resourceGroups/default/providers/Applications.Core/environments/default", annotations.Status.Environment)
	require.Equal(t, "/planes/radius/local/resourcegroups/default-deployment-change-envapp/providers/Applications.Core/applications/deployment-change-envapp", annotations.Status.Application)
	radius.CompleteOperation(annotations.Status.Operation.ResumeToken, nil)

	// Deployment will update after operation completes
	annotations = waitForStateReady(t, client, name)
	require.Equal(t, "/planes/radius/local/resourcegroups/default-deployment-change-envapp/providers/Applications.Core/containers/test-deployment-change-envapp", annotations.Status.Container)

	createEnvironment(radius, "new-environment")

	// Now update the deployment to change the environment and application.
	err = client.Get(ctx, name, deployment)
	require.NoError(t, err)

	deployment.Annotations[AnnotationRadiusEnvironment] = "new-environment"
	deployment.Annotations[AnnotationRadiusApplication] = "new-application"

	err = client.Update(ctx, deployment)
	require.NoError(t, err)

	// Now the deployment will delete and re-create the resource.

	// Deletion of the container is in progress.
	annotations = waitForStateDeleting(t, client, name)
	radius.CompleteOperation(annotations.Status.Operation.ResumeToken, nil)

	// Resource should be gone.
	_, err = radius.Containers(annotations.Status.Scope).Get(ctx, name.Name, nil)
	require.Error(t, err)

	// Recipe will be waiting for extender to complete provisioning.
	annotations = waitForStateUpdating(t, client, name)
	require.Equal(t, "/planes/radius/local/resourcegroups/new-environment-new-application", annotations.Status.Scope)
	require.Equal(t, "/planes/radius/local/resourceGroups/new-environment/providers/Applications.Core/environments/new-environment", annotations.Status.Environment)
	require.Equal(t, "/planes/radius/local/resourcegroups/new-environment-new-application/providers/Applications.Core/applications/new-application", annotations.Status.Application)
	radius.CompleteOperation(annotations.Status.Operation.ResumeToken, nil)

	// Recipe will update after operation completes
	annotations = waitForStateReady(t, client, name)
	require.Equal(t, "/planes/radius/local/resourcegroups/new-environment-new-application/providers/Applications.Core/containers/test-deployment-change-envapp", annotations.Status.Container)

	err = client.Delete(ctx, deployment)
	require.NoError(t, err)

	// Deletion of the container is in progress.
	annotations = waitForStateDeleting(t, client, name)
	radius.CompleteOperation(annotations.Status.Operation.ResumeToken, nil)

	// Now deleting of the deployment object can complete.
	waitForDeploymentDeleted(t, client, name)
}

// Creates a deployment with Radius enabled.
//
// Then exercises the cleanup path by disabling Radius.
func Test_DeploymentReconciler_RadiusEnabled_ThenRadiusDisabled(t *testing.T) {
	ctx := testcontext.New(t)
	radius, client := SetupDeploymentTest(t)

	name := types.NamespacedName{Namespace: "deployment-enabled-disabled", Name: "test-deployment-enabled-disabled"}
	err := client.Create(ctx, &corev1.Namespace{ObjectMeta: ctrl.ObjectMeta{Name: name.Namespace}})
	require.NoError(t, err)

	deployment := makeDeployment(name)
	deployment.Annotations[AnnotationRadiusEnabled] = "true"
	err = client.Create(ctx, deployment)
	require.NoError(t, err)

	// Deployment will be waiting for environment to be created.
	createEnvironment(radius, "default")

	// Deployment will be waiting for container to complete deployment.
	annotations := waitForStateUpdating(t, client, name)

	radius.CompleteOperation(annotations.Status.Operation.ResumeToken, nil)

	// Deployment will update after operation completes
	annotations = waitForStateReady(t, client, name)

	container, err := radius.Containers(annotations.Status.Scope).Get(ctx, deployment.Name, nil)
	require.NoError(t, err)
	require.Equal(t, "manual", string(*container.Properties.ResourceProvisioning))
	require.Equal(t, []*v20231001preview.ResourceReference{{ID: to.Ptr("/planes/kubernetes/local/namespaces/deployment-enabled-disabled/providers/apps/Deployment/" + deployment.Name)}}, container.Properties.Resources)

	// Trigger cleanup by disabling Radius.
	err = client.Get(ctx, name, deployment)
	require.NoError(t, err)
	deployment.Annotations[AnnotationRadiusEnabled] = "false"
	err = client.Update(ctx, deployment)
	require.NoError(t, err)

	// Deletion of the container is in progress.
	annotations = waitForStateDeleting(t, client, name)
	radius.CompleteOperation(annotations.Status.Operation.ResumeToken, nil)

	waitForRadiusContainerDeleted(t, client, name)
}

// Creates a deployment with Radius enabled and connections to two recipes.
//
// Then makes those recipes Ready so connections can be enabled.
//
// Then changes the configuration to *drop* one of the connections.
//
// Then exercises the cleanup path by disabling Radius - and shows that we can revert
// the changes Radius made to the deployment.
func Test_DeploymentReconciler_Connections(t *testing.T) {
	ctx := testcontext.New(t)
	radius, client := SetupDeploymentTest(t)

	name := types.NamespacedName{Namespace: "deployment-connections", Name: "test-deployment-connections"}
	secretName := types.NamespacedName{Namespace: name.Namespace, Name: fmt.Sprintf("%s-connections", name.Name)}
	err := client.Create(ctx, &corev1.Namespace{ObjectMeta: ctrl.ObjectMeta{Name: name.Namespace}})
	require.NoError(t, err)

	deployment := makeDeployment(name)
	deployment.Annotations[AnnotationRadiusEnabled] = "true"
	deployment.Annotations[AnnotationRadiusConnectionPrefix+"a"] = "recipe-a"
	deployment.Annotations[AnnotationRadiusConnectionPrefix+"b"] = "recipe-b"

	err = client.Create(ctx, deployment)
	require.NoError(t, err)

	// Deployment will be waiting for environment to be created.
	createEnvironment(radius, "default")

	// Deployment will be waiting for recipe resources to be created
	_ = waitForStateWaiting(t, client, name)

	// Create the recipes, but don't mark them as provisioned yet.
	recipeA := makeRecipe(types.NamespacedName{Namespace: name.Namespace, Name: "recipe-a"}, "Applications.Core/extenders")
	recipeB := makeRecipe(types.NamespacedName{Namespace: name.Namespace, Name: "recipe-b"}, "Applications.Core/extenders")

	err = client.Create(ctx, recipeA)
	require.NoError(t, err)
	err = client.Create(ctx, recipeB)
	require.NoError(t, err)

	// Deployment will be waiting for recipe resources to be created.
	annotations := waitForStateWaiting(t, client, name)

	// Create the radius resources associated with the recipes
	extenderA := generated.GenericResource{
		Properties: map[string]any{
			"a-value": "a",
			"secrets": map[string]string{
				"a-secret": "a",
			},
		},
	}
	poller, err := radius.Resources(annotations.Status.Scope, "Applications.Core/extenders").BeginCreateOrUpdate(ctx, recipeA.Name, extenderA, nil)
	require.NoError(t, err)
	token, err := poller.ResumeToken()
	require.NoError(t, err)
	radius.CompleteOperation(token, nil)

	extenderB := generated.GenericResource{
		Properties: map[string]any{
			"b-value": "b",
			"secrets": map[string]string{
				"b-secret": "b",
			},
		},
	}
	poller, err = radius.Resources(annotations.Status.Scope, "Applications.Core/extenders").BeginCreateOrUpdate(ctx, recipeB.Name, extenderB, nil)
	require.NoError(t, err)
	token, err = poller.ResumeToken()
	require.NoError(t, err)
	radius.CompleteOperation(token, nil)

	recipeA.Status = radappiov1alpha3.RecipeStatus{
		Resource: annotations.Status.Scope + "/providers/Applications.Core/extenders/" + recipeA.Name,
	}
	recipeB.Status = radappiov1alpha3.RecipeStatus{
		Resource: annotations.Status.Scope + "/providers/Applications.Core/extenders/" + recipeB.Name,
	}

	// Mark the recipes as provisioned.
	err = client.Status().Update(ctx, recipeA)
	require.NoError(t, err)
	err = client.Status().Update(ctx, recipeB)
	require.NoError(t, err)

	// Now we can create the container
	annotations = waitForStateUpdating(t, client, name)

	radius.CompleteOperation(annotations.Status.Operation.ResumeToken, nil)

	// Deployment will update after operation completes
	annotations = waitForStateReady(t, client, name)

	container, err := radius.Containers(annotations.Status.Scope).Get(ctx, deployment.Name, nil)
	require.NoError(t, err)
	require.Equal(t, "manual", string(*container.Properties.ResourceProvisioning))
	require.Equal(t, map[string]*v20231001preview.ConnectionProperties{
		"a": {
			Source: to.Ptr(annotations.Status.Scope + "/providers/Applications.Core/extenders/" + recipeA.Name),
		},
		"b": {
			Source: to.Ptr(annotations.Status.Scope + "/providers/Applications.Core/extenders/" + recipeB.Name),
		},
	}, container.Properties.Connections)
	require.Equal(t, []*v20231001preview.ResourceReference{{ID: to.Ptr("/planes/kubernetes/local/namespaces/deployment-connections/providers/apps/Deployment/" + deployment.Name)}}, container.Properties.Resources)

	err = client.Get(ctx, name, deployment)
	require.NoError(t, err)

	// Secret should have been created.
	secret := corev1.Secret{}
	err = client.Get(ctx, secretName, &secret)
	require.NoError(t, err)

	expectedSecretData := map[string][]byte{
		"CONNECTION_A_A-SECRET": []byte("a"),
		"CONNECTION_A_A-VALUE":  []byte("a"),
		"CONNECTION_B_B-SECRET": []byte("b"),
		"CONNECTION_B_B-VALUE":  []byte("b"),
	}
	require.Equal(t, expectedSecretData, secret.Data)

	// Secret should be mapped as env-vars
	expectedEnvFrom := []corev1.EnvFromSource{
		{
			SecretRef: &corev1.SecretEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{Name: fmt.Sprintf("%s-connections", deployment.Name)},
				Optional:             to.Ptr(false),
			},
		},
	}

	require.Equal(t, expectedEnvFrom, deployment.Spec.Template.Spec.Containers[0].EnvFrom)

	// Deployment should have the hash of the secret data.
	hash := deployment.Spec.Template.Annotations[kubernetes.AnnotationSecretHash]
	require.NotEmpty(t, hash)

	// Trigger a change by removing one of the connections.
	err = client.Get(ctx, name, deployment)
	require.NoError(t, err)
	delete(deployment.Annotations, AnnotationRadiusConnectionPrefix+"a")
	err = client.Update(ctx, deployment)
	require.NoError(t, err)

	// Container will be updated.
	annotations = waitForStateUpdating(t, client, name)

	radius.CompleteOperation(annotations.Status.Operation.ResumeToken, nil)

	// Deployment will update after operation completes
	_ = waitForStateReady(t, client, name)

	err = client.Get(ctx, name, deployment)
	require.NoError(t, err)

	// Secret should have been updated.
	err = client.Get(ctx, secretName, &secret)
	require.NoError(t, err)

	expectedSecretData = map[string][]byte{
		"CONNECTION_B_B-SECRET": []byte("b"),
		"CONNECTION_B_B-VALUE":  []byte("b"),
	}
	require.Equal(t, expectedSecretData, secret.Data)

	// Secret should be mapped as env-vars
	require.Equal(t, expectedEnvFrom, deployment.Spec.Template.Spec.Containers[0].EnvFrom)

	// Deployment should have a DIFFERENT hash of the secret data.
	require.NotEqual(t, hash, deployment.Spec.Template.Annotations[kubernetes.AnnotationSecretHash])

	// Trigger cleanup by disabling Radius.
	err = client.Get(ctx, name, deployment)
	require.NoError(t, err)
	deployment.Annotations[AnnotationRadiusEnabled] = "false"
	err = client.Update(ctx, deployment)
	require.NoError(t, err)

	// Deletion of the container is in progress.
	annotations = waitForStateDeleting(t, client, name)
	radius.CompleteOperation(annotations.Status.Operation.ResumeToken, nil)

	waitForRadiusContainerDeleted(t, client, name)

	// Deployment should have Radius changes reverted.
	err = client.Get(ctx, name, deployment)
	require.NoError(t, err)
	require.Empty(t, deployment.Spec.Template.Spec.Containers[0].EnvFrom)

	// Secret should be gone
	err = client.Get(ctx, secretName, &secret)
	require.Error(t, err)
	require.True(t, apierrors.IsNotFound(err))
}

// Creates a deployment with Radius disabled.
//
// Then checks the Events for Disabled-Disabled.
func Test_DeploymentReconciler_RadiusDisabled_ThenRadiusDisabled_ByAnnotation(t *testing.T) {
	ctx := testcontext.New(t)
	_, client := SetupDeploymentTest(t)

	name := types.NamespacedName{
		Namespace: "deployment-disabled-disabled-by-annotation",
		Name:      "test-deployment-disabled-disabled-by-annotation",
	}
	err := client.Create(ctx, &corev1.Namespace{ObjectMeta: ctrl.ObjectMeta{Name: name.Namespace}})
	require.NoError(t, err)

	deployment := makeDeployment(name)
	err = client.Create(ctx, deployment)
	require.NoError(t, err)

	waitForEvent(t, client,
		expectedEvent{
			EventType: "Normal",
			Reason:    "NoOp",
			Message:   fmt.Sprintf("Radius is not enabled for %s", deployment.Name),
			Count:     1,
		},
	)

	// Explicitly setting Radius disabled
	deployment.Annotations = map[string]string{
		AnnotationRadiusEnabled: "false",
	}
	err = client.Update(ctx, deployment)
	require.NoError(t, err)

	// We expect the same event to be generated.
	// Because Radius was disabled and is still disabled for the deployment.
	waitForEvent(t, client,
		expectedEvent{
			EventType: "Normal",
			Reason:    "NoOp",
			Message:   fmt.Sprintf("Radius is not enabled for %s", deployment.Name),
			Count:     2,
		},
	)
}

// Creates a deployment with Radius disabled.
//
// Then checks the Events for Disabled-Disabled.
func Test_DeploymentReconciler_RadiusDisabled_ThenRadiusDisabled(t *testing.T) {
	ctx := testcontext.New(t)
	_, client := SetupDeploymentTest(t)

	name := types.NamespacedName{
		Namespace: "deployment-disabled-disabled",
		Name:      "test-deployment-disabled-disabled",
	}
	err := client.Create(ctx, &corev1.Namespace{ObjectMeta: ctrl.ObjectMeta{Name: name.Namespace}})
	require.NoError(t, err)

	deployment := makeDeployment(name)
	err = client.Create(ctx, deployment)
	require.NoError(t, err)

	waitForEvent(t, client,
		expectedEvent{
			EventType: "Normal",
			Reason:    "NoOp",
			Message:   fmt.Sprintf("Radius is not enabled for %s", deployment.Name),
			Count:     1,
		},
	)

	// Update Labels of the Deployment so that the Reconciler can detect a change.
	deployment.Labels = map[string]string{"foo": "bar"}
	err = client.Update(ctx, deployment)
	require.NoError(t, err)

	// We expect the same event to be generated.
	// Because Radius was disabled and is still disabled for the deployment.
	waitForEvent(t, client,
		expectedEvent{
			EventType: "Normal",
			Reason:    "NoOp",
			Message:   fmt.Sprintf("Radius is not enabled for %s", deployment.Name),
			Count:     2,
		},
	)
}

func waitForStateWaiting(t *testing.T, client client.Client, name types.NamespacedName) *deploymentAnnotations {
	ctx := testcontext.New(t)

	logger := t
	var annotations deploymentAnnotations
	require.EventuallyWithTf(t, func(t *assert.CollectT) {
		logger.Logf("Fetching Deployment: %+v", name)
		current := &appsv1.Deployment{}
		err := client.Get(ctx, name, current)
		require.NoError(t, err)

		annotations, err = readAnnotations(current)
		require.NoError(t, err)
		assert.NotNil(t, annotations)
		logger.Logf("Annotations.Status: %+v", annotations.Status)

		if assert.NotNil(t, annotations.Status) && assert.Equal(t, deploymentPhraseWaiting, annotations.Status.Phrase) {
			assert.Empty(t, annotations.Status.Operation)
		}
	}, deploymentTestWaitDuration, deploymentTestWaitInterval, "waiting for state to be Waiting")

	return &annotations
}

func waitForStateUpdating(t *testing.T, client client.Client, name types.NamespacedName) *deploymentAnnotations {
	ctx := testcontext.New(t)

	logger := t
	var annotations deploymentAnnotations
	require.EventuallyWithTf(t, func(t *assert.CollectT) {
		logger.Logf("Fetching Deployment: %+v", name)
		current := &appsv1.Deployment{}
		err := client.Get(ctx, name, current)
		require.NoError(t, err)

		annotations, err = readAnnotations(current)
		require.NoError(t, err)
		assert.NotNil(t, annotations)
		logger.Logf("Annotations.Status: %+v", annotations.Status)

		if assert.NotNil(t, annotations.Status) && assert.Equal(t, deploymentPhraseUpdating, annotations.Status.Phrase) {
			assert.NotEmpty(t, annotations.Status.Operation)
		}
	}, deploymentTestWaitDuration, deploymentTestWaitInterval, "waiting for state to be Updating")

	return &annotations
}

func waitForStateReady(t *testing.T, client client.Client, name types.NamespacedName) *deploymentAnnotations {
	ctx := testcontext.New(t)

	logger := t
	var annotations deploymentAnnotations
	require.EventuallyWithTf(t, func(t *assert.CollectT) {
		logger.Logf("Fetching Deployment: %+v", name)
		current := &appsv1.Deployment{}
		err := client.Get(ctx, name, current)
		require.NoError(t, err)

		annotations, err = readAnnotations(current)
		require.NoError(t, err)
		assert.NotNil(t, annotations)
		logger.Logf("Annotations.Status: %+v", annotations.Status)

		if assert.NotNil(t, annotations.Status) && assert.Equal(t, deploymentPhraseReady, annotations.Status.Phrase) {
			assert.Empty(t, annotations.Status.Operation)
		}
	}, deploymentTestWaitDuration, deploymentTestWaitInterval, "waiting for state to be Ready")

	return &annotations
}

func waitForStateDeleting(t *testing.T, client client.Client, name types.NamespacedName) *deploymentAnnotations {
	ctx := testcontext.New(t)

	logger := t
	var annotations deploymentAnnotations
	require.EventuallyWithTf(t, func(t *assert.CollectT) {
		logger.Logf("Fetching Deployment: %+v", name)
		current := &appsv1.Deployment{}
		err := client.Get(ctx, name, current)
		require.NoError(t, err)

		annotations, err = readAnnotations(current)
		require.NoError(t, err)
		assert.NotNil(t, annotations)
		logger.Logf("Annotations.Status: %+v", annotations.Status)

		if assert.NotNil(t, annotations.Status) && assert.Equal(t, deploymentPhraseDeleting, annotations.Status.Phrase) {
			assert.NotEmpty(t, annotations.Status.Operation)
		}
	}, deploymentTestWaitDuration, deploymentTestWaitInterval, "waiting for state to be Deleting")

	return &annotations
}

type expectedEvent struct {
	EventType string
	Reason    string
	Message   string
	Count     int
}

// waitForEvent waits for the expected event to occur in the cluster.
//
// We can have multiple events as the result of the List function but we are only interested in the expected event.
func waitForEvent(t *testing.T, client client.Client, event expectedEvent) {
	ctx := testcontext.New(t)
	logger := t

	require.EventuallyWithTf(t, func(t *assert.CollectT) {
		logger.Log("Fetching Events")

		events := &corev1.EventList{}
		err := client.List(ctx, events)
		require.NoError(t, err)

		found := false
		for _, e := range events.Items {
			// If the event is the one we are looking for, check the count.
			if e.Type == event.EventType && e.Reason == event.Reason &&
				e.Message == event.Message && assert.Equal(t, event.Count, int(e.Count)) {
				found = true
			}
		}
		assert.True(t, found)
	}, deploymentTestWaitDuration, deploymentTestWaitInterval, "Waiting for the expected event")
}

func waitForRadiusContainerDeleted(t *testing.T, client client.Client, name types.NamespacedName) *deploymentAnnotations {
	ctx := testcontext.New(t)

	logger := t
	var annotations *deploymentAnnotations
	require.EventuallyWithTf(t, func(t *assert.CollectT) {
		logger.Logf("Fetching Deployment: %+v", name)
		current := &appsv1.Deployment{}
		err := client.Get(ctx, name, current)
		require.NoError(t, err)

		logger.Logf("Annotations: %+v", current.Annotations)
		assert.NotContains(t, current.Annotations, AnnotationRadiusStatus)
		assert.NotContains(t, current.Annotations, AnnotationRadiusConfigurationHash)
	}, deploymentTestWaitDuration, deploymentTestWaitInterval, "waiting for state to be Deleting")

	return annotations
}

func waitForDeploymentDeleted(t *testing.T, client client.Client, name types.NamespacedName) {
	ctx := testcontext.New(t)

	logger := t
	require.Eventuallyf(t, func() bool {
		logger.Logf("Fetching Deployment: %+v", name)
		err := client.Get(ctx, name, &appsv1.Deployment{})
		return apierrors.IsNotFound(err)
	}, deploymentTestWaitDuration, deploymentTestWaitInterval, "waiting for deployment to be deleted")
}
