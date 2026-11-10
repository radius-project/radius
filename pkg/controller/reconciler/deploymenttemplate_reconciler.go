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

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armdeployments/v3"
	"github.com/radius-project/radius/pkg/cli/clients"
	radappiov1alpha3 "github.com/radius-project/radius/pkg/controller/api/radapp.io/v1alpha3"
	"github.com/radius-project/radius/pkg/hashutil"
	sdkclients "github.com/radius-project/radius/pkg/sdk/clients"
	"github.com/radius-project/radius/pkg/ucp/resources"
	resources_kubernetes "github.com/radius-project/radius/pkg/ucp/resources/kubernetes"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"uuid"
)

const (
	coordinatorIdle                = "Idle"
	coordinatorPutDispatching      = "PutDispatching"
	coordinatorPutRunning          = "PutRunning"
	coordinatorPutInventoryPending = "PutInventoryPending"
	coordinatorDeleteDispatching   = "DeleteDispatching"
	coordinatorDeleteRunning       = "DeleteRunning"
	coordinatorBlocked             = "Blocked"
	coordinatorRetryWaiting        = "RetryWaiting"
	outputCurrent                  = "Current"
	outputPendingDelete            = "PendingDelete"
)

// DeploymentTemplateReconciler owns all deployment and output deletion requests.
// DeploymentResource objects are inventory projections, not independent workers.
type DeploymentTemplateReconciler struct {
	Discovery                 discovery.DiscoveryInterface
	KubernetesClient          dynamic.Interface
	APIReader                 client.Reader
	Client                    client.Client
	Scheme                    *k8sruntime.Scheme
	EventRecorder             record.EventRecorder
	Radius                    RadiusClient
	ResourceDeploymentsClient sdkclients.ResourceDeploymentsClient
	DelayInterval             time.Duration
	DeleteRetryInterval       time.Duration
}

