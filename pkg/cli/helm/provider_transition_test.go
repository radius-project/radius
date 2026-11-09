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
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	ktesting "k8s.io/client-go/testing"
	"k8s.io/utils/ptr"
)

type providerChecksStub struct {
	snapshot     providerAggregationSnapshot
	preflightErr error
	verifyErr    error
	verifyHook   func()
	verified     int
}

func (c *providerChecksStub) aggregation(context.Context) (providerAggregationSnapshot, error) {
	return c.snapshot, nil
}

func (c *providerChecksStub) preflight(_ context.Context, plan providerTransitionPlan, targets map[string]appsv1.DeploymentSpec) error {
	if c.preflightErr != nil {
		return c.preflightErr
	}
	for _, spec := range targets {
		if spec.Template.Spec.Containers[0].Env[0].Value != plan.Target {
			return errors.New("wrong target provider")
		}
	}
	return nil
}

func (c *providerChecksStub) verify(context.Context, providerTransitionPlan) error {
	c.verified++
	if c.verifyHook != nil {
		c.verifyHook()
	}
	return c.verifyErr
}

func providerFixture(t *testing.T, kube kubernetes.Interface, source, target, mode string) (*providerTransitionStore, providerTransitionPlan, map[string]appsv1.DeploymentSpec) {
	t.Helper()
	plan := providerTransitionPlan{
		ID: uuid.NewString(), Namespace: RadiusSystemNamespace, Release: radiusReleaseName,
		Source: source, Target: target, Mode: mode,
		Aggregation: providerAggregationSnapshot{
			APIServiceUID: "api", JournalUID: "journal", Revision: uuid.NewString() + "/complete",
			TrustSHA256: strings.Repeat("a", 64), SignerSHA256: strings.Repeat("b", 64), Scope: "aggregationOnly", Complete: true,
		},
	}
	targets := map[string]appsv1.DeploymentSpec{}
	for index, name := range rotationComponents {
		replicas := int32(index % 3)
		d := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: plan.Namespace,
				UID: types.UID(uuid.NewString()), ResourceVersion: "1",
				Labels: map[string]string{"app.kubernetes.io/managed-by": "Helm"},
				Annotations: map[string]string{
					"meta.helm.sh/release-name": plan.Release, "meta.helm.sh/release-namespace": plan.Namespace,
				}},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": name}},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": name}},
					Spec: corev1.PodSpec{Containers: []corev1.Container{{
						Name: name, Image: "test.invalid/radius:fixed",
						Env: []corev1.EnvVar{{Name: "PROVIDER", Value: source}},
					}}},
				},
			},
		}
		// The API server owns these fields; fake clients need explicit identities.
		if _, ok := kube.(*fake.Clientset); !ok {
			d.UID, d.ResourceVersion = "", ""
		}
		created, err := kube.AppsV1().Deployments(plan.Namespace).Create(t.Context(), d, metav1.CreateOptions{})
		require.NoError(t, err)
		spec := *created.Spec.DeepCopy()
		spec.Template.Spec.Containers[0].Env[0].Value = target
		spec.Template.Spec.Containers[0].Image = "test.invalid/radius:paired"
		targets[name] = spec
	}
	store := &providerTransitionStore{kube: kube, checks: &providerChecksStub{snapshot: plan.Aggregation}}
	planned, err := store.plan(t.Context(), plan.Namespace, source, target, mode, targets)
	require.NoError(t, err)
	return store, planned, targets
}

func providerFakeFixture(t *testing.T, source, target, mode string) (*providerTransitionStore, providerTransitionPlan, map[string]appsv1.DeploymentSpec, *fake.Clientset) {
	t.Helper()
	kube := fake.NewClientset()
	kube.PrependReactor("create", "*", func(action ktesting.Action) (bool, runtime.Object, error) {
		object := action.(ktesting.CreateAction).GetObject()
		if metadata, ok := object.(metav1.Object); ok {
			if metadata.GetUID() == "" {
				metadata.SetUID(types.UID(uuid.NewString()))
			}
			if metadata.GetResourceVersion() == "" {
				metadata.SetResourceVersion("1")
			}
		}
		return false, nil, nil
	})
	s, plan, targets := providerFixture(t, kube, source, target, mode)
	kube.ClearActions()
	return s, plan, targets, kube
}

func providerConverge(t *testing.T, s *providerTransitionStore, plan providerTransitionPlan) {
	t.Helper()
	for _, w := range plan.Workloads {
		d, err := s.kube.AppsV1().Deployments(plan.Namespace).Get(t.Context(), w.Name, metav1.GetOptions{})
		require.NoError(t, err)
		d.Status = appsv1.DeploymentStatus{
			ObservedGeneration: d.Generation, Replicas: *d.Spec.Replicas,
			UpdatedReplicas: *d.Spec.Replicas, AvailableReplicas: *d.Spec.Replicas, ReadyReplicas: *d.Spec.Replicas,
		}
		_, err = s.kube.AppsV1().Deployments(plan.Namespace).UpdateStatus(t.Context(), d, metav1.UpdateOptions{})
		require.NoError(t, err)
	}
}

func providerAssertNoWrites(t *testing.T, kube *fake.Clientset) {
	t.Helper()
	for _, action := range kube.Actions() {
		require.Contains(t, []string{"get", "list"}, action.GetVerb(), "%s %s", action.GetVerb(), action.GetResource().Resource)
	}
}

