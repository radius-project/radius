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
	"context"
	"fmt"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/go-logr/logr"
	"github.com/radius-project/radius/pkg/cli/clients"
	radappiov1alpha3 "github.com/radius-project/radius/pkg/controller/api/radapp.io/v1alpha3"
	"github.com/radius-project/radius/pkg/corerp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/kubernetes"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
)

// DeploymentReconciler reconciles a Deployment object.
type DeploymentReconciler struct {
	// Client is the Kubernetes client.
	Client client.Client

	// Scheme is the Kubernetes scheme.
	Scheme *runtime.Scheme

	// EventRecorder is the Kubernetes event recorder.
	EventRecorder record.EventRecorder

	// Radius is the Radius client.
	Radius RadiusClient

	// DelayInterval is the amount of time to wait between operations.
	DelayInterval time.Duration
}

// Reconcile is the main reconciliation loop for the Deployment resource.
func (r *DeploymentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := ucplog.FromContextOrDiscard(ctx).WithValues("kind", "Deployment", "name", req.Name, "namespace", req.Namespace)
	ctx = logr.NewContext(ctx, logger)

	deployment := appsv1.Deployment{}
	err := r.Client.Get(ctx, req.NamespacedName, &deployment)
	if apierrors.IsNotFound(err) {
		// This can happen due to a data-race if the deployment is created and then deleted before we can
		// reconcile it. There's nothing to do here.
		logger.Info("Deployment has already been deleted.")
		return ctrl.Result{}, nil
	} else if err != nil {
		logger.Error(err, "Unable to fetch resource.")
		return ctrl.Result{}, err
	}

	// Our algorithm is as follows:
	//
	// 1. Check if we have an "operation" in progress. If so, check it's status.
	//   a. If the operation is still in progress, then queue another reconcile (polling).
	//   b. If the operation completed successfully then update the status and continue processing (happy-path).
	//   c. If the operation failed then update the status and continue processing (retry).
	// 2. If the deployment is being deleted then process deletion.
	//   a. This may require us to start a DELETE operation. After that we can continue polling.
	// 3. If the deployment is not being deleted then process this as a creation or update.
	//   a. This may require us to start a PUT operation. After that we can continue polling.
	//
	// We do it this way because it guarantees that we only have one operation going at a time.

	// Since Deployment is a built-in type in Kubernetes we can't add our own status field to it.
	// We have to store our status in an annotation.
	annotations, err := readAnnotations(&deployment)
	if err != nil {
		logger.Error(err, "Failed to read deployment status.")
		deployment.Annotations[AnnotationRadiusStatus] = ""

		// This could happen if someone manually edited the annotations. We can reset it to empty
		// and repair it on the next reconcile.
	}

	if annotations.Status != nil && annotations.Status.Operation != nil {
		// NOTE: if reconcileOperation completes successfully, then it will return a "zero" result,
		// this means the operation has completed and we should continue processing.
		result, err := r.reconcileOperation(ctx, &deployment, annotations)
		if err != nil {
			logger.Error(err, "Unable to reconcile in-progress operation.")
			return ctrl.Result{}, err
		} else if result.IsZero() {
			// NOTE: if reconcileOperation completes successfully, then it will return a "zero" result,
			// this means the operation has completed and we should continue processing.
			logger.Info("Operation completed successfully.")
		} else {
			logger.Info("Requeueing to continue operation.")
			return result, nil
		}
	}

	// If the Deployment is being deleted **or** if Radius is no longer enabled, then we should
	// clean up any Radius state.
	if deployment.DeletionTimestamp != nil || (annotations.Configuration == nil && annotations.Status != nil) {
		return r.reconcileDelete(ctx, &deployment, annotations)
	}

	return r.reconcileUpdate(ctx, &deployment, annotations)
}

