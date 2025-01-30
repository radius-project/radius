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

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/go-logr/logr"
	"github.com/radius-project/radius/pkg/cli/clients"
	"github.com/radius-project/radius/pkg/cli/clients_new/generated"
	radappiov1alpha3 "github.com/radius-project/radius/pkg/controller/api/radapp.io/v1alpha3"
	sdkclients "github.com/radius-project/radius/pkg/sdk/clients"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RecipeReconciler reconciles a Recipe object.
type RecipeReconciler struct {
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

// Reconcile is the main reconciliation loop for the Recipe resource.
func (r *RecipeReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := ucplog.FromContextOrDiscard(ctx).WithValues("kind", "Recipe", "name", req.Name, "namespace", req.Namespace)
	ctx = logr.NewContext(ctx, logger)

	recipe := radappiov1alpha3.Recipe{}
	err := r.Client.Get(ctx, req.NamespacedName, &recipe)
	if apierrors.IsNotFound(err) {
		// This can happen due to a data-race if the recipe is created and then deleted before we can
		// reconcile it. There's nothing to do here.
		logger.Info("Recipe is being deleted.")
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
	// 2. If the recipe is being deleted then process deletion.
	//   a. This may require us to start a DELETE operation. After that we can continue polling.
	// 3. If the recipe is not being deleted then process this as a creation or update.
	//   a. This may require us to start a PUT operation. After that we can continue polling.
	//
	// We do it this way because it guarantees that we only have one operation going at a time.

	if recipe.Status.Operation != nil {
		// NOTE: if reconcileOperation completes successfully, then it will return a "zero" result,
		// this means the operation has completed and we should continue processing.
		result, err := r.reconcileOperation(ctx, &recipe)
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

	if recipe.DeletionTimestamp != nil {
		return r.reconcileDelete(ctx, &recipe)
	}

	return r.reconcileUpdate(ctx, &recipe)
}

// ReconileOperation reconciles a Recipe that has an operation in progress.
func (r *RecipeReconciler) reconcileOperation(ctx context.Context, recipe *radappiov1alpha3.Recipe) (ctrl.Result, error) {
	logger := ucplog.FromContextOrDiscard(ctx)

	// NOTE: the pollers are actually different types, so we have to duplicate the code
	// for the PUT and DELETE handling. This makes me sad :( but there isn't a great
	// solution besides duplicating the code.
	//
	// The only difference between these two codepaths is how they handle success.
	if recipe.Status.Operation.OperationKind == radappiov1alpha3.OperationKindPut {
		poller, err := r.Radius.Resources(recipe.Status.Scope, recipe.Spec.Type).ContinueCreateOperation(ctx, recipe.Status.Operation.ResumeToken)
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
			r.EventRecorder.Event(recipe, corev1.EventTypeWarning, "ResourceError", err.Error())
			logger.Error(err, "Update failed.")

			recipe.Status.Operation = nil
			recipe.Status.Phrase = radappiov1alpha3.PhraseFailed

			err = r.Client.Status().Update(ctx, recipe)
			if err != nil {
				return ctrl.Result{}, err
			}

			return ctrl.Result{Requeue: true, RequeueAfter: r.requeueDelay()}, nil
		}

		// If we get here, the operation was a success. Update the status and continue.
		//
		// NOTE: we don't need to save the status here, because we're going to continue reconciling.
		recipe.Status.Operation = nil
		recipe.Status.Resource = recipe.Status.Scope + "/providers/" + recipe.Spec.Type + "/" + recipe.Name
		return ctrl.Result{}, nil

	} else if recipe.Status.Operation.OperationKind == radappiov1alpha3.OperationKindDelete {
		poller, err := r.Radius.Resources(recipe.Status.Scope, recipe.Spec.Type).ContinueDeleteOperation(ctx, recipe.Status.Operation.ResumeToken)
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
			r.EventRecorder.Event(recipe, corev1.EventTypeWarning, "ResourceError", err.Error())
			logger.Error(err, "Delete failed.")

			recipe.Status.Operation = nil
			recipe.Status.Phrase = radappiov1alpha3.PhraseFailed

			err = r.Client.Status().Update(ctx, recipe)
			if err != nil {
				return ctrl.Result{}, err
			}

			return ctrl.Result{Requeue: true, RequeueAfter: r.requeueDelay()}, nil
		}

		// If we get here, the operation was a success. Update the status and continue.
		//
		// NOTE: we don't need to save the status here, because we're going to continue reconciling.
		recipe.Status.Operation = nil
		recipe.Status.Resource = ""
		return ctrl.Result{}, nil
	}

	// If we get here, this was an unknown operation kind. This is a bug in our code, or someone
	// tampered with the status of the object. Just reset the state and move on.
	logger.Error(fmt.Errorf("unknown operation kind: %s", recipe.Status.Operation.OperationKind), "Unknown operation kind.")

	recipe.Status.Operation = nil
	recipe.Status.Phrase = radappiov1alpha3.PhraseFailed

	err := r.Client.Status().Update(ctx, recipe)
	if err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *RecipeReconciler) reconcileUpdate(ctx context.Context, recipe *radappiov1alpha3.Recipe) (ctrl.Result, error) {
	logger := ucplog.FromContextOrDiscard(ctx)

	// Ensure that our finalizer is present before we start any operations.
	if controllerutil.AddFinalizer(recipe, RecipeFinalizer) {
		err := r.Client.Update(ctx, recipe)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	// Since we're going to reconcile, update the observed generation.
	//
	// We don't want to do this if we're in the middle of an operation, because we haven't
	// fully processed any status changes until the async operation completes.
	recipe.Status.ObservedGeneration = recipe.Generation

	environmentName := "default"
	if recipe.Spec.Environment != "" {
		environmentName = recipe.Spec.Environment
	}

	applicationName := recipe.Namespace
	if recipe.Spec.Application != "" {
		applicationName = recipe.Spec.Application
	}

	resourceGroupID, environmentID, applicationID, err := resolveDependencies(ctx, r.Radius, "/planes/radius/local", environmentName, applicationName)
	if err != nil {
		r.EventRecorder.Event(recipe, corev1.EventTypeWarning, "DependencyError", err.Error())
		logger.Error(err, "Unable to resolve dependencies.")
		return ctrl.Result{}, fmt.Errorf("failed to resolve dependencies: %w", err)
	}

	recipe.Status.Scope = resourceGroupID
	recipe.Status.Environment = environmentID
	recipe.Status.Application = applicationID

	updatePoller, deletePoller, err := r.startPutOrDeleteOperationIfNeeded(ctx, recipe)
	if err != nil {
		logger.Error(err, "Unable to create or update resource.")
		r.EventRecorder.Event(recipe, corev1.EventTypeWarning, "ResourceError", err.Error())
		return ctrl.Result{}, err
	} else if updatePoller != nil {
		// We've successfully started an operation. Update the status and requeue.
		token, err := updatePoller.ResumeToken()
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to get operation token: %w", err)
		}

		recipe.Status.Operation = &radappiov1alpha3.ResourceOperation{ResumeToken: token, OperationKind: radappiov1alpha3.OperationKindPut}
		recipe.Status.Phrase = radappiov1alpha3.PhraseUpdating
		err = r.Client.Status().Update(ctx, recipe)
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

		recipe.Status.Operation = &radappiov1alpha3.ResourceOperation{ResumeToken: token, OperationKind: radappiov1alpha3.OperationKindDelete}
		recipe.Status.Phrase = radappiov1alpha3.PhraseDeleting
		err = r.Client.Status().Update(ctx, recipe)
		if err != nil {
			return ctrl.Result{}, err
		}

		return ctrl.Result{Requeue: true, RequeueAfter: r.requeueDelay()}, nil
	}

	// If we get here then it means we can process the result of the operation.
	logger.Info("Resource is in desired state.", "resourceId", recipe.Status.Resource)

	err = r.updateSecret(ctx, recipe)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to process secret %s: %w", recipe.Spec.SecretName, err)
	}

	recipe.Status.Phrase = radappiov1alpha3.PhraseReady
	err = r.Client.Status().Update(ctx, recipe)
	if err != nil {
		return ctrl.Result{}, err
	}

	r.EventRecorder.Event(recipe, corev1.EventTypeNormal, "Reconciled", "Successfully reconciled resource.")
	return ctrl.Result{}, nil
}

