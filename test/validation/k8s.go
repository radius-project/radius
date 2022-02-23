// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package validation

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	kuberneteskeys "github.com/project-radius/radius/pkg/kubernetes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	watchk8s "k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	v1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/restmapper"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1alpha1 "sigs.k8s.io/gateway-api/apis/v1alpha1"
)

const (
	IntervalForDeploymentCreation = 3 * time.Second
	IntervalForPodShutdown        = 3 * time.Second
	IntervalForResourceCreation   = 3 * time.Second

	// We want to make sure to produce some output any time we're in a watch
	// otherwise it's hard to know if it got stuck.
	IntervalForWatchHeartbeat = 10 * time.Second
)

type K8sObjectSet struct {
	Namespaces map[string][]K8sObject
}

type K8sObject struct {
	Labels map[string]string
}

func NewK8sObjectForResource(application string, name string) K8sObject {
	return K8sObject{
		// NOTE: we use the selector labels here because the selector labels are intended
		// to be determininistic. We might add things to the descriptive labels that are NON deterministic.
		Labels: kuberneteskeys.MakeSelectorLabels(application, name),
	}
}

func ValidateDeploymentsRunning(ctx context.Context, t *testing.T, k8s *kubernetes.Clientset, expected K8sObjectSet) {
	for namespace, expectedPods := range expected.Namespaces {
		t.Logf("validating deployments in namespace %v", namespace)
		for {
			select {
			case <-time.After(IntervalForDeploymentCreation):
				t.Logf("at %s waiting for deployments in namespace %s to appear.. ", time.Now().Format("2006-01-02 15:04:05"), namespace)

				var err error
				deployments, err := k8s.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{})
				require.NoErrorf(t, err, "failed to list pods in namespace %v", namespace)

				remaining := make([]K8sObject, len(expectedPods))
				copy(remaining, expectedPods)

				for _, actualDeployment := range deployments.Items {
					// validate that this matches one of our expected deployment
					index := matchesExpectedLabels(remaining, actualDeployment.Labels)
					if index == nil {
						// this is not a match, check if it has a radius application label
						t.Log(t,
							"unrecognized deployment, could not find a match for Deployment with namespace: %v name: %v labels: %v",
							actualDeployment.Namespace,
							actualDeployment.Name,
							actualDeployment.Labels)
						continue
					}

					// trim the list of 'remaining' deployments
					remaining = append(remaining[:*index], remaining[*index+1:]...)
				}

				if len(remaining) == 0 {
					return
				}
				for _, remainingPod := range remaining {
					t.Logf("failed to match deployment in namespace %v with labels %v, retrying", namespace, remainingPod.Labels)
				}

			case <-ctx.Done():
				assert.Fail(t, "timed out after waiting for deployments to be created")
				return
			}
		}
	}
}

// SaveContainerLogs get container logs for all containers in a namespace and saves them to disk.
func SaveLogsForController(ctx context.Context, k8s *kubernetes.Clientset, namespace string, logPrefix string) error {
	return watchForPods(ctx, k8s, namespace, logPrefix, "")
}

// SaveAndWatchContainerLogsForApp watches for all containers in a namespace and saves them to disk.
func SaveLogsForApplication(ctx context.Context, k8s *kubernetes.Clientset, namespace string, logPrefix string, appName string) error {
	return watchForPods(ctx, k8s, namespace, logPrefix, fmt.Sprintf("%s=%s", kuberneteskeys.LabelRadiusApplication, appName))
}

func watchForPods(ctx context.Context, k8s *kubernetes.Clientset, namespace string, logPrefix string, labelSelector string) error {
	if err := os.MkdirAll(logPrefix, os.ModePerm); err != nil {
		log.Printf("Failed to create output log directory '%s' Error was: '%q'. Container logs will be discarded", logPrefix, err)
		return nil
	}

	podClient := k8s.CoreV1().Pods(namespace)

	// Filter only radius applications for a pod
	podList, err := podClient.Watch(ctx, metav1.ListOptions{
		Watch:         true,
		LabelSelector: labelSelector,
	})
	if err != nil {
		return err
	}

	go func() {
		pods := map[string]bool{}
		for event := range podList.ResultChan() {
			pod, ok := event.Object.(*corev1.Pod)
			if !ok {
				_, ok := event.Object.(*metav1.Status)
				if ok {
					// Ignore statuses, these might be the result of a connection dropping or the watch being cancelled.
					continue
				}

				log.Printf("Could not convert object to pod or status, was %+v.", event.Object)
				continue
			}

			// Only start one log capture per pod
			_, ok = pods[pod.Name]
			if pod.Status.Phase != corev1.PodRunning || ok {
				continue
			}
			pods[pod.Name] = true

			for _, container := range pod.Spec.Containers {
				go streamLogFile(ctx, podClient, *pod, container, logPrefix)
			}
		}
	}()

	return nil
}

