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
		return inventory, errCertificateObserverUnavailable
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
