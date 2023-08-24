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

	contourv1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/radius-project/radius/pkg/kubernetes"
	"github.com/radius-project/radius/pkg/kubeutil"
	"github.com/radius-project/radius/pkg/resourcemodel"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	"github.com/radius-project/radius/pkg/ucp/resources"
	resources_kubernetes "github.com/radius-project/radius/pkg/ucp/resources/kubernetes"
	"github.com/radius-project/radius/pkg/ucp/ucplog"

	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
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
	// MaxDeploymentTimeout is the max timeout for waiting for a deployment to be ready.
	// Deployment duration should not reach to this timeout since async operation worker will time out context before MaxDeploymentTimeout.
	MaxDeploymentTimeout          = time.Minute * time.Duration(10)
	MaxHTTPProxyDeploymentTimeout = time.Minute * time.Duration(10)
	// DefaultCacheResyncInterval is the interval for resyncing informer.
	DefaultCacheResyncInterval = time.Second * time.Duration(30)
	HTTPProxyConditionValid    = "Valid"
	HTTPProxyStatusInvalid     = "invalid"
	HTTPProxyStatusValid       = "valid"
)

// NewKubernetesHandler creates a new KubernetesHandler which is used to handle Kubernetes resources.
func NewKubernetesHandler(client client.Client, clientSet k8s.Interface, discoveryClient discovery.ServerResourcesInterface, dynamicClientSet dynamic.Interface) ResourceHandler {
	return &kubernetesHandler{
		client:                     client,
		clientSet:                  clientSet,
		k8sDiscoveryClient:         discoveryClient,
		dynamicClientSet:           dynamicClientSet,
		deploymentTimeOut:          MaxDeploymentTimeout,
		httpProxyDeploymentTimeout: MaxHTTPProxyDeploymentTimeout,
		cacheResyncInterval:        DefaultCacheResyncInterval,
	}
}

type kubernetesHandler struct {
	client    client.Client
	clientSet k8s.Interface
	// k8sDiscoveryClient is the Kubernetes client to used for API version lookups on Kubernetes resources. Override this for testing.
	k8sDiscoveryClient discovery.ServerResourcesInterface
	dynamicClientSet   dynamic.Interface

	deploymentTimeOut          time.Duration
	httpProxyDeploymentTimeout time.Duration
	cacheResyncInterval        time.Duration
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
		err = handler.waitUntilDeploymentIsReady(ctx, &item)
		if err != nil {
			return nil, err
		}
		logger.Info(fmt.Sprintf("Deployment %s in namespace %s is ready", item.GetName(), item.GetNamespace()))
		return properties, nil
	case "httpproxy":
		err = handler.waitUntilHTTPProxyIsReady(ctx, &item)
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

func (handler *kubernetesHandler) waitUntilDeploymentIsReady(ctx context.Context, item client.Object) error {
	logger := ucplog.FromContextOrDiscard(ctx)

	// When the deployment is done, an error nil will be sent
	// In case of an error, the error will be sent
	doneCh := make(chan error, 1)

	ctx, cancel := context.WithTimeout(ctx, handler.deploymentTimeOut)
	// This ensures that the informer is stopped when this function is returned.
	defer cancel()

	err := handler.startInformers(ctx, item, doneCh)
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

	case err := <-doneCh:
		if err == nil {
			logger.Info(fmt.Sprintf("Marking deployment %s in namespace %s as complete", item.GetName(), item.GetNamespace()))
		}
		return err
	}
}

func (handler *kubernetesHandler) addEventHandler(ctx context.Context, informerFactory informers.SharedInformerFactory, informer cache.SharedIndexInformer, item client.Object, doneCh chan<- error) {
	logger := ucplog.FromContextOrDiscard(ctx)

	_, err := informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj any) {
			handler.checkDeploymentStatus(ctx, informerFactory, item, doneCh)
		},
		UpdateFunc: func(_, newObj any) {
			handler.checkDeploymentStatus(ctx, informerFactory, item, doneCh)
		},
	})

	if err != nil {
		logger.Error(err, "failed to add event handler")
	}
}

