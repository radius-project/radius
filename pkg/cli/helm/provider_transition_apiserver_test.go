//go:build !windows

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

package helm

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/utils/ptr"
)

func TestProviderTransitionAPIServer(t *testing.T) {
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		t.Skip("Set KUBEBUILDER_ASSETS for isolated provider transition tests.")
	}
	kubeconfig, kube := startTestAPIServer(t)
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	require.NoError(t, err)
	_, err = kube.CoreV1().Namespaces().Create(t.Context(), &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: RadiusSystemNamespace},
	}, metav1.CreateOptions{})
	require.NoError(t, err)
	s, plan, targets := providerFixture(t, kube, "radius", "external", "enforce")
	require.NoError(t, s.begin(t.Context(), plan, targets, true))
	require.Error(t, s.begin(t.Context(), plan, targets, true), "cannot replace an existing journal")

	t.Run("journal CAS", func(t *testing.T) {
		cm, state, err := s.read(t.Context(), plan.Namespace)
		require.NoError(t, err)
		changed := cm.DeepCopy()
		changed.Annotations = map[string]string{"operator-note": "changed"}
		_, err = kube.CoreV1().ConfigMaps(plan.Namespace).Update(t.Context(), changed, metav1.UpdateOptions{})
		require.NoError(t, err)
		require.True(t, apierrors.IsConflict(s.withLock(t.Context(), plan.Namespace, func(lease *coordinationv1.Lease) error {
			return s.advance(t.Context(), lease, cm, state, providerStaging)
		})))
		_, state, err = s.read(t.Context(), plan.Namespace)
		require.NoError(t, err)
		require.Equal(t, providerStopping, state.Phase)
	})

	t.Run("delayed scale update cannot overwrite drift", func(t *testing.T) {
		paused, release := make(chan struct{}), make(chan struct{})
		var once sync.Once
		unblock := func() { once.Do(func() { close(release) }) }
		t.Cleanup(unblock)
		delayedConfig := rest.CopyConfig(config)
		name := plan.Workloads[0].Name
		delayedConfig.WrapTransport = func(base http.RoundTripper) http.RoundTripper {
			return rotationRoundTripper(func(request *http.Request) (*http.Response, error) {
				if request.Method == http.MethodPut && strings.HasSuffix(request.URL.Path, "/deployments/"+name) {
					close(paused)
					select {
					case <-release:
					case <-request.Context().Done():
						return nil, request.Context().Err()
					}
				}
				return base.RoundTrip(request)
			})
		}
		delayedKube, err := kubernetes.NewForConfig(delayedConfig)
		require.NoError(t, err)
		delayed := &providerTransitionStore{kube: delayedKube, checks: s.checks}
		ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
		defer cancel()
		result := make(chan error, 1)
		go func() { result <- delayed.step(ctx, plan.Namespace, plan.ID, targets) }()
		select {
		case <-paused:
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
		require.ErrorContains(t, s.step(t.Context(), plan.Namespace, plan.ID, targets), "do not steal", "concurrent runner is fenced")
		d, err := kube.AppsV1().Deployments(plan.Namespace).Get(t.Context(), name, metav1.GetOptions{})
		require.NoError(t, err)
		d.Spec.Replicas = ptr.To(int32(7))
		_, err = kube.AppsV1().Deployments(plan.Namespace).Update(t.Context(), d, metav1.UpdateOptions{})
		require.NoError(t, err)
		unblock()
		require.True(t, apierrors.IsConflict(<-result))
		live, err := kube.AppsV1().Deployments(plan.Namespace).Get(t.Context(), name, metav1.GetOptions{})
		require.NoError(t, err)
		require.EqualValues(t, 7, *live.Spec.Replicas)
		require.Empty(t, live.Annotations[providerTransitionMarker])
		require.ErrorContains(t, s.step(t.Context(), plan.Namespace, plan.ID, targets), "drifted")
		// Explicit operator repair, not automatic rollback by the transition.
		live.Spec.Replicas = ptr.To(plan.Workloads[0].Replicas)
		_, err = kube.AppsV1().Deployments(plan.Namespace).Update(t.Context(), live, metav1.UpdateOptions{})
		require.NoError(t, err)
	})

	t.Run("lock cleanup cannot delete replacement", func(t *testing.T) {
		var replacement *coordinationv1.Lease
		err := s.withLock(t.Context(), plan.Namespace, func(lease *coordinationv1.Lease) error {
			client := kube.CoordinationV1().Leases(plan.Namespace)
			live, err := client.Get(t.Context(), providerTransitionLock, metav1.GetOptions{})
			require.NoError(t, err)
			require.NoError(t, client.Delete(t.Context(), live.Name, metav1.DeleteOptions{
				Preconditions: &metav1.Preconditions{UID: &live.UID, ResourceVersion: &live.ResourceVersion},
			}))
			replacement, err = client.Create(t.Context(), &coordinationv1.Lease{
				ObjectMeta: metav1.ObjectMeta{Name: providerTransitionLock},
				Spec:       coordinationv1.LeaseSpec{HolderIdentity: ptr.To("operator")},
			}, metav1.CreateOptions{})
			require.NoError(t, err)
			require.ErrorContains(t, s.checkLock(t.Context(), lease), "lock changed")
			return nil
		})
		require.ErrorContains(t, err, "cannot release")
		live, err := kube.CoordinationV1().Leases(plan.Namespace).Get(t.Context(), providerTransitionLock, metav1.GetOptions{})
		require.NoError(t, err)
		require.Equal(t, replacement.UID, live.UID)
		require.NoError(t, kube.CoordinationV1().Leases(plan.Namespace).Delete(t.Context(), live.Name, metav1.DeleteOptions{
			Preconditions: &metav1.Preconditions{UID: &live.UID, ResourceVersion: &live.ResourceVersion},
		}))
	})

	t.Run("real status barrier and exact restore", func(t *testing.T) {
		require.ErrorIs(t, s.step(t.Context(), plan.Namespace, plan.ID, targets), errProviderTransitionWaiting)
		_, state, err := s.read(t.Context(), plan.Namespace)
		require.NoError(t, err)
		require.Equal(t, providerStopping, state.Phase)
		for range 4 {
			providerStepConverged(t, s, plan, targets)
		}
		_, state, err = s.read(t.Context(), plan.Namespace)
		require.NoError(t, err)
		require.Equal(t, providerComplete, state.Phase)
		for _, w := range plan.Workloads {
			d, hash, err := s.deployment(t.Context(), plan, w)
			require.NoError(t, err)
			require.Equal(t, w.Replicas, *d.Spec.Replicas)
			require.Equal(t, w.TargetSHA256, hash)
		}
	})

	t.Run("target pods require target ReplicaSet ownership", func(t *testing.T) {
		w := plan.Workloads[0]
		d, err := kube.AppsV1().Deployments(plan.Namespace).Get(t.Context(), w.Name, metav1.GetOptions{})
		require.NoError(t, err)
		rs, err := kube.AppsV1().ReplicaSets(plan.Namespace).Create(t.Context(), &appsv1.ReplicaSet{
			ObjectMeta: metav1.ObjectMeta{Name: "target-rs", OwnerReferences: []metav1.OwnerReference{{
				APIVersion: "apps/v1", Kind: "Deployment", Name: w.Name, UID: w.UID, Controller: ptr.To(true),
			}}},
			Spec: appsv1.ReplicaSetSpec{Replicas: ptr.To(int32(0)), Selector: d.Spec.Selector, Template: d.Spec.Template},
		}, metav1.CreateOptions{})
		require.NoError(t, err)
		pod, err := kube.CoreV1().Pods(plan.Namespace).Create(t.Context(), &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "target-pod", Labels: d.Spec.Template.Labels},
			Spec:       d.Spec.Template.Spec,
		}, metav1.CreateOptions{})
		require.NoError(t, err)
		require.ErrorContains(t, s.step(t.Context(), plan.Namespace, plan.ID, targets), "unowned provider pod")
		pod.OwnerReferences = []metav1.OwnerReference{{
			APIVersion: "apps/v1", Kind: "ReplicaSet", Name: rs.Name, UID: rs.UID, Controller: ptr.To(true),
		}}
		_, err = kube.CoreV1().Pods(plan.Namespace).Update(t.Context(), pod, metav1.UpdateOptions{})
		require.NoError(t, err)
		require.NoError(t, s.step(t.Context(), plan.Namespace, plan.ID, targets))
		rs.Spec.Template.Spec.Containers[0].Env[0].Value = plan.Source
		_, err = kube.AppsV1().ReplicaSets(plan.Namespace).Update(t.Context(), rs, metav1.UpdateOptions{})
		require.NoError(t, err)
		require.ErrorContains(t, s.step(t.Context(), plan.Namespace, plan.ID, targets), "old or unowned provider pod")
	})

	t.Run("aggregation change invalidates complete journal", func(t *testing.T) {
		s.checks.(*providerChecksStub).snapshot.SignerSHA256 = strings.Repeat("c", 64)
		err := s.step(t.Context(), plan.Namespace, plan.ID, targets)
		require.ErrorContains(t, err, "aggregation identity")
		require.False(t, errors.Is(err, errProviderTransitionWaiting))
	})

	t.Run("configuration resourceVersion drift", func(t *testing.T) {
		cm, err := kube.CoreV1().ConfigMaps(plan.Namespace).Create(t.Context(), &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "versioned-config"},
			Data:       map[string]string{"config": "fixed"},
		}, metav1.CreateOptions{})
		require.NoError(t, err)
		pin, err := s.configuration(t.Context(), plan.Namespace, cm.Name)
		require.NoError(t, err)
		pinned := plan
		pinned.Configurations = []providerTransitionConfig{pin}
		require.NoError(t, s.checkConfigurations(t.Context(), pinned))
		cm.Annotations = map[string]string{"operator": "changed"}
		_, err = kube.CoreV1().ConfigMaps(plan.Namespace).Update(t.Context(), cm, metav1.UpdateOptions{})
		require.NoError(t, err)
		require.ErrorContains(t, s.checkConfigurations(t.Context(), pinned), "configuration versioned-config changed")
	})
}
