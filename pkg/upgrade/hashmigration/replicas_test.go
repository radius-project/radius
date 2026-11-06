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

package hashmigration

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func replicaTestPod(name string, app string, image string, phase corev1.PodPhase, terminating bool) *corev1.Pod {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: DefaultNamespace, Labels: map[string]string{LabelAppName: app}},
		Spec: corev1.PodSpec{Containers: []corev1.Container{
			{Name: "sidecar", Image: "sidecar:1"},
			{Name: app, Image: image},
		}},
		Status: corev1.PodStatus{Phase: phase},
	}
	if terminating {
		now := metav1.Now()
		pod.DeletionTimestamp = &now
		pod.Finalizers = []string{"test/keep"}
	}
	return pod
}

func Test_OtherReleasePods(t *testing.T) {
	t.Parallel()

	const app = "applications-rp"
	self := replicaTestPod("rp-new-1", app, "rp:v2", corev1.PodRunning, false)
	for _, tt := range []struct {
		name     string
		pods     []runtime.Object
		self     string
		expected []string
	}{
		{name: "rollout finished", pods: []runtime.Object{self, replicaTestPod("rp-new-2", app, "rp:v2", corev1.PodRunning, false)}, self: self.Name, expected: []string{}},
		{
			name: "older replicas still run",
			pods: []runtime.Object{
				self,
				replicaTestPod("rp-old-2", app, "rp:v1", corev1.PodRunning, false),
				replicaTestPod("rp-old-1", app, "rp:v1", corev1.PodPending, false),
			},
			self:     self.Name,
			expected: []string{"rp-old-1", "rp-old-2"},
		},
		{name: "older replica terminating", pods: []runtime.Object{self, replicaTestPod("rp-old", app, "rp:v1", corev1.PodRunning, true)}, self: self.Name, expected: []string{"rp-old"}},
		{
			name: "older replicas stopped",
			pods: []runtime.Object{
				self,
				replicaTestPod("rp-evicted", app, "rp:v1", corev1.PodFailed, false),
				replicaTestPod("rp-done", app, "rp:v1", corev1.PodSucceeded, false),
			},
			self:     self.Name,
			expected: []string{},
		},
		{name: "pods of other components", pods: []runtime.Object{self, replicaTestPod("ucp-old", "ucp", "ucp:v1", corev1.PodRunning, false)}, self: self.Name, expected: []string{}},
		{name: "not running in Kubernetes", pods: []runtime.Object{replicaTestPod("rp-old", app, "rp:v1", corev1.PodRunning, false)}, self: "laptop"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			others, err := OtherReleasePods(t.Context(), fake.NewClientset(tt.pods...), DefaultNamespace, app, tt.self)
			require.NoError(t, err)
			require.Equal(t, tt.expected, others)
		})
	}

	t.Run("pods cannot be listed", func(t *testing.T) {
		t.Parallel()
		for _, tt := range []struct {
			name    string
			err     error
			wantErr bool
		}{
			{name: "forbidden", err: apierrors.NewForbidden(schema.GroupResource{Resource: "pods"}, "", errors.New("denied"))},
			{name: "unavailable", err: apierrors.NewServiceUnavailable("unavailable"), wantErr: true},
		} {
			client := fake.NewClientset(self)
			client.PrependReactor("list", "pods", func(k8stesting.Action) (bool, runtime.Object, error) { return true, nil, tt.err })
			others, err := OtherReleasePods(t.Context(), client, DefaultNamespace, app, self.Name)
			if tt.wantErr {
				require.Error(t, err, tt.name)
			} else {
				require.NoError(t, err, tt.name)
				require.Nil(t, others, tt.name)
			}
		}
	})
}
