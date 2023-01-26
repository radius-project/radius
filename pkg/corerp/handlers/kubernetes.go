// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handlers

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/project-radius/radius/pkg/kubernetes"
	"github.com/project-radius/radius/pkg/resourcemodel"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
	"github.com/project-radius/radius/pkg/ucp/store"

	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	k8s "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	DefaultCacheResyncInterval   = time.Minute * 10
	DefaultDeploymentTimeout     = time.Minute * 5
	DefaultTestDeploymentTimeout = time.Second * 5
)

var TestHook bool

func NewKubernetesHandler(client client.Client, clientSet k8s.Interface) ResourceHandler {
	return &kubernetesHandler{client: client, clientSet: clientSet}
}

type kubernetesHandler struct {
	client    client.Client
	clientSet k8s.Interface
}

func (handler *kubernetesHandler) Put(ctx context.Context, options *PutOptions) (map[string]string, error) {
	item, err := convertToUnstructured(*options.Resource)
	if err != nil {
		return nil, err
	}

	// For a Kubernetes resource we only need to store the ObjectMeta and TypeMeta data
	properties := map[string]string{
		KubernetesKindKey:       item.GetKind(),
		KubernetesAPIVersionKey: item.GetAPIVersion(),
		KubernetesNamespaceKey:  item.GetNamespace(),
		ResourceName:            item.GetName(),
	}

	err = handler.PatchNamespace(ctx, item.GetNamespace())
	if err != nil {
		return nil, err
	}

	if options.Resource.Deployed {
		return properties, nil
	}

	err = handler.client.Patch(ctx, &item, client.Apply, &client.PatchOptions{FieldManager: kubernetes.FieldManager})
	if err != nil {
		return nil, err
	}

	options.Resource.Identity = resourcemodel.ResourceIdentity{
		ResourceType: &resourcemodel.ResourceType{
			Type:     options.Resource.ResourceType.Type,
			Provider: resourcemodel.ProviderKubernetes,
		},
		Data: resourcemodel.KubernetesIdentity{
			Name:       item.GetName(),
			Namespace:  item.GetNamespace(),
			Kind:       item.GetKind(),
			APIVersion: item.GetAPIVersion(),
		},
	}

	if !strings.EqualFold(item.GetKind(), "Deployment") {
		return properties, nil // only checking further the Deployment output resource status
	}

	timeout := DefaultDeploymentTimeout

	// Setting the lower limits for testing when TestHook is enabled
	if TestHook {
		timeout = DefaultTestDeploymentTimeout
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	readinessCh := make(chan bool, 1)
	watchErrorCh := make(chan error, 1)
	go func() {
		handler.watchUntilReady(ctx, &item, readinessCh, watchErrorCh)
	}()

	select {
	case <-ctx.Done():
		// Get the final deployment status
		dep, err := handler.clientSet.AppsV1().Deployments(item.GetNamespace()).Get(ctx, item.GetName(), metav1.GetOptions{})
		if err != nil {
			return nil, fmt.Errorf("deployment timed out, name: %s, namespace %s, error occured while fetching latest status: %w", item.GetName(), item.GetNamespace(), err)
		}
		// Now get the latest available observation of deployment current state
		// note that there can be a race condition here, by the time it fetches the latest status, deployment might be succeeded
		status := v1.DeploymentCondition{}
		if len(dep.Status.Conditions) >= 1 {
			status = dep.Status.Conditions[len(dep.Status.Conditions)-1]
		}

		return nil, fmt.Errorf("deployment timed out, name: %s, namespace %s, status: %s, reason: %s", item.GetName(), item.GetNamespace(), status.Message, status.Reason)
	case <-readinessCh:
		return properties, nil
	case <-watchErrorCh:
		return nil, err
	}
}

func (handler *kubernetesHandler) PatchNamespace(ctx context.Context, namespace string) error {
	// Ensure that the namespace exists that we're able to operate upon.
	ns := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "Namespace",
			"metadata": map[string]any{
				"name": namespace,
				"labels": map[string]any{
					kubernetes.LabelManagedBy: kubernetes.LabelManagedByRadiusRP,
				},
			},
		},
	}

	err := handler.client.Patch(ctx, ns, client.Apply, &client.PatchOptions{FieldManager: kubernetes.FieldManager})
	if err != nil {
		// we consider this fatal - without a namespace we won't be able to apply anything else
		return fmt.Errorf("error applying namespace: %w", err)
	}

	return nil
}

