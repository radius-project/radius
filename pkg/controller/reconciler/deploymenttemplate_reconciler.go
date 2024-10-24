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
	// TODOWILLSMITH: put algorithm here
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
		scope, err := parseDeploymentScopeFromProviderConfig(deploymentTemplate.Spec.ProviderConfig)
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
			deploymentTemplate.Status.Message = err.Error()

			err = r.Client.Status().Update(ctx, deploymentTemplate)
			if err != nil {
				return ctrl.Result{}, err
			}

			return ctrl.Result{Requeue: true, RequeueAfter: r.requeueDelay()}, nil
		}

		logger.Info("Creating output resources.")

		//TODOWILLSMITH: clean this up
		outputResources := make([]string, 0)
		outputResourceList := resp.Properties["outputResources"].([]any)
		for _, resource := range outputResourceList {
			resource2 := resource.(map[string]any)
			outputResources = append(outputResources, resource2["id"].(string))
		}

		// compare outputResources with existing DeploymentResources
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
				// resource is not present in deploymentTemplate.Status.OutputResources but is in outputResources, create it

				resourceName := generateDeploymentResourceName(outputResourceId)
				deploymentResource := &radappiov1alpha3.DeploymentResource{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: deploymentTemplate.Namespace,
					},
					Spec: radappiov1alpha3.DeploymentResourceSpec{
						ID: outputResourceId,
					},
				}

				if controllerutil.AddFinalizer(deploymentResource, DeploymentResourceFinalizer) {
					if err := controllerutil.SetControllerReference(deploymentTemplate, deploymentResource, r.Scheme); err != nil {
						return ctrl.Result{}, err
					}

					err = r.Client.Create(ctx, deploymentResource)
					if err != nil {
						return ctrl.Result{}, err
					}
				}
			}
		}

		for _, resource := range deploymentTemplate.Status.OutputResources {
			if _, ok := newOutputResources[resource]; !ok {
				// resource is present in deploymentTemplate.Status.OutputResources but not in outputResources, delete it
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
		return ctrl.Result{}, nil

	} else if deploymentTemplate.Status.Operation.OperationKind == radappiov1alpha3.OperationKindDelete {
		providerConfig := sdkclients.ProviderConfig{}
		err := json.Unmarshal([]byte(deploymentTemplate.Spec.ProviderConfig), &providerConfig)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to unmarshal template: %w", err)
		}

		poller, err := r.Radius.Resources(providerConfig.Deployments.Value.Scope, deploymentResourceType).ContinueDeleteOperation(ctx, deploymentTemplate.Status.Operation.ResumeToken)
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
			r.EventRecorder.Event(deploymentTemplate, corev1.EventTypeWarning, "ResourceError", err.Error())
			logger.Error(err, "Delete failed.")

			deploymentTemplate.Status.Operation = nil
			deploymentTemplate.Status.Phrase = radappiov1alpha3.DeploymentTemplatePhraseFailed
			deploymentTemplate.Status.Message = err.Error()

			err = r.Client.Status().Update(ctx, deploymentTemplate)
			if err != nil {
				return ctrl.Result{}, err
			}

			return ctrl.Result{Requeue: true, RequeueAfter: r.requeueDelay()}, nil
		}

		// If we get here, the operation was a success. Update the status and continue.
		//
		// NOTE: we don't need to save the status here, because we're going to continue reconciling.
		deploymentTemplate.Status.Operation = nil
		deploymentTemplate.Status.Resource = ""
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

	updatePoller, deletePoller, err := r.startPutOrDeleteOperationIfNeeded(ctx, deploymentTemplate)
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
	} else if deletePoller != nil {
		// We've successfully started an operation. Update the status and requeue.
		token, err := deletePoller.ResumeToken()
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to get operation token: %w", err)
		}

		deploymentTemplate.Status.Operation = &radappiov1alpha3.ResourceOperation{ResumeToken: token, OperationKind: radappiov1alpha3.OperationKindDelete}
		deploymentTemplate.Status.Phrase = radappiov1alpha3.DeploymentTemplatePhraseDeleting
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

	// Since we're going to reconcile, update the observed generation.
	//
	// We don't want to do this if we're in the middle of an operation, because we haven't
	// fully processed any status changes until the async operation completes.
	deploymentTemplate.Status.ObservedGeneration = deploymentTemplate.Generation

	poller, err := r.startDeleteOperationIfNeeded(ctx, deploymentTemplate)
	if err != nil {
		logger.Error(err, "Unable to delete resource.")
		r.EventRecorder.Event(deploymentTemplate, corev1.EventTypeWarning, "ResourceError", err.Error())
		return ctrl.Result{}, err
	} else if poller != nil {
		// We've successfully started an operation. Update the status and requeue.
		token, err := poller.ResumeToken()
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to get operation token: %w", err)
		}

		providerConfig := sdkclients.ProviderConfig{}
		err = json.Unmarshal([]byte(deploymentTemplate.Spec.ProviderConfig), &providerConfig)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to unmarshal template: %w", err)
		}

		deploymentTemplate.Status.Operation = &radappiov1alpha3.ResourceOperation{ResumeToken: token, OperationKind: radappiov1alpha3.OperationKindDelete}
		deploymentTemplate.Status.Phrase = radappiov1alpha3.DeploymentTemplatePhraseDeleting
		deploymentTemplate.Status.ProviderConfig = deploymentTemplate.Spec.ProviderConfig
		err = r.Client.Status().Update(ctx, deploymentTemplate)
		if err != nil {
			return ctrl.Result{}, err
		}

		return ctrl.Result{Requeue: true, RequeueAfter: r.requeueDelay()}, nil
	}

	logger.Info("Resource is deleted.")

	// At this point we've cleaned up everything. We can remove the finalizer which will allow deletion of the
	// DeploymentTemplate
	if controllerutil.RemoveFinalizer(deploymentTemplate, DeploymentTemplateFinalizer) {
		err := r.Client.Update(ctx, deploymentTemplate)
		if err != nil {
			return ctrl.Result{}, err
		}

		deploymentTemplate.Status.ObservedGeneration = deploymentTemplate.Generation
	}

	deploymentTemplate.Status.Phrase = radappiov1alpha3.DeploymentTemplatePhraseDeleted
	err = r.Client.Status().Update(ctx, deploymentTemplate)
	if err != nil {
		return ctrl.Result{}, err
	}

	r.EventRecorder.Event(deploymentTemplate, corev1.EventTypeNormal, "Reconciled", "Successfully reconciled resource.")
	return ctrl.Result{}, nil
}