func providerStepConverged(t *testing.T, s *providerTransitionStore, plan providerTransitionPlan, targets map[string]appsv1.DeploymentSpec) {
	t.Helper()
	for range 10 {
		err := s.step(t.Context(), plan.Namespace, plan.ID, targets)
		if !errors.Is(err, errProviderTransitionWaiting) {
			require.NoError(t, err)
			return
		}
		providerConverge(t, s, plan)
	}
	t.Fatal("provider transition did not converge")
}

func TestProviderTransitionValidation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		change func(*providerTransitionState)
	}{
		{"unknown source", func(s *providerTransitionState) { s.Plan.Source = "mesh" }},
		{"empty provider", func(s *providerTransitionState) { s.Plan.Target = "" }},
		{"same provider", func(s *providerTransitionState) { s.Plan.Target = s.Plan.Source }},
		{"off bridge", func(s *providerTransitionState) { s.Plan.Mode = "off" }},
		{"legacy aggregation", func(s *providerTransitionState) { s.Plan.Aggregation.JournalUID = "" }},
		{"unfinished aggregation", func(s *providerTransitionState) { s.Plan.Aggregation.Complete = false }},
		{"active aggregation revision", func(s *providerTransitionState) { s.Plan.Aggregation.Revision = uuid.NewString() + "/issue" }},
		{"incomplete predecessor", func(s *providerTransitionState) { s.Plan.PreviousJournalUID = "previous" }},
		{"unknown aggregation scope", func(s *providerTransitionState) { s.Plan.Aggregation.Scope = "" }},
		{"missing signer", func(s *providerTransitionState) { s.Plan.Aggregation.SignerSHA256 = "" }},
		{"bad digest", func(s *providerTransitionState) { s.Plan.Aggregation.TrustSHA256 = strings.Repeat("G", 64) }},
		{"wrong namespace", func(s *providerTransitionState) { s.Plan.Namespace = "../other" }},
		{"bad phase", func(s *providerTransitionState) { s.Phase = "rollback" }},
		{"missing workload", func(s *providerTransitionState) { s.Plan.Workloads = s.Plan.Workloads[:4] }},
		{"duplicate workload", func(s *providerTransitionState) { s.Plan.Workloads[1] = s.Plan.Workloads[0] }},
		{"negative replicas", func(s *providerTransitionState) { s.Plan.Workloads[0].Replicas = -1 }},
		{"missing version", func(s *providerTransitionState) { s.Plan.Workloads[0].ResourceVersion = "" }},
		{"missing uid", func(s *providerTransitionState) { s.Plan.Workloads[0].UID = "" }},
		{"unchanged spec", func(s *providerTransitionState) { s.Plan.Workloads[0].TargetSHA256 = s.Plan.Workloads[0].SourceSHA256 }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, plan, _, _ := providerFakeFixture(t, "radius", "external", "enforce")
			state := providerTransitionState{Version: 1, Plan: plan, Phase: providerStopping}
			require.NoError(t, state.validate())
			tt.change(&state)
			require.Error(t, state.validate())
		})
	}
}

func TestProviderTransitionInvalidPreflightDoesNotWrite(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"no consent", "no evidence adapter", "credential failure", "aggregation drift", "target drift", "source drift", "wrong provider", "autoscaler"} {
		t.Run(name, func(t *testing.T) {
			s, plan, targets, kube := providerFakeFixture(t, "radius", "external", "enforce")
			confirmed := true
			switch name {
			case "no consent":
				confirmed = false
			case "no evidence adapter":
				s.checks = nil
			case "credential failure":
				s.checks.(*providerChecksStub).preflightErr = errors.New("credentials unavailable")
			case "aggregation drift":
				s.checks.(*providerChecksStub).snapshot.JournalUID = "replaced"
			case "target drift":
				spec := targets["ucp"]
				spec.Template.Spec.Containers[0].Image = "unapproved"
				targets["ucp"] = spec
			case "source drift":
				plan.Workloads[0].ResourceVersion = "outdated"
			case "wrong provider":
				spec := targets["ucp"]
				spec.Template.Spec.Containers[0].Env[0].Value = "off"
				targets["ucp"] = spec
				hash, err := providerSpecHash(spec)
				require.NoError(t, err)
				for i := range plan.Workloads {
					if plan.Workloads[i].Name == "ucp" {
						plan.Workloads[i].TargetSHA256 = hash
					}
				}
			case "autoscaler":
				_, err := kube.AutoscalingV2().HorizontalPodAutoscalers(plan.Namespace).Create(t.Context(), &autoscalingv2.HorizontalPodAutoscaler{
					ObjectMeta: metav1.ObjectMeta{Name: "hpa"},
					Spec: autoscalingv2.HorizontalPodAutoscalerSpec{ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
						APIVersion: "apps/v1", Kind: "Deployment", Name: "ucp",
					}},
				}, metav1.CreateOptions{})
				require.NoError(t, err)
				kube.ClearActions()
			}
			require.Error(t, s.begin(t.Context(), plan, targets, confirmed))
			providerAssertNoWrites(t, kube)
		})
	}
}