// Reconcile advances one durable lifecycle transition for the template.
func (r *DeploymentTemplateReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	owner := &radappiov1alpha3.DeploymentTemplate{}
	if err := r.reader().Get(ctx, req.NamespacedName, owner); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if owner.DeletionTimestamp != nil && !controllerutil.ContainsFinalizer(owner, DeploymentTemplateFinalizer) {
		return ctrl.Result{}, nil
	}
	if controllerutil.AddFinalizer(owner, DeploymentTemplateFinalizer) {
		if err := r.Client.Update(ctx, owner); err != nil {
			return ctrl.Result{}, err
		}
		return r.again(), nil
	}
	if owner.Status.Coordinator == nil {
		return r.initializeCoordinator(ctx, owner)
	}
	if owner.Status.Coordinator.Phase == coordinatorBlocked {
		// Do not replay an ambiguous DELETE against a subsequently recreated output.
		return ctrl.Result{}, nil
	}
	seen := map[string]bool{}
	for _, output := range owner.Status.Coordinator.Outputs {
		if output.State != outputCurrent && output.State != outputPendingDelete {
			return r.blockCoordinator(ctx, owner, fmt.Sprintf("unknown output state %q", output.State))
		}
		if seen[outputIdentity(output.ID)] {
			return r.blockCoordinator(ctx, owner, fmt.Sprintf("duplicate output identity %q", output.ID))
		}
		seen[outputIdentity(output.ID)] = true
		if _, err := resources.ParseResource(output.ID); err != nil {
			return r.blockCoordinator(ctx, owner, fmt.Sprintf("invalid inventory identity: %v", err))
		}
	}
	switch owner.Status.Coordinator.Phase {
	case coordinatorPutDispatching, coordinatorDeleteDispatching:
		return r.blockCoordinator(ctx, owner, "controller restarted before the dispatched request's outcome was recorded; resolve the outstanding operation before resuming the coordinator")
	case coordinatorPutRunning:
		return r.pollPut(ctx, owner)
	case coordinatorPutInventoryPending:
		return r.recoverFailedPutInventory(ctx, owner)
	case coordinatorDeleteRunning:
		if deadline := owner.Status.Coordinator.RetryAfter; deadline != nil {
			if remaining := time.Until(deadline.Time); remaining > 0 {
				return ctrl.Result{RequeueAfter: remaining}, nil
			}
		}
		return r.pollDelete(ctx, owner)
	case coordinatorRetryWaiting:
		if owner.Status.Coordinator.RetryAfter == nil {
			return r.blockCoordinator(ctx, owner, "retry state has no deadline")
		}
		if remaining := time.Until(owner.Status.Coordinator.RetryAfter.Time); remaining > 0 {
			return ctrl.Result{RequeueAfter: remaining}, nil
		}
		owner.Status.Coordinator.Phase = coordinatorIdle
		owner.Status.Coordinator.RetryAfter = nil
	case coordinatorIdle:
	default:
		return r.blockCoordinator(ctx, owner, fmt.Sprintf("unknown coordinator phase %q", owner.Status.Coordinator.Phase))
	}
	children, err := r.ownedTrackers(ctx, owner)
	if err != nil {
		return ctrl.Result{}, err
	}
	for _, child := range children {
		if child.Status.ProvisionedResourceID != "" && !sameOutput(child.Status.ProvisionedResourceID, child.Spec.Id) {
			return r.blockCoordinator(ctx, owner, fmt.Sprintf("tracker %q conflicts with its protected output identity", child.Name))
		}
	}
	for i := range children {
		child := &children[i]
		if child.Status.Operation == nil {
			continue
		}
		if child.Status.Operation.OperationKind != radappiov1alpha3.OperationKindDelete {
			return r.blockCoordinator(ctx, owner, "legacy tracker has an unknown operation kind")
		}
		eligible, err := r.trackerEligible(ctx, child)
		if err != nil {
			return ctrl.Result{}, err
		}
		if !eligible {
			return r.blockCoordinator(ctx, owner, "legacy DELETE is not authorized by its tracker identity")
		}
		if child.Status.ProvisionedResourceID == "" {
			identity, uid := child.Spec.Id, child.UID
			child.Status.ProvisionedResourceID = identity
			if err := r.Client.Status().Update(ctx, child); err != nil {
				return ctrl.Result{}, err
			}
			persisted := &radappiov1alpha3.DeploymentResource{}
			if err := r.reader().Get(ctx, client.ObjectKeyFromObject(child), persisted); err != nil {
				return ctrl.Result{}, err
			}
			if persisted.UID != uid || !sameOutput(persisted.Status.ProvisionedResourceID, identity) || !sameOutput(persisted.Spec.Id, identity) {
				return r.blockCoordinator(ctx, owner, "legacy DELETE identity was not persisted; verify the tracker and CRD schema before adoption")
			}
			return r.again(), nil
		}
		if findOutput(owner, child.Spec.Id) == nil {
			owner.Status.Coordinator.Outputs = append(owner.Status.Coordinator.Outputs, radappiov1alpha3.DeploymentOutputStatus{ID: child.Spec.Id, State: outputPendingDelete})
		}
		findOutput(owner, child.Spec.Id).State = outputPendingDelete
		owner.Status.Operation = child.Status.Operation.DeepCopy()
		owner.Status.Coordinator.Phase = coordinatorDeleteRunning
		owner.Status.Coordinator.ResourceID = child.Spec.Id
		if slices.ContainsFunc(owner.Status.OutputResources, func(id string) bool { return sameOutput(id, child.Spec.Id) }) {
			owner.Status.StatusHash = ""
		}
		return r.saveAndRequeue(ctx, owner)
	}
	if owner.DeletionTimestamp == nil {
		namespace := &corev1.Namespace{}
		if err := r.reader().Get(ctx, client.ObjectKey{Name: owner.Namespace}, namespace); err != nil {
			return ctrl.Result{}, err
		}
		if namespace.DeletionTimestamp != nil {
			if err := r.Client.Delete(ctx, owner); client.IgnoreNotFound(err) != nil {
				return ctrl.Result{}, err
			}
			return r.again(), nil
		}
	}

	changed := false
	for i := range owner.Status.Coordinator.Outputs {
		output := &owner.Status.Coordinator.Outputs[i]
		if owner.DeletionTimestamp != nil && output.State != outputPendingDelete {
			output.State = outputPendingDelete
			changed = true
		}
		for _, child := range children {
			if sameOutput(child.Spec.Id, output.ID) && child.DeletionTimestamp != nil &&
				controllerutil.ContainsFinalizer(&child, DeploymentResourceFinalizer) && output.State == outputCurrent {
				output.State = outputPendingDelete
				owner.Status.StatusHash = ""
				changed = true
			}
		}
	}
	if changed {
		return r.saveAndRequeue(ctx, owner)
	}

	// Complete safe tracker-only retirement before planning further work.
	for i := range children {
		child := &children[i]
		output := findOutput(owner, child.Spec.Id)
		if (output == nil && child.DeletionTimestamp == nil) ||
			(child.DeletionTimestamp != nil && controllerutil.ContainsFinalizer(child, DeploymentResourceFinalizer)) {
			if err := r.retireTracker(ctx, child); err != nil {
				return ctrl.Result{}, err
			}
			return r.again(), nil
		}
	}
	for _, output := range owner.Status.Coordinator.Outputs {
		if output.State != outputPendingDelete {
			continue
		}
		dependency, err := isApplicationOrEnvironment(output.ID)
		if err != nil {
			return r.blockCoordinator(ctx, owner, err.Error())
		}
		if dependency && r.hasLeafOutputs(owner, output.ID) {
			continue
		}
		return r.dispatchDelete(ctx, owner, output.ID)
	}
	if owner.DeletionTimestamp != nil {
		if len(owner.Status.Coordinator.Outputs) != 0 {
			return r.again(), nil
		}
		if controllerutil.RemoveFinalizer(owner, DeploymentTemplateFinalizer) {
			return ctrl.Result{}, r.Client.Update(ctx, owner)
		}
		return ctrl.Result{}, nil
	}
	if !isUpToDate(owner) {
		return r.dispatchPut(ctx, owner)
	}
	for _, output := range owner.Status.Coordinator.Outputs {
		if output.State == outputCurrent {
			if err := r.ensureTracker(ctx, owner, output.ID, children); err != nil {
				return ctrl.Result{}, err
			}
		}
	}
	phrase := radappiov1alpha3.DeploymentTemplatePhraseReady
	for _, output := range owner.Status.Coordinator.Outputs {
		if output.State == outputPendingDelete {
			phrase = radappiov1alpha3.DeploymentTemplatePhraseReadyPendingCleanup
		}
	}
	if owner.Status.Phrase != phrase {
		owner.Status.Phrase = phrase
		return r.saveAndRequeue(ctx, owner)
	}
	return ctrl.Result{}, nil
}

