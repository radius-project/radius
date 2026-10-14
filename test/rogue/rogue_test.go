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

package rogue

import (
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"

	"github.com/radius-project/radius/test/rogue/rogueclient"
)

func Test_ClientArgs(t *testing.T) {
	t.Parallel()

	base := []string{clientBinary, "request", "--max-time", "30s"}
	with := func(args ...string) []string {
		return append(append([]string{}, base...), args...)
	}

	tests := []struct {
		name    string
		req     Request
		hasTLS  bool
		want    []string
		wantErr string
	}{
		{
			name: "defaults to GET",
			req:  Request{URL: "http://applications-rp.radius-system:5443/apis"},
			want: with("-X", "GET", "http://applications-rp.radius-system:5443/apis"),
		},
		{
			name: "headers are sorted and body is sent raw",
			req: Request{
				Method:  "PUT",
				URL:     "http://x/y",
				Headers: map[string]string{"x-remote-user": "admin", "Content-Type": "application/json"},
				Body:    `{"a":"$b"}`,
			},
			want: with(
				"-X", "PUT",
				"-H", "Content-Type: application/json",
				"-H", "x-remote-user: admin",
				"--data", `{"a":"$b"}`,
				"http://x/y"),
		},
		{
			name:   "client certificate and CA from the mounted secret",
			req:    Request{URL: "https://ucp.radius-system/apis", ClientCert: true, CACert: true},
			hasTLS: true,
			want: with(
				"-X", "GET",
				"--cert", TLSMountPath+"/tls.crt",
				"--key", TLSMountPath+"/tls.key",
				"--cacert", TLSMountPath+"/ca.crt",
				"https://ucp.radius-system/apis"),
		},
		{
			name: "insecure skips server verification",
			req:  Request{URL: "https://x", Insecure: true},
			want: with("-X", "GET", "-k", "https://x"),
		},
		{
			name:    "URL is required",
			req:     Request{},
			wantErr: "URL is required",
		},
		{
			name:    "client certificate requires a TLS secret",
			req:     Request{URL: "https://x", ClientCert: true},
			wantErr: "TLSSecretName",
		},
		{
			name:    "CA certificate requires a TLS secret",
			req:     Request{URL: "https://x", CACert: true},
			wantErr: "TLSSecretName",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := clientArgs(tt.req, tt.hasTLS)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func Test_ParseOutput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		stdout     string
		wantStatus int
		wantBody   string
		wantErr    bool
	}{
		{
			name:       "status and body",
			stdout:     `{"value":[]}` + rogueclient.StatusMarker + "200",
			wantStatus: 200,
			wantBody:   `{"value":[]}`,
		},
		{
			name:       "empty body",
			stdout:     rogueclient.StatusMarker + "401",
			wantStatus: 401,
		},
		{
			name:       "body containing newlines",
			stdout:     "line1\nline2\n" + rogueclient.StatusMarker + "403\n",
			wantStatus: 403,
			wantBody:   "line1\nline2\n",
		},
		{
			name:       "no HTTP response",
			stdout:     rogueclient.StatusMarker + "0",
			wantStatus: 0,
		},
		{
			name:    "missing marker",
			stdout:  "partial output",
			wantErr: true,
		},
		{
			name:    "non-numeric status",
			stdout:  rogueclient.StatusMarker + "abc",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			status, body, err := parseOutput(tt.stdout)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.wantStatus, status)
			require.Equal(t, tt.wantBody, body)
		})
	}
}

func Test_DefaultImage(t *testing.T) {
	t.Run("uses the test registry and tag", func(t *testing.T) {
		t.Setenv("DOCKER_REGISTRY", "radius-registry:5000")
		t.Setenv("REL_VERSION", "pr-123")
		require.Equal(t, "radius-registry:5000/rogue-client:pr-123", DefaultImage())
		require.Equal(t, DefaultImage(), newPod(Options{Namespace: "ns"}).Spec.Containers[0].Image)
	})

	t.Run("defaults to the published latest image", func(t *testing.T) {
		t.Setenv("DOCKER_REGISTRY", "")
		t.Setenv("REL_VERSION", "")
		require.Equal(t, "ghcr.io/radius-project/rogue-client:latest", DefaultImage())
	})
}