func TestProviderTransitionLifecycle(t *testing.T) {
	t.Parallel()
	for _, source := range []string{"radius", "external"} {
		for _, mode := range []string{"dryRun", "enforce"} {
			t.Run(source+"/"+mode, func(t *testing.T) {
				target := "external"
				if source == "external" {
					target = "radius"
				}
				s, plan, targets, kube := providerFakeFixture(t, source, target, mode)
				require.NoError(t, s.begin(t.Context(), plan, targets, true))
				kube.PrependReactor("update", "deployments", func(action ktesting.Action) (bool, runtime.Object, error) {
					if action.GetSubresource() != "" {
						return false, nil, nil
					}
					updated := action.(ktesting.UpdateAction).GetObject().(*appsv1.Deployment)
					for _, w := range plan.Workloads {
						live, err := kube.Tracker().Get(appsv1.SchemeGroupVersion.WithResource("deployments"), plan.Namespace, w.Name)
						require.NoError(t, err)
						d := live.(*appsv1.Deployment)
						newProvider := updated.Spec.Template.Spec.Containers[0].Env[0].Value
						if newProvider == target && *updated.Spec.Replicas == 0 &&
							updated.Annotations[providerTransitionMarker] == providerMarker(plan, providerStaging) {
							require.Zero(t, *d.Spec.Replicas, "target spec written before all source replicas stopped")
						}
						if newProvider == target && *updated.Spec.Replicas > 0 {
							require.Equal(t, target, d.Spec.Template.Spec.Containers[0].Env[0].Value, "mixed providers during restore")
						}
					}
					return false, nil, nil
				})
				for range []providerTransitionPhase{providerStopping, providerStaging, providerStarting, providerVerifying} {
					providerConverge(t, s, plan)
					providerStepConverged(t, s, plan, targets)
				}
				_, state, err := s.read(t.Context(), plan.Namespace)
				require.NoError(t, err)
				require.Equal(t, providerComplete, state.Phase)
				require.Equal(t, plan.Aggregation, state.Plan.Aggregation)
				for _, w := range plan.Workloads {
					d, hash, err := s.deployment(t.Context(), plan, w)
					require.NoError(t, err)
					require.Equal(t, w.Replicas, *d.Spec.Replicas)
					require.Equal(t, w.TargetSHA256, hash)
				}
				providerStepConverged(t, s, plan, targets)
				require.Equal(t, 2, s.checks.(*providerChecksStub).verified, "completion retries reverify instead of trusting stored success")
			})
		}
	}
}

func TestProviderTransitionPartialWritesResume(t *testing.T) {
	t.Parallel()
	for _, phase := range []providerTransitionPhase{providerStopping, providerStaging, providerStarting} {
		for index := 1; index <= len(rotationComponents); index++ {
			t.Run(fmt.Sprintf("%s/%d", phase, index), func(t *testing.T) {
				s, plan, targets, kube := providerFakeFixture(t, "radius", "external", "enforce")
				require.NoError(t, s.begin(t.Context(), plan, targets, true))
				for {
					_, state, err := s.read(t.Context(), plan.Namespace)
					require.NoError(t, err)
					if state.Phase == phase {
						break
					}
					providerConverge(t, s, plan)
					providerStepConverged(t, s, plan, targets)
				}
				count := 0
				kube.PrependReactor("update", "deployments", func(action ktesting.Action) (bool, runtime.Object, error) {
					if action.GetSubresource() == "" {
						count++
						if count == index {
							return true, nil, errors.New("interrupted write")
						}
					}
					return false, nil, nil
				})
				providerConverge(t, s, plan)
				require.ErrorContains(t, s.step(t.Context(), plan.Namespace, plan.ID, targets), "interrupted write")
				_, state, err := s.read(t.Context(), plan.Namespace)
				require.NoError(t, err)
				require.Equal(t, phase, state.Phase)
				for state.Phase != providerComplete {
					providerConverge(t, s, plan)
					providerStepConverged(t, s, plan, targets)
					_, state, err = s.read(t.Context(), plan.Namespace)
					require.NoError(t, err)
				}
			})
		}
	}
}

func TestProviderTransitionWaitsForPodsAndReplicaSets(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"running", "terminating", "orphan", "replicaset"} {
		t.Run(name, func(t *testing.T) {
			s, plan, targets, kube := providerFakeFixture(t, "radius", "external", "enforce")
			require.NoError(t, s.begin(t.Context(), plan, targets, true))
			var err error
			if name == "replicaset" {
				w := plan.Workloads[0]
				_, err = kube.AppsV1().ReplicaSets(plan.Namespace).Create(t.Context(), &appsv1.ReplicaSet{
					ObjectMeta: metav1.ObjectMeta{Name: "old", OwnerReferences: []metav1.OwnerReference{{
						APIVersion: "apps/v1", Kind: "Deployment", Name: w.Name, UID: w.UID, Controller: ptr.To(true),
					}}},
					Spec: appsv1.ReplicaSetSpec{Replicas: ptr.To(int32(1))},
				}, metav1.CreateOptions{})
			} else {
				pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "old", Labels: map[string]string{"app": "ucp"}}}
				if name == "terminating" {
					pod.DeletionTimestamp = ptr.To(metav1.Now())
				}
				_, err = kube.CoreV1().Pods(plan.Namespace).Create(t.Context(), pod, metav1.CreateOptions{})
			}
			require.NoError(t, err)
			require.ErrorIs(t, s.step(t.Context(), plan.Namespace, plan.ID, targets), errProviderTransitionWaiting)
			_, state, err := s.read(t.Context(), plan.Namespace)
			require.NoError(t, err)
			require.Equal(t, providerStopping, state.Phase)
			for _, w := range plan.Workloads {
				d, hash, err := s.deployment(t.Context(), plan, w)
				require.NoError(t, err)
				require.Zero(t, *d.Spec.Replicas)
				require.Equal(t, w.SourceSHA256, hash)
			}
		})
	}
}

