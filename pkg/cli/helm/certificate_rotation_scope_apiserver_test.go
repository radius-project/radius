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
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/radius-project/radius/pkg/authz/mtls"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"
)

func TestRotationScopeNativeJournalAndRecovery(t *testing.T) {
	s, _, _, target := newRotationTestStore(t)
	_, template := rotationRunnerFixture(t)
	plan := aggregationOnlyState(t, template).Plan
	digest, _, err := mtls.TrustBundleDigest(target, time.Now())
	require.NoError(t, err)
	plan.TargetTrustSHA256 = digest
	plan.TargetTrustMap = "radius-mtls-ca-" + plan.TargetTrustSHA256[:32]
	ctx := t.Context()
	state, err := s.beginPlanned(ctx, template.ID, target, plan)
	require.NoError(t, err)
	require.Equal(t, 2, state.SchemaVersion)
	journal, loaded, err := s.read(ctx)
	require.NoError(t, err)
	require.Equal(t, state, loaded)
	require.Empty(t, loaded.Plan.Consumers)
	require.NotContains(t, journal.Data[rotationJournalKey], `"consumers":null`)
	require.NoError(t, s.reconcileTrust(ctx, state.ID))
	advanceRotationTest(t, s, rotationAdopt)
	require.Error(t, s.advance(ctx, journal, state, rotationAdopt), "stale journal cannot advance")

	next, _, _ := testSigningCA(t, nil, nil, nil, nil)
	id := uuid.NewString()
	recoveryPlan := *plan
	recoveryPlan.Issuer = "radius-ca-" + strings.ReplaceAll(id, "-", "")[:16]
	recoveryPlan.ServingSecret = "radius-aggregation-" + strings.ReplaceAll(id, "-", "")[:16]
	recoveryPlan.TargetTrustSHA256, _, err = mtls.TrustBundleDigest(next, time.Now())
	require.NoError(t, err)
	recoveryPlan.TargetTrustMap = "radius-mtls-ca-" + recoveryPlan.TargetTrustSHA256[:32]
	journal, _, err = s.read(ctx)
	require.NoError(t, err)
	recovery, err := s.beginRecovery(ctx, id, next, &recoveryPlan, journal)
	require.NoError(t, err, "aggregation history must not require nonexistent component CA")
	require.Equal(t, 2, recovery.SchemaVersion)
	require.True(t, recovery.Recovery)
	require.NoError(t, s.reconcileTrust(ctx, recovery.ID))
	retry, err := s.beginRecovery(ctx, id, next, &recoveryPlan, journal)
	require.Error(t, err, "caller must re-read changed journal before retry")
	require.Empty(t, retry.ID)
	journal, loaded, err = s.read(ctx)
	require.NoError(t, err)
	require.Equal(t, recovery, loaded)
	advanceRotationTest(t, s, rotationComplete)
	api, err := s.dynamic.Resource(apiServiceResource).Get(ctx, radiusAPIService, metav1.GetOptions{})
	require.NoError(t, err)
	bundle, err := apiServiceBundle(api)
	require.NoError(t, err)
	require.Equal(t, next, bundle)
	require.Equal(t, recovery.ID+"/complete", api.GetAnnotations()[rotationRevision])
}

