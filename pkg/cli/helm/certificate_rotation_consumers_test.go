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
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/radius-project/radius/pkg/authz/mtls"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
	"k8s.io/utils/ptr"
)

func rotationObserverFixture(t *testing.T) (*rotationConsumerObserver, []rotationConsumer) {
	t.Helper()
	ca, _, _ := testSigningCA(t, nil, nil, nil, nil)
	return rotationObserverFixtureForCA(t, ca)
}

func rotationObserverFixtureForCA(t *testing.T, ca []byte) (*rotationConsumerObserver, []rotationConsumer) {
	t.Helper()
	digest, roots, err := mtls.TrustBundleDigest(ca, time.Now())
	require.NoError(t, err)
	trust := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "radius-trust-test", Namespace: RadiusSystemNamespace,
			UID: types.UID(uuid.NewString()), ResourceVersion: "1", Labels: map[string]string{rotationOwner: radiusReleaseName}},
		Immutable: ptr.To(true), Data: map[string]string{"ca.crt": string(ca)},
	}
	objects := []runtime.Object{trust}
	expected := []rotationConsumer{}
	for _, name := range rotationComponents {
		container := name
		if name == "bicep-de" {
			container = "de"
		}
		template := corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app.kubernetes.io/name": name}},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{{
					Name: container,
					Env: []corev1.EnvVar{
						{Name: "UCP_MTLS_STARTUP_PROOF_ENABLED", Value: "true"},
						{Name: "UCP_MTLS_EXPECTED_TRUST_SHA256", Value: digest},
						{Name: "UCP_MTLS_POD_UID", ValueFrom: &corev1.EnvVarSource{
							FieldRef: &corev1.ObjectFieldSelector{FieldPath: "metadata.uid"},
						}},
					},
					VolumeMounts: []corev1.VolumeMount{{Name: "mtls", MountPath: "/var/run/radius/mtls", ReadOnly: true}},
				}},
				Volumes: []corev1.Volume{{
					Name: "mtls",
					VolumeSource: corev1.VolumeSource{Projected: &corev1.ProjectedVolumeSource{Sources: []corev1.VolumeProjection{
						{Secret: &corev1.SecretProjection{LocalObjectReference: corev1.LocalObjectReference{Name: name + "-mtls"},
							Items: []corev1.KeyToPath{{Key: "tls.crt", Path: "tls.crt"}, {Key: "tls.key", Path: "tls.key"}}}},
						{ConfigMap: &corev1.ConfigMapProjection{LocalObjectReference: corev1.LocalObjectReference{Name: trust.Name},
							Items: []corev1.KeyToPath{{Key: "ca.crt", Path: "ca.crt"}}}},
					}}},
				}},
			},
		}
		deployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: RadiusSystemNamespace, UID: types.UID(uuid.NewString()), ResourceVersion: "1",
				Generation: 1, Annotations: map[string]string{"meta.helm.sh/release-name": radiusReleaseName, "meta.helm.sh/release-namespace": RadiusSystemNamespace}},
			Spec: appsv1.DeploymentSpec{Replicas: ptr.To(int32(1)), Template: template,
				Strategy: appsv1.DeploymentStrategy{Type: appsv1.RecreateDeploymentStrategyType}},
			Status: appsv1.DeploymentStatus{ObservedGeneration: 1, Replicas: 1, UpdatedReplicas: 1, ReadyReplicas: 1, AvailableReplicas: 1},
		}
		set := &appsv1.ReplicaSet{
			ObjectMeta: metav1.ObjectMeta{Name: name + "-rs", Namespace: RadiusSystemNamespace, UID: types.UID(uuid.NewString()), ResourceVersion: "1",
				OwnerReferences: []metav1.OwnerReference{{APIVersion: "apps/v1", Kind: "Deployment", Name: name, UID: deployment.UID, Controller: ptr.To(true)}}},
			Spec:   appsv1.ReplicaSetSpec{Replicas: ptr.To(int32(1)), Template: *template.DeepCopy()},
			Status: appsv1.ReplicaSetStatus{Replicas: 1},
		}
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: name + "-pod", Namespace: RadiusSystemNamespace, UID: types.UID(uuid.NewString()), ResourceVersion: "1",
				Labels:          map[string]string{"app.kubernetes.io/name": name},
				OwnerReferences: []metav1.OwnerReference{{APIVersion: "apps/v1", Kind: "ReplicaSet", Name: set.Name, UID: set.UID, Controller: ptr.To(true)}}},
			Spec: *template.Spec.DeepCopy(),
			Status: corev1.PodStatus{Phase: corev1.PodRunning, ContainerStatuses: []corev1.ContainerStatus{{
				Name: container, Ready: true, ContainerID: "containerd://test",
				State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{StartedAt: metav1.NewTime(time.Now().Add(-time.Minute))}},
			}}},
		}
		objects = append(objects, deployment, set, pod)
		expected = append(expected, rotationConsumer{Name: name, DeploymentUID: deployment.UID,
			TrustMap: trust.Name, TrustSHA256: digest, LeafSHA256: strings.Repeat("a", 64), ProofPort: 9090})
	}
	observer := &rotationConsumerObserver{
		kube: fake.NewClientset(objects...), namespace: RadiusSystemNamespace, release: radiusReleaseName,
		observe: func(_ context.Context, _ *rest.Config, target TLSProofTarget) (PodTLSProof, error) {
			return PodTLSProof{Evidence: mtls.StartupEvidence{
				SchemaVersion: 1, Ready: true, ProofKind: "startup-tls-initialized",
				PodUID: string(target.PodUID), BootID: uuid.NewString(), InitializedAt: time.Now().Add(-time.Second),
				TrustBundleSHA256: target.TrustBundleSHA256, CASHA256: roots, LeafSHA256: target.LeafSHA256,
				NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
			}}, nil
		},
	}
	return observer, expected
}

