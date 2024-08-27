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

package util

import (
	"testing"

	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/recipes"
	"github.com/stretchr/testify/require"
)

func Test_PathParser(t *testing.T) {
	repository, tag, err := parsePath("ghcr.io/radius-project/dev/recipes/functionaltest/parameters/mongodatabases/azure:1.0")
	require.NoError(t, err)
	require.Equal(t, "ghcr.io/radius-project/dev/recipes/functionaltest/parameters/mongodatabases/azure", repository)
	require.Equal(t, "1.0", tag)
}

func Test_PathParserErr(t *testing.T) {
	repository, tag, err := parsePath("http://user:passwd@example.com/test/bar:v1")
	require.Error(t, err)
	require.Equal(t, "", repository)
	require.Equal(t, "", tag)
}

func Test_GetRegistrySecrets(t *testing.T) {
	testset := []struct {
		definition   recipes.Configuration
		templatePath string
		secrets      map[string]map[string]string
		exp          map[string]string
	}{
		{
			definition: recipes.Configuration{
				RecipeConfig: datamodel.RecipeConfigProperties{
					Bicep: datamodel.BicepConfigProperties{
						Authentication: map[string]datamodel.RegistrySecretConfig{
							"test.azurecr.io": {
								Secret: "/planes/radius/local/resourcegroups/default/providers/Applications.Core/secretStores/acr",
							},
							"123456789012.dkr.ecr.us-west-2.amazonaws.com": {
								Secret: "/planes/radius/local/resourcegroups/default/providers/Applications.Core/secretStores/ecr",
							},
						},
					},
				},
			},
			templatePath: "test.azurecr.io/test-private-registry:latest",
			secrets: map[string]map[string]string{
				"/planes/radius/local/resourcegroups/default/providers/Applications.Core/secretStores/acr": {
					"username": "test-username",
					"password": "test-password",
				},
			},
			exp: map[string]string{
				"username": "test-username",
				"password": "test-password",
			},
		},
		{
			definition: recipes.Configuration{
				RecipeConfig: datamodel.RecipeConfigProperties{
					Bicep: datamodel.BicepConfigProperties{},
				},
			},
			templatePath: "test.azurecr.io/test-private-registry:latest",
			secrets: map[string]map[string]string{
				"/planes/radius/local/resourcegroups/default/providers/Applications.Core/secretStores/acr": {
					"username": "test-username",
					"password": "test-password",
				},
			},
			exp: nil,
		},
	}
	for _, tc := range testset {
		secrets, err := GetRegistrySecrets(tc.definition, tc.templatePath, tc.secrets)
		require.NoError(t, err)
		require.Equal(t, secrets, tc.exp)
	}
}
