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
	"errors"
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"uuid"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armdeployments/v3"
	"github.com/go-logr/logr"
	"github.com/radius-project/radius/pkg/cli/clients"
	radappiov1alpha3 "github.com/radius-project/radius/pkg/controller/api/radapp.io/v1alpha3"
	"github.com/radius-project/radius/pkg/hashutil"
	sdkclients "github.com/radius-project/radius/pkg/sdk/clients"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
	corev1 "k8s.io/api/core/v1"
)

// DeploymentTemplateReconciler reconciles a DeploymentTemplate object.
type DeploymentTemplateReconciler struct {
	// Client is the Kubernetes client.
	Client client.Client

	// Scheme is the Kubernetes scheme.
	Scheme *k8sruntime.Scheme

	// EventRecorder is the Kubernetes event recorder.
	EventRecorder record.EventRecorder

	// Radius is the Radius client.
	Radius RadiusClient

	// ResourceDeploymentsClient is the client for managing deployments.
	ResourceDeploymentsClient sdkclients.ResourceDeploymentsClient

	// DelayInterval is the amount of time to wait between operations.
	DelayInterval time.Duration

	// SourcePolicy selects the sources that may drive generated templates.
	SourcePolicy *FluxSourcePolicy
}

// Reconcile is the main reconciliation loop for the DeploymentTemplate resource.
func (r *DeploymentTemplateReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := ucplog.FromContextOrDiscard(ctx).WithValues("kind", "DeploymentTemplate", "name", req.Name, "namespace", req.Namespace)
	ctx = logr.NewContext(ctx, logger)

	deploymentTemplate := radappiov1alpha3.DeploymentTemplate{}
	err := r.Client.Get(ctx, req.NamespacedName, &deploymentTemplate)
	if apierrors.IsNotFound(err) {
		// This can happen due to a data-race if the Deployment Template is created and then deleted before we can
		// reconcile it. There's nothing to do here.
		logger.Info("DeploymentTemplate is being deleted.")
		return ctrl.Result{}, nil
	} else if err != nil {
		logger.Error(err, "Unable to fetch resource.")
		return ctrl.Result{}, err
	}

	result, err := r.reconcileTemplate(ctx, &deploymentTemplate)
	var hold *sourceHold
	if errors.As(err, &hold) {
		if deploymentTemplate.Status.Phrase != radappiov1alpha3.DeploymentTemplatePhraseHeld {
			deploymentTemplate.Status.Phrase = radappiov1alpha3.DeploymentTemplatePhraseHeld
			if err := r.Client.Status().Update(ctx, &deploymentTemplate); err != nil {
				return ctrl.Result{}, err
			}
			return reportSourceHold(ctx, &deploymentTemplate, r.EventRecorder, hold), nil
		}
		return ctrl.Result{RequeueAfter: DeleteRetryDelay}, nil
	}
	return result, err
}

