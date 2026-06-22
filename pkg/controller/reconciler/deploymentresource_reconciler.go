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
	"context"
	"fmt"
	"strings"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/go-logr/logr"
	"github.com/radius-project/radius/pkg/cli/clients"
	radappiov1alpha3 "github.com/radius-project/radius/pkg/controller/api/radapp.io/v1alpha3"
	sdkclients "github.com/radius-project/radius/pkg/sdk/clients"
	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
	corev1 "k8s.io/api/core/v1"
)

// DeploymentResourceReconciler reconciles a DeploymentResource object.
type DeploymentResourceReconciler struct {
	// Client is the Kubernetes client.
	Client client.Client

	// Scheme is the Kubernetes scheme.
	Scheme *runtime.Scheme

	// EventRecorder is the Kubernetes event recorder.
	EventRecorder record.EventRecorder

	// Radius is the Radius client.
	Radius RadiusClient

	// ResourceDeploymentsClient is the client for managing deployments.
	ResourceDeploymentsClient sdkclients.ResourceDeploymentsClient

	// DelayInterval is the amount of time to wait between operations.
	DelayInterval time.Duration

	// DeleteRetryInterval overrides DeleteRetryDelay for delete-path error
	// requeues. If zero, DeleteRetryDelay is used.
	DeleteRetryInterval time.Duration
}