func (handler *kubernetesHandler) addHTTPProxyEventHandler(ctx context.Context, informerFactory dynamicinformer.DynamicSharedInformerFactory, informer cache.SharedIndexInformer, item client.Object, doneCh chan<- error) {
	logger := ucplog.FromContextOrDiscard(ctx)

	_, err := informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj any) {
			handler.checkHTTPProxyStatus(ctx, informerFactory, item, doneCh)
		},
		UpdateFunc: func(_, newObj any) {
			handler.checkHTTPProxyStatus(ctx, informerFactory, item, doneCh)
		},
	})

	if err != nil {
		logger.Error(err, "failed to add event handler")
	}
}

func (handler *kubernetesHandler) startInformers(ctx context.Context, item client.Object, doneCh chan<- error) error {
	logger := ucplog.FromContextOrDiscard(ctx)

	informerFactory := informers.NewSharedInformerFactoryWithOptions(handler.clientSet, handler.cacheResyncInterval, informers.WithNamespace(item.GetNamespace()))
	// Add event handlers to the pod informer
	handler.addEventHandler(ctx, informerFactory, informerFactory.Core().V1().Pods().Informer(), item, doneCh)

	// Add event handlers to the deployment informer
	handler.addEventHandler(ctx, informerFactory, informerFactory.Apps().V1().Deployments().Informer(), item, doneCh)

	// Add event handlers to the replicaset informer
	handler.addEventHandler(ctx, informerFactory, informerFactory.Apps().V1().ReplicaSets().Informer(), item, doneCh)

	// Start the informers
	informerFactory.Start(ctx.Done())

	// Wait for the deployment and pod informer's cache to be synced.
	informerFactory.WaitForCacheSync(ctx.Done())

	logger.Info(fmt.Sprintf("Informers started and caches synced for deployment: %s in namespace: %s", item.GetName(), item.GetNamespace()))
	return nil
}

// HTTPProxyInformer returns the HTTPProxy informer.
func HTTPProxyInformer(dynamicInformerFactory dynamicinformer.DynamicSharedInformerFactory) informers.GenericInformer {
	return dynamicInformerFactory.ForResource(contourv1.HTTPProxyGVR)
}

// Check if all the pods in the deployment are ready
func (handler *kubernetesHandler) checkDeploymentStatus(ctx context.Context, informerFactory informers.SharedInformerFactory, item client.Object, doneCh chan<- error) bool {
	logger := ucplog.FromContextOrDiscard(ctx).WithValues("deploymentName", item.GetName(), "namespace", item.GetNamespace())

	// Get the deployment
	deployment, err := informerFactory.Apps().V1().Deployments().Lister().Deployments(item.GetNamespace()).Get(item.GetName())
	if err != nil {
		logger.Info("Unable to find deployment")
		return false
	}

	deploymentReplicaSet := handler.getCurrentReplicaSetForDeployment(ctx, informerFactory, deployment)
	if deploymentReplicaSet == nil {
		logger.Info("Unable to find replica set for deployment")
		return false
	}

	allReady := handler.checkAllPodsReady(ctx, informerFactory, deployment, deploymentReplicaSet, doneCh)
	if !allReady {
		logger.Info("All pods are not ready yet for deployment")
		return false
	}

	// Check if the deployment is ready
	if deployment.Status.ObservedGeneration != deployment.Generation {
		logger.Info(fmt.Sprintf("Deployment status is not ready: Observed generation: %d, Generation: %d, Deployment Replicaset: %s", deployment.Status.ObservedGeneration, deployment.Generation, deploymentReplicaSet.Name))
		return false
	}

	// ObservedGeneration should be updated to latest generation to avoid stale replicas
	for _, c := range deployment.Status.Conditions {
		// check for complete deployment condition
		// Reference https://kubernetes.io/docs/concepts/workloads/controllers/deployment/#complete-deployment
		if c.Type == v1.DeploymentProgressing && c.Status == corev1.ConditionTrue && strings.EqualFold(c.Reason, "NewReplicaSetAvailable") {
			logger.Info(fmt.Sprintf("Deployment is ready. Observed generation: %d, Generation: %d, Deployment Replicaset: %s", deployment.Status.ObservedGeneration, deployment.Generation, deploymentReplicaSet.Name))
			doneCh <- nil
			return true
		} else {
			logger.Info(fmt.Sprintf("Deployment status is: %s - %s, Reason: %s, Deployment replicaset: %s", c.Type, c.Status, c.Reason, deploymentReplicaSet.Name))
		}
	}
	return false
}