func (r *DeploymentTemplateReconciler) reconcileTemplate(ctx context.Context, deploymentTemplate *radappiov1alpha3.DeploymentTemplate) (ctrl.Result, error) {
	logger := ucplog.FromContextOrDiscard(ctx)
	// Our algorithm is as follows:
	//
	// 1. Check if there is an in-progress operation. If so, check its status:
	// 	1. If the operation is still in progress, then queue another reconcile operation and continue processing.
	// 	2. If the operation completed successfully:
	// 			1. Diff the resources in the `properties.outputResources` field returned by the Radius API with the resources in the `status.outputResources` field on the `DeploymentTemplate` resource.
	// 			2. Depending on the diff, create or delete `DeploymentResource` resources on the cluster. In the case of create, add the `DeploymentTemplate` as the owner of the `DeploymentResource` and set the `radapp.io/deployment-resource-finalizer` finalizer on the `DeploymentResource`.
	// 			3. Update the `status.phrase` for the `DeploymentTemplate` to `Ready`.
	// 			4. Continue processing.
	// 	3. If the operation failed, then update the `status.phrase` as `Failed` and continue processing.
	// 2. If the `DeploymentTemplate` is being deleted, then process deletion:
	// 	1. Since the `DeploymentResources` are owned by the `DeploymentTemplate`, the `DeploymentResource` resources will be deleted first. Once they are deleted, the `DeploymentTemplate` resource will be deleted.
	// 	2. Once the dependent resources are deleted, remove the `radapp.io/deployment-template-finalizer` finalizer from the `DeploymentTemplate`.
	// 3. If the `DeploymentTemplate` is not being deleted then process this as a create or update:
	// 	1. Add the `radapp.io/deployment-template-finalizer` finalizer onto the `DeploymentTemplate` resource.
	// 	2. Check if the desired state of the `DeploymentTemplate` resource matches the observed state. If it does, then the resource is up-to-date and we can continue processing.
	// 	3. Otherwise, queue a PUT operation against the Radius API to deploy the ARM JSON in the `spec.template` field with the parameters in the `spec.parameters` field.
	// 	4. Set the `status.phrase` for the `DeploymentTemplate` to `Updating` and the `status.operation` to the operation returned by the Radius API.
	// 	5. Continue processing.
	//
	// We do it this way because it guarantees that we only have one operation going at a time.

	if deploymentTemplate.Status.Operation != nil {
		result, err := r.reconcileOperation(ctx, deploymentTemplate)
		if err != nil {
			logger.Error(err, "Unable to reconcile in-progress operation.")
			return ctrl.Result{}, err
		} else if result.IsZero() {
			// If reconcileOperation completes successfully, then it will return a "zero" result,
			// this means the operation has completed and we should continue processing.
			logger.Info("Operation completed successfully.")
		} else {
			logger.Info("Requeueing to continue operation.")
			return result, nil
		}
	}

	if deploymentTemplate.ObjectMeta.DeletionTimestamp != nil {
		return r.reconcileDelete(ctx, deploymentTemplate)
	}

	return r.reconcileUpdate(ctx, deploymentTemplate)
}

