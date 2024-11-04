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
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/go-logr/logr"
	"github.com/radius-project/radius/pkg/cli/clients_new/generated"
	radappiov1alpha3 "github.com/radius-project/radius/pkg/controller/api/radapp.io/v1alpha3"
	sdkclients "github.com/radius-project/radius/pkg/sdk/clients"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
	corev1 "k8s.io/api/core/v1"
)

const (
	deploymentResourceType = "Microsoft.Resources/deployments"
)

// DeploymentTemplateReconciler reconciles a DeploymentTemplate object.
type DeploymentTemplateReconciler struct {
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
	// 	3. If the operation failed, then update the `status.phrase` and `status.message` as `Failed` with the reason for the failure and continue processing.
	// 2. If the `DeploymentTemplate` is being deleted, then process deletion:
	// 	1. Remove the `radapp.io/deployment-template-finalizer` finalizer from the `DeploymentTemplate`.
	// 	1. Since the `DeploymentResources` are owned by the `DeploymentTemplate`, the `DeploymentResource` resources will be deleted first. Once they are deleted, the `DeploymentTemplate` resource will be deleted.
	// 4. If the `DeploymentTemplate` is not being deleted then process this as a create or update:
	// 	1. Add the `radapp.io/deployment-template-finalizer` finalizer onto the `DeploymentTemplate` resource.
	// 	2. Queue a PUT operation against the Radius API to deploy the ARM JSON in the `spec.template` field with the parameters in the `spec.parameters` field.
	// 	3. Set the `status.phrase` for the `DeploymentTemplate` to `Updating` and the `status.operation` to the operation returned by the Radius API.
	// 	4. Continue processing.
	//
	// We do it this way because it guarantees that we only have one operation going at a time.

	if deploymentTemplate.Status.Operation != nil {
		result, err := r.reconcileOperation(ctx, &deploymentTemplate)
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

	if deploymentTemplate.DeletionTimestamp != nil {
		return r.reconcileDelete(ctx, &deploymentTemplate)
	}

	return r.reconcileUpdate(ctx, &deploymentTemplate)
}

// reconcileOperation reconciles a DeploymentTemplate that has an operation in progress.
func (r *DeploymentTemplateReconciler) reconcileOperation(ctx context.Context, deploymentTemplate *radappiov1alpha3.DeploymentTemplate) (ctrl.Result, error) {
	logger := ucplog.FromContextOrDiscard(ctx)

	if deploymentTemplate.Status.Operation.OperationKind == radappiov1alpha3.OperationKindPut {
		scope, err := ParseDeploymentScopeFromProviderConfig(deploymentTemplate.Spec.ProviderConfig)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to parse deployment scope: %w", err)
		}

		poller, err := r.Radius.Resources(scope, deploymentResourceType).ContinueCreateOperation(ctx, deploymentTemplate.Status.Operation.ResumeToken)
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
		resp, err := poller.Result(ctx)
		if err != nil {
			// Operation failed, reset state and retry.
			r.EventRecorder.Event(deploymentTemplate, corev1.EventTypeWarning, "ResourceError", err.Error())
			logger.Error(err, "Update failed.")

			deploymentTemplate.Status.Operation = nil
			deploymentTemplate.Status.Phrase = radappiov1alpha3.DeploymentTemplatePhraseFailed
			err = r.Client.Status().Update(ctx, deploymentTemplate)
			if err != nil {
				return ctrl.Result{}, err
			}

			return ctrl.Result{Requeue: true, RequeueAfter: r.requeueDelay()}, nil
		}

		logger.Info("Creating output resources.")

		// Get outputResources from the response
		outputResources := make([]string, 0)
		outputResourceList := resp.Properties["outputResources"].([]any)
		for _, resource := range outputResourceList {
			outputResource := resource.(map[string]any)
			outputResources = append(outputResources, outputResource["id"].(string))
		}

		// Compare outputResources with existing DeploymentResources
		// if is present in deploymentTemplate.Status.OutputResources but not in outputResources, delete it
		// if is not present in deploymentTemplate.Status.OutputResources but is in outputResources, create it
		// if is present in both, do nothing

		existingOutputResources := make(map[string]bool)
		for _, resource := range deploymentTemplate.Status.OutputResources {
			existingOutputResources[resource] = true
		}

		newOutputResources := make(map[string]bool)
		for _, resource := range outputResources {
			newOutputResources[resource] = true
		}

		for _, outputResourceId := range outputResources {
			if _, ok := existingOutputResources[outputResourceId]; !ok {
				// Resource is not present in deploymentTemplate.Status.OutputResources but is in outputResources, create it

				resourceName := generateDeploymentResourceName(outputResourceId)
				deploymentResource := &radappiov1alpha3.DeploymentResource{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: deploymentTemplate.Namespace,
					},
					Spec: radappiov1alpha3.DeploymentResourceSpec{
						Id:             outputResourceId,
						ProviderConfig: deploymentTemplate.Spec.ProviderConfig,
						Repository:     deploymentTemplate.Spec.Repository,
					},
				}

				if controllerutil.AddFinalizer(deploymentResource, DeploymentResourceFinalizer) {
					// Add the DeploymentTemplate as the owner of the DeploymentResource
					if err := controllerutil.SetControllerReference(deploymentTemplate, deploymentResource, r.Scheme); err != nil {
						return ctrl.Result{}, err
					}

					// Create the DeploymentResource
					err = r.Client.Create(ctx, deploymentResource)
					if err != nil {
						return ctrl.Result{}, err
					}
				}
			}
		}

		for _, resource := range deploymentTemplate.Status.OutputResources {
			if _, ok := newOutputResources[resource]; !ok {
				// Resource is present in deploymentTemplate.Status.OutputResources but not in outputResources, delete it
				logger.Info("Deleting resource.", "resourceId", resource)
				resourceName := generateDeploymentResourceName(resource)
				err := r.Client.Delete(ctx, &radappiov1alpha3.DeploymentResource{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: deploymentTemplate.Namespace,
					},
				})
				if err != nil {
					return ctrl.Result{}, err
				}
			}
		}

		providerConfig := sdkclients.ProviderConfig{}
		err = json.Unmarshal([]byte(deploymentTemplate.Spec.ProviderConfig), &providerConfig)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to unmarshal template: %w", err)
		}

		// If we get here, the operation was a success. Update the status and continue.
		//
		// NOTE: we don't need to save the status here, because we're going to continue reconciling.
		deploymentTemplate.Status.Operation = nil
		deploymentTemplate.Status.OutputResources = outputResources
		deploymentTemplate.Status.Template = deploymentTemplate.Spec.Template
		deploymentTemplate.Status.Parameters = deploymentTemplate.Spec.Parameters
		deploymentTemplate.Status.Resource = providerConfig.Deployments.Value.Scope + "/providers/" + deploymentResourceType + "/" + deploymentTemplate.Name
		deploymentTemplate.Status.ProviderConfig = deploymentTemplate.Spec.ProviderConfig
		deploymentTemplate.Status.Repository = deploymentTemplate.Spec.Repository

		return ctrl.Result{}, nil
	}

	// If we get here, this was an unknown operation kind. This is a bug in our code, or someone
	// tampered with the status of the object. Just reset the state and move on.
	logger.Error(fmt.Errorf("unknown operation kind: %s", deploymentTemplate.Status.Operation.OperationKind), "Unknown operation kind.")

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

	// Since we're going to reconcile, update the observed generation.
	//
	// We don't want to do this if we're in the middle of an operation, because we haven't
	// fully processed any status changes until the async operation completes.
	deploymentTemplate.Status.ObservedGeneration = deploymentTemplate.Generation

	updatePoller, err := r.startPutOperationIfNeeded(ctx, deploymentTemplate)
	if err != nil {
		logger.Error(err, "Unable to create or update resource.")
		r.EventRecorder.Event(deploymentTemplate, corev1.EventTypeWarning, "ResourceError", err.Error())
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

		return ctrl.Result{Requeue: true, RequeueAfter: r.requeueDelay()}, nil
	}

	// If we get here then it means we can process the result of the operation.
	logger.Info("Resource is in desired state.", "resourceId", deploymentTemplate.Status.Resource)

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

	logger.Info("Resource is being deleted.", "resourceId", deploymentTemplate.Status.Resource)

	// Since we're going to reconcile, update the observed generation.
	//
	// We don't want to do this if we're in the middle of an operation, because we haven't
	// fully processed any status changes until the async operation completes.
	deploymentTemplate.Status.ObservedGeneration = deploymentTemplate.Generation
	deploymentTemplate.Status.Operation = nil
	deploymentTemplate.Status.Phrase = radappiov1alpha3.DeploymentTemplatePhraseDeleting

	// List all DeploymentResource objects in the same namespace
	deploymentResourceList := &radappiov1alpha3.DeploymentResourceList{}
	err := r.Client.List(ctx, deploymentResourceList, client.InNamespace(deploymentTemplate.Namespace))
	if err != nil {
		return ctrl.Result{}, nil
	}

	// Filter the list to include only those owned by the current DeploymentTemplate
	var ownedResources []radappiov1alpha3.DeploymentResource
	for _, resource := range deploymentResourceList.Items {
		if isOwnedBy(resource, deploymentTemplate) {
			ownedResources = append(ownedResources, resource)
		}
	}

	// If there are still owned DeploymentResources, we need to trigger deletion and wait for them
	// to be deleted before we can delete the DeploymentTemplate.
	if len(ownedResources) > 0 {
		logger.Info("Owned resources still exist, waiting for deletion.")

		// Trigger deletion of owned resources
		for _, resource := range ownedResources {
			err := r.Client.Delete(ctx, &resource)
			if err != nil {
				return ctrl.Result{}, err
			}
		}

		err = r.Client.Status().Update(ctx, deploymentTemplate)
		if err != nil {
			return ctrl.Result{}, err
		}

		return ctrl.Result{Requeue: true, RequeueAfter: r.requeueDelay()}, nil
	}

	logger.Info("Resource is deleted.")

	// At this point we've cleaned up everything. We can remove the finalizer which will allow
	// deletion of the DeploymentTemplate
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

	// If we get here, then we're in a bad state. We should have removed the finalizer, but we didn't.
	// We should requeue and try again.

	return ctrl.Result{Requeue: true, RequeueAfter: r.requeueDelay()}, nil
}

