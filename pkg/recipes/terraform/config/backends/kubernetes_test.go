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
	"fmt"
	"strings"
	"testing"

	"github.com/radius-project/radius/pkg/recipes"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	testTemplatePath    = "Azure/redis/azurerm"
	testRecipeName      = "redis-azure"
	testTemplateVersion = "1.1.0"
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

const (
	testSecretSuffix = "test-secret-suffix"
)

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

func Test_GenerateSecretSuffix_invalid_resourceid(t *testing.T) {
	_, resourceRecipe := getTestInputs()
	resourceRecipe.ResourceID = "invalid"
	_, err := generateSecretSuffix(&resourceRecipe)
	require.Equal(t, err.Error(), "'invalid' is not a valid resource id")
}

func Test_GenerateSecretSuffix_with_lengthy_resource_name(t *testing.T) {
	_, resourceRecipe := getTestInputs()
	act, err := generateSecretSuffix(&resourceRecipe)
	require.NoError(t, err)
	hasher := sha1.New()
	_, _ = hasher.Write([]byte(strings.ToLower("env-app-" + resourceRecipe.ResourceID)))
	hash := hasher.Sum(nil)
	require.Equal(t, act, "env-app-redis."+fmt.Sprintf("%x", hash))
}

func Test_GenerateKubernetesBackendConfig_Error(t *testing.T) {
	t.Setenv("KUBERNETES_SERVICE_HOST", "testvalue")
	t.Setenv("KUBERNETES_SERVICE_PORT", "1111")

	backend, err := generateKubernetesBackendConfig("test-suffix")
	require.Error(t, err)
	require.Nil(t, backend)
}
