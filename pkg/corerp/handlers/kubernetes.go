// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handlers

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/project-radius/radius/pkg/kubernetes"
	"github.com/project-radius/radius/pkg/providers"
	"github.com/project-radius/radius/pkg/radlogger"
	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/resourcemodel"
	v1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	k8s "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func NewKubernetesHandler(k8s client.Client, k8sClientSet k8s.Interface) ResourceHandler {
	informerFactory := informers.NewSharedInformerFactory(k8sClientSet, 15*time.Second)
	// Start the informer
	informerFactory.Start(wait.NeverStop)
	informerFactory.WaitForCacheSync(wait.NeverStop)
	watchCh := make(chan bool)
	return &kubernetesHandler{k8s: k8s, k8sClientSet: k8sClientSet, informerFactory: informerFactory, watchCh: watchCh}
}

type kubernetesHandler struct {
	k8s             client.Client
	k8sClientSet    k8s.Interface
	informerFactory informers.SharedInformerFactory
	// watch chanel for deployment readiness
	watchCh chan bool
}

func (handler *kubernetesHandler) Put(ctx context.Context, resource *outputresource.OutputResource) error {
	item, err := convertToUnstructured(*resource)
	if err != nil {
		return err
	}

	err = handler.PatchNamespace(ctx, item.GetNamespace())
	if err != nil {
		return err
	}

	if resource.Deployed {
		return nil
	}

	err = handler.k8s.Patch(ctx, &item, client.Apply, &client.PatchOptions{FieldManager: kubernetes.FieldManager})
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, 1*time.Minute) // 1 minute deployment readiness timeout
	defer cancel()

	handler.watchUntilReady(ctx, &item)

	select {
	case <-ctx.Done():
		return fmt.Errorf("deployment timed out, deployment: %s in namespace %s is not ready", item.GetName(), item.GetNamespace())
	case <-handler.watchCh:
		return nil
	}
}

func (handler *kubernetesHandler) GetResourceIdentity(ctx context.Context, resource outputresource.OutputResource) (resourcemodel.ResourceIdentity, error) {
	item, err := convertToUnstructured(resource)
	if err != nil {
		return resourcemodel.ResourceIdentity{}, err
	}

	identity := resourcemodel.ResourceIdentity{
		ResourceType: &resourcemodel.ResourceType{
			Type:     resource.ResourceType.Type,
			Provider: providers.ProviderKubernetes,
		},
		Data: resourcemodel.KubernetesIdentity{
			Name:       item.GetName(),
			Namespace:  item.GetNamespace(),
			Kind:       item.GetKind(),
			APIVersion: item.GetAPIVersion(),
		},
	}

	return identity, err
}

func (handler *kubernetesHandler) GetResourceNativeIdentityKeyProperties(ctx context.Context, resource outputresource.OutputResource) (map[string]string, error) {
	item, err := convertToUnstructured(resource)
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

	return properties, err
}

func (handler *kubernetesHandler) PatchNamespace(ctx context.Context, namespace string) error {
	// Ensure that the namespace exists that we're able to operate upon.
	ns := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Namespace",
			"metadata": map[string]interface{}{
				"name": namespace,
				"labels": map[string]interface{}{
					kubernetes.LabelManagedBy: kubernetes.LabelManagedByRadiusRP,
				},
			},
		},
	}

	err := handler.k8s.Patch(ctx, ns, client.Apply, &client.PatchOptions{FieldManager: kubernetes.FieldManager})
	if err != nil {
		// we consider this fatal - without a namespace we won't be able to apply anything else
		return fmt.Errorf("error applying namespace: %w", err)
	}

	return nil
}

func (handler *kubernetesHandler) Delete(ctx context.Context, resource outputresource.OutputResource) error {
	identity := resource.Identity.Data.(resourcemodel.KubernetesIdentity)
	item := unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": identity.APIVersion,
			"kind":       identity.Kind,
			"metadata": map[string]interface{}{
				"namespace": identity.Namespace,
				"name":      identity.Name,
			},
		},
	}

	return client.IgnoreNotFound(handler.k8s.Delete(ctx, &item))
}

func convertToUnstructured(resource outputresource.OutputResource) (unstructured.Unstructured, error) {
	if resource.ResourceType.Provider != providers.ProviderKubernetes {
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

func (handler *kubernetesHandler) watchUntilReady(ctx context.Context, item client.Object) {
	logger := radlogger.GetLogger(ctx)
	logger.Info(fmt.Sprintf("Watching for deployment changes"))
	deploymentInformer := handler.informerFactory.Apps().V1().Deployments()
	handlers := cache.ResourceEventHandlerFuncs{
		AddFunc: func(new_obj interface{}) {
			logger.Info(fmt.Sprintf("New deployment is added"))
			obj, ok := new_obj.(*v1.Deployment)
			if !ok {
				e := errors.New("not a appsv1.Deployment type")
				logger.Error(e, "expected obj to be a *appsv1.Deployment, got %T", obj)
			}
			handler.watchUntilDeploymentReady(ctx, obj)
		},
		UpdateFunc: func(old_obj, new_obj interface{}) {
			logger.Info(fmt.Sprintf("Deployment is updated"))
			old, ok := old_obj.(*v1.Deployment)
			if !ok {
				e := errors.New("not a appsv1.Deployment type")
				logger.Error(e, "expected old obj to be a *appsv1.Deployment, got %T", old_obj)
			}
			new, ok := new_obj.(*v1.Deployment)
			if !ok {
				e := errors.New("not a appsv1.Deployment type")
				logger.Error(e, "expected new obj to be a *appsv1.Deployment, got %T", new_obj)
			}
			if old.ResourceVersion != new.ResourceVersion {
				handler.watchUntilDeploymentReady(ctx, new)
			}
		},
		DeleteFunc: func(obj interface{}) {
			logger.Info(fmt.Sprintf("Deployment is deleted"))
			// no-op here
		},
	}

	deploymentInformer.Informer().AddEventHandler(handlers)
}

func (handler *kubernetesHandler) watchUntilDeploymentReady(ctx context.Context, obj *v1.Deployment) {
	for _, c := range obj.Status.Conditions {
		// check for complete deployment condition
		// Reference https://kubernetes.io/docs/concepts/workloads/controllers/deployment/#complete-deployment
		if c.Type == v1.DeploymentProgressing && c.Status == "True" && c.Reason == "NewReplicaSetAvailable" {
			handler.watchCh <- true
		}
	}
}