// Reconcile is the main reconciliation loop for the DeploymentResource resource.
func (r *DeploymentResourceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := ucplog.FromContextOrDiscard(ctx).WithValues("kind", "DeploymentResource", "name", req.Name, "namespace", req.Namespace)
	ctx = logr.NewContext(ctx, logger)

	deploymentResource := radappiov1alpha3.DeploymentResource{}
	err := r.Client.Get(ctx, req.NamespacedName, &deploymentResource)
	if apierrors.IsNotFound(err) {
		// This can happen due to a data-race if the Deployment Resource is created and then deleted before we can
		// reconcile it. There's nothing to do here.
		logger.Info("DeploymentResource is being deleted.")
		return ctrl.Result{}, nil
	} else if err != nil {
		logger.Error(err, "Unable to fetch resource.")
		return ctrl.Result{}, err
	}

	// Our algorithm is as follows:
	//
	// 1. Check if there is an in-progress deletion. If so, check its status:
	// 	1. If the deletion is still in progress, then queue another reconcile operation and continue processing.
	// 	2. If the deletion completed successfully, then remove the `radapp.io/deployment-resource-finalizer` finalizer from the resource and continue processing.
	// 	3. If the operation failed, then update the `status.phrase` and `status.message` as `Failed`.
	// 2. If the `DeploymentTemplate` is being deleted, then process deletion:
	// 	1. Send a DELETE operation to the Radius API to delete the resource specified in the `spec.resourceId` field.
	// 	2. Continue processing.
	// 3. If the `DeploymentTemplate` is not being deleted then process this as a create or update:
	// 	1. Set the `status.phrase` for the `DeploymentResource` to `Ready`.
	// 	2. Continue processing.
	//
	// We do it this way because it guarantees that we only have one operation going at a time.

	if deploymentResource.Status.Operation != nil {
		result, err := r.reconcileOperation(ctx, &deploymentResource)
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

	if deploymentResource.DeletionTimestamp != nil {
		return r.reconcileDelete(ctx, &deploymentResource)
	}

	logger.Info("Resource is in desired state.")

	deploymentResource.Status.Phrase = radappiov1alpha3.DeploymentResourcePhraseReady
	deploymentResource.Status.Id = deploymentResource.Spec.Id
	err = r.Client.Status().Update(ctx, &deploymentResource)
	if err != nil {
		return ctrl.Result{}, err
	}

	r.EventRecorder.Event(&deploymentResource, corev1.EventTypeNormal, "Reconciled", "Successfully reconciled resource.")
	return ctrl.Result{}, nil
}

// reconcileOperation reconciles a DeploymentResource that has an operation in progress.
func (r *DeploymentResourceReconciler) reconcileOperation(ctx context.Context, deploymentResource *radappiov1alpha3.DeploymentResource) (ctrl.Result, error) {
	logger := ucplog.FromContextOrDiscard(ctx)

	if deploymentResource.Status.Operation.OperationKind == radappiov1alpha3.OperationKindDelete {
		poller, err := r.ResourceDeploymentsClient.ContinueDeleteOperation(ctx, deploymentResource.Status.Operation.ResumeToken)
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
			if clients.Is404Error(err) {
				// The resource was not found, so we can consider it deleted.
				logger.Info("Resource was not found.")

				// At this point we've cleaned up everything. We can remove the finalizer which will allow deletion of the
				// DeploymentResource
				if err := r.completeDeleteOperation(ctx, deploymentResource); err != nil {
					return ctrl.Result{}, err
				}
				return ctrl.Result{}, nil
			}

			// Operation failed, reset state and retry.
			r.EventRecorder.Event(deploymentResource, corev1.EventTypeWarning, "ResourceError", err.Error())
			logger.Error(err, "Delete failed.")

			if statusErr := r.updateFailedStatus(ctx, deploymentResource); statusErr != nil {
				return ctrl.Result{}, statusErr
			}

			// Bounded RequeueAfter instead of returning the error: avoids
			// controller-runtime's exponential rate-limiter for transient
			// dependency errors during a delete cascade.
			return ctrl.Result{RequeueAfter: r.deleteRetryDelay()}, nil
		}

		// If we get here, the operation was a success. Update the status and remove finalizer.
		logger.Info("Resource is deleted.")

		// At this point we've cleaned up everything. We can remove the finalizer which will allow deletion of the
		// DeploymentResource. Also update the status in the same update to avoid multiple API calls.
		if err := r.completeDeleteOperation(ctx, deploymentResource); err != nil {
			return ctrl.Result{}, err
		}

		return ctrl.Result{}, nil
	}

	// If we get here, this was an unknown operation kind. This is a bug in our code, or someone
	// tampered with the status of the object. Just reset the state and move on.
	logger.Error(fmt.Errorf("unknown operation kind: %s", deploymentResource.Status.Operation.OperationKind), "Unknown operation kind.")

	if err := r.updateFailedStatus(ctx, deploymentResource); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *DeploymentResourceReconciler) reconcileDelete(ctx context.Context, deploymentResource *radappiov1alpha3.DeploymentResource) (ctrl.Result, error) {
	logger := ucplog.FromContextOrDiscard(ctx)

	logger.Info("Resource is being deleted.")

	// Only delete the referenced resource if this controller provisioned it for a DeploymentTemplate.
	// A DeploymentResource is eligible for deletion when it is a controller-owned child of an existing
	// DeploymentTemplate and its Spec.Id falls within that template's deployment scope. Skipping the
	// delete otherwise avoids issuing a UCP delete for a resource the controller never created (for
	// example a DeploymentResource whose Spec.Id points outside its owning template).
	ownerRef := metav1.GetControllerOf(deploymentResource)
	eligible, err := r.deleteIsEligible(ctx, deploymentResource, ownerRef)
	if err != nil {
		return ctrl.Result{}, err
	}
	if !eligible {
		logger.Info("Skipping delete: resource is not within an owning DeploymentTemplate's scope.", "resourceId", deploymentResource.Spec.Id)
		r.EventRecorder.Event(deploymentResource, corev1.EventTypeNormal, EventDeploymentResourceDeleteSkipped,
			fmt.Sprintf("Skipping delete of %q: it is not within the owning DeploymentTemplate's deployment scope", deploymentResource.Spec.Id))

		// Nothing to delete in UCP. Remove the finalizer so the object is not wedged in
		// Terminating; the controller never provisioned the referenced resource.
		return ctrl.Result{}, r.completeDeleteOperation(ctx, deploymentResource)
	}

	// Check if the resource is being used by another resource. ownerRef is guaranteed non-nil
	// here because the eligibility check above succeeded.
	deploymentResourceList, err := listResourcesWithSameOwner(ctx, r.Client, deploymentResource.Namespace, *ownerRef)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Check if the resource is being used by another resource
	dependentResource, err := checkForDeploymentResourceDependencies(deploymentResource, deploymentResourceList)
	if err != nil {
		return ctrl.Result{}, err
	}

	if dependentResource != "" {
		logger.Info("Resource is an application or environment, being used by another resource. Waiting for dependent resource to be deleted.", "resourceId", deploymentResource.Spec.Id, "dependentResource", dependentResource)
		// Requeue after a delay to check dependencies again
		return ctrl.Result{Requeue: true, RequeueAfter: r.requeueDelay()}, nil
	}

	// Since we're going to proceed with deletion, update the observed generation and status.
	//
	// We don't want to do this if we're in the middle of an operation, because we haven't
	// fully processed any status changes until the async operation completes.
	deploymentResource.Status.ObservedGeneration = deploymentResource.Generation
	deploymentResource.Status.Phrase = radappiov1alpha3.DeploymentResourcePhraseDeleting
	err = r.Client.Status().Update(ctx, deploymentResource)
	if err != nil {
		return ctrl.Result{}, err
	}

	deletePoller, err := r.startDeleteOperation(ctx, deploymentResource)
	if err != nil {
		logger.Error(err, "Unable to delete resource.")
		r.EventRecorder.Event(deploymentResource, corev1.EventTypeWarning, "ResourceError", err.Error())
		if statusErr := r.updateFailedStatus(ctx, deploymentResource); statusErr != nil {
			return ctrl.Result{}, statusErr
		}
		// Bounded retry; see reconcileOperation.
		return ctrl.Result{RequeueAfter: r.deleteRetryDelay()}, nil
	} else if deletePoller != nil && !deletePoller.Done() {
		// We've successfully started an operation. Update the status and requeue.
		token, err := deletePoller.ResumeToken()
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to get operation token: %w", err)
		}

		deploymentResource.Status.Operation = &radappiov1alpha3.ResourceOperation{ResumeToken: token, OperationKind: radappiov1alpha3.OperationKindDelete}
		err = r.Client.Status().Update(ctx, deploymentResource)
		if err != nil {
			return ctrl.Result{}, err
		}

		return ctrl.Result{Requeue: true, RequeueAfter: r.requeueDelay()}, nil
	} else if deletePoller != nil && deletePoller.Done() {
		// Synchronous delete completed, but we need to verify it succeeded
		_, err = deletePoller.Result(ctx)
		if err != nil {
			if clients.Is404Error(err) {
				// The resource was not found, so we can consider it deleted.
				logger.Info("Resource was not found during synchronous delete.")
			} else {
				// Delete failed, update status and return error
				logger.Error(err, "Synchronous delete failed.")
				r.EventRecorder.Event(deploymentResource, corev1.EventTypeWarning, "ResourceError", err.Error())

				if statusErr := r.updateFailedStatus(ctx, deploymentResource); statusErr != nil {
					return ctrl.Result{}, statusErr
				}
				// Bounded retry; see reconcileOperation.
				return ctrl.Result{RequeueAfter: r.deleteRetryDelay()}, nil
			}
		}
	}

	// If we get here then the delete operation succeeded (either synchronously or the resource wasn't found).
	logger.Info("Resource is deleted.")

	// At this point we've cleaned up everything. We can remove the finalizer which will allow deletion of the
	// DeploymentResource
	if err := r.completeDeleteOperation(ctx, deploymentResource); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *DeploymentResourceReconciler) startDeleteOperation(ctx context.Context, deploymentResource *radappiov1alpha3.DeploymentResource) (sdkclients.Poller[sdkclients.ClientDeleteResponse], error) {
	logger := ucplog.FromContextOrDiscard(ctx)

	resourceId := deploymentResource.Spec.Id
	radiusAPIVersion := "2023-10-01-preview"

	logger.Info("Starting DELETE operation.")
	poller, err := r.ResourceDeploymentsClient.Delete(ctx, resourceId, radiusAPIVersion)
	if err != nil {
		return nil, err
	} else if poller != nil {
		return poller, nil
	}

	// Deletion was synchronous
	return nil, nil
}

