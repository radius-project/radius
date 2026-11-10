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
	"fmt"
	"maps"
	"strings"
	"time"

	"github.com/radius-project/radius/pkg/authz/mtls"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
)

func aggregationProofPort(spec corev1.PodSpec) (uint16, error) {
	for _, container := range spec.Containers {
		if container.Name == "ucp" {
			for _, port := range container.Ports {
				if port.Name == "metrics" && port.ContainerPort > 0 && port.ContainerPort <= 65535 &&
					(port.Protocol == "" || port.Protocol == corev1.ProtocolTCP) {
					return uint16(port.ContainerPort), nil
				}
			}
		}
	}
	return 0, errors.New("aggregation serving evidence requires UCP's declared management listener")
}

func rotationUsesAggregationSecret(spec corev1.PodSpec) bool {
	matches := func(name string) bool {
		return name == "ucp-cert" || strings.HasPrefix(name, "radius-aggregation-") || strings.HasPrefix(name, "ucp-cert-aggregation-")
	}
	for _, volume := range spec.Volumes {
		if volume.Secret != nil && matches(volume.Secret.SecretName) {
			return true
		}
		if volume.Projected != nil {
			for _, projection := range volume.Projected.Sources {
				if projection.Secret != nil && matches(projection.Secret.Name) {
					return true
				}
			}
		}
	}
	return false
}

func rotationAggregationProjection(spec corev1.PodSpec, plan *rotationPlan) error {
	projected, mounted := false, false
	expected := rotationTLSProjection("cert", plan.ServingSecret, plan.TargetTrustMap)
	for _, volume := range spec.Volumes {
		if volume.Name == "cert" && volume.Projected != nil &&
			apiequality.Semantic.DeepEqual(volume.Projected.Sources, expected.Projected.Sources) {
			projected = true
		}
	}
	for _, container := range spec.Containers {
		if container.Name == "ucp" {
			for _, mount := range container.VolumeMounts {
				if mount.Name == "cert" && mount.MountPath == "/var/tls/cert" && mount.ReadOnly &&
					mount.SubPath == "" && mount.SubPathExpr == "" {
					mounted = true
				}
			}
		}
	}
	if !projected || !mounted {
		return errors.New("aggregation serving identity and immutable trust projection differ from the plan")
	}
	return nil
}

// aggregationInventory is the final convergence barrier, not a candidate-surge
// inventory. It inspects only aggregation PKI, without inventing component proof.
func (s *rotationStore) aggregationInventory(ctx context.Context, state rotationState) (rotationInventory, error) {
	return s.aggregationServingInventory(ctx, state, false)
}

