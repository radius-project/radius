package handlers

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/radius-project/radius/pkg/ucp/ucplog"
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/informers"
	k8s "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// MaxDeploymentTimeout is the max timeout for waiting for a deployment to be ready.
	// Deployment duration should not reach to this timeout since async operation worker will time out context before MaxDeploymentTimeout.
	MaxDeploymentTimeout = time.Minute * time.Duration(10)
)

type deploymentWaiter struct {
	clientSet           k8s.Interface
	deploymentTimeOut   time.Duration
	cacheResyncInterval time.Duration
}

func NewDeploymentWaiter(clientSet k8s.Interface) *deploymentWaiter {
	return &deploymentWaiter{
		clientSet:           clientSet,
		deploymentTimeOut:   MaxDeploymentTimeout,
		cacheResyncInterval: DefaultCacheResyncInterval,
	}
}

func (handler *deploymentWaiter) addEventHandler(ctx context.Context, informerFactory informers.SharedInformerFactory, informer cache.SharedIndexInformer, item client.Object, doneCh chan<- deploymentStatus) {
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

// addDynamicEventHandler is not implemented for deploymentWaiter
func (handler *deploymentWaiter) addDynamicEventHandler(ctx context.Context, informerFactory dynamicinformer.DynamicSharedInformerFactory, informer cache.SharedIndexInformer, item client.Object, doneCh chan<- error) {
}

type deploymentStatus struct {
	possibleFailureCause string
	err                  error
}

func (handler *deploymentWaiter) waitUntilReady(ctx context.Context, item client.Object) error {
	logger := ucplog.FromContextOrDiscard(ctx)

	// When the deployment is done, an error nil will be sent
	// In case of an error, the error will be sent
	doneCh := make(chan deploymentStatus, 1)

	ctx, cancel := context.WithTimeout(ctx, handler.deploymentTimeOut)
	// This ensures that the informer is stopped when this function is returned.
	defer cancel()

	err := handler.startInformers(ctx, item, doneCh)
	if err != nil {
		logger.Error(err, "failed to start deployment informer")
		return err
	}

	var possibleFailureCauses []string

	for {
		select {
		case <-ctx.Done():
			fmt.Println("@@@@@ Inside ctx.Done()")
			// Get the final deployment status
			dep, err := handler.clientSet.AppsV1().Deployments(item.GetNamespace()).Get(ctx, item.GetName(), metav1.GetOptions{})
			if err != nil {
				return fmt.Errorf("deployment timed out, name: %s, namespace %s, error occured while fetching latest status: %w", item.GetName(), item.GetNamespace(), err)
			}

			fmt.Println("@@@@@ Now get the latest available observation of deployment current state")

			// Now get the latest available observation of deployment current state
			// note that there can be a race condition here, by the time it fetches the latest status, deployment might be succeeded
			status := v1.DeploymentCondition{}
			fmt.Println("@@@@@ len(dep.Status.Conditions)", len(dep.Status.Conditions))
			if len(dep.Status.Conditions) > 0 {
				status = dep.Status.Conditions[len(dep.Status.Conditions)-1]
			}

			fmt.Println("@@@@@ Marking deployment as timed out")
			// Return the error with the possible failure causes
			errString := fmt.Sprintf("deployment timed out, name: %s, namespace %s, status: %s, reason: %s", item.GetName(), item.GetNamespace(), status.Message, status.Reason)
			if len(possibleFailureCauses) > 0 {
				errString += fmt.Sprintf(", possible failure causes: %s", strings.Join(possibleFailureCauses, ", "))
			}
			return errors.New(errString)

		case status := <-doneCh:
			if status.err != nil {
				logger.Info(fmt.Sprintf("Marking deployment %s in namespace %s as complete", item.GetName(), item.GetNamespace()))
				return status.err
			} else if status.possibleFailureCause != "" {
				possibleFailureCauses = append(possibleFailureCauses, status.possibleFailureCause)
				fmt.Println("@@@@@ possibleFailureCauses", status.possibleFailureCause)
				continue
			} else {
				// Deployment is ready
				return nil
			}
		}
	}
}

// Check if all the pods in the deployment are ready
func (handler *deploymentWaiter) checkDeploymentStatus(ctx context.Context, informerFactory informers.SharedInformerFactory, item client.Object, doneCh chan<- deploymentStatus) bool {
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
			doneCh <- deploymentStatus{
				possibleFailureCause: "",
				err:                  nil,
			}
			return true
		} else {
			logger.Info(fmt.Sprintf("Deployment status is: %s - %s, Reason: %s, Deployment replicaset: %s", c.Type, c.Status, c.Reason, deploymentReplicaSet.Name))
		}
	}
	return false
}