// reconcileOperation reconciles a DeploymentTemplate that has an operation in progress.
func (r *DeploymentTemplateReconciler) reconcileOperation(ctx context.Context, deploymentTemplate *radappiov1alpha3.DeploymentTemplate) (ctrl.Result, error) {
	logger := ucplog.FromContextOrDiscard(ctx)

	if deploymentTemplate.Status.Operation.OperationKind == radappiov1alpha3.OperationKindPut {
		poller, err := r.ResourceDeploymentsClient.ContinueCreateOperation(ctx, deploymentTemplate.Status.Operation.ResumeToken)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to continue PUT operation: %w", err)
		}

		_, err = poller.Poll(ctx)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to poll operation status: %w", err)
		}

		if !poller.Done() {
			return ctrl.Result{RequeueAfter: r.requeueDelay()}, nil
		}

		// If we get here, the operation is complete.
		resp, err := poller.Result(ctx)
		if err != nil {
			// Special case: If deletion is requested and we get a 404 error, treat it as success
			// This can happen when resources are deleted while an operation is in progress
			if deploymentTemplate.ObjectMeta.DeletionTimestamp != nil && clients.Is404Error(err) {
				logger.Info("Operation returned 404 during deletion, treating as success.")
				deploymentTemplate.Status.Operation = nil
				deploymentTemplate.Status.OperationSpecHash = ""
				err := r.Client.Status().Update(ctx, deploymentTemplate)
				if err != nil {
					return ctrl.Result{}, err
				}
				return ctrl.Result{}, nil
			}

			// Operation failed, reset state and schedule delayed retry.
			r.EventRecorder.Event(deploymentTemplate, corev1.EventTypeWarning, "ResourceError", err.Error())
			logger.Error(err, "Update failed.")

			if err := r.updateFailedStatus(ctx, deploymentTemplate); err != nil {
				return ctrl.Result{}, err
			}

			return ctrl.Result{}, nil
		}

		if err := r.SourcePolicy.checkTemplate(ctx, deploymentTemplate); err != nil {
			return ctrl.Result{}, err
		}

		logger.Info("Creating output resources.")

		// Get outputResources from the response. A nil OutputResources field is
		// treated as an empty set so we still run the diff-delete logic below;
		// otherwise a deployment whose response omits OutputResources would
		// leave previously-tracked DRs dangling and stick the DT in
		// ReadyPendingCleanup forever.
		outputResources := make([]string, 0)
		if resp.Properties != nil && resp.Properties.OutputResources != nil {
			for _, resource := range resp.Properties.OutputResources {
				if resource.ID != nil {
					outputResources = append(outputResources, *resource.ID)
				}
			}
		}

		newOutputResources := make(map[string]bool)
		for _, resource := range outputResources {
			newOutputResources[resource] = true
		}

		desiredDeletionPending := false
		for _, outputResourceId := range outputResources {
			// Recreate missing trackers even when the ID was recorded before cleanup.
			resourceName, err := generateDeploymentResourceName(outputResourceId)
			if err != nil {
				return ctrl.Result{}, err
			}

			deploymentResource := &radappiov1alpha3.DeploymentResource{
				Name:      resourceName,
				Namespace: deploymentTemplate.Namespace,
				Spec:      radappiov1alpha3.DeploymentResourceSpec{Id: outputResourceId},
			}

			controllerutil.AddFinalizer(deploymentResource, DeploymentResourceFinalizer)
			if err := controllerutil.SetControllerReference(deploymentTemplate, deploymentResource, r.Scheme); err != nil {
				return ctrl.Result{}, err
			}

			if err := r.SourcePolicy.checkTemplate(ctx, deploymentTemplate); err != nil {
				return ctrl.Result{}, err
			}
			err = r.Client.Create(ctx, deploymentResource)
			if apierrors.IsAlreadyExists(err) {
				reader := client.Reader(r.Client)
				if r.SourcePolicy != nil && r.SourcePolicy.reader != nil {
					reader = r.SourcePolicy.reader
				}
				var existing radappiov1alpha3.DeploymentResource
				if err := reader.Get(ctx, client.ObjectKeyFromObject(deploymentResource), &existing); err != nil {
					return ctrl.Result{}, err
				}
				if !isOwnedBy(existing, deploymentTemplate) || existing.Spec.Id != outputResourceId {
					return ctrl.Result{}, &sourceHold{"OutputResourceMismatch", "An output resource is already tracked by a different template or resource ID"}
				}
				if existing.DeletionTimestamp != nil && !controllerutil.ContainsFinalizer(&existing, DeploymentResourceFinalizer) {
					// A retired tracker retained by another finalizer cannot own cleanup.
					// Keep the result until a replacement can be created.
					return ctrl.Result{RequeueAfter: r.requeueDelay()}, nil
				}
				recreating, err := r.recreateDependencyBlockedOutput(ctx, deploymentTemplate, &existing)
				if err != nil {
					return ctrl.Result{}, err
				}
				if recreating {
					return ctrl.Result{RequeueAfter: r.requeueDelay()}, nil
				}
				desiredDeletionPending = desiredDeletionPending || deploymentResourceDeletionPending(&existing)
			} else if err != nil {
				return ctrl.Result{}, err
			}
		}

		if deploymentTemplate.DeletionTimestamp != nil {
			// Transfer the completed output inventory to live trackers before
			// dropping the operation, including any retired during restoration.
			// Let owner cleanup delete all children without waiting on their
			// dependencies here.
			deploymentTemplate.Status.Operation = nil
			deploymentTemplate.Status.OperationSpecHash = ""
			deploymentTemplate.Status.OutputResources = outputResources
			deploymentTemplate.Status.Phrase = radappiov1alpha3.DeploymentTemplatePhraseDeleting
			return ctrl.Result{}, r.Client.Status().Update(ctx, deploymentTemplate)
		}

		for _, resource := range deploymentTemplate.Status.OutputResources {
			if _, ok := newOutputResources[resource]; !ok {
				// Resource is present in deploymentTemplate.Status.OutputResources but not in outputResources, delete it

				logger.Info("Deleting resource.", "resourceId", resource)
				resourceName, err := generateDeploymentResourceName(resource)
				if err != nil {
					return ctrl.Result{}, err
				}

				if err := r.SourcePolicy.checkTemplate(ctx, deploymentTemplate); err != nil {
					return ctrl.Result{}, err
				}
				var existing radappiov1alpha3.DeploymentResource
				err = r.Client.Get(ctx, client.ObjectKey{Name: resourceName, Namespace: deploymentTemplate.Namespace}, &existing)
				if apierrors.IsNotFound(err) {
					continue
				} else if err != nil {
					return ctrl.Result{}, err
				}
				if !isOwnedBy(existing, deploymentTemplate) || existing.Spec.Id != resource {
					return ctrl.Result{}, &sourceHold{"OutputResourceMismatch", "An obsolete output is no longer tracked by this template"}
				}
				err = r.Client.Delete(ctx, &existing, client.Preconditions{UID: &existing.UID, ResourceVersion: &existing.ResourceVersion})
				if client.IgnoreNotFound(err) != nil {
					return ctrl.Result{}, err
				}
			}
		}

		hash := deploymentTemplate.Status.OperationSpecHash
		// Legacy operations have no submitted hash. Only hash the current spec
		// if it still matches the generation that the operation submitted.
		if hash == "" && deploymentTemplate.Generation == deploymentTemplate.Status.ObservedGeneration {
			hash, err = computeHash(deploymentTemplate)
			if err != nil {
				return ctrl.Result{}, err
			}
		}

		cleanup, err := r.pendingDeploymentResourceCleanup(ctx, deploymentTemplate, outputResources)
		if err != nil {
			return ctrl.Result{}, err
		}
		desiredDeletionPending = desiredDeletionPending || cleanup.desiredDeleting
		cleanup.pending = cleanup.pending || desiredDeletionPending
		if desiredDeletionPending {
			// An older DELETE can erase this PUT's output. Reapply after it drains.
			hash = ""
		}

		// If we get here, the operation was a success. Update the status and continue.
		deploymentTemplate.Status.Operation = nil
		deploymentTemplate.Status.OperationSpecHash = ""
		deploymentTemplate.Status.OutputResources = outputResources
		deploymentTemplate.Status.StatusHash = hash
		if cleanup.pending {
			deploymentTemplate.Status.Phrase = radappiov1alpha3.DeploymentTemplatePhraseReadyPendingCleanup
		} else {
			deploymentTemplate.Status.Phrase = radappiov1alpha3.DeploymentTemplatePhraseReady
		}

		err = r.Client.Status().Update(ctx, deploymentTemplate)
		if err != nil {
			return ctrl.Result{}, err
		}

		if cleanup.pending {
			// Requeue as a backstop; the Owns(&DeploymentResource{}) watch also
			// wakes us up when DRs are removed.
			return ctrl.Result{RequeueAfter: r.requeueDelay()}, nil
		}

		return ctrl.Result{}, nil
	}

	// If we get here, this was an unknown operation kind. This is a bug in our code, or someone
	// tampered with the status of the object. Just reset the state and move on.
	errorMessage := fmt.Errorf("unknown operation kind: %s", deploymentTemplate.Status.Operation.OperationKind)
	logger.Error(errorMessage, "Unknown operation kind.")

	deploymentTemplate.Status.Operation = nil
	deploymentTemplate.Status.Phrase = radappiov1alpha3.DeploymentTemplatePhraseFailed
	err := r.Client.Status().Update(ctx, deploymentTemplate)
	if err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *DeploymentTemplateReconciler) reconcileUpdate(ctx context.Context, deploymentTemplate *radappiov1alpha3.DeploymentTemplate) (ctrl.Result, error) {
	logger := ucplog.FromContextOrDiscard(ctx)
	if err := r.SourcePolicy.checkTemplate(ctx, deploymentTemplate); err != nil {
		return ctrl.Result{}, err
	}

	logger.Info("Reconciling resource.")

	// Ensure that our finalizer is present before we start any operations.
	if controllerutil.AddFinalizer(deploymentTemplate, DeploymentTemplateFinalizer) {
		err := r.Client.Update(ctx, deploymentTemplate)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	cleanup, err := r.pendingDeploymentResourceCleanup(ctx, deploymentTemplate, deploymentTemplate.Status.OutputResources)
	if err != nil {
		return ctrl.Result{}, err
	}
	if cleanup.pending && (cleanup.blocksUpdate || isUpToDate(deploymentTemplate)) {
		changed := deploymentTemplate.Status.Phrase != radappiov1alpha3.DeploymentTemplatePhraseReadyPendingCleanup ||
			(cleanup.desiredDeleting && deploymentTemplate.Status.StatusHash != "")
		deploymentTemplate.Status.Phrase = radappiov1alpha3.DeploymentTemplatePhraseReadyPendingCleanup
		if cleanup.desiredDeleting {
			deploymentTemplate.Status.StatusHash = ""
		}
		if changed {
			if err := r.Client.Status().Update(ctx, deploymentTemplate); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{RequeueAfter: r.requeueDelay()}, nil
	}

	// Since we're going to reconcile, update the observed generation.
	//
	// We don't want to do this if we're in the middle of an operation, because we haven't
	// fully processed any status changes until the async operation completes.
	deploymentTemplate.Status.ObservedGeneration = deploymentTemplate.Generation

	updatePoller, err := r.startPutOperationIfNeeded(ctx, deploymentTemplate)
	if err != nil {
		var hold *sourceHold
		if errors.As(err, &hold) {
			return ctrl.Result{}, err
		}
		logger.Error(err, "Unable to create or update resource.")
		r.EventRecorder.Event(deploymentTemplate, corev1.EventTypeWarning, "ResourceError", err.Error())

		if statusErr := r.updateFailedStatus(ctx, deploymentTemplate); statusErr != nil {
			return ctrl.Result{}, statusErr
		}

		// Return the error to the controller-runtime.
		// This will cause the controller to requeue the request and retry the operation.
		return ctrl.Result{}, err
	} else if updatePoller != nil {
		// We've successfully started an operation. Update the status and requeue.
		token, err := updatePoller.ResumeToken()
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to get operation token: %w", err)
		}

		deploymentTemplate.Status.Operation = &radappiov1alpha3.ResourceOperation{ResumeToken: token, OperationKind: radappiov1alpha3.OperationKindPut}
		hash, err := computeHash(deploymentTemplate)
		if err != nil {
			return ctrl.Result{}, err
		}
		deploymentTemplate.Status.OperationSpecHash = hash
		deploymentTemplate.Status.Phrase = radappiov1alpha3.DeploymentTemplatePhraseUpdating
		err = r.Client.Status().Update(ctx, deploymentTemplate)
		if err != nil {
			return ctrl.Result{}, err
		}

		return ctrl.Result{RequeueAfter: r.requeueDelay()}, nil
	}

	// If we get here then it means we can process the result of the operation.
	logger.Info("Resource is in desired state.")

	deploymentTemplate.Status.Phrase = radappiov1alpha3.DeploymentTemplatePhraseReady
	err = r.Client.Status().Update(ctx, deploymentTemplate)
	if err != nil {
		return ctrl.Result{}, err
	}

	r.EventRecorder.Event(deploymentTemplate, corev1.EventTypeNormal, "Reconciled", "Successfully reconciled resource.")
	return ctrl.Result{}, nil
}

func (r *DeploymentTemplateReconciler) reconcileDelete(ctx context.Context, deploymentTemplate *radappiov1alpha3.DeploymentTemplate) (ctrl.Result, error) {
	logger := ucplog.FromContextOrDiscard(ctx)
	if err := r.SourcePolicy.checkTemplate(ctx, deploymentTemplate); err != nil {
		return ctrl.Result{}, err
	}

	logger.Info("Resource is being deleted.")

	// Since we're going to reconcile, update the observed generation.
	//
	// We don't want to do this if we're in the middle of an operation, because we haven't
	// fully processed any status changes until the async operation completes.
	deploymentTemplate.Status.ObservedGeneration = deploymentTemplate.Generation
	deploymentTemplate.Status.Phrase = radappiov1alpha3.DeploymentTemplatePhraseDeleting
	err := r.Client.Status().Update(ctx, deploymentTemplate)
	if err != nil {
		return ctrl.Result{}, err
	}

	ownedResources, err := r.ownedDeploymentResources(ctx, deploymentTemplate)
	if err != nil {
		return ctrl.Result{}, err
	}

	// If there are still owned DeploymentResources, we need to trigger deletion and wait for them
	// to be deleted before we can delete the DeploymentTemplate.
	if len(ownedResources) > 0 {
		logger.Info("Owned resources still exist, waiting for deletion.")

		// Trigger deletion of owned resources
		for _, resource := range ownedResources {
			if err := r.SourcePolicy.checkTemplate(ctx, deploymentTemplate); err != nil {
				return ctrl.Result{}, err
			}
			err := r.Client.Delete(ctx, &resource)
			if err != nil && !apierrors.IsNotFound(err) {
				logger.Error(err, "Failed to delete owned resource, continuing with other resources", "resourceName", resource.Name)
			}
		}

		return ctrl.Result{}, fmt.Errorf("Requeuing to wait for owned resources to be deleted, %d remaining", len(ownedResources))
	}

	logger.Info("Resource is deleted.")

	// At this point we've cleaned up everything. We can remove the finalizer which will allow
	// deletion of the DeploymentTemplate.
	if err := r.SourcePolicy.checkTemplate(ctx, deploymentTemplate); err != nil {
		return ctrl.Result{}, err
	}
	if controllerutil.RemoveFinalizer(deploymentTemplate, DeploymentTemplateFinalizer) {
		deploymentTemplate.Status.ObservedGeneration = deploymentTemplate.Generation
		deploymentTemplate.Status.Phrase = radappiov1alpha3.DeploymentTemplatePhraseDeleted
		err = r.Client.Update(ctx, deploymentTemplate)
		if err != nil {
			return ctrl.Result{}, err
		}

		r.EventRecorder.Event(deploymentTemplate, corev1.EventTypeNormal, "Reconciled", "Successfully reconciled resource.")
		return ctrl.Result{}, nil
	}

	logger.Info("Finalizer was not removed, requeueing.")
	err = r.Client.Status().Update(ctx, deploymentTemplate)
	if err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *DeploymentTemplateReconciler) startPutOperationIfNeeded(ctx context.Context, deploymentTemplate *radappiov1alpha3.DeploymentTemplate) (sdkclients.Poller[sdkclients.ClientCreateOrUpdateResponse], error) {
	logger := ucplog.FromContextOrDiscard(ctx)
	if err := r.SourcePolicy.checkTemplate(ctx, deploymentTemplate); err != nil {
		return nil, err
	}

	specParameters := convertToARMJSONParameters(deploymentTemplate.Spec.Parameters)

	// If the resource is already created and is up-to-date, then we don't need to do anything.
	if isUpToDate(deploymentTemplate) {
		logger.Info("Resource is up-to-date.")
		return nil, nil
	}

	logger.Info("Desired state has changed, starting PUT operation.")

	var template any
	err := json.Unmarshal([]byte(deploymentTemplate.Spec.Template), &template)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal template: %w", err)
	}

	providerConfig := sdkclients.ProviderConfig{}
	err = json.Unmarshal([]byte(deploymentTemplate.Spec.ProviderConfig), &providerConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal providerConfig: %w", err)
	}
	if providerConfig.Deployments == nil {
		return nil, fmt.Errorf("providerConfig.Deployments is nil")
	}
	if providerConfig.Deployments.Value.Scope == "" {
		return nil, fmt.Errorf("providerConfig.Deployments.Value.Scope is empty")
	}

	// Create the Radius resource group corresponding the providerConfig.Deployments.Value.Scope
	// if it does not exist. This is necessary because the resource group is required for the
	// deployment operation.
	if err := r.SourcePolicy.checkTemplate(ctx, deploymentTemplate); err != nil {
		return nil, err
	}
	err = createResourceGroupIfNotExists(ctx, r.Radius, providerConfig.Deployments.Value.Scope)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource group: %w", err)
	}

	deploymentName := fmt.Sprintf("deploymenttemplate-%v", uuid.New().String())
	resourceID := providerConfig.Deployments.Value.Scope + "/providers/" + "Microsoft.Resources/deployments" + "/" + deploymentName

	logger.Info("Starting PUT operation.")
	if err := r.SourcePolicy.checkTemplate(ctx, deploymentTemplate); err != nil {
		return nil, err
	}
	poller, err := r.ResourceDeploymentsClient.CreateOrUpdate(ctx,
		sdkclients.Deployment{
			Properties: &sdkclients.DeploymentProperties{
				Template:       template,
				Parameters:     specParameters,
				ProviderConfig: providerConfig,
				Mode:           armdeployments.DeploymentModeIncremental,
			},
		},
		resourceID,
		sdkclients.DeploymentsClientAPIVersion,
	)
	if err != nil {
		return nil, err
	} else if poller != nil {
		return poller, nil
	}

	// Update was synchronous
	return nil, nil
}