// See https://github.com/dapr/dapr/blob/22bb68bc89a86fc64c2c27dfd219ba68a38fb2ad/tests/platforms/kubernetes/appmanager.go#L706 for reference.
func streamLogFile(ctx context.Context, podClient v1.PodInterface, pod corev1.Pod, container corev1.Container, logPrefix string) {
	filename := fmt.Sprintf("%s/%s.%s.log", logPrefix, pod.Name, container.Name)
	log.Printf("Streaming Kubernetes logs to %s", filename)
	req := podClient.GetLogs(pod.Name, &corev1.PodLogOptions{
		Container: container.Name,
		Follow:    true,
	})
	stream, err := req.Stream(ctx)
	if err != nil && err == ctx.Err() {
		return
	} else if err != nil {
		log.Printf("Error reading log stream for %s. Error was %+v", filename, err)
		return
	}
	defer stream.Close()

	fh, err := os.Create(filename)
	if err != nil {
		log.Printf("Error creating %s. Error was %s", filename, err)
		return
	}
	defer fh.Close()

	buf := make([]byte, 2000)

	for {
		numBytes, err := stream.Read(buf)

		if err == io.EOF {
			break
		}

		if err != nil && err == ctx.Err() {
			return
		} else if err != nil {
			log.Printf("Error reading log stream for %s. Error was %+v", filename, err)
			return
		}

		if numBytes == 0 {
			continue
		}

		_, err = fh.Write(buf[:numBytes])
		if err != nil {
			log.Printf("Error writing to %s. Error was %s", filename, err)
			return
		}
	}

	log.Printf("Saved container logs to %s", filename)
}

// ValidatePodsRunning validates the namespaces and pods specified in each namespace are running
func ValidatePodsRunning(ctx context.Context, t *testing.T, k8s *kubernetes.Clientset, expected K8sObjectSet) {
	for namespace, expectedPods := range expected.Namespaces {
		t.Logf("validating pods in namespace %v", namespace)
		var actualPods *corev1.PodList
		applicationPods := []corev1.Pod{}
		for {
			select {
			case <-time.After(IntervalForResourceCreation):
				t.Logf("at %s waiting for pods in namespace %s to appear.. ", time.Now().Format("2006-01-02 15:04:05"), namespace)

				var err error
				actualPods, err = k8s.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
				require.NoErrorf(t, err, "failed to list pods in namespace %v", namespace)

				// log all the data so its there if we need to analyze a failure
				logPods(t, actualPods.Items)

				// copy the list of expected pods so we can remove from it
				//
				// this way we "check off" each pod as it is matched
				remaining := make([]K8sObject, len(expectedPods))
				copy(remaining, expectedPods)

				for _, actualPod := range actualPods.Items {
					// validate that this matches one of our expected pods
					index := matchesExpectedLabels(remaining, actualPod.Labels)
					if index == nil {
						// this is not a match
						t.Logf("unrecognized pod, could not find a match for Pod with namespace: %v name: %v labels: %v",
							actualPod.Namespace,
							actualPod.Name,
							actualPod.Labels)
						continue
					}

					// trim the list of 'remaining' pods
					remaining = append(remaining[:*index], remaining[*index+1:]...)
					applicationPods = append(applicationPods, actualPod)
				}

				if len(remaining) == 0 {
					goto podcheck
				}
				for _, remainingPod := range remaining {
					t.Logf("failed to match pod in namespace %v with labels %v, retrying", namespace, remainingPod.Labels)
				}

			case <-ctx.Done():
				assert.Fail(t, "timed out after waiting for pods to be created")
				return
			}
		}

		// Now check the status of the pods
	podcheck:
		for _, applicationPod := range applicationPods {
			monitor := PodMonitor{
				K8s: k8s,
				Pod: applicationPod,
			}
			monitor.ValidateRunning(ctx, t)
		}
	}
}

