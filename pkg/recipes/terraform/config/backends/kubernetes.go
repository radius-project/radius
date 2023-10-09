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
	"context"
	"crypto/sha1"
	"errors"
	"fmt"
	"strings"

	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

var _ Backend = (*kubernetesBackend)(nil)

const (
	RadiusNamespace   = "radius-system"
	BackendKubernetes = "kubernetes"

	// KubernetesBackendNamePrefix is the default prefix added by Terraform to the generated Kubernetes secret name.
	// Terraform generates the secret name in the format "tfstate-{workspace}-{secret_suffix}". Default terraform workspace
	// is used for recipes deployment.
	// https://developer.hashicorp.com/terraform/language/settings/backends/kubernetes
	// https://developer.hashicorp.com/terraform/language/state/workspaces
	KubernetesBackendNamePrefix = "tfstate-default-"
)

var _ Backend = (*kubernetesBackend)(nil)

type kubernetesBackend struct {
	k8sClientSet kubernetes.Interface
}

func NewKubernetesBackend(k8sClientSet kubernetes.Interface) Backend {
	return &kubernetesBackend{k8sClientSet: k8sClientSet}
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

// ValidateBackendExists checks if the Kubernetes secret for Terraform state file exists.
// name is the name of the backend Kubernetes secret resource that is created as a part of terraform apply
// during recipe deployment.
func (p *kubernetesBackend) ValidateBackendExists(ctx context.Context, name string) (bool, error) {
	logger := ucplog.FromContextOrDiscard(ctx)

	_, err := p.k8sClientSet.CoreV1().Secrets(RadiusNamespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if k8s_errors.IsNotFound(err) {
			logger.Info(fmt.Sprintf("Kubernetes secret %q does not exist: %s", name, err.Error()))
			return false, nil
		}

		return false, err
	}

	return true, nil
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

	hasher := sha1.New()
	_, err = hasher.Write([]byte(strings.ToLower(fmt.Sprintf("%s-%s-%s", parsedEnvID.Name(), parsedAppID.Name(), parsedResourceID.String()))))
	if err != nil {
		return "", err
	}
	hash := hasher.Sum(nil)

	return fmt.Sprintf("%x", hash), nil
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
