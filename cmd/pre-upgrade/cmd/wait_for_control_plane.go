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

package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"github.com/spf13/cobra"
	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const (
	radiusAggregatedAPIPath = "/apis/api.ucp.dev/v1alpha3"

	defaultControlPlaneReadinessTimeout      = 60 * time.Second
	defaultControlPlaneReadinessPollInterval = 2 * time.Second
	defaultControlPlaneRequestTimeout        = 10 * time.Second
	authorizationFailureThreshold            = 3
)

type waitForControlPlaneOptions struct {
	timeout        time.Duration
	pollInterval   time.Duration
	requestTimeout time.Duration
	namespace      string
	deployments    []string
}

type controlPlaneProbe func(context.Context) error

func newWaitForControlPlaneCommand() *cobra.Command {
	options := waitForControlPlaneOptions{
		timeout:        defaultControlPlaneReadinessTimeout,
		pollInterval:   defaultControlPlaneReadinessPollInterval,
		requestTimeout: defaultControlPlaneRequestTimeout,
	}

	cmd := &cobra.Command{
		Use:   "wait-for-control-plane",
		Short: "Wait for the Radius aggregated API, and optionally core Deployments, to become available",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := options.validate(); err != nil {
				return err
			}

			logger, flush, err := newLogger()
			if err != nil {
				return fmt.Errorf("failed to initialize logger: %w", err)
			}
			defer flush()

			client, err := newInClusterClient()
			if err != nil {
				return err
			}

			return waitForControlPlane(cmd.Context(), options, logger, func(ctx context.Context) error {
				if err := probeControlPlane(ctx, client.Discovery().RESTClient(), options.requestTimeout); err != nil {
					return err
				}
				return probeDeployments(ctx, client, options.namespace, options.deployments, options.requestTimeout)
			})
		},
	}

	cmd.Flags().DurationVar(&options.timeout, "timeout", options.timeout, "Maximum time to wait for the Radius aggregated API")
	cmd.Flags().DurationVar(&options.pollInterval, "poll-interval", options.pollInterval, "Delay between readiness probes")
	cmd.Flags().DurationVar(&options.requestTimeout, "request-timeout", options.requestTimeout, "Timeout for each readiness probe")
	cmd.Flags().StringVar(&options.namespace, "namespace", "", "Namespace of the Deployments passed with --deployment")
	cmd.Flags().StringArrayVar(&options.deployments, "deployment", nil, "Deployment that must also be available (repeatable; requires --namespace)")
	return cmd
}

func (o waitForControlPlaneOptions) validate() error {
	switch {
	case o.timeout <= 0:
		return fmt.Errorf("timeout must be greater than zero")
	case o.pollInterval <= 0:
		return fmt.Errorf("poll interval must be greater than zero")
	case o.requestTimeout <= 0:
		return fmt.Errorf("request timeout must be greater than zero")
	case len(o.deployments) > 0 && o.namespace == "":
		return fmt.Errorf("namespace is required when deployments are specified")
	default:
		return nil
	}
}

func newInClusterClient() (kubernetes.Interface, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load in-cluster Kubernetes configuration: %w", err)
	}

	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes client: %w", err)
	}
	return client, nil
}

func probeControlPlane(ctx context.Context, client rest.Interface, requestTimeout time.Duration) error {
	probeCtx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	return client.Get().AbsPath(radiusAggregatedAPIPath).Do(probeCtx).Error()
}

// probeDeployments returns an error unless every named Deployment has finished rolling out
// its current generation and all of its desired replicas are available.
func probeDeployments(ctx context.Context, client kubernetes.Interface, namespace string, names []string, requestTimeout time.Duration) error {
	for _, name := range names {
		probeCtx, cancel := context.WithTimeout(ctx, requestTimeout)
		deployment, err := client.AppsV1().Deployments(namespace).Get(probeCtx, name, metav1.GetOptions{})
		cancel()
		if err != nil {
			return err
		}
		if err := deploymentAvailable(deployment); err != nil {
			return err
		}
	}
	return nil
}

func deploymentAvailable(deployment *appsv1.Deployment) error {
	desired := int32(1)
	if deployment.Spec.Replicas != nil {
		desired = *deployment.Spec.Replicas
	}
	status := deployment.Status
	switch {
	case status.ObservedGeneration < deployment.Generation:
		return fmt.Errorf("deployment %q has not observed generation %d", deployment.Name, deployment.Generation)
	case status.UpdatedReplicas < desired:
		return fmt.Errorf("deployment %q has %d of %d replicas updated", deployment.Name, status.UpdatedReplicas, desired)
	case status.AvailableReplicas < desired:
		return fmt.Errorf("deployment %q has %d of %d replicas available", deployment.Name, status.AvailableReplicas, desired)
	default:
		return nil
	}
}

func waitForControlPlane(ctx context.Context, options waitForControlPlaneOptions, logger logr.Logger, probe controlPlaneProbe) error {
	waitCtx, cancel := context.WithTimeout(ctx, options.timeout)
	defer cancel()

	started := time.Now()
	attempts := 0
	authorizationFailures := 0
	var lastErr error

	for {
		attempts++
		lastErr = probe(waitCtx)
		if lastErr == nil {
			logger.Info("Radius control plane is available", "attempts", attempts, "elapsed", time.Since(started))
			return nil
		}

		if apierrors.IsUnauthorized(lastErr) || apierrors.IsForbidden(lastErr) {
			authorizationFailures++
		} else {
			authorizationFailures = 0
		}
		if authorizationFailures >= authorizationFailureThreshold {
			return fmt.Errorf("control plane readiness probe failed with a non-retryable authorization error after %d attempt(s): %w", attempts, lastErr)
		}

		logger.Info("Waiting for the Radius control plane", "attempt", attempts, "error", lastErr)

		timer := time.NewTimer(options.pollInterval)
		select {
		case <-timer.C:
		case <-waitCtx.Done():
			timer.Stop()
			return fmt.Errorf("Radius control plane did not become available within %s after %d attempt(s): %w (last probe failure: %v)",
				options.timeout, attempts, waitCtx.Err(), lastErr)
		}
	}
}
