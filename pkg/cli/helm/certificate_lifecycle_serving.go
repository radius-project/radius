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
	"reflect"
	"slices"

	"github.com/radius-project/radius/pkg/authz/mtls"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

func (inventory *servingInventory) pin(kind string, object metav1.Object) error {
	if object.GetUID() == "" || object.GetResourceVersion() == "" || object.GetDeletionTimestamp() != nil {
		return fmt.Errorf("%s %s has no live serving identity", kind, object.GetName())
	}
	inventory.versions = append(inventory.versions, fmt.Sprintf("%s/%s/%s@%s", kind, object.GetName(), object.GetUID(), object.GetResourceVersion()))
	return nil
}

func readLifecycleServingInventory(ctx context.Context, kube kubernetes.Interface, state certificateLifecycle, target *certificateTarget) (servingInventory, error) {
	var inventory servingInventory
	if err := ctx.Err(); err != nil {
		return inventory, err
	}
	if target == nil || target.Provider != state.Provider || !slices.Contains([]rotationPhase{rotationIssue, rotationReady, rotationRetire, rotationComplete}, state.Phase) {
		return inventory, errors.New("serving inventory requires the selected issuance or retirement target")
	}
	if plane := state.Planes["aggregation"]; plane.Affected {
		var err error
		inventory, err = readAggregationServingInventoryForCA(ctx, kube, state.Namespace, plane.Source.Bundle, plane.NewCA)
		if err != nil {
			return inventory, err
		}
	}
	if plane := state.Planes["internal"]; plane.Affected {
		if len(target.Receivers) == 0 {
			return inventory, errors.New("internal serving inventory has no selected receivers")
		}
		settings := resolveSPIFFESettings(nil, target.Values)
		driver, err := kube.StorageV1().CSIDrivers().Get(ctx, settings.CSIDriverName, metav1.GetOptions{})
		if err != nil {
			return inventory, err
		}
		if err := inventory.pin("CSIDriver", driver); err != nil {
			return inventory, err
		}
		seen := map[string]bool{}
		for _, receiver := range target.Receivers {
			portNames := map[string][]string{
				"ucp": {"internal"}, "applications-rp": {"applications-rp"},
				"dynamic-rp": {"dynamic-rp"}, "bicep-de": {"bicep-de-api"},
			}[receiver]
			if seen[receiver] || len(portNames) == 0 {
				return inventory, errors.New("serving inventory has a duplicate or unsupported receiver")
			}
			seen[receiver] = true
			deployment, err := kube.AppsV1().Deployments(state.Namespace).Get(ctx, receiver, metav1.GetOptions{})
			if err != nil {
				return inventory, err
			}
			if err := inventory.pin("Deployment", deployment); err != nil {
				return inventory, err
			}
			replicas := int32(1)
			if deployment.Spec.Replicas != nil {
				replicas = *deployment.Spec.Replicas
			}
			if replicas < 1 || deployment.Spec.Paused || deployment.Generation <= 0 ||
				deployment.Status.ObservedGeneration != deployment.Generation || deployment.Status.Replicas != replicas ||
				deployment.Status.UpdatedReplicas != replicas || deployment.Status.ReadyReplicas != replicas ||
				deployment.Status.AvailableReplicas != replicas || deployment.Spec.Template.Spec.ServiceAccountName != receiver {
				return inventory, fmt.Errorf("serving deployment %s has not converged to its selected identity", receiver)
			}
			selector, err := metav1.LabelSelectorAsSelector(deployment.Spec.Selector)
			if err != nil || selector.Empty() {
				return inventory, errors.New("serving deployment requires a nonempty pod selector")
			}
			account, err := kube.CoreV1().ServiceAccounts(state.Namespace).Get(ctx, receiver, metav1.GetOptions{})
			if err != nil {
				return inventory, err
			}
			if err := inventory.pin("ServiceAccount", account); err != nil {
				return inventory, err
			}
			pods, err := kube.CoreV1().Pods(state.Namespace).List(ctx, metav1.ListOptions{LabelSelector: selector.String(), Limit: 1000})
			if err != nil {
				return inventory, err
			}
			if pods.Continue != "" || int32(len(pods.Items)) != replicas {
				return inventory, fmt.Errorf("serving pod inventory for %s is incomplete or still rolling", receiver)
			}
			for index := range pods.Items {
				pod := &pods.Items[index]
				if err := inventory.pin("Pod", pod); err != nil {
					return inventory, err
				}
				owner := metav1.GetControllerOf(pod)
				if owner == nil || owner.Kind != "ReplicaSet" || owner.APIVersion != "apps/v1" {
					return inventory, errors.New("serving pod has no ReplicaSet identity")
				}
				rs, err := kube.AppsV1().ReplicaSets(state.Namespace).Get(ctx, owner.Name, metav1.GetOptions{})
				if err != nil {
					return inventory, err
				}
				if err := inventory.pin("ReplicaSet", rs); err != nil {
					return inventory, err
				}
				current, expected := rs.Spec.Template.DeepCopy(), deployment.Spec.Template.DeepCopy()
				delete(current.Labels, appsv1.DefaultDeploymentUniqueLabelKey)
				delete(expected.Labels, appsv1.DefaultDeploymentUniqueLabelKey)
				if !apiequality.Semantic.DeepEqual(current, expected) {
					return inventory, errors.New("serving pod belongs to an old workload template")
				}
				for _, portName := range portNames {
					port := int32(0)
					for _, container := range deployment.Spec.Template.Spec.Containers {
						if container.Name == receiver {
							for _, declared := range container.Ports {
								if declared.Name == portName && (declared.Protocol == "" || declared.Protocol == corev1.ProtocolTCP) {
									if port != 0 {
										return inventory, errors.New("serving listener declaration is ambiguous")
									}
									port = declared.ContainerPort
								}
							}
						}
					}
					if port <= 0 || port > 65535 {
						return inventory, fmt.Errorf("receiver %s has no selected %s listener", receiver, portName)
					}
					observation := ServingCertificateTarget{
						TLSProofTarget: TLSProofTarget{Namespace: state.Namespace, PodName: pod.Name, PodUID: pod.UID, ContainerName: receiver, Port: uint16(port)},
						WorkloadUID:    deployment.UID, WorkloadGeneration: deployment.Generation,
						ServiceAccount: account.Name, ServiceAccountUID: account.UID,
						Driver: driver.Name, DriverUID: driver.UID, CA: plane.NewCA,
						SPIFFEID: mtls.SPIFFEID(target.TrustDomain, state.Namespace, receiver),
					}
					observed, err := observedServingPod(ctx, kube, observation)
					if err != nil {
						return inventory, err
					}
					if observed.UID != pod.UID || observed.ResourceVersion != pod.ResourceVersion {
						return inventory, errors.New("serving pod changed during inventory collection")
					}
					if _, err := proofContainer(observed, observation.TLSProofTarget); err != nil {
						return inventory, err
					}
					inventory.targets = append(inventory.targets, observation)
				}
			}
		}
	}
	slices.Sort(inventory.versions)
	slices.SortFunc(inventory.targets, cmpServingTarget)
	return inventory, ctx.Err()
}