// reconcileOperation reconciles a Deployment that has an operation in progress.
func (r *DeploymentReconciler) reconcileOperation(ctx context.Context, deployment *appsv1.Deployment, annotations *deploymentAnnotations) (ctrl.Result, error) {
	logger := ucplog.FromContextOrDiscard(ctx)

	// NOTE: the pollers are actually different types, so we have to duplicate the code
	// for the PUT and DELETE handling. This makes me sad :( but there isn't a great
	// solution besides duplicating the code.
	//
	// The only difference between these two codepaths is how they handle success.
	if annotations.Status.Operation.OperationKind == radappiov1alpha3.OperationKindPut {
		poller, err := r.Radius.Containers(annotations.Status.Scope).ContinueCreateOperation(ctx, annotations.Status.Operation.ResumeToken)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to continue PUT operation: %w", err)
		}

		_, err = poller.Poll(ctx)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to poll operation status: %w", err)
		}

		if !poller.Done() {
			return ctrl.Result{Requeue: true, RequeueAfter: r.requeueDelay()}, nil
		}

		// If we get here, the operation is complete.
		_, err = poller.Result(ctx)
		if err != nil {
			// Operation failed, reset state and retry.
			r.EventRecorder.Event(deployment, corev1.EventTypeWarning, "ResourceError", err.Error())
			logger.Error(err, "Update failed.")

			annotations.Status.Operation = nil
			annotations.Status.Phrase = deploymentPhraseFailed

			err = r.saveState(ctx, deployment, annotations)
			if err != nil {
				return ctrl.Result{}, err
			}

			return ctrl.Result{Requeue: true, RequeueAfter: r.requeueDelay()}, nil
		}

		// If we get here, the operation was a success. Update the status and continue.
		//
		// NOTE: we don't need to save the status here, because we're going to continue reconciling.
		annotations.Status.Operation = nil
		annotations.Status.Container = annotations.Status.Scope + "/providers/Applications.Core/containers/" + deployment.Name
		return ctrl.Result{}, nil
	} else if annotations.Status.Operation.OperationKind == radappiov1alpha3.OperationKindDelete {
		poller, err := r.Radius.Containers(annotations.Status.Scope).ContinueDeleteOperation(ctx, annotations.Status.Operation.ResumeToken)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to continue DELETE operation: %w", err)
		}

		_, err = poller.Poll(ctx)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to poll operation status: %w", err)
		}

		if !poller.Done() {
			return ctrl.Result{Requeue: true, RequeueAfter: r.requeueDelay()}, nil
		}

		// If we get here, the operation is complete.
		_, err = poller.Result(ctx)
		if err != nil {
			// Operation failed, reset state and retry.
			r.EventRecorder.Event(deployment, corev1.EventTypeWarning, "ResourceError", err.Error())
			logger.Error(err, "Delete failed.")

			annotations.Status.Operation = nil
			annotations.Status.Phrase = deploymentPhraseFailed

			err = r.saveState(ctx, deployment, annotations)
			if err != nil {
				return ctrl.Result{}, err
			}

			return ctrl.Result{Requeue: true, RequeueAfter: r.requeueDelay()}, nil
		}

		// If we get here, the operation was a success. Update the status and continue.
		//
		// NOTE: we don't need to save the status here, because we're going to continue reconciling.
		annotations.Status.Operation = nil
		annotations.Status.Container = ""
		return ctrl.Result{}, nil
	}

	// If we get here, this was an unknown operation kind. This is a bug in our code, or someone
	// tampered with the status of the object. Just reset the state and move on.
	logger.Error(fmt.Errorf("unknown operation kind: %s", annotations.Status.Operation.OperationKind), "Unknown operation kind.")

	annotations.Status.Operation = nil
	annotations.Status.Phrase = deploymentPhraseFailed

	err := r.saveState(ctx, deployment, annotations)
	if err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *DeploymentReconciler) reconcileUpdate(ctx context.Context, deployment *appsv1.Deployment, annotations *deploymentAnnotations) (ctrl.Result, error) {
	logger := ucplog.FromContextOrDiscard(ctx)

	// Ensure that our finalizer is present before we start any operations.
	if controllerutil.AddFinalizer(deployment, DeploymentFinalizer) {
		err := r.Client.Update(ctx, deployment)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	environmentName := "default"
	if annotations.Configuration.Environment != "" {
		environmentName = annotations.Configuration.Environment
	}

	applicationName := deployment.Namespace
	if annotations.Configuration.Application != "" {
		applicationName = annotations.Configuration.Application
	}

	resourceGroupID, environmentID, applicationID, err := resolveDependencies(ctx, r.Radius, "/planes/radius/local", environmentName, applicationName)
	if err != nil {
		r.EventRecorder.Event(deployment, corev1.EventTypeWarning, "DependencyError", err.Error())
		logger.Error(err, "Unable to resolve dependencies.")
		return ctrl.Result{}, fmt.Errorf("failed to resolve dependencies: %w", err)
	}

	annotations.Status.Scope = resourceGroupID
	annotations.Status.Environment = environmentID
	annotations.Status.Application = applicationID

	// There are three possible states returned here:
	//
	// 1) err != nil - an error happened, this will be retried next reconcile.
	// 2) waiting == true - we're waiting on dependencies, this will be retried next reconcile.
	// 3) updatePoller != nil - we've started a PUT operation, this will be checked next reconcile.
	// 4) deletePoller != nil - we've started a DELETE operation, this will be checked next reconcile.
	updatePoller, deletePoller, waiting, err := r.startPutOrDeleteOperationIfNeeded(ctx, deployment, annotations)
	if err != nil {
		logger.Error(err, "Unable to create or update resource.")
		r.EventRecorder.Event(deployment, corev1.EventTypeWarning, "ResourceError", err.Error())
		return ctrl.Result{}, err
	} else if waiting {
		logger.Info("Waiting on dependencies.")
		r.EventRecorder.Event(deployment, corev1.EventTypeNormal, "DependencyNotReady", "Waiting on dependencies.")

		annotations.Status.Phrase = deploymentPhraseWaiting
		err = r.saveState(ctx, deployment, annotations)
		if err != nil {
			return ctrl.Result{}, err
		}

		// We don't need to requeue here because we watch Recipes and will be notified when
		// the state changes.
		return ctrl.Result{}, nil
	} else if updatePoller != nil {
		// We've successfully started an operation. Update the status and requeue.
		token, err := updatePoller.ResumeToken()
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to get operation token: %w", err)
		}

		annotations.Status.Operation = &radappiov1alpha3.ResourceOperation{ResumeToken: token, OperationKind: radappiov1alpha3.OperationKindPut}
		annotations.Status.Phrase = deploymentPhraseUpdating
		err = r.saveState(ctx, deployment, annotations)
		if err != nil {
			return ctrl.Result{}, err
		}

		return ctrl.Result{Requeue: true, RequeueAfter: r.requeueDelay()}, nil
	} else if deletePoller != nil {
		// We've successfully started an operation. Update the status and requeue.
		token, err := deletePoller.ResumeToken()
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to get operation token: %w", err)
		}

		annotations.Status.Operation = &radappiov1alpha3.ResourceOperation{ResumeToken: token, OperationKind: radappiov1alpha3.OperationKindDelete}
		annotations.Status.Phrase = deploymentPhraseDeleting
		err = r.saveState(ctx, deployment, annotations)
		if err != nil {
			return ctrl.Result{}, err
		}

		return ctrl.Result{Requeue: true, RequeueAfter: r.requeueDelay()}, nil
	}

	// If we get here then it means we can process the result of the operation.
	logger.Info("Resource is in desired state.", "resourceId", annotations.Status.Container)

	annotations.Status.Phrase = deploymentPhraseReady
	err = r.updateDeployment(ctx, deployment, annotations)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update deployment: %w", err)
	}

	err = r.saveState(ctx, deployment, annotations)
	if err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *DeploymentReconciler) reconcileDelete(ctx context.Context, deployment *appsv1.Deployment, annotations *deploymentAnnotations) (ctrl.Result, error) {
	logger := ucplog.FromContextOrDiscard(ctx)

	poller, err := r.startDeleteOperationIfNeeded(ctx, deployment, annotations)
	if err != nil {
		logger.Error(err, "Unable to delete resource.")
		r.EventRecorder.Event(deployment, corev1.EventTypeWarning, "ResourceError", err.Error())
		return ctrl.Result{}, err
	} else if poller != nil {
		// We've successfully started an operation. Update the status and requeue.
		token, err := poller.ResumeToken()
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to get operation token: %w", err)
		}

		annotations.Status.Operation = &radappiov1alpha3.ResourceOperation{ResumeToken: token, OperationKind: radappiov1alpha3.OperationKindDelete}
		annotations.Status.Phrase = deploymentPhraseDeleting
		err = r.saveState(ctx, deployment, annotations)
		if err != nil {
			return ctrl.Result{}, err
		}

		return ctrl.Result{Requeue: true, RequeueAfter: r.requeueDelay()}, nil
	}

	logger.Info("Resource is deleted.")

	err = r.cleanupDeployment(ctx, deployment)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to cleanup deployment: %w", err)
	}

	// At this point we've cleaned up everything. We can remove the finalizer which will allow deletion of the
	// recipe.
	controllerutil.RemoveFinalizer(deployment, DeploymentFinalizer)
	err = r.Client.Update(ctx, deployment)
	if err != nil {
		return ctrl.Result{}, err
	}

	r.EventRecorder.Event(deployment, corev1.EventTypeNormal, "Reconciled", "Successfully reconciled resource.")
	return ctrl.Result{}, nil
}

