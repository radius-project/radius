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

package backends

import (
	"crypto/sha1"
	"errors"
	"fmt"
	"strings"

	"github.com/project-radius/radius/pkg/recipes"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

var _ Backend = (*kubernetesBackend)(nil)

type kubernetesBackend struct{}

const (
	RadiusNamespace   = "radius-system"
	BackendKubernetes = "kubernetes"
)

func NewKubernetesBackend() Backend {
	return &kubernetesBackend{}
}

// BuildBackend generates the Terraform backend configuration for Kubernetes backend.
// It returns an error if the in cluster config cannot be retrieved, and uses default kubeconfig file if
// in-cluster config is not present.
// https://developer.hashicorp.com/terraform/language/settings/backends/kubernetes
func (p *kubernetesBackend) BuildBackend(resourceRecipe *recipes.ResourceMetadata) (map[string]any, error) {
	secretSuffix, err := generateSecretSuffix(resourceRecipe)
	if err != nil {
		return nil, err
	}
	return generateKubernetesBackendConfig(secretSuffix)
}

// generateSecretSuffix returns a unique string from the resourceID, environmentID, and applicationID
// which is used as key for kubernetes secret in defining terraform backend.
func generateSecretSuffix(resourceRecipe *recipes.ResourceMetadata) (string, error) {
	parsedResourceID, err := resources.Parse(resourceRecipe.ResourceID)
	if err != nil {
		return "", err
	}

	parsedEnvID, err := resources.Parse(resourceRecipe.EnvironmentID)
	if err != nil {
		return "", err
	}

	parsedAppID, err := resources.Parse(resourceRecipe.ApplicationID)
	if err != nil {
		return "", err
	}

	prefix := fmt.Sprintf("%s-%s-%s", parsedEnvID.Name(), parsedAppID.Name(), parsedResourceID.Name())

	// Kubernetes enforces a character limit of 63 characters on the suffix for state file stored in kubernetes secret.
	// 22 = 63 (max length of Kubernetes secret suffix) - 40 (hex hash length) - 1 (dot separator)
	maxResourceNameLen := 22
	if len(prefix) >= maxResourceNameLen {
		prefix = prefix[:maxResourceNameLen]
	}

	hasher := sha1.New()
	_, err = hasher.Write([]byte(strings.ToLower(fmt.Sprintf("%s-%s-%s", parsedEnvID.Name(), parsedAppID.Name(), parsedResourceID.String()))))
	if err != nil {
		return "", err
	}
	hash := hasher.Sum(nil)

	// example: env-app-redis.ec291e26078b7ea8a74abfac82530005a0ecbf15
	return fmt.Sprintf("%s.%x", prefix, hash), nil
}

// generateKubernetesBackendConfig returns Terraform backend configuration to store Terraform state file for the deployment.
// Currently, the supported backend for Terraform Recipes is Kubernetes secret. https://developer.hashicorp.com/terraform/language/settings/backends/kubernetes
func generateKubernetesBackendConfig(secretSuffix string) (map[string]interface{}, error) {
	backend := map[string]interface{}{
		BackendKubernetes: map[string]interface{}{
			"secret_suffix": secretSuffix,
			"namespace":     RadiusNamespace,
		},
	}

	_, err := rest.InClusterConfig()
	if err != nil {
		// If in cluster config is not present, then use default kubeconfig file.
		if errors.Is(err, rest.ErrNotInCluster) {
			if value, found := backend[BackendKubernetes]; found {
				backendValue := value.(map[string]interface{})
				backendValue["config_path"] = clientcmd.RecommendedHomeFile
			}
		} else {
			return nil, err
		}
	} else {
		if value, found := backend[BackendKubernetes]; found {
			backendValue := value.(map[string]interface{})
			backendValue["in_cluster_config"] = true
		}
	}

	return backend, nil
}
