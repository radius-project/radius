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
	"cmp"
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/radius-project/radius/pkg/authz"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
)

type providerRuntimeChecks struct {
	ca       *rotationStore
	config   *rest.Config
	validate func(context.Context, providerTransitionPlan, map[string]appsv1.DeploymentSpec) error
	observe  func(context.Context, providerTransitionPlan) error
}

func (c *providerRuntimeChecks) aggregation(ctx context.Context) (providerAggregationSnapshot, error) {
	return c.ca.aggregationTransitionSnapshot(ctx)
}

func (c *providerRuntimeChecks) preflight(ctx context.Context, plan providerTransitionPlan, targets map[string]appsv1.DeploymentSpec) error {
	if c.validate == nil {
		return errors.New("provider transition requires live release, policy and target validation")
	}
	return c.validate(ctx, plan, targets)
}

func (c *providerRuntimeChecks) verify(ctx context.Context, plan providerTransitionPlan) error {
	if err := c.ca.checkAggregationTransitionSnapshot(ctx, plan.Aggregation); err != nil {
		return err
	}
	if c.observe != nil {
		if err := c.observe(ctx, plan); err != nil {
			return err
		}
		return c.ca.checkAggregationTransitionSnapshot(ctx, plan.Aggregation)
	}
	_, ca, err := c.ca.read(ctx)
	if err != nil {
		return err
	}
	if plan.Target == "external" {
		if err := c.verifyExternal(ctx, plan); err != nil {
			return err
		}
	} else if plan.Target == "radius" {
		consumers := make([]rotationConsumer, 0, len(plan.Workloads))
		for _, workload := range plan.Workloads {
			deployment, err := c.ca.kube.AppsV1().Deployments(plan.Namespace).Get(ctx, workload.Name, metav1.GetOptions{})
			if err != nil {
				return err
			}
			secret, err := c.ca.kube.CoreV1().Secrets(plan.Namespace).Get(ctx, workload.Name+"-mtls", metav1.GetOptions{})
			if err != nil {
				return err
			}
			digest, err := rotationLeafDigest(secret, ca.NewCA, plan.Namespace, workload.Name, true, c.ca.now())
			if err != nil {
				return fmt.Errorf("provider component %s has no valid selected-CA credential: %w", workload.Name, err)
			}
			port, err := bootstrapComponentProofPort(*deployment, workload.Name)
			if err != nil {
				return err
			}
			consumers = append(consumers, rotationConsumer{Name: workload.Name, DeploymentUID: workload.UID,
				TrustMap: ca.Plan.TargetTrustMap, TrustSHA256: plan.Aggregation.TrustSHA256, LeafSHA256: digest, ProofPort: port})
		}
		observer := rotationConsumerObserver{kube: c.ca.kube, config: c.config, namespace: plan.Namespace, release: plan.Release}
		if err := observer.verify(ctx, consumers); err != nil {
			return err
		}
	} else {
		return errors.New("unknown target provider evidence")
	}
	aggregate := rotationRunner{store: c.ca, config: c.config}
	if ca.Bootstrap {
		err = aggregate.verifyBootstrapEvidence(ctx, ca, false)
	} else {
		err = aggregate.verifyAggregation(ctx, ca)
	}
	if err != nil {
		return err
	}
	return c.ca.checkAggregationTransitionSnapshot(ctx, plan.Aggregation)
}

type providerExternalInventory struct {
	targets  map[string][]externalIdentityTarget
	versions map[types.UID]string
}

