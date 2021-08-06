// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package validation

import (
	"context"
	"testing"
	"time"

	kuberneteskeys "github.com/Azure/radius/pkg/kubernetes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	IntervalForDeploymentCreation = 10 * time.Second
	IntervalForPodShutdown        = 10 * time.Second
	IntervalForPodCreation        = 10 * time.Second

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

func NewK8sObjectForComponent(application string, name string) K8sObject {
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

// ValidatePodsRunning validates the namespaces and pods specified in each namespace are running
func ValidatePodsRunning(ctx context.Context, t *testing.T, k8s *kubernetes.Clientset, expected K8sObjectSet) {
	for namespace, expectedPods := range expected.Namespaces {
		t.Logf("validating pods in namespace %v", namespace)
		var actualPods *corev1.PodList
		for {
			select {
			case <-time.After(IntervalForPodCreation):
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
						t.Log(t,
							"unrecognized pod, could not find a match for Pod with namespace: %v name: %v labels: %v",
							actualPod.Namespace,
							actualPod.Name,
							actualPod.Labels)
						continue
					}

					// trim the list of 'remaining' pods
					remaining = append(remaining[:*index], remaining[*index+1:]...)
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
		for _, actualPod := range actualPods.Items {
			monitor := PodMonitor{
				K8s: k8s,
				Pod: actualPod,
			}
			monitor.ValidateRunning(ctx, t)
		}
	}
}

func ValidateNoPodsInNamespace(ctx context.Context, t *testing.T, k8s *kubernetes.Clientset, namespace string) {
	actualPods, err := k8s.CoreV1().Pods(namespace).List(context.Background(), metav1.ListOptions{})
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
			t.Logf("at %s waiting for pods in namespace %s to shut down.. ", time.Now().Format("2006-01-02 15:04:05"), namespace)

			actualPods, err := k8s.CoreV1().Pods(namespace).List(context.Background(), metav1.ListOptions{})
			assert.NoErrorf(t, err, "failed to list pods in namespace %s", namespace)

			logPods(t, actualPods.Items)
			if len(actualPods.Items) == 0 {
				t.Logf("success! no pods found in namespace %s", namespace)
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
		return
	}

	t.Logf("watching pod %v for status.. current: %v", pm.Pod.Name, pm.Pod.Status)

	// If we're not in the running state, we need to wait a bit to see if things work out.
	watch, err := pm.K8s.CoreV1().Pods(pm.Pod.Namespace).Watch(ctx, metav1.SingleObject(pm.Pod.ObjectMeta))
	require.NoErrorf(t, err, "failed to watch pod: %v", pm.Pod.Name)
	defer watch.Stop()

	for {
		select {
		case <-time.After(IntervalForWatchHeartbeat):
			t.Logf("at %s watching pod %v for status.. ", time.Now().Format("2006-01-02 15:04:05"), pm.Pod.Name)

		case event := <-watch.ResultChan():
			pod, ok := event.Object.(*corev1.Pod)
			require.Truef(t, ok, "object %T is not a pod", event.Object)

			if pod.Status.Phase == corev1.PodRunning {
				t.Logf("success! pod %v has status: %v", pod.Name, pod.Status)
				return
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
