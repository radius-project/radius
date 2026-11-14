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
	"bytes"
	"context"
	"errors"
	"fmt"
	"math"
	"slices"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
)

func rotationComponentBundle(state rotationState) []byte {
	if state.Phase == rotationRetire || state.Phase == rotationComplete {
		return bytes.Clone(state.NewCA)
	}
	if bytes.Equal(bytes.TrimSpace(state.Plan.OldComponentCA), bytes.TrimSpace(state.NewCA)) {
		return bytes.Clone(state.NewCA)
	}
	return append(append(bytes.Clone(bytes.TrimSpace(state.Plan.OldComponentCA)), '\n'), state.NewCA...)
}

func rotationTLSProjection(name, secret, trust string) corev1.Volume {
	return corev1.Volume{Name: name, VolumeSource: corev1.VolumeSource{
		Projected: &corev1.ProjectedVolumeSource{Sources: []corev1.VolumeProjection{
			{Secret: &corev1.SecretProjection{LocalObjectReference: corev1.LocalObjectReference{Name: secret},
				Items: []corev1.KeyToPath{{Key: "tls.crt", Path: "tls.crt"}, {Key: "tls.key", Path: "tls.key"}}}},
			{ConfigMap: &corev1.ConfigMapProjection{LocalObjectReference: corev1.LocalObjectReference{Name: trust},
				Items: []corev1.KeyToPath{{Key: "ca.crt", Path: "ca.crt"}}}},
		}},
	}}
}

func (s *rotationStore) reconcileRotationDeployment(ctx context.Context, transaction string, workload rotationWorkload) error {
	return s.reconcileRotationDeploymentServing(ctx, transaction, workload, false)
}

func (s *rotationStore) reconcileRotationDeploymentServing(ctx context.Context, transaction string, workload rotationWorkload, serving bool) error {
	client := s.kube.AppsV1().Deployments(s.namespace)
	deployment, err := client.Get(ctx, workload.Name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("cannot inspect rotation deployment: %w", err)
	}
	state, err := s.plannedPhase(ctx, transaction, rotationAdopt, rotationIssue, rotationRetire)
	if err != nil {
		return err
	}
	if !slices.Contains(state.Plan.Consumers, workload) || deployment.UID != workload.UID ||
		!rotationOwned(deployment, s.namespace, s.release) || deployment.Spec.Paused {
		return errors.New("rotation deployment identity, ownership, or pause state changed")
	}
	if err := checkRotationObjectRevision(deployment, state); err != nil {
		return err
	}
	trust, digest, err := s.publishComponentTrust(ctx, rotationComponentBundle(state))
	if err != nil {
		return err
	}
	containerName := workload.Name
	if workload.Name == "bicep-de" {
		containerName = "de"
	}
	found := false
	for index := range deployment.Spec.Template.Spec.Containers {
		container := &deployment.Spec.Template.Spec.Containers[index]
		if container.Name != containerName {
			continue
		}
		found = true
		enabled := false
		for _, variable := range container.Env {
			if variable.Name == "UCP_MTLS_STARTUP_PROOF_ENABLED" && variable.Value == "true" && variable.ValueFrom == nil {
				enabled = true
			}
		}
		if !enabled {
			return errors.New("rotation requires already enabled, verified process evidence; it does not guess image compatibility")
		}
		setRotationEnv(container, corev1.EnvVar{Name: "UCP_MTLS_EXPECTED_TRUST_SHA256", Value: digest})
		setRotationEnv(container, corev1.EnvVar{Name: "UCP_MTLS_POD_UID", ValueFrom: &corev1.EnvVarSource{
			FieldRef: &corev1.ObjectFieldSelector{APIVersion: "v1", FieldPath: "metadata.uid"},
		}})
	}
	if !found {
		return errors.New("planned rotation container is missing")
	}
	if err := replaceRotationVolume(&deployment.Spec.Template.Spec, rotationTLSProjection("mtls", workload.Name+"-mtls", trust.Name)); err != nil {
		return err
	}
	if workload.Name == "controller" {
		if deployment.Spec.Replicas != nil && *deployment.Spec.Replicas != 1 {
			return errors.New("rotation does not surge or change the controller's replica count")
		}
		deployment.Spec.Strategy = appsv1.DeploymentStrategy{Type: appsv1.RecreateDeploymentStrategyType}
	} else {
		deployment.Spec.Strategy = appsv1.DeploymentStrategy{Type: appsv1.RollingUpdateDeploymentStrategyType,
			RollingUpdate: &appsv1.RollingUpdateDeployment{MaxSurge: ptr.To(intstr.FromInt32(1)), MaxUnavailable: ptr.To(intstr.FromInt32(0))}}
	}
	if workload.Name == "ucp" && (state.Phase != rotationAdopt || serving && state.SchemaVersion == 2) {
		if err := s.configureAggregationDeployment(ctx, state, deployment); err != nil {
			return err
		}
	}
	deployment.Annotations[rotationRevision] = state.revision()
	if err := s.checkWrite(ctx); err != nil {
		return err
	}
	_, err = client.Update(ctx, deployment, metav1.UpdateOptions{FieldManager: FieldManager})
	if err != nil {
		return fmt.Errorf("cannot update rotation deployment; reread the phase before retrying: %w", err)
	}
	return nil
}