// ValidateGatewaysRunning validates the namespaces and gateways specified in each namespace are running
func ValidateGatewaysRunning(ctx context.Context, t *testing.T, client client.Client, expected K8sObjectSet) {
	for namespace, expectedGateways := range expected.Namespaces {
		t.Logf("validating gateways in namespace %v", namespace)
		for {
			select {
			case <-time.After(IntervalForResourceCreation):
				t.Logf("at %s waiting for gateways in namespace %s to appear.. ", time.Now().Format("2006-01-02 15:04:05"), namespace)

				var err error
				var actualGateways gatewayv1alpha1.GatewayList
				err = client.List(ctx, &actualGateways)
				require.NoErrorf(t, err, "failed to list gateways in namespace %v", namespace)

				remaining := make([]K8sObject, len(expectedGateways))
				copy(remaining, expectedGateways)

				for _, gateway := range actualGateways.Items {
					index := matchesExpectedLabels(remaining, gateway.Labels)
					if index == nil {
						t.Logf("unrecognized gateway, could not find a match for gateway with namespace: %v name: %v labels: %v",
							gateway.Namespace,
							gateway.Name,
							gateway.Labels)
						continue
					}

					remaining = append(remaining[:*index], remaining[*index+1:]...)
				}

				if len(remaining) == 0 {
					return
				}
				for _, remainingIngress := range remaining {
					t.Logf("failed to match gateway in namespace %v with labels %v, retrying", namespace, remainingIngress.Labels)
				}

			case <-ctx.Done():
				assert.Fail(t, "timed out after waiting for gateways to be created")
				return
			}
		}
	}
}

// ValidateHttpRoutesRunning validates the namespaces and gateways specified in each namespace are running
func ValidateHttpRoutesRunning(ctx context.Context, t *testing.T, client client.Client, expected K8sObjectSet) {
	for namespace, expectedHttpRoutes := range expected.Namespaces {
		t.Logf("validating gateways in namespace %v", namespace)
		for {
			select {
			case <-time.After(IntervalForResourceCreation):
				t.Logf("at %s waiting for gateways in namespace %s to appear.. ", time.Now().Format("2006-01-02 15:04:05"), namespace)

				var err error
				var actualRoutes gatewayv1alpha1.HTTPRouteList
				err = client.List(ctx, &actualRoutes)
				require.NoErrorf(t, err, "failed to list httproutes in namespace %v", namespace)

				remaining := make([]K8sObject, len(expectedHttpRoutes))
				copy(remaining, expectedHttpRoutes)

				for _, httproute := range actualRoutes.Items {
					index := matchesExpectedLabels(remaining, httproute.Labels)
					if index == nil {
						t.Logf("unrecognized httproute, could not find a match for Ingress with namespace: %v name: %v labels: %v",
							httproute.Namespace,
							httproute.Name,
							httproute.Labels)
						continue
					}

					remaining = append(remaining[:*index], remaining[*index+1:]...)
				}

				if len(remaining) == 0 {
					return
				}
				for _, remainingIngress := range remaining {
					t.Logf("failed to match httproute in namespace %v with labels %v, retrying", namespace, remainingIngress.Labels)
				}

			case <-ctx.Done():
				assert.Fail(t, "timed out after waiting for httproutes to be created")
				return
			}
		}
	}
}

// ValidateServicesRunning validates the namespaces and services specified in each namespace are running
func ValidateServicesRunning(ctx context.Context, t *testing.T, k8s *kubernetes.Clientset, expected K8sObjectSet) {
	for namespace, expectedServices := range expected.Namespaces {
		t.Logf("validating services in namespace %v", namespace)
		for {
			select {
			case <-time.After(IntervalForResourceCreation):
				t.Logf("at %s waiting for services in namespace %s to appear.. ", time.Now().Format("2006-01-02 15:04:05"), namespace)

				var err error
				actualServices, err := k8s.CoreV1().Services(namespace).List(ctx, metav1.ListOptions{})
				require.NoErrorf(t, err, "failed to list services in namespace %v", namespace)

				// copy the list of expected services so we can remove from it
				//
				// this way we "check off" each service as it is matched
				remaining := make([]K8sObject, len(expectedServices))
				copy(remaining, expectedServices)

				for _, service := range actualServices.Items {
					// validate that this matches one of our expected services
					index := matchesExpectedLabels(remaining, service.Labels)
					if index == nil {
						// this is not a match
						t.Logf("unrecognized service, could not find a match for Service with namespace: %v name: %v labels: %v",
							service.Namespace,
							service.Name,
							service.Labels)
						continue
					}

					// trim the list of 'remaining' services
					remaining = append(remaining[:*index], remaining[*index+1:]...)
				}

				if len(remaining) == 0 {
					return
				}
				for _, remainingService := range remaining {
					t.Logf("failed to match service in namespace %v with labels %v, retrying", namespace, remainingService.Labels)
				}

			case <-ctx.Done():
				assert.Fail(t, "timed out after waiting for services to be created")
				return
			}
		}
	}
}

