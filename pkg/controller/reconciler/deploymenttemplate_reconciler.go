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
	"slices"
	"strings"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/discovery"
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
	"github.com/radius-project/radius/pkg/ucp/resources"
	resources_kubernetes "github.com/radius-project/radius/pkg/ucp/resources/kubernetes"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
	corev1 "k8s.io/api/core/v1"
)

var errOutputTrackerTerminating = errors.New("deployment output tracker is terminating")

// A completed PUT must not bless outputs which a concurrent cleanup may have deleted.
const outputRedeploymentRequired = "output-redeployment-required"

// DeploymentTemplateReconciler reconciles a DeploymentTemplate object.
type DeploymentTemplateReconciler struct {
	// Discovery confirms API removal when deleting outputs without child trackers.
	Discovery discovery.DiscoveryInterface

	// APIReader reads output identities without waiting for the informer cache.
	APIReader client.Reader

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
		result, err := r.reconcileOperation(ctx, &deploymentTemplate)
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

	if deploymentTemplate.Status.Phrase == radappiov1alpha3.DeploymentTemplatePhraseDeleting && deploymentTemplate.DeletionTimestamp == nil {
		// Namespace cleanup inventory was persisted, but the prior template DELETE may have failed.
		if err := r.Client.Delete(ctx, &deploymentTemplate); client.IgnoreNotFound(err) != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: r.requeueDelay()}, nil
	}
	if deploymentTemplate.ObjectMeta.DeletionTimestamp != nil {
		return r.reconcileDelete(ctx, &deploymentTemplate)
	}

	return r.reconcileUpdate(ctx, &deploymentTemplate)
}

