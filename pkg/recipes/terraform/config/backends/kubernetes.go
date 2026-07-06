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
	"errors"
	"fmt"
	"strings"

	"github.com/radius-project/radius/pkg/hashutil"
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
	secretSuffix, err := p.resolveSecretSuffix(resourceRecipe)
	if err != nil {
		return nil, err
	}

	return generateKubernetesBackendConfig(secretSuffix)
}

// resolveSecretSuffix returns the secret suffix to use for the Terraform state backend.
//
// It prefers the current (SHA-256) suffix, but falls back to the legacy (SHA-1) suffix when a
// Terraform state secret already exists under the legacy name (written by an older version of
// Radius). New deployments use the current suffix. This keeps the SHA-1 -> SHA-256 migration from
// losing existing Terraform state. See https://github.com/radius-project/radius/issues/8084.
func (p *kubernetesBackend) resolveSecretSuffix(resourceRecipe *recipes.ResourceMetadata) (string, error) {
	currentSuffix, err := generateSecretSuffix(resourceRecipe)
	if err != nil {
		return "", err
	}

	// Some callers only need the computed suffix and do not provide a Kubernetes client (for example
	// test tooling). Without a client we cannot check for an existing state secret, so use the current
	// suffix.
	if p.k8sClientSet == nil {
		return currentSuffix, nil
	}

	// A background context is sufficient here: this is a single, short-lived existence check used to
	// pick the correct (current vs legacy) state secret name before Terraform runs.
	ctx := context.Background()

	exists, err := p.ValidateBackendExists(ctx, KubernetesBackendNamePrefix+currentSuffix)
	if err != nil {
		return "", err
	}
	if exists {
		return currentSuffix, nil
	}

	legacySuffix, err := generateLegacySecretSuffix(resourceRecipe)
	if err != nil {
		return "", err
	}

	exists, err = p.ValidateBackendExists(ctx, KubernetesBackendNamePrefix+legacySuffix)
	if err != nil {
		return "", err
	}
	if exists {
		return legacySuffix, nil
	}

	return currentSuffix, nil
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

// secretSuffixLength is the number of hexadecimal characters of the hash used for the Terraform
// state secret suffix. The Terraform Kubernetes backend stores secret_suffix as a Kubernetes label
// value (limited to 63 characters), so the SHA-256 hash (64 hex characters) is truncated. 40
// characters (160 bits) matches the legacy SHA-1 width and keeps ample collision resistance.
const secretSuffixLength = 40

// generateSecretSuffix returns a unique string from the resourceID, environmentID, and applicationID
// which is used as key for kubernetes secret in defining terraform backend.
func generateSecretSuffix(resourceRecipe *recipes.ResourceMetadata) (string, error) {
	input, err := secretSuffixInput(resourceRecipe)
	if err != nil {
		return "", err
	}

	return hashutil.Hex([]byte(input))[:secretSuffixLength], nil
}

// generateLegacySecretSuffix returns the legacy SHA-1 based secret suffix.
//
// SHA-1 is retained only to locate Terraform state secrets created by older versions of Radius
// during the migration to SHA-256. Use generateSecretSuffix for new values. See
// https://github.com/radius-project/radius/issues/8084.
func generateLegacySecretSuffix(resourceRecipe *recipes.ResourceMetadata) (string, error) {
	input, err := secretSuffixInput(resourceRecipe)
	if err != nil {
		return "", err
	}

	return hashutil.LegacyHex([]byte(input)), nil
}

// secretSuffixInput returns the deterministic input string that the Terraform state secret suffix is
// derived from for the given recipe.
func secretSuffixInput(resourceRecipe *recipes.ResourceMetadata) (string, error) {
	parsedResourceID, err := resources.Parse(resourceRecipe.ResourceID)
	if err != nil {
		return "", err
	}

	parsedEnvID, err := resources.Parse(resourceRecipe.EnvironmentID)
	if err != nil {
		return "", err
	}

	appName := ""
	if resourceRecipe.ApplicationID != "" {
		parsedAppID, err := resources.Parse(resourceRecipe.ApplicationID)
		if err != nil {
			return "", err
		}
		appName = parsedAppID.Name()
	}

	if appName != "" {
		return strings.ToLower(fmt.Sprintf("%s-%s-%s", parsedEnvID.Name(), appName, parsedResourceID.String())), nil
	}

	return strings.ToLower(fmt.Sprintf("%s-%s", parsedEnvID.Name(), parsedResourceID.String())), nil
}

// generateKubernetesBackendConfig returns Terraform backend configuration to store Terraform state file for the deployment.
// Currently, the supported backend for Terraform Recipes is Kubernetes secret. https://developer.hashicorp.com/terraform/language/settings/backends/kubernetes
func generateKubernetesBackendConfig(secretSuffix string) (map[string]any, error) {
	backend := map[string]any{
		BackendKubernetes: map[string]any{
			"secret_suffix": secretSuffix,
			"namespace":     RadiusNamespace,
		},
	}

	_, err := rest.InClusterConfig()
	if err != nil {
		// If in cluster config is not present, then use default kubeconfig file.
		if errors.Is(err, rest.ErrNotInCluster) {
			if value, found := backend[BackendKubernetes]; found {
				backendValue := value.(map[string]any)
				backendValue["config_path"] = clientcmd.RecommendedHomeFile
			}
		} else {
			return nil, err
		}
	} else {
		if value, found := backend[BackendKubernetes]; found {
			backendValue := value.(map[string]any)
			backendValue["in_cluster_config"] = true
		}
	}

	return backend, nil
}
