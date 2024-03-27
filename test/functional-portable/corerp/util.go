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
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/recipes/terraform/config/backends"
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

// GetSecretSuffix returns the secret suffix for a given resource
func GetSecretSuffix(resourceID, envName, appName string) (string, error) {
	envID := "/planes/radius/local/resourcegroups/kind-radius/providers/Applications.Core/environments/" + envName
	appID := "/planes/radius/local/resourcegroups/kind-radius/providers/Applications.Core/applications/" + appName

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