func (s *rotationStore) reconcileAggregationDeployment(ctx context.Context, transaction string) error {
	client := s.kube.AppsV1().Deployments(s.namespace)
	deployment, err := client.Get(ctx, "ucp", metav1.GetOptions{})
	if err != nil {
		return err
	}
	state, err := s.plannedPhase(ctx, transaction, rotationAdopt, rotationIssue, rotationRetire)
	if err != nil {
		return err
	}
	if state.Plan.effectiveScope() != rotationAggregationOnly ||
		deployment.UID != state.Plan.Aggregation.DeploymentUID ||
		!rotationOwned(deployment, s.namespace, s.release) || deployment.Spec.Paused {
		return errors.New("aggregation deployment identity, scope, ownership or pause state changed")
	}
	if err := checkRotationObjectRevision(deployment, state); err != nil {
		return err
	}
	if err := s.configureAggregationDeployment(ctx, state, deployment); err != nil {
		return err
	}
	deployment.Annotations[rotationRevision] = state.revision()
	if err := s.checkWrite(ctx); err != nil {
		return err
	}
	_, err = client.Update(ctx, deployment, metav1.UpdateOptions{FieldManager: FieldManager})
	if err != nil {
		return fmt.Errorf("cannot update aggregation deployment; reread the phase before retrying: %w", err)
	}
	return nil
}

