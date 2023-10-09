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
	"fmt"
	"strings"
	"testing"

	"github.com/radius-project/radius/pkg/recipes"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	testTemplatePath    = "Azure/redis/azurerm"
	testRecipeName      = "redis-azure"
	testTemplateVersion = "1.1.0"
	envName             = "env"
	appName             = "app"
	resourceName        = "redis"

	testSecretSuffix = "test-secret-suffix"
)

var (
	envParams = map[string]any{
		"resource_group_name": "test-rg",
		"sku":                 "C",
	}

	resourceParams = map[string]any{
		"redis_cache_name": "redis-test",
		"sku":              "P",
	}
)

func getTestInputs() (recipes.EnvironmentDefinition, recipes.ResourceMetadata) {
	envRecipe := recipes.EnvironmentDefinition{
		Name:            testRecipeName,
		TemplatePath:    testTemplatePath,
		TemplateVersion: testTemplateVersion,
		Parameters:      envParams,
	}

	resourceRecipe := recipes.ResourceMetadata{
		Name:          testRecipeName,
		Parameters:    resourceParams,
		EnvironmentID: "/planes/radius/local/resourceGroups/test-group/providers/Applications.Environments/testEnv/env",
		ApplicationID: "/planes/radius/local/resourceGroups/test-group/providers/Applications.Applications/testApp/app",
		ResourceID:    "/planes/radius/local/resourceGroups/test-group/providers/Applications.Datastores/redisCaches/redis",
	}
	return envRecipe, resourceRecipe
}

func Test_GenerateKubernetesBackendConfig(t *testing.T) {
	t.Setenv("KUBERNETES_SERVICE_HOST", "")
	t.Setenv("KUBERNETES_SERVICE_PORT", "")
	actualConfig, err := generateKubernetesBackendConfig(testSecretSuffix)
	require.NoError(t, err)
	expectedConfig := map[string]interface{}{
		"kubernetes": map[string]interface{}{
			"config_path":   clientcmd.RecommendedHomeFile,
			"secret_suffix": testSecretSuffix,
			"namespace":     RadiusNamespace,
		},
	}
	require.Equal(t, expectedConfig, actualConfig)
}

func Test_GenerateSecretSuffix(t *testing.T) {
	_, resourceRecipe := getTestInputs()
	hasher := sha1.New()
	_, err := hasher.Write([]byte(strings.ToLower(fmt.Sprintf("%s-%s-%s", envName, appName, resourceRecipe.ResourceID))))
	require.NoError(t, err)
	expSecret := fmt.Sprintf("%x", hasher.Sum(nil))
	secret, err := generateSecretSuffix(&resourceRecipe)
	require.NoError(t, err)
	require.Equal(t, expSecret, secret)
}

func Test_GenerateSecretSuffix_invalid_resourceid(t *testing.T) {
	_, resourceRecipe := getTestInputs()
	resourceRecipe.ResourceID = "invalid"
	_, err := generateSecretSuffix(&resourceRecipe)
	require.Equal(t, err.Error(), "'invalid' is not a valid resource id")
}

func Test_GenerateSecretSuffix_invalid_envid(t *testing.T) {
	_, resourceRecipe := getTestInputs()
	resourceRecipe.EnvironmentID = "invalid"
	_, err := generateSecretSuffix(&resourceRecipe)
	require.Equal(t, err.Error(), "'invalid' is not a valid resource id")
}

func Test_GenerateSecretSuffix_invalid_appid(t *testing.T) {
	_, resourceRecipe := getTestInputs()
	resourceRecipe.ApplicationID = "invalid"
	_, err := generateSecretSuffix(&resourceRecipe)
	require.Equal(t, err.Error(), "'invalid' is not a valid resource id")
}

func Test_ValidateBackendExists(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-secret",
			Namespace: RadiusNamespace,
		},
		Data: map[string][]byte{
			"key": []byte("value"),
		},
	}
	_, err := clientset.CoreV1().Secrets(RadiusNamespace).Create(context.Background(), secret, metav1.CreateOptions{})
	require.NoError(t, err)

	b := NewKubernetesBackend(clientset)
	exists, err := b.ValidateBackendExists(context.Background(), "test-secret")
	require.NoError(t, err)
	require.True(t, exists)

	// Validate that the function returns false for a non-existent secret.
	exists, err = b.ValidateBackendExists(context.Background(), "invalid-secret")
	require.NoError(t, err)
	require.False(t, exists)

	// Validate error is returned for errors other than NotFound.
	clientset.Fake.PrependReactor("get", "secrets", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, nil, k8s_errors.NewServerTimeout(schema.GroupResource{Resource: "test-secret"}, "get", 1)
	})
	exists, err = b.ValidateBackendExists(context.Background(), "test-secret")
	require.Error(t, err)
	require.True(t, k8s_errors.IsServerTimeout(err))
	require.False(t, exists)
}