func (r *DeploymentReconciler) startPutOrDeleteOperationIfNeeded(ctx context.Context, deployment *appsv1.Deployment, annotations *deploymentAnnotations) (Poller[v20231001preview.ContainersClientCreateOrUpdateResponse], Poller[v20231001preview.ContainersClientDeleteResponse], bool, error) {
	logger := ucplog.FromContextOrDiscard(ctx)

	resourceID := annotations.Status.Scope + "/providers/Applications.Core/containers/" + deployment.Name

	// Check the annotations first to see how the current configuration compares to the desired configuration.
	if annotations.Status.Container != "" && !strings.EqualFold(annotations.Status.Container, resourceID) {
		// If we get here it means that the environment or application changed, so we should delete
		// the old resource and create a new one.
		logger.Info("Container is already created but is out-of-date")

		logger.Info("Starting DELETE operation.")
		poller, err := deleteContainer(ctx, r.Radius, annotations.Status.Container)
		if err != nil {
			return nil, nil, false, err
		} else if poller != nil {
			return nil, poller, false, nil
		}

		// Deletion completed synchronously.
		annotations.Status.Container = ""
	}

	// Note: we separate this check from the previous block, because it could complete synchronously.
	if !annotations.IsUpToDate() {
		logger.Info("Container configuration is out-of-date.")
	} else if annotations.Status.Container != "" {
		logger.Info("Container is already created and is up-to-date.")
		return nil, nil, false, nil
	}

	logger.Info("Starting PUT operation.")
	properties := v20231001preview.ContainerProperties{
		Application:          to.Ptr(annotations.Status.Application),
		ResourceProvisioning: to.Ptr(v20231001preview.ContainerResourceProvisioningManual),
		Connections:          map[string]*v20231001preview.ConnectionProperties{},
		Container: &v20231001preview.Container{
			Image: to.Ptr("none"),
		},
		Resources: []*v20231001preview.ResourceReference{
			{
				ID: to.Ptr("/planes/kubernetes/local/namespaces/" + deployment.Namespace + "/providers/apps/Deployment/" + deployment.Name),
			},
		},
	}

	for name, source := range annotations.Configuration.Connections {
		recipe := radappiov1alpha3.Recipe{}
		err := r.Client.Get(ctx, client.ObjectKey{Namespace: deployment.Namespace, Name: source}, &recipe)
		if apierrors.IsNotFound(err) {
			logger.Info("Recipe does not exist.", "recipe", source)
			return nil, nil, true, nil
		} else if err != nil {
			return nil, nil, false, fmt.Errorf("failed to fetch recipe %s: %w", source, err)
		} else if recipe.Status.Resource == "" {
			logger.Info("Recipe is not ready.", "recipe", source)
			return nil, nil, true, nil
		}

		properties.Connections[name] = &v20231001preview.ConnectionProperties{
			Source: to.Ptr(recipe.Status.Resource),
		}
	}

	poller, err := createOrUpdateContainer(ctx, r.Radius, resourceID, &properties)
	if err != nil {
		return nil, nil, false, err
	} else if poller != nil {
		return poller, nil, false, nil
	}

	// Update completed synchronously
	annotations.Status.Container = resourceID
	return poller, nil, false, nil
}

