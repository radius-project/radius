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
	"encoding/json"
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
	"k8s.io/apimachinery/pkg/types"
	kubefake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
	k8stesting "k8s.io/client-go/testing"
	"k8s.io/utils/ptr"
)

func aggregationOnlyState(t *testing.T, state rotationState) rotationState {
	t.Helper()
	encoded, err := json.Marshal(state)
	require.NoError(t, err)
	var copy rotationState
	require.NoError(t, json.Unmarshal(encoded, &copy))
	workload, err := copy.Plan.aggregationWorkload()
	require.NoError(t, err)
	copy.SchemaVersion = 2
	copy.Plan.Scope, copy.Plan.Aggregation = rotationAggregationOnly, &workload
	copy.Plan.OldComponentCA, copy.Plan.Consumers = nil, nil
	return copy
}

func TestRotationScopeJournalCompatibility(t *testing.T) {
	_, legacy := rotationRunnerFixture(t)
	tests := []struct {
		name   string
		change func(*rotationState)
		valid  bool
	}{
		{"legacy managed", func(s *rotationState) {}, true},
		{"version two aggregation", func(s *rotationState) { *s = aggregationOnlyState(t, *s) }, true},
		{"version two managed", func(s *rotationState) {
			w, err := s.Plan.aggregationWorkload()
			require.NoError(t, err)
			s.SchemaVersion, s.Plan.Scope, s.Plan.Aggregation = 2, rotationComponentsAndAggregation, &w
		}, true},
		{"legacy explicit scope", func(s *rotationState) { s.Plan.Scope = rotationComponentsAndAggregation }, false},
		{"legacy aggregation identity", func(s *rotationState) {
			s.Plan.Aggregation = &rotationAggregationWorkload{DeploymentUID: "uid", ProofPort: 9090}
		}, false},
		{"v2 missing scope", func(s *rotationState) { s.SchemaVersion = 2 }, false},
		{"v2 missing aggregation", func(s *rotationState) { s.SchemaVersion, s.Plan.Scope = 2, rotationComponentsAndAggregation }, false},
		{"unknown scope", func(s *rotationState) { *s = aggregationOnlyState(t, *s); s.Plan.Scope = "unknown" }, false},
		{"aggregate component CA", func(s *rotationState) {
			*s = aggregationOnlyState(t, *s)
			s.Plan.OldComponentCA = legacy.Plan.OldComponentCA
		}, false},
		{"aggregate component consumers", func(s *rotationState) { *s = aggregationOnlyState(t, *s); s.Plan.Consumers = legacy.Plan.Consumers }, false},
		{"aggregate empty UID", func(s *rotationState) { *s = aggregationOnlyState(t, *s); s.Plan.Aggregation.DeploymentUID = "" }, false},
		{"aggregate empty port", func(s *rotationState) { *s = aggregationOnlyState(t, *s); s.Plan.Aggregation.ProofPort = 0 }, false},
		{"managed missing component", func(s *rotationState) { s.Plan.Consumers = s.Plan.Consumers[:4] }, false},
		{"managed different UCP", func(s *rotationState) {
			s.SchemaVersion, s.Plan.Scope = 2, rotationComponentsAndAggregation
			s.Plan.Aggregation = &rotationAggregationWorkload{DeploymentUID: "wrong", ProofPort: 9090}
		}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded, err := json.Marshal(legacy)
			require.NoError(t, err)
			var state rotationState
			require.NoError(t, json.Unmarshal(encoded, &state))
			tt.change(&state)
			encoded, err = json.Marshal(state)
			require.NoError(t, err)
			decoded, err := decodeRotationState(string(encoded))
			require.NoError(t, err)
			err = decoded.validate(time.Now())
			if tt.valid {
				require.NoError(t, err)
				require.NotContains(t, string(encoded), "null")
				if decoded.SchemaVersion == 1 {
					require.Equal(t, rotationComponentsAndAggregation, decoded.Plan.effectiveScope())
				}
			} else {
				require.Error(t, err)
			}
		})
	}
}