// deleteIsEligible reports whether the controller should issue a UCP delete for deploymentResource.
// A DeploymentResource is eligible only when it is controlled by a DeploymentTemplate that still
// exists in the same namespace and the resource it references (Spec.Id) is within that template's
// deployment scope. This keeps the controller from deleting a resource it did not provision, such as
// a DeploymentResource whose Spec.Id points outside its owning template. It returns false (without an
// error) when eligibility cannot be established so the caller can safely skip the delete.
func (r *DeploymentResourceReconciler) deleteIsEligible(ctx context.Context, deploymentResource *radappiov1alpha3.DeploymentResource, ownerRef *metav1.OwnerReference) (bool, error) {
	logger := ucplog.FromContextOrDiscard(ctx)

	// DeploymentResources created by the controller always have their owning DeploymentTemplate as
	// the controller reference.
	if ownerRef == nil || ownerRef.Kind != deploymentTemplateKind {
		logger.Info("DeploymentResource is not controlled by a DeploymentTemplate.")
		return false, nil
	}

	owner := &radappiov1alpha3.DeploymentTemplate{}
	err := r.Client.Get(ctx, client.ObjectKey{Namespace: deploymentResource.Namespace, Name: ownerRef.Name}, owner)
	if apierrors.IsNotFound(err) {
		logger.Info("Owning DeploymentTemplate was not found.", "owner", ownerRef.Name)
		return false, nil
	} else if err != nil {
		return false, err
	}

	// Guard against an owner reference that names an existing DeploymentTemplate but carries a stale
	// UID (for example a template that was deleted and recreated under the same name).
	if owner.UID != ownerRef.UID {
		logger.Info("Owning DeploymentTemplate UID does not match the owner reference.", "owner", ownerRef.Name)
		return false, nil
	}

	scope, err := ParseDeploymentScopeFromProviderConfig(owner.Spec.ProviderConfig)
	if err != nil {
		return false, fmt.Errorf("failed to determine deployment scope from owning DeploymentTemplate: %w", err)
	}

	within, err := resourceWithinScope(deploymentResource.Spec.Id, scope)
	if err != nil {
		// An unparseable Spec.Id cannot be checked against the scope, so skip the delete rather than
		// surfacing a retryable error for input that can never become valid.
		logger.Info("Unable to parse resource id for scope check; skipping delete.", "resourceId", deploymentResource.Spec.Id, "error", err.Error())
		return false, nil
	}
	if !within {
		logger.Info("Resource is outside the owning DeploymentTemplate's deployment scope.", "resourceId", deploymentResource.Spec.Id, "expectedScope", scope)
		return false, nil
	}

	return true, nil
}

