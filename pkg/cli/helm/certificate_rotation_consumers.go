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
	"slices"
	"time"

	"github.com/radius-project/radius/pkg/authz/mtls"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

var rotationComponents = []string{"ucp", "applications-rp", "dynamic-rp", "controller", "bicep-de"}

type rotationConsumer struct {
	DeploymentUID types.UID `json:"deploymentUID"`
	Name          string    `json:"name"`
	TrustMap      string    `json:"trustMap"`
	TrustSHA256   string    `json:"trustSHA256"`
	LeafSHA256    string    `json:"leafSHA256"`
	ProofPort     uint16    `json:"proofPort"`
}

type rotationConsumerObserver struct {
	kube      kubernetes.Interface
	config    *rest.Config
	namespace string
	release   string
	observe   func(context.Context, *rest.Config, TLSProofTarget) (PodTLSProof, error)
}

type rotationInventory struct {
	targets  []TLSProofTarget
	versions map[types.UID]string
}

// verify requires current process evidence from all five component consumers.
// The second inventory rejects rollout/scaling/restart races during observation.
// It does not attest the Kubernetes aggregation transport or force-deleted processes.
func (o *rotationConsumerObserver) verify(ctx context.Context, expected []rotationConsumer) error {
	before, err := o.inventory(ctx, expected)
	if err != nil {
		return err
	}
	observe := o.observe
	if observe == nil {
		observe = ObserveTLSStartupProof
	}
	for _, target := range before.targets {
		proof, err := observe(ctx, o.config, target)
		if err != nil {
			return fmt.Errorf("CA rotation is waiting for %s to load trust: %w", target.PodName, err)
		}
		if err := proof.Evidence.Validate(string(target.PodUID), target.TrustBundleSHA256, target.LeafSHA256, time.Now()); err != nil {
			return fmt.Errorf("CA rotation received invalid process evidence from %s: %w", target.PodName, err)
		}
	}
	after, err := o.inventory(ctx, expected)
	if err != nil {
		return err
	}
	if !maps.Equal(before.versions, after.versions) {
		return errors.New("CA rotation consumer inventory changed during observation; repeat the entire barrier")
	}
	return nil
}