func (r *DeploymentTemplateReconciler) initializeCoordinator(ctx context.Context, owner *radappiov1alpha3.DeploymentTemplate) (ctrl.Result, error) {
	owner.Status.Coordinator = &radappiov1alpha3.DeploymentCoordinatorStatus{Phase: coordinatorIdle}
	for _, id := range owner.Status.OutputResources {
		if findOutput(owner, id) == nil {
			owner.Status.Coordinator.Outputs = append(owner.Status.Coordinator.Outputs, radappiov1alpha3.DeploymentOutputStatus{ID: id, State: outputCurrent})
		}
	}
	children, err := r.ownedTrackers(ctx, owner)
	if err != nil {
		return ctrl.Result{}, err
	}
	for _, child := range children {
		eligible, err := r.trackerEligible(ctx, &child)
		if err != nil {
			return ctrl.Result{}, err
		}
		if !eligible {
			r.EventRecorder.Event(owner, corev1.EventTypeNormal, EventDeploymentResourceDeleteSkipped, fmt.Sprintf("Tracker %q does not authorize deletion of %q", child.Name, child.Spec.Id))
			continue
		}
		if findOutput(owner, child.Spec.Id) == nil {
			owner.Status.Coordinator.Outputs = append(owner.Status.Coordinator.Outputs, radappiov1alpha3.DeploymentOutputStatus{ID: child.Spec.Id, State: outputPendingDelete})
		}
		// Known tokens remain on passive children until the coordinator adopts them.
		if child.Status.Operation == nil && child.DeletionTimestamp != nil &&
			(child.Status.Phrase == radappiov1alpha3.DeploymentResourcePhraseDeleting || child.Status.Phrase == radappiov1alpha3.DeploymentResourcePhraseFailed) {
			return r.blockCoordinator(ctx, owner, "legacy tracker may have dispatched DELETE without a saved token; resolve its outcome before adoption")
		}
	}
	if owner.Status.Operation != nil && owner.Status.Coordinator.Phase == coordinatorIdle {
		switch owner.Status.Operation.OperationKind {
		case radappiov1alpha3.OperationKindPut:
			owner.Status.Coordinator.Phase = coordinatorPutRunning
			// A legacy operation did not persist its submitted hash. Force a fresh PUT.
		case radappiov1alpha3.OperationKindDelete:
			if len(owner.Status.OutputResources) == 0 {
				return r.blockCoordinator(ctx, owner, "legacy inventory DELETE has no recorded target")
			}
			owner.Status.Coordinator.Phase = coordinatorDeleteRunning
			owner.Status.Coordinator.ResourceID = owner.Status.OutputResources[0]
			findOutput(owner, owner.Status.Coordinator.ResourceID).State = outputPendingDelete
		default:
			return r.blockCoordinator(ctx, owner, "unknown legacy operation kind")
		}
	}
	return r.saveAndRequeue(ctx, owner)
}

func (r *DeploymentTemplateReconciler) dispatchPut(ctx context.Context, owner *radappiov1alpha3.DeploymentTemplate) (ctrl.Result, error) {
	var template any
	if err := json.Unmarshal([]byte(owner.Spec.Template), &template); err != nil {
		return r.failCoordinator(ctx, owner, fmt.Errorf("invalid template: %w", err))
	}
	config := sdkclients.ProviderConfig{}
	if err := json.Unmarshal([]byte(owner.Spec.ProviderConfig), &config); err != nil {
		return r.failCoordinator(ctx, owner, fmt.Errorf("invalid provider config: %w", err))
	}
	scope, err := ParseDeploymentScopeFromProviderConfig(owner.Spec.ProviderConfig)
	if err != nil {
		return r.failCoordinator(ctx, owner, err)
	}
	if err := createResourceGroupIfNotExists(ctx, r.Radius, scope); err != nil {
		return r.failCoordinator(ctx, owner, err)
	}
	hash, err := computeHash(owner)
	if err != nil {
		return ctrl.Result{}, err
	}
	state := owner.Status.Coordinator
	state.Phase = coordinatorPutDispatching
	state.SubmittedHash = hash
	state.RetryAfter = nil
	state.ResourceID = scope + "/providers/Microsoft.Resources/deployments/deploymenttemplate-" + uuid.New().String()
	state.Message = ""
	owner.Status.ObservedGeneration = owner.Generation
	owner.Status.StatusHash = ""
	owner.Status.Phrase = radappiov1alpha3.DeploymentTemplatePhraseUpdating
	if err := r.saveCoordinator(ctx, owner); err != nil {
		return ctrl.Result{}, err
	}
	mutationContext := policy.WithRetryOptions(ctx, policy.RetryOptions{MaxRetries: -1})
	poller, err := r.ResourceDeploymentsClient.CreateOrUpdate(mutationContext, sdkclients.Deployment{
		Properties: &sdkclients.DeploymentProperties{
			Template: template, Parameters: convertToARMJSONParameters(owner.Spec.Parameters),
			ProviderConfig: config, Mode: armdeployments.DeploymentModeIncremental,
		},
	}, state.ResourceID, sdkclients.DeploymentsClientAPIVersion)
	return r.finishDispatch(ctx, owner, func(current *radappiov1alpha3.DeploymentTemplate) (ctrl.Result, error) {
		if err != nil {
			if requestRejected(err) {
				return r.failCoordinator(ctx, current, err)
			}
			return r.blockCoordinator(ctx, current, fmt.Sprintf("PUT outcome is uncertain: %v", err))
		}
		if poller == nil {
			return r.blockCoordinator(ctx, current, "PUT returned no output inventory or operation token")
		}
		if poller.Done() {
			resp, err := poller.Result(ctx)
			if err != nil {
				return r.failedPutInventoryPending(ctx, current, err)
			}
			return r.completePut(ctx, current, resp)
		}
		token, err := poller.ResumeToken()
		if err != nil {
			return r.blockCoordinator(ctx, current, fmt.Sprintf("PUT resume token unavailable: %v", err))
		}
		current.Status.Operation = &radappiov1alpha3.ResourceOperation{OperationKind: radappiov1alpha3.OperationKindPut, ResumeToken: token}
		current.Status.Coordinator.Phase = coordinatorPutRunning
		return r.saveAndRequeue(ctx, current)
	})
}