func TestAggregationTransitionNativeReplacement(t *testing.T) {
	s, _, _, _ := newRotationTestStore(t)
	installRotationTestCRDs(t, s)
	fixture, managed := completedAggregationFixture(t)
	state := aggregationOnlyState(t, managed)
	ctx, ns := t.Context(), s.namespace
	meta := func(name string) metav1.ObjectMeta {
		return metav1.ObjectMeta{Name: name, Namespace: ns, Annotations: map[string]string{
			"meta.helm.sh/release-name": s.release, "meta.helm.sh/release-namespace": ns}}
	}
	for _, name := range []string{state.Plan.SigningSecret, state.Plan.ServingSecret} {
		secret, err := fixture.kube.CoreV1().Secrets(ns).Get(ctx, name, metav1.GetOptions{})
		require.NoError(t, err)
		secret.ObjectMeta = meta(name)
		created, err := s.kube.CoreV1().Secrets(ns).Create(ctx, secret, metav1.CreateOptions{})
		require.NoError(t, err)
		if name == state.Plan.SigningSecret {
			state.Plan.SigningUID = created.UID
		}
	}
	service, err := s.kube.CoreV1().Services(ns).Create(ctx, &corev1.Service{ObjectMeta: meta("ucp"),
		Spec: corev1.ServiceSpec{Selector: state.Plan.ServiceSelector, Ports: []corev1.ServicePort{{Name: "https", Port: 443}}}}, metav1.CreateOptions{})
	require.NoError(t, err)
	state.Plan.ServiceUID = service.UID
	deployment, err := s.kube.AppsV1().Deployments(ns).Create(ctx, &appsv1.Deployment{ObjectMeta: meta("ucp"),
		Spec: appsv1.DeploymentSpec{Replicas: ptr.To(int32(0)),
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app.kubernetes.io/name": "ucp"}},
			Template: corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app.kubernetes.io/name": "ucp"}},
				Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "ucp", Image: "not-run-envtest",
					VolumeMounts: []corev1.VolumeMount{{Name: "cert", MountPath: "/var/tls/cert", ReadOnly: true}}}},
					Volumes: []corev1.Volume{rotationTLSProjection("cert", state.Plan.ServingSecret, state.Plan.TargetTrustMap)}}}}}, metav1.CreateOptions{})
	require.NoError(t, err)
	state.Plan.Aggregation.DeploymentUID = deployment.UID
	trust, err := fixture.kube.CoreV1().ConfigMaps(ns).Get(ctx, state.Plan.TargetTrustMap, metav1.GetOptions{})
	require.NoError(t, err)
	trust.ObjectMeta = metav1.ObjectMeta{Name: trust.Name, Namespace: ns, Labels: map[string]string{rotationOwner: s.release}}
	_, err = s.kube.CoreV1().ConfigMaps(ns).Create(ctx, trust, metav1.CreateOptions{})
	require.NoError(t, err)
	for _, resource := range []schema.GroupVersionResource{rotationIssuerResource, rotationCertificateResource} {
		name := state.Plan.Issuer
		if resource == rotationCertificateResource {
			name = state.Plan.ServingSecret
		}
		object, err := fixture.dynamic.Resource(resource).Namespace(ns).Get(ctx, name, metav1.GetOptions{})
		require.NoError(t, err)
		object.SetUID("")
		object.SetResourceVersion("")
		object.SetCreationTimestamp(metav1.Time{})
		_, err = s.dynamic.Resource(resource).Namespace(ns).Create(ctx, object, metav1.CreateOptions{})
		require.NoError(t, err)
	}
	state, err = s.beginPlanned(ctx, state.ID, state.NewCA, state.Plan)
	require.NoError(t, err)
	require.NoError(t, s.reconcileTrust(ctx, state.ID))
	// This tests storage and identity pinning only; no running consumers or
	// cert-manager controller are represented by these synthetic phase advances.
	for _, phase := range rotationPhases[1:] {
		advanceRotationTest(t, s, phase)
	}
	snapshot, err := s.legacyAggregationTransitionSnapshot(ctx)
	require.NoError(t, err)
	require.Equal(t, rotationAggregationOnly, snapshot.Scope)
	require.NoError(t, s.checkLegacyAggregationTransitionSnapshot(ctx, snapshot))
	issuer, err := s.dynamic.Resource(rotationIssuerResource).Namespace(ns).Get(ctx, state.Plan.Issuer, metav1.GetOptions{})
	require.NoError(t, err)
	require.NoError(t, s.dynamic.Resource(rotationIssuerResource).Namespace(ns).Delete(ctx, issuer.GetName(), metav1.DeleteOptions{}))
	issuer.SetUID("")
	issuer.SetResourceVersion("")
	issuer.SetCreationTimestamp(metav1.Time{})
	replaced, err := s.dynamic.Resource(rotationIssuerResource).Namespace(ns).Create(ctx, issuer, metav1.CreateOptions{})
	require.NoError(t, err)
	require.NotEqual(t, snapshot.SignerSHA256, "")
	require.NotEmpty(t, replaced.GetUID())
	require.ErrorContains(t, s.checkLegacyAggregationTransitionSnapshot(ctx, snapshot), "aggregation state changed")
}
