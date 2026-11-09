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
	"math"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/radius-project/radius/pkg/authz/mtls"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/utils/ptr"
)

func newPlannedRotationTest(t *testing.T) (*rotationStore, *rest.Config, rotationState) {
	t.Helper()
	store, config, old, target := newRotationTestStore(t)
	ctx := t.Context()
	id := uuid.NewString()
	digest, _, err := mtls.TrustBundleDigest(target, time.Now())
	require.NoError(t, err)
	owned := func(name string) metav1.ObjectMeta {
		return metav1.ObjectMeta{Name: name, Namespace: store.namespace, Labels: map[string]string{"app.kubernetes.io/managed-by": "Helm"}, Annotations: map[string]string{
			"meta.helm.sh/release-name": store.release, "meta.helm.sh/release-namespace": store.namespace,
		}}
	}
	service, err := store.kube.CoreV1().Services(store.namespace).Create(ctx, &corev1.Service{
		ObjectMeta: owned("ucp"), Spec: corev1.ServiceSpec{
			Selector: map[string]string{"app.kubernetes.io/name": "ucp"},
			Ports:    []corev1.ServicePort{{Name: "https", Port: 443}},
		},
	}, metav1.CreateOptions{})
	require.NoError(t, err)
	plan := &rotationPlan{
		TargetMode: "caSecret", TargetTrustMap: "radius-mtls-ca-" + digest[:32], TargetTrustSHA256: digest,
		OldComponentCA: old, SigningSecret: "operator-ca", SigningUID: types.UID(uuid.NewString()),
		Issuer: "radius-ca-" + strings.ReplaceAll(id, "-", "")[:16], ServingSecret: "radius-aggregation-" + strings.ReplaceAll(id, "-", "")[:16],
		ServiceUID: service.UID, ServiceSelector: service.Spec.Selector,
		UCPProgressDeadlineSeconds: 600,
	}
	for _, name := range rotationComponents {
		container := name
		if name == "bicep-de" {
			container = "de"
		}
		labels := map[string]string{"app.kubernetes.io/name": name}
		deployment, err := store.kube.AppsV1().Deployments(store.namespace).Create(ctx, &appsv1.Deployment{
			ObjectMeta: owned(name),
			Spec: appsv1.DeploymentSpec{
				Replicas: ptr.To(int32(1)), Selector: &metav1.LabelSelector{MatchLabels: labels},
				Template: corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: labels}, Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: container, Image: "unused-envtest-image",
						Env: []corev1.EnvVar{{Name: "UCP_MTLS_STARTUP_PROOF_ENABLED", Value: "true"}}}},
					Volumes: []corev1.Volume{
						{Name: "mtls", VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: name + "-mtls"}}},
						{Name: "cert", VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: "ucp-cert"}}},
					},
				}},
			},
		}, metav1.CreateOptions{})
		require.NoError(t, err)
		workload := rotationWorkload{Name: name, UID: deployment.UID, CertificateUID: types.UID(uuid.NewString()),
			OldIssuer: "radius-ca-issuer", ProofPort: 9090}
		plan.Consumers = append(plan.Consumers, workload)
	}
	state, err := store.beginPlanned(ctx, id, target, plan)
	require.NoError(t, err)
	require.NoError(t, store.reconcileTrust(ctx, id))
	return store, config, state
}

func TestRotationRoutingAndRolloutFences(t *testing.T) {
	for _, scope := range []rotationScope{rotationComponentsAndAggregation, rotationAggregationOnly} {
		t.Run(string(scope), func(t *testing.T) {
			testRotationRoutingAndRolloutFences(t, scope)
		})
	}
}