func Test_NewPod(t *testing.T) {
	t.Parallel()

	t.Run("defaults", func(t *testing.T) {
		t.Parallel()

		pod := newPod(Options{Namespace: "rogue-ns", Image: "registry/rogue-client:tag"})

		require.Equal(t, "rogue-ns", pod.Namespace)
		require.Equal(t, podNamePrefix, pod.GenerateName)
		require.Equal(t, labelValue, pod.Labels[labelKey])
		require.Equal(t, corev1.RestartPolicyNever, pod.Spec.RestartPolicy)
		require.False(t, *pod.Spec.AutomountServiceAccountToken)
		require.Len(t, pod.Spec.Containers, 1)
		require.Empty(t, pod.Spec.Volumes)

		container := pod.Spec.Containers[0]
		require.Equal(t, "registry/rogue-client:tag", container.Image)
		require.Equal(t, []string{clientBinary, "idle"}, container.Command)
		require.True(t, *container.SecurityContext.RunAsNonRoot)
		require.Equal(t, int64(65532), *container.SecurityContext.RunAsUser)
		require.True(t, *container.SecurityContext.ReadOnlyRootFilesystem)
		require.False(t, *container.SecurityContext.AllowPrivilegeEscalation)
		require.Equal(t, []corev1.Capability{"ALL"}, container.SecurityContext.Capabilities.Drop)
		require.Equal(t, corev1.SeccompProfileTypeRuntimeDefault, container.SecurityContext.SeccompProfile.Type)
		require.Empty(t, container.VolumeMounts)
	})

	t.Run("custom image and TLS secret", func(t *testing.T) {
		t.Parallel()

		pod := newPod(Options{Namespace: "ns", Image: "example.com/rogue:1", TLSSecretName: "ucp-tls"})

		container := pod.Spec.Containers[0]
		require.Equal(t, "example.com/rogue:1", container.Image)
		require.Len(t, pod.Spec.Volumes, 1)
		require.Equal(t, "ucp-tls", pod.Spec.Volumes[0].Secret.SecretName)
		require.Len(t, container.VolumeMounts, 1)
		require.Equal(t, TLSMountPath, container.VolumeMounts[0].MountPath)
		require.True(t, container.VolumeMounts[0].ReadOnly)
		require.Equal(t, pod.Spec.Volumes[0].Name, container.VolumeMounts[0].Name)
	})
}

func Test_PodWaitingState(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		pod  *corev1.Pod
		want string
	}{
		{
			name: "waiting container includes reason and message",
			pod: &corev1.Pod{
				Status: corev1.PodStatus{
					ContainerStatuses: []corev1.ContainerStatus{{
						Name:  "rogue",
						State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "ErrImagePull", Message: "pull denied"}},
					}},
				},
			},
			want: `container "rogue" is waiting: ErrImagePull: pull denied`,
		},
		{
			name: "waiting init container is reported first",
			pod: &corev1.Pod{
				Status: corev1.PodStatus{
					InitContainerStatuses: []corev1.ContainerStatus{{
						Name:  "init",
						State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CreateContainerConfigError"}},
					}},
					ContainerStatuses: []corev1.ContainerStatus{{
						Name:  "rogue",
						State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "ContainerCreating"}},
					}},
				},
			},
			want: `init container "init" is waiting: CreateContainerConfigError`,
		},
		{
			name: "empty when no container is waiting",
			pod: &corev1.Pod{
				Status: corev1.PodStatus{
					ContainerStatuses: []corev1.ContainerStatus{{
						Name:  "rogue",
						State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}},
					}},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tt.want, podWaitingState(tt.pod))
		})
	}
}

func Test_PodStartFailure(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		pod     *corev1.Pod
		wantErr string
	}{
		{
			name: "image pull is fatal",
			pod: &corev1.Pod{
				Status: corev1.PodStatus{
					ContainerStatuses: []corev1.ContainerStatus{{
						Name:  "rogue",
						State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "ImagePullBackOff", Message: "pull denied"}},
					}},
				},
			},
			wantErr: `container "rogue" is waiting: ImagePullBackOff: pull denied`,
		},
		{
			name: "missing secret is fatal",
			pod: &corev1.Pod{
				Status: corev1.PodStatus{
					ContainerStatuses: []corev1.ContainerStatus{{
						Name:  "rogue",
						State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CreateContainerConfigError", Message: "secret not found"}},
					}},
				},
			},
			wantErr: `container "rogue" is waiting: CreateContainerConfigError: secret not found`,
		},
		{
			name: "container creating is not fatal",
			pod: &corev1.Pod{
				Status: corev1.PodStatus{
					ContainerStatuses: []corev1.ContainerStatus{{
						Name:  "rogue",
						State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "ContainerCreating"}},
					}},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := podStartFailure(tt.pod)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
		})
	}
}
