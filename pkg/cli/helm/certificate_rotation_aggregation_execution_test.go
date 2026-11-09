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
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/radius-project/radius/pkg/authz/mtls"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	kubefake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
	k8stesting "k8s.io/client-go/testing"
)

func aggregationExecutionFixture(t *testing.T, aggregationOnly bool) (*rotationRunner, rotationState) {
	t.Helper()
	runner, state := rotationRunnerFixture(t)
	workload, err := state.Plan.aggregationWorkload()
	require.NoError(t, err)
	state.SchemaVersion, state.Plan.Scope, state.Plan.Aggregation = 2, rotationComponentsAndAggregation, &workload
	if aggregationOnly {
		state = aggregationOnlyState(t, state)
	}
	s, ctx := runner.store, t.Context()
	kube := s.kube.(*kubefake.Clientset)
	deployment, err := kube.AppsV1().Deployments(s.namespace).Get(ctx, "ucp", metav1.GetOptions{})
	require.NoError(t, err)
	deployment.Spec.Template.Spec.Containers[0].Ports = []corev1.ContainerPort{{Name: "metrics", ContainerPort: 9090}}
	require.NoError(t, kube.Tracker().Update(appsv1.SchemeGroupVersion.WithResource("deployments"), deployment, s.namespace))
	storeAggregationExecutionState(t, s, state)
	runner.observeAggregation = func(ctx context.Context, _ *rest.Config, target TLSProofTarget) (PodAggregationProof, error) {
		pod, err := kube.CoreV1().Pods(s.namespace).Get(ctx, target.PodName, metav1.GetOptions{})
		if err != nil {
			return PodAggregationProof{}, err
		}
		var projection *corev1.ProjectedVolumeSource
		for _, volume := range pod.Spec.Volumes {
			if volume.Name == "cert" {
				projection = volume.Projected
			}
		}
		require.NotNil(t, projection)
		secret, err := kube.CoreV1().Secrets(s.namespace).Get(ctx, projection.Sources[0].Secret.Name, metav1.GetOptions{})
		if err != nil {
			return PodAggregationProof{}, err
		}
		trust, err := kube.CoreV1().ConfigMaps(s.namespace).Get(ctx, projection.Sources[1].ConfigMap.Name, metav1.GetOptions{})
		if err != nil {
			return PodAggregationProof{}, err
		}
		// The simulated kubelet projects real bytes; evidence comes from the
		// serving Source, never from the observer's expected digest fields.
		directory := t.TempDir()
		for name, value := range map[string][]byte{
			"tls.crt": secret.Data["tls.crt"], "tls.key": secret.Data["tls.key"], "ca.crt": []byte(trust.Data["ca.crt"]),
		} {
			require.NoError(t, os.WriteFile(filepath.Join(directory, name), value, 0600))
		}
		source, err := mtls.NewSource(directory)
		if err != nil {
			return PodAggregationProof{}, err
		}
		proof, err := mtls.NewAggregationProof(source, s.namespace, string(pod.UID), target.TrustBundleSHA256)
		if err != nil {
			return PodAggregationProof{}, err
		}
		evidence, err := proof.Snapshot()
		return PodAggregationProof{Evidence: evidence}, err
	}
	return runner, state
}

func storeAggregationExecutionState(t *testing.T, s *rotationStore, state rotationState) {
	t.Helper()
	require.NoError(t, state.validate(time.Now()))
	journal, _, err := s.read(t.Context())
	require.NoError(t, err)
	data, err := json.Marshal(state)
	require.NoError(t, err)
	journal.Data[rotationJournalKey] = string(data)
	_, err = s.kube.CoreV1().ConfigMaps(s.namespace).Update(t.Context(), journal, metav1.UpdateOptions{})
	require.NoError(t, err)
}

