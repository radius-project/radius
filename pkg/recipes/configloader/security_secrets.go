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

package configloader

import (
	"context"
	"fmt"
	"strings"

	aztoken "github.com/radius-project/radius/pkg/azure/tokencredentials"
	"github.com/radius-project/radius/pkg/cli/clients_new/generated"
	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/ucp/resources"
	resources_kubernetes "github.com/radius-project/radius/pkg/ucp/resources/kubernetes"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// defaultSecuritySecretKind is the secret kind assumed when a Radius.Security/secrets resource does not set
// `properties.kind`. It mirrors the schema default for the resource type.
const defaultSecuritySecretKind = "generic"

// loadSecuritySecret retrieves secret data for a Radius.Security/secrets resource.
//
// Radius.Security/secrets is a dynamic resource whose sensitive `data.*.value` fields are redacted from the
// Radius database once provisioning succeeds. The plaintext lives only in the Kubernetes Secret that the
// resource's recipe materializes in the environment namespace. This reader therefore fetches the resource to
// learn its `kind` and locate its backing Kubernetes Secret output resource, then reads the secret values
// directly from Kubernetes.
func (e *secretsLoader) loadSecuritySecret(ctx context.Context, secretResourceID resources.ID, secretKeysFilter []string) (recipes.SecretData, error) {
	if e.KubernetesProvider == nil {
		return recipes.SecretData{}, fmt.Errorf("kubernetes client is not configured; cannot read Radius.Security/secrets resource '%s'", secretResourceID.String())
	}

	client, err := generated.NewGenericResourcesClient(secretResourceID.Type(), secretResourceID.RootScope(), &aztoken.AnonymousCredential{}, e.ArmClientOptions)
	if err != nil {
		return recipes.SecretData{}, err
	}

	resource, err := client.Get(ctx, secretResourceID.Name(), nil)
	if err != nil {
		return recipes.SecretData{}, err
	}

	if resource.Properties == nil {
		return recipes.SecretData{}, fmt.Errorf("secret resource '%s' has no properties", secretResourceID.String())
	}

	// The secret kind maps to the recipes.SecretData type and is consumed, for Bicep registry authentication,
	// by the registry auth client. It is not redacted, so it is read from the resource properties.
	kind := defaultSecuritySecretKind
	if k, ok := resource.Properties["kind"].(string); ok && k != "" {
		kind = k
	}

	namespace, name, err := findKubernetesSecretOutputResource(resource.Properties)
	if err != nil {
		return recipes.SecretData{}, fmt.Errorf("failed to locate backing Kubernetes Secret for secret resource '%s': %w", secretResourceID.String(), err)
	}

	clientset, err := e.KubernetesProvider.ClientGoClient()
	if err != nil {
		return recipes.SecretData{}, err
	}

	secret, err := clientset.CoreV1().Secrets(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return recipes.SecretData{}, fmt.Errorf("failed to read Kubernetes Secret '%s/%s' for secret resource '%s': %w", namespace, name, secretResourceID.String(), err)
	}

	secretData := recipes.SecretData{
		Type: kind,
		Data: make(map[string]string),
	}

	// If no filter is provided, return all keys present in the backing secret.
	if len(secretKeysFilter) == 0 {
		secretKeysFilter = make([]string, 0, len(secret.Data))
		for key := range secret.Data {
			secretKeysFilter = append(secretKeysFilter, key)
		}
	}

	for _, key := range secretKeysFilter {
		// client-go decodes Secret.Data values from base64 automatically.
		value, ok := secret.Data[key]
		if !ok {
			return recipes.SecretData{}, fmt.Errorf("'%s' secret key was not found in secret resource '%s'", key, secretResourceID.String())
		}
		secretData.Data[key] = string(value)
	}

	return secretData, nil
}

// findKubernetesSecretOutputResource scans the `status.outputResources` of a Radius.Security/secrets resource
// and returns the namespace and name of the Kubernetes Secret it provisioned.
//
// The output-resource type recorded for the backing Secret varies with how the recipe declares it
// (`kubernetes_secret` -> core/Secret, `kubernetes_secret_v1` -> core/secretv1, `kubernetes_manifest` -> v1/Secret),
// so the match is on the Kubernetes resource kind ("secret") rather than the full UCP type string.
func findKubernetesSecretOutputResource(properties map[string]any) (namespace string, name string, err error) {
	status, ok := properties["status"].(map[string]any)
	if !ok {
		return "", "", fmt.Errorf("resource has no status")
	}

	outputResources, ok := status["outputResources"].([]any)
	if !ok || len(outputResources) == 0 {
		return "", "", fmt.Errorf("resource has no output resources")
	}

	for _, entry := range outputResources {
		outputResource, ok := entry.(map[string]any)
		if !ok {
			continue
		}

		idValue, ok := outputResource["id"].(string)
		if !ok || idValue == "" {
			continue
		}

		id, err := resources.ParseResource(idValue)
		if err != nil {
			continue
		}

		// Only consider Kubernetes-plane resources, and match the Secret kind tolerantly across the
		// resource-name variants Terraform produces (Secret / secretv1 / secret_v1).
		if !strings.HasPrefix(strings.ToLower(id.PlaneNamespace()), resources_kubernetes.PlaneTypeKubernetes+"/") {
			continue
		}

		_, kind, ns, n := resources_kubernetes.ToParts(id)
		kindLower := strings.ToLower(strings.ReplaceAll(kind, "_", ""))
		if kindLower != "secret" && kindLower != "secretv1" {
			continue
		}
		if n == "" {
			continue
		}

		return ns, n, nil
	}

	return "", "", fmt.Errorf("no Kubernetes Secret output resource found")
}