func (r *DeploymentTemplateReconciler) requeueDelay() time.Duration {
	delay := r.DelayInterval
	if delay == 0 {
		delay = PollingDelay
	}

	return delay
}

func ParseDeploymentScopeFromProviderConfig(providerConfig any) (string, error) {
	var data []byte
	switch v := providerConfig.(type) {
	case string:
		data = []byte(v)
	case []byte:
		data = v
	default:
		return "", fmt.Errorf("providerConfig must be a string or []byte, got %T", providerConfig)
	}

	config := sdkclients.ProviderConfig{}
	err := json.Unmarshal([]byte(data), &config)
	if err != nil {
		return "", fmt.Errorf("failed to unmarshal providerConfig: %w", err)
	}

	if config.Deployments == nil {
		return "", fmt.Errorf("providerConfig.Deployments is nil")
	}

	return config.Deployments.Value.Scope, nil
}

func isOwnedBy(resource radappiov1alpha3.DeploymentResource, owner *radappiov1alpha3.DeploymentTemplate) bool {
	for _, ownerRef := range resource.OwnerReferences {
		if ownerRef.Kind == "DeploymentTemplate" && ownerRef.Name == owner.Name && ownerRef.UID == owner.UID {
			return true
		}
	}
	return false
}

func (r *DeploymentTemplateReconciler) ownedDeploymentResources(ctx context.Context, deploymentTemplate *radappiov1alpha3.DeploymentTemplate) ([]radappiov1alpha3.DeploymentResource, error) {
	deploymentResourceList := &radappiov1alpha3.DeploymentResourceList{}
	reader := client.Reader(r.Client)
	if r.SourcePolicy != nil && r.SourcePolicy.reader != nil {
		reader = r.SourcePolicy.reader
	}
	if err := reader.List(ctx, deploymentResourceList, client.InNamespace(deploymentTemplate.Namespace)); err != nil {
		return nil, err
	}
	owned := make([]radappiov1alpha3.DeploymentResource, 0, len(deploymentResourceList.Items))
	for _, dr := range deploymentResourceList.Items {
		if isOwnedBy(dr, deploymentTemplate) {
			owned = append(owned, dr)
		}
	}
	return owned, nil
}