func TestProviderTransitionNeverStealsLock(t *testing.T) {
	t.Parallel()
	s, plan, targets, kube := providerFakeFixture(t, "radius", "external", "enforce")
	lease, err := kube.CoordinationV1().Leases(plan.Namespace).Create(t.Context(), &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{Name: providerTransitionLock},
		Spec: coordinationv1.LeaseSpec{
			HolderIdentity: ptr.To("another operation"), LeaseDurationSeconds: ptr.To(int32(1)),
			AcquireTime: &metav1.MicroTime{},
		},
	}, metav1.CreateOptions{})
	require.NoError(t, err)
	require.ErrorContains(t, s.begin(t.Context(), plan, targets, true), "do not steal")
	live, err := kube.CoordinationV1().Leases(plan.Namespace).Get(t.Context(), lease.Name, metav1.GetOptions{})
	require.NoError(t, err)
	require.Equal(t, lease, live)
}

func TestProviderTransitionCancellationAndEvidenceFailure(t *testing.T) {
	t.Parallel()
	s, plan, targets, kube := providerFakeFixture(t, "radius", "external", "enforce")
	require.NoError(t, s.begin(t.Context(), plan, targets, true))
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	count := 0
	kube.PrependReactor("update", "deployments", func(action ktesting.Action) (bool, runtime.Object, error) {
		if action.GetSubresource() == "" {
			count++
			if count == 2 {
				cancel()
			}
		}
		return false, nil, nil
	})
	require.ErrorIs(t, s.step(ctx, plan.Namespace, plan.ID, targets), context.Canceled)
	leases, err := kube.CoordinationV1().Leases(plan.Namespace).List(t.Context(), metav1.ListOptions{})
	require.NoError(t, err)
	require.Empty(t, leases.Items)
	for range []providerTransitionPhase{providerStopping, providerStaging, providerStarting} {
		providerConverge(t, s, plan)
		providerStepConverged(t, s, plan, targets)
	}
	providerConverge(t, s, plan)
	checks := s.checks.(*providerChecksStub)
	checks.verifyErr = errors.New("target authentication failed")
	require.ErrorContains(t, s.step(t.Context(), plan.Namespace, plan.ID, targets), "target authentication failed")
	_, state, err := s.read(t.Context(), plan.Namespace)
	require.NoError(t, err)
	require.Equal(t, providerVerifying, state.Phase)
	checks.verifyErr = nil
	require.NoError(t, s.step(t.Context(), plan.Namespace, plan.ID, targets))
}

func TestProviderTransitionRoundTripAndPredecessor(t *testing.T) {
	t.Parallel()
	s, plan, targets, kube := providerFakeFixture(t, "radius", "external", "dryRun")
	require.NoError(t, s.begin(t.Context(), plan, targets, true))
	for range 4 {
		providerStepConverged(t, s, plan, targets)
	}
	reverseTargets := map[string]appsv1.DeploymentSpec{}
	for name, spec := range targets {
		copy := spec.DeepCopy()
		copy.Template.Spec.Containers[0].Env[0].Value = "radius"
		reverseTargets[name] = *copy
	}
	kube.ClearActions()
	reverse, err := s.plan(t.Context(), plan.Namespace, "external", "radius", "dryRun", reverseTargets)
	require.NoError(t, err)
	providerAssertNoWrites(t, kube)
	require.NotEmpty(t, reverse.PreviousJournalUID)
	require.NotEmpty(t, reverse.PreviousJournalVersion)
	stale := reverse
	stale.PreviousJournalUID = "replaced"
	require.ErrorContains(t, s.begin(t.Context(), stale, reverseTargets, true), "predecessor")
	providerAssertNoWrites(t, kube)
	require.NoError(t, s.begin(t.Context(), reverse, reverseTargets, true))
	_, err = s.plan(t.Context(), plan.Namespace, "external", "radius", "dryRun", reverseTargets)
	require.ErrorContains(t, err, "resume the existing")
	for range 4 {
		providerStepConverged(t, s, reverse, reverseTargets)
	}
	for _, w := range reverse.Workloads {
		d, _, err := s.deployment(t.Context(), reverse, w)
		require.NoError(t, err)
		require.Equal(t, "radius", d.Spec.Template.Spec.Containers[0].Env[0].Value)
		require.Equal(t, w.Replicas, *d.Spec.Replicas)
	}
}

