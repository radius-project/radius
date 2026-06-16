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
	"errors"
	"os"

	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/recipes/kubernetes/clusteraccess"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	KubernetesProviderName = "kubernetes"
)

var _ Provider = (*kubernetesProvider)(nil)

type kubernetesProvider struct{}

// BuildConfig generates the Terraform provider configuration for the Kubernetes provider.
//
// When RADIUS_TARGET_KUBECONFIG is set (the multi-cluster v1 contract), the
// provider is pointed at that kubeconfig so the recipe deploys to the external
// target cluster. Otherwise it falls back to the in-cluster config, or the
// default kubeconfig file when not running in-cluster.
//
// Note: the Terraform state backend is intentionally not affected by
// RADIUS_TARGET_KUBECONFIG — state stays on the control-plane cluster. Only the
// provider (where workloads are created) targets the external cluster.
// https://registry.terraform.io/providers/hashicorp/kubernetes/latest/docs#in-cluster-config
func (p *kubernetesProvider) BuildConfig(ctx context.Context, envConfig *recipes.Configuration) (map[string]any, error) {
	// Honor the injected target kubeconfig first (multi-cluster v1 contract).
	if targetKubeconfig := os.Getenv(clusteraccess.TargetKubeconfigEnvVar); targetKubeconfig != "" {
		return map[string]any{
			"config_path": targetKubeconfig,
		}, nil
	}

	_, err := rest.InClusterConfig()
	if err != nil {
		// If in cluster config is not present, then use default kubeconfig file.
		if errors.Is(err, rest.ErrNotInCluster) {
			return map[string]any{
				"config_path": clientcmd.RecommendedHomeFile,
			}, nil
		}

		return nil, err
	}

	// No additional config is needed if in cluster config is present.
	// https://registry.terraform.io/providers/hashicorp/kubernetes/latest/docs#in-cluster-config
	return nil, nil
}
