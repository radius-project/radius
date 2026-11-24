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
	"time"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

// gateTestDeployment returns the Deployment of app at revision, which the Deployment controller has observed.
func gateTestDeployment(app string, revision string) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: app, Namespace: DefaultNamespace, UID: types.UID(app), Generation: 2,
			Annotations: map[string]string{annotationRevision: revision},
		},
		Spec:   appsv1.DeploymentSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: app, Image: app + ":" + revision}}}}},
		Status: appsv1.DeploymentStatus{ObservedGeneration: 2},
	}
}

// gateTestReplicaSet returns the ReplicaSet of revision of deployment, named after the revision.
func gateTestReplicaSet(deployment *appsv1.Deployment, revision string) *appsv1.ReplicaSet {
	name := deployment.Name + "-" + revision
	return &appsv1.ReplicaSet{ObjectMeta: metav1.ObjectMeta{
		Name: name, Namespace: DefaultNamespace, UID: types.UID(name),
		Annotations:     map[string]string{annotationRevision: revision},
		OwnerReferences: []metav1.OwnerReference{*metav1.NewControllerRef(deployment, appsv1.SchemeGroupVersion.WithKind("Deployment"))},
	}}
}

// gateTestPod returns a pod of app that the ReplicaSet named replicaSet owns, or no ReplicaSet if it is "".
func gateTestPod(name string, app string, replicaSet string, phase corev1.PodPhase, terminating bool) *corev1.Pod {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: DefaultNamespace, Labels: map[string]string{LabelAppName: app}},
		Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: app, Image: app + ":any"}}},
		Status:     corev1.PodStatus{Phase: phase},
	}
	if replicaSet != "" {
		pod.OwnerReferences = []metav1.OwnerReference{{APIVersion: "apps/v1", Kind: "ReplicaSet", Name: replicaSet, UID: types.UID(replicaSet), Controller: new(true)}}
	}
	if terminating {
		now := metav1.Now()
		pod.DeletionTimestamp = &now
		pod.Finalizers = []string{"test/keep"}
	}
	return pod
}

func Test_OutdatedPods(t *testing.T) {
	t.Parallel()

	const self = "ucp-new"
	running := corev1.PodRunning
	ucp := gateTestDeployment("ucp", "2")
	rp := gateTestDeployment("applications-rp", "2")
	withSelf := func(objects ...runtime.Object) []runtime.Object {
		return append([]runtime.Object{ucp, gateTestReplicaSet(ucp, "2"), gateTestPod(self, "ucp", "ucp-2", running, false)}, objects...)
	}
	for _, tt := range []struct {
		name     string
		objects  []runtime.Object
		expected []string
	}{
		{
			name:     "rollouts finished",
			objects:  withSelf(rp, gateTestReplicaSet(rp, "2"), gateTestPod("rp-new", "applications-rp", "applications-rp-2", running, false)),
			expected: []string{},
		},
		{
			name: "older replicas still run",
			objects: withSelf(rp, gateTestReplicaSet(rp, "1"), gateTestReplicaSet(rp, "2"),
				gateTestPod("rp-new", "applications-rp", "applications-rp-2", running, false),
				gateTestPod("rp-old-2", "applications-rp", "applications-rp-1", running, false),
				gateTestPod("rp-old-1", "applications-rp", "applications-rp-1", corev1.PodPending, false)),
			expected: []string{"rp-old-1", "rp-old-2"},
		},
		{
			name: "older replica terminating",
			objects: withSelf(gateTestReplicaSet(ucp, "1"),
				gateTestPod("ucp-old", "ucp", "ucp-1", running, true)),
			expected: []string{"ucp-old"},
		},
		{
			name: "older replicas stopped",
			objects: withSelf(gateTestReplicaSet(ucp, "1"),
				gateTestPod("ucp-evicted", "ucp", "ucp-1", corev1.PodFailed, false),
				gateTestPod("ucp-done", "ucp", "ucp-1", corev1.PodSucceeded, false)),
			expected: []string{},
		},
		{
			// A webhook can inject containers, such as the sidecar of a service mesh, or change images.
			name: "containers changed by an admission webhook",
			objects: withSelf(func() runtime.Object {
				pod := gateTestPod("ucp-meshed", "ucp", "ucp-2", running, false)
				pod.Spec.Containers = append(pod.Spec.Containers, corev1.Container{Name: "istio-proxy", Image: "proxy"})
				pod.Spec.Containers[0].Image = "mirror.example.com/ucp@sha256:0000"
				return pod
			}()),
			expected: []string{},
		},
		{
			name: "rollout not observed yet",
			objects: func() []runtime.Object {
				changed := gateTestDeployment("dynamic-rp", "1")
				changed.Generation = 3
				return withSelf(changed, gateTestReplicaSet(changed, "1"), gateTestPod("drp", "dynamic-rp", "dynamic-rp-1", running, false))
			}(),
			expected: []string{"drp"},
		},
		{
			// A rollback makes the ReplicaSet of the earlier template the current revision, so this pod is outdated.
			name: "rollback in progress",
			objects: func() []runtime.Object {
				rollback := gateTestDeployment("ucp", "3")
				return []runtime.Object{rollback, gateTestReplicaSet(rollback, "2"), gateTestReplicaSet(rollback, "3"),
					gateTestPod(self, "ucp", "ucp-2", running, false), gateTestPod("ucp-rollback", "ucp", "ucp-3", running, false)}
			}(),
			expected: []string{self},
		},
		{
			name:     "ReplicaSet deleted",
			objects:  withSelf(gateTestPod("ucp-orphan", "ucp", "ucp-1", running, true)),
			expected: []string{"ucp-orphan"},
		},
		{
			name: "pods that the Deployment does not own",
			objects: withSelf(
				gateTestPod("ucp-debug", "ucp", "", running, false),
				func() runtime.Object {
					other := gateTestReplicaSet(gateTestDeployment("other", "1"), "1")
					other.Name, other.UID = "other-rs", "other-rs"
					return other
				}(),
				gateTestPod("ucp-other", "ucp", "other-rs", running, false)),
			expected: []string{},
		},
		{
			name:     "components without a Deployment",
			objects:  withSelf(gateTestPod("de-old", "bicep-de", "bicep-de-1", running, false)),
			expected: []string{},
		},
		{
			name:    "not running in Kubernetes",
			objects: []runtime.Object{ucp, gateTestReplicaSet(ucp, "1"), gateTestPod("ucp-old", "ucp", "ucp-1", running, false)},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			pods, err := OutdatedPods(t.Context(), fake.NewClientset(tt.objects...), DefaultNamespace, self, "ucp", "applications-rp", "dynamic-rp")
			require.NoError(t, err)
			require.Equal(t, tt.expected, pods)
		})
	}

	t.Run("failures are not mistaken for finished rollouts", func(t *testing.T) {
		t.Parallel()
		for _, call := range []struct{ verb, resource string }{{"get", "pods"}, {"list", "pods"}, {"get", "deployments"}, {"get", "replicasets"}} {
			client := fake.NewClientset(withSelf()...)
			client.PrependReactor(call.verb, call.resource, func(k8stesting.Action) (bool, runtime.Object, error) {
				return true, nil, apierrors.NewForbidden(schema.GroupResource{Resource: call.resource}, "", errors.New("denied"))
			})
			_, err := OutdatedPods(t.Context(), client, DefaultNamespace, self, "ucp")
			require.True(t, apierrors.IsForbidden(errors.Unwrap(err)), "%s %s: %v", call.verb, call.resource, err)
		}
	})
}