// resourceWithinScope reports whether resourceID belongs to the given deployment scope (the root
// scope, e.g. "/planes/radius/local/resourceGroups/my-group"). The comparison is case-insensitive
// because resource ids and scopes can legitimately differ in casing.
func resourceWithinScope(resourceID string, scope string) (bool, error) {
	id, err := resources.ParseResource(resourceID)
	if err != nil {
		return false, err
	}

	return strings.EqualFold(strings.TrimRight(id.RootScope(), "/"), strings.TrimRight(scope, "/")), nil
}

func (r *DeploymentResourceReconciler) requeueDelay() time.Duration {
	delay := r.DelayInterval
	if delay == 0 {
		delay = PollingDelay
	}

	return delay
}

func (r *DeploymentResourceReconciler) deleteRetryDelay() time.Duration {
	if r.DeleteRetryInterval != 0 {
		return r.DeleteRetryInterval
	}
	return DeleteRetryDelay
}

// completeDeleteOperation removes the finalizer and updates the resource status to mark it as deleted.
// This helper reduces duplication across the reconciler.
func (r *DeploymentResourceReconciler) completeDeleteOperation(ctx context.Context, deploymentResource *radappiov1alpha3.DeploymentResource) error {
	if controllerutil.RemoveFinalizer(deploymentResource, DeploymentResourceFinalizer) {
		deploymentResource.Status.ObservedGeneration = deploymentResource.Generation
		deploymentResource.Status.Operation = nil
		deploymentResource.Status.Phrase = radappiov1alpha3.DeploymentResourcePhraseDeleted
		return r.Client.Update(ctx, deploymentResource)
	}
	return nil
}