// ValidateResourcesCreated validates that the required Kubernetes resources were created correctly.
func ValidateResourcesCreated(ctx context.Context, t *testing.T, k8s *kubernetes.Clientset, dynamic dynamic.Interface, expected []unstructured.Unstructured) {
	restMapper := restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(k8s.DiscoveryClient))
	for _, r := range expected {
		resourceId := fmt.Sprintf("%q/%s/%s", r.GroupVersionKind(), r.GetNamespace(), r.GetName())
		t.Logf("validating resources %s", resourceId)
		for {
			select {
			case <-time.After(IntervalForResourceCreation):
				mapping, err := restMapper.RESTMapping(r.GroupVersionKind().GroupKind(), r.GroupVersionKind().Version)
				if err != nil {
					t.Errorf("unknown kind %q: %v", r.GroupVersionKind().String(), err)
				}
				var output *unstructured.Unstructured
				if mapping.Scope == meta.RESTScopeNamespace {
					output, err = dynamic.Resource(mapping.Resource).Namespace(r.GetNamespace()).Get(context.TODO(), r.GetName(), metav1.GetOptions{})
				} else {
					output, err = dynamic.Resource(mapping.Resource).Get(context.TODO(), r.GetName(), metav1.GetOptions{})
				}
				if err != nil {
					if errors.IsNotFound(err) {
						t.Logf("resource %q/%s/%s not yet created", r.GroupVersionKind(), r.GetNamespace(), r.GetName())
						continue
					}
					assert.Failf(t, "fail to look for resource %s: %v", resourceId, err)
				}
				meta, _ := output.Object["metadata"].(map[string]interface{})
				// ignore some fields that only known at runtime.
				delete(meta, "managedFields")
				delete(meta, "resourceVersion")
				delete(meta, "creationTimestamp")
				delete(meta, "uid")

				annotations, _ := meta["annotations"].(map[string]interface{})
				// ignore deployment template label: these are generated names and won't match.
				delete(annotations, kuberneteskeys.LabelRadiusDeployment)
				if len(annotations) == 0 {
					delete(meta, "annotations")
				}
				if diff := cmp.Diff(r, *output); diff != "" {
					assert.Fail(t, "resource "+resourceId+" mismatch (-want,+diff): "+diff)
				}
				return
			case <-ctx.Done():
				assert.Fail(t, "timed out after waiting for services to be created")
				return
			}
		}
	}
}

func ValidateNoPodsInApplication(ctx context.Context, t *testing.T, k8s *kubernetes.Clientset, namespace string, application string) {
	labelset := kuberneteskeys.MakeSelectorLabels(application, "")

	actualPods, err := k8s.CoreV1().Pods(namespace).List(context.Background(), metav1.ListOptions{
		LabelSelector: labels.SelectorFromSet(labelset).String(),
	})
	assert.NoErrorf(t, err, "failed to list pods in namespace %s", namespace)

	logPods(t, actualPods.Items)

	// There's an inherent race condition in verifying that the Pods are gone. We're at the
	// mercy of the Kubernetes event loop. We'll wait for pods to disappear if we find them.
	if len(actualPods.Items) == 0 {
		return
	}
	for {
		select {

		case <-ctx.Done():
			assert.Fail(t, "timed out waiting for pods to be deleted")
			return

		case <-time.After(IntervalForPodShutdown):
			t.Logf("at %s waiting for pods in namespace %s for application %s to shut down.. ", time.Now().Format("2006-01-02 15:04:05"), namespace, application)

			actualPods, err := k8s.CoreV1().Pods(namespace).List(context.Background(), metav1.ListOptions{
				LabelSelector: labels.SelectorFromSet(labelset).String(),
			})
			assert.NoErrorf(t, err, "failed to list pods in namespace %s for application %s", namespace, application)

			logPods(t, actualPods.Items)
			if len(actualPods.Items) == 0 {
				t.Logf("success! no pods found in namespace %s for application %s", namespace, application)
				return
			}
		}
	}
}