func (handler *kubernetesHandler) waitUntilHTTPProxyIsReady(ctx context.Context, obj client.Object) error {
	logger := ucplog.FromContextOrDiscard(ctx).WithValues("httpProxyName", obj.GetName(), "namespace", obj.GetNamespace())

	doneCh := make(chan error, 1)

	ctx, cancel := context.WithTimeout(ctx, handler.httpProxyDeploymentTimeout)
	// This ensures that the informer is stopped when this function is returned.
	defer cancel()

	// Create dynamic informer for HTTPProxy
	dynamicInformerFactory := dynamicinformer.NewFilteredDynamicSharedInformerFactory(handler.dynamicClientSet, 0, obj.GetNamespace(), nil)
	httpProxyInformer := dynamicInformerFactory.ForResource(contourv1.HTTPProxyGVR)
	// Add event handlers to the http proxy informer
	handler.addHTTPProxyEventHandler(ctx, dynamicInformerFactory, httpProxyInformer.Informer(), obj, doneCh)

	// Start the informers
	dynamicInformerFactory.Start(ctx.Done())

	// Wait for the cache to be synced.
	dynamicInformerFactory.WaitForCacheSync(ctx.Done())

	select {
	case <-ctx.Done():
		// Get the final status
		proxy, err := httpProxyInformer.Lister().Get(obj.GetName())

		if err != nil {
			return fmt.Errorf("proxy deployment timed out, name: %s, namespace %s, error occured while fetching latest status: %w", obj.GetName(), obj.GetNamespace(), err)
		}

		p := contourv1.HTTPProxy{}
		err = runtime.DefaultUnstructuredConverter.FromUnstructured(proxy.(*unstructured.Unstructured).Object, &p)
		if err != nil {
			return fmt.Errorf("proxy deployment timed out, name: %s, namespace %s, error occured while fetching latest status: %w", obj.GetName(), obj.GetNamespace(), err)
		}

		status := contourv1.DetailedCondition{}
		if len(p.Status.Conditions) > 0 {
			status = p.Status.Conditions[len(p.Status.Conditions)-1]
		}
		return fmt.Errorf("HTTP proxy deployment timed out, name: %s, namespace %s, status: %s, reason: %s", obj.GetName(), obj.GetNamespace(), status.Message, status.Reason)
	case err := <-doneCh:
		if err == nil {
			logger.Info(fmt.Sprintf("Marking HTTP proxy deployment %s in namespace %s as complete", obj.GetName(), obj.GetNamespace()))
		}
		return err
	}
}