func (r *DeploymentTemplateReconciler) startPutOrDeleteOperationIfNeeded(ctx context.Context, deploymentTemplate *radappiov1alpha3.DeploymentTemplate) (Poller[generated.GenericResourcesClientCreateOrUpdateResponse], Poller[generated.GenericResourcesClientDeleteResponse], error) {
	logger := ucplog.FromContextOrDiscard(ctx)

	// If the resource is already created and is up-to-date, then we don't need to do anything.
	if deploymentTemplate.Status.Template == deploymentTemplate.Spec.Template && deploymentTemplate.Status.Parameters == deploymentTemplate.Spec.Parameters {
		logger.Info("Resource is already created and is up-to-date.")
		return nil, nil, nil
	}

	logger.Info("Template or parameters have changed, starting PUT operation.")

	var template any
	err := json.Unmarshal([]byte(deploymentTemplate.Spec.Template), &template)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to unmarshal template: %w", err)
	}

	var parameters any
	err = json.Unmarshal([]byte(deploymentTemplate.Spec.Parameters), &parameters)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to unmarshal parameters: %w", err)
	}

	// TODO PR: Is there a better way to check for all of this stuff?
	providerConfig := sdkclients.ProviderConfig{}
	err = json.Unmarshal([]byte(deploymentTemplate.Spec.ProviderConfig), &providerConfig)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to unmarshal template: %w", err)
	}
	if providerConfig.Deployments == nil {
		return nil, nil, fmt.Errorf("providerConfig.Deployments is nil")
	}
	if providerConfig.Deployments.Value.Scope == "" {
		return nil, nil, fmt.Errorf("providerConfig.Deployments.Value.Scope is empty")
	}
	if providerConfig.Radius == nil {
		return nil, nil, fmt.Errorf("providerConfig.Radius is nil")
	}
	if providerConfig.Radius.Value.Scope == "" {
		return nil, nil, fmt.Errorf("providerConfig.Radius.Value.Scope is empty")
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
		return nil, nil, err
	} else if poller != nil {
		return poller, nil, nil
	}

	// Update was synchronous
	deploymentTemplate.Status.Resource = resourceID
	return nil, nil, nil
}

func (r *DeploymentTemplateReconciler) startDeleteOperationIfNeeded(ctx context.Context, deploymentTemplate *radappiov1alpha3.DeploymentTemplate) (Poller[generated.GenericResourcesClientDeleteResponse], error) {
	logger := ucplog.FromContextOrDiscard(ctx)
	if deploymentTemplate.Status.Resource == "" {
		logger.Info("Resource is already deleted (or was never created).")
		return nil, nil
	}

	// TODOWILLSMITH: do we need to do anything here? wait for DeploymentResources to be deleted?

	// Deletion was synchronous

	deploymentTemplate.Status.Resource = ""
	return nil, nil
}

func (r *DeploymentTemplateReconciler) requeueDelay() time.Duration {
	delay := r.DelayInterval
	if delay == 0 {
		delay = PollingDelay
	}

	return delay
}

func parseDeploymentScopeFromProviderConfig(providerConfig string) (string, error) {
	config := sdkclients.ProviderConfig{}
	json.Unmarshal([]byte(providerConfig), &config)

	if config.Deployments == nil {
		return "", fmt.Errorf("providerConfig.Deployments is nil")
	}

	return config.Deployments.Value.Scope, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *DeploymentTemplateReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&radappiov1alpha3.DeploymentTemplate{}).
		Owns(&radappiov1alpha3.DeploymentResource{}).
		Complete(r)
}