func Test_ReleaseGate(t *testing.T) {
	// The gate reads the name of this pod from the package variable hostname.
	original := hostname
	t.Cleanup(func() { hostname = original })
	hostname = func() (string, error) { return "ucp-new", nil }

	ucp := gateTestDeployment("ucp", "2")
	client := fake.NewClientset(ucp, gateTestReplicaSet(ucp, "1"), gateTestReplicaSet(ucp, "2"),
		gateTestPod("ucp-new", "ucp", "ucp-2", corev1.PodRunning, false), gateTestPod("ucp-old", "ucp", "ucp-1", corev1.PodRunning, false))
	lists, clients := 0, 0
	client.PrependReactor("list", "pods", func(k8stesting.Action) (bool, runtime.Object, error) {
		lists++
		return false, nil, nil
	})
	now := time.Now()
	gate := NewReleaseGate(func() (kubernetes.Interface, error) {
		clients++
		return client, nil
	}, DefaultNamespace, "ucp")
	gate.now = func() time.Time { return now }

	message, err := gate.Wait(t.Context())
	require.NoError(t, err)
	require.Contains(t, message, "ucp-old")
	_, err = gate.Wait(t.Context())
	require.NoError(t, err)
	require.Equal(t, 2, lists, "a closed gate looks again every time")

	require.NoError(t, client.CoreV1().Pods(DefaultNamespace).Delete(t.Context(), "ucp-old", metav1.DeleteOptions{}))
	message, err = gate.Wait(t.Context())
	require.NoError(t, err)
	require.Empty(t, message)

	// An open gate stays open for a few seconds without looking again.
	_, err = client.CoreV1().Pods(DefaultNamespace).Create(t.Context(), gateTestPod("ucp-rollback", "ucp", "ucp-1", corev1.PodRunning, false), metav1.CreateOptions{})
	require.NoError(t, err)
	message, err = gate.Wait(t.Context())
	require.NoError(t, err)
	require.Empty(t, message)
	require.Equal(t, 3, lists)

	now = now.Add(gateOpenFor + time.Second)
	message, err = gate.Wait(t.Context())
	require.NoError(t, err)
	require.Contains(t, message, "ucp-rollback")
	require.Equal(t, 1, clients, "the gate builds its client once")

	failing := NewReleaseGate(func() (kubernetes.Interface, error) { return nil, errors.New("no cluster") }, DefaultNamespace, "ucp")
	_, err = failing.Wait(t.Context())
	require.ErrorContains(t, err, "no cluster")

	message, err = Wait(t.Context(), nil)
	require.NoError(t, err)
	require.Empty(t, message, "a step without a gate never waits")
}