type deploymentResourceCleanup struct {
	pending         bool
	desiredDeleting bool
	blocksUpdate    bool
}

func (r *DeploymentTemplateReconciler) pendingDeploymentResourceCleanup(ctx context.Context, deploymentTemplate *radappiov1alpha3.DeploymentTemplate, currentOutputResources []string) (deploymentResourceCleanup, error) {
	desired := make(map[string]struct{}, len(currentOutputResources))
	for _, id := range currentOutputResources {
		desired[id] = struct{}{}
	}
	owned, err := r.ownedDeploymentResources(ctx, deploymentTemplate)
	if err != nil {
		return deploymentResourceCleanup{}, err
	}
	var cleanup deploymentResourceCleanup
	for _, dr := range owned {
		_, wanted := desired[dr.Spec.Id]
		deleting := deploymentResourceDeletionPending(&dr)
		if wanted && !deleting {
			continue
		}
		cleanup.pending = true
		cleanup.desiredDeleting = cleanup.desiredDeleting || (wanted && deleting)
		blocked, err := deletionWaitingForLiveDependency(&dr, owned)
		if err != nil {
			return deploymentResourceCleanup{}, err
		}
		// Live children cannot disappear through this template's output diff
		// until a newer deployment completes, so their blocked parents can wait.
		cleanup.blocksUpdate = cleanup.blocksUpdate || !blocked
	}
	return cleanup, nil
}