func TestRotationAggregationOnlyPhases(t *testing.T) {
	runner, state := aggregationExecutionFixture(t, true)
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	runner.observe = func(context.Context, *rest.Config, TLSProofTarget) (PodTLSProof, error) {
		t.Fatal("aggregation-only phases must never request component evidence")
		return PodTLSProof{}, nil
	}
	for _, phase := range []rotationPhase{rotationPublish, rotationAdopt, rotationIssue, rotationRetire} {
		require.NoError(t, runner.run(ctx, state.ID, phase), phase)
		require.NoError(t, runner.run(ctx, state.ID, phase), "phase retry")
	}
	_, complete, err := runner.store.read(ctx)
	require.NoError(t, err)
	require.Equal(t, rotationComplete, complete.Phase)
	require.Empty(t, complete.Plan.Consumers)
	require.NoError(t, runner.verifyAggregation(ctx, complete))
}

func TestRotationAggregationAdoptionPrecedesComponentIssuance(t *testing.T) {
	runner, state := aggregationExecutionFixture(t, false)
	s := runner.store
	require.NoError(t, runner.run(t.Context(), state.ID, rotationPublish))
	probe := runner.probe
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	runner.probe = func(context.Context, *rest.Config, map[types.UID]bool) error {
		cancel()
		return errors.New("API server has not loaded candidate trust")
	}
	require.ErrorContains(t, runner.run(ctx, state.ID, rotationAdopt), "interrupted")
	_, current, err := s.read(t.Context())
	require.NoError(t, err)
	require.Equal(t, rotationAdopt, current.Phase)
	for _, workload := range state.Plan.Consumers {
		certificate, err := s.dynamic.Resource(rotationCertificateResource).Namespace(s.namespace).Get(t.Context(), workload.Name+"-cert", metav1.GetOptions{})
		require.NoError(t, err)
		issuer, _, err := unstructured.NestedString(certificate.Object, "spec", "issuerRef", "name")
		require.NoError(t, err)
		require.Equal(t, workload.OldIssuer, issuer)
	}
	require.ErrorContains(t, runner.run(t.Context(), state.ID, rotationIssue), "complete it before")
	runner.probe = probe
	resumed, stop := context.WithTimeout(t.Context(), 15*time.Second)
	defer stop()
	require.NoError(t, runner.run(resumed, state.ID, rotationAdopt))
	require.NoError(t, runner.run(resumed, state.ID, rotationIssue))
}

func TestRotationAggregationOnlyRecovery(t *testing.T) {
	for _, held := range []bool{false, true} {
		t.Run(map[bool]string{false: "before issuance", true: "held candidate"}[held], func(t *testing.T) {
			runner, original := aggregationExecutionFixture(t, true)
			if held {
				runner, original = heldAggregationExecutionFixture(t)
			}
			s := runner.store
			journal, _, err := s.read(t.Context())
			require.NoError(t, err)
			id := uuid.NewString()
			plan := *original.Plan
			plan.Issuer = "radius-ca-" + strings.ReplaceAll(id, "-", "")[:16]
			plan.ServingSecret = "ucp-cert-aggregation-" + strings.ReplaceAll(id, "-", "")[:16]
			recovery, err := s.beginRecovery(t.Context(), id, original.NewCA, &plan, journal)
			require.NoError(t, err)
			runner.observe = func(context.Context, *rest.Config, TLSProofTarget) (PodTLSProof, error) {
				t.Fatal("aggregation-only recovery must not request component evidence")
				return PodTLSProof{}, nil
			}
			ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
			defer cancel()
			require.NoError(t, runner.run(ctx, recovery.ID, rotationRecover))
			require.NoError(t, runner.run(ctx, recovery.ID, rotationRecover))
			_, completed, err := s.read(ctx)
			require.NoError(t, err)
			require.True(t, completed.Recovery)
			require.Equal(t, rotationComplete, completed.Phase)
			require.Empty(t, completed.Plan.Consumers)
			require.NoError(t, runner.verifyAggregation(ctx, completed))
			require.Error(t, runner.run(ctx, original.ID, rotationIssue))
		})
	}
}

