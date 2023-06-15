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

	corerp_config "github.com/project-radius/radius/pkg/corerp/config"
	"github.com/project-radius/radius/pkg/kubernetes"
	"github.com/project-radius/radius/pkg/kubeutil"
	"github.com/project-radius/radius/pkg/resourcemodel"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/project-radius/radius/pkg/ucp/ucplog"

	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/informers"
	k8s "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// DefaultCacheResyncInterval is the interval for resyncing informer.
	DefaultCacheResyncInterval = time.Second * time.Duration(10)
)

// NewKubernetesHandler creates Kubernetes Resource handler instance.
func NewKubernetesHandler(client client.Client, clientSet k8s.Interface) ResourceHandler {
	return &kubernetesHandler{
		client:    client,
		clientSet: clientSet,

		deploymentTimeOut:   corerp_config.AsyncCreateOrUpdateContainerTimeout,
		cacheResyncInterval: DefaultCacheResyncInterval,
	}
}

type kubernetesHandler struct {
	client    client.Client
	clientSet k8s.Interface

	deploymentTimeOut   time.Duration
	cacheResyncInterval time.Duration
}

// Put creates or updates a Kubernetes resource described in PutOptions.
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
	statusCh := make(chan string, 1)

	ctx, cancel := context.WithTimeout(ctx, handler.deploymentTimeOut)
	// This ensures that the informer is stopped when this function is returned.
	defer cancel()

	err := handler.startDeploymentInformer(ctx, item, doneCh, statusCh)
	if err != nil {
		logger.Error(err, "failed to start deployment informer")
		return err
	}

	// Get the latest status for the deployment.
	lastStatus := fmt.Sprintf("unknown status, name: %s, namespace %s", item.GetName(), item.GetNamespace())

	for {
		select {
		case <-ctx.Done():
			// TODO: Deployment doesn't describe the detail of POD failures, so we should get the errors from POD - https://github.com/project-radius/radius/issues/5686
			err := fmt.Errorf("deployment has timed out with the status: %s", lastStatus)
			logger.Error(err, "Kubernetes handler failed")
			return err

		case <-doneCh:
			logger.Info(fmt.Sprintf("Marking deployment %s in namespace %s as complete", item.GetName(), item.GetNamespace()))
			return nil

		case status := <-statusCh:
			lastStatus = status
		}
	}
}

func (handler *kubernetesHandler) startDeploymentInformer(ctx context.Context, item client.Object, doneCh chan<- bool, statusCh chan<- string) error {
	informers := informers.NewSharedInformerFactoryWithOptions(handler.clientSet, handler.cacheResyncInterval, informers.WithNamespace(item.GetNamespace()))
	deploymentInformer := informers.Apps().V1().Deployments().Informer()
	handlers := cache.ResourceEventHandlerFuncs{
		AddFunc: func(new_obj any) {
			obj := new_obj.(*v1.Deployment)
			// There might be parallel deployments in progress, we need to make sure we are watching the right one
			if obj.Name != item.GetName() {
				return
			}
			handler.checkDeploymentStatus(ctx, obj, doneCh, statusCh)
		},
		UpdateFunc: func(old_obj, new_obj any) {
			old := old_obj.(*v1.Deployment)
			new := new_obj.(*v1.Deployment)

			// There might be parallel deployments in progress, we need to make sure we are watching the right one
			if new.Name != item.GetName() {
				return
			}

			if old.ResourceVersion != new.ResourceVersion {
				handler.checkDeploymentStatus(ctx, new, doneCh, statusCh)
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

func (handler *kubernetesHandler) checkDeploymentStatus(ctx context.Context, obj *v1.Deployment, doneCh chan<- bool, statusCh chan<- string) {
	logger := ucplog.FromContextOrDiscard(ctx)
	cond := obj.Status.Conditions
	for i, c := range cond {
		if i == len(cond)-1 {
			statusCh <- fmt.Sprintf("%s (%s), name: %s, namespace: %s", c.Message, c.Reason, obj.GetName(), obj.GetNamespace())
		}

		// check for complete deployment condition
		// Reference https://kubernetes.io/docs/concepts/workloads/controllers/deployment/#complete-deployment
		if c.Type == v1.DeploymentProgressing && c.Status == corev1.ConditionTrue && strings.EqualFold(c.Reason, "NewReplicaSetAvailable") {
			// ObservedGeneration should be updated to latest generation to avoid stale replicas
			if obj.Status.ObservedGeneration >= obj.Generation {
				logger.Info(fmt.Sprintf("Deployment %s in namespace %s is ready. Observed generation: %d, Generation: %d", obj.Name, obj.Namespace, obj.Status.ObservedGeneration, obj.Generation))
				doneCh <- true
				return
			}
		}
	}
}

// Delete deletes a Kubernetes resource.
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