func testRotationRoutingAndRolloutFences(t *testing.T, scope rotationScope) {
	t.Helper()
	store, config, state := newPlannedRotationTest(t)
	ctx, id, plan := t.Context(), state.ID, state.Plan
	var ucp rotationWorkload
	for _, workload := range plan.Consumers {
		if workload.Name == "ucp" {
			ucp = workload
		}
	}
	if scope == rotationAggregationOnly {
		state = aggregationOnlyState(t, state)
		storeAggregationExecutionState(t, store, state)
		deployment, err := store.kube.AppsV1().Deployments(store.namespace).Get(ctx, "ucp", metav1.GetOptions{})
		require.NoError(t, err)
		deployment.Spec.Template.Spec.Containers[0].Ports = []corev1.ContainerPort{{Name: "metrics", ContainerPort: 9090}}
		deployment.Spec.Template.Spec.Containers[0].VolumeMounts = []corev1.VolumeMount{{Name: "cert", MountPath: "/var/tls/cert", ReadOnly: true}}
		_, err = store.kube.AppsV1().Deployments(store.namespace).Update(ctx, deployment, metav1.UpdateOptions{})
		require.NoError(t, err)
	}
	reconcile := func(s *rotationStore) error {
		if scope == rotationAggregationOnly {
			return s.reconcileAggregationDeployment(ctx, id)
		}
		return s.reconcileRotationDeployment(ctx, id, ucp)
	}
	advanceRotationTest(t, store, rotationAdopt)
	if scope == rotationComponentsAndAggregation {
		for _, workload := range plan.Consumers {
			require.NoError(t, store.reconcileRotationDeployment(ctx, id, workload))
		}
		controller, err := store.kube.AppsV1().Deployments(store.namespace).Get(ctx, "controller", metav1.GetOptions{})
		require.NoError(t, err)
		require.Equal(t, appsv1.RecreateDeploymentStrategyType, controller.Spec.Strategy.Type)
	}
	advanceRotationTest(t, store, rotationIssue)
	_, err := store.setAggregationRoute(ctx, id, "old", "old-hash", nil)
	require.NoError(t, err)
	require.NoError(t, reconcile(store))
	held, err := store.kube.AppsV1().Deployments(store.namespace).Get(ctx, "ucp", metav1.GetOptions{})
	require.NoError(t, err)
	require.Equal(t, int32(math.MaxInt32-1), held.Spec.MinReadySeconds)
	routed, err := store.setAggregationRoute(ctx, id, "new", "new-hash", nil)
	require.NoError(t, err)

	paused, release := make(chan struct{}), make(chan struct{})
	var unblock sync.Once
	t.Cleanup(func() { unblock.Do(func() { close(release) }) })
	delayedConfig := rest.CopyConfig(config)
	delayedConfig.WrapTransport = func(base http.RoundTripper) http.RoundTripper {
		return rotationRoundTripper(func(request *http.Request) (*http.Response, error) {
			if request.Method == http.MethodPut && strings.HasSuffix(request.URL.Path, "/deployments/ucp") {
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
	delayed, err := kubernetes.NewForConfig(delayedConfig)
	require.NoError(t, err)
	writer := *store
	writer.kube = delayed
	result := make(chan error, 1)
	go func() { result <- reconcile(&writer) }()
	select {
	case <-paused:
	case <-time.After(10 * time.Second):
		t.Fatal("deployment writer did not reach its write")
	}
	_, err = store.setAggregationRoute(ctx, id, "complete", "", routed)
	require.NoError(t, err)
	require.NoError(t, reconcile(store))
	unblock.Do(func() { close(release) })
	require.ErrorContains(t, <-result, "the object has been modified")
	_, err = store.setAggregationRoute(ctx, id, "old", "old-hash", routed)
	require.ErrorContains(t, err, "the object has been modified", "failed probes cannot restore a route after another runner completes")
	_, err = store.setAggregationRoute(ctx, id, "old", "old-hash", nil)
	require.ErrorContains(t, err, "already completed")
	require.NoError(t, reconcile(store), "fresh retries derive release from live routing")
	live, err := store.kube.AppsV1().Deployments(store.namespace).Get(ctx, "ucp", metav1.GetOptions{})
	require.NoError(t, err)
	require.Equal(t, int32(0), live.Spec.MinReadySeconds)
}
