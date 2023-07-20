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
	"strconv"
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
	"k8s.io/apimachinery/pkg/labels"
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

// NewKubernetesHandler creates Kubernetes Resource handler instance.
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

// Put creates or updates a Kubernetes resource described in PutOptions.
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
		logger.Info(fmt.Sprintf("Deployment %s in namespace %s is ready", item.GetName(), item.GetNamespace()))
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

func (handler *kubernetesHandler) addPodInformer(ctx context.Context, informers informers.SharedInformerFactory, item client.Object, doneCh chan<- error) cache.SharedIndexInformer {
	// Retrieve the pod informer from the factory
	podInformer := informers.Core().V1().Pods().Informer()

	// Add event handlers to the pod informer
	podInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj any) {
			handler.checkDeploymentStatus(ctx, informers, item, obj, doneCh)
		},
		UpdateFunc: func(_, newObj any) {
			handler.checkDeploymentStatus(ctx, informers, item, newObj, doneCh)
		},
	})

	return podInformer
}

func (handler *kubernetesHandler) addDeploymentInformer(ctx context.Context, informers informers.SharedInformerFactory, item client.Object, doneCh chan<- error) cache.SharedIndexInformer {
	deploymentInformer := informers.Apps().V1().Deployments().Informer()
	handlers := cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj any) {
			handler.checkDeploymentStatus(ctx, informers, item, obj, doneCh)
		},
		UpdateFunc: func(_, obj any) {
			handler.checkDeploymentStatus(ctx, informers, item, obj, doneCh)
		},
	}

	deploymentInformer.AddEventHandler(handlers)
	return deploymentInformer
}

func (handler *kubernetesHandler) addReplicaSetInformer(ctx context.Context, informers informers.SharedInformerFactory, item client.Object, doneCh chan<- error) cache.SharedIndexInformer {
	replicaSetInformer := informers.Apps().V1().ReplicaSets().Informer()
	handlers := cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj any) {
			handler.checkDeploymentStatus(ctx, informers, item, obj, doneCh)
		},
		UpdateFunc: func(_, obj any) {
			handler.checkDeploymentStatus(ctx, informers, item, obj, doneCh)
		},
	}

	replicaSetInformer.AddEventHandler(handlers)
	return replicaSetInformer
}

func (handler *kubernetesHandler) checkDeploymentStatus(ctx context.Context, informerFactory informers.SharedInformerFactory, item client.Object, obj any, doneCh chan<- error) {
	logger := ucplog.FromContextOrDiscard(ctx)

	// Get the deployment
	deployment, err := informerFactory.Apps().V1().Deployments().Lister().Deployments(item.GetNamespace()).Get(item.GetName())
	if err != nil {
		logger.Info(fmt.Sprintf("Unable to find deployment %s in namespace %s", item.GetName(), item.GetNamespace()))
		return
	}

	// Check if the deployment is ready
	handler.checkDeploymentReadiness(ctx, informerFactory, deployment, doneCh)
}

func (handler *kubernetesHandler) getReplicaSetName(pod *corev1.Pod) string {
	for _, owner := range pod.ObjectMeta.OwnerReferences {
		if owner.Kind == "ReplicaSet" {
			return owner.Name
		}
	}
	return ""
}

