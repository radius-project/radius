/*
Copyright 2026 The Radius Authors.

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

package upgrade_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"

	"github.com/distribution/reference"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

type deploymentEngineImage struct {
	repository string
	tag        string
}

func deploymentEngineImageFromEnv() (deploymentEngineImage, error) {
	image := deploymentEngineImage{repository: os.Getenv("DE_IMAGE"), tag: os.Getenv("DE_TAG")}
	if (image.repository == "") != (image.tag == "") {
		return deploymentEngineImage{}, fmt.Errorf("DE_IMAGE and DE_TAG must both be set or both be empty")
	}
	return image, nil
}

func (image deploymentEngineImage) reference() string {
	if image.repository == "" {
		return ""
	}

	// Match radius.image in the chart, including qualified references that already
	// carry a tag or digest. A port in the registry is not an embedded image tag.
	first, _, hasPath := strings.Cut(image.repository, "/")
	if hasPath && (strings.ContainsAny(first, ".:") || first == "localhost") {
		last := image.repository[strings.LastIndex(image.repository, "/")+1:]
		if strings.Contains(last, ":") {
			return image.repository
		}
		return image.repository + ":" + image.tag
	}
	return "ghcr.io/radius-project/" + image.repository + ":" + image.tag
}

type deploymentEngineImageObservation struct {
	podName      string
	expected     string
	image        string
	runtimeImage string
	imageID      string
}

func requireDeploymentEngineImage(t *testing.T, ctx context.Context, client kubernetes.Interface, image deploymentEngineImage, stage string) {
	t.Helper()

	observations, err := deploymentEngineImages(ctx, client, image.reference())
	for _, observation := range observations {
		imageID := observation.imageID
		if imageID == "" {
			imageID = "unavailable"
		}
		t.Logf("Deployment Engine after %s: pod=%s/%s expected=%q image=%q runtimeImage=%q imageID=%q",
			stage, radiusNamespace, observation.podName, observation.expected, observation.image, observation.runtimeImage, imageID)
	}
	require.NoErrorf(t, err, "Deployment Engine image verification failed after %s", stage)
}

func deploymentEngineImages(ctx context.Context, client kubernetes.Interface, expected string) ([]deploymentEngineImageObservation, error) {
	deployment, err := client.AppsV1().Deployments(radiusNamespace).Get(ctx, "bicep-de", metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("get Deployment Engine deployment: %w", err)
	}
	index := slices.IndexFunc(deployment.Spec.Template.Spec.Containers, func(container corev1.Container) bool {
		return container.Name == "de"
	})
	if index < 0 {
		return nil, fmt.Errorf("deployment %s/bicep-de has no de container", radiusNamespace)
	}
	deploymentImage := deployment.Spec.Template.Spec.Containers[index].Image
	if expected == "" {
		expected = deploymentImage
	}
	if deploymentImage == "" || deploymentImage != expected {
		return nil, fmt.Errorf("deployment %s/bicep-de image is %q, expected %q", radiusNamespace, deploymentImage, expected)
	}

	if deployment.Spec.Selector == nil {
		return nil, fmt.Errorf("deployment %s/bicep-de has no pod selector", radiusNamespace)
	}
	selector, err := metav1.LabelSelectorAsSelector(deployment.Spec.Selector)
	if err != nil {
		return nil, fmt.Errorf("read Deployment Engine pod selector: %w", err)
	}
	if selector.Empty() {
		return nil, fmt.Errorf("deployment %s/bicep-de has an empty pod selector", radiusNamespace)
	}
	pods, err := client.CoreV1().Pods(radiusNamespace).List(ctx, metav1.ListOptions{LabelSelector: selector.String()})
	if err != nil {
		return nil, fmt.Errorf("list Deployment Engine pods: %w", err)
	}

	var observations []deploymentEngineImageObservation
	for _, pod := range pods.Items {
		if pod.DeletionTimestamp != nil {
			continue
		}
		index := slices.IndexFunc(pod.Spec.Containers, func(container corev1.Container) bool {
			return container.Name == "de"
		})
		if index < 0 {
			return observations, fmt.Errorf("pod %s/%s has no de container", radiusNamespace, pod.Name)
		}
		statusIndex := slices.IndexFunc(pod.Status.ContainerStatuses, func(status corev1.ContainerStatus) bool {
			return status.Name == "de"
		})
		if statusIndex < 0 {
			return observations, fmt.Errorf("pod %s/%s has no de container status", radiusNamespace, pod.Name)
		}
		status := pod.Status.ContainerStatuses[statusIndex]
		observation := deploymentEngineImageObservation{
			podName:      pod.Name,
			expected:     expected,
			image:        pod.Spec.Containers[index].Image,
			runtimeImage: status.Image,
			imageID:      status.ImageID,
		}
		observations = append(observations, observation)
		if observation.image != expected {
			return observations, fmt.Errorf("pod %s/%s de image is %q, expected %q", radiusNamespace, pod.Name, observation.image, expected)
		}
		if !deploymentEngineRuntimeImageMatches(expected, status.Image) {
			return observations, fmt.Errorf("pod %s/%s de runtime image is %q, expected %q", radiusNamespace, pod.Name, status.Image, expected)
		}
		if status.State.Running == nil || !status.Ready {
			return observations, fmt.Errorf("pod %s/%s de container is not running and ready", radiusNamespace, pod.Name)
		}
	}
	if len(observations) == 0 {
		return nil, fmt.Errorf("no active Deployment Engine pods found in %s", radiusNamespace)
	}
	return observations, nil
}

func deploymentEngineRuntimeImageMatches(expectedImage, runtimeImage string) bool {
	expected, err := reference.ParseAnyReference(expectedImage)
	if err != nil {
		return false
	}
	actual, err := reference.ParseAnyReference(runtimeImage)
	if err != nil {
		return false
	}
	if expectedNamed, ok := expected.(reference.Named); ok {
		if actualNamed, ok := actual.(reference.Named); ok {
			if expectedNamed.Name() != actualNamed.Name() {
				return false
			}
			expected = reference.TagNameOnly(expectedNamed)
			actual = reference.TagNameOnly(actualNamed)
		}
	}
	expectedDigest, expectedPinned := expected.(reference.Digested)
	actualDigest, actualPinned := actual.(reference.Digested)
	if expectedPinned {
		return actualPinned && expectedDigest.Digest() == actualDigest.Digest()
	}
	// Some runtimes report a resolved digest instead of the requested tag. Without
	// an expected digest, verify the repository when present and record the ID.
	if actualPinned {
		return true
	}
	return expected.String() == actual.String()
}

func Test_deploymentEngineImageFromEnv(t *testing.T) {
	for _, tt := range []struct {
		name       string
		repository string
		tag        string
		wantErr    bool
	}{
		{name: "chart defaults"},
		{name: "candidate", repository: "candidate.example:5001/custom-de", tag: "00123"},
		{name: "repository only", repository: "candidate.example/custom-de", wantErr: true},
		{name: "tag only", tag: "candidate-13081", wantErr: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("DE_IMAGE", tt.repository)
			t.Setenv("DE_TAG", tt.tag)
			image, err := deploymentEngineImageFromEnv()
			if tt.wantErr {
				require.EqualError(t, err, "DE_IMAGE and DE_TAG must both be set or both be empty")
				require.Zero(t, image)
				return
			}
			require.NoError(t, err)
			require.Equal(t, deploymentEngineImage{repository: tt.repository, tag: tt.tag}, image)
		})
	}
}

func Test_deploymentEngineImagePartialInputFailsBeforeCleanup(t *testing.T) {
	for _, variable := range []string{"DE_IMAGE", "DE_TAG"} {
		t.Run(variable, func(t *testing.T) {
			t.Setenv("DE_IMAGE", "")
			t.Setenv("DE_TAG", "")
			t.Setenv(variable, "candidate")
			// Even a regression must not access a real cluster or run Helm.
			t.Setenv("KUBECONFIG", t.TempDir()+"/missing-kubeconfig")
			t.Setenv("PATH", t.TempDir())
			cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^Test_PreflightContainer$", "-test.v")
			output, err := cmd.CombinedOutput()
			require.Error(t, err)
			require.Contains(t, string(output), "DE_IMAGE and DE_TAG must both be set or both be empty")
			require.NotContains(t, string(output), "Failed to create Kubernetes client")
			require.NotContains(t, string(output), "Cleaning up any existing Radius installation")
		})
	}
}

func Test_deploymentEngineImageReference(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name       string
		repository string
		tag        string
		want       string
	}{
		{name: "chart default"},
		{name: "qualified", repository: "candidate.example/custom-de", tag: "candidate", want: "candidate.example/custom-de:candidate"},
		{name: "registry port", repository: "candidate.example:5001/custom-de", tag: "00123", want: "candidate.example:5001/custom-de:00123"},
		{name: "localhost", repository: "localhost/custom-de", tag: "candidate", want: "localhost/custom-de:candidate"},
		{name: "short name", repository: "deployment-engine", tag: "candidate", want: "ghcr.io/radius-project/deployment-engine:candidate"},
		{name: "nested short name", repository: "custom/deployment-engine", tag: "candidate", want: "ghcr.io/radius-project/custom/deployment-engine:candidate"},
		{name: "embedded tag", repository: "candidate.example:5001/custom-de:pinned", tag: "ignored", want: "candidate.example:5001/custom-de:pinned"},
		{name: "embedded digest", repository: "candidate.example/custom-de@sha256:abc", tag: "ignored", want: "candidate.example/custom-de@sha256:abc"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			image := deploymentEngineImage{repository: tt.repository, tag: tt.tag}
			require.Equal(t, tt.want, image.reference())
		})
	}
}

func Test_deploymentEngineRuntimeImageMatches(t *testing.T) {
	t.Parallel()
	digest := "sha256:" + strings.Repeat("a", 64)
	otherDigest := "sha256:" + strings.Repeat("b", 64)
	for _, tt := range []struct {
		name     string
		expected string
		actual   string
		want     bool
	}{
		{name: "same candidate", expected: "candidate.example/de:pr-13081", actual: "candidate.example/de:pr-13081", want: true},
		{name: "fallback tag", expected: "candidate.example/de:pr-13081", actual: "candidate.example/de:latest"},
		{name: "wrong repository", expected: "candidate.example/de:pr-13081", actual: "ghcr.io/radius-project/deployment-engine:pr-13081"},
		{name: "normalized Docker Hub", expected: "docker.io/library/de:latest", actual: "de", want: true},
		{name: "resolved digest", expected: "candidate.example/de:pr-13081", actual: "candidate.example/de@" + digest, want: true},
		{name: "opaque digest", expected: "candidate.example/de:pr-13081", actual: digest, want: true},
		{name: "wrong resolved repository", expected: "candidate.example/de:pr-13081", actual: "wrong.example/de@" + digest},
		{name: "pinned digest", expected: "candidate.example/de@" + digest, actual: "candidate.example/de@" + digest, want: true},
		{name: "wrong pinned digest", expected: "candidate.example/de@" + digest, actual: "candidate.example/de@" + otherDigest},
		{name: "missing runtime image", expected: "candidate.example/de:pr-13081"},
		{name: "invalid runtime image", expected: "candidate.example/de:pr-13081", actual: "not an image"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, deploymentEngineRuntimeImageMatches(tt.expected, tt.actual))
		})
	}
}

func Test_deploymentEngineImages(t *testing.T) {
	t.Parallel()

	const candidate = "candidate.example:5001/custom-de:candidate-13081"
	const chartDefault = "ghcr.io/radius-project/deployment-engine:latest"
	const imageID = "containerd://sha256:resolved-candidate"
	for _, tt := range []struct {
		name      string
		change    func(*appsv1.Deployment, *corev1.Pod)
		extraPods func(*corev1.Pod) []runtime.Object
		defaults  bool
		noPod     bool
		wantErr   string
		wantImage string
		wantNoID  bool
		wantCount int
	}{
		{name: "candidate", wantCount: 1},
		{
			name: "chart defaults",
			change: func(deployment *appsv1.Deployment, pod *corev1.Pod) {
				deployment.Spec.Template.Spec.Containers[1].Image = chartDefault
				pod.Spec.Containers[1].Image = chartDefault
				pod.Status.ContainerStatuses[1].Image = chartDefault
			},
			defaults: true, wantImage: chartDefault, wantCount: 1,
		},
		{
			name: "chart fallback",
			change: func(deployment *appsv1.Deployment, pod *corev1.Pod) {
				deployment.Spec.Template.Spec.Containers[1].Image = chartDefault
				pod.Spec.Containers[1].Image = chartDefault
			},
			wantErr: "deployment radius-system/bicep-de image is",
		},
		{
			name: "wrong deployment tag",
			change: func(deployment *appsv1.Deployment, _ *corev1.Pod) {
				deployment.Spec.Template.Spec.Containers[1].Image = "candidate.example:5001/custom-de:wrong"
			},
			wantErr: "deployment radius-system/bicep-de image is",
		},
		{
			name: "wrong pod image",
			change: func(_ *appsv1.Deployment, pod *corev1.Pod) {
				pod.Spec.Containers[1].Image = chartDefault
			},
			wantErr: "pod radius-system/bicep-de-test de image is",
		},
		{
			name: "wrong runtime image",
			change: func(_ *appsv1.Deployment, pod *corev1.Pod) {
				pod.Status.ContainerStatuses[1].Image = chartDefault
			},
			wantErr: "pod radius-system/bicep-de-test de runtime image is",
		},
		{
			name: "missing deployment container",
			change: func(deployment *appsv1.Deployment, _ *corev1.Pod) {
				deployment.Spec.Template.Spec.Containers = nil
			},
			wantErr: "has no de container",
		},
		{
			name: "empty default deployment image",
			change: func(deployment *appsv1.Deployment, _ *corev1.Pod) {
				deployment.Spec.Template.Spec.Containers[1].Image = ""
			},
			defaults: true, wantErr: "deployment radius-system/bicep-de image is",
		},
		{
			name: "missing pod container",
			change: func(_ *appsv1.Deployment, pod *corev1.Pod) {
				pod.Spec.Containers = pod.Spec.Containers[:1]
			},
			wantErr: "has no de container",
		},
		{
			name: "missing container status",
			change: func(_ *appsv1.Deployment, pod *corev1.Pod) {
				pod.Status.ContainerStatuses = pod.Status.ContainerStatuses[:1]
			},
			wantErr: "has no de container status",
		},
		{
			name: "not running",
			change: func(_ *appsv1.Deployment, pod *corev1.Pod) {
				pod.Status.ContainerStatuses[1].State.Running = nil
			},
			wantErr: "is not running and ready",
		},
		{
			name: "not ready",
			change: func(_ *appsv1.Deployment, pod *corev1.Pod) {
				pod.Status.ContainerStatuses[1].Ready = false
			},
			wantErr: "is not running and ready",
		},
		{
			name: "image ID unavailable",
			change: func(_ *appsv1.Deployment, pod *corev1.Pod) {
				pod.Status.ContainerStatuses[1].ImageID = ""
			},
			wantNoID: true, wantCount: 1,
		},
		{name: "no pods", noPod: true, wantErr: "no active Deployment Engine pods"},
		{
			name: "only terminating pods",
			change: func(_ *appsv1.Deployment, pod *corev1.Pod) {
				now := metav1.Now()
				pod.DeletionTimestamp = &now
			},
			wantErr: "no active Deployment Engine pods",
		},
		{
			name: "multiple pods",
			extraPods: func(pod *corev1.Pod) []runtime.Object {
				second := pod.DeepCopy()
				second.Name = "second"
				return []runtime.Object{second}
			},
			wantCount: 2,
		},
		{
			name: "mismatched second pod",
			extraPods: func(pod *corev1.Pod) []runtime.Object {
				second := pod.DeepCopy()
				second.Name = "second"
				second.Spec.Containers[1].Image = chartDefault
				return []runtime.Object{second}
			},
			wantErr: "pod radius-system/second de image is",
		},
		{
			name: "ignore terminating and unrelated pods",
			extraPods: func(pod *corev1.Pod) []runtime.Object {
				terminating := pod.DeepCopy()
				terminating.Name = "terminating"
				terminating.Spec.Containers[1].Image = chartDefault
				now := metav1.Now()
				terminating.DeletionTimestamp = &now
				unrelated := terminating.DeepCopy()
				unrelated.Name = "unrelated"
				unrelated.DeletionTimestamp = nil
				unrelated.Labels = map[string]string{"app.kubernetes.io/name": "ucp"}
				return []runtime.Object{terminating, unrelated}
			},
			wantCount: 1,
		},
		{
			name: "missing selector",
			change: func(deployment *appsv1.Deployment, _ *corev1.Pod) {
				deployment.Spec.Selector = nil
			},
			wantErr: "has no pod selector",
		},
		{
			name: "empty selector",
			change: func(deployment *appsv1.Deployment, _ *corev1.Pod) {
				deployment.Spec.Selector = &metav1.LabelSelector{}
			},
			wantErr: "has an empty pod selector",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			containers := []corev1.Container{{Name: "sidecar", Image: "unrelated"}, {Name: "de", Image: candidate}}
			deployment := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "bicep-de", Namespace: radiusNamespace},
				Spec: appsv1.DeploymentSpec{
					Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app.kubernetes.io/name": "bicep-de"}},
					Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: slices.Clone(containers)}},
				},
			}
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "bicep-de-test", Namespace: radiusNamespace,
					Labels: map[string]string{"app.kubernetes.io/name": "bicep-de"},
				},
				Spec: corev1.PodSpec{Containers: containers},
				Status: corev1.PodStatus{ContainerStatuses: []corev1.ContainerStatus{
					{Name: "sidecar"},
					{Name: "de", Image: candidate, ImageID: imageID, Ready: true, State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}},
				}},
			}
			if tt.change != nil {
				tt.change(deployment, pod)
			}
			objects := []runtime.Object{deployment}
			if !tt.noPod {
				objects = append(objects, pod)
			}
			if tt.extraPods != nil {
				objects = append(objects, tt.extraPods(pod)...)
			}
			expected := candidate
			if tt.defaults {
				expected = ""
			}
			observations, err := deploymentEngineImages(t.Context(), fake.NewClientset(objects...), expected)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.Len(t, observations, tt.wantCount)
			wantImage := tt.wantImage
			if wantImage == "" {
				wantImage = candidate
			}
			for _, observation := range observations {
				require.NotEmpty(t, observation.podName)
				require.Equal(t, wantImage, observation.expected)
				require.Equal(t, wantImage, observation.image)
				require.Equal(t, wantImage, observation.runtimeImage)
				if tt.wantNoID {
					require.Empty(t, observation.imageID)
				} else {
					require.Equal(t, imageID, observation.imageID)
				}
			}
		})
	}
}

func Test_deploymentEngineImagesAPIErrors(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name     string
		verb     string
		resource string
	}{
		{name: "missing deployment"},
		{name: "deployment get error", verb: "get", resource: "deployments"},
		{name: "pod list error", verb: "list", resource: "pods"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			client := fake.NewClientset()
			if tt.verb != "" {
				deployment := &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{Name: "bicep-de", Namespace: radiusNamespace},
					Spec: appsv1.DeploymentSpec{
						Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app.kubernetes.io/name": "bicep-de"}},
						Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "de", Image: "candidate"}}}},
					},
				}
				require.NoError(t, client.Tracker().Add(deployment))
				failure := errors.New("API unavailable")
				client.PrependReactor(tt.verb, tt.resource, func(k8stesting.Action) (bool, runtime.Object, error) {
					return true, nil, failure
				})
				_, err := deploymentEngineImages(t.Context(), client, "candidate")
				require.ErrorIs(t, err, failure)
				return
			}
			_, err := deploymentEngineImages(t.Context(), client, "candidate")
			require.ErrorContains(t, err, "get Deployment Engine deployment")
		})
	}
}