func (r *DeploymentTemplateReconciler) pollPut(ctx context.Context, owner *radappiov1alpha3.DeploymentTemplate) (ctrl.Result, error) {
	if owner.Status.Operation == nil || owner.Status.Operation.OperationKind != radappiov1alpha3.OperationKindPut {
		return r.blockCoordinator(ctx, owner, "running PUT has no matching operation")
	}
	poller, err := r.ResourceDeploymentsClient.ContinueCreateOperation(ctx, owner.Status.Operation.ResumeToken)
	if err != nil {
		return r.operationReadFailed(ctx, owner, err)
	}
	if _, err := poller.Poll(ctx); err != nil {
		return r.operationReadFailed(ctx, owner, err)
	}
	if !poller.Done() {
		return r.again(), nil
	}
	resp, err := poller.Result(ctx)
	if err != nil {
		return r.failedPutInventoryPending(ctx, owner, err)
	}
	return r.completePut(ctx, owner, resp)
}

func (r *DeploymentTemplateReconciler) failedPutInventoryPending(ctx context.Context, owner *radappiov1alpha3.DeploymentTemplate, err error) (ctrl.Result, error) {
	// Azure pollers return a zero result on failure, not the deployment's partial inventory.
	owner.Status.Coordinator.Phase = coordinatorPutInventoryPending
	owner.Status.Coordinator.Message = err.Error()
	owner.Status.Phrase = radappiov1alpha3.DeploymentTemplatePhraseFailed
	return r.saveAndRequeue(ctx, owner)
}

func (r *DeploymentTemplateReconciler) recoverFailedPutInventory(ctx context.Context, owner *radappiov1alpha3.DeploymentTemplate) (ctrl.Result, error) {
	if owner.Status.Coordinator.ResourceID == "" {
		return r.blockCoordinator(ctx, owner, "failed PUT has no durable deployment ID for inventory recovery")
	}
	resp, err := r.ResourceDeploymentsClient.Get(ctx, owner.Status.Coordinator.ResourceID, sdkclients.DeploymentsClientAPIVersion)
	if err != nil {
		return r.operationReadFailed(ctx, owner, fmt.Errorf("read failed deployment inventory: %w", err))
	}
	if resp.Properties == nil {
		return r.operationReadFailed(ctx, owner, fmt.Errorf("failed deployment has no inventory properties"))
	}
	if err := r.retainPartialPutOutputs(owner, resp); err != nil {
		return r.blockCoordinator(ctx, owner, err.Error())
	}
	return r.failCoordinator(ctx, owner, errors.New(owner.Status.Coordinator.Message))
}

func (r *DeploymentTemplateReconciler) retainPartialPutOutputs(owner *radappiov1alpha3.DeploymentTemplate, resp sdkclients.ClientCreateOrUpdateResponse) error {
	if resp.Properties == nil {
		return nil
	}
	for _, ref := range resp.Properties.OutputResources {
		if ref == nil || ref.ID == nil {
			continue
		}
		if _, err := resources.ParseResource(*ref.ID); err != nil {
			return fmt.Errorf("invalid partial deployment output: %w", err)
		}
		if findOutput(owner, *ref.ID) == nil {
			owner.Status.Coordinator.Outputs = append(owner.Status.Coordinator.Outputs, radappiov1alpha3.DeploymentOutputStatus{ID: *ref.ID, State: outputCurrent})
		}
	}
	return nil
}