type PodMonitor struct {
	K8s *kubernetes.Clientset
	Pod corev1.Pod
}

func (pm PodMonitor) ValidateRunning(ctx context.Context, t *testing.T) {
	if pm.Pod.Status.Phase == corev1.PodRunning {
		if checkReadiness(t, &pm.Pod) {
			return
		}
	}

	t.Logf("watching pod %v for status.. current: %v", pm.Pod.Name, pm.Pod.Status)

	// If we're not in the running state, we need to wait a bit to see if things work out.
	watch, err := pm.K8s.CoreV1().Pods(pm.Pod.Namespace).Watch(ctx, metav1.SingleObject(pm.Pod.ObjectMeta))
	require.NoErrorf(t, err, "failed to watch pod: %v", pm.Pod.Name)
	defer watch.Stop()

	// Sometimes, the pods may take a little bit to become ready.
	// Therefore, if the readiness check fails, will retry a few times instead
	// of instantly failing
	const MaxRetryAttempts = 10
	attempt := 0
	for {
		select {
		case <-time.After(IntervalForWatchHeartbeat):
			t.Logf("at %s watching pod %v for status.. ", time.Now().Format("2006-01-02 15:04:05"), pm.Pod.Name)

		case event := <-watch.ResultChan():
			pod, ok := event.Object.(*corev1.Pod)
			if !ok {
				// Check the status if there is a failure.
				// Errors usually have a status as the object type
				if event.Type == watchk8s.Error {
					status, ok := event.Object.(*metav1.Status)
					if ok {
						if status.Reason == "Expired" {
							t.Logf("skipped pod watch expiration error: %s", status.Message)
						} else {
							t.Errorf("pod watch error with status reason: %s, message: %s", status.Reason, status.Message)
						}
					}
				}
				require.IsTypef(t, &corev1.Pod{}, event.Object, "object %T is not a pod", event.Object)
			}

			if pod.Status.Phase == corev1.PodRunning {
				t.Logf("success! pod %v has status: %v", pod.Name, pod.Status)
				if checkReadiness(t, pod) {
					return
				}
				if attempt >= MaxRetryAttempts {
					assert.Failf(t, "pod %v failed readiness checks", pod.Name)
				} else {
					t.Logf("Readiness check failed. Retrying - attempt %d", attempt)
					attempt++
				}
			} else if pod.Status.Phase == corev1.PodFailed {
				assert.Failf(t, "pod %v entered a failing state", pod.Name)
				return
			}

			t.Logf("watching pod %v for status.. current: %v", pod.Name, pod.Status)

		case <-ctx.Done():
			assert.Failf(t, "timed out after waiting for pod %v to enter running status", pm.Pod.Name)
			return
		}
	}
}

func checkReadiness(t *testing.T, pod *corev1.Pod) bool {
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.ContainersReady &&
			condition.Status == corev1.ConditionTrue {
			// All okay
			return true
		}
	}
	return false
}

func logPods(t *testing.T, pods []corev1.Pod) {
	t.Log("Found the following pods:")
	if len(pods) == 0 {
		t.Logf("(none)")
	} else {
		for _, pod := range pods {
			t.Logf("namespace: %v name: %v labels: %v", pod.Namespace, pod.Name, pod.Labels)
		}
	}
}

// returns the index if its found, otherwise nil
func matchesExpectedLabels(expectedPods []K8sObject, labels map[string]string) *int {
	if labels == nil {
		return nil
	}
	for index, expectedPod := range expectedPods {

		// we don't need to match all of the labels on the expected pod
		matchesPod := true
		for key, value := range expectedPod.Labels {
			// just in case

			if labels[key] != value {
				// not a match for this pod
				matchesPod = false
				break
			}
		}

		if matchesPod {
			return &index
		}
	}

	return nil
}
