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

package kubernetes

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/radius-project/radius/pkg/kubeutil"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
)

// loadCLIConfig loads the kubeconfig for the rad CLI. Like kubectl and Helm, it merges the files listed in KUBECONFIG,
// or it loads ~/.kube/config when KUBECONFIG is not set, and it skips missing files. It returns an error when the
// merged kubeconfig is empty.
func loadCLIConfig() (*api.Config, error) {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	cfg, err := rules.Load()
	if err != nil {
		return nil, err
	}

	if api.IsConfigEmpty(cfg) {
		return nil, noUsableKubeconfigError(rules)
	}

	return cfg, nil
}

// NewCLIClientConfig builds a Kubernetes client config for the rad CLI with the CLI QPS and Burst defaults. It selects
// the kubeconfig files the same way as loadCLIConfig and uses contextName, or the current context when contextName is
// empty.
//
// Inside a pod, client-go uses the pod's service account when the only problem with the kubeconfig is that no cluster
// server is selected. That includes an empty kubeconfig and a kubeconfig with no current context. A kubeconfig that
// selects a cluster server always wins. Helm does the same, so rad and Helm use the same cluster and credentials.
// Outside a pod, Helm falls back to http://localhost:8080 in those cases, and NewCLIClientConfig returns an error.
func NewCLIClientConfig(contextName string) (*rest.Config, error) {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	config, err := newCLIClientConfigLoader(rules, contextName).ClientConfig()
	if clientcmd.IsEmptyConfig(err) {
		// Keep client-go's error when kubeconfig files exist but no context is selected.
		if raw, loadErr := rules.Load(); loadErr == nil && api.IsConfigEmpty(raw) {
			err = noUsableKubeconfigError(rules)
		}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to initialize Kubernetes client config: %w", err)
	}

	kubeutil.ApplyQPSAndBurst(config, kubeutil.DefaultCLIQPS, kubeutil.DefaultCLIBurst)
	return config, nil
}

// newCLIClientConfigLoader resolves kubeconfig files and contexts the same way as kubectl and Helm, including the
// service account fallback that NewCLIClientConfig describes.
func newCLIClientConfigLoader(rules *clientcmd.ClientConfigLoadingRules, contextName string) clientcmd.ClientConfig {
	return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, &clientcmd.ConfigOverrides{CurrentContext: contextName})
}

func noUsableKubeconfigError(rules *clientcmd.ClientConfigLoadingRules) error {
	return fmt.Errorf("no usable kubeconfig found (checked: %s)", strings.Join(rules.GetLoadingPrecedence(), string(filepath.ListSeparator)))
}