func (handler *kubernetesHandler) checkHTTPProxyStatus(ctx context.Context, dynamicInformerFactory dynamicinformer.DynamicSharedInformerFactory, obj client.Object, doneCh chan<- error) bool {
	logger := ucplog.FromContextOrDiscard(ctx).WithValues("httpProxyName", obj.GetName(), "namespace", obj.GetNamespace())
	selector := labels.SelectorFromSet(
		map[string]string{
			kubernetes.LabelManagedBy: kubernetes.LabelManagedByRadiusRP,
			kubernetes.LabelName:      obj.GetName(),
		},
	)
	proxies, err := HTTPProxyInformer(dynamicInformerFactory).Lister().List(selector)
	if err != nil {
		logger.Info(fmt.Sprintf("Unable to list http proxies: %s", err.Error()))
		return false
	}

	for _, proxy := range proxies {
		p := contourv1.HTTPProxy{}
		err = runtime.DefaultUnstructuredConverter.FromUnstructured(proxy.(*unstructured.Unstructured).Object, &p)
		if err != nil {
			logger.Info(fmt.Sprintf("Unable to convert http proxy: %s", err.Error()))
			continue
		}

		if len(p.Spec.Includes) == 0 && len(p.Spec.Routes) > 0 {
			// This is a route HTTP proxy. We will not validate deployment completion for it and return success here
			logger.Info("Not validating the deployment of route HTTP proxy for ", p.Name)
			doneCh <- nil
			return true
		}

		// We will check the status for the root HTTP proxy
		if p.Status.CurrentStatus == HTTPProxyStatusInvalid {
			if strings.Contains(p.Status.Description, "see Errors for details") {
				var msg string
				for _, c := range p.Status.Conditions {
					if c.ObservedGeneration != p.Generation {
						continue
					}
					if c.Type == HTTPProxyConditionValid && c.Status == "False" {
						for _, e := range c.Errors {
							msg += fmt.Sprintf("Error - Type: %s, Status: %s, Reason: %s, Message: %s\n", e.Type, e.Status, e.Reason, e.Message)
						}
					}
				}
				doneCh <- errors.New(msg)
			} else {
				doneCh <- fmt.Errorf("Failed to deploy HTTP proxy. Description: %s", p.Status.Description)
			}
			return false
		} else if p.Status.CurrentStatus == HTTPProxyStatusValid {
			// The HTTPProxy is ready
			doneCh <- nil
			return true
		}
	}
	return false
}

// Gets the current replica set for the deployment
func (handler *kubernetesHandler) getCurrentReplicaSetForDeployment(ctx context.Context, informerFactory informers.SharedInformerFactory, deployment *v1.Deployment) *v1.ReplicaSet {
	if deployment == nil {
		return nil
	}

	logger := ucplog.FromContextOrDiscard(ctx).WithValues("deploymentName", deployment.Name, "namespace", deployment.Namespace)

	// List all replicasets for this deployment
	rl, err := informerFactory.Apps().V1().ReplicaSets().Lister().ReplicaSets(deployment.Namespace).List(labels.Everything())
	if err != nil {
		// This is a valid state which will eventually be resolved. Therefore, only log the error here.
		logger.Info(fmt.Sprintf("Unable to list replicasets for deployment: %s", err.Error()))
		return nil
	}

	if len(rl) == 0 {
		// This is a valid state which will eventually be resolved. Therefore, only log the error here.
		return nil
	}

	deploymentRevision := deployment.Annotations["deployment.kubernetes.io/revision"]

	// Find the latest ReplicaSet associated with the deployment
	for _, rs := range rl {
		if !metav1.IsControlledBy(rs, deployment) {
			continue
		}
		if rs.Annotations == nil {
			continue
		}
		revision, ok := rs.Annotations["deployment.kubernetes.io/revision"]
		if !ok {
			continue
		}

		// The first answer here https://stackoverflow.com/questions/59848252/kubectl-retrieving-the-current-new-replicaset-for-a-deployment-in-json-forma
		// looks like the best way to determine the current replicaset.
		// Match the replica set revision with the deployment revision
		if deploymentRevision == revision {
			return rs
		}
	}

	return nil
}

func (handler *kubernetesHandler) checkAllPodsReady(ctx context.Context, informerFactory informers.SharedInformerFactory, obj *v1.Deployment, deploymentReplicaSet *v1.ReplicaSet, doneCh chan<- error) bool {
	logger := ucplog.FromContextOrDiscard(ctx).WithValues("deploymentName", obj.GetName(), "namespace", obj.GetNamespace())
	logger.Info("Checking if all pods in the deployment are ready")

	podsInDeployment, err := handler.getPodsInDeployment(ctx, informerFactory, obj, deploymentReplicaSet)
	if err != nil {
		logger.Info(fmt.Sprintf("Error getting pods for deployment: %s", err.Error()))
		return false
	}

	allReady := true
	for _, pod := range podsInDeployment {
		podReady, err := handler.checkPodStatus(ctx, &pod)
		if err != nil {
			// Terminate the deployment and return the error encountered
			doneCh <- err
			return false
		}
		if !podReady {
			allReady = false
		}
	}

	if allReady {
		logger.Info(fmt.Sprintf("All %d pods in the deployment are ready", len(podsInDeployment)))
	}
	return allReady
}