func (handler *kubernetesHandler) getNewestReplicaSetForDeployment(ctx context.Context, informerFactory informers.SharedInformerFactory, item client.Object) string {
	logger := ucplog.FromContextOrDiscard(ctx)

	// Get all ReplicaSets in the namespace
	// List all replicasets for this deployment
	rl, err := informerFactory.Apps().V1().ReplicaSets().Lister().ReplicaSets(item.GetNamespace()).List(labels.Everything())
	if err != nil {
		// This is a valid state which will eventually be resolved. Therefore, only log the error here.
		logger.Info(fmt.Sprintf("Unable to find replicasets for deployment %s in namespace %s", item.GetName(), item.GetNamespace()))
		return ""
	}

	replicaSets := &v1.ReplicaSetList{}
	for _, rs := range rl {
		replicaSets.Items = append(replicaSets.Items, *rs)
	}

	if replicaSets == nil || len(replicaSets.Items) == 0 {
		// This is a valid state which will eventually be resolved. Therefore, only log the error here.
		logger.Info(fmt.Sprintf("Unable to get replicasets in namespace %s", item.GetNamespace()))
		return ""
	}

	deployment, err := handler.clientSet.AppsV1().Deployments(item.GetNamespace()).Get(ctx, item.GetName(), metav1.GetOptions{})
	if err != nil {
		// This is a valid state which will eventually be resolved. Therefore, only log the error here.
		logger.Info(fmt.Sprintf("Unable to get deployment %s in namespace %s: %s", item.GetName(), item.GetNamespace(), err.Error()))
		return ""
	}

	if deployment == nil {
		// This is a valid state which will eventually be resolved. Therefore, only log the error here.
		logger.Info(fmt.Sprintf("Unable to get deployment %s in namespace %s: %s", item.GetName(), item.GetNamespace(), err.Error()))
		return ""
	}

	// Find the latest ReplicaSet associated with the deployment
	var latestRS *v1.ReplicaSet
	var latestRevision int64 = -1
	for i, rs := range replicaSets.Items {
		if !metav1.IsControlledBy(&rs, deployment) {
			continue
		}
		if rs.Annotations == nil {
			continue
		}
		revisionStr, ok := rs.Annotations["deployment.kubernetes.io/revision"]
		if !ok {
			continue
		}

		revision, err := strconv.ParseInt(revisionStr, 10, 64)
		if err != nil {
			continue
		}

		if latestRS == nil || revision > latestRevision {
			latestRS = &replicaSets.Items[i]
			latestRevision = revision
		}
	}

	if latestRS == nil {
		logger.Info(fmt.Sprintf("Unable to find any replicasets for deployment %s in namespace %s", item.GetName(), item.GetNamespace()))
		return ""
	}

	return latestRS.Name
}