func (o *rotationConsumerObserver) inventory(ctx context.Context, expected []rotationConsumer) (rotationInventory, error) {
	inventory := rotationInventory{versions: map[types.UID]string{}}
	consumers := map[string]rotationConsumer{}
	if len(expected) != len(rotationComponents) {
		return inventory, errors.New("CA rotation must account for UCP, both resource providers, controller, and deployment engine")
	}
	deployments := map[types.UID]*appsv1.Deployment{}
	replicas := map[types.UID]int32{}
	for _, consumer := range expected {
		if !slices.Contains(rotationComponents, consumer.Name) || consumers[consumer.Name].Name != "" ||
			consumer.DeploymentUID == "" || consumer.TrustMap == "" || consumer.ProofPort == 0 || consumer.LeafSHA256 == "" {
			return inventory, errors.New("invalid or duplicate CA rotation consumer")
		}
		consumers[consumer.Name] = consumer
		deployment, err := o.kube.AppsV1().Deployments(o.namespace).Get(ctx, consumer.Name, metav1.GetOptions{})
		if err != nil {
			return inventory, fmt.Errorf("cannot inspect CA rotation deployment %s: %w", consumer.Name, err)
		}
		if deployment.UID != consumer.DeploymentUID || deployment.DeletionTimestamp != nil || deployment.Spec.Paused ||
			deployment.Annotations["meta.helm.sh/release-name"] != o.release ||
			deployment.Annotations["meta.helm.sh/release-namespace"] != o.namespace {
			return inventory, fmt.Errorf("CA rotation deployment %s changed identity, ownership, or is paused/terminating", consumer.Name)
		}
		count := int32(1)
		if deployment.Spec.Replicas != nil {
			count = *deployment.Spec.Replicas
		}
		if count < 1 || deployment.Status.ObservedGeneration < deployment.Generation ||
			deployment.Status.UpdatedReplicas != count || deployment.Status.ReadyReplicas != count ||
			deployment.Status.AvailableReplicas != count || deployment.Status.Replicas != count {
			return inventory, fmt.Errorf("CA rotation deployment %s has not converged", consumer.Name)
		}
		if consumer.Name == "controller" && (count != 1 || deployment.Spec.Strategy.Type != appsv1.RecreateDeploymentStrategyType) {
			return inventory, errors.New("CA rotation requires a single controller with Recreate strategy, not concurrent active controllers")
		}
		if err := rotationConsumerTemplate(deployment.Spec.Template, consumer); err != nil {
			return inventory, fmt.Errorf("CA rotation deployment %s: %w", consumer.Name, err)
		}
		trust, err := o.kube.CoreV1().ConfigMaps(o.namespace).Get(ctx, consumer.TrustMap, metav1.GetOptions{})
		if err != nil {
			return inventory, fmt.Errorf("cannot inspect CA rotation public trust: %w", err)
		}
		digest, _, err := mtls.TrustBundleDigest([]byte(trust.Data["ca.crt"]), time.Now())
		if err != nil || digest != consumer.TrustSHA256 || trust.Immutable == nil || !*trust.Immutable ||
			trust.UID == "" || trust.DeletionTimestamp != nil || trust.Labels[rotationOwner] != o.release {
			return inventory, errors.New("CA rotation consumer trust must be an intact immutable public generation")
		}
		inventory.versions[trust.UID] = trust.ResourceVersion
		inventory.versions[deployment.UID] = deployment.ResourceVersion
		deployments[deployment.UID], replicas[deployment.UID] = deployment, count
	}
	// List the namespace, not just mutable labels: an old pod must not disappear
	// from the barrier merely because its selector label was changed.
	sets, err := o.kube.AppsV1().ReplicaSets(o.namespace).List(ctx, metav1.ListOptions{Limit: 1000})
	if err != nil {
		return inventory, fmt.Errorf("cannot inventory CA rotation ReplicaSets: %w", err)
	}
	if sets.Continue != "" {
		return inventory, errors.New("CA rotation ReplicaSet inventory exceeds the supported bound")
	}
	setOwners := map[types.UID]*appsv1.Deployment{}
	for _, set := range sets.Items {
		owner := metav1.GetControllerOf(&set)
		if owner == nil || owner.APIVersion != "apps/v1" || owner.Kind != "Deployment" {
			continue
		}
		deployment := deployments[owner.UID]
		if deployment == nil {
			continue
		}
		if owner.Name != deployment.Name || set.UID == "" {
			return inventory, errors.New("CA rotation ReplicaSet has inconsistent ownership")
		}
		template := set.Spec.Template.DeepCopy()
		delete(template.Labels, appsv1.DefaultDeploymentUniqueLabelKey)
		current := deployment.Spec.Template.DeepCopy()
		delete(current.Labels, appsv1.DefaultDeploymentUniqueLabelKey)
		if set.Spec.Replicas == nil || *set.Spec.Replicas != 0 || set.Status.Replicas != 0 {
			if set.DeletionTimestamp != nil || !apiequality.Semantic.DeepEqual(template, current) {
				return inventory, fmt.Errorf("CA rotation is waiting for old ReplicaSet %s to stop", set.Name)
			}
		}
		setOwners[set.UID] = deployment
		inventory.versions[set.UID] = set.ResourceVersion
	}
	pods, err := o.kube.CoreV1().Pods(o.namespace).List(ctx, metav1.ListOptions{Limit: 1000})
	if err != nil {
		return inventory, fmt.Errorf("cannot inventory CA rotation pods: %w", err)
	}
	if pods.Continue != "" {
		return inventory, errors.New("CA rotation pod inventory exceeds the supported bound")
	}
	counts := map[types.UID]int32{}
	for _, pod := range pods.Items {
		owner := metav1.GetControllerOf(&pod)
		var deployment *appsv1.Deployment
		if owner != nil && owner.APIVersion == "apps/v1" && owner.Kind == "ReplicaSet" {
			deployment = setOwners[owner.UID]
		}
		if deployment == nil {
			if slices.Contains(rotationComponents, pod.Labels["app.kubernetes.io/name"]) || rotationUsesComponentSecret(pod.Spec) {
				return inventory, fmt.Errorf("CA rotation found unaccounted consumer pod %s", pod.Name)
			}
			continue
		}
		consumer := consumers[deployment.Name]
		if pod.UID == "" || pod.DeletionTimestamp != nil || pod.Status.Phase != corev1.PodRunning {
			return inventory, fmt.Errorf("CA rotation is waiting for pod %s to be replaced or become running", pod.Name)
		}
		if err := rotationConsumerTemplate(corev1.PodTemplateSpec{Spec: pod.Spec}, consumer); err != nil {
			return inventory, fmt.Errorf("CA rotation pod %s: %w", pod.Name, err)
		}
		container := consumer.Name
		if container == "bicep-de" {
			container = "de"
		}
		target := TLSProofTarget{
			Namespace: o.namespace, PodName: pod.Name, PodUID: pod.UID,
			ContainerName: container, Port: consumer.ProofPort,
			TrustBundleSHA256: consumer.TrustSHA256, LeafSHA256: consumer.LeafSHA256,
		}
		status, err := proofContainer(&pod, target)
		if err != nil || !status.Ready {
			return inventory, fmt.Errorf("CA rotation container in %s is not ready", pod.Name)
		}
		inventory.targets = append(inventory.targets, target)
		inventory.versions[pod.UID] = pod.ResourceVersion
		counts[deployment.UID]++
	}
	if !maps.Equal(counts, replicas) {
		return inventory, errors.New("CA rotation pod inventory does not match the converged deployments")
	}
	return inventory, nil
}