func TestProviderTransitionJournalRejectsCorruption(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		change func(*corev1.ConfigMap)
	}{
		{"owner", func(cm *corev1.ConfigMap) { cm.Labels = nil }},
		{"helm adoption", func(cm *corev1.ConfigMap) {
			cm.Annotations = map[string]string{"meta.helm.sh/release-name": radiusReleaseName}
		}},
		{"extra data", func(cm *corev1.ConfigMap) { cm.Data["extra"] = "value" }},
		{"invalid JSON", func(cm *corev1.ConfigMap) { cm.Data[providerTransitionKey] = "{" }},
		{"oversized", func(cm *corev1.ConfigMap) { cm.Data[providerTransitionKey] = strings.Repeat("x", 65537) }},
		{"trailing JSON", func(cm *corev1.ConfigMap) { cm.Data[providerTransitionKey] += "{}" }},
		{"unknown field", func(cm *corev1.ConfigMap) {
			cm.Data[providerTransitionKey] = strings.Replace(cm.Data[providerTransitionKey], `"version":`, `"unknown":true,"version":`, 1)
		}},
		{"duplicate field", func(cm *corev1.ConfigMap) {
			cm.Data[providerTransitionKey] = strings.Replace(cm.Data[providerTransitionKey], `"version":`, `"version":1,"version":`, 1)
		}},
		{"escaped duplicate", func(cm *corev1.ConfigMap) {
			cm.Data[providerTransitionKey] = strings.Replace(cm.Data[providerTransitionKey], `"phase":`, `"\u0070hase":"stopping","phase":`, 1)
		}},
		{"nested duplicate", func(cm *corev1.ConfigMap) {
			cm.Data[providerTransitionKey] = strings.Replace(cm.Data[providerTransitionKey], `"mode":`, `"mode":"enforce","mode":`, 1)
		}},
		{"null optional array", func(cm *corev1.ConfigMap) {
			cm.Data[providerTransitionKey] = strings.Replace(cm.Data[providerTransitionKey], `"workloads":`, `"configurations":null,"workloads":`, 1)
		}},
		{"null optional scalar", func(cm *corev1.ConfigMap) {
			cm.Data[providerTransitionKey] = strings.Replace(cm.Data[providerTransitionKey], `"replicas":`, `"previousMarker":null,"replicas":`, 1)
		}},
		{"off", func(cm *corev1.ConfigMap) {
			cm.Data[providerTransitionKey] = strings.ReplaceAll(cm.Data[providerTransitionKey], `"enforce"`, `"off"`)
		}},
		{"namespace", func(cm *corev1.ConfigMap) {
			cm.Data[providerTransitionKey] = strings.ReplaceAll(cm.Data[providerTransitionKey], RadiusSystemNamespace, "different")
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, plan, targets, kube := providerFakeFixture(t, "radius", "external", "enforce")
			require.NoError(t, s.begin(t.Context(), plan, targets, true))
			cm, _, err := s.read(t.Context(), plan.Namespace)
			require.NoError(t, err)
			tt.change(cm)
			_, err = kube.CoreV1().ConfigMaps(plan.Namespace).Update(t.Context(), cm, metav1.UpdateOptions{})
			require.NoError(t, err)
			kube.ClearActions()
			require.Error(t, s.step(t.Context(), plan.Namespace, plan.ID, targets))
			providerAssertNoWrites(t, kube)
		})
	}
}

func TestProviderTransitionFailedPhaseCommitResumes(t *testing.T) {
	t.Parallel()
	s, plan, targets, kube := providerFakeFixture(t, "radius", "external", "enforce")
	require.NoError(t, s.begin(t.Context(), plan, targets, true))
	interrupted := false
	kube.PrependReactor("update", "configmaps", func(action ktesting.Action) (bool, runtime.Object, error) {
		if !interrupted {
			interrupted = true
			return true, nil, errors.New("journal write failed")
		}
		return false, nil, nil
	})
	require.ErrorContains(t, s.step(t.Context(), plan.Namespace, plan.ID, targets), "journal write failed")
	_, state, err := s.read(t.Context(), plan.Namespace)
	require.NoError(t, err)
	require.Equal(t, providerStopping, state.Phase)
	for range 4 {
		providerStepConverged(t, s, plan, targets)
	}
}

func TestProviderTransitionJournalDoesNotStoreRenderedCredentials(t *testing.T) {
	t.Parallel()
	s, plan, targets, kube := providerFakeFixture(t, "radius", "external", "enforce")
	spec := targets["ucp"]
	spec.Template.Spec.Containers[0].Env = append(spec.Template.Spec.Containers[0].Env,
		corev1.EnvVar{Name: "EXAMPLE_SECRET", Value: "test-secret-must-not-be-in-journal"})
	targets["ucp"] = spec
	plan, err := s.plan(t.Context(), plan.Namespace, plan.Source, plan.Target, plan.Mode, targets)
	require.NoError(t, err)
	require.NoError(t, s.begin(t.Context(), plan, targets, true))
	cm, err := kube.CoreV1().ConfigMaps(plan.Namespace).Get(t.Context(), providerTransitionJournal, metav1.GetOptions{})
	require.NoError(t, err)
	data, err := json.Marshal(cm)
	require.NoError(t, err)
	require.NotContains(t, string(data), "test-secret-must-not-be-in-journal")
	require.NotContains(t, string(data), "EXAMPLE_SECRET")
}