func (r *DeploymentReconciler) startDeleteOperationIfNeeded(ctx context.Context, deployment *appsv1.Deployment, annotations *deploymentAnnotations) (Poller[v20231001preview.ContainersClientDeleteResponse], error) {
	logger := ucplog.FromContextOrDiscard(ctx)
	if annotations.Status.Container == "" {
		logger.Info("Container is already deleted (or was never created).")
		return nil, nil
	}

	logger.Info("Starting DELETE operation.")
	poller, err := deleteContainer(ctx, r.Radius, annotations.Status.Container)
	if err != nil {
		return nil, err
	} else if poller != nil {
		return poller, nil
	}

	// Deletion completed synchronously.
	annotations.Status.Container = ""
	return nil, nil
}

func (r *DeploymentReconciler) updateDeployment(ctx context.Context, deployment *appsv1.Deployment, annotations *deploymentAnnotations) error {
	// We store the connection values in a Kubernetes secret and then use the secret to populate environment variables.
	secretName := client.ObjectKey{Namespace: deployment.Namespace, Name: fmt.Sprintf("%s-connections", deployment.Name)}

	if len(annotations.Configuration.Connections) == 0 {
		// No need for a secret if there are no connections.
		removeSecretReference(deployment, fmt.Sprintf("%s-connections", deployment.Name))
		delete(deployment.Spec.Template.ObjectMeta.Annotations, kubernetes.AnnotationSecretHash)

		err := r.Client.Delete(ctx, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Namespace: secretName.Namespace, Name: secretName.Name}})
		if err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to delete secret %s: %w", secretName.Name, err)
		}

		return nil
	}

	// First retrieve the secret.
	createSecret := false
	secret := corev1.Secret{}
	err := r.Client.Get(ctx, secretName, &secret)
	if apierrors.IsNotFound(err) {
		// It's OK if the secret doesn't exist yet. We'll create it below.
		createSecret = true
		secret.Name = secretName.Name
		secret.Namespace = secretName.Namespace
		secret.OwnerReferences = []metav1.OwnerReference{
			*metav1.NewControllerRef(deployment, appsv1.SchemeGroupVersion.WithKind("Deployment")),
		}
	} else if err != nil {
		return fmt.Errorf("failed to fetch secret %s: %w", secretName, err)
	}

	// envtest has some quirky behavior around StringData which makes it hard to test. So we're
	// using Data directly.
	secret.Data = map[string][]byte{}

	for name, source := range annotations.Configuration.Connections {
		recipe := radappiov1alpha3.Recipe{}
		err := r.Client.Get(ctx, client.ObjectKey{Namespace: deployment.Namespace, Name: source}, &recipe)
		if err != nil {
			return fmt.Errorf("failed to fetch recipe %s: %w", source, err)
		}

		if recipe.Status.Resource == "" {
			return fmt.Errorf("recipe %s is not ready", source)
		}

		id, err := resources.Parse(recipe.Status.Resource)
		if err != nil {
			return err
		}

		response, err := r.Radius.Resources(id.RootScope(), id.Type()).Get(ctx, id.Name())
		if err != nil {
			return fmt.Errorf("failed to fetch resource %s: %w", id, err)
		}

		secrets, err := r.Radius.Resources(id.RootScope(), id.Type()).ListSecrets(ctx, id.Name())
		if clients.Is404Error(err) {
			// This is fine. The resource doesn't have any secrets.
			secrets.Value = map[string]*string{}
		} else if err != nil {
			return fmt.Errorf("failed to fetch secrets for resource %s: %w", id, err)
		}

		values, err := resourceToConnectionEnvVars(name, response.GenericResource, secrets)
		if err != nil {
			return fmt.Errorf("failed to read values resource %s: %w", id, err)
		}

		for k, v := range values {
			secret.Data[k] = []byte(v)
		}
	}

	// Add the hash of the secret data to the Pod definition. This will force a rollout when the secrets
	// change.
	hash := kubernetes.HashSecretData(secret.Data)
	if deployment.Spec.Template.ObjectMeta.Annotations == nil {
		deployment.Spec.Template.ObjectMeta.Annotations = map[string]string{}
	}
	deployment.Spec.Template.ObjectMeta.Annotations[kubernetes.AnnotationSecretHash] = hash

	addSecretReference(deployment, secretName.Name)

	if createSecret {
		err = r.Client.Create(ctx, &secret)
		if err != nil {
			return fmt.Errorf("failed to create secret %s: %w", secretName, err)
		}
	} else {
		err = r.Client.Update(ctx, &secret)
		if err != nil {
			return fmt.Errorf("failed to update secret %s: %w", secretName, err)
		}
	}

	return nil
}

