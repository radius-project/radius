/*
Copyright 2023 The Radius Authors.

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

package handlers

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/project-radius/radius/pkg/kubernetes"
	"github.com/project-radius/radius/pkg/kubeutil"
	"github.com/project-radius/radius/pkg/resourcemodel"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/project-radius/radius/pkg/ucp/ucplog"

	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/informers"
	k8s "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// MaxDeploymentTimeout is the max timeout for waiting for a deployment to be ready.
	// Deployment duration should not reach to this timeout since async operation worker will time out context before MaxDeploymentTimeout.
	MaxDeploymentTimeout = time.Minute * time.Duration(10)
	// DefaultCacheResyncInterval is the interval for resyncing informer.
	DefaultCacheResyncInterval = time.Second * time.Duration(30)
)

// # Function Explanation
//
// NewKubernetesHandler creates a new KubernetesHandler which is used to handle Kubernetes resources.
func NewKubernetesHandler(client client.Client, clientSet k8s.Interface) ResourceHandler {
	return &kubernetesHandler{
		client:              client,
		clientSet:           clientSet,
		deploymentTimeOut:   MaxDeploymentTimeout,
		cacheResyncInterval: DefaultCacheResyncInterval,
	}
}

type kubernetesHandler struct {
	client    client.Client
	clientSet k8s.Interface

	deploymentTimeOut   time.Duration
	cacheResyncInterval time.Duration
}

// # Function Explanation
//
// Put stores the Kubernetes resource in the cluster and returns the properties of the resource. If the resource is a
// deployment, it also waits until the deployment is ready.
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

	err = kubeutil.PatchNamespace(ctx, handler.client, item.GetNamespace())
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

	// Monitor the created or updated resource until it is ready.
	switch strings.ToLower(item.GetKind()) {
	case "deployment":
		// Monitor the deployment until it is ready.
		err = handler.waitUntilDeploymentIsReady(ctx, &item)
		if err != nil {
			return nil, err
		}
		return properties, nil
	default:
		// We do not monitor the other resource types.
		return properties, nil
	}
}

func (handler *kubernetesHandler) waitUntilDeploymentIsReady(ctx context.Context, item client.Object) error {
	logger := ucplog.FromContextOrDiscard(ctx)

	doneCh := make(chan bool, 1)
	errCh := make(chan error, 1)

	ctx, cancel := context.WithTimeout(ctx, handler.deploymentTimeOut)
	// This ensures that the informer is stopped when this function is returned.
	defer cancel()

	err := handler.startDeploymentInformer(ctx, item, doneCh, errCh)
	if err != nil {
		logger.Error(err, "failed to start deployment informer")
		return err
	}

	select {
	case <-ctx.Done():
		// Get the final deployment status
		dep, err := handler.clientSet.AppsV1().Deployments(item.GetNamespace()).Get(ctx, item.GetName(), metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("deployment timed out, name: %s, namespace %s, error occured while fetching latest status: %w", item.GetName(), item.GetNamespace(), err)
		}

		// Now get the latest available observation of deployment current state
		// note that there can be a race condition here, by the time it fetches the latest status, deployment might be succeeded
		status := v1.DeploymentCondition{}
		if len(dep.Status.Conditions) > 0 {
			status = dep.Status.Conditions[len(dep.Status.Conditions)-1]
		}
		return fmt.Errorf("deployment timed out, name: %s, namespace %s, status: %s, reason: %s", item.GetName(), item.GetNamespace(), status.Message, status.Reason)

	case <-doneCh:
		logger.Info(fmt.Sprintf("Marking deployment %s in namespace %s as complete", item.GetName(), item.GetNamespace()))
		return nil

	case err := <-errCh:
		return err
	}
}

func (handler *kubernetesHandler) startDeploymentInformer(ctx context.Context, item client.Object, doneCh chan<- bool, errCh chan<- error) error {
	informers := informers.NewSharedInformerFactoryWithOptions(handler.clientSet, handler.cacheResyncInterval, informers.WithNamespace(item.GetNamespace()))
	deploymentInformer := informers.Apps().V1().Deployments().Informer()
	handlers := cache.ResourceEventHandlerFuncs{
		AddFunc: func(new_obj any) {
			obj := new_obj.(*v1.Deployment)
			// There might be parallel deployments in progress, we need to make sure we are watching the right one
			if obj.Name != item.GetName() {
				return
			}
			handler.checkDeploymentStatus(ctx, obj, doneCh)
		},
		UpdateFunc: func(old_obj, new_obj any) {
			old := old_obj.(*v1.Deployment)
			new := new_obj.(*v1.Deployment)

			// There might be parallel deployments in progress, we need to make sure we are watching the right one
			if new.Name != item.GetName() {
				return
			}

			if old.ResourceVersion != new.ResourceVersion {
				handler.checkDeploymentStatus(ctx, new, doneCh)
			}
		},
	}

	deploymentInformer.AddEventHandler(handlers)
	informers.Start(ctx.Done())

	// Wait for the deployment informer's cache to be synced.
	if !cache.WaitForCacheSync(ctx.Done(), deploymentInformer.HasSynced) {
		err := fmt.Errorf("cache sync is failed for deployment informer: name: %s, namespace %s", item.GetName(), item.GetNamespace())
		return err
	}

	return nil
}

func (handler *kubernetesHandler) checkDeploymentStatus(ctx context.Context, obj *v1.Deployment, doneCh chan<- bool) {
	logger := ucplog.FromContextOrDiscard(ctx)
	for _, c := range obj.Status.Conditions {
		// check for complete deployment condition
		// Reference https://kubernetes.io/docs/concepts/workloads/controllers/deployment/#complete-deployment
		if c.Type == v1.DeploymentProgressing && c.Status == corev1.ConditionTrue && strings.EqualFold(c.Reason, "NewReplicaSetAvailable") {
			logger.Info(fmt.Sprintf("Deployment status for deployment: %s in namespace: %s is: %s - %s, Reason: %s", obj.Name, obj.Namespace, c.Type, c.Status, c.Reason))

			// ObservedGeneration should be updated to latest generation to avoid stale replicas
			if obj.Status.ObservedGeneration >= obj.Generation {
				logger.Info(fmt.Sprintf("Deployment %s in namespace %s is ready. Observed generation: %d, Generation: %d", obj.Name, obj.Namespace, obj.Status.ObservedGeneration, obj.Generation))
				doneCh <- true
				return
			}
		}
	}
}

// # Function Explanation
//
// Delete decodes the identity data from the DeleteOptions, creates an unstructured object from the identity data,
// and then attempts to delete the object from the Kubernetes cluster, returning an error if one occurs.
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
		return unstructured.Unstructured{}, fmt.Errorf("invalid resource type provider: %s", resource.ResourceType.Provider)
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