func (r *DeploymentTemplateReconciler) recreateDependencyBlockedOutput(ctx context.Context, template *radappiov1alpha3.DeploymentTemplate, output *radappiov1alpha3.DeploymentResource) (bool, error) {
	if template.DeletionTimestamp != nil || output.DeletionTimestamp == nil {
		return false, nil
	}
	owned, err := r.ownedDeploymentResources(ctx, template)
	if err != nil {
		return false, err
	}
	blocked, err := deletionWaitingForLiveDependency(output, owned)
	if err != nil || !blocked {
		return false, err
	}
	if err := r.SourcePolicy.checkTemplate(ctx, template); err != nil {
		return false, err
	}
	// The backend DELETE has not started and the completed PUT desires this
	// output again. Replace only its terminating tracker, retaining the backend.
	// Resource-version concurrency with the deleting controller's status write
	// prevents retiring a tracker that has since begun backend deletion.
	if controllerutil.RemoveFinalizer(output, DeploymentResourceFinalizer) {
		if err := r.Client.Update(ctx, output); err != nil {
			return false, err
		}
	}
	return true, nil
}

func deploymentResourceDeletionPending(resource *radappiov1alpha3.DeploymentResource) bool {
	return resource.DeletionTimestamp != nil ||
		(resource.Status.Operation != nil && resource.Status.Operation.OperationKind == radappiov1alpha3.OperationKindDelete)
}