func TestRotationConsumerBarrier(t *testing.T) {
	tests := []struct {
		name   string
		change func(*testing.T, *rotationConsumerObserver, *[]rotationConsumer)
		want   string
	}{
		{name: "all expected process evidence", change: func(_ *testing.T, _ *rotationConsumerObserver, _ *[]rotationConsumer) {}},
		{name: "missing engine", want: "must account for", change: func(_ *testing.T, _ *rotationConsumerObserver, expected *[]rotationConsumer) {
			*expected = (*expected)[:4]
		}},
		{name: "replaced deployment", want: "changed identity", change: func(_ *testing.T, _ *rotationConsumerObserver, expected *[]rotationConsumer) {
			(*expected)[0].DeploymentUID = types.UID(uuid.NewString())
		}},
		{name: "duplicate consumer", want: "duplicate", change: func(_ *testing.T, _ *rotationConsumerObserver, expected *[]rotationConsumer) {
			(*expected)[4] = (*expected)[0]
		}},
		{name: "unloaded trust", want: "waiting for", change: func(_ *testing.T, observer *rotationConsumerObserver, _ *[]rotationConsumer) {
			observer.observe = func(context.Context, *rest.Config, TLSProofTarget) (PodTLSProof, error) {
				return PodTLSProof{}, errors.New("process has not loaded the new bundle")
			}
		}},
		{name: "invalid loaded leaf", want: "invalid process evidence", change: func(_ *testing.T, observer *rotationConsumerObserver, _ *[]rotationConsumer) {
			original := observer.observe
			observer.observe = func(ctx context.Context, config *rest.Config, target TLSProofTarget) (PodTLSProof, error) {
				proof, err := original(ctx, config, target)
				proof.Evidence.LeafSHA256 = strings.Repeat("b", 64)
				return proof, err
			}
		}},
		{name: "unowned consumer without labels", want: "unaccounted consumer", change: func(t *testing.T, observer *rotationConsumerObserver, _ *[]rotationConsumer) {
			pod, err := observer.kube.CoreV1().Pods(observer.namespace).Get(t.Context(), "ucp-pod", metav1.GetOptions{})
			require.NoError(t, err)
			pod.Name, pod.UID, pod.ResourceVersion, pod.Labels, pod.OwnerReferences = "unowned", types.UID(uuid.NewString()), "", nil, nil
			_, err = observer.kube.CoreV1().Pods(observer.namespace).Create(t.Context(), pod, metav1.CreateOptions{})
			require.NoError(t, err)
		}},
		{name: "terminating pod", want: "waiting for pod", change: func(t *testing.T, observer *rotationConsumerObserver, _ *[]rotationConsumer) {
			pod, err := observer.kube.CoreV1().Pods(observer.namespace).Get(t.Context(), "controller-pod", metav1.GetOptions{})
			require.NoError(t, err)
			now := metav1.Now()
			pod.DeletionTimestamp = &now
			_, err = observer.kube.CoreV1().Pods(observer.namespace).Update(t.Context(), pod, metav1.UpdateOptions{})
			require.NoError(t, err)
		}},
		{name: "mutable trust", want: "immutable public generation", change: func(t *testing.T, observer *rotationConsumerObserver, expected *[]rotationConsumer) {
			trust, err := observer.kube.CoreV1().ConfigMaps(observer.namespace).Get(t.Context(), (*expected)[0].TrustMap, metav1.GetOptions{})
			require.NoError(t, err)
			trust.Immutable = ptr.To(false)
			_, err = observer.kube.CoreV1().ConfigMaps(observer.namespace).Update(t.Context(), trust, metav1.UpdateOptions{})
			require.NoError(t, err)
		}},
		{name: "controller surge", want: "single controller", change: func(t *testing.T, observer *rotationConsumerObserver, _ *[]rotationConsumer) {
			deployment, err := observer.kube.AppsV1().Deployments(observer.namespace).Get(t.Context(), "controller", metav1.GetOptions{})
			require.NoError(t, err)
			deployment.Spec.Strategy.Type = appsv1.RollingUpdateDeploymentStrategyType
			_, err = observer.kube.AppsV1().Deployments(observer.namespace).Update(t.Context(), deployment, metav1.UpdateOptions{})
			require.NoError(t, err)
		}},
		{name: "restart during observation", want: "inventory changed", change: func(t *testing.T, observer *rotationConsumerObserver, _ *[]rotationConsumer) {
			original := observer.observe
			observer.observe = func(ctx context.Context, config *rest.Config, target TLSProofTarget) (PodTLSProof, error) {
				proof, err := original(ctx, config, target)
				pod, getErr := observer.kube.CoreV1().Pods(observer.namespace).Get(ctx, target.PodName, metav1.GetOptions{})
				require.NoError(t, getErr)
				// The fake tracker does not increment resourceVersion; model the
				// native API-server update that a container restart would cause.
				pod.ResourceVersion = "2"
				_, updateErr := observer.kube.CoreV1().Pods(observer.namespace).UpdateStatus(ctx, pod, metav1.UpdateOptions{})
				require.NoError(t, updateErr)
				return proof, err
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			observer, expected := rotationObserverFixture(t)
			tt.change(t, observer, &expected)
			err := observer.verify(t.Context(), expected)
			if tt.want == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tt.want)
			}
		})
	}
}
