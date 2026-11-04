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

package container

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	appsv1ac "k8s.io/client-go/applyconfigurations/apps/v1"
	corev1ac "k8s.io/client-go/applyconfigurations/core/v1"
	metav1ac "k8s.io/client-go/applyconfigurations/meta/v1"
	k8s "k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	"github.com/radius-project/radius/pkg/kubernetes"
	"github.com/radius-project/radius/pkg/upgrade/hashmigration"
)

// Test_RecordSecretHashes_APIServer records secret hashes on a real API server, which, unlike the fake clientset,
// tracks server-side apply field ownership and the Deployment generation that drives rollouts.
func Test_RecordSecretHashes_APIServer(t *testing.T) {
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		t.Skip("Run make test or set KUBEBUILDER_ASSETS to record secret hashes on an isolated API server.")
	}

	// A regression must fail on this unusable config, never reach a user's cluster.
	unusedKubeconfig := filepath.Join(t.TempDir(), "unused-kubeconfig")
	require.NoError(t, os.WriteFile(unusedKubeconfig, []byte("invalid kubeconfig"), 0600))
	t.Setenv("KUBECONFIG", unusedKubeconfig)
	useExistingCluster := false
	environment := &envtest.Environment{UseExistingCluster: &useExistingCluster}
	config, err := environment.Start()
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, environment.Stop())
	})
	client, err := k8s.NewForConfig(config)
	require.NoError(t, err)

	ctx := t.Context()
	_, err = client.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: secretHashTestNamespace}}, metav1.CreateOptions{})
	require.NoError(t, err)
	data := map[string][]byte{"CONNECTION_REDIS_PASSWORD": []byte("password")}
	_, err = client.CoreV1().Secrets(secretHashTestNamespace).Create(ctx, secretHashTestSecret(secretHashTestName, data), metav1.CreateOptions{})
	require.NoError(t, err)

	// Apply the Deployment the way Radius does: server-side apply with the radius-rp field manager.
	apply := func(hash string) {
		labels := map[string]string{kubernetes.LabelRadiusResource: secretHashTestName}
		deployment := appsv1ac.Deployment(secretHashTestName, secretHashTestNamespace).
			WithLabels(labels).
			WithSpec(appsv1ac.DeploymentSpec().
				WithSelector(metav1ac.LabelSelector().WithMatchLabels(labels)).
				WithTemplate(corev1ac.PodTemplateSpec().
					WithLabels(labels).
					WithAnnotations(map[string]string{kubernetes.AnnotationSecretHash: hash}).
					WithSpec(corev1ac.PodSpec().WithContainers(corev1ac.Container().WithName("app").WithImage("app:1")))))
		_, err := client.AppsV1().Deployments(secretHashTestNamespace).Apply(ctx, deployment, metav1.ApplyOptions{FieldManager: kubernetes.FieldManager})
		require.NoError(t, err)
	}

	legacy, current := kubernetes.LegacyHashSecretData(data), kubernetes.HashSecretData(data)
	apply(legacy)
	before, err := client.AppsV1().Deployments(secretHashTestNamespace).Get(ctx, secretHashTestName, metav1.GetOptions{})
	require.NoError(t, err)

	result, err := RecordSecretHashes(ctx, client)
	require.NoError(t, err)
	require.Equal(t, hashmigration.Result{Migrated: 1}, result)

	recorded, err := client.AppsV1().Deployments(secretHashTestNamespace).Get(ctx, secretHashTestName, metav1.GetOptions{})
	require.NoError(t, err)
	require.Equal(t, current, recorded.Annotations[kubernetes.AnnotationSecretHashSHA256])

	// Kubernetes advances the generation of a Deployment for annotation changes too, because the Deployment
	// controller copies them to the ReplicaSet. The pod template, and so the ReplicaSet and its pods, must not change.
	require.Equal(t, before.Spec, recorded.Spec)

	// A stale Deployment fails the resource-version precondition instead of overwriting a newer one.
	stale := recordSecretHash(ctx, client, before.DeepCopy())
	require.Equal(t, 1, stale.Remaining)
	require.Zero(t, stale.Migrated)

	// Radius applies the Deployment again without the annotation, which another field manager owns, so it stays.
	apply(legacy)
	reapplied, err := client.AppsV1().Deployments(secretHashTestNamespace).Get(ctx, secretHashTestName, metav1.GetOptions{})
	require.NoError(t, err)
	require.Equal(t, current, reapplied.Annotations[kubernetes.AnnotationSecretHashSHA256])
	require.Equal(t, recorded.Generation, reapplied.Generation, "reapplying an unchanged Deployment must not change it")
	require.Equal(t, before.Spec, reapplied.Spec)

	// A pass after the migration finds nothing to record.
	result, err = RecordSecretHashes(ctx, client)
	require.NoError(t, err)
	require.Equal(t, hashmigration.Result{}, result)

	// A changed secret replaces the pod template hash and rolls out the workload, as before.
	apply(kubernetes.HashSecretData(map[string][]byte{"CONNECTION_REDIS_PASSWORD": []byte("rotated")}))
	rolled, err := client.AppsV1().Deployments(secretHashTestNamespace).Get(ctx, secretHashTestName, metav1.GetOptions{})
	require.NoError(t, err)
	require.Greater(t, rolled.Generation, reapplied.Generation)
	require.NotEqual(t, before.Spec.Template, rolled.Spec.Template)
}