func rotationConsumerTemplate(template corev1.PodTemplateSpec, expected rotationConsumer) error {
	containerName := expected.Name
	if containerName == "bicep-de" {
		containerName = "de"
	}
	for _, container := range append(slices.Clone(template.Spec.Containers), template.Spec.InitContainers...) {
		if container.Name == containerName {
			continue
		}
		material := corev1.PodSpec{Containers: []corev1.Container{container}}
		for _, mount := range container.VolumeMounts {
			if mount.Name == "mtls" {
				return errors.New("additional container consumes component TLS material without process evidence")
			}
			for _, volume := range template.Spec.Volumes {
				if volume.Name == mount.Name {
					material.Volumes = append(material.Volumes, volume)
				}
			}
		}
		if rotationUsesComponentSecret(material) {
			return errors.New("additional container consumes component TLS material without process evidence")
		}
	}
	for _, container := range template.Spec.Containers {
		if container.Name != containerName {
			continue
		}
		env := map[string]corev1.EnvVar{}
		for _, variable := range container.Env {
			if _, duplicate := env[variable.Name]; duplicate {
				return errors.New("duplicate environment variable in CA rotation consumer")
			}
			env[variable.Name] = variable
		}
		podUID := env["UCP_MTLS_POD_UID"]
		if env["UCP_MTLS_STARTUP_PROOF_ENABLED"].Value != "true" ||
			env["UCP_MTLS_EXPECTED_TRUST_SHA256"].Value != expected.TrustSHA256 ||
			env["UCP_MTLS_STARTUP_PROOF_ENABLED"].ValueFrom != nil ||
			env["UCP_MTLS_EXPECTED_TRUST_SHA256"].ValueFrom != nil ||
			podUID.Value != "" || podUID.ValueFrom == nil || podUID.ValueFrom.FieldRef == nil ||
			podUID.ValueFrom.FieldRef.FieldPath != "metadata.uid" {
			return errors.New("process evidence is not pinned to the intended trust generation and pod identity")
		}
		for _, mount := range container.VolumeMounts {
			if mount.Name != "mtls" || mount.MountPath != "/var/run/radius/mtls" ||
				!mount.ReadOnly || mount.SubPath != "" || mount.SubPathExpr != "" {
				continue
			}
			for _, volume := range template.Spec.Volumes {
				if volume.Name != mount.Name || volume.Projected == nil || len(volume.Projected.Sources) != 2 {
					continue
				}
				secret, trust := volume.Projected.Sources[0].Secret, volume.Projected.Sources[1].ConfigMap
				if secret != nil && secret.Name == expected.Name+"-mtls" && trust != nil &&
					trust.Name == expected.TrustMap && len(trust.Items) == 1 &&
					trust.Items[0].Key == "ca.crt" && trust.Items[0].Path == "ca.crt" &&
					len(secret.Items) == 2 && secret.Items[0].Key == "tls.crt" && secret.Items[0].Path == "tls.crt" &&
					secret.Items[1].Key == "tls.key" && secret.Items[1].Path == "tls.key" {
					return nil
				}
			}
		}
		return errors.New("consumer does not project the intended immutable trust generation")
	}
	return errors.New("CA rotation consumer container is missing")
}

func rotationUsesComponentSecret(spec corev1.PodSpec) bool {
	names := []string{}
	for _, volume := range spec.Volumes {
		if volume.Secret != nil {
			names = append(names, volume.Secret.SecretName)
		}
		if volume.Projected != nil {
			for _, source := range volume.Projected.Sources {
				if source.Secret != nil {
					names = append(names, source.Secret.Name)
				}
			}
		}
	}
	for _, container := range append(slices.Clone(spec.Containers), spec.InitContainers...) {
		for _, env := range container.Env {
			if env.ValueFrom != nil && env.ValueFrom.SecretKeyRef != nil {
				names = append(names, env.ValueFrom.SecretKeyRef.Name)
			}
		}
		for _, env := range container.EnvFrom {
			if env.SecretRef != nil {
				names = append(names, env.SecretRef.Name)
			}
		}
	}
	for _, component := range rotationComponents {
		if slices.Contains(names, component+"-mtls") {
			return true
		}
	}
	return false
}