func (r *DeploymentReconciler) cleanupDeployment(ctx context.Context, deployment *appsv1.Deployment) error {
	delete(deployment.Annotations, AnnotationRadiusStatus)
	delete(deployment.Annotations, AnnotationRadiusConfigurationHash)
	delete(deployment.Spec.Template.ObjectMeta.Annotations, kubernetes.AnnotationSecretHash)

	secretName := client.ObjectKey{Namespace: deployment.Namespace, Name: fmt.Sprintf("%s-connections", deployment.Name)}
	err := r.Client.Delete(ctx, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Namespace: secretName.Namespace, Name: secretName.Name}})
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete secret %s: %w", secretName.Name, err)
	}

	removeSecretReference(deployment, secretName.Name)
	return nil
}

func (r *DeploymentReconciler) saveState(ctx context.Context, deployment *appsv1.Deployment, annotations *deploymentAnnotations) error {
	err := annotations.ApplyToDeployment(deployment)
	if err != nil {
		return fmt.Errorf("unable to apply annotations: %w", err)
	}

	err = r.Client.Update(ctx, deployment)
	if err != nil {
		return err
	}

	return nil
}

func (r *DeploymentReconciler) findDeploymentsForRecipe(ctx context.Context, obj client.Object) []reconcile.Request {
	recipe := obj.(*radappiov1alpha3.Recipe)

	deployments := &appsv1.DeploymentList{}
	options := &client.ListOptions{
		FieldSelector: fields.OneTermEqualSelector(indexField, recipe.Name),
		Namespace:     recipe.Namespace,
	}
	err := r.Client.List(ctx, deployments, options)
	if err != nil {
		return []reconcile.Request{}
	}

	requests := []reconcile.Request{}
	for _, item := range deployments.Items {
		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      item.GetName(),
				Namespace: item.GetNamespace(),
			},
		})
	}
	return requests
}

func (r *DeploymentReconciler) requeueDelay() time.Duration {
	delay := r.DelayInterval
	if delay == 0 {
		delay = PollingDelay
	}

	return delay
}

const indexField = "spec.recipe-reference"

func (r *DeploymentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &appsv1.Deployment{}, indexField, func(rawObj client.Object) []string {
		deployment := rawObj.(*appsv1.Deployment)
		annotations, err := readAnnotations(deployment)
		if err != nil {
			return []string{}
		} else if annotations == nil || annotations.Configuration == nil {
			return []string{}
		}

		recipes := []string{}
		for _, recipe := range annotations.Configuration.Connections {
			recipes = append(recipes, recipe)
		}

		return recipes
	}); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&appsv1.Deployment{}).
		Watches(&radappiov1alpha3.Recipe{}, handler.EnqueueRequestsFromMapFunc(r.findDeploymentsForRecipe), builder.WithPredicates(predicate.ResourceVersionChangedPredicate{})).
		Owns(&corev1.Secret{}).
		Complete(r)
}