func (r *DeploymentTemplateReconciler) startPutOperationIfNeeded(ctx context.Context, deploymentTemplate *radappiov1alpha3.DeploymentTemplate) (Poller[generated.GenericResourcesClientCreateOrUpdateResponse], error) {
	logger := ucplog.FromContextOrDiscard(ctx)

	// If the resource is already created and is up-to-date, then we don't need to do anything.
	if deploymentTemplate.Status.Template == deploymentTemplate.Spec.Template &&
		deploymentTemplate.Status.Parameters == deploymentTemplate.Spec.Parameters &&
		deploymentTemplate.Status.Repository == deploymentTemplate.Spec.Repository &&
		deploymentTemplate.Status.ProviderConfig == deploymentTemplate.Spec.ProviderConfig {
		logger.Info("Resource is already created and is up-to-date.")
		return nil, nil
	}

	logger.Info("Template, parameters, repository, or providerConfig have changed, starting PUT operation.")

	var template any
	err := json.Unmarshal([]byte(deploymentTemplate.Spec.Template), &template)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal template: %w", err)
	}

	var parameters any
	err = json.Unmarshal([]byte(deploymentTemplate.Spec.Parameters), &parameters)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal parameters: %w", err)
	}

	providerConfig := sdkclients.ProviderConfig{}
	err = json.Unmarshal([]byte(deploymentTemplate.Spec.ProviderConfig), &providerConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal template: %w", err)
	}
	if providerConfig.Deployments == nil {
		return nil, fmt.Errorf("providerConfig.Deployments is nil")
	}
	if providerConfig.Deployments.Value.Scope == "" {
		return nil, fmt.Errorf("providerConfig.Deployments.Value.Scope is empty")
	}

	// NOTE: using resource groups with lowercase here is a workaround for a casing bug in `rad app graph`.
	// When https://github.com/radius-project/radius/issues/6422 is fixed we can use the more correct casing.
	resourceGroupID := "/planes/radius/local/resourcegroups/default"
	err = createResourceGroupIfNotExists(ctx, r.Radius, resourceGroupID)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource group: %w", err)
	}

	logger.Info("Starting PUT operation.")
	properties := map[string]any{
		"mode":           "Incremental",
		"providerConfig": providerConfig,
		"template":       template,
		"parameters":     parameters,
	}

	resourceID := providerConfig.Deployments.Value.Scope + "/providers/" + deploymentResourceType + "/" + deploymentTemplate.Name
	poller, err := createOrUpdateResource(ctx, r.Radius, resourceID, properties)
	if err != nil {
		return nil, err
	} else if poller != nil {
		return poller, nil
	}

	// Update was synchronous
	deploymentTemplate.Status.Resource = resourceID
	err = r.Client.Status().Update(ctx, deploymentTemplate)
	if err != nil {
		return nil, err
	}

	return nil, nil
}

func (r *DeploymentTemplateReconciler) requeueDelay() time.Duration {
	delay := r.DelayInterval
	if delay == 0 {
		delay = PollingDelay
	}

	return delay
}

func ParseDeploymentScopeFromProviderConfig(providerConfig string) (string, error) {
	config := sdkclients.ProviderConfig{}
	err := json.Unmarshal([]byte(providerConfig), &config)
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
		if ownerRef.Kind == "DeploymentTemplate" && ownerRef.Name == owner.Name {
			return true
		}
	}
	return false
}

// SetupWithManager sets up the controller with the Manager.
func (r *DeploymentTemplateReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&radappiov1alpha3.DeploymentTemplate{}).
		Owns(&radappiov1alpha3.DeploymentResource{}).
		Complete(r)
}