func (c *providerRuntimeChecks) externalInventory(ctx context.Context, plan providerTransitionPlan) (providerExternalInventory, error) {
	inventory := providerExternalInventory{targets: map[string][]externalIdentityTarget{}, versions: map[types.UID]string{}}
	store := providerTransitionStore{kube: c.ca.kube}
	state := providerTransitionState{Version: 1, Plan: plan, Phase: providerVerifying}
	if err := store.inspect(ctx, state); err != nil {
		return inventory, err
	}
	if err := store.checkReady(ctx, state); err != nil {
		return inventory, err
	}
	deployments := map[types.UID]providerTransitionWorkload{}
	for _, workload := range plan.Workloads {
		if workload.Replicas < 1 {
			return inventory, errors.New("external readiness requires at least one current process for every component")
		}
		deployments[workload.UID] = workload
		d, _, err := store.deployment(ctx, plan, workload)
		if err != nil {
			return inventory, err
		}
		inventory.versions[d.UID] = d.ResourceVersion
	}
	sets, err := c.ca.kube.AppsV1().ReplicaSets(plan.Namespace).List(ctx, metav1.ListOptions{Limit: 1000})
	if err != nil {
		return inventory, err
	}
	pods, err := c.ca.kube.CoreV1().Pods(plan.Namespace).List(ctx, metav1.ListOptions{Limit: 1000})
	if err != nil {
		return inventory, err
	}
	if sets.Continue != "" || pods.Continue != "" {
		return inventory, errors.New("external identity inventory exceeds its supported bound")
	}
	owners := map[types.UID]providerTransitionWorkload{}
	setNames := map[types.UID]string{}
	for _, set := range sets.Items {
		owner := metav1.GetControllerOf(&set)
		if owner == nil {
			continue
		}
		workload, found := deployments[owner.UID]
		if !found {
			continue
		}
		if owner.APIVersion != "apps/v1" || owner.Kind != "Deployment" || owner.Name != workload.Name || set.UID == "" {
			return inventory, errors.New("external identity ReplicaSet ownership changed")
		}
		owners[set.UID] = workload
		setNames[set.UID] = set.Name
		inventory.versions[set.UID] = set.ResourceVersion
	}
	for _, pod := range pods.Items {
		owner := metav1.GetControllerOf(&pod)
		if owner == nil {
			continue
		}
		workload, found := owners[owner.UID]
		if !found {
			continue
		}
		if owner.APIVersion != "apps/v1" || owner.Kind != "ReplicaSet" || owner.Name != setNames[owner.UID] {
			return inventory, errors.New("external identity pod ownership changed")
		}
		container, port := workload.Name, uint16(0)
		switch workload.Name {
		case "ucp":
			port = 9444
		case "applications-rp":
			port = 5443
		case "dynamic-rp":
			port = 8082
		case "bicep-de":
			container, port = "de", 6443
		case "controller":
			port, err = bootstrapComponentProofPort(appsv1.Deployment{Spec: appsv1.DeploymentSpec{
				Template: corev1.PodTemplateSpec{Spec: pod.Spec},
			}}, workload.Name)
			if err != nil {
				return inventory, err
			}
		}
		target := externalIdentityTarget{Component: workload.Name, TLSProofTarget: TLSProofTarget{
			Namespace: plan.Namespace, PodName: pod.Name, PodUID: pod.UID, ContainerName: container, Port: port}}
		if _, err := identityPod(&pod, target); err != nil {
			return inventory, err
		}
		inventory.targets[workload.Name] = append(inventory.targets[workload.Name], target)
		inventory.versions[pod.UID] = pod.ResourceVersion
	}
	for _, workload := range plan.Workloads {
		if len(inventory.targets[workload.Name]) != int(workload.Replicas) {
			return inventory, errors.New("external identity pod population does not match the pinned replicas")
		}
		slices.SortFunc(inventory.targets[workload.Name], func(a, b externalIdentityTarget) int {
			return cmp.Compare(a.PodName, b.PodName)
		})
	}
	return inventory, nil
}

func (c *providerRuntimeChecks) verifyExternal(ctx context.Context, plan providerTransitionPlan) error {
	before, err := c.externalInventory(ctx, plan)
	if err != nil {
		return err
	}
	ucps := map[types.UID]externalIdentityTarget{}
	for _, ucp := range before.targets["ucp"] {
		ucps[ucp.PodUID] = ucp
	}
	observer := externalIdentityObserver{kube: c.ca.kube, config: c.config}
	for _, component := range rotationComponents {
		caller := before.targets["ucp"][0]
		if component == "ucp" {
			caller = before.targets["bicep-de"][0]
		}
		for _, target := range before.targets[component] {
			if _, err := observer.observe(ctx, authz.Mode(plan.Mode), target, caller, ucps); err != nil {
				return fmt.Errorf("provider readiness for %s: %w", component, err)
			}
		}
	}
	after, err := c.externalInventory(ctx, plan)
	if err != nil {
		return err
	}
	if !maps.Equal(before.versions, after.versions) {
		return errors.New("external identity fleet changed during receiver observations")
	}
	return nil
}
