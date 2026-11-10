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
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	radappiov1alpha3 "github.com/radius-project/radius/pkg/controller/api/radapp.io/v1alpha3"
	"github.com/radius-project/radius/pkg/ucp/resources"
	resources_kubernetes "github.com/radius-project/radius/pkg/ucp/resources/kubernetes"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// DeploymentResourceReconciler maintains passive inventory projections.
// Only the owning DeploymentTemplate coordinator may dispatch output operations.
type DeploymentResourceReconciler struct {
	APIReader     client.Reader
	Discovery     discovery.DiscoveryInterface
	Client        client.Client
	Scheme        *runtime.Scheme
	EventRecorder record.EventRecorder
}

type outputDeleteUncertainError struct{ error }

func (e outputDeleteUncertainError) Unwrap() error { return e.error }

func requestRejected(err error) bool {
	var status apierrors.APIStatus
	if errors.As(err, &status) {
		code := status.Status().Code
		return code >= 400 && code < 500 && code != 408
	}
	var response *azcore.ResponseError
	if errors.As(err, &response) {
		return response.StatusCode >= 400 && response.StatusCode < 500 && response.StatusCode != 408
	}
	return false
}

// Reconcile never sends a DELETE or polls an operation, including legacy tokens.
// The template watch handles requests and adopts legacy operations durably.
func (r *DeploymentResourceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	child := &radappiov1alpha3.DeploymentResource{}
	reader := r.APIReader
	if reader == nil {
		reader = r.Client
	}
	if err := reader.Get(ctx, req.NamespacedName, child); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if child.DeletionTimestamp != nil {
		ref := metav1.GetControllerOf(child)
		if ref != nil && ref.Kind == deploymentTemplateKind && ref.APIVersion == radappiov1alpha3.GroupVersion.String() {
			owner := &radappiov1alpha3.DeploymentTemplate{}
			if err := reader.Get(ctx, client.ObjectKey{Namespace: child.Namespace, Name: ref.Name}, owner); err == nil {
				if owner.UID == ref.UID {
					return ctrl.Result{}, nil
				}
			} else if !apierrors.IsNotFound(err) {
				return ctrl.Result{}, err
			}
		}
		// An orphan is not authorized to delete an external resource.
		if controllerutil.RemoveFinalizer(child, DeploymentResourceFinalizer) {
			r.EventRecorder.Event(child, corev1.EventTypeNormal, EventDeploymentResourceDeleteSkipped, "Skipping output deletion for an orphaned inventory tracker")
			return ctrl.Result{}, r.Client.Update(ctx, child)
		}
		return ctrl.Result{}, nil
	}
	if child.Status.Operation != nil || child.Status.Phrase == radappiov1alpha3.DeploymentResourcePhraseDeleting ||
		child.Status.Phrase == radappiov1alpha3.DeploymentResourcePhraseFailed {
		// Preserve legacy evidence until the coordinator has adopted it.
		return ctrl.Result{}, nil
	}
	if child.Status.Phrase == radappiov1alpha3.DeploymentResourcePhraseReady && child.Status.Id == child.Spec.Id {
		return ctrl.Result{}, nil
	}
	child.Status.Phrase = radappiov1alpha3.DeploymentResourcePhraseReady
	child.Status.Id = child.Spec.Id
	return ctrl.Result{}, r.Client.Status().Update(ctx, child)
}

func (r *DeploymentResourceReconciler) deleteKubernetesOutput(ctx context.Context, trackerNamespace string, id resources.ID) (bool, error) {
	group, kind, namespace, name := resources_kubernetes.ToParts(id)
	mapping, err := r.Client.RESTMapper().RESTMapping(schema.GroupKind{Group: group, Kind: kind})
	refreshed := false
	if err != nil {
		if !meta.IsNoMatchError(err) || r.Discovery == nil {
			return false, fmt.Errorf("failed to resolve Kubernetes output resource %q: %w", id.String(), err)
		}
		mapping, err = r.freshKubernetesMapping(group, kind)
		if err != nil {
			return false, err
		}
		refreshed = true
	}
	for {
		if mapping == nil {
			return true, nil
		}
		resource := &unstructured.Unstructured{}
		resource.SetGroupVersionKind(mapping.GroupVersionKind)
		resource.SetNamespace(namespace)
		resource.SetName(name)
		err = r.Client.Delete(ctx, resource, client.PropagationPolicy(metav1.DeletePropagationBackground))
		if apierrors.IsNotFound(err) {
			if !refreshed && r.Discovery != nil {
				mapping, err = r.freshKubernetesMapping(group, kind)
				if err != nil {
					return false, err
				}
				refreshed = true
				continue
			}
			return true, nil
		}
		if err != nil {
			if !requestRejected(err) {
				err = outputDeleteUncertainError{err}
			}
			return false, fmt.Errorf("failed to delete Kubernetes output resource %q: %w", id.String(), err)
		}
		if group == "" && kind == "Namespace" && name == trackerNamespace {
			if err := r.Client.Get(ctx, client.ObjectKeyFromObject(resource), resource); err != nil {
				if apierrors.IsNotFound(err) {
					return true, nil
				}
				return false, fmt.Errorf("failed to check output namespace %q: %w", name, err)
			}
			if resource.GetDeletionTimestamp() != nil {
				return true, nil
			}
		}
		return false, nil
	}
}

