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

package clusteraccess

import (
	"context"
	"fmt"
	"os"

	"github.com/radius-project/radius/pkg/kubeutil"
	"github.com/radius-project/radius/pkg/recipes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// TargetKubeconfigEnvVar is the environment variable that points at a kubeconfig
// for an external target cluster. The Repo Radius deploy workflow mounts a
// kubeconfig into the Radius RP pods and sets this variable; honoring it is the
// v1 multi-cluster contract that Radius owns. It is defined canonically in
// kubeutil so the recipe path and the direct-resource path share one source.
const TargetKubeconfigEnvVar = kubeutil.TargetKubeconfigEnvVar

// injectedKubeconfigStrategy resolves the cluster described by the kubeconfig at
// the path in RADIUS_TARGET_KUBECONFIG. Radius does not create or own this
// kubeconfig; it is supplied (and refreshed) out-of-band by the workflow.
//
// getenv and loadConfig are fields so tests can substitute them.
type injectedKubeconfigStrategy struct {
	getenv     func(string) string
	loadConfig func(path string) (*rest.Config, error)
}

var _ clusterStrategy = (*injectedKubeconfigStrategy)(nil)

// newInjectedKubeconfigStrategy creates an injectedKubeconfigStrategy backed by
// the process environment and a clientcmd-based loader.
func newInjectedKubeconfigStrategy() *injectedKubeconfigStrategy {
	return &injectedKubeconfigStrategy{
		getenv: os.Getenv,
		loadConfig: func(path string) (*rest.Config, error) {
			return clientcmd.BuildConfigFromFlags("", path)
		},
	}
}

// appliesTo reports whether RADIUS_TARGET_KUBECONFIG is set to a non-empty path.
func (s *injectedKubeconfigStrategy) appliesTo(_ *recipes.Configuration) bool {
	return s.getenv(TargetKubeconfigEnvVar) != ""
}

// restConfig loads the kubeconfig at RADIUS_TARGET_KUBECONFIG. It returns an
// error (rather than silently falling back to the control-plane cluster) when
// the kubeconfig is missing or unreadable, so a misconfigured external target
// fails loudly.
func (s *injectedKubeconfigStrategy) restConfig(_ context.Context, _ *recipes.Configuration) (*rest.Config, error) {
	path := s.getenv(TargetKubeconfigEnvVar)
	config, err := s.loadConfig(path)
	if err != nil {
		return nil, fmt.Errorf("failed to load target kubeconfig from %s=%q: %w", TargetKubeconfigEnvVar, path, err)
	}

	return config, nil
}

// kubeconfigSource returns the path from RADIUS_TARGET_KUBECONFIG so a
// kubeconfig-path consumer (the Terraform kubernetes provider) targets the same
// injected kubeconfig without Radius copying its bearer token into generated
// configuration.
func (s *injectedKubeconfigStrategy) kubeconfigSource(_ context.Context, _ *recipes.Configuration) (KubeconfigSource, error) {
	return KubeconfigSource{Path: s.getenv(TargetKubeconfigEnvVar)}, nil
}