func (handler *deploymentWaiter) startInformers(ctx context.Context, item client.Object, doneCh chan<- deploymentStatus) error {
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

// Gets the current replica set for the deployment
func (handler *deploymentWaiter) getCurrentReplicaSetForDeployment(ctx context.Context, informerFactory informers.SharedInformerFactory, deployment *v1.Deployment) *v1.ReplicaSet {
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

func (handler *deploymentWaiter) checkAllPodsReady(ctx context.Context, informerFactory informers.SharedInformerFactory, obj *v1.Deployment, deploymentReplicaSet *v1.ReplicaSet, doneCh chan<- deploymentStatus) bool {
	logger := ucplog.FromContextOrDiscard(ctx).WithValues("deploymentName", obj.GetName(), "namespace", obj.GetNamespace())
	logger.Info("Checking if all pods in the deployment are ready")

	podsInDeployment, err := handler.getPodsInDeployment(ctx, informerFactory, obj, deploymentReplicaSet)
	if err != nil {
		logger.Info(fmt.Sprintf("Error getting pods for deployment: %s", err.Error()))
		return false
	}

	allReady := true
	for _, pod := range podsInDeployment {
		podReady, status := handler.checkPodStatus(ctx, informerFactory, &pod)
		if status != (deploymentStatus{}) {
			doneCh <- status
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

func (handler *deploymentWaiter) getPodsInDeployment(ctx context.Context, informerFactory informers.SharedInformerFactory, deployment *v1.Deployment, deploymentReplicaSet *v1.ReplicaSet) ([]corev1.Pod, error) {
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

func (handler *deploymentWaiter) checkPodStatus(ctx context.Context, informerFactory informers.SharedInformerFactory, pod *corev1.Pod) (bool, deploymentStatus) {
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
		return false, deploymentStatus{}
	}

	for _, cs := range pod.Status.ContainerStatuses {
		// Check if the container state is terminated or unable to start due to crash loop, image pull back off or error
		// Note that sometimes a pod can go into running state but can crash later and can go undetected by this condition
		// We will rely on the user defining a readiness probe to ensure that the pod is ready to serve traffic for those cases
		if cs.State.Terminated != nil {
			logger.Info(fmt.Sprintf("Container state is terminated Reason: %s, Message: %s", cs.State.Terminated.Reason, cs.State.Terminated.Message))
			return false, deploymentStatus{possibleFailureCause: "", err: fmt.Errorf("Container state is 'Terminated' Reason: %s, Message: %s", cs.State.Terminated.Reason, cs.State.Terminated.Message)}
		} else if cs.State.Waiting != nil {
			if cs.State.Waiting.Reason == "ErrImagePull" || cs.State.Waiting.Reason == "CrashLoopBackOff" || cs.State.Waiting.Reason == "ImagePullBackOff" {
				message := cs.State.Waiting.Message
				if cs.LastTerminationState.Terminated != nil {
					message += " LastTerminationState: " + cs.LastTerminationState.Terminated.Message
				}
				return false, deploymentStatus{possibleFailureCause: "", err: fmt.Errorf("Container state is 'Waiting' Reason: %s, Message: %s", cs.State.Waiting.Reason, message)}
			} else {
				return false, deploymentStatus{}
			}
		} else if cs.State.Running == nil {
			// The container is not yet running
			return false, deploymentStatus{}
		} else if !cs.Ready {
			// The container is running but has not passed its readiness probe yet
			// Check the pod events to see if the pod failed readiness probe
			fmt.Println("@@@@@ cs.Ready false. Checking pod events")
			events, err := informerFactory.Core().V1().Events().Lister().Events(pod.Namespace).List(labels.Everything())
			if err != nil {
				logger.Info("Unable to get events for pod")
				return false, deploymentStatus{}
			}

			// Sort events by creation timestamp in descending order
			sort.Slice(events, func(i, j int) bool {
				return events[i].CreationTimestamp.Time.After(events[j].CreationTimestamp.Time)
			})

			for _, event := range events {
				fmt.Println("@@@@@ event.Message", event.Message)
				if strings.Contains(event.Message, "Readiness probe failed") {
					return false, deploymentStatus{possibleFailureCause: fmt.Sprintf("Container failed readiness probe. Reason: %s, Message: %s", event.Reason, event.Message), err: nil}
				}
			}

			// The pod is not ready yet but has not failed readiness probe either. So continue waiting for the pod to be ready
			return false, deploymentStatus{}
		}
	}

	events, err := informerFactory.Core().V1().Events().Lister().Events(pod.Namespace).List(labels.Everything())
	if err != nil {
		logger.Info("Unable to get events for pod")
		return false, deploymentStatus{}
	}

	// Sort events by creation timestamp in descending order
	sort.Slice(events, func(i, j int) bool {
		return events[i].CreationTimestamp.Time.After(events[j].CreationTimestamp.Time)
	})

	for _, event := range events {
		fmt.Println("@@@@@ event.Message", event.Message)
		if strings.Contains(event.Message, "Readiness probe failed") {
			return false, deploymentStatus{possibleFailureCause: fmt.Sprintf("Container failed readiness probe. Reason: %s, Message: %s", event.Reason, event.Message), err: nil}
		}
	}

	if !conditionPodReady {
		return false, deploymentStatus{}
	}
	logger.Info("All containers for pod are ready")
	return true, deploymentStatus{}
}