// reconcileOperation reconciles a DeploymentTemplate that has an operation in progress.
func (r *DeploymentTemplateReconciler) reconcileOperation(ctx context.Context, deploymentTemplate *radappiov1alpha3.DeploymentTemplate) (ctrl.Result, error) {
	logger := ucplog.FromContextOrDiscard(ctx)

	if deploymentTemplate.Status.Operation.OperationKind == radappiov1alpha3.OperationKindDelete {
		return r.reconcileInventoryDeleteOperation(ctx, deploymentTemplate)
	}

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

		terminatingOutput := deploymentTemplate.Status.StatusHash == outputRedeploymentRequired
		replacingTracker := false
		for _, outputResourceId := range outputResources {
			resourceName, err := generateDeploymentResourceName(outputResourceId)
			if err != nil {
				return ctrl.Result{}, err
			}

			deploymentResource := &radappiov1alpha3.DeploymentResource{
				Name:       resourceName,
				Namespace:  deploymentTemplate.Namespace,
				Finalizers: []string{DeploymentResourceFinalizer},
				Spec: radappiov1alpha3.DeploymentResourceSpec{
					Id: outputResourceId,
				},
			}

			if err := controllerutil.SetControllerReference(deploymentTemplate, deploymentResource, r.Scheme); err != nil {
				return ctrl.Result{}, err
			}

			// Admission may reject a duplicate POST at quota before returning AlreadyExists.
			existing := &radappiov1alpha3.DeploymentResource{}
			if err := r.APIReader.Get(ctx, client.ObjectKeyFromObject(deploymentResource), existing); apierrors.IsNotFound(err) {
				if err := r.Client.Create(ctx, deploymentResource); err != nil && !apierrors.IsAlreadyExists(err) {
					if apierrors.HasStatusCause(err, corev1.NamespaceTerminatingCause) {
						return r.retainOutputsForNamespaceDeletion(ctx, deploymentTemplate, outputResources)
					}
					return ctrl.Result{}, err
				}
			} else if err != nil {
				return ctrl.Result{}, err
			}
			if err := r.recordOutputIdentity(ctx, deploymentTemplate, outputResourceId, false); err != nil {
				if errors.Is(err, errOutputTrackerTerminating) {
					dependency, err := isApplicationOrEnvironment(outputResourceId)
					if err != nil {
						return ctrl.Result{}, err
					}
					if !dependency {
						if err := r.requireOutputRedeployment(ctx, deploymentTemplate); err != nil {
							return ctrl.Result{}, err
						}
					}
					// Retain every output before discarding the completed deployment result.
					if err := r.recordOutputIdentity(ctx, deploymentTemplate, outputResourceId, true); err != nil {
						return ctrl.Result{}, err
					}
					replacing, err := r.replaceDependencyBlockedTracker(ctx, deploymentTemplate, outputResourceId)
					if err != nil {
						return ctrl.Result{}, err
					}
					if replacing {
						replacingTracker = true
						continue
					}
					terminatingOutput = true
					if err := r.requireOutputRedeployment(ctx, deploymentTemplate); err != nil {
						return ctrl.Result{}, err
					}
					continue
				}
				return ctrl.Result{}, err
			}
		}

		if replacingTracker {
			// Retry retention of this completed result, not a new PUT, once the old tracker is gone.
			return ctrl.Result{RequeueAfter: r.requeueDelay()}, nil
		}

		for _, resource := range deploymentTemplate.Status.OutputResources {
			if _, ok := newOutputResources[resource]; !ok {
				// Resource is present in deploymentTemplate.Status.OutputResources but not in outputResources, delete it

				if err := r.recordOutputIdentity(ctx, deploymentTemplate, resource, true); client.IgnoreNotFound(err) != nil {
					return ctrl.Result{}, err
				}
				logger.Info("Deleting resource.", "resourceId", resource)
				resourceName, err := generateDeploymentResourceName(resource)
				if err != nil {
					return ctrl.Result{}, err
				}

				err = r.Client.Delete(ctx, &radappiov1alpha3.DeploymentResource{
					Name:      resourceName,
					Namespace: deploymentTemplate.Namespace,
				})
				if client.IgnoreNotFound(err) != nil {
					return ctrl.Result{}, err
				}
			}
		}

		// ObservedGeneration is the generation this operation submitted. If the spec changed since,
		// leave the hash empty so the next reconcile deploys the current spec.
		var hash string
		if !terminatingOutput && deploymentTemplate.Generation == deploymentTemplate.Status.ObservedGeneration {
			hash, err = computeHash(deploymentTemplate)
			if err != nil {
				return ctrl.Result{}, err
			}
		}

		// If any owned DR has a Spec.Id not in the new outputResources set, it
		// is a residual from the diff-delete above whose UCP cascade is still
		// in flight. Hold Phrase at ReadyPendingCleanup until those drain so
		// Ready reflects the true steady state.
		residualsPresent, err := r.hasResidualDeploymentResources(ctx, deploymentTemplate, outputResources)
		if err != nil {
			return ctrl.Result{}, err
		}

		// If we get here, the operation was a success. Update the status and continue.
		deploymentTemplate.Status.Operation = nil
		deploymentTemplate.Status.OutputResources = outputResources
		deploymentTemplate.Status.StatusHash = hash
		if residualsPresent || terminatingOutput {
			deploymentTemplate.Status.Phrase = radappiov1alpha3.DeploymentTemplatePhraseReadyPendingCleanup
		} else {
			deploymentTemplate.Status.Phrase = radappiov1alpha3.DeploymentTemplatePhraseReady
		}

		err = r.Client.Status().Update(ctx, deploymentTemplate)
		if err != nil {
			return ctrl.Result{}, err
		}

		if residualsPresent || terminatingOutput {
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

	logger.Info("Reconciling resource.")

	// Ensure that our finalizer is present before we start any operations.
	if controllerutil.AddFinalizer(deploymentTemplate, DeploymentTemplateFinalizer) {
		err := r.Client.Update(ctx, deploymentTemplate)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	// Wait for active cleanup, but let edits remove resources blocking dependency-gated cleanup.
	children := &radappiov1alpha3.DeploymentResourceList{}
	if err := r.APIReader.List(ctx, children, client.InNamespace(deploymentTemplate.Namespace)); err != nil {
		return ctrl.Result{}, err
	}
	ownedChildren := make([]radappiov1alpha3.DeploymentResource, 0, len(children.Items))
	for _, child := range children.Items {
		if isOwnedBy(child, deploymentTemplate) {
			ownedChildren = append(ownedChildren, child)
		}
	}
	for _, child := range ownedChildren {
		if child.DeletionTimestamp != nil {
			dependent, err := checkForDeploymentResourceDependencies(&child, ownedChildren)
			if err != nil {
				return ctrl.Result{}, err
			}
			retryFailed := deploymentTemplate.Status.Phrase == radappiov1alpha3.DeploymentTemplatePhraseFailed
			if dependent != "" && child.Status.Operation == nil && (deploymentTemplate.Generation != deploymentTemplate.Status.ObservedGeneration || retryFailed) {
				continue
			}
			if !retryFailed && deploymentTemplate.Status.Phrase != radappiov1alpha3.DeploymentTemplatePhraseReadyPendingCleanup {
				deploymentTemplate.Status.Phrase = radappiov1alpha3.DeploymentTemplatePhraseReadyPendingCleanup
				if err := r.Client.Status().Update(ctx, deploymentTemplate); err != nil {
					return ctrl.Result{}, err
				}
			}
			return ctrl.Result{RequeueAfter: r.requeueDelay()}, nil
		}
	}

	// Since we're going to reconcile, update the observed generation.
	//
	// We don't want to do this if we're in the middle of an operation, because we haven't
	// fully processed any status changes until the async operation completes.
	deploymentTemplate.Status.ObservedGeneration = deploymentTemplate.Generation

	updatePoller, err := r.startPutOperationIfNeeded(ctx, deploymentTemplate)
	if err != nil {
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
		deploymentTemplate.Status.Phrase = radappiov1alpha3.DeploymentTemplatePhraseUpdating
		err = r.Client.Status().Update(ctx, deploymentTemplate)
		if err != nil {
			return ctrl.Result{}, err
		}

		return ctrl.Result{RequeueAfter: r.requeueDelay()}, nil
	}

	// If we get here then it means we can process the result of the operation.
	logger.Info("Resource is in desired state.")

	// If we're still in ReadyPendingCleanup from a prior reconcile, re-evaluate
	// residuals (woken by the Owns watch) and only transition to Ready once
	// they've drained.
	if deploymentTemplate.Status.Phrase == radappiov1alpha3.DeploymentTemplatePhraseReadyPendingCleanup {
		residualsPresent, err := r.hasResidualDeploymentResources(ctx, deploymentTemplate, deploymentTemplate.Status.OutputResources)
		if err != nil {
			return ctrl.Result{}, err
		}
		if residualsPresent {
			return ctrl.Result{RequeueAfter: r.requeueDelay()}, nil
		}
	}

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

	logger.Info("Resource is being deleted.")

	// Since we're going to reconcile, update the observed generation.
	//
	// We don't want to do this if we're in the middle of an operation, because we haven't
	// fully processed any status changes until the async operation completes.
	if deploymentTemplate.Status.ObservedGeneration != deploymentTemplate.Generation ||
		(deploymentTemplate.Status.Phrase != radappiov1alpha3.DeploymentTemplatePhraseDeleting && deploymentTemplate.Status.Phrase != radappiov1alpha3.DeploymentTemplatePhraseFailed) {
		deploymentTemplate.Status.ObservedGeneration = deploymentTemplate.Generation
		if deploymentTemplate.Status.Phrase != radappiov1alpha3.DeploymentTemplatePhraseFailed {
			deploymentTemplate.Status.Phrase = radappiov1alpha3.DeploymentTemplatePhraseDeleting
		}
		if err := r.Client.Status().Update(ctx, deploymentTemplate); err != nil {
			return ctrl.Result{}, err
		}
	}

	// List all DeploymentResource objects in the same namespace
	deploymentResourceList := &radappiov1alpha3.DeploymentResourceList{}
	err := r.APIReader.List(ctx, deploymentResourceList, client.InNamespace(deploymentTemplate.Namespace))
	if err != nil {
		return ctrl.Result{}, err
	}

	// Filter the list to include only those owned by the current DeploymentTemplate
	var ownedResources []radappiov1alpha3.DeploymentResource
	for _, resource := range deploymentResourceList.Items {
		if isOwnedBy(resource, deploymentTemplate) {
			ownedResources = append(ownedResources, resource)
		}
	}

	namespaceTerminating := false
	if len(deploymentTemplate.Status.OutputResources) > 0 {
		namespace := &corev1.Namespace{}
		if err := r.APIReader.Get(ctx, client.ObjectKey{Name: deploymentTemplate.Namespace}, namespace); err != nil {
			return ctrl.Result{}, err
		}
		namespaceTerminating = namespace.DeletionTimestamp != nil
	}
	if namespaceTerminating {
		for i, outputID := range deploymentTemplate.Status.OutputResources {
			dependency, err := isApplicationOrEnvironment(outputID)
			if err != nil {
				return r.inventoryDeleteFailed(ctx, deploymentTemplate, err)
			}
			if dependency || slices.ContainsFunc(ownedResources, func(child radappiov1alpha3.DeploymentResource) bool { return child.Spec.Id == outputID }) {
				continue
			}
			if i != 0 {
				deploymentTemplate.Status.OutputResources[0], deploymentTemplate.Status.OutputResources[i] = outputID, deploymentTemplate.Status.OutputResources[0]
				if err := r.Client.Status().Update(ctx, deploymentTemplate); err != nil {
					return ctrl.Result{}, err
				}
			}
			return r.deleteUntrackedOutput(ctx, deploymentTemplate)
		}
	}

	// If there are still owned DeploymentResources, we need to trigger deletion and wait for them
	// to be deleted before we can delete the DeploymentTemplate.
	if len(ownedResources) > 0 {
		logger.Info("Owned resources still exist, waiting for deletion.")

		// Trigger deletion of owned resources
		for _, resource := range ownedResources {
			err := r.Client.Delete(ctx, &resource)
			if err != nil && !apierrors.IsNotFound(err) {
				logger.Error(err, "Failed to delete owned resource, continuing with other resources", "resourceName", resource.Name)
			}
		}

		return ctrl.Result{}, fmt.Errorf("Requeuing to wait for owned resources to be deleted, %d remaining", len(ownedResources))
	}

	if namespaceTerminating && len(deploymentTemplate.Status.OutputResources) > 0 {
		return r.deleteUntrackedOutput(ctx, deploymentTemplate)
	}

	logger.Info("Resource is deleted.")

	// At this point we've cleaned up everything. We can remove the finalizer which will allow
	// deletion of the DeploymentTemplate.
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
	err = createResourceGroupIfNotExists(ctx, r.Radius, providerConfig.Deployments.Value.Scope)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource group: %w", err)
	}

	deploymentName := fmt.Sprintf("deploymenttemplate-%v", uuid.New().String())
	resourceID := providerConfig.Deployments.Value.Scope + "/providers/" + "Microsoft.Resources/deployments" + "/" + deploymentName

	logger.Info("Starting PUT operation.")
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

func (r *DeploymentTemplateReconciler) recordOutputIdentity(ctx context.Context, owner *radappiov1alpha3.DeploymentTemplate, outputID string, allowTerminating bool) error {
	name, err := generateDeploymentResourceName(outputID)
	if err != nil {
		return err
	}
	child := &radappiov1alpha3.DeploymentResource{}
	if err := r.APIReader.Get(ctx, client.ObjectKey{Namespace: owner.Namespace, Name: name}, child); err != nil {
		return err
	}
	if !isOwnedBy(*child, owner) || child.Spec.Id != outputID {
		return fmt.Errorf("deployment output %q does not match its owned DeploymentResource", outputID)
	}
	if !allowTerminating && child.DeletionTimestamp != nil {
		return errOutputTrackerTerminating
	}
	if !allowTerminating && !controllerutil.ContainsFinalizer(child, DeploymentResourceFinalizer) {
		return fmt.Errorf("deployment output %q has no cleanup finalizer", outputID)
	}
	if child.Status.ProvisionedResourceID != "" {
		if child.Status.ProvisionedResourceID != outputID {
			return fmt.Errorf("deployment output %q conflicts with recorded identity", outputID)
		}
		return nil
	}
	base := child.DeepCopy()
	child.Status.ProvisionedResourceID = outputID
	if err := r.Client.Status().Patch(ctx, child, client.MergeFromWithOptions(base, client.MergeFromWithOptimisticLock{})); err != nil {
		return err
	}
	child = &radappiov1alpha3.DeploymentResource{}
	if err := r.APIReader.Get(ctx, client.ObjectKey{Namespace: owner.Namespace, Name: name}, child); err != nil {
		return err
	}
	if child.Status.ProvisionedResourceID != outputID {
		return fmt.Errorf("DeploymentResource schema pruned the recorded output identity; retry after schema migration")
	}
	return nil
}

func (r *DeploymentTemplateReconciler) requireOutputRedeployment(ctx context.Context, owner *radappiov1alpha3.DeploymentTemplate) error {
	if owner.Status.StatusHash == outputRedeploymentRequired {
		return nil
	}
	owner.Status.StatusHash = outputRedeploymentRequired
	return r.Client.Status().Update(ctx, owner)
}

func (r *DeploymentTemplateReconciler) replaceDependencyBlockedTracker(ctx context.Context, owner *radappiov1alpha3.DeploymentTemplate, outputID string) (bool, error) {
	children := &radappiov1alpha3.DeploymentResourceList{}
	if err := r.APIReader.List(ctx, children, client.InNamespace(owner.Namespace)); err != nil {
		return false, err
	}
	owned := make([]radappiov1alpha3.DeploymentResource, 0, len(children.Items))
	for _, child := range children.Items {
		if isOwnedBy(child, owner) {
			owned = append(owned, child)
		}
	}
	for i := range owned {
		child := &owned[i]
		if child.Spec.Id != outputID {
			continue
		}
		dependent, err := checkForDeploymentResourceDependencies(child, owned)
		if err != nil {
			return false, err
		}
		if dependent == "" || child.Status.Operation != nil || child.DeletionTimestamp == nil {
			return false, nil
		}
		// Deletion is irreversible, but this dependency-gated tracker has not deleted its output.
		// Retire only the tracker and retain the completed PUT until a replacement is persisted.
		base := child.DeepCopy()
		if controllerutil.RemoveFinalizer(child, DeploymentResourceFinalizer) {
			if err := r.Client.Patch(ctx, child, client.MergeFromWithOptions(base, client.MergeFromWithOptimisticLock{})); err != nil {
				return false, err
			}
		}
		return true, nil
	}
	return false, apierrors.NewNotFound(radappiov1alpha3.GroupVersion.WithResource("deploymentresources").GroupResource(), outputID)
}

func (r *DeploymentTemplateReconciler) retainOutputsForNamespaceDeletion(ctx context.Context, owner *radappiov1alpha3.DeploymentTemplate, outputs []string) (ctrl.Result, error) {
	for _, id := range outputs {
		if !slices.Contains(owner.Status.OutputResources, id) {
			owner.Status.OutputResources = append(owner.Status.OutputResources, id)
		}
	}
	owner.Status.Operation = nil
	owner.Status.StatusHash = ""
	owner.Status.Phrase = radappiov1alpha3.DeploymentTemplatePhraseDeleting
	if err := r.Client.Status().Update(ctx, owner); err != nil {
		return ctrl.Result{}, err
	}
	if owner.DeletionTimestamp == nil {
		if err := r.Client.Delete(ctx, owner); client.IgnoreNotFound(err) != nil {
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{RequeueAfter: r.requeueDelay()}, nil
}

// deleteUntrackedOutput drains protected inventory when NamespaceLifecycle prevents child creation.
// The first inventory entry owns Status.Operation until its DELETE has completed.
func (r *DeploymentTemplateReconciler) deleteUntrackedOutput(ctx context.Context, owner *radappiov1alpha3.DeploymentTemplate) (ctrl.Result, error) {
	// Delete leaf resources before applications/environments, just as child cleanup does.
	index := 0
	for i, outputID := range owner.Status.OutputResources {
		id, err := resources.ParseResource(outputID)
		if err != nil {
			return r.inventoryDeleteFailed(ctx, owner, err)
		}
		if !strings.EqualFold(id.Type(), "Applications.Core/applications") && !strings.EqualFold(id.Type(), "Applications.Core/environments") {
			index = i
			break
		}
	}
	if index != 0 {
		owner.Status.OutputResources[0], owner.Status.OutputResources[index] = owner.Status.OutputResources[index], owner.Status.OutputResources[0]
		if err := r.Client.Status().Update(ctx, owner); err != nil {
			return ctrl.Result{}, err
		}
	}
	outputID := owner.Status.OutputResources[0]
	id, err := resources.ParseResource(outputID)
	if err != nil {
		return r.inventoryDeleteFailed(ctx, owner, err)
	}
	if plane := id.FindScope(resources_kubernetes.PlaneTypeKubernetes); plane != "" {
		if !strings.EqualFold(plane, resources_kubernetes.PlaneNameTODO) {
			return r.skipInventoryOutput(ctx, owner, outputID)
		}
		deleter := &DeploymentResourceReconciler{Client: r.Client, Discovery: r.Discovery}
		done, err := deleter.deleteKubernetesOutput(ctx, owner.Namespace, id)
		if err != nil {
			return r.inventoryDeleteFailed(ctx, owner, err)
		}
		if !done {
			return ctrl.Result{RequeueAfter: r.requeueDelay()}, nil
		}
		return r.completeInventoryOutput(ctx, owner)
	}
	scope, err := ParseDeploymentScopeFromProviderConfig(owner.Spec.ProviderConfig)
	if err != nil {
		return r.inventoryDeleteFailed(ctx, owner, err)
	}
	within, err := resourceWithinScope(outputID, scope)
	if err != nil {
		return r.inventoryDeleteFailed(ctx, owner, err)
	}
	if !within {
		return r.skipInventoryOutput(ctx, owner, outputID)
	}
	poller, err := r.ResourceDeploymentsClient.Delete(ctx, outputID, "2023-10-01-preview")
	if err != nil {
		if clients.Is404Error(err) {
			return r.completeInventoryOutput(ctx, owner)
		}
		return r.inventoryDeleteFailed(ctx, owner, err)
	}
	if poller == nil {
		return r.completeInventoryOutput(ctx, owner)
	}
	if poller.Done() {
		if _, err := poller.Result(ctx); err != nil && !clients.Is404Error(err) {
			return r.inventoryDeleteFailed(ctx, owner, err)
		}
		return r.completeInventoryOutput(ctx, owner)
	}
	token, err := poller.ResumeToken()
	if err != nil {
		return r.inventoryDeleteFailed(ctx, owner, err)
	}
	owner.Status.Operation = &radappiov1alpha3.ResourceOperation{ResumeToken: token, OperationKind: radappiov1alpha3.OperationKindDelete}
	owner.Status.Phrase = radappiov1alpha3.DeploymentTemplatePhraseDeleting
	if err := r.Client.Status().Update(ctx, owner); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: r.requeueDelay()}, nil
}

func (r *DeploymentTemplateReconciler) reconcileInventoryDeleteOperation(ctx context.Context, owner *radappiov1alpha3.DeploymentTemplate) (ctrl.Result, error) {
	if owner.DeletionTimestamp == nil || len(owner.Status.OutputResources) == 0 {
		return r.inventoryDeleteFailed(ctx, owner, fmt.Errorf("inventory DELETE requires a terminating template with a recorded output"))
	}
	poller, err := r.ResourceDeploymentsClient.ContinueDeleteOperation(ctx, owner.Status.Operation.ResumeToken)
	if err != nil {
		return r.inventoryDeleteFailed(ctx, owner, err)
	}
	if _, err := poller.Poll(ctx); err != nil {
		return r.inventoryDeleteFailed(ctx, owner, err)
	}
	if !poller.Done() {
		return ctrl.Result{RequeueAfter: r.requeueDelay()}, nil
	}
	if _, err := poller.Result(ctx); err != nil && !clients.Is404Error(err) {
		return r.inventoryDeleteFailed(ctx, owner, err)
	}
	return r.completeInventoryOutput(ctx, owner)
}

func (r *DeploymentTemplateReconciler) completeInventoryOutput(ctx context.Context, owner *radappiov1alpha3.DeploymentTemplate) (ctrl.Result, error) {
	owner.Status.OutputResources = owner.Status.OutputResources[1:]
	owner.Status.Operation = nil
	if err := r.Client.Status().Update(ctx, owner); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: r.requeueDelay()}, nil
}

func (r *DeploymentTemplateReconciler) skipInventoryOutput(ctx context.Context, owner *radappiov1alpha3.DeploymentTemplate, outputID string) (ctrl.Result, error) {
	r.EventRecorder.Event(owner, corev1.EventTypeNormal, EventDeploymentResourceDeleteSkipped,
		fmt.Sprintf("Skipping delete of %q: output is outside the template's deployment scope or local Kubernetes cluster", outputID))
	return r.completeInventoryOutput(ctx, owner)
}

func (r *DeploymentTemplateReconciler) inventoryDeleteFailed(ctx context.Context, owner *radappiov1alpha3.DeploymentTemplate, err error) (ctrl.Result, error) {
	ucplog.FromContextOrDiscard(ctx).Error(err, "Unable to delete recorded template output.")
	r.EventRecorder.Event(owner, corev1.EventTypeWarning, "ResourceError", err.Error())
	if statusErr := r.updateFailedStatus(ctx, owner); statusErr != nil {
		return ctrl.Result{}, statusErr
	}
	return ctrl.Result{RequeueAfter: DeleteRetryDelay}, nil
}

func isOwnedBy(resource radappiov1alpha3.DeploymentResource, owner *radappiov1alpha3.DeploymentTemplate) bool {
	ref := metav1.GetControllerOf(&resource)
	return ref != nil && ref.APIVersion == radappiov1alpha3.GroupVersion.String() &&
		ref.Kind == deploymentTemplateKind && ref.Name == owner.Name && ref.UID == owner.UID
}

// hasResidualDeploymentResources reports whether any DeploymentResource owned
// by this DeploymentTemplate has a Spec.Id that is not in currentOutputResources
// (i.e., a residual from a prior diff whose deletion is still draining).
func (r *DeploymentTemplateReconciler) hasResidualDeploymentResources(ctx context.Context, deploymentTemplate *radappiov1alpha3.DeploymentTemplate, currentOutputResources []string) (bool, error) {
	desired := make(map[string]struct{}, len(currentOutputResources))
	for _, id := range currentOutputResources {
		desired[id] = struct{}{}
	}

	deploymentResourceList := &radappiov1alpha3.DeploymentResourceList{}
	if err := r.Client.List(ctx, deploymentResourceList, client.InNamespace(deploymentTemplate.Namespace)); err != nil {
		return false, err
	}

	for _, dr := range deploymentResourceList.Items {
		if !isOwnedBy(dr, deploymentTemplate) {
			continue
		}
		if _, ok := desired[dr.Spec.Id]; !ok {
			return true, nil
		}
	}
	return false, nil
}

// computeHash computes a hash of the DeploymentTemplate's spec (desired state)
// to save in the status (observed state).
func computeHash(deploymentTemplate *radappiov1alpha3.DeploymentTemplate) (string, error) {
	b, err := json.Marshal(deploymentTemplate.Spec)
	if err != nil {
		return "", err
	}

	return hashutil.Hex(b), nil
}

// isUpToDate returns true if the desired state of the DeploymentTemplate
// matches the observed state.
//
// It accepts both the current SHA-256 hash and the legacy SHA-1 hash so that
// upgrading Radius (which changes the hash algorithm) does not flag an otherwise
// unchanged DeploymentTemplate as out-of-date and trigger an unnecessary deployment.
func isUpToDate(deploymentTemplate *radappiov1alpha3.DeploymentTemplate) bool {
	b, err := json.Marshal(deploymentTemplate.Spec)
	if err != nil {
		return false
	}

	stored := deploymentTemplate.Status.StatusHash
	return stored == hashutil.Hex(b) || stored == hashutil.LegacyHex(b)
}

// updateFailedStatus updates the deployment template status to failed state and clears the operation.
// This helper reduces duplication when handling operation failures.
func (r *DeploymentTemplateReconciler) updateFailedStatus(ctx context.Context, deploymentTemplate *radappiov1alpha3.DeploymentTemplate) error {
	if deploymentTemplate.Status.Operation == nil && deploymentTemplate.Status.Phrase == radappiov1alpha3.DeploymentTemplatePhraseFailed {
		return nil
	}
	deploymentTemplate.Status.Operation = nil
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
