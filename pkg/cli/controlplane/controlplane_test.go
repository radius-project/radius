/*
Copyright 2023 The Radius Authors.

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

package controlplane

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	fakediscovery "k8s.io/client-go/discovery/fake"
	"k8s.io/client-go/kubernetes/fake"
	ktesting "k8s.io/client-go/testing"
)

const testNamespace = "radius-system"

func deployment(name string, replicas int32) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: testNamespace},
		Spec:       appsv1.DeploymentSpec{Replicas: &replicas},
		// The fake clientset does not run controllers, so seed status to match spec so the
		// scale-down/up waits converge immediately.
		Status: appsv1.DeploymentStatus{Replicas: replicas, AvailableReplicas: replicas},
	}
}

// newReconcilingClientset returns a fake clientset that mimics the deployment controller: on every
// create/update it copies Spec.Replicas into the status fields. Without this the fake never moves
// status to match a scale request and the scaler's status-based waits would never converge.
func newReconcilingClientset(objs ...runtime.Object) *fake.Clientset {
	clientset := fake.NewSimpleClientset(objs...)
	clientset.Discovery().(*fakediscovery.FakeDiscovery).Resources = []*metav1.APIResourceList{
		{GroupVersion: radiusAPIGroupVersion},
	}
	reconcile := func(action ktesting.Action) (bool, runtime.Object, error) {
		obj := action.(interface{ GetObject() runtime.Object }).GetObject()
		dep, ok := obj.(*appsv1.Deployment)
		if !ok || dep.Spec.Replicas == nil {
			return false, nil, nil
		}
		dep.Status.Replicas = *dep.Spec.Replicas
		dep.Status.AvailableReplicas = *dep.Spec.Replicas
		// Return handled=false so the default tracker still persists the (now-mutated) object.
		return false, dep, nil
	}
	clientset.PrependReactor("create", "deployments", reconcile)
	clientset.PrependReactor("update", "deployments", reconcile)
	return clientset
}

func Test_ScaleDown_RecordsReplicasAndZeroes(t *testing.T) {
	clientset := newReconcilingClientset(
		deployment("ucp", 1),
		deployment("applications-rp", 2),
		deployment("dynamic-rp", 1),
	)
	scaler := NewScaler(clientset, testNamespace)

	saved, err := scaler.ScaleDown(t.Context())
	require.NoError(t, err)
	require.Equal(t, map[string]int32{"ucp": 1, "applications-rp": 2, "dynamic-rp": 1}, saved)

	for _, name := range Deployments {
		d, err := clientset.AppsV1().Deployments(testNamespace).Get(t.Context(), name, metav1.GetOptions{})
		require.NoError(t, err)
		require.Equal(t, int32(0), *d.Spec.Replicas, "deployment %q should be scaled to zero", name)
	}
}

func Test_ScaleDown_SkipsMissingDeployments(t *testing.T) {
	// Only ucp exists; the other two are absent (partial install) and must be skipped.
	clientset := newReconcilingClientset(deployment("ucp", 1))
	scaler := NewScaler(clientset, testNamespace)

	saved, err := scaler.ScaleDown(t.Context())
	require.NoError(t, err)
	require.Equal(t, map[string]int32{"ucp": 1}, saved)
}

func Test_ScaleUp_RestoresSavedReplicas(t *testing.T) {
	clientset := newReconcilingClientset(
		deployment("ucp", 0),
		deployment("applications-rp", 0),
	)
	scaler := NewScaler(clientset, testNamespace)

	err := scaler.ScaleUp(t.Context(), map[string]int32{"ucp": 1, "applications-rp": 3})
	require.NoError(t, err)

	ucp, err := clientset.AppsV1().Deployments(testNamespace).Get(t.Context(), "ucp", metav1.GetOptions{})
	require.NoError(t, err)
	require.Equal(t, int32(1), *ucp.Spec.Replicas)

	rp, err := clientset.AppsV1().Deployments(testNamespace).Get(t.Context(), "applications-rp", metav1.GetOptions{})
	require.NoError(t, err)
	require.Equal(t, int32(3), *rp.Spec.Replicas)
}

func Test_ScaleUp_RetriesUntilRadiusAPIServiceAvailable(t *testing.T) {
	clientset := newReconcilingClientset(deployment("ucp", 0))
	attempts := 0
	clientset.PrependReactor("get", "resource", func(ktesting.Action) (bool, runtime.Object, error) {
		attempts++
		if attempts == 1 {
			return true, nil, errors.New("service unavailable")
		}
		return false, nil, nil
	})

	scaler := NewScaler(clientset, testNamespace)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	err := scaler.ScaleUp(ctx, map[string]int32{"ucp": 1})
	require.NoError(t, err)
	require.Equal(t, 2, attempts)
}

func Test_ScaleUp_SkipsAPIServiceWaitWhenUCPRemainsScaledDown(t *testing.T) {
	clientset := newReconcilingClientset(deployment("ucp", 0))
	attempts := 0
	clientset.PrependReactor("get", "resource", func(ktesting.Action) (bool, runtime.Object, error) {
		attempts++
		return true, nil, errors.New("service unavailable")
	})

	scaler := NewScaler(clientset, testNamespace)
	err := scaler.ScaleUp(t.Context(), map[string]int32{"ucp": 0})
	require.NoError(t, err)
	require.Zero(t, attempts)
}

func Test_ScaleDownThenUp_RoundTrip(t *testing.T) {
	clientset := newReconcilingClientset(
		deployment("ucp", 1),
		deployment("applications-rp", 2),
		deployment("dynamic-rp", 1),
	)
	scaler := NewScaler(clientset, testNamespace)
	ctx := t.Context()

	saved, err := scaler.ScaleDown(ctx)
	require.NoError(t, err)
	require.NoError(t, scaler.ScaleUp(ctx, saved))

	for name, want := range map[string]int32{"ucp": 1, "applications-rp": 2, "dynamic-rp": 1} {
		d, err := clientset.AppsV1().Deployments(testNamespace).Get(ctx, name, metav1.GetOptions{})
		require.NoError(t, err)
		require.Equal(t, want, *d.Spec.Replicas, "deployment %q replicas should be restored", name)
	}
}

func Test_replicasOf_DefaultsToOne(t *testing.T) {
	require.Equal(t, int32(1), replicasOf(&appsv1.Deployment{}), "nil replicas defaults to 1")

	zero := int32(0)
	require.Equal(t, int32(0), replicasOf(&appsv1.Deployment{Spec: appsv1.DeploymentSpec{Replicas: &zero}}),
		"an explicit zero is preserved, not coerced to 1")

	three := int32(3)
	require.Equal(t, int32(3), replicasOf(&appsv1.Deployment{Spec: appsv1.DeploymentSpec{Replicas: &three}}))
}
