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
	"encoding/json"
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
	"github.com/radius-project/radius/pkg/cli/clients_new/generated"
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

	// DelayInterval is the amount of time to wait between operations.
	DelayInterval time.Duration
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

	if deploymentResource.ObjectMeta.DeletionTimestamp != nil {
		return r.reconcileDelete(ctx, &deploymentResource)
	}

	logger.Info("Resource is in desired state.", "resourceId", deploymentResource.Spec.Id)

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
		providerConfig := sdkclients.ProviderConfig{}
		err := json.Unmarshal([]byte(deploymentResource.Spec.ProviderConfig), &providerConfig)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to unmarshal providerConfig: %w", err)
		}

		poller, err := r.Radius.Resources(providerConfig.Deployments.Value.Scope, deploymentResourceType).ContinueDeleteOperation(ctx, deploymentResource.Status.Operation.ResumeToken)
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
				if controllerutil.RemoveFinalizer(deploymentResource, DeploymentResourceFinalizer) {
					deploymentResource.Status.ObservedGeneration = deploymentResource.Generation
					deploymentResource.Status.Phrase = radappiov1alpha3.DeploymentResourcePhraseDeleted
					err = r.Client.Update(ctx, deploymentResource)
					if err != nil {
						return ctrl.Result{}, err
					}
				}
				return ctrl.Result{}, nil
			}

			// Operation failed, reset state and retry.
			r.EventRecorder.Event(deploymentResource, corev1.EventTypeWarning, "ResourceError", err.Error())
			logger.Error(err, "Delete failed.")

			deploymentResource.Status.Operation = nil
			deploymentResource.Status.Phrase = radappiov1alpha3.DeploymentResourcePhraseFailed
			err = r.Client.Status().Update(ctx, deploymentResource)
			if err != nil {
				return ctrl.Result{}, err
			}

			return ctrl.Result{Requeue: true, RequeueAfter: r.requeueDelay()}, nil
		}

		// If we get here, the operation was a success. Update the status and continue.
		deploymentResource.Status.Operation = nil
		deploymentResource.Status.Phrase = radappiov1alpha3.DeploymentResourcePhraseDeleted
		err = r.Client.Status().Update(ctx, deploymentResource)
		if err != nil {
			return ctrl.Result{}, err
		}

		logger.Info("Resource is deleted.")

		// At this point we've cleaned up everything. We can remove the finalizer which will allow deletion of the
		// DeploymentResource
		if controllerutil.RemoveFinalizer(deploymentResource, DeploymentResourceFinalizer) {
			deploymentResource.Status.ObservedGeneration = deploymentResource.Generation
			deploymentResource.Status.Phrase = radappiov1alpha3.DeploymentResourcePhraseDeleted
			err = r.Client.Update(ctx, deploymentResource)
			if err != nil {
				return ctrl.Result{}, err
			}
		}

		return ctrl.Result{}, nil
	}

	// If we get here, this was an unknown operation kind. This is a bug in our code, or someone
	// tampered with the status of the object. Just reset the state and move on.
	logger.Error(fmt.Errorf("unknown operation kind: %s", deploymentResource.Status.Operation.OperationKind), "Unknown operation kind.")

	deploymentResource.Status.Operation = nil
	deploymentResource.Status.Phrase = radappiov1alpha3.DeploymentResourcePhraseFailed
	err := r.Client.Status().Update(ctx, deploymentResource)
	if err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *DeploymentResourceReconciler) reconcileDelete(ctx context.Context, deploymentResource *radappiov1alpha3.DeploymentResource) (ctrl.Result, error) {
	logger := ucplog.FromContextOrDiscard(ctx)

	logger.Info("Resource is being deleted.", "resourceId", deploymentResource.Spec.Id)

	// Since we're going to reconcile, update the observed generation.
	//
	// We don't want to do this if we're in the middle of an operation, because we haven't
	// fully processed any status changes until the async operation completes.
	deploymentResource.Status.ObservedGeneration = deploymentResource.Generation

	// Check if the resource is being used by another resource
	deploymentResourceList, err := listResourcesWithSameOwner(ctx, r.Client, deploymentResource.Namespace, deploymentResource.OwnerReferences[0])
	if err != nil {
		return ctrl.Result{}, err
	}

	// Check if the resource is being used by another resource
	dependentResource, err := checkForDeploymentResourceDependencies(deploymentResource, deploymentResourceList)
	if err != nil {
		return ctrl.Result{}, err
	}

	if dependentResource != "" {
		logger.Info("Resource is an application or environment, being used by another resource.", "resourceId", deploymentResource.Spec.Id, "dependentResource", dependentResource)
		return ctrl.Result{Requeue: true, RequeueAfter: r.requeueDelay()}, nil
	}

	poller, err := r.startDeleteOperation(ctx, deploymentResource)
	if err != nil {
		logger.Error(err, "Unable to delete resource.")
		r.EventRecorder.Event(deploymentResource, corev1.EventTypeWarning, "ResourceError", err.Error())
		return ctrl.Result{}, err
	} else if poller != nil {
		// We've successfully started an operation. Update the status and requeue.
		token, err := poller.ResumeToken()
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to get operation token: %w", err)
		}

		deploymentResource.Status.Operation = &radappiov1alpha3.ResourceOperation{ResumeToken: token, OperationKind: radappiov1alpha3.OperationKindDelete}
		deploymentResource.Status.Phrase = radappiov1alpha3.DeploymentResourcePhraseDeleting
		err = r.Client.Status().Update(ctx, deploymentResource)
		if err != nil {
			return ctrl.Result{}, err
		}

		return ctrl.Result{Requeue: true, RequeueAfter: r.requeueDelay()}, nil
	}

	logger.Info("Resource is deleted.")

	// At this point we've cleaned up everything. We can remove the finalizer which will allow deletion of the
	// DeploymentResource
	if controllerutil.RemoveFinalizer(deploymentResource, DeploymentResourceFinalizer) {
		deploymentResource.Status.ObservedGeneration = deploymentResource.Generation
		deploymentResource.Status.Phrase = radappiov1alpha3.DeploymentResourcePhraseDeleted
		err = r.Client.Update(ctx, deploymentResource)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	logger.Info("Finalizer was not removed, requeueing.")

	err = r.Client.Status().Update(ctx, deploymentResource)
	if err != nil {
		return ctrl.Result{}, err
	}

	// If we get here, then we're in a bad state. We should have removed the finalizer, but we didn't.
	// We should requeue and try again.

	return ctrl.Result{Requeue: true, RequeueAfter: r.requeueDelay()}, nil
}

func (r *DeploymentResourceReconciler) startDeleteOperation(ctx context.Context, deploymentResource *radappiov1alpha3.DeploymentResource) (Poller[generated.GenericResourcesClientDeleteResponse], error) {
	logger := ucplog.FromContextOrDiscard(ctx)

	resourceId := deploymentResource.Spec.Id

	logger.Info("Starting DELETE operation.")
	poller, err := deleteResource(ctx, r.Radius, resourceId)
	if err != nil {
		return nil, err
	} else if poller != nil {
		return poller, nil
	}

	// Deletion was synchronous
	return nil, nil
}

func (r *DeploymentResourceReconciler) requeueDelay() time.Duration {
	delay := r.DelayInterval
	if delay == 0 {
		delay = PollingDelay
	}

	return delay
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