func (s *rotationStore) aggregationServingInventory(ctx context.Context, state rotationState, candidate bool) (rotationInventory, error) {
	inventory := rotationInventory{versions: map[types.UID]string{}}
	if state.Plan == nil {
		return inventory, errors.New("aggregation inventory requires a resource plan")
	}
	workload, err := state.Plan.aggregationWorkload()
	if err != nil {
		return inventory, err
	}
	deployment, err := s.kube.AppsV1().Deployments(s.namespace).Get(ctx, "ucp", metav1.GetOptions{})
	if err != nil {
		return inventory, err
	}
	count := int32(1)
	if deployment.Spec.Replicas != nil {
		count = *deployment.Spec.Replicas
	}
	if deployment.UID != workload.DeploymentUID || !rotationOwned(deployment, s.namespace, s.release) ||
		deployment.Spec.Paused || count < 1 || deployment.Status.ObservedGeneration < deployment.Generation {
		return inventory, errors.New("aggregation deployment identity or convergence is not established")
	}
	if !candidate && (deployment.Status.Replicas != count || deployment.Status.UpdatedReplicas != count ||
		deployment.Status.ReadyReplicas != count || deployment.Status.AvailableReplicas != count) {
		return inventory, errors.New("aggregation deployment identity or convergence is not established")
	}
	if err := rotationAggregationProjection(deployment.Spec.Template.Spec, state.Plan); err != nil {
		return inventory, err
	}
	port, err := aggregationProofPort(deployment.Spec.Template.Spec)
	if err != nil || port != workload.ProofPort {
		return inventory, errors.New("aggregation management listener changed")
	}
	trust, err := s.kube.CoreV1().ConfigMaps(s.namespace).Get(ctx, state.Plan.TargetTrustMap, metav1.GetOptions{})
	if err != nil {
		return inventory, err
	}
	digest, _, err := mtls.TrustBundleDigest([]byte(trust.Data["ca.crt"]), s.now())
	if err != nil || digest != state.Plan.TargetTrustSHA256 || trust.UID == "" || trust.DeletionTimestamp != nil ||
		trust.Immutable == nil || !*trust.Immutable || trust.Labels[rotationOwner] != s.release {
		return inventory, errors.New("aggregation trust generation is missing, mutable or changed")
	}
	leaf, err := s.kube.CoreV1().Secrets(s.namespace).Get(ctx, state.Plan.ServingSecret, metav1.GetOptions{})
	if err != nil {
		return inventory, err
	}
	leafDigest, err := rotationLeafDigest(leaf, state.NewCA, s.namespace, "ucp", false, s.now())
	if err != nil {
		return inventory, err
	}
	inventory.versions[deployment.UID], inventory.versions[trust.UID], inventory.versions[leaf.UID] =
		deployment.ResourceVersion, trust.ResourceVersion, leaf.ResourceVersion
	sets, err := s.kube.AppsV1().ReplicaSets(s.namespace).List(ctx, metav1.ListOptions{Limit: 1000})
	if err != nil {
		return inventory, err
	}
	if sets.Continue != "" {
		return inventory, errors.New("aggregation ReplicaSet inventory exceeds its supported bound")
	}
	owned := map[types.UID]string{}
	currentSets := map[types.UID]bool{}
	for _, set := range sets.Items {
		owner := metav1.GetControllerOf(&set)
		if owner == nil || owner.UID != deployment.UID {
			continue
		}
		if owner.Kind != "Deployment" || owner.APIVersion != "apps/v1" || owner.Name != deployment.Name || set.UID == "" {
			return inventory, errors.New("aggregation ReplicaSet ownership is inconsistent")
		}
		template, current := set.Spec.Template.DeepCopy(), deployment.Spec.Template.DeepCopy()
		delete(template.Labels, appsv1.DefaultDeploymentUniqueLabelKey)
		delete(current.Labels, appsv1.DefaultDeploymentUniqueLabelKey)
		currentSets[set.UID] = set.DeletionTimestamp == nil && apiequality.Semantic.DeepEqual(template, current)
		if set.Spec.Replicas == nil || *set.Spec.Replicas != 0 || set.Status.Replicas != 0 {
			if !candidate && !currentSets[set.UID] {
				return inventory, errors.New("an old aggregation ReplicaSet has not stopped")
			}
		}
		owned[set.UID] = set.Name
		inventory.versions[set.UID] = set.ResourceVersion
	}
	pods, err := s.kube.CoreV1().Pods(s.namespace).List(ctx, metav1.ListOptions{Limit: 1000})
	if err != nil {
		return inventory, err
	}
	if pods.Continue != "" {
		return inventory, errors.New("aggregation pod inventory exceeds its supported bound")
	}
	for _, pod := range pods.Items {
		owner := metav1.GetControllerOf(&pod)
		if owner == nil || owned[owner.UID] == "" {
			if pod.Labels["app.kubernetes.io/name"] == "ucp" || rotationUsesAggregationSecret(pod.Spec) {
				return inventory, fmt.Errorf("unaccounted aggregation consumer %s", pod.Name)
			}
			continue
		}
		if owner.Kind != "ReplicaSet" || owner.APIVersion != "apps/v1" || owner.Name != owned[owner.UID] || pod.UID == "" {
			return inventory, errors.New("aggregation pod ownership is inconsistent")
		}
		inventory.versions[pod.UID] = pod.ResourceVersion
		if candidate && !currentSets[owner.UID] {
			continue
		}
		if err := rotationAggregationProjection(pod.Spec, state.Plan); err != nil {
			return inventory, err
		}
		port, err := aggregationProofPort(pod.Spec)
		if err != nil || port != workload.ProofPort {
			return inventory, errors.New("aggregation pod management listener changed")
		}
		target := TLSProofTarget{Namespace: s.namespace, PodName: pod.Name, PodUID: pod.UID, ContainerName: "ucp",
			Port: workload.ProofPort, TrustBundleSHA256: digest, LeafSHA256: leafDigest}
		container, err := proofContainer(&pod, target)
		if err != nil || !container.Ready {
			return inventory, errors.New("aggregation pod is not running and ready")
		}
		inventory.targets = append(inventory.targets, target)
	}
	if !candidate && len(inventory.targets) != int(count) ||
		candidate && (len(inventory.targets) == 0 || len(inventory.targets) > int(count)) {
		return inventory, errors.New("aggregation pod inventory does not match the converged deployment")
	}
	return inventory, nil
}