func (r *DeploymentTemplateReconciler) completePut(ctx context.Context, owner *radappiov1alpha3.DeploymentTemplate, resp sdkclients.ClientCreateOrUpdateResponse) (ctrl.Result, error) {
	var outputs []string
	if resp.Properties != nil {
		for _, ref := range resp.Properties.OutputResources {
			if ref != nil && ref.ID != nil && !slices.ContainsFunc(outputs, func(id string) bool { return sameOutput(id, *ref.ID) }) {
				if _, err := resources.ParseResource(*ref.ID); err != nil {
					return r.blockCoordinator(ctx, owner, fmt.Sprintf("invalid completed output: %v", err))
				}
				outputs = append(outputs, *ref.ID)
			}
		}
	}
	state := owner.Status.Coordinator
	for i := range state.Outputs {
		state.Outputs[i].State = outputPendingDelete
	}
	for _, id := range outputs {
		if output := findOutput(owner, id); output != nil {
			output.State = outputCurrent
		} else {
			state.Outputs = append(state.Outputs, radappiov1alpha3.DeploymentOutputStatus{ID: id, State: outputCurrent})
		}
	}
	// Commit inventory before projecting any children. Tracker disappearance cannot erase it.
	owner.Status.OutputResources = outputs
	owner.Status.StatusHash = state.SubmittedHash
	owner.Status.Operation = nil
	owner.Status.Phrase = radappiov1alpha3.DeploymentTemplatePhraseReadyPendingCleanup
	state.Phase, state.ResourceID, state.Message = coordinatorIdle, "", ""
	state.RetryAfter = nil
	return r.saveAndRequeue(ctx, owner)
}

func (r *DeploymentTemplateReconciler) dispatchDelete(ctx context.Context, owner *radappiov1alpha3.DeploymentTemplate, outputID string) (ctrl.Result, error) {
	id, err := resources.ParseResource(outputID)
	if err != nil {
		return r.blockCoordinator(ctx, owner, err.Error())
	}
	if plane := id.FindScope(resources_kubernetes.PlaneTypeKubernetes); plane != "" {
		if !strings.EqualFold(plane, resources_kubernetes.PlaneNameTODO) {
			return r.skipOutput(ctx, owner, outputID)
		}
	} else {
		scope, err := ParseDeploymentScopeFromProviderConfig(owner.Spec.ProviderConfig)
		if err != nil {
			return r.failCoordinator(ctx, owner, err)
		}
		within, err := resourceWithinScope(outputID, scope)
		if err != nil {
			return r.failCoordinator(ctx, owner, err)
		}
		if !within {
			return r.skipOutput(ctx, owner, outputID)
		}
	}
	state := owner.Status.Coordinator
	state.Phase, state.ResourceID, state.Message = coordinatorDeleteDispatching, outputID, ""
	state.RetryAfter = nil
	if owner.DeletionTimestamp != nil {
		owner.Status.Phrase = radappiov1alpha3.DeploymentTemplatePhraseDeleting
	} else {
		owner.Status.Phrase = radappiov1alpha3.DeploymentTemplatePhraseReadyPendingCleanup
	}
	if err := r.saveCoordinator(ctx, owner); err != nil {
		return ctrl.Result{}, err
	}
	if id.FindScope(resources_kubernetes.PlaneTypeKubernetes) != "" {
		deleter := &DeploymentResourceReconciler{Client: r.Client, Discovery: r.Discovery, KubernetesClient: r.KubernetesClient}
		done, deleteErr := deleter.deleteKubernetesOutput(ctx, owner.Namespace, id)
		return r.finishDispatch(ctx, owner, func(current *radappiov1alpha3.DeploymentTemplate) (ctrl.Result, error) {
			return r.recordKubernetesDelete(ctx, current, done, deleteErr)
		})
	}
	mutationContext := policy.WithRetryOptions(ctx, policy.RetryOptions{MaxRetries: -1})
	poller, err := r.ResourceDeploymentsClient.Delete(mutationContext, outputID, "2023-10-01-preview")
	return r.finishDispatch(ctx, owner, func(current *radappiov1alpha3.DeploymentTemplate) (ctrl.Result, error) {
		if err != nil {
			if clients.Is404Error(err) {
				return r.completeOutputDelete(ctx, current)
			}
			if requestRejected(err) {
				return r.failCoordinator(ctx, current, err)
			}
			return r.blockCoordinator(ctx, current, fmt.Sprintf("DELETE outcome for %q is uncertain: %v", outputID, err))
		}
		if poller == nil {
			return r.completeOutputDelete(ctx, current)
		}
		if poller.Done() {
			if _, err := poller.Result(ctx); err != nil && !clients.Is404Error(err) {
				return r.failCoordinator(ctx, current, err)
			}
			return r.completeOutputDelete(ctx, current)
		}
		token, err := poller.ResumeToken()
		if err != nil {
			return r.blockCoordinator(ctx, current, fmt.Sprintf("DELETE resume token unavailable: %v", err))
		}
		current.Status.Operation = &radappiov1alpha3.ResourceOperation{OperationKind: radappiov1alpha3.OperationKindDelete, ResumeToken: token}
		current.Status.Coordinator.Phase = coordinatorDeleteRunning
		return r.saveAndRequeue(ctx, current)
	})
}