func cmpServingTarget(a, b ServingCertificateTarget) int {
	if a.PodName < b.PodName || a.PodName == b.PodName && a.Port < b.Port {
		return -1
	}
	if a.PodName == b.PodName && a.Port == b.Port {
		return 0
	}
	return 1
}

func observeLifecycleServing(ctx context.Context, config *rest.Config, kube kubernetes.Interface, state certificateLifecycle, target *certificateTarget,
	observe func(context.Context, *rest.Config, ServingCertificateTarget) (ServingCertificateObservation, error)) error {
	before, err := readLifecycleServingInventory(ctx, kube, state, target)
	if err != nil {
		return err
	}
	if len(before.targets) == 0 {
		return errors.New("certificate lifecycle has no affected serving targets")
	}
	for _, selected := range before.targets {
		if _, err := observe(ctx, config, selected); err != nil {
			return fmt.Errorf("serving receiver %s/%s:%d: %w", selected.Namespace, selected.PodName, selected.Port, err)
		}
	}
	after, err := readLifecycleServingInventory(ctx, kube, state, target)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(before, after) {
		return errors.New("certificate serving inventory changed during observation")
	}
	return nil
}

func (e *certificateLifecycleExecutor) observeServing(ctx context.Context, state certificateLifecycle) error {
	if err := e.writer.check(ctx); err != nil {
		return err
	}
	flags := genericclioptions.NewConfigFlags(false)
	flags.Context, flags.Namespace = &e.kubeContext, &state.Namespace
	config, err := flags.ToRESTConfig()
	if err != nil {
		return err
	}
	if err := observeLifecycleServing(ctx, config, e.writer.store.kube, state, e.target, ObserveServingCertificate); err != nil {
		return err
	}
	return e.writer.check(ctx)
}