func TestAggregationOnlyPlanning(t *testing.T) {
	runner, state := rotationRunnerFixture(t)
	s, ctx := runner.store, t.Context()
	kube := s.kube.(*kubefake.Clientset)
	deployment, err := kube.AppsV1().Deployments(s.namespace).Get(ctx, "ucp", metav1.GetOptions{})
	require.NoError(t, err)
	deployment.Spec.ProgressDeadlineSeconds = ptr.To(int32(600))
	deployment.Spec.Template.Spec.Containers[0].Ports = []corev1.ContainerPort{{Name: "metrics", ContainerPort: 9090}}
	require.NoError(t, kube.Tracker().Update(appsv1.SchemeGroupVersion.WithResource("deployments"), deployment, s.namespace))
	require.NoError(t, kube.Tracker().Delete(corev1.SchemeGroupVersion.WithResource("configmaps"), s.namespace, rotationJournalName))
	kube.ClearActions()
	selection, err := planRotationResources(ctx, s, nil, state.ID, state.Plan.SigningSecret, state.Plan.TargetMode,
		nil, state.NewCA, nil, nil, nil, rotationAggregationOnly)
	require.NoError(t, err)
	plan := selection.Plan
	require.Empty(t, plan.Consumers)
	require.Empty(t, plan.OldComponentCA)
	require.Equal(t, rotationAggregationOnly, plan.Scope)
	for _, action := range kube.Actions() {
		require.Contains(t, []string{"get", "list"}, action.GetVerb())
		if action.GetResource().Resource == "deployments" {
			require.Equal(t, "ucp", action.(k8stesting.GetAction).GetName())
		}
	}
	state, err = s.beginPlanned(ctx, selection.ID, selection.NewCA, plan)
	require.NoError(t, err)
	kube.ClearActions()
	require.NoError(t, runner.run(ctx, state.ID, rotationPublish))
	_, current, err := s.read(ctx)
	require.NoError(t, err)
	require.Equal(t, rotationAdopt, current.Phase)
	require.Error(t, runner.verifyConsumers(ctx, state))
	_, err = s.rotationConsumerTargets(ctx, state)
	require.Error(t, err)
	for _, action := range kube.Actions() {
		require.Contains(t, []string{"configmaps", "secrets", "namespaces"}, action.GetResource().Resource)
		if action.GetResource().Resource == "namespaces" {
			require.Equal(t, "get", action.GetVerb())
			require.Equal(t, s.namespace, action.(k8stesting.GetAction).GetName())
		}
		if action.GetResource().Resource == "secrets" {
			require.Equal(t, plan.SigningSecret, action.(k8stesting.GetAction).GetName())
		}
	}
}

func completedAggregationFixture(t *testing.T) (*rotationStore, rotationState) {
	t.Helper()
	runner, state := rotationRunnerFixture(t)
	for _, phase := range []rotationPhase{rotationPublish, rotationAdopt, rotationIssue, rotationRetire} {
		require.NoError(t, runner.run(t.Context(), state.ID, phase))
	}
	_, state, err := runner.store.read(t.Context())
	require.NoError(t, err)
	// Management-listener declarations are not part of the old simulated
	// component fixture; make the new serving-evidence inventory explicit.
	kube, ns, ctx := runner.store.kube.(*kubefake.Clientset), runner.store.namespace, t.Context()
	deployment, err := kube.AppsV1().Deployments(ns).Get(ctx, "ucp", metav1.GetOptions{})
	require.NoError(t, err)
	ports := []corev1.ContainerPort{{Name: "metrics", ContainerPort: 9090}}
	deployment.Spec.Template.Spec.Containers[0].Ports = ports
	require.NoError(t, kube.Tracker().Update(appsv1.SchemeGroupVersion.WithResource("deployments"), deployment, ns))
	sets, err := kube.AppsV1().ReplicaSets(ns).List(ctx, metav1.ListOptions{})
	require.NoError(t, err)
	for _, set := range sets.Items {
		if set.OwnerReferences[0].Name == "ucp" {
			set.Spec.Template.Spec.Containers[0].Ports = ports
			require.NoError(t, kube.Tracker().Update(appsv1.SchemeGroupVersion.WithResource("replicasets"), &set, ns))
		}
	}
	pods, err := kube.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{LabelSelector: "app.kubernetes.io/name=ucp"})
	require.NoError(t, err)
	for _, pod := range pods.Items {
		pod.Spec.Containers[0].Ports = ports
		require.NoError(t, kube.Tracker().Update(corev1.SchemeGroupVersion.WithResource("pods"), &pod, ns))
	}
	return runner.store, state
}