func (r *RecipeReconciler) reconcileDelete(ctx context.Context, recipe *radappiov1alpha3.Recipe) (ctrl.Result, error) {
	logger := ucplog.FromContextOrDiscard(ctx)

	// Since we're going to reconcile, update the observed generation.
	//
	// We don't want to do this if we're in the middle of an operation, because we haven't
	// fully processed any status changes until the async operation completes.
	recipe.Status.ObservedGeneration = recipe.Generation

	poller, err := r.startDeleteOperationIfNeeded(ctx, recipe)
	if err != nil {
		logger.Error(err, "Unable to delete resource.")
		r.EventRecorder.Event(recipe, corev1.EventTypeWarning, "ResourceError", err.Error())
		return ctrl.Result{}, err
	} else if poller != nil {
		// We've successfully started an operation. Update the status and requeue.
		token, err := poller.ResumeToken()
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to get operation token: %w", err)
		}

		recipe.Status.Operation = &radappiov1alpha3.ResourceOperation{ResumeToken: token, OperationKind: radappiov1alpha3.OperationKindDelete}
		recipe.Status.Phrase = radappiov1alpha3.PhraseDeleting
		err = r.Client.Status().Update(ctx, recipe)
		if err != nil {
			return ctrl.Result{}, err
		}

		return ctrl.Result{Requeue: true, RequeueAfter: r.requeueDelay()}, nil
	}

	logger.Info("Resource is deleted.")

	err = r.deleteSecret(ctx, recipe)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to process secret %s: %w", recipe.Spec.SecretName, err)
	}

	// At this point we've cleaned up everything. We can remove the finalizer which will allow deletion of the
	// recipe.
	if controllerutil.RemoveFinalizer(recipe, RecipeFinalizer) {
		err := r.Client.Update(ctx, recipe)
		if err != nil {
			return ctrl.Result{}, err
		}

		recipe.Status.ObservedGeneration = recipe.Generation
	}

	recipe.Status.Phrase = radappiov1alpha3.PhraseDeleted
	err = r.Client.Status().Update(ctx, recipe)
	if err != nil {
		return ctrl.Result{}, err
	}

	r.EventRecorder.Event(recipe, corev1.EventTypeNormal, "Reconciled", "Successfully reconciled resource.")
	return ctrl.Result{}, nil
}

