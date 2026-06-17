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

package corerp

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/radius-project/radius/pkg/cli"
	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/recipes/terraform/config/backends"
	"github.com/radius-project/radius/pkg/ucp/resources"
	resources_radius "github.com/radius-project/radius/pkg/ucp/resources/radius"
	"github.com/radius-project/radius/test/rp"
)

// TestSecretDeletion tests that the secret is deleted after the resource is deleted
func TestSecretDeletion(t *testing.T, ctx context.Context, test rp.RPTest, appName, envName, resourceID string, secretNamespace string, secretPrefix string) {
	secretSuffix, err := GetSecretSuffix(resourceID, envName, appName)
	require.NoError(t, err)

	secret, err := test.Options.K8sClient.CoreV1().Secrets(secretNamespace).
		Get(ctx, secretPrefix+secretSuffix, metav1.GetOptions{})
	require.Error(t, err)
	require.True(t, apierrors.IsNotFound(err))
	require.Equal(t, secret, &corev1.Secret{})
}

// CurrentWorkspaceResourceGroup loads the rad workspace currently configured for tests and
// returns the resource group from its scope. If no workspace is configured it falls back
// to the historical default "kind-radius" used by the cloud test harness.
func CurrentWorkspaceResourceGroup() string {
	config, err := cli.LoadConfig("")
	if err != nil {
		return "kind-radius"
	}
	workspace, err := cli.GetWorkspace(config, "")
	if err != nil || workspace == nil || workspace.Scope == "" {
		return "kind-radius"
	}
	scope, err := resources.ParseScope(workspace.Scope)
	if err != nil {
		return "kind-radius"
	}
	if rg := scope.FindScope(resources_radius.ScopeResourceGroups); rg != "" {
		return rg
	}
	return "kind-radius"
}

// GetSecretSuffix returns the secret suffix for a given resource. The resource group
// embedded in the environment / application / resource IDs is normalized to the
// current workspace's resource group so the computed suffix matches the secret name
// produced by the Terraform recipe at runtime.
func GetSecretSuffix(resourceID, envName, appName string) (string, error) {
	rg := CurrentWorkspaceResourceGroup()
	envID := "/planes/radius/local/resourcegroups/" + rg + "/providers/Applications.Core/environments/" + envName
	appID := "/planes/radius/local/resourcegroups/" + rg + "/providers/Applications.Core/applications/" + appName
	resourceID = rewriteResourceGroup(resourceID, rg)

	resourceRecipe := recipes.ResourceMetadata{
		EnvironmentID: envID,
		ApplicationID: appID,
		ResourceID:    resourceID,
		Parameters:    nil,
	}

	backend := backends.NewKubernetesBackend(nil)
	secretMap, err := backend.BuildBackend(&resourceRecipe)
	if err != nil {
		return "", err
	}
	kubernetes := secretMap["kubernetes"].(map[string]any)

	return kubernetes["secret_suffix"].(string), nil
}

// rewriteResourceGroup rewrites the resource group segment of a Radius resource ID to
// the supplied resource group name. Returns the input unchanged if it cannot be parsed.
func rewriteResourceGroup(resourceID, rg string) string {
	parsed, err := resources.ParseResource(resourceID)
	if err != nil {
		return resourceID
	}
	current := parsed.FindScope(resources_radius.ScopeResourceGroups)
	if current == "" || current == rg {
		return resourceID
	}
	// Case-insensitive replacement of the resourcegroups segment value while preserving the
	// rest of the ID exactly.
	return strings.ReplaceAll(resourceID, "/"+current+"/", "/"+rg+"/")
}