func TestProviderTransitionResumeRejectsDriftBeforeWrites(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"uid", "metadata", "replicas", "image", "paused", "transaction", "aggregation", "autoscaler"} {
		t.Run(name, func(t *testing.T) {
			s, plan, targets, kube := providerFakeFixture(t, "radius", "external", "enforce")
			require.NoError(t, s.begin(t.Context(), plan, targets, true))
			providerStepConverged(t, s, plan, targets)
			d, err := kube.AppsV1().Deployments(plan.Namespace).Get(t.Context(), "ucp", metav1.GetOptions{})
			require.NoError(t, err)
			transaction := plan.ID
			switch name {
			case "uid":
				d.UID = "replaced"
			case "metadata":
				d.Labels["operator"] = "changed"
			case "replicas":
				d.Spec.Replicas = ptr.To(int32(9))
			case "image":
				d.Spec.Template.Spec.Containers[0].Image = "unapproved"
			case "paused":
				d.Spec.Paused = true
			case "transaction":
				transaction = uuid.NewString()
			case "aggregation":
				s.checks.(*providerChecksStub).snapshot.Revision = uuid.NewString() + "/complete"
			case "autoscaler":
				_, err = kube.AutoscalingV2().HorizontalPodAutoscalers(plan.Namespace).Create(t.Context(), &autoscalingv2.HorizontalPodAutoscaler{
					ObjectMeta: metav1.ObjectMeta{Name: "new-hpa"},
					Spec: autoscalingv2.HorizontalPodAutoscalerSpec{ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
						APIVersion: "apps/v1", Kind: "Deployment", Name: "ucp",
					}},
				}, metav1.CreateOptions{})
				require.NoError(t, err)
			}
			_, err = kube.AppsV1().Deployments(plan.Namespace).Update(t.Context(), d, metav1.UpdateOptions{})
			require.NoError(t, err)
			kube.ClearActions()
			require.Error(t, s.step(t.Context(), plan.Namespace, transaction, targets))
			providerAssertNoWrites(t, kube)
		})
	}
}

func TestProviderTransitionDoesNotStartWithReappearingOldPod(t *testing.T) {
	t.Parallel()
	s, plan, targets, kube := providerFakeFixture(t, "radius", "external", "enforce")
	require.NoError(t, s.begin(t.Context(), plan, targets, true))
	providerStepConverged(t, s, plan, targets)
	providerStepConverged(t, s, plan, targets)
	_, err := kube.CoreV1().Pods(plan.Namespace).Create(t.Context(), &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "unexpected", Labels: map[string]string{"app": "ucp"}},
	}, metav1.CreateOptions{})
	require.NoError(t, err)
	require.ErrorContains(t, s.step(t.Context(), plan.Namespace, plan.ID, targets), "old or unowned provider pod")
	for _, w := range plan.Workloads {
		d, _, err := s.deployment(t.Context(), plan, w)
		require.NoError(t, err)
		require.Zero(t, *d.Spec.Replicas)
	}
}

func TestProviderTransitionCancellationBeforeMutation(t *testing.T) {
	t.Parallel()
	s, plan, targets, kube := providerFakeFixture(t, "radius", "external", "enforce")
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	require.ErrorIs(t, s.begin(ctx, plan, targets, true), context.Canceled)
	providerAssertNoWrites(t, kube)
}

func TestProviderTransitionCancellationAfterLastWorkload(t *testing.T) {
	t.Parallel()
	s, plan, targets, kube := providerFakeFixture(t, "radius", "external", "enforce")
	require.NoError(t, s.begin(t.Context(), plan, targets, true))
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	count := 0
	kube.PrependReactor("update", "deployments", func(action ktesting.Action) (bool, runtime.Object, error) {
		if action.GetSubresource() == "" {
			count++
			if count == len(rotationComponents) {
				cancel()
			}
		}
		return false, nil, nil
	})
	require.ErrorIs(t, s.step(ctx, plan.Namespace, plan.ID, targets), context.Canceled)
	_, state, err := s.read(t.Context(), plan.Namespace)
	require.NoError(t, err)
	require.Equal(t, providerStopping, state.Phase)
	providerStepConverged(t, s, plan, targets)
}

func TestProviderTransitionPinsConfiguration(t *testing.T) {
	t.Parallel()
	for _, drift := range []string{"none", "source", "target"} {
		t.Run(drift, func(t *testing.T) {
			s, plan, targets, kube := providerFakeFixture(t, "radius", "external", "enforce")
			for _, name := range []string{"source-config", "target-config"} {
				_, err := kube.CoreV1().ConfigMaps(plan.Namespace).Create(t.Context(), &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{Name: name},
					Data:       map[string]string{"config": name + "-original"},
				}, metav1.CreateOptions{})
				require.NoError(t, err)
			}
			d, err := kube.AppsV1().Deployments(plan.Namespace).Get(t.Context(), "ucp", metav1.GetOptions{})
			require.NoError(t, err)
			d.Spec.Template.Spec.Containers[0].EnvFrom = []corev1.EnvFromSource{{
				ConfigMapRef: &corev1.ConfigMapEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: "source-config"}},
			}}
			_, err = kube.AppsV1().Deployments(plan.Namespace).Update(t.Context(), d, metav1.UpdateOptions{})
			require.NoError(t, err)
			spec := targets["ucp"]
			spec.Template.Spec.Containers[0].EnvFrom = []corev1.EnvFromSource{{
				ConfigMapRef: &corev1.ConfigMapEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: "target-config"}},
			}}
			targets["ucp"] = spec
			plan, err = s.plan(t.Context(), plan.Namespace, plan.Source, plan.Target, plan.Mode, targets)
			require.NoError(t, err)
			require.Len(t, plan.Configurations, 2)
			require.NoError(t, s.begin(t.Context(), plan, targets, true))
			providerStepConverged(t, s, plan, targets)
			if drift != "none" {
				name := drift + "-config"
				config, err := kube.CoreV1().ConfigMaps(plan.Namespace).Get(t.Context(), name, metav1.GetOptions{})
				require.NoError(t, err)
				config.Data["config"] = "changed"
				_, err = kube.CoreV1().ConfigMaps(plan.Namespace).Update(t.Context(), config, metav1.UpdateOptions{})
				require.NoError(t, err)
				kube.ClearActions()
				require.ErrorContains(t, s.step(t.Context(), plan.Namespace, plan.ID, targets), "provider configuration")
				providerAssertNoWrites(t, kube)
				return
			}
			kube.ClearActions()
			for range 3 {
				providerStepConverged(t, s, plan, targets)
			}
			for _, action := range kube.Actions() {
				if action.GetVerb() == "update" && action.GetResource().Resource == "configmaps" {
					require.Equal(t, providerTransitionJournal, action.(ktesting.UpdateAction).GetObject().(*corev1.ConfigMap).Name)
				}
			}
		})
	}
}

