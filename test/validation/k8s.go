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

type PodSet struct {
	Namespaces map[string][]Pod
}

type Pod struct {
	Labels map[string]string
}

func NewPodForComponent(application string, name string) Pod {
	return Pod{
		Labels: map[string]string{
			workloads.LabelRadiusApplication: application,
			workloads.LabelRadiusComponent:   name,
		},
	}
}

// ValidatePodsRunning validates the namespaces and pods specified in each namespace are running
func ValidatePodsRunning(t *testing.T, k8s *kubernetes.Clientset, expected PodSet) {
	ctx := context.Background()

	for namespace, expectedPods := range expected.Namespaces {
		t.Logf("validating pods in namespace %v", namespace)

		actualPods, err := k8s.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
		require.NoErrorf(t, err, "failed to list pods in namespace %v", namespace)

		// log all the data so its there if we need to analyze a failure
		logPods(t, actualPods.Items)

		// switch to using assert so we can validate all the details without failing fast
		assert.Equalf(t, len(expectedPods), len(actualPods.Items), "different number of pods than expected in namespace %v", namespace)

		// copy the list of expected pods so we can remove from it
		//
		// this way we "check off" each pod as it is matched
		remaining := make([]Pod, len(expectedPods))
		copy(remaining, expectedPods)

		for _, actualPod := range actualPods.Items {
			// validate that this matches one of our expected pods
			index := matchesExpectedPod(remaining, actualPod)
			if index == nil {
				// this is not a match
				assert.Failf(t, "count not find a match for Pod with namespace: %v name: %v labels: %v", actualPod.Namespace, actualPod.Name, actualPod.Labels)
				continue
			}

			// trim the list of 'remaining' pods
			remaining = append(remaining[:*index], remaining[*index+1:]...)
		}

		for _, remainingPod := range remaining {
			assert.Failf(t, "failed to match pod in namespace %v with labels %v", namespace, remainingPod.Labels)
		}

		// Now check the status of the pods
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

				// allow max of 60 seconds to pass without updates
				case <-time.After(60 * time.Second):
					assert.Failf(t, "timed out after waiting for pod %v to enter running status", actualPod.Name)
					break loop
				}
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
func matchesExpectedPod(expectedPods []Pod, actualPod corev1.Pod) *int {
	for index, expectedPod := range expectedPods {

		// we don't need to match all of the labels on the expected pod
		matchesPod := true
		for key, value := range expectedPod.Labels {
			// just in case
			if actualPod.Labels == nil {
				return nil
			}

			if actualPod.Labels[key] != value {
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