func (r *DeploymentTemplateReconciler) pollDelete(ctx context.Context, owner *radappiov1alpha3.DeploymentTemplate) (ctrl.Result, error) {
	id, err := resources.ParseResource(owner.Status.Coordinator.ResourceID)
	if err != nil {
		return r.blockCoordinator(ctx, owner, err.Error())
	}
	if id.FindScope(resources_kubernetes.PlaneTypeKubernetes) != "" {
		return r.dispatchDelete(ctx, owner, id.String())
	}
	if owner.Status.Operation == nil || owner.Status.Operation.OperationKind != radappiov1alpha3.OperationKindDelete {
		return r.blockCoordinator(ctx, owner, "running DELETE has no matching operation")
	}
	poller, err := r.ResourceDeploymentsClient.ContinueDeleteOperation(ctx, owner.Status.Operation.ResumeToken)
	if err != nil {
		return r.operationReadFailed(ctx, owner, err)
	}
	if _, err := poller.Poll(ctx); err != nil {
		return r.operationReadFailed(ctx, owner, err)
	}
	if !poller.Done() {
		return r.again(), nil
	}
	if _, err := poller.Result(ctx); err != nil && !clients.Is404Error(err) {
		// This is a known terminal failure, unlike a dispatch/transport failure.
		if clearErr := r.clearLegacyToken(ctx, owner, radappiov1alpha3.DeploymentResourcePhraseFailed); clearErr != nil {
			return ctrl.Result{}, clearErr
		}
		return r.failCoordinator(ctx, owner, err)
	}
	return r.completeOutputDelete(ctx, owner)
}

func (r *DeploymentTemplateReconciler) recordKubernetesDelete(ctx context.Context, owner *radappiov1alpha3.DeploymentTemplate, done bool, err error) (ctrl.Result, error) {
	if err != nil {
		var uncertain outputDeleteUncertainError
		if errors.As(err, &uncertain) {
			return r.blockCoordinator(ctx, owner, err.Error())
		}
		return r.failCoordinator(ctx, owner, err)
	}
	if done {
		return r.completeOutputDelete(ctx, owner)
	}
	owner.Status.Coordinator.Phase = coordinatorDeleteRunning
	owner.Status.Coordinator.RetryAfter = coordinatorDeadline(r.requeueDelay())
	return r.saveAndRequeue(ctx, owner)
}

// finishDispatch attaches a response to the same persisted intent, even if spec
// edits changed resourceVersion while the HTTP request was outstanding.
func (r *DeploymentTemplateReconciler) finishDispatch(ctx context.Context, owner *radappiov1alpha3.DeploymentTemplate, finish func(*radappiov1alpha3.DeploymentTemplate) (ctrl.Result, error)) (ctrl.Result, error) {
	intent := owner.Status.Coordinator.DeepCopy()
	var result ctrl.Result
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		current := &radappiov1alpha3.DeploymentTemplate{}
		if err := r.reader().Get(ctx, client.ObjectKeyFromObject(owner), current); err != nil {
			return err
		}
		if current.UID != owner.UID || !apiequality.Semantic.DeepEqual(current.Status.Coordinator, intent) {
			return fmt.Errorf("coordinator changed while request was outstanding; refusing to overwrite its recovery state")
		}
		var err error
		result, err = finish(current)
		return err
	})
	return result, err
}

func (r *DeploymentTemplateReconciler) completeOutputDelete(ctx context.Context, owner *radappiov1alpha3.DeploymentTemplate) (ctrl.Result, error) {
	state := owner.Status.Coordinator
	target := state.ResourceID
	// Token completion is independent of the child's mutable spec identity.
	if err := r.clearLegacyToken(ctx, owner, radappiov1alpha3.DeploymentResourcePhraseDeleted); err != nil {
		return ctrl.Result{}, err
	}
	// Children are passive, so removing one cannot dispatch another DELETE.
	children, err := r.ownedTrackers(ctx, owner)
	if err != nil {
		return ctrl.Result{}, err
	}
	for i := range children {
		if sameOutput(children[i].Spec.Id, target) {
			operation := children[i].Status.Operation
			if operation != nil && (owner.Status.Operation == nil || operation.ResumeToken != owner.Status.Operation.ResumeToken) {
				// Another pre-upgrade request for this output still needs to drain.
				owner.Status.Operation = nil
				state.Phase, state.ResourceID = coordinatorIdle, ""
				owner.Status.StatusHash = ""
				return r.saveAndRequeue(ctx, owner)
			}
			if err := r.retireTracker(ctx, &children[i]); err != nil {
				return ctrl.Result{}, err
			}
		}
	}
	state.Outputs = slices.DeleteFunc(state.Outputs, func(output radappiov1alpha3.DeploymentOutputStatus) bool { return sameOutput(output.ID, target) })
	state.Phase, state.ResourceID, state.Message = coordinatorIdle, "", ""
	state.RetryAfter = nil
	owner.Status.Operation = nil
	return r.saveAndRequeue(ctx, owner)
}

