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
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ktesting "k8s.io/client-go/testing"
	"k8s.io/utils/ptr"
)

func TestProviderExternalFleetInventory(t *testing.T) {
	t.Parallel()
	for _, scenario := range []string{"valid", "missing replica", "terminating", "unready", "wrong ServiceAccount",
		"wrong owner name", "wrong owner kind", "unowned", "old ReplicaSet", "missing controller port", "pagination"} {
		t.Run(scenario, func(t *testing.T) {
			store, _, targets, kube := providerFakeFixture(t, "radius", "external", "enforce")
			ctx, namespace := t.Context(), RadiusSystemNamespace
			for _, name := range rotationComponents {
				d, err := kube.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
				require.NoError(t, err)
				d.Spec.Replicas = ptr.To(int32(2))
				d.Spec.Template.Spec.ServiceAccountName = name
				if name == "controller" {
					d.Spec.Replicas = ptr.To(int32(1))
					d.Spec.Template.Spec.Containers[0].Ports = []corev1.ContainerPort{{Name: "metrics", ContainerPort: 8080}}
				}
				if name == "bicep-de" {
					d.Spec.Template.Spec.Containers[0].Name = "de"
				}
				d, err = kube.AppsV1().Deployments(namespace).Update(ctx, d, metav1.UpdateOptions{})
				require.NoError(t, err)
				target := *d.Spec.DeepCopy()
				target.Template.Spec.Containers[0].Env[0].Value = "external"
				targets[name] = target
			}
			plan, err := store.plan(ctx, namespace, "radius", "external", "enforce", targets)
			require.NoError(t, err)
			require.NoError(t, store.begin(ctx, plan, targets, true))
			for range 4 {
				providerStepConverged(t, store, plan, targets)
			}
			for _, workload := range plan.Workloads {
				d, err := kube.AppsV1().Deployments(namespace).Get(ctx, workload.Name, metav1.GetOptions{})
				require.NoError(t, err)
				set, err := kube.AppsV1().ReplicaSets(namespace).Create(ctx, &appsv1.ReplicaSet{
					ObjectMeta: metav1.ObjectMeta{Name: workload.Name + "-target", Namespace: namespace,
						OwnerReferences: []metav1.OwnerReference{{APIVersion: "apps/v1", Kind: "Deployment",
							Name: workload.Name, UID: workload.UID, Controller: ptr.To(true)}}},
					Spec: appsv1.ReplicaSetSpec{Replicas: d.Spec.Replicas, Selector: d.Spec.Selector, Template: d.Spec.Template},
				}, metav1.CreateOptions{})
				require.NoError(t, err)
				for index := int32(0); index < workload.Replicas; index++ {
					_, pod := identityTargetFixture(workload.Name)
					pod.Name = fmt.Sprintf("%s-%d", workload.Name, index)
					pod.Spec = *d.Spec.Template.Spec.DeepCopy()
					pod.Labels = d.Spec.Template.Labels
					pod.Status.ContainerStatuses[0].Name = d.Spec.Template.Spec.Containers[0].Name
					pod.OwnerReferences = []metav1.OwnerReference{{APIVersion: "apps/v1", Kind: "ReplicaSet",
						Name: set.Name, UID: set.UID, Controller: ptr.To(true)}}
					_, err := kube.CoreV1().Pods(namespace).Create(ctx, pod, metav1.CreateOptions{})
					require.NoError(t, err)
				}
			}
			checks := providerRuntimeChecks{ca: &rotationStore{kube: kube, namespace: namespace}}
			before, err := checks.externalInventory(ctx, plan)
			require.NoError(t, err)
			require.Len(t, before.versions, 19)
			require.Len(t, before.targets["ucp"], 2)
			require.Equal(t, "de", before.targets["bicep-de"][0].ContainerName)
			require.Equal(t, uint16(8080), before.targets["controller"][0].Port)
			pod, err := kube.CoreV1().Pods(namespace).Get(ctx, "ucp-0", metav1.GetOptions{})
			require.NoError(t, err)
			switch scenario {
			case "missing replica":
				require.NoError(t, kube.CoreV1().Pods(namespace).Delete(ctx, pod.Name, metav1.DeleteOptions{}))
			case "terminating":
				pod.DeletionTimestamp = ptr.To(metav1.NewTime(time.Now()))
			case "unready":
				pod.Status.ContainerStatuses[0].Ready = false
			case "wrong ServiceAccount":
				pod.Spec.ServiceAccountName = "applications-rp"
			case "wrong owner name":
				pod.OwnerReferences[0].Name = "different"
			case "wrong owner kind":
				pod.OwnerReferences[0].Kind = "Job"
			case "unowned":
				pod.OwnerReferences = nil
			case "old ReplicaSet":
				set, err := kube.AppsV1().ReplicaSets(namespace).Get(ctx, "ucp-target", metav1.GetOptions{})
				require.NoError(t, err)
				set.Spec.Template.Spec.Containers[0].Image = "test.invalid/old"
				_, err = kube.AppsV1().ReplicaSets(namespace).Update(ctx, set, metav1.UpdateOptions{})
				require.NoError(t, err)
			case "missing controller port":
				controller, err := kube.CoreV1().Pods(namespace).Get(ctx, "controller-0", metav1.GetOptions{})
				require.NoError(t, err)
				controller.Spec.Containers[0].Ports = nil
				_, err = kube.CoreV1().Pods(namespace).Update(ctx, controller, metav1.UpdateOptions{})
				require.NoError(t, err)
			case "pagination":
				kube.PrependReactor("list", "pods", func(ktesting.Action) (bool, runtime.Object, error) {
					return true, &corev1.PodList{ListMeta: metav1.ListMeta{Continue: "remaining"}}, nil
				})
			}
			if scenario != "missing replica" {
				_, err = kube.CoreV1().Pods(namespace).Update(ctx, pod, metav1.UpdateOptions{})
				require.NoError(t, err)
			}
			after, err := checks.externalInventory(ctx, plan)
			if scenario != "valid" {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, before, after)
			pod.UID = types.UID(uuid.NewString())
			pod.ResourceVersion = "2"
			_, err = kube.CoreV1().Pods(namespace).Update(ctx, pod, metav1.UpdateOptions{})
			require.NoError(t, err)
			replaced, err := checks.externalInventory(ctx, plan)
			require.NoError(t, err)
			require.NotEqual(t, before.versions, replaced.versions, "the observation fence must distinguish a replaced process")
		})
	}
}