func TestRotationAggregationSignerDriftAfterEvidence(t *testing.T) {
	runner, state := aggregationExecutionFixture(t, true)
	require.NoError(t, runner.run(t.Context(), state.ID, rotationPublish))
	observe := runner.observeAggregation
	changed := false
	runner.observeAggregation = func(ctx context.Context, config *rest.Config, target TLSProofTarget) (PodAggregationProof, error) {
		proof, err := observe(ctx, config, target)
		if !changed && err == nil {
			secret, err := runner.store.kube.CoreV1().Secrets(runner.store.namespace).Get(ctx, state.Plan.SigningSecret, metav1.GetOptions{})
			require.NoError(t, err)
			secret.Data["tls.key"] = []byte("replaced signing material")
			_, err = runner.store.kube.CoreV1().Secrets(runner.store.namespace).Update(ctx, secret, metav1.UpdateOptions{})
			require.NoError(t, err)
			changed = true
		}
		return proof, err
	}
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	require.Error(t, runner.run(ctx, state.ID, rotationAdopt))
	require.True(t, changed)
	_, current, err := runner.store.read(t.Context())
	require.NoError(t, err)
	require.Equal(t, rotationAdopt, current.Phase)
}

func TestRotationManagedSeparateServingEvidence(t *testing.T) {
	runner, state := aggregationExecutionFixture(t, false)
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	observations := 0
	observe := runner.observeAggregation
	runner.observeAggregation = func(ctx context.Context, config *rest.Config, target TLSProofTarget) (PodAggregationProof, error) {
		observations++
		return observe(ctx, config, target)
	}
	for _, phase := range []rotationPhase{rotationPublish, rotationAdopt, rotationIssue, rotationRetire} {
		require.NoError(t, runner.run(ctx, state.ID, phase), phase)
	}
	require.Positive(t, observations)
	_, completed, err := runner.store.read(ctx)
	require.NoError(t, err)
	require.Equal(t, rotationComplete, completed.Phase)
	require.NoError(t, runner.verifyAggregation(ctx, completed))
}

func heldAggregationExecutionFixture(t *testing.T) (*rotationRunner, rotationState) {
	t.Helper()
	runner, state := aggregationExecutionFixture(t, true)
	s, ctx := runner.store, t.Context()
	state.Phase = rotationIssue
	storeAggregationExecutionState(t, s, state)
	require.NoError(t, s.reconcileTrust(ctx, state.ID))
	require.NoError(t, s.ensureRotationIssuer(ctx, state.ID))
	require.NoError(t, s.ensureAggregationCertificate(ctx, state.ID))
	_, hash, err := s.aggregationPods(ctx, state, false)
	require.NoError(t, err)
	_, err = s.setAggregationRoute(ctx, state.ID, "old", hash, nil)
	require.NoError(t, err)
	require.NoError(t, s.reconcileAggregationDeployment(ctx, state.ID))
	return runner, state
}

func TestAggregationIndependentServingRollout(t *testing.T) {
	runner, state := heldAggregationExecutionFixture(t)
	s, ctx := runner.store, t.Context()
	kube := s.kube.(*kubefake.Clientset)
	kube.ClearActions()
	runner.observe = func(context.Context, *rest.Config, TLSProofTarget) (PodTLSProof, error) {
		t.Fatal("aggregation rollout must not use component evidence")
		return PodTLSProof{}, nil
	}
	ids, hash, err := runner.observeAggregationCandidates(ctx, state)
	require.NoError(t, err)
	require.Len(t, ids, 1)
	require.Error(t, s.verifyAggregationServing(ctx, nil, state, runner.observeAggregation),
		"the final convergence barrier must reject the retained old backend")
	deployment, err := kube.AppsV1().Deployments(s.namespace).Get(ctx, "ucp", metav1.GetOptions{})
	require.NoError(t, err)
	require.Equal(t, int32(math.MaxInt32-1), deployment.Spec.MinReadySeconds)
	componentSpec := deployment.Spec.Template.Spec.DeepCopy()
	routed, err := s.setAggregationRoute(ctx, state.ID, "new", hash, nil)
	require.NoError(t, err)
	require.NoError(t, runner.probe(ctx, nil, ids))
	_, err = s.setAggregationRoute(ctx, state.ID, "complete", "", routed)
	require.NoError(t, err)
	require.NoError(t, s.reconcileAggregationDeployment(ctx, state.ID))
	require.NoError(t, runner.verifyAggregation(ctx, state))
	deployment, err = kube.AppsV1().Deployments(s.namespace).Get(ctx, "ucp", metav1.GetOptions{})
	require.NoError(t, err)
	require.Equal(t, state.Plan.UCPMinReadySeconds, deployment.Spec.MinReadySeconds)
	require.Equal(t, componentSpec, &deployment.Spec.Template.Spec, "releasing the hold preserves pod configuration")
	for _, action := range kube.Actions() {
		if action.GetVerb() == "get" && action.GetResource().Resource == "secrets" {
			require.Equal(t, state.Plan.ServingSecret, action.(k8stesting.GetAction).GetName())
		}
		if action.GetVerb() == "update" {
			require.Contains(t, []string{"services", "deployments"}, action.GetResource().Resource)
		}
	}
}