func (r *DeploymentResourceReconciler) freshKubernetesMapping(group, kind string) (*meta.RESTMapping, error) {
	_, lists, err := r.Discovery.ServerGroupsAndResources()
	if err != nil {
		var partial *discovery.ErrGroupDiscoveryFailed
		if !errors.As(err, &partial) || len(partial.Groups) == 0 {
			return nil, fmt.Errorf("failed to refresh Kubernetes output discovery: %w", err)
		}
		for gv := range partial.Groups {
			if gv.Group == group {
				return nil, fmt.Errorf("failed to refresh Kubernetes output discovery: %w", err)
			}
		}
	}
	for _, list := range lists {
		gv, err := schema.ParseGroupVersion(list.GroupVersion)
		if err != nil {
			return nil, fmt.Errorf("invalid discovered group version: %w", err)
		}
		for _, resource := range list.APIResources {
			if gv.Group == group && resource.Kind == kind && !strings.Contains(resource.Name, "/") {
				scope := meta.RESTScopeRoot
				if resource.Namespaced {
					scope = meta.RESTScopeNamespace
				}
				return &meta.RESTMapping{Resource: gv.WithResource(resource.Name), GroupVersionKind: gv.WithKind(kind), Scope: scope}, nil
			}
		}
	}
	return nil, nil
}

func (r *DeploymentResourceReconciler) deleteIsEligible(ctx context.Context, child *radappiov1alpha3.DeploymentResource, ref *metav1.OwnerReference) (bool, error) {
	if ref == nil || ref.Kind != deploymentTemplateKind || ref.APIVersion != radappiov1alpha3.GroupVersion.String() {
		return false, nil
	}
	reader := r.APIReader
	if reader == nil {
		reader = r.Client
	}
	owner := &radappiov1alpha3.DeploymentTemplate{}
	if err := reader.Get(ctx, client.ObjectKey{Namespace: child.Namespace, Name: ref.Name}, owner); err != nil {
		return false, client.IgnoreNotFound(err)
	}
	if owner.UID != ref.UID {
		return false, nil
	}
	if child.Status.ProvisionedResourceID != "" && !sameOutput(child.Status.ProvisionedResourceID, child.Spec.Id) {
		return false, nil
	}
	id, err := resources.ParseResource(child.Spec.Id)
	if err != nil {
		ucplog.FromContextOrDiscard(ctx).Info("Skipping tracker with invalid output identity", "resourceID", child.Spec.Id)
		return false, nil
	}
	if plane := id.FindScope(resources_kubernetes.PlaneTypeKubernetes); plane != "" {
		if !strings.EqualFold(plane, resources_kubernetes.PlaneNameTODO) {
			return false, nil
		}
		if child.Status.ProvisionedResourceID != "" {
			return child.Status.ProvisionedResourceID == id.String(), nil
		}
		return slices.Contains(owner.Status.OutputResources, id.String()), nil
	}
	scope, err := ParseDeploymentScopeFromProviderConfig(owner.Spec.ProviderConfig)
	if err != nil {
		ucplog.FromContextOrDiscard(ctx).Info("Skipping tracker whose deployment scope cannot be established", "error", err.Error())
		return false, nil
	}
	return resourceWithinScope(child.Spec.Id, scope)
}

func resourceWithinScope(resourceID, scope string) (bool, error) {
	id, err := resources.ParseResource(resourceID)
	if err != nil {
		return false, err
	}
	return strings.EqualFold(strings.TrimRight(id.RootScope(), "/"), strings.TrimRight(scope, "/")), nil
}

func isApplicationOrEnvironment(outputID string) (bool, error) {
	id, err := resources.ParseResource(outputID)
	if err != nil {
		return false, err
	}
	return strings.EqualFold(id.Type(), "Applications.Core/applications") || strings.EqualFold(id.Type(), "Applications.Core/environments"), nil
}

// SetupWithManager registers passive tracker status maintenance.
func (r *DeploymentResourceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).For(&radappiov1alpha3.DeploymentResource{}).Complete(r)
}
