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

package providers

import (
	"context"

	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/recipes/kubernetes/clusteraccess"
)

const (
	KubernetesProviderName = "kubernetes"
)

var _ Provider = (*kubernetesProvider)(nil)

// kubernetesProvider builds the Terraform kubernetes provider configuration. It
// delegates the target-cluster decision to a clusteraccess.ClusterAccessResolver
// so the injected-kubeconfig (multi-cluster v1) and in-cluster/local fallback
// logic lives in one place.
type kubernetesProvider struct {
	resolver clusteraccess.ClusterAccessResolver
}

// newKubernetesProvider creates a kubernetesProvider backed by resolver.
func newKubernetesProvider(resolver clusteraccess.ClusterAccessResolver) *kubernetesProvider {
	return &kubernetesProvider{resolver: resolver}
}

// BuildConfig generates the Terraform provider configuration for the Kubernetes provider.
//
// It asks the cluster access resolver where the recipe should deploy. When
// RADIUS_TARGET_KUBECONFIG is set (the multi-cluster v1 contract), the provider
// is pointed at that kubeconfig so the recipe deploys to the external target
// cluster. Otherwise it uses the in-cluster config, or the default kubeconfig
// file when not running in-cluster.
//
// Note: the Terraform state backend is intentionally not affected by
// RADIUS_TARGET_KUBECONFIG — state stays on the control-plane cluster. Only the
// provider (where workloads are created) targets the external cluster.
// https://registry.terraform.io/providers/hashicorp/kubernetes/latest/docs#in-cluster-config
func (p *kubernetesProvider) BuildConfig(ctx context.Context, envConfig *recipes.Configuration) (map[string]any, error) {
	source, err := p.resolver.ResolveKubeconfigSource(ctx, envConfig)
	if err != nil {
		return nil, err
	}

	// An empty Path means the in-cluster config is used; the kubernetes provider
	// detects that natively, so no additional config is needed.
	// https://registry.terraform.io/providers/hashicorp/kubernetes/latest/docs#in-cluster-config
	if source.Path == "" {
		return nil, nil
	}

	return map[string]any{
		"config_path": source.Path,
	}, nil
}
