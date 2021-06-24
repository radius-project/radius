// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package validation

import (
	"context"
	"testing"
	"time"

	"github.com/Azure/radius/pkg/workloads"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const IntervalForPodShutdown = 10 * time.Second

type K8sObjectSet struct {
	Namespaces map[string][]K8sObject
}

type K8sObject struct {
	Labels map[string]string
}

func NewK8sObjectForComponent(application string, name string) K8sObject {
	return K8sObject{
		Labels: map[string]string{
			workloads.LabelRadiusApplication: application,
			workloads.LabelRadiusComponent:   name,
		},
	}
}

func ValidateDeploymentsRunning(ctx context.Context, t *testing.T, k8s *kubernetes.Clientset, expected K8sObjectSet) {
	for namespace, expectedPods := range expected.Namespaces {
		t.Logf("validating deployments in namespace %v", namespace)
		for {
			select {
			case <-time.After(10 * time.Second):
				var err error

				deployments, err := k8s.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{})
				require.NoErrorf(t, err, "failed to list pods in namespace %v", namespace)

				remaining := make([]K8sObject, len(expectedPods))
				copy(remaining, expectedPods)

				for _, actualDeployment := range deployments.Items {
					// validate that this matches one of our expected deployment
					index := matchesExpectedLabels(remaining, actualDeployment.Labels)
					if index == nil {
						// this is not a match
						assert.Failf(t,
							"unrecognized deployment",
							"count not find a match for Pod with namespace: %v name: %v labels: %v",
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
			case <-time.After(10 * time.Second):
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
						assert.Failf(t, "unrecognized pod", "count not find a match for Pod with namespace: %v name: %v labels: %v", actualPod.Namespace, actualPod.Name, actualPod.Labels)
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
			}
		}

		// Now check the status of the pods
	podcheck:
		for _, actualPod := range actualPods.Items {
			if actualPod.Status.Phase == corev1.PodRunning {
				continue
			}

			t.Logf("watching pod %v for status.. current: %v", actualPod.Name, actualPod.Status)

			// If we're not in the running state, we need to wait a bit to see if things work out.
			watch, err := k8s.CoreV1().Pods(namespace).Watch(ctx, metav1.SingleObject(actualPod.ObjectMeta))
			require.NoErrorf(t, err, "failed to watch pod: %v", actualPod.Name)
			defer watch.Stop()

		loop:
			for {
				select {
				case event := <-watch.ResultChan():
					pod, ok := event.Object.(*corev1.Pod)
					require.Truef(t, ok, "object %T is not a pod", event.Object)

					if pod.Status.Phase == corev1.PodRunning {
						t.Logf("success! pod %v has status: %v", pod.Name, pod.Status)
						break loop
					} else if pod.Status.Phase == corev1.PodFailed {
						assert.Failf(t, "pod %v entered a failing state", pod.Name)
						break loop
					}

					t.Logf("watching pod %v for status.. current: %v", pod.Name, pod.Status)

				case <-ctx.Done():
					assert.Failf(t, "timed out after waiting for pod %v to enter running status", actualPod.Name)
					break loop
				}
			}
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

		case <-time.After(IntervalForPodShutdown):
			actualPods, err := k8s.CoreV1().Pods(namespace).List(context.Background(), metav1.ListOptions{})
			assert.NoErrorf(t, err, "failed to list pods in namespace %s", namespace)

			logPods(t, actualPods.Items)
			if len(actualPods.Items) == 0 {
				// Success! pods are gone.
				return
			}
		}
	}

}

func logPods(t *testing.T, pods []corev1.Pod) {
	t.Log("Found the following pods:")
	for _, pod := range pods {
		t.Logf("namespace: %v name: %v labels: %v", pod.Namespace, pod.Name, pod.Labels)
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