func TestAggregationOnlyInventory(t *testing.T) {
	for _, scenario := range []string{"success", "wrong kind", "wrong leaf", "changed pod", "missing pod", "old ReplicaSet", "unowned pod", "unlabelled old consumer", "mutable trust", "bad mount", "wrong port", "observer failure"} {
		t.Run(scenario, func(t *testing.T) {
			s, managed := completedAggregationFixture(t)
			state := aggregationOnlyState(t, managed)
			ctx, kube := t.Context(), s.kube.(*kubefake.Clientset)
			inventory, err := s.aggregationInventory(ctx, state)
			require.NoError(t, err)
			require.Len(t, inventory.targets, 1)
			target := inventory.targets[0]
			pod, err := kube.CoreV1().Pods(s.namespace).Get(ctx, target.PodName, metav1.GetOptions{})
			require.NoError(t, err)
			switch scenario {
			case "missing pod":
				require.NoError(t, kube.CoreV1().Pods(s.namespace).Delete(ctx, pod.Name, metav1.DeleteOptions{}))
			case "unowned pod":
				pod.Name, pod.UID, pod.OwnerReferences = "rogue", types.UID(uuid.NewString()), nil
				_, err := kube.CoreV1().Pods(s.namespace).Create(ctx, pod, metav1.CreateOptions{})
				require.NoError(t, err)
			case "unlabelled old consumer":
				pod.Name, pod.UID, pod.OwnerReferences, pod.Labels = "rogue", types.UID(uuid.NewString()), nil, nil
				pod.Spec.Volumes = []corev1.Volume{{Name: "renamed", VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{SecretName: "ucp-cert"}}}}
				_, err := kube.CoreV1().Pods(s.namespace).Create(ctx, pod, metav1.CreateOptions{})
				require.NoError(t, err)
			case "wrong port":
				pod.Spec.Containers[0].Ports[0].ContainerPort++
				_, err := kube.CoreV1().Pods(s.namespace).Update(ctx, pod, metav1.UpdateOptions{})
				require.NoError(t, err)
			case "bad mount":
				pod.Spec.Containers[0].VolumeMounts = nil
				_, err := kube.CoreV1().Pods(s.namespace).Update(ctx, pod, metav1.UpdateOptions{})
				require.NoError(t, err)
			case "old ReplicaSet":
				sets, err := kube.AppsV1().ReplicaSets(s.namespace).List(ctx, metav1.ListOptions{})
				require.NoError(t, err)
				for _, set := range sets.Items {
					if set.OwnerReferences[0].Name == "ucp" {
						set.Spec.Template.Spec.Containers[0].Image = "old-image"
						_, err = kube.AppsV1().ReplicaSets(s.namespace).Update(ctx, &set, metav1.UpdateOptions{})
						require.NoError(t, err)
					}
				}
			case "mutable trust":
				trust, err := kube.CoreV1().ConfigMaps(s.namespace).Get(ctx, state.Plan.TargetTrustMap, metav1.GetOptions{})
				require.NoError(t, err)
				trust.Immutable = ptr.To(false)
				_, err = kube.CoreV1().ConfigMaps(s.namespace).Update(ctx, trust, metav1.UpdateOptions{})
				require.NoError(t, err)
			}
			kube.ClearActions()
			observe := func(_ context.Context, _ *rest.Config, target TLSProofTarget) (PodAggregationProof, error) {
				if scenario == "observer failure" {
					return PodAggregationProof{}, errors.New("unavailable")
				}
				_, roots, err := mtls.TrustBundleDigest(state.NewCA, time.Now())
				require.NoError(t, err)
				evidence := mtls.AggregationEvidence{SchemaVersion: 1, Ready: true, ProofKind: "aggregation-serving-tls-initialized",
					PodUID: string(target.PodUID), BootID: uuid.NewString(), InitializedAt: time.Now().Add(-time.Second),
					TrustBundleSHA256: target.TrustBundleSHA256, CASHA256: roots, LeafSHA256: target.LeafSHA256,
					NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour)}
				switch scenario {
				case "wrong kind":
					evidence.ProofKind = "startup-tls-initialized"
				case "wrong leaf":
					evidence.LeafSHA256 = strings.Repeat("a", 64)
				case "changed pod":
					_, err := kube.CoreV1().Pods(s.namespace).Update(ctx, pod, metav1.UpdateOptions{})
					require.NoError(t, err)
				}
				return PodAggregationProof{Evidence: evidence}, nil
			}
			err = s.verifyAggregationServing(ctx, nil, state, observe)
			if scenario == "success" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
			for _, action := range kube.Actions() {
				if action.GetResource().Resource == "secrets" {
					require.Equal(t, state.Plan.ServingSecret, action.(k8stesting.GetAction).GetName())
				}
			}
		})
	}
}