func TestProviderTransitionConfigurationReferences(t *testing.T) {
	t.Parallel()
	spec := corev1.PodSpec{
		Volumes: []corev1.Volume{
			{VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: "volume"}}}},
			{VolumeSource: corev1.VolumeSource{Projected: &corev1.ProjectedVolumeSource{Sources: []corev1.VolumeProjection{
				{ConfigMap: &corev1.ConfigMapProjection{LocalObjectReference: corev1.LocalObjectReference{Name: "projected"}}},
			}}}},
		},
		Containers: []corev1.Container{{
			Env: []corev1.EnvVar{{ValueFrom: &corev1.EnvVarSource{ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: "key"}, Key: "example",
			}}}},
		}},
		InitContainers: []corev1.Container{{
			EnvFrom: []corev1.EnvFromSource{{ConfigMapRef: &corev1.ConfigMapEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{Name: "init"},
			}}},
		}},
	}
	require.Equal(t, []string{"init", "key", "projected", "volume"}, providerConfigurationNames(spec))
}

func TestProviderTransitionRechecksPopulationAfterEvidence(t *testing.T) {
	t.Parallel()
	for _, phase := range []providerTransitionPhase{providerVerifying, providerComplete} {
		t.Run(string(phase), func(t *testing.T) {
			s, plan, targets, kube := providerFakeFixture(t, "radius", "external", "enforce")
			require.NoError(t, s.begin(t.Context(), plan, targets, true))
			for {
				_, state, err := s.read(t.Context(), plan.Namespace)
				require.NoError(t, err)
				if state.Phase == phase {
					break
				}
				providerStepConverged(t, s, plan, targets)
			}
			providerConverge(t, s, plan)
			s.checks.(*providerChecksStub).verifyHook = func() {
				_, err := kube.CoreV1().Pods(plan.Namespace).Create(t.Context(), &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{Name: "unexpected-during-evidence", Labels: map[string]string{"app": "ucp"}},
				}, metav1.CreateOptions{})
				require.NoError(t, err)
			}
			require.ErrorContains(t, s.step(t.Context(), plan.Namespace, plan.ID, targets), "old or unowned provider pod")
			_, state, err := s.read(t.Context(), plan.Namespace)
			require.NoError(t, err)
			require.Equal(t, phase, state.Phase)
		})
	}
}

func TestProviderTransitionRechecksMutableStateAfterEvidence(t *testing.T) {
	t.Parallel()
	for _, phase := range []providerTransitionPhase{providerVerifying, providerComplete} {
		for _, change := range []string{"configuration", "readiness", "autoscaler"} {
			t.Run(string(phase)+"/"+change, func(t *testing.T) {
				s, plan, targets, kube := providerFakeFixture(t, "radius", "external", "enforce")
				cm, err := kube.CoreV1().ConfigMaps(plan.Namespace).Create(t.Context(), &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{Name: "pinned"},
					Data:       map[string]string{"config": "original"},
				}, metav1.CreateOptions{})
				require.NoError(t, err)
				pin, err := s.configuration(t.Context(), plan.Namespace, cm.Name)
				require.NoError(t, err)
				plan.Configurations = []providerTransitionConfig{pin}
				require.NoError(t, s.begin(t.Context(), plan, targets, true))
				for {
					_, state, err := s.read(t.Context(), plan.Namespace)
					require.NoError(t, err)
					if state.Phase == phase {
						break
					}
					providerStepConverged(t, s, plan, targets)
				}
				providerConverge(t, s, plan)
				s.checks.(*providerChecksStub).verifyHook = func() {
					switch change {
					case "configuration":
						cm.Data["config"] = "changed"
						_, err := kube.CoreV1().ConfigMaps(plan.Namespace).Update(t.Context(), cm, metav1.UpdateOptions{})
						require.NoError(t, err)
					case "readiness":
						d, err := kube.AppsV1().Deployments(plan.Namespace).Get(t.Context(), plan.Workloads[1].Name, metav1.GetOptions{})
						require.NoError(t, err)
						d.Status.ReadyReplicas = 0
						_, err = kube.AppsV1().Deployments(plan.Namespace).UpdateStatus(t.Context(), d, metav1.UpdateOptions{})
						require.NoError(t, err)
					case "autoscaler":
						_, err := kube.AutoscalingV2().HorizontalPodAutoscalers(plan.Namespace).Create(t.Context(), &autoscalingv2.HorizontalPodAutoscaler{
							ObjectMeta: metav1.ObjectMeta{Name: "new-hpa"},
							Spec: autoscalingv2.HorizontalPodAutoscalerSpec{ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
								APIVersion: "apps/v1", Kind: "Deployment", Name: "ucp",
							}},
						}, metav1.CreateOptions{})
						require.NoError(t, err)
					}
				}

				require.Error(t, s.step(t.Context(), plan.Namespace, plan.ID, targets))
				_, state, err := s.read(t.Context(), plan.Namespace)
				require.NoError(t, err)
				require.Equal(t, phase, state.Phase)
			})
		}
	}
}