func (s *rotationStore) configureAggregationDeployment(ctx context.Context, state rotationState, deployment *appsv1.Deployment) error {
	servingTrust, digest, err := s.publishComponentTrust(ctx, state.NewCA)
	if err != nil {
		return err
	}
	if err := replaceRotationVolume(&deployment.Spec.Template.Spec, rotationTLSProjection("cert", state.Plan.ServingSecret, servingTrust.Name)); err != nil {
		return err
	}
	if state.SchemaVersion == 2 {
		port, err := aggregationProofPort(deployment.Spec.Template.Spec)
		if err != nil || port != state.Plan.Aggregation.ProofPort {
			return errors.New("aggregation serving proof requires its pinned management listener")
		}
		for index := range deployment.Spec.Template.Spec.Containers {
			container := &deployment.Spec.Template.Spec.Containers[index]
			if container.Name != "ucp" {
				continue
			}
			setRotationEnv(container, corev1.EnvVar{Name: "UCP_AGGREGATION_STARTUP_PROOF_ENABLED", Value: "true"})
			setRotationEnv(container, corev1.EnvVar{Name: "UCP_AGGREGATION_EXPECTED_TRUST_SHA256", Value: digest})
			setRotationEnv(container, corev1.EnvVar{Name: "UCP_AGGREGATION_POD_UID", ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{APIVersion: "v1", FieldPath: "metadata.uid"},
			}})
		}
		if err := rotationAggregationProjection(deployment.Spec.Template.Spec, state.Plan); err != nil {
			return err
		}
	}
	deployment.Spec.Strategy = appsv1.DeploymentStrategy{Type: appsv1.RollingUpdateDeploymentStrategyType,
		RollingUpdate: &appsv1.RollingUpdateDeployment{MaxSurge: ptr.To(intstr.FromInt32(1)), MaxUnavailable: ptr.To(intstr.FromInt32(0))}}
	deployment.Spec.MinReadySeconds = state.Plan.UCPMinReadySeconds
	deployment.Spec.ProgressDeadlineSeconds = ptr.To(state.Plan.UCPProgressDeadlineSeconds)
	if state.Phase == rotationIssue || state.Phase == rotationAdopt && state.SchemaVersion == 2 {
		// Read routing after the deployment so a queued hold cannot overwrite
		// a completed release of the retained old backend.
		service, err := s.kube.CoreV1().Services(s.namespace).Get(ctx, "ucp", metav1.GetOptions{})
		if err != nil {
			return err
		}
		if service.UID != state.Plan.ServiceUID || !rotationOwned(service, s.namespace, s.release) ||
			service.Annotations[rotationTransaction] != state.ID {
			return errors.New("aggregation rollout requires its retained, owned Service route")
		}
		switch service.Annotations[rotationRoute] {
		case "old", "new":
			// This is an explicit hold, not a timed adoption proof.
			deployment.Spec.MinReadySeconds = math.MaxInt32 - 1
			deployment.Spec.ProgressDeadlineSeconds = ptr.To(int32(math.MaxInt32))
		case "complete":
		default:
			return errors.New("aggregation rollout has no recognized routing state")
		}
	}
	return nil
}

func setRotationEnv(container *corev1.Container, value corev1.EnvVar) {
	for index := range container.Env {
		if container.Env[index].Name == value.Name {
			container.Env[index] = value
			return
		}
	}
	container.Env = append(container.Env, value)
}

func replaceRotationVolume(spec *corev1.PodSpec, volume corev1.Volume) error {
	for index := range spec.Volumes {
		if spec.Volumes[index].Name == volume.Name {
			spec.Volumes[index] = volume
			return nil
		}
	}
	return fmt.Errorf("rotation requires the existing %s TLS volume", volume.Name)
}

func (s *rotationStore) rotationConsumerTargets(ctx context.Context, state rotationState) ([]rotationConsumer, error) {
	if state.Plan == nil || state.Plan.effectiveScope() != rotationComponentsAndAggregation {
		return nil, errors.New("component TLS inventory is unavailable for aggregation-only rotation")
	}
	trust, digest, err := s.publishComponentTrust(ctx, rotationComponentBundle(state))
	if err != nil {
		return nil, err
	}
	ca := state.NewCA
	if state.Phase == rotationAdopt {
		ca = state.Plan.OldComponentCA
	}
	targets := make([]rotationConsumer, 0, len(state.Plan.Consumers))
	for _, workload := range state.Plan.Consumers {
		secret, err := s.kube.CoreV1().Secrets(s.namespace).Get(ctx, workload.Name+"-mtls", metav1.GetOptions{})
		if err != nil {
			return nil, fmt.Errorf("cannot inspect issued component leaf: %w", err)
		}
		leaf, err := rotationLeafDigest(secret, ca, s.namespace, workload.Name, true, s.now())
		if err != nil {
			return nil, fmt.Errorf("component %s: %w", workload.Name, err)
		}
		targets = append(targets, rotationConsumer{Name: workload.Name, DeploymentUID: workload.UID,
			TrustMap: trust.Name, TrustSHA256: digest, LeafSHA256: leaf, ProofPort: workload.ProofPort})
	}
	return targets, nil
}