// computeHash computes a hash of the DeploymentTemplate's spec (desired state)
// to save in the status (observed state).
func computeHash(deploymentTemplate *radappiov1alpha3.DeploymentTemplate) (string, error) {
	b, err := templateHashInput(deploymentTemplate)
	if err != nil {
		return "", err
	}

	return hashutil.Hex(b), nil
}

func templateHashInput(deploymentTemplate *radappiov1alpha3.DeploymentTemplate) ([]byte, error) {
	spec := deploymentTemplate.Spec
	// Attribution is checked separately before effects; observing a new artifact alone
	// must not redeploy an otherwise unchanged template.
	spec.SourceRef = nil
	return json.Marshal(spec)
}

// isUpToDate returns true if the desired state of the DeploymentTemplate
// matches the observed state.
//
// It accepts both the current SHA-256 hash and the legacy SHA-1 hash so that
// upgrading Radius (which changes the hash algorithm) does not flag an otherwise
// unchanged DeploymentTemplate as out-of-date and trigger an unnecessary deployment.
func isUpToDate(deploymentTemplate *radappiov1alpha3.DeploymentTemplate) bool {
	b, err := templateHashInput(deploymentTemplate)
	if err != nil {
		return false
	}

	stored := deploymentTemplate.Status.StatusHash
	return stored == hashutil.Hex(b) || stored == hashutil.LegacyHex(b)
}

// updateFailedStatus updates the deployment template status to failed state and clears the operation.
// This helper reduces duplication when handling operation failures.
func (r *DeploymentTemplateReconciler) updateFailedStatus(ctx context.Context, deploymentTemplate *radappiov1alpha3.DeploymentTemplate) error {
	deploymentTemplate.Status.Operation = nil
	deploymentTemplate.Status.OperationSpecHash = ""
	deploymentTemplate.Status.Phrase = radappiov1alpha3.DeploymentTemplatePhraseFailed
	return r.Client.Status().Update(ctx, deploymentTemplate)
}

// SetupWithManager sets up the controller with the Manager.
func (r *DeploymentTemplateReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&radappiov1alpha3.DeploymentTemplate{}).
		Owns(&radappiov1alpha3.DeploymentResource{}).
		Complete(r)
}