func (handler *kubernetesHandler) checkPodStatus(ctx context.Context, pod *corev1.Pod) (bool, error) {
	logger := ucplog.FromContextOrDiscard(ctx)

	conditionPodReady := true
	for _, cc := range pod.Status.Conditions {
		// If the resource limits for the container cannot be satisfied, the pod will not be scheduled
		if cc.Type == corev1.PodScheduled && cc.Status == corev1.ConditionFalse {
			logger.Info(fmt.Sprintf("Pod %s in namespace %s is not scheduled. Reason: %s, Message: %s", pod.Name, pod.Namespace, cc.Reason, cc.Message))
			return false, fmt.Errorf("Pod %s in namespace %s is not scheduled. Reason: %s, Message: %s", pod.Name, pod.Namespace, cc.Reason, cc.Message)
		}

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
	logger.Info(fmt.Sprintf("All containers for pod %s in namespace: %s are ready", pod.Name, pod.Namespace))
	return true, nil
}

func (handler *kubernetesHandler) startInformers(ctx context.Context, item client.Object, doneCh chan<- error) error {
	logger := ucplog.FromContextOrDiscard(ctx)
	informers := informers.NewSharedInformerFactoryWithOptions(handler.clientSet, handler.cacheResyncInterval, informers.WithNamespace(item.GetNamespace()))

	handler.addPodInformer(ctx, informers, item, doneCh)
	handler.addDeploymentInformer(ctx, informers, item, doneCh)
	handler.addReplicaSetInformer(ctx, informers, item, doneCh)

	// Start the informers
	informers.Start(ctx.Done())

	// Wait for the deployment and pod informer's cache to be synced.
	informers.WaitForCacheSync(ctx.Done())

	logger.Info(fmt.Sprintf("Informers started and caches synced for deployment: %s in namespace: %s", item.GetName(), item.GetNamespace()))
	return nil
}

func (handler *kubernetesHandler) getPodsInDeployment(ctx context.Context, informerFactory informers.SharedInformerFactory, deployment *v1.Deployment, deploymentReplicaSet string) ([]corev1.Pod, error) {
	logger := ucplog.FromContextOrDiscard(ctx)

	pods := []corev1.Pod{}

	// List all pods that match the current replica set
	pl, err := informerFactory.Core().V1().Pods().Lister().Pods(deployment.GetNamespace()).List(labels.Set(deployment.Spec.Selector.MatchLabels).AsSelector())
	if err != nil {
		logger.Info(fmt.Sprintf("Unable to find pods for deployment %s in namespace %s", deployment.GetName(), deployment.GetNamespace()))
		return []corev1.Pod{}, nil
	}

	podList := &corev1.PodList{}
	for _, p := range pl {
		podList.Items = append(podList.Items, *p)
	}

	// Filter out the pods that are not in the Deployment's current ReplicaSet
	for _, p := range podList.Items {
		if handler.getReplicaSetName(&p) == deploymentReplicaSet {
			pods = append(pods, p)
		}
	}

	return pods, nil
}

func (handler *kubernetesHandler) checkAllPodsReady(ctx context.Context, informerFactory informers.SharedInformerFactory, obj *v1.Deployment, deploymentReplicaSet string, doneCh chan<- error) bool {
	logger := ucplog.FromContextOrDiscard(ctx)
	logger.Info(fmt.Sprintf("Checking if all pods in the deployment %s in namespace %s are ready", obj.Name, obj.Namespace))

	podsInDeployment, err := handler.getPodsInDeployment(ctx, informerFactory, obj, deploymentReplicaSet)
	if err != nil {
		logger.Info(fmt.Sprintf("Error getting pods for deployment %s in namespace %s: %s", obj.GetName(), obj.GetNamespace(), err))
		return false
	}

	allReady := true
	for _, pod := range podsInDeployment {
		status, err := handler.checkPodStatus(ctx, &pod)
		if err != nil {
			doneCh <- err
		}
		if !status {
			allReady = false
		}
	}

	logger.Info(fmt.Sprintf("All %d pods in the deployment are ready", len(podsInDeployment)))
	return allReady
}

func (handler *kubernetesHandler) checkDeploymentReadiness(ctx context.Context, informerFactory informers.SharedInformerFactory, obj *v1.Deployment, doneCh chan<- error) {
	logger := ucplog.FromContextOrDiscard(ctx)
	deploymentReplicaSet := handler.getNewestReplicaSetForDeployment(ctx, informerFactory, obj)

	for _, c := range obj.Status.Conditions {
		// check for complete deployment condition
		// Reference https://kubernetes.io/docs/concepts/workloads/controllers/deployment/#complete-deployment
		if c.Type == v1.DeploymentProgressing && c.Status == corev1.ConditionTrue && strings.EqualFold(c.Reason, "NewReplicaSetAvailable") {
			logger.Info(fmt.Sprintf("Deployment status for deployment: %s in namespace: %s is: %s - %s, Reason: %s, Deployment replicaset: %s", obj.Name, obj.Namespace, c.Type, c.Status, c.Reason, deploymentReplicaSet))

			// ObservedGeneration should be updated to latest generation to avoid stale replicas
			if obj.Status.ObservedGeneration >= obj.Generation {
				// Sometimes, this check can kick in before the pod informer. Therefore, check all pods in the deployment are ready here too.
				allReady := handler.checkAllPodsReady(ctx, informerFactory, obj, deploymentReplicaSet, doneCh)

				if allReady && deploymentReplicaSet != "" {
					logger.Info(fmt.Sprintf("Deployment %s in namespace %s is ready. Observed generation: %d, Generation: %d", obj.Name, obj.Namespace, obj.Status.ObservedGeneration, obj.Generation))
					doneCh <- nil
					return
				}
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
