/*
Copyright 2023 The Radius Authors.

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

// Package controlplane scales the Radius control-plane deployments up and down.
//
// It exists to support restoring state into a running control plane. The PostgreSQL-backed
// resource providers open pgx connection pools at startup and cache prepared statements per
// connection. Restoring a pg_dump (which uses DROP TABLE / CREATE TABLE) underneath those live
// connections invalidates the cached statements and races the providers' own writes. Scaling the
// providers to zero before restore — and back up afterward — makes the restore atomic with
// respect to its consumers and avoids stale prepared-statement errors, without requiring a
// separate "restart" step.
package controlplane

import (
	"context"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	k8s "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"

	"github.com/radius-project/radius/pkg/cli/kubernetes"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
)

// Deployments lists the control-plane deployments that hold PostgreSQL connection pools and must
// therefore be quiesced while state is restored. Other components (the deployment engine, the
// dashboard) do not connect to the database directly and are left running.
var Deployments = []string{"ucp", "applications-rp", "dynamic-rp"}

const (
	// scaleTimeout bounds how long to wait for a scale operation to converge.
	scaleTimeout = 2 * time.Minute

	// scalePollInterval is how often deployment status is polled while waiting.
	scalePollInterval = 2 * time.Second
)

// Scaler scales the control-plane deployments in a namespace.
type Scaler struct {
	clientset k8s.Interface
	namespace string
}

// NewScaler creates a Scaler backed by the supplied Kubernetes clientset and namespace.
func NewScaler(clientset k8s.Interface, namespace string) *Scaler {
	return &Scaler{clientset: clientset, namespace: namespace}
}

// NewScalerForContext builds a Scaler from a kubeconfig context name, targeting the given
// namespace.
func NewScalerForContext(kubeContext, namespace string) (*Scaler, error) {
	clientset, _, err := kubernetes.NewClientset(kubeContext)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes client: %w", err)
	}
	return NewScaler(clientset, namespace), nil
}

// ScaleDown scales every control-plane deployment to zero replicas and waits until their pods are
// gone. It returns the previous replica counts keyed by deployment name so they can be restored by
// ScaleUp. Deployments that are not present are skipped (a partial install is not an error).
func (s *Scaler) ScaleDown(ctx context.Context) (map[string]int32, error) {
	logger := ucplog.FromContextOrDiscard(ctx)
	saved := make(map[string]int32, len(Deployments))

	for _, name := range Deployments {
		deployment, err := s.clientset.AppsV1().Deployments(s.namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			if k8serrors.IsNotFound(err) {
				logger.Info("Control-plane deployment not found, skipping", "deployment", name)
				continue
			}
			return saved, fmt.Errorf("failed to read deployment %q: %w", name, err)
		}

		saved[name] = replicasOf(deployment)

		logger.Info("Scaling down control-plane deployment", "deployment", name)
		if err := s.setReplicas(ctx, name, 0); err != nil {
			return saved, err
		}
	}

	for name := range saved {
		if err := s.waitForReplicas(ctx, name, func(d *appsv1.Deployment) bool {
			return d.Status.Replicas == 0
		}); err != nil {
			return saved, fmt.Errorf("timed out waiting for deployment %q to scale down: %w", name, err)
		}
	}

	return saved, nil
}

// ScaleUp restores each deployment to the replica count captured by ScaleDown and waits until the
// deployments report that many available replicas, so the control plane is serving again before
// the command returns.
func (s *Scaler) ScaleUp(ctx context.Context, saved map[string]int32) error {
	logger := ucplog.FromContextOrDiscard(ctx)

	for name, replicas := range saved {
		logger.Info("Scaling up control-plane deployment", "deployment", name, "replicas", replicas)
		if err := s.setReplicas(ctx, name, replicas); err != nil {
			return err
		}
	}

	for name, replicas := range saved {
		want := replicas
		if err := s.waitForReplicas(ctx, name, func(d *appsv1.Deployment) bool {
			return d.Status.AvailableReplicas >= want
		}); err != nil {
			return fmt.Errorf("timed out waiting for deployment %q to scale up: %w", name, err)
		}
	}

	return nil
}

// setReplicas sets the replica count of a deployment, retrying on optimistic-concurrency conflicts.
func (s *Scaler) setReplicas(ctx context.Context, name string, replicas int32) error {
	deployments := s.clientset.AppsV1().Deployments(s.namespace)
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		deployment, getErr := deployments.Get(ctx, name, metav1.GetOptions{})
		if getErr != nil {
			return getErr
		}
		deployment.Spec.Replicas = &replicas
		_, updateErr := deployments.Update(ctx, deployment, metav1.UpdateOptions{})
		return updateErr
	})
	if err != nil {
		return fmt.Errorf("failed to scale deployment %q to %d: %w", name, replicas, err)
	}
	return nil
}

// waitForReplicas polls the deployment until cond is satisfied or the timeout elapses.
func (s *Scaler) waitForReplicas(ctx context.Context, name string, cond func(*appsv1.Deployment) bool) error {
	return wait.PollUntilContextTimeout(ctx, scalePollInterval, scaleTimeout, true, func(ctx context.Context) (bool, error) {
		deployment, err := s.clientset.AppsV1().Deployments(s.namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		return cond(deployment), nil
	})
}

// replicasOf returns the configured replica count of a deployment, defaulting to 1 when unset
// (the Kubernetes default) so a deployment is never accidentally restored to zero replicas.
func replicasOf(deployment *appsv1.Deployment) int32 {
	if deployment.Spec.Replicas == nil {
		return 1
	}
	if *deployment.Spec.Replicas == 0 {
		return 1
	}
	return *deployment.Spec.Replicas
}