// updateFailedStatus updates the resource status to failed state and clears the operation.
// This helper reduces duplication when handling operation failures.
func (r *DeploymentResourceReconciler) updateFailedStatus(ctx context.Context, deploymentResource *radappiov1alpha3.DeploymentResource) error {
	deploymentResource.Status.Operation = nil
	deploymentResource.Status.Phrase = radappiov1alpha3.DeploymentResourcePhraseFailed
	return r.Client.Status().Update(ctx, deploymentResource)
}

// SetupWithManager sets up the controller with the Manager.
func (r *DeploymentResourceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&radappiov1alpha3.DeploymentResource{}).
		Complete(r)
}

func listResourcesWithSameOwner(ctx context.Context, c client.Client, namespace string, ownerRef metav1.OwnerReference) ([]radappiov1alpha3.DeploymentResource, error) {
	// List all DeploymentResource objects in the same namespace
	deploymentResourceList := &radappiov1alpha3.DeploymentResourceList{}
	err := c.List(ctx, deploymentResourceList, client.InNamespace(namespace))
	if err != nil {
		return nil, err
	}

	// Filter resources based on OwnerReference
	var filteredResources []radappiov1alpha3.DeploymentResource
	for _, dr := range deploymentResourceList.Items {
		for _, or := range dr.OwnerReferences {
			if or.UID == ownerRef.UID {
				filteredResources = append(filteredResources, dr)
				break
			}
		}
	}

	return filteredResources, nil
}

// checkForDeploymentResourceDependencies checks if the deploymentResource is an application or environment.
// If it is, it checks if other (non-application or environment) resources exist.
// If other resources exist, it returns the ID of one of the dependent resources.
// NOTE: This is a workaround for existing Radius API behavior. Since deleting
// an application or environment can leave hanging resources, we need to make sure to
// delete these resources before deleting the application or environment.
// https://github.com/radius-project/radius/issues/8164
func checkForDeploymentResourceDependencies(deploymentResource *radappiov1alpha3.DeploymentResource, deploymentResourceList []radappiov1alpha3.DeploymentResource) (string, error) {
	deploymentResourceID, err := resources.ParseResource(deploymentResource.Spec.Id)
	if err != nil {
		return "", err
	}

	// If the deploymentResource is an application or environment, check if other resources exist
	if strings.EqualFold(deploymentResourceID.Type(), "Applications.Core/applications") || strings.EqualFold(deploymentResourceID.Type(), "Applications.Core/environments") {
		resourceCount := 0
		dependentResource := ""
		for _, dr := range deploymentResourceList {
			if dr.Status.Phrase == radappiov1alpha3.DeploymentResourcePhraseDeleted {
				continue
			}

			id, err := resources.ParseResource(dr.Spec.Id)
			if err != nil {
				return "", err
			}

			// don't count applications or environments
			if !strings.EqualFold(id.Type(), "Applications.Core/applications") && !strings.EqualFold(id.Type(), "Applications.Core/environments") {
				resourceCount++
				dependentResource = dr.Spec.Id
			}
		}

		return dependentResource, nil
	}

	// If the deploymentResource is not an application or environment, just return
	return "", nil
}