func TestProviderTransitionCompletedJournalAdapter(t *testing.T) {
	t.Parallel()
	s, plan, targets, _ := providerFakeFixture(t, "radius", "external", "enforce")
	require.NoError(t, s.begin(t.Context(), plan, targets, true))
	for range 4 {
		providerStepConverged(t, s, plan, targets)
	}
	completed, _, err := s.read(t.Context(), plan.Namespace)
	require.NoError(t, err)
	require.NoError(t, validateCompletedProviderTransitionJournal(completed, plan.Namespace, plan.Release))
	tests := []struct {
		name   string
		change func(*corev1.ConfigMap)
	}{
		{"wrong name", func(cm *corev1.ConfigMap) { cm.Name = "unrelated" }},
		{"wrong namespace", func(cm *corev1.ConfigMap) { cm.Namespace = "other" }},
		{"missing uid", func(cm *corev1.ConfigMap) { cm.UID = "" }},
		{"missing version", func(cm *corev1.ConfigMap) { cm.ResourceVersion = "" }},
		{"wrong owner", func(cm *corev1.ConfigMap) { cm.Labels[providerTransitionMarker] = "other" }},
		{"deleting", func(cm *corev1.ConfigMap) { cm.DeletionTimestamp = ptr.To(metav1.Now()) }},
		{"phase alone", func(cm *corev1.ConfigMap) { cm.Data[providerTransitionKey] = `{"version":1,"phase":"complete"}` }},
		{"unknown schema", func(cm *corev1.ConfigMap) {
			cm.Data[providerTransitionKey] = strings.ReplaceAll(cm.Data[providerTransitionKey], `"version":1`, `"version":2`)
		}},
		{"duplicate phase", func(cm *corev1.ConfigMap) {
			cm.Data[providerTransitionKey] = strings.ReplaceAll(cm.Data[providerTransitionKey], `"phase":"complete"`, `"phase":"stopping","phase":"complete"`)
		}},
		{"invalid workload", func(cm *corev1.ConfigMap) {
			cm.Data[providerTransitionKey] = strings.ReplaceAll(cm.Data[providerTransitionKey], `"name":"ucp"`, `"name":"unrelated"`)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cm := completed.DeepCopy()
			tt.change(cm)
			before := cm.DeepCopy()
			require.Error(t, validateCompletedProviderTransitionJournal(cm, plan.Namespace, plan.Release))
			require.Equal(t, before, cm, "the validator must be read-only")
		})
	}
	for _, phase := range []providerTransitionPhase{providerStopping, providerStaging, providerStarting, providerVerifying} {
		t.Run(string(phase), func(t *testing.T) {
			cm := completed.DeepCopy()
			cm.Data[providerTransitionKey] = strings.ReplaceAll(cm.Data[providerTransitionKey], `"phase":"complete"`, `"phase":"`+string(phase)+`"`)
			require.ErrorContains(t, validateCompletedProviderTransitionJournal(cm, plan.Namespace, plan.Release), "unfinished")
		})
	}
	require.Error(t, validateCompletedProviderTransitionJournal(nil, plan.Namespace, plan.Release))
	require.Error(t, validateCompletedProviderTransitionJournal(completed, "other", plan.Release))
	require.Error(t, validateCompletedProviderTransitionJournal(completed, plan.Namespace, "other"))
}

func TestProviderTransitionSharedCAGuard(t *testing.T) {
	t.Parallel()
	s, plan, targets, _ := providerFakeFixture(t, "radius", "external", "enforce")
	impl := &Impl{}
	check := func(wantComplete bool) {
		t.Helper()
		require.NoError(t, withIdentityOperation(t.Context(), s.kube, plan.Namespace, func(operation *identityOperation) error {
			err := operation.checkProviderJournal(t.Context(), plan.Release,
				impl.completedProviderJournalValidator(plan.Namespace, plan.Release))
			if wantComplete {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
			require.Error(t, s.withLock(t.Context(), plan.Namespace, func(*coordinationv1.Lease) error {
				t.Fatal("provider transition must not enter an active CA operation")
				return nil
			}))
			return nil
		}))
	}
	check(true)
	require.NoError(t, s.begin(t.Context(), plan, targets, true))
	for range 4 {
		check(false)
		providerStepConverged(t, s, plan, targets)
	}
	check(true)
	completed, _, err := s.read(t.Context(), plan.Namespace)
	require.NoError(t, err)
	called := false
	impl.validateCompletedProviderJournal = func(*corev1.ConfigMap) error {
		called = true
		return nil
	}
	require.NoError(t, impl.completedProviderJournalValidator(plan.Namespace, plan.Release)(completed))
	require.True(t, called)
	called = false
	require.Error(t, impl.completedProviderJournalValidator("other", plan.Release)(completed))
	require.False(t, called, "an optional hook cannot bypass namespace or full-schema validation")
}
