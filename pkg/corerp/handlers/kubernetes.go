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

	"github.com/radius-project/radius/pkg/kubernetes"
	"github.com/radius-project/radius/pkg/kubeutil"
	"github.com/radius-project/radius/pkg/resourcemodel"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	"github.com/radius-project/radius/pkg/ucp/resources"
	resources_kubernetes "github.com/radius-project/radius/pkg/ucp/resources/kubernetes"
	"github.com/radius-project/radius/pkg/ucp/ucplog"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/informers"
	k8s "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// DefaultCacheResyncInterval is the interval for resyncing informer.
	DefaultCacheResyncInterval = time.Second * time.Duration(30)
)

// Create an interface for deployment waiter and http proxy waiter
type ResourceWaiter interface {
	addDynamicEventHandler(ctx context.Context, informerFactory dynamicinformer.DynamicSharedInformerFactory, informer cache.SharedIndexInformer, item client.Object, doneCh chan<- error)
	addEventHandler(ctx context.Context, informerFactory informers.SharedInformerFactory, informer cache.SharedIndexInformer, item client.Object, doneCh chan<- error)
	waitUntilReady(ctx context.Context, item client.Object) error
}

// NewKubernetesHandler creates a new KubernetesHandler which is used to handle Kubernetes resources.
func NewKubernetesHandler(client client.Client, clientSet k8s.Interface, discoveryClient discovery.ServerResourcesInterface, dynamicClientSet dynamic.Interface) ResourceHandler {
	return &kubernetesHandler{
		client:             client,
		k8sDiscoveryClient: discoveryClient,
		httpProxyWaiter:    NewHTTPProxyWaiter(dynamicClientSet),
		deploymentWaiter:   NewDeploymentWaiter(clientSet),
	}
}

type kubernetesHandler struct {
	client client.Client
	// k8sDiscoveryClient is the Kubernetes client to used for API version lookups on Kubernetes resources. Override this for testing.
	k8sDiscoveryClient discovery.ServerResourcesInterface
	httpProxyWaiter    ResourceWaiter
	deploymentWaiter   ResourceWaiter
}

// Put stores the Kubernetes resource in the cluster and returns the properties of the resource. If the resource is a
// deployment, it also waits until the deployment is ready.
func (handler *kubernetesHandler) Put(ctx context.Context, options *PutOptions) (map[string]string, error) {
	logger := ucplog.FromContextOrDiscard(ctx)
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

	err = handler.client.Patch(ctx, &item, client.Apply, &client.PatchOptions{FieldManager: kubernetes.FieldManager})
	if err != nil {
		return nil, err
	}

	groupVersion, err := schema.ParseGroupVersion(item.GetAPIVersion())
	if err != nil {
		return nil, err
	}

	id := resources_kubernetes.IDFromParts(
		resources_kubernetes.PlaneNameTODO,
		groupVersion.Group,
		item.GetKind(),
		item.GetNamespace(),
		item.GetName())
	options.Resource.ID = id

	// Monitor the created or updated resource until it is ready.
	switch strings.ToLower(item.GetKind()) {
	case "deployment":
		// Monitor the deployment until it is ready.
		err = handler.deploymentWaiter.waitUntilReady(ctx, &item)
		if err != nil {
			return nil, err
		}
		logger.Info(fmt.Sprintf("Deployment %s in namespace %s is ready", item.GetName(), item.GetNamespace()))
		return properties, nil
	case "httpproxy":
		err = handler.httpProxyWaiter.waitUntilReady(ctx, &item)
		if err != nil {
			return nil, err
		}
		logger.Info(fmt.Sprintf("HTTP Proxy %s in namespace %s is ready", item.GetName(), item.GetNamespace()))
		return properties, nil
	default:
		// We do not monitor the other resource types.
		return properties, nil
	}
}

// Delete decodes the identity data from the DeleteOptions, creates an unstructured object from the identity data,
// and then attempts to delete the object from the Kubernetes cluster, returning an error if one occurs.
func (handler *kubernetesHandler) Delete(ctx context.Context, options *DeleteOptions) error {
	apiVersion, err := handler.lookupKubernetesAPIVersion(ctx, options.Resource.ID)
	if err != nil {
		return err
	}

	group, kind, namespace, name := resources_kubernetes.ToParts(options.Resource.ID)
	item := unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": schema.GroupVersion{Group: group, Version: apiVersion}.String(),
			"kind":       kind,
			"metadata": map[string]any{
				"namespace": namespace,
				"name":      name,
			},
		},
	}

	return client.IgnoreNotFound(handler.client.Delete(ctx, &item))
}

func (handler *kubernetesHandler) lookupKubernetesAPIVersion(ctx context.Context, id resources.ID) (string, error) {
	group, kind, namespace, _ := resources_kubernetes.ToParts(id)
	var resourceLists []*metav1.APIResourceList
	var err error
	if namespace == "" {
		resourceLists, err = handler.k8sDiscoveryClient.ServerPreferredResources()
		if err != nil {
			return "", fmt.Errorf("could not find API version for type %q: %w", id.Type(), err)
		}
	} else {
		resourceLists, err = handler.k8sDiscoveryClient.ServerPreferredNamespacedResources()
		if err != nil {
			return "", fmt.Errorf("could not find API version for type %q: %w", id.Type(), err)
		}
	}

	for _, resourceList := range resourceLists {
		// We know the group but not the version. This will give us the the list of resources and their preferred versions.
		gv, err := schema.ParseGroupVersion(resourceList.GroupVersion)
		if err != nil {
			return "", err
		}

		if group != gv.Group {
			continue
		}

		for _, resource := range resourceList.APIResources {
			if resource.Kind == kind {
				return gv.Version, nil
			}
		}
	}

	return "", fmt.Errorf("could not find API version for type %q, type was not found", id.Type())
}

func convertToUnstructured(resource rpv1.OutputResource) (unstructured.Unstructured, error) {
	obj, ok := resource.CreateResource.Data.(runtime.Object)
	if !ok {
		return unstructured.Unstructured{}, errors.New("inner type was not a runtime.Object")
	}

	resourceType := resource.GetResourceType()
	if resourceType.Provider != resourcemodel.ProviderKubernetes {
		return unstructured.Unstructured{}, fmt.Errorf("invalid resource type provider: %s", resourceType.Provider)
	}

	c, err := runtime.DefaultUnstructuredConverter.ToUnstructured(resource.CreateResource.Data)
	if err != nil {
		return unstructured.Unstructured{}, fmt.Errorf("could not convert object %v to unstructured: %w", obj.GetObjectKind(), err)
	}

	return unstructured.Unstructured{Object: c}, nil
}
