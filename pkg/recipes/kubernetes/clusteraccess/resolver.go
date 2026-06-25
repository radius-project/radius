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

// Package clusteraccess resolves, per recipe execution, which Kubernetes cluster
// a recipe should deploy to and how to authenticate to it. It returns an
// in-memory *rest.Config that the recipe engines consume instead of assuming the
// in-cluster config.
//
// The resolver dispatches to an ordered list of strategies. The first strategy
// whose appliesTo reports true is used. Strategy precedence is:
//
//  1. injectedKubeconfigStrategy — a kubeconfig supplied out-of-band via
//     RADIUS_TARGET_KUBECONFIG (the Repo Radius workflow contract).
//  2. localStrategy — the control-plane cluster (in-cluster config, falling back
//     to the local kubeconfig when not running in-cluster). This is the default
//     and matches today's behavior.
//
// Cloud-derived strategies (EKS/AKS) are added later behind the same seam.
package clusteraccess

import (
	"context"

	"github.com/radius-project/radius/pkg/recipes"
	"k8s.io/client-go/rest"
)

// KubeconfigSource describes how a kubeconfig-based consumer (notably the
// Terraform kubernetes provider, which reads a kubeconfig path natively) should
// reach the target cluster.
//
// A Path-based source is preferred over an in-memory *rest.Config for consumers
// that read a kubeconfig file directly: it keeps any bearer token in the file
// the workflow already mounted rather than copying it into generated config, and
// it preserves exec-plugin / client-certificate kubeconfigs that a *rest.Config
// round-trip would lose.
type KubeconfigSource struct {
	// Path is the kubeconfig file path to use. An empty Path means the in-cluster
	// config should be used (no explicit kubeconfig file).
	Path string
}

// ClusterAccessResolver returns the Kubernetes cluster a recipe execution
// targets and how to authenticate to it. The returned config and any embedded
// credentials are scoped to a single recipe execution and must not be persisted.
type ClusterAccessResolver interface {
	// Resolve returns a *rest.Config for the cluster targeted by this execution.
	// When nothing names an external cluster, it returns the control-plane
	// (in-cluster / local kubeconfig) config.
	Resolve(ctx context.Context, envConfig *recipes.Configuration) (*rest.Config, error)

	// ResolveKubeconfigSource returns a KubeconfigSource for the cluster targeted
	// by this execution, for consumers that read a kubeconfig path natively (the
	// Terraform kubernetes provider). When nothing names an external cluster, it
	// describes the control-plane (in-cluster / local kubeconfig) cluster.
	ResolveKubeconfigSource(ctx context.Context, envConfig *recipes.Configuration) (KubeconfigSource, error)
}

// clusterStrategy resolves access to one kind of target cluster.
type clusterStrategy interface {
	// appliesTo reports whether this strategy handles the current execution.
	appliesTo(envConfig *recipes.Configuration) bool
	// restConfig builds an in-memory *rest.Config for the target cluster.
	restConfig(ctx context.Context, envConfig *recipes.Configuration) (*rest.Config, error)
	// kubeconfigSource describes the target cluster as a kubeconfig path (or the
	// in-cluster config) for consumers that read a kubeconfig file directly.
	kubeconfigSource(ctx context.Context, envConfig *recipes.Configuration) (KubeconfigSource, error)
}

// resolver is the default ClusterAccessResolver. It evaluates strategies in
// order and uses the first one that applies.
type resolver struct {
	strategies []clusterStrategy
}

var _ ClusterAccessResolver = (*resolver)(nil)

// NewResolver creates a ClusterAccessResolver with the default strategy set:
// injected kubeconfig first, then the local (control-plane) cluster.
func NewResolver() ClusterAccessResolver {
	return &resolver{
		strategies: []clusterStrategy{
			newInjectedKubeconfigStrategy(),
			newLocalStrategy(),
		},
	}
}

// Resolve returns the *rest.Config from the first strategy that applies to
// envConfig. localStrategy always applies, so Resolve never falls through.
func (r *resolver) Resolve(ctx context.Context, envConfig *recipes.Configuration) (*rest.Config, error) {
	for _, s := range r.strategies {
		if s.appliesTo(envConfig) {
			return s.restConfig(ctx, envConfig)
		}
	}

	// Unreachable in practice: localStrategy.appliesTo always returns true.
	return nil, errNoStrategy
}

// ResolveKubeconfigSource returns the KubeconfigSource from the first strategy
// that applies to envConfig. localStrategy always applies, so it never falls
// through.
func (r *resolver) ResolveKubeconfigSource(ctx context.Context, envConfig *recipes.Configuration) (KubeconfigSource, error) {
	for _, s := range r.strategies {
		if s.appliesTo(envConfig) {
			return s.kubeconfigSource(ctx, envConfig)
		}
	}

	// Unreachable in practice: localStrategy.appliesTo always returns true.
	return KubeconfigSource{}, errNoStrategy
}
