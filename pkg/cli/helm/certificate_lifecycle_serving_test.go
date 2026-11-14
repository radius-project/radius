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
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	kubefake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
	"k8s.io/utils/ptr"
)

func lifecycleServingFixture(t *testing.T) (*kubefake.Clientset, certificateLifecycle, *certificateTarget) {
	t.Helper()
	state, _ := lifecycleFixture(t)
	state.Phase = rotationIssue
	aggregation := state.Planes["aggregation"]
	aggregation.Affected = false
	state.Planes["aggregation"] = aggregation
	target := &certificateTarget{Provider: "radius", TrustDomain: SPIFFETrustDomainDefault,
		Receivers: []string{"ucp", "applications-rp", "dynamic-rp", "bicep-de"}, Values: map[string]any{}}
	meta := func(name string) metav1.ObjectMeta {
		return metav1.ObjectMeta{Name: name, Namespace: state.Namespace, UID: types.UID(name), ResourceVersion: "1"}
	}
	driver := &storagev1.CSIDriver{ObjectMeta: meta(SPIFFECSIDriverNameDefault)}
	driver.Namespace = ""
	objects := []runtime.Object{driver}
	for _, name := range target.Receivers {
		ports := map[string][]corev1.ContainerPort{
			"ucp":             {{Name: "internal", ContainerPort: 9444}},
			"applications-rp": {{Name: "applications-rp", ContainerPort: 5443}, {Name: "app-pr-rp", ContainerPort: 5444}},
			"dynamic-rp":      {{Name: "dynamic-rp", ContainerPort: 8082}},
			"bicep-de":        {{Name: "bicep-de-api", ContainerPort: 6443}},
		}[name]
		labels := map[string]string{"app": name}
		template := corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: labels},
			Spec: corev1.PodSpec{ServiceAccountName: name,
				Containers: []corev1.Container{{Name: name, Image: "fixture", Ports: ports,
					VolumeMounts: []corev1.VolumeMount{{Name: "spiffe", MountPath: "/var/run/radius/spiffe", ReadOnly: true}}}},
				Volumes: []corev1.Volume{{Name: "spiffe", VolumeSource: corev1.VolumeSource{CSI: &corev1.CSIVolumeSource{Driver: driver.Name}}}}}}
		deployment := &appsv1.Deployment{ObjectMeta: meta(name),
			Spec:   appsv1.DeploymentSpec{Replicas: ptr.To(int32(1)), Selector: &metav1.LabelSelector{MatchLabels: labels}, Template: template},
			Status: appsv1.DeploymentStatus{ObservedGeneration: 1, Replicas: 1, UpdatedReplicas: 1, ReadyReplicas: 1, AvailableReplicas: 1}}
		deployment.Generation = 1
		rs := &appsv1.ReplicaSet{ObjectMeta: meta(name + "-rs"), Spec: appsv1.ReplicaSetSpec{Template: *template.DeepCopy()}}
		rs.OwnerReferences = []metav1.OwnerReference{{APIVersion: "apps/v1", Kind: "Deployment", Name: name, UID: deployment.UID, Controller: ptr.To(true)}}
		pod := &corev1.Pod{ObjectMeta: meta(name + "-one"), Spec: *template.Spec.DeepCopy(),
			Status: corev1.PodStatus{Phase: corev1.PodRunning,
				Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}},
				ContainerStatuses: []corev1.ContainerStatus{{Name: name, Ready: true, ContainerID: "fixture://" + name,
					State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{StartedAt: metav1.NewTime(time.Now().Add(-time.Minute))}}}}}}
		pod.Labels = labels
		pod.OwnerReferences = []metav1.OwnerReference{{APIVersion: "apps/v1", Kind: "ReplicaSet", Name: rs.Name, UID: rs.UID, Controller: ptr.To(true)}}
		objects = append(objects, deployment, rs, pod, &corev1.ServiceAccount{ObjectMeta: meta(name)})
	}
	return kubefake.NewClientset(objects...), state, target
}