func (r *rotationRunner) observeAggregationServingCandidates(ctx context.Context, state rotationState) (map[types.UID]bool, string, error) {
	before, err := r.store.aggregationServingInventory(ctx, state, true)
	if err != nil {
		return nil, "", err
	}
	ids, hash, err := r.store.aggregationPods(ctx, state, true)
	if err != nil {
		return nil, "", err
	}
	observe := r.observeAggregation
	if observe == nil {
		observe = ObserveAggregationProof
	}
	observed := map[types.UID]bool{}
	for _, target := range before.targets {
		proof, err := observe(ctx, r.config, target)
		if err != nil {
			return nil, "", err
		}
		if err := proof.Evidence.Validate(string(target.PodUID), target.TrustBundleSHA256, target.LeafSHA256, time.Now()); err != nil {
			return nil, "", err
		}
		observed[target.PodUID] = true
	}
	if !maps.Equal(ids, observed) {
		return nil, "", errors.New("aggregation candidate inventory changed before serving evidence observation")
	}
	after, err := r.store.aggregationServingInventory(ctx, state, true)
	if err != nil {
		return nil, "", err
	}
	if !maps.Equal(before.versions, after.versions) {
		return nil, "", errors.New("aggregation candidate inventory changed during serving evidence observation")
	}
	return ids, hash, nil
}

func (s *rotationStore) verifyAggregationServing(ctx context.Context, config *rest.Config, state rotationState,
	observe func(context.Context, *rest.Config, TLSProofTarget) (PodAggregationProof, error)) error {
	_, err := s.aggregationServingEvidence(ctx, config, state, observe)
	return err
}

func (s *rotationStore) aggregationServingEvidence(ctx context.Context, config *rest.Config, state rotationState,
	observe func(context.Context, *rest.Config, TLSProofTarget) (PodAggregationProof, error)) (rotationInventory, error) {
	before, err := s.aggregationInventory(ctx, state)
	if err != nil {
		return rotationInventory{}, err
	}
	if observe == nil {
		observe = ObserveAggregationProof
	}
	for _, target := range before.targets {
		proof, err := observe(ctx, config, target)
		if err != nil {
			return rotationInventory{}, err
		}
		if err := proof.Evidence.Validate(string(target.PodUID), target.TrustBundleSHA256, target.LeafSHA256, time.Now()); err != nil {
			return rotationInventory{}, err
		}
	}
	after, err := s.aggregationInventory(ctx, state)
	if err != nil {
		return rotationInventory{}, err
	}
	if !maps.Equal(before.versions, after.versions) {
		return rotationInventory{}, errors.New("aggregation inventory changed during serving evidence observation")
	}
	return after, nil
}
