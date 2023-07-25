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

	"github.com/project-radius/radius/pkg/recipes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	KubernetesProviderName = "kubernetes"
)

type kubernetesProvider struct{}

// # Function Explanation
//
// NewKubernetesProvider creates a new KubernetesProvider instance.
func NewKubernetesProvider() Provider {
	return &kubernetesProvider{}
}

// # Function Explanation
//
// BuildKubernetesProviderConfig generates the Terraform provider configuration for Kubernetes provider.
// It returns an error if the in cluster config is not present.
// https://registry.terraform.io/providers/hashicorp/azurerm/latest/docs
func (p *kubernetesProvider) BuildConfig(ctx context.Context, envConfig *recipes.Configuration) (map[string]any, error) {
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