func TestAggregationServingCandidateFences(t *testing.T) {
	for _, scenario := range []string{"component evidence", "wrong serving leaf", "pod changes", "unowned pod", "unlabelled consumer", "observer error"} {
		t.Run(scenario, func(t *testing.T) {
			runner, state := heldAggregationExecutionFixture(t)
			s, ctx := runner.store, t.Context()
			observe := runner.observeAggregation
			runner.observeAggregation = func(ctx context.Context, config *rest.Config, target TLSProofTarget) (PodAggregationProof, error) {
				proof, err := observe(ctx, config, target)
				require.NoError(t, err)
				switch scenario {
				case "component evidence":
					proof.Evidence.ProofKind = "startup-tls-initialized"
				case "wrong serving leaf":
					proof.Evidence.LeafSHA256 = state.Plan.TargetTrustSHA256
				case "observer error":
					return PodAggregationProof{}, errors.New("serving Source unavailable")
				default:
					pod, err := s.kube.CoreV1().Pods(s.namespace).Get(ctx, target.PodName, metav1.GetOptions{})
					require.NoError(t, err)
					if scenario == "pod changes" {
						pod.Annotations["changed"] = "during proof"
						_, err = s.kube.CoreV1().Pods(s.namespace).Update(ctx, pod, metav1.UpdateOptions{})
					} else {
						pod.Name, pod.UID, pod.OwnerReferences = "unowned-serving", types.UID(uuid.NewString()), nil
						if scenario == "unlabelled consumer" {
							pod.Labels = nil
						}
						_, err = s.kube.CoreV1().Pods(s.namespace).Create(ctx, pod, metav1.CreateOptions{})
					}
					require.NoError(t, err)
				}
				return proof, nil
			}
			_, _, err := runner.observeAggregationCandidates(ctx, state)
			require.Error(t, err)
		})
	}
}

func TestAggregationServingRolloutResume(t *testing.T) {
	for _, scenario := range []string{"failed probe", "completed route with held deployment"} {
		t.Run(scenario, func(t *testing.T) {
			runner, state := heldAggregationExecutionFixture(t)
			s, ctx := runner.store, t.Context()
			probe := runner.probe
			if scenario == "failed probe" {
				attempt, cancel := context.WithCancel(ctx)
				defer cancel()
				runner.probe = func(context.Context, *rest.Config, map[types.UID]bool) error {
					cancel()
					return errors.New("aggregator has not adopted target trust")
				}
				require.ErrorContains(t, runner.migrateAggregation(attempt, state), "interrupted")
				service, err := s.kube.CoreV1().Services(s.namespace).Get(ctx, "ucp", metav1.GetOptions{})
				require.NoError(t, err)
				require.Equal(t, "old", service.Annotations[rotationRoute])
				require.Equal(t, service.Annotations[rotationOldHash], service.Spec.Selector[appsv1.DefaultDeploymentUniqueLabelKey])
				runner.probe = probe
			} else {
				_, hash, err := runner.observeAggregationCandidates(ctx, state)
				require.NoError(t, err)
				routed, err := s.setAggregationRoute(ctx, state.ID, "new", hash, nil)
				require.NoError(t, err)
				_, err = s.setAggregationRoute(ctx, state.ID, "complete", "", routed)
				require.NoError(t, err)
			}
			require.NoError(t, runner.migrateAggregation(ctx, state))
			require.NoError(t, runner.verifyAggregation(ctx, state))
			deployment, err := s.kube.AppsV1().Deployments(s.namespace).Get(ctx, "ucp", metav1.GetOptions{})
			require.NoError(t, err)
			require.Equal(t, state.Plan.UCPMinReadySeconds, deployment.Spec.MinReadySeconds)
		})
	}
}

