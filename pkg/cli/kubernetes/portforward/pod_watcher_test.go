// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package portforward

import (
	"context"
	"testing"

	"github.com/project-radius/radius/pkg/kubernetes"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_podWatcher_CanShutdownGracefully(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	pod := &corev1.Pod{
		ObjectMeta: v1.ObjectMeta{
			Name: "test-app-test-container-abcd-efghij",
			Labels: map[string]string{
				kubernetes.LabelRadiusResource: "test-container",
			},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				corev1.Container{
					Ports: []corev1.ContainerPort{
						corev1.ContainerPort{
							ContainerPort: 3000,
						},
					},
				},
			},
		},
	}

	statusChan := make(chan StatusMessage, 10)
	pw := NewPodWatcher(Options{StatusChan: statusChan}, pod, cancel)

	// Simulate success
	pw.forwarderOverride = NewFakeForwarder
	go func() { _ = pw.Run(ctx) }()

	messages := []StatusMessage{}
	messages = append(messages, <-statusChan)

	cancel()

	messages = append(messages, <-statusChan)

	expected := []StatusMessage{
		StatusMessage{
			Kind:          KindConnected,
			ContainerName: "test-container",
			ReplicaName:   "test-app-test-container-abcd-efghij",
			LocalPort:     3000,
			RemotePort:    3000,
		},
		StatusMessage{
			Kind:          KindDisconnected,
			ContainerName: "test-container",
			ReplicaName:   "test-app-test-container-abcd-efghij",
			LocalPort:     3000,
			RemotePort:    3000,
		},
	}
	require.Equal(t, expected, messages)
	pw.Wait()
}

func Test_podWatcher_CanStartWhenPodIsReady(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	pod := &corev1.Pod{
		ObjectMeta: v1.ObjectMeta{
			Name: "test-app-test-container-abcd-efghij",
			Labels: map[string]string{
				kubernetes.LabelRadiusResource: "test-container",
			},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodUnknown,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				corev1.Container{
					Ports: []corev1.ContainerPort{
						corev1.ContainerPort{
							ContainerPort: 3000,
						},
					},
				},
			},
		},
	}

	statusChan := make(chan StatusMessage, 10) // Copy pod to avoid data-race
	pw := NewPodWatcher(Options{StatusChan: statusChan}, pod.DeepCopy(), cancel)

	// Simulate success
	pw.forwarderOverride = NewFakeForwarder
	go func() { _ = pw.Run(ctx) }()

	pod.Status.Phase = corev1.PodRunning
	pw.Updated <- pod.DeepCopy() // Copy to avoid data-race

	messages := []StatusMessage{}
	messages = append(messages, <-statusChan)

	cancel()

	messages = append(messages, <-statusChan)

	expected := []StatusMessage{
		StatusMessage{
			Kind:          KindConnected,
			ContainerName: "test-container",
			ReplicaName:   "test-app-test-container-abcd-efghij",
			LocalPort:     3000,
			RemotePort:    3000,
		},
		StatusMessage{
			Kind:          KindDisconnected,
			ContainerName: "test-container",
			ReplicaName:   "test-app-test-container-abcd-efghij",
			LocalPort:     3000,
			RemotePort:    3000,
		},
	}
	require.Equal(t, expected, messages)
	pw.Wait()
}
