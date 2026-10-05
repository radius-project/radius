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
)

func Test_CurlArgs(t *testing.T) {
	t.Parallel()

	base := []string{"curl", "-sS", "--max-time", "30", "-w", statusMarker + "%{http_code}"}
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
				"--data-raw", `{"a":"$b"}`,
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

			got, err := curlArgs(tt.req, tt.hasTLS)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func Test_ParseCurlOutput(t *testing.T) {
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
			stdout:     `{"value":[]}` + statusMarker + "200",
			wantStatus: 200,
			wantBody:   `{"value":[]}`,
		},
		{
			name:       "empty body",
			stdout:     statusMarker + "401",
			wantStatus: 401,
		},
		{
			name:       "body containing newlines",
			stdout:     "line1\nline2\n" + statusMarker + "403\n",
			wantStatus: 403,
			wantBody:   "line1\nline2\n",
		},
		{
			name:       "no HTTP response",
			stdout:     statusMarker + "000",
			wantStatus: 0,
		},
		{
			name:    "missing marker",
			stdout:  "partial output",
			wantErr: true,
		},
		{
			name:    "non-numeric status",
			stdout:  statusMarker + "abc",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			status, body, err := parseCurlOutput(tt.stdout)
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

func Test_NewPod(t *testing.T) {
	t.Parallel()

	t.Run("defaults", func(t *testing.T) {
		t.Parallel()

		pod := newPod(Options{Namespace: "rogue-ns"})

		require.Equal(t, "rogue-ns", pod.Namespace)
		require.Equal(t, podNamePrefix, pod.GenerateName)
		require.Equal(t, labelValue, pod.Labels[labelKey])
		require.Equal(t, corev1.RestartPolicyNever, pod.Spec.RestartPolicy)
		require.False(t, *pod.Spec.AutomountServiceAccountToken)
		require.Len(t, pod.Spec.Containers, 1)
		require.Empty(t, pod.Spec.Volumes)

		container := pod.Spec.Containers[0]
		require.Equal(t, DefaultImage, container.Image)
		require.Equal(t, []string{"sleep", "infinity"}, container.Command)
		require.True(t, *container.SecurityContext.RunAsNonRoot)
		require.Equal(t, runAsUser, *container.SecurityContext.RunAsUser)
		require.False(t, *container.SecurityContext.AllowPrivilegeEscalation)
		require.Empty(t, container.VolumeMounts)
	})

	t.Run("custom image and TLS secret", func(t *testing.T) {
		t.Parallel()

		pod := newPod(Options{Namespace: "ns", Image: "example.com/curl:1", TLSSecretName: "ucp-tls"})

		container := pod.Spec.Containers[0]
		require.Equal(t, "example.com/curl:1", container.Image)
		require.Len(t, pod.Spec.Volumes, 1)
		require.Equal(t, "ucp-tls", pod.Spec.Volumes[0].Secret.SecretName)
		require.Len(t, container.VolumeMounts, 1)
		require.Equal(t, TLSMountPath, container.VolumeMounts[0].MountPath)
		require.True(t, container.VolumeMounts[0].ReadOnly)
		require.Equal(t, pod.Spec.Volumes[0].Name, container.VolumeMounts[0].Name)
	})
}