func TestAggregationCandidateDriftDoesNotReleaseHold(t *testing.T) {
	for _, completedRoute := range []bool{false, true} {
		t.Run(map[bool]string{false: "candidate route", true: "completed route"}[completedRoute], func(t *testing.T) {
			runner, state := heldAggregationExecutionFixture(t)
			s := runner.store
			if completedRoute {
				_, hash, err := runner.observeAggregationCandidates(t.Context(), state)
				require.NoError(t, err)
				routed, err := s.setAggregationRoute(t.Context(), state.ID, "new", hash, nil)
				require.NoError(t, err)
				_, err = s.setAggregationRoute(t.Context(), state.ID, "complete", "", routed)
				require.NoError(t, err)
			}
			probe := runner.probe
			runner.probe = func(ctx context.Context, config *rest.Config, ids map[types.UID]bool) error {
				require.NoError(t, probe(ctx, config, ids))
				pods, err := s.kube.CoreV1().Pods(s.namespace).List(ctx, metav1.ListOptions{LabelSelector: "app.kubernetes.io/name=ucp"})
				require.NoError(t, err)
				for _, item := range pods.Items {
					if !ids[item.UID] {
						continue
					}
					pod := item.DeepCopy()
					pod.Name, pod.UID, pod.OwnerReferences = "unowned-after-probe", types.UID(uuid.NewString()), nil
					_, err = s.kube.CoreV1().Pods(s.namespace).Create(ctx, pod, metav1.CreateOptions{})
					require.NoError(t, err)
				}
				return nil
			}
			ctx, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()
			require.Error(t, runner.migrateAggregation(ctx, state))
			deployment, err := s.kube.AppsV1().Deployments(s.namespace).Get(t.Context(), "ucp", metav1.GetOptions{})
			require.NoError(t, err)
			require.Equal(t, int32(math.MaxInt32-1), deployment.Spec.MinReadySeconds)
			service, err := s.kube.CoreV1().Services(s.namespace).Get(t.Context(), "ucp", metav1.GetOptions{})
			require.NoError(t, err)
			require.Equal(t, map[bool]string{false: "old", true: "complete"}[completedRoute], service.Annotations[rotationRoute])
		})
	}
}

func TestAggregationFinalEvidenceRecheckedAfterProbe(t *testing.T) {
	for _, scenario := range []string{"unowned pod", "pod metadata", "APIService metadata"} {
		t.Run(scenario, func(t *testing.T) {
			runner, state := heldAggregationExecutionFixture(t)
			s, ctx := runner.store, t.Context()
			require.NoError(t, runner.migrateAggregation(ctx, state))
			probe := runner.probe
			runner.probe = func(ctx context.Context, config *rest.Config, ids map[types.UID]bool) error {
				require.NoError(t, probe(ctx, config, ids))
				if scenario == "APIService metadata" {
					api, err := s.dynamic.Resource(apiServiceResource).Get(ctx, radiusAPIService, metav1.GetOptions{})
					require.NoError(t, err)
					annotations := api.GetAnnotations()
					annotations["changed"] = "during probe"
					api.SetAnnotations(annotations)
					_, err = s.dynamic.Resource(apiServiceResource).Update(ctx, api, metav1.UpdateOptions{})
					require.NoError(t, err)
					return nil
				}

				pods, err := s.kube.CoreV1().Pods(s.namespace).List(ctx, metav1.ListOptions{LabelSelector: "app.kubernetes.io/name=ucp"})
				require.NoError(t, err)
				require.Len(t, pods.Items, 1)
				pod := pods.Items[0].DeepCopy()
				if scenario == "pod metadata" {
					pod.Annotations["changed"] = "during probe"
					_, err = s.kube.CoreV1().Pods(s.namespace).Update(ctx, pod, metav1.UpdateOptions{})
				} else {
					pod.Name, pod.UID, pod.OwnerReferences = "unowned-after-probe", types.UID(uuid.NewString()), nil
					_, err = s.kube.CoreV1().Pods(s.namespace).Create(ctx, pod, metav1.CreateOptions{})
				}
				require.NoError(t, err)
				return nil
			}
			require.Error(t, runner.verifyAggregation(ctx, state))
		})
	}
}