func TestLifecycleCSIInventoryAndObservation(t *testing.T) {
	for _, scenario := range []string{"complete fleet", "missing receiver", "rolling", "old replica set", "not ready", "wrong account",
		"not mounted", "driver replaced", "pod restarted", "serving rejects", "canceled"} {
		t.Run(scenario, func(t *testing.T) {
			kube, state, target := lifecycleServingFixture(t)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			switch scenario {
			case "missing receiver":
				require.NoError(t, kube.AppsV1().Deployments(state.Namespace).Delete(ctx, "dynamic-rp", metav1.DeleteOptions{}))
			case "rolling":
				deployment, err := kube.AppsV1().Deployments(state.Namespace).Get(ctx, "ucp", metav1.GetOptions{})
				require.NoError(t, err)
				deployment.Status.UpdatedReplicas = 0
				_, err = kube.AppsV1().Deployments(state.Namespace).UpdateStatus(ctx, deployment, metav1.UpdateOptions{})
				require.NoError(t, err)
			case "old replica set":
				rs, err := kube.AppsV1().ReplicaSets(state.Namespace).Get(ctx, "ucp-rs", metav1.GetOptions{})
				require.NoError(t, err)
				rs.Spec.Template.Spec.Containers[0].Image = "old-fixture"
				_, err = kube.AppsV1().ReplicaSets(state.Namespace).Update(ctx, rs, metav1.UpdateOptions{})
				require.NoError(t, err)
			case "not ready", "wrong account", "not mounted":
				pod, err := kube.CoreV1().Pods(state.Namespace).Get(ctx, "ucp-one", metav1.GetOptions{})
				require.NoError(t, err)
				switch scenario {
				case "not ready":
					pod.Status.ContainerStatuses[0].Ready = false
				case "wrong account":
					pod.Spec.ServiceAccountName = "other"
				case "not mounted":
					pod.Spec.Containers[0].VolumeMounts = nil
				}
				_, err = kube.CoreV1().Pods(state.Namespace).Update(ctx, pod, metav1.UpdateOptions{})
				require.NoError(t, err)
			case "canceled":
				cancel()
			}
			observations := 0
			// Inject only the TLS measurement. Real tunnel/chain checks are covered
			// by TestObserveServingCertificate; this is not a running CSI fleet.
			observe := func(ctx context.Context, _ *rest.Config, selected ServingCertificateTarget) (ServingCertificateObservation, error) {
				observations++
				require.Equal(t, state.Planes["internal"].NewCA, selected.CA, "serving measurement must require new-only trust")
				require.NotEmpty(t, selected.DriverUID)
				require.NotEmpty(t, selected.ServiceAccountUID)
				require.NotEmpty(t, selected.WorkloadUID)
				require.NotEmpty(t, selected.PodUID)
				if observations == 1 && scenario == "driver replaced" {
					driver, err := kube.StorageV1().CSIDrivers().Get(ctx, SPIFFECSIDriverNameDefault, metav1.GetOptions{})
					require.NoError(t, err)
					driver.UID = "replacement"
					_, err = kube.StorageV1().CSIDrivers().Update(ctx, driver, metav1.UpdateOptions{})
					require.NoError(t, err)
				}
				if observations == 1 && scenario == "pod restarted" {
					pod, err := kube.CoreV1().Pods(state.Namespace).Get(ctx, "ucp-one", metav1.GetOptions{})
					require.NoError(t, err)
					pod.ResourceVersion = "2"
					pod.Status.ContainerStatuses[0].RestartCount++
					_, err = kube.CoreV1().Pods(state.Namespace).UpdateStatus(ctx, pod, metav1.UpdateOptions{})
					require.NoError(t, err)
				}
				if scenario == "serving rejects" {
					return ServingCertificateObservation{}, errors.New("fixture TLS chain rejected")
				}
				return ServingCertificateObservation{PodUID: selected.PodUID}, ctx.Err()
			}
			err := observeLifecycleServing(ctx, nil, kube, state, target, observe)
			if scenario == "complete fleet" {
				require.NoError(t, err)
				require.Equal(t, 4, observations, "include every actual receiver listener and the gated engine, not the unused legacy 5444 declaration")
			} else {
				require.Error(t, err)
			}
		})
	}
}