func (r *RecipeReconciler) startPutOrDeleteOperationIfNeeded(ctx context.Context, recipe *radappiov1alpha3.Recipe) (sdkclients.Poller[generated.GenericResourcesClientCreateOrUpdateResponse], sdkclients.Poller[generated.GenericResourcesClientDeleteResponse], error) {
	logger := ucplog.FromContextOrDiscard(ctx)

	resourceID := recipe.Status.Scope + "/providers/" + recipe.Spec.Type + "/" + recipe.Name
	if recipe.Status.Resource != "" && !strings.EqualFold(recipe.Status.Resource, resourceID) {
		// If we get here it means that the environment or application changed, so we should delete
		// the old resource and create a new one.
		logger.Info("Resource is already created but is out-of-date")

		logger.Info("Starting DELETE operation.")
		poller, err := deleteResource(ctx, r.Radius, recipe.Status.Resource)
		if err != nil {
			return nil, nil, err
		} else if poller != nil {
			return nil, poller, nil
		}

		// Deletion was synchronous
		recipe.Status.Resource = ""
	}

	// Note: we separate this check from the previous block, because it could complete synchronously.
	if recipe.Status.Resource != "" {
		logger.Info("Resource is already created and is up-to-date.")
		return nil, nil, nil
	}

	logger.Info("Starting PUT operation.")
	properties := map[string]any{
		"application":          recipe.Status.Application,
		"environment":          recipe.Status.Environment,
		"resourceProvisioning": "recipe",
	}

	poller, err := createOrUpdateResource(ctx, r.Radius, resourceID, properties)
	if err != nil {
		return nil, nil, err
	} else if poller != nil {
		return poller, nil, nil
	}

	// Update was synchronous
	recipe.Status.Resource = resourceID
	return nil, nil, nil
}

func (r *RecipeReconciler) startDeleteOperationIfNeeded(ctx context.Context, recipe *radappiov1alpha3.Recipe) (sdkclients.Poller[generated.GenericResourcesClientDeleteResponse], error) {
	logger := ucplog.FromContextOrDiscard(ctx)
	if recipe.Status.Resource == "" {
		logger.Info("Resource is already deleted (or was never created).")
		return nil, nil
	}

	logger.Info("Starting DELETE operation.")
	poller, err := deleteResource(ctx, r.Radius, recipe.Status.Resource)
	if err != nil {
		return nil, err
	} else if poller != nil {
		return poller, err
	}

	// Deletion was synchronous

	recipe.Status.Resource = ""
	return nil, nil
}