func (r *DeploymentTemplateReconciler) clearLegacyToken(ctx context.Context, owner *radappiov1alpha3.DeploymentTemplate, phrase radappiov1alpha3.DeploymentResourcePhrase) error {
	if owner.Status.Operation == nil {
		return nil
	}
	children, err := r.ownedTrackers(ctx, owner)
	if err != nil {
		return err
	}
	for i := range children {
		child := &children[i]
		if child.Status.Operation != nil && child.Status.Operation.ResumeToken == owner.Status.Operation.ResumeToken {
			child.Status.Operation = nil
			child.Status.Phrase = phrase
			if err := r.Client.Status().Update(ctx, child); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *DeploymentTemplateReconciler) ensureTracker(ctx context.Context, owner *radappiov1alpha3.DeploymentTemplate, id string, children []radappiov1alpha3.DeploymentResource) error {
	for i := range children {
		child := &children[i]
		if !sameOutput(child.Spec.Id, id) {
			continue
		}
		if child.DeletionTimestamp != nil {
			return nil
		}
		if child.Status.ProvisionedResourceID != "" && !sameOutput(child.Status.ProvisionedResourceID, id) {
			return fmt.Errorf("tracker %q conflicts with recorded output identity", child.Name)
		}
		if sameOutput(child.Status.ProvisionedResourceID, id) {
			return nil
		}
		child.Status.ProvisionedResourceID = id
		return r.Client.Status().Update(ctx, child)
	}
	name, err := generateDeploymentResourceName(id)
	if err != nil {
		return err
	}
	child := &radappiov1alpha3.DeploymentResource{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: owner.Namespace, Finalizers: []string{DeploymentResourceFinalizer}},
		Spec:       radappiov1alpha3.DeploymentResourceSpec{Id: id},
	}
	if err := controllerutil.SetControllerReference(owner, child, r.Scheme); err != nil {
		return err
	}
	// GET first: duplicate POST can fail quota admission before AlreadyExists.
	existing := &radappiov1alpha3.DeploymentResource{}
	if err := r.reader().Get(ctx, client.ObjectKeyFromObject(child), existing); err == nil {
		return fmt.Errorf("tracker %q exists but does not match this template output", name)
	} else if !apierrors.IsNotFound(err) {
		return err
	}
	if err := r.Client.Create(ctx, child); err != nil {
		return err
	}
	child.Status.ProvisionedResourceID = id
	return r.Client.Status().Update(ctx, child)
}

func (r *DeploymentTemplateReconciler) retireTracker(ctx context.Context, child *radappiov1alpha3.DeploymentResource) error {
	if child.DeletionTimestamp == nil {
		if err := r.Client.Delete(ctx, child); client.IgnoreNotFound(err) != nil {
			return err
		}
		if err := r.reader().Get(ctx, client.ObjectKeyFromObject(child), child); err != nil {
			return client.IgnoreNotFound(err)
		}
	}
	if controllerutil.RemoveFinalizer(child, DeploymentResourceFinalizer) {
		return r.Client.Update(ctx, child)
	}
	return nil
}

func (r *DeploymentTemplateReconciler) ownedTrackers(ctx context.Context, owner *radappiov1alpha3.DeploymentTemplate) ([]radappiov1alpha3.DeploymentResource, error) {
	list := &radappiov1alpha3.DeploymentResourceList{}
	if err := r.reader().List(ctx, list, client.InNamespace(owner.Namespace)); err != nil {
		return nil, err
	}
	return slices.DeleteFunc(list.Items, func(child radappiov1alpha3.DeploymentResource) bool { return !isOwnedBy(child, owner) }), nil
}

func (r *DeploymentTemplateReconciler) trackerEligible(ctx context.Context, child *radappiov1alpha3.DeploymentResource) (bool, error) {
	checker := &DeploymentResourceReconciler{Client: r.Client, APIReader: r.reader()}
	return checker.deleteIsEligible(ctx, child, metav1.GetControllerOf(child))
}

func findOutput(owner *radappiov1alpha3.DeploymentTemplate, id string) *radappiov1alpha3.DeploymentOutputStatus {
	for i := range owner.Status.Coordinator.Outputs {
		if sameOutput(owner.Status.Coordinator.Outputs[i].ID, id) {
			return &owner.Status.Coordinator.Outputs[i]
		}
	}
	return nil
}

func (r *DeploymentTemplateReconciler) hasLeafOutputs(owner *radappiov1alpha3.DeploymentTemplate, except string) bool {
	for _, output := range owner.Status.Coordinator.Outputs {
		if sameOutput(output.ID, except) {
			continue
		}
		dependency, err := isApplicationOrEnvironment(output.ID)
		if err != nil || !dependency {
			return true
		}
	}
	return false
}

func (r *DeploymentTemplateReconciler) saveCoordinator(ctx context.Context, owner *radappiov1alpha3.DeploymentTemplate) error {
	if len(owner.Status.Coordinator.Outputs) == 0 {
		owner.Status.Coordinator.Outputs = nil
	}
	expected := owner.DeepCopy().Status
	if err := r.Client.Status().Update(ctx, owner); err != nil {
		return err
	}
	persisted := &radappiov1alpha3.DeploymentTemplate{}
	if err := r.reader().Get(ctx, client.ObjectKeyFromObject(owner), persisted); err != nil {
		return err
	}
	if !apiequality.Semantic.DeepEqual(expected.Coordinator, persisted.Status.Coordinator) || !apiequality.Semantic.DeepEqual(expected.Operation, persisted.Status.Operation) {
		return fmt.Errorf("template lifecycle state was not persisted; verify the installed CRD schema before dispatching operations")
	}
	return nil
}

func (r *DeploymentTemplateReconciler) saveAndRequeue(ctx context.Context, owner *radappiov1alpha3.DeploymentTemplate) (ctrl.Result, error) {
	if err := r.saveCoordinator(ctx, owner); err != nil {
		return ctrl.Result{}, err
	}
	return r.again(), nil
}

func (r *DeploymentTemplateReconciler) blockCoordinator(ctx context.Context, owner *radappiov1alpha3.DeploymentTemplate, message string) (ctrl.Result, error) {
	owner.Status.Coordinator.Phase = coordinatorBlocked
	owner.Status.Coordinator.Message = message
	owner.Status.Phrase = radappiov1alpha3.DeploymentTemplatePhraseFailed
	r.EventRecorder.Event(owner, corev1.EventTypeWarning, "OperationOutcomeUnknown", message)
	ucplog.FromContextOrDiscard(ctx).Error(fmt.Errorf("%s", message), "Template lifecycle requires operator recovery")
	return ctrl.Result{}, r.saveCoordinator(ctx, owner)
}

func (r *DeploymentTemplateReconciler) failCoordinator(ctx context.Context, owner *radappiov1alpha3.DeploymentTemplate, err error) (ctrl.Result, error) {
	owner.Status.Coordinator.Phase = coordinatorRetryWaiting
	owner.Status.Coordinator.RetryAfter = coordinatorDeadline(r.deleteRetryDelay())
	owner.Status.Coordinator.Message = err.Error()
	owner.Status.Operation = nil
	owner.Status.Phrase = radappiov1alpha3.DeploymentTemplatePhraseFailed
	r.EventRecorder.Event(owner, corev1.EventTypeWarning, "ResourceError", err.Error())
	if saveErr := r.saveCoordinator(ctx, owner); saveErr != nil {
		return ctrl.Result{}, saveErr
	}
	return ctrl.Result{RequeueAfter: r.deleteRetryDelay()}, nil
}

func coordinatorDeadline(delay time.Duration) *metav1.Time {
	// metav1.Time is serialized at second precision. Round up, never shorten a retry.
	return &metav1.Time{Time: time.Now().Add(delay).UTC().Truncate(time.Second).Add(time.Second)}
}

func (r *DeploymentTemplateReconciler) operationReadFailed(ctx context.Context, owner *radappiov1alpha3.DeploymentTemplate, err error) (ctrl.Result, error) {
	r.EventRecorder.Event(owner, corev1.EventTypeWarning, "ResourceError", err.Error())
	// Preserve the token and phase; a polling failure is not a terminal operation failure.
	return ctrl.Result{RequeueAfter: r.deleteRetryDelay()}, nil
}

func (r *DeploymentTemplateReconciler) skipOutput(ctx context.Context, owner *radappiov1alpha3.DeploymentTemplate, id string) (ctrl.Result, error) {
	r.EventRecorder.Event(owner, corev1.EventTypeNormal, EventDeploymentResourceDeleteSkipped, fmt.Sprintf("Skipping out-of-scope output %q", id))
	owner.Status.Coordinator.ResourceID = id
	return r.completeOutputDelete(ctx, owner)
}

func (r *DeploymentTemplateReconciler) reader() client.Reader {
	if r.APIReader != nil {
		return r.APIReader
	}
	return r.Client
}

func (r *DeploymentTemplateReconciler) requeueDelay() time.Duration {
	if r.DelayInterval != 0 {
		return r.DelayInterval
	}
	return PollingDelay
}

func (r *DeploymentTemplateReconciler) deleteRetryDelay() time.Duration {
	if r.DeleteRetryInterval != 0 {
		return r.DeleteRetryInterval
	}
	return DeleteRetryDelay
}

func (r *DeploymentTemplateReconciler) again() ctrl.Result {
	return ctrl.Result{RequeueAfter: r.requeueDelay()}
}

// ParseDeploymentScopeFromProviderConfig extracts the required deployment scope.
func ParseDeploymentScopeFromProviderConfig(providerConfig any) (string, error) {
	var data []byte
	switch value := providerConfig.(type) {
	case string:
		data = []byte(value)
	case []byte:
		data = value
	default:
		return "", fmt.Errorf("providerConfig must be a string or []byte, got %T", providerConfig)
	}
	config := sdkclients.ProviderConfig{}
	if err := json.Unmarshal(data, &config); err != nil {
		return "", fmt.Errorf("failed to unmarshal providerConfig: %w", err)
	}
	if config.Deployments == nil || config.Deployments.Value.Scope == "" {
		return "", fmt.Errorf("providerConfig.Deployments.Value.Scope is required")
	}
	return config.Deployments.Value.Scope, nil
}

func isOwnedBy(resource radappiov1alpha3.DeploymentResource, owner *radappiov1alpha3.DeploymentTemplate) bool {
	ref := metav1.GetControllerOf(&resource)
	return ref != nil && ref.APIVersion == radappiov1alpha3.GroupVersion.String() &&
		ref.Kind == deploymentTemplateKind && ref.Name == owner.Name && ref.UID == owner.UID
}

func computeHash(owner *radappiov1alpha3.DeploymentTemplate) (string, error) {
	data, err := json.Marshal(owner.Spec)
	if err != nil {
		return "", err
	}
	return hashutil.Hex(data), nil
}

func isUpToDate(owner *radappiov1alpha3.DeploymentTemplate) bool {
	data, err := json.Marshal(owner.Spec)
	if err != nil {
		return false
	}
	return owner.Status.StatusHash == hashutil.Hex(data) || owner.Status.StatusHash == hashutil.LegacyHex(data)
}

// SetupWithManager registers the coordinator and watches its passive inventory.
func (r *DeploymentTemplateReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&radappiov1alpha3.DeploymentTemplate{}).
		Owns(&radappiov1alpha3.DeploymentResource{}).
		Complete(r)
}