func (handler *kubernetesHandler) Delete(ctx context.Context, options *DeleteOptions) error {
	identity := &resourcemodel.KubernetesIdentity{}
	if err := store.DecodeMap(options.Resource.Identity.Data, identity); err != nil {
		return err
	}

	item := unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": identity.APIVersion,
			"kind":       identity.Kind,
			"metadata": map[string]any{
				"namespace": identity.Namespace,
				"name":      identity.Name,
			},
		},
	}

	return client.IgnoreNotFound(handler.client.Delete(ctx, &item))
}

func convertToUnstructured(resource rpv1.OutputResource) (unstructured.Unstructured, error) {
	if resource.ResourceType.Provider != resourcemodel.ProviderKubernetes {
		return unstructured.Unstructured{}, errors.New("wrong resource type")
	}

	obj, ok := resource.Resource.(runtime.Object)
	if !ok {
		return unstructured.Unstructured{}, errors.New("inner type was not a runtime.Object")
	}

	c, err := runtime.DefaultUnstructuredConverter.ToUnstructured(resource.Resource)
	if err != nil {
		return unstructured.Unstructured{}, fmt.Errorf("could not convert object %v to unstructured: %w", obj.GetObjectKind(), err)
	}

	return unstructured.Unstructured{Object: c}, nil
}

func (handler *kubernetesHandler) watchUntilReady(ctx context.Context, item client.Object, readinessCh chan<- bool, watchErrorCh chan<- error) {
	informerFactory := informers.NewSharedInformerFactoryWithOptions(handler.clientSet, DefaultCacheResyncInterval, informers.WithNamespace(item.GetNamespace()))

	deploymentInformer := informerFactory.Apps().V1().Deployments().Informer()
	handlers := cache.ResourceEventHandlerFuncs{
		AddFunc: func(new_obj any) {
			obj, ok := new_obj.(*v1.Deployment)
			if !ok {
				watchErrorCh <- errors.New("deployment object is not of appsv1.Deployment type")
			}

			handler.watchUntilDeploymentReady(ctx, obj, readinessCh)
		},
		UpdateFunc: func(old_obj, new_obj any) {
			old, ok := old_obj.(*v1.Deployment)
			if !ok {
				watchErrorCh <- errors.New("old deployment object is not of appsv1.Deployment type")
			}
			new, ok := new_obj.(*v1.Deployment)
			if !ok {
				watchErrorCh <- errors.New("new deployment object not of appsv1.Deployment type")
			}
			if old.ResourceVersion != new.ResourceVersion {
				handler.watchUntilDeploymentReady(ctx, new, readinessCh)
			}
		},
		DeleteFunc: func(obj any) {
			// no-op here
		},
	}

	deploymentInformer.AddEventHandler(handlers)
	// Start the informer
	informerFactory.Start(wait.NeverStop)
	informerFactory.WaitForCacheSync(wait.NeverStop)
}

func (handler *kubernetesHandler) watchUntilDeploymentReady(ctx context.Context, obj *v1.Deployment, readinessCh chan<- bool) {
	for _, c := range obj.Status.Conditions {
		// check for complete deployment condition
		// Reference https://kubernetes.io/docs/concepts/workloads/controllers/deployment/#complete-deployment
		if c.Type == v1.DeploymentProgressing && c.Status == corev1.ConditionTrue && strings.EqualFold(c.Reason, "NewReplicaSetAvailable") {
			// ObservedGeneration should be updated to latest generation to avoid stale replicas
			if obj.Status.ObservedGeneration >= obj.Generation {
				readinessCh <- true
			}
		}
	}
}