func (r *RecipeReconciler) updateSecret(ctx context.Context, recipe *radappiov1alpha3.Recipe) error {
	logger := ucplog.FromContextOrDiscard(ctx)

	// If the secret name changed, delete the old secret.
	if recipe.Spec.SecretName != recipe.Status.Secret.Name && recipe.Status.Secret.Name != "" {
		logger.Info("Deleting stale secret", "secret", recipe.Status.Secret.Name)
		err := r.Client.Delete(ctx, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      recipe.Status.Secret.Name,
				Namespace: recipe.Namespace,
			},
		})
		if err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to delete stale secret %s: %w", recipe.Status.Secret.Name, err)
		}
	}

	if recipe.Spec.SecretName == "" {
		logger.Info("No secret name specified, skipping secret creation")
		recipe.Status.Secret = corev1.ObjectReference{}
		return nil
	}

	logger.Info("Creating or updating secret.", "secret", recipe.Spec.SecretName)
	result, err := fetchResource(ctx, r.Radius, recipe.Status.Resource)
	if err != nil {
		return fmt.Errorf("failed to read resource: %w", err)
	}

	secret := &corev1.Secret{}
	err = r.Client.Get(ctx, client.ObjectKey{Namespace: recipe.Namespace, Name: recipe.Spec.SecretName}, secret)
	if apierrors.IsNotFound(err) {
		// This is OK, we'll create it next.
		secret = nil
	} else if err != nil {
		return fmt.Errorf("failed to fetch secret %s: %w", recipe.Spec.SecretName, err)
	}

	// Initialize the secret if it doesn't exist.
	if secret == nil {
		secret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      recipe.Spec.SecretName,
				Namespace: recipe.Namespace,
				OwnerReferences: []metav1.OwnerReference{
					*metav1.NewControllerRef(recipe, radappiov1alpha3.GroupVersion.WithKind("Recipe")),
				},
			},
		}

		err = r.Client.Create(ctx, secret)
		if err != nil {
			return fmt.Errorf("failed to create secret %s: %w", secret.Name, err)
		}
	}

	// envtest has some quirky behavior around StringData which makes it hard to test. So we're
	// using Data directly.
	if secret.Data == nil {
		secret.Data = map[string][]byte{}
	}

	values, err := resourceToConnectionValues(result.GenericResource)
	if err != nil {
		return fmt.Errorf("failed to read connection values: %w", err)
	}

	for k, v := range values {
		secret.Data[k] = []byte(v)
	}

	secrets, err := r.Radius.Resources(recipe.Status.Scope, recipe.Spec.Type).ListSecrets(ctx, recipe.Name)
	if clients.Is404Error(err) {
		// Safe to ignore. Not everything implements this.
	} else if err != nil {
		return fmt.Errorf("failed to list secrets: %w", err)
	} else {
		for k, v := range secrets.Value {
			secret.Data[k] = []byte(*v)
		}
	}

	err = r.Client.Update(ctx, secret)
	if err != nil {
		return fmt.Errorf("failed to update secret %s: %w", secret.Name, err)
	}

	recipe.Status.Secret = corev1.ObjectReference{
		APIVersion: "v1",
		Kind:       "Secret",
		Namespace:  secret.Namespace,
		Name:       secret.Name,
		UID:        secret.UID,
	}

	return nil
}

func (r *RecipeReconciler) deleteSecret(ctx context.Context, recipe *radappiov1alpha3.Recipe) error {
	logger := ucplog.FromContextOrDiscard(ctx)

	if recipe.Status.Secret.Name != "" {
		logger.Info("Deleting secret.", "secret", recipe.Status.Secret.Name)
		err := r.Client.Delete(ctx, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      recipe.Status.Secret.Name,
				Namespace: recipe.Namespace,
			},
		})
		if err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to delete secret %s: %w", recipe.Status.Secret.Name, err)
		}
	}

	recipe.Status.Secret = corev1.ObjectReference{}
	return nil
}

func (r *RecipeReconciler) requeueDelay() time.Duration {
	delay := r.DelayInterval
	if delay == 0 {
		delay = PollingDelay
	}

	return delay
}

// SetupWithManager sets up the controller with the Manager.
func (r *RecipeReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&radappiov1alpha3.Recipe{}).
		Owns(&corev1.Secret{}).
		Complete(r)
}