func (handler *kubernetesHandler) getPodsInDeployment(ctx context.Context, informerFactory informers.SharedInformerFactory, deployment *v1.Deployment, deploymentReplicaSet *v1.ReplicaSet) ([]corev1.Pod, error) {
	logger := ucplog.FromContextOrDiscard(ctx)

	pods := []corev1.Pod{}

	// List all pods that match the current replica set
	pl, err := informerFactory.Core().V1().Pods().Lister().Pods(deployment.GetNamespace()).List(labels.Set(deployment.Spec.Selector.MatchLabels).AsSelector())
	if err != nil {
		logger.Info(fmt.Sprintf("Unable to find pods for deployment %s in namespace %s", deployment.GetName(), deployment.GetNamespace()))
		return []corev1.Pod{}, nil
	}

	// Filter out the pods that are not in the Deployment's current ReplicaSet
	for _, p := range pl {
		if !metav1.IsControlledBy(p, deploymentReplicaSet) {
			continue
		}
		pods = append(pods, *p)
	}

	return pods, nil
}

func (handler *kubernetesHandler) checkPodStatus(ctx context.Context, pod *corev1.Pod) (bool, error) {
	logger := ucplog.FromContextOrDiscard(ctx).WithValues("podName", pod.Name, "namespace", pod.Namespace)

	conditionPodReady := true
	for _, cc := range pod.Status.Conditions {
		if cc.Type == corev1.PodReady && cc.Status != corev1.ConditionTrue {
			// Do not return false here else if the pod transitions to a crash loop backoff state,
			// we won't be able to detect that condition.
			conditionPodReady = false
		}

		if cc.Type == corev1.ContainersReady && cc.Status != corev1.ConditionTrue {
			// Do not return false here else if the pod transitions to a crash loop backoff state,
			// we won't be able to detect that condition.
			conditionPodReady = false
		}
	}

	// Sometimes container statuses are not yet available and we do not want to falsely return that the containers are ready
	if len(pod.Status.ContainerStatuses) <= 0 {
		return false, nil
	}

	for _, cs := range pod.Status.ContainerStatuses {
		// Check if the container state is terminated or unable to start due to crash loop, image pull back off or error
		// Note that sometimes a pod can go into running state but can crash later and can go undetected by this condition
		// We will rely on the user defining a readiness probe to ensure that the pod is ready to serve traffic for those cases
		if cs.State.Terminated != nil {
			logger.Info(fmt.Sprintf("Container state is terminated Reason: %s, Message: %s", cs.State.Terminated.Reason, cs.State.Terminated.Message))
			return false, fmt.Errorf("Container state is 'Terminated' Reason: %s, Message: %s", cs.State.Terminated.Reason, cs.State.Terminated.Message)
		} else if cs.State.Waiting != nil {
			if cs.State.Waiting.Reason == "ErrImagePull" || cs.State.Waiting.Reason == "CrashLoopBackOff" || cs.State.Waiting.Reason == "ImagePullBackOff" {
				message := cs.State.Waiting.Message
				if cs.LastTerminationState.Terminated != nil {
					message += " LastTerminationState: " + cs.LastTerminationState.Terminated.Message
				}
				return false, fmt.Errorf("Container state is 'Waiting' Reason: %s, Message: %s", cs.State.Waiting.Reason, message)
			} else {
				return false, nil
			}
		} else if cs.State.Running == nil {
			// The container is not yet running
			return false, nil
		} else if !cs.Ready {
			// The container is running but has not passed its readiness probe yet
			return false, nil
		}
	}

	if !conditionPodReady {
		return false, nil
	}
	logger.Info("All containers for pod are ready")
	return true, nil
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
