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

package driver

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	gomock "github.com/golang/mock/gomock"
	"github.com/google/uuid"
	tfjson "github.com/hashicorp/terraform-json"
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/recipes"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"

	"github.com/radius-project/radius/pkg/recipes/terraform"
	"github.com/radius-project/radius/test/testcontext"
	"github.com/stretchr/testify/require"
)

func setup(t *testing.T) (terraform.MockTerraformExecutor, terraformDriver) {
	ctrl := gomock.NewController(t)
	tfExecutor := terraform.NewMockTerraformExecutor(ctrl)

	driver := terraformDriver{tfExecutor, TerraformOptions{Path: t.TempDir()}}

	return *tfExecutor, driver
}

func buildTestInputs() (recipes.Configuration, recipes.ResourceMetadata, recipes.EnvironmentDefinition) {
	envConfig := recipes.Configuration{
		Providers: datamodel.Providers{
			Azure: datamodel.ProvidersAzure{
				Scope: "scope",
			},
		},
	}

	recipeMetadata := recipes.ResourceMetadata{
		Name:          "redis-azure",
		ApplicationID: "/planes/radius/local/resourcegroups/test-rg/providers/applications.core/applications/app1",
		EnvironmentID: "/planes/radius/local/resourcegroups/test-rg/providers/applications.core/environments/env1",
		ResourceID:    "/planes/radius/local/resourceGroups/test-rg/providers/applications.datastores/rediscaches/test-redis-recipe",
		Parameters: map[string]any{
			"redis_cache_name": "redis-test",
		},
	}

	envRecipe := recipes.EnvironmentDefinition{
		Name:         "redis-azure",
		Driver:       recipes.TemplateKindBicep,
		TemplatePath: "Azure/redis/azurerm",
		ResourceType: "Applications.Datastores/redisCaches",
	}

	return envConfig, recipeMetadata, envRecipe
}

func Test_Terraform_Execute_Success(t *testing.T) {
	ctx := testcontext.New(t)
	armCtx := &v1.ARMRequestContext{
		OperationID: uuid.New(),
	}
	ctx = v1.WithARMRequestContext(ctx, armCtx)

	tfExecutor, driver := setup(t)
	envConfig, recipeMetadata, envRecipe := buildTestInputs()
	tfDir := filepath.Join(driver.options.Path, armCtx.OperationID.String())
	options := terraform.Options{
		RootDir:        tfDir,
		EnvConfig:      &envConfig,
		ResourceRecipe: &recipeMetadata,
		EnvRecipe:      &envRecipe,
	}
	expectedOutput := &recipes.RecipeOutput{
		Values: map[string]any{
			"host": "myrediscache.redis.cache.windows.net",
			"port": float64(6379),
		},
		Secrets:   map[string]any{},
		Resources: []string{},
	}

	expectedTFState := &tfjson.State{
		Values: &tfjson.StateValues{
			Outputs: map[string]*tfjson.StateOutput{
				recipes.ResultPropertyName: {
					Value: map[string]any{
						"values": map[string]any{
							"host": "myrediscache.redis.cache.windows.net",
							"port": json.Number("6379"),
						},
					},
				},
			},
		},
	}

	tfExecutor.EXPECT().Deploy(ctx, options).Times(1).Return(expectedTFState, nil)

	recipeOutput, err := driver.Execute(ctx, ExecuteOptions{
		BaseOptions: BaseOptions{
			Configuration: envConfig,
			Recipe:        recipeMetadata,
			Definition:    envRecipe,
		},
	})
	require.NoError(t, err)
	require.Equal(t, expectedOutput, recipeOutput)
	// Verify directory cleanup
	_, err = os.Stat(tfDir)
	require.True(t, os.IsNotExist(err), "Expected directory %s to be removed, but it still exists", tfDir)
}

func Test_Terraform_Execute_DeploymentFailure(t *testing.T) {
	ctx := testcontext.New(t)
	armCtx := &v1.ARMRequestContext{
		OperationID: uuid.New(),
	}
	ctx = v1.WithARMRequestContext(ctx, armCtx)

	tfExecutor, driver := setup(t)
	envConfig, recipeMetadata, envRecipe := buildTestInputs()
	tfDir := filepath.Join(driver.options.Path, armCtx.OperationID.String())
	options := terraform.Options{
		RootDir:        tfDir,
		EnvConfig:      &envConfig,
		ResourceRecipe: &recipeMetadata,
		EnvRecipe:      &envRecipe,
	}
	recipeError := recipes.RecipeError{
		ErrorDetails: v1.ErrorDetails{
			Code:    recipes.RecipeDeploymentFailed,
			Message: "Failed to deploy terraform module",
		},
		DeploymentStatus: "executionError",
	}
	tfExecutor.EXPECT().Deploy(ctx, options).Times(1).Return(nil, errors.New("Failed to deploy terraform module"))

	_, err := driver.Execute(ctx, ExecuteOptions{
		BaseOptions: BaseOptions{
			Configuration: envConfig,
			Recipe:        recipeMetadata,
			Definition:    envRecipe,
		},
	})
	require.Error(t, err)
	require.Equal(t, err, &recipeError)
	// Verify directory cleanup
	_, err = os.Stat(tfDir)
	require.True(t, os.IsNotExist(err), "Expected directory %s to be removed, but it still exists", tfDir)
}

func Test_Terraform_Execute_OutputsFailure(t *testing.T) {
	ctx := testcontext.New(t)
	armCtx := &v1.ARMRequestContext{
		OperationID: uuid.New(),
	}
	ctx = v1.WithARMRequestContext(ctx, armCtx)

	tfExecutor, driver := setup(t)
	envConfig, recipeMetadata, envRecipe := buildTestInputs()
	tfDir := filepath.Join(driver.options.Path, armCtx.OperationID.String())
	options := terraform.Options{
		RootDir:        tfDir,
		EnvConfig:      &envConfig,
		ResourceRecipe: &recipeMetadata,
		EnvRecipe:      &envRecipe,
	}

	expectedTFState := &tfjson.State{
		Values: &tfjson.StateValues{
			Outputs: map[string]*tfjson.StateOutput{
				recipes.ResultPropertyName: {
					Value: map[string]any{
						"values": map[string]any{
							"host": "myrediscache.redis.cache.windows.net",
							"port": json.Number("6379"),
						},
						"invalid": "invalid field",
					},
				},
			},
		},
	}
	recipeError := recipes.RecipeError{
		ErrorDetails: v1.ErrorDetails{
			Code:    recipes.InvalidRecipeOutputs,
			Message: "failed to read the recipe output \"result\": json: unknown field \"invalid\"",
		},
		DeploymentStatus: "executionError",
	}
	tfExecutor.EXPECT().Deploy(ctx, options).Times(1).Return(expectedTFState, nil)

	_, err := driver.Execute(ctx, ExecuteOptions{
		BaseOptions: BaseOptions{
			Configuration: envConfig,
			Recipe:        recipeMetadata,
			Definition:    envRecipe,
		},
	})
	require.Error(t, err)
	require.Equal(t, err, &recipeError)
	// Verify directory cleanup
	_, err = os.Stat(tfDir)
	require.True(t, os.IsNotExist(err), "Expected directory %s to be removed, but it still exists", tfDir)
}

func Test_Terraform_Execute_EmptyPath(t *testing.T) {
	_, driver := setup(t)
	driver.options.Path = ""
	envConfig, recipeMetadata, envRecipe := buildTestInputs()
	expErr := recipes.RecipeError{
		ErrorDetails: v1.ErrorDetails{
			Code:    recipes.RecipeDeploymentFailed,
			Message: "path is a required option for Terraform driver",
		},
		DeploymentStatus: "setupError",
	}
	_, err := driver.Execute(testcontext.New(t), ExecuteOptions{
		BaseOptions: BaseOptions{
			Configuration: envConfig,
			Recipe:        recipeMetadata,
			Definition:    envRecipe,
		},
	})
	require.Error(t, err)
	require.Equal(t, err, &expErr)

}

func Test_Terraform_Execute_EmptyOperationID_Success(t *testing.T) {
	ctx := testcontext.New(t)
	ctx = v1.WithARMRequestContext(ctx, &v1.ARMRequestContext{})

	tfExecutor, driver := setup(t)
	envConfig, recipeMetadata, envRecipe := buildTestInputs()
	expectedOutput := &recipes.RecipeOutput{
		Values: map[string]any{
			"host": "myrediscache.redis.cache.windows.net",
			"port": float64(6379),
		},
		Secrets:   map[string]any{},
		Resources: []string{},
	}

	expectedTFState := &tfjson.State{
		Values: &tfjson.StateValues{
			Outputs: map[string]*tfjson.StateOutput{
				recipes.ResultPropertyName: {
					Value: map[string]any{
						"values": map[string]any{
							"host": "myrediscache.redis.cache.windows.net",
							"port": json.Number("6379"),
						},
					},
				},
			},
		},
	}

	tfExecutor.EXPECT().
		Deploy(ctx, gomock.Any()).
		Times(1).
		Return(expectedTFState, nil)

	recipeOutput, err := driver.Execute(ctx, ExecuteOptions{
		BaseOptions: BaseOptions{
			Configuration: envConfig,
			Recipe:        recipeMetadata,
			Definition:    envRecipe,
		},
	})
	require.NoError(t, err)
	require.Equal(t, expectedOutput, recipeOutput)
}

func Test_Terraform_Execute_MissingARMRequestContext_Panics(t *testing.T) {
	ctx := testcontext.New(t)
	// Do not add ARMRequestContext to the context

	_, driver := setup(t)
	envConfig, recipeMetadata, envRecipe := buildTestInputs()

	require.Panics(t, func() {
		_, _ = driver.Execute(ctx, ExecuteOptions{
			BaseOptions: BaseOptions{
				Configuration: envConfig,
				Recipe:        recipeMetadata,
				Definition:    envRecipe,
			},
		})
	})
}

func TestTerraformDriver_GetRecipeMetadata_Success(t *testing.T) {
	ctx := testcontext.New(t)
	armCtx := &v1.ARMRequestContext{
		OperationID: uuid.New(),
	}
	ctx = v1.WithARMRequestContext(ctx, armCtx)

	tfExecutor, driver := setup(t)
	_, _, envRecipe := buildTestInputs()

	tfDir := filepath.Join(driver.options.Path, armCtx.OperationID.String())
	expectedOutput := map[string]any{
		"parameters": map[string]any{
			"redis_cache_name": "redis-test",
		},
	}
	options := terraform.Options{
		RootDir:        tfDir,
		ResourceRecipe: &recipes.ResourceMetadata{},
		EnvRecipe:      &envRecipe,
	}
	tfExecutor.EXPECT().GetRecipeMetadata(ctx, options).Times(1).Return(expectedOutput, nil)

	recipeData, err := driver.GetRecipeMetadata(ctx, BaseOptions{
		Recipe:     recipes.ResourceMetadata{},
		Definition: envRecipe,
	})
	require.NoError(t, err)
	require.Equal(t, expectedOutput, recipeData)
	// Verify directory cleanup
	_, err = os.Stat(tfDir)
	require.True(t, os.IsNotExist(err), "Expected directory %s to be removed, but it still exists", tfDir)
}

func Test_Terraform_GetRecipeMetadata_EmptyPath(t *testing.T) {
	_, driver := setup(t)
	driver.options.Path = ""
	_, _, envRecipe := buildTestInputs()

	expErr := recipes.RecipeError{
		ErrorDetails: v1.ErrorDetails{
			Code:    recipes.RecipeGetMetadataFailed,
			Message: "path is a required option for Terraform driver",
		},
	}

	_, err := driver.GetRecipeMetadata(testcontext.New(t), BaseOptions{
		Recipe:     recipes.ResourceMetadata{},
		Definition: envRecipe,
	})
	require.Error(t, err)
	require.Equal(t, err, &expErr)
}

func TestTerraformDriver_GetRecipeMetadata_Failure(t *testing.T) {
	ctx := testcontext.New(t)
	armCtx := &v1.ARMRequestContext{
		OperationID: uuid.New(),
	}
	ctx = v1.WithARMRequestContext(ctx, armCtx)

	tfExecutor, driver := setup(t)
	_, _, envRecipe := buildTestInputs()

	tfDir := filepath.Join(driver.options.Path, armCtx.OperationID.String())
	options := terraform.Options{
		RootDir:        tfDir,
		ResourceRecipe: &recipes.ResourceMetadata{},
		EnvRecipe:      &envRecipe,
	}

	expErr := recipes.RecipeError{
		ErrorDetails: v1.ErrorDetails{
			Code:    recipes.RecipeGetMetadataFailed,
			Message: "Failed to download module",
		},
	}
	tfExecutor.EXPECT().GetRecipeMetadata(ctx, options).Times(1).Return(nil, errors.New("Failed to download module"))

	_, err := driver.GetRecipeMetadata(ctx, BaseOptions{
		Recipe:     recipes.ResourceMetadata{},
		Definition: envRecipe,
	})
	require.Error(t, err)
	require.Equal(t, &expErr, err)
}

func Test_Terraform_Delete_Success(t *testing.T) {
	ctx := testcontext.New(t)
	ctx = v1.WithARMRequestContext(ctx, &v1.ARMRequestContext{})

	tfExecutor, driver := setup(t)
	envConfig, recipeMetadata, envRecipe := buildTestInputs()

	tfExecutor.EXPECT().
		Delete(ctx, gomock.Any()).
		Times(1).
		Return(nil)

	err := driver.Delete(ctx, DeleteOptions{
		BaseOptions: BaseOptions{
			Configuration: envConfig,
			Recipe:        recipeMetadata,
			Definition:    envRecipe,
		},
		OutputResources: []rpv1.OutputResource{},
	})
	require.NoError(t, err)
}

func Test_Terraform_Delete_EmptyPath(t *testing.T) {
	_, driver := setup(t)
	driver.options.Path = ""
	envConfig, recipeMetadata, envRecipe := buildTestInputs()

	expErr := recipes.RecipeError{
		ErrorDetails: v1.ErrorDetails{
			Code:    recipes.RecipeDeletionFailed,
			Message: "path is a required option for Terraform driver",
		},
	}

	err := driver.Delete(testcontext.New(t), DeleteOptions{
		BaseOptions: BaseOptions{
			Configuration: envConfig,
			Recipe:        recipeMetadata,
			Definition:    envRecipe,
		},
		OutputResources: []rpv1.OutputResource{},
	})
	require.Error(t, err)
	require.Equal(t, err, &expErr)
}

func Test_Terraform_Delete_Failure(t *testing.T) {
	ctx := testcontext.New(t)
	ctx = v1.WithARMRequestContext(ctx, &v1.ARMRequestContext{})

	tfExecutor, driver := setup(t)
	envConfig, recipeMetadata, envRecipe := buildTestInputs()

	tfExecutor.EXPECT().
		Delete(ctx, gomock.Any()).
		Times(1).
		Return(errors.New("Failed to delete terraform module"))

	expErr := recipes.RecipeError{
		ErrorDetails: v1.ErrorDetails{
			Code:    recipes.RecipeDeletionFailed,
			Message: "Failed to delete terraform module",
		},
	}

	err := driver.Delete(ctx, DeleteOptions{
		BaseOptions: BaseOptions{
			Configuration: envConfig,
			Recipe:        recipeMetadata,
			Definition:    envRecipe,
		},
		OutputResources: []rpv1.OutputResource{},
	})
	require.Error(t, err)
	require.Equal(t, &expErr, err)
}

func Test_Terraform_PrepareRecipeResponse(t *testing.T) {
	d := &terraformDriver{}
	tests := []struct {
		desc             string
		state            *tfjson.State
		expectedResponse *recipes.RecipeOutput
		expectedErr      error
	}{
		{
			desc: "valid state",
			state: &tfjson.State{
				Values: &tfjson.StateValues{
					Outputs: map[string]*tfjson.StateOutput{
						recipes.ResultPropertyName: {
							Value: map[string]any{
								"values": map[string]any{
									"host": "testhost",
									"port": json.Number("6379"),
								},
								"secrets": map[string]any{
									"connectionString": "testConnectionString",
								},
								"resources": []any{"outputResourceId1", "/planes/aws/aws/accounts/179022619019/regions/us-east-2/providers/AWS.ec2/subnet/subnet-0ddfaa93733f98002"},
							},
						},
					},
					RootModule: &tfjson.StateModule{
						ChildModules: []*tfjson.StateModule{
							{
								Resources: []*tfjson.StateResource{
									{
										ProviderName: "registry.terraform.io/hashicorp/aws",
										AttributeValues: map[string]any{
											"arn": "arn:aws:ec2:us-east-2:179022619019:subnet/subnet-0ddfaa93733f98002",
										},
									},
									{
										ProviderName: "registry.terraform.io/hashicorp/azurerm",
										AttributeValues: map[string]any{
											"id": "/subscriptions/66d1209e-1382-45d3-99bb-650e6bf63fc0/resourceGroups/vhiremath-dev/providers/Microsoft.DocumentDB/databaseAccounts/tf-test-cosmos",
										},
									},
									{
										Type:         "kubernetes_deployment",
										ProviderName: "registry.terraform.io/hashicorp/kubernetes",
										AttributeValues: map[string]any{
											"id": "default/test-redis",
										},
									},
									{
										Type:         "kubernetes_manifest",
										ProviderName: "registry.terraform.io/hashicorp/kubernetes",
										AttributeValues: map[string]any{
											"manifest": map[string]any{
												"apiVersion": "dapr.io/v1alpha1",
												"kind":       "Component",
												"metadata": map[string]any{
													"name":      "test-dapr",
													"namespace": "test-namespace",
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			expectedResponse: &recipes.RecipeOutput{
				Values: map[string]any{
					"host": "testhost",
					"port": float64(6379),
				},
				Secrets: map[string]any{
					"connectionString": "testConnectionString",
				},
				Resources: []string{"outputResourceId1",
					"/planes/aws/aws/accounts/179022619019/regions/us-east-2/providers/AWS.ec2/subnet/subnet-0ddfaa93733f98002",
					"/subscriptions/66d1209e-1382-45d3-99bb-650e6bf63fc0/resourceGroups/vhiremath-dev/providers/Microsoft.DocumentDB/databaseAccounts/tf-test-cosmos",
					"/planes/kubernetes/local/namespaces/default/providers/apps/Deployment/test-redis",
					"/planes/kubernetes/local/namespaces/test-namespace/providers/dapr.io/Component/test-dapr",
				},
			},
		},
		{
			desc: "invalid state",
			state: &tfjson.State{
				Values: &tfjson.StateValues{
					Outputs: map[string]*tfjson.StateOutput{
						recipes.ResultPropertyName: {
							Value: map[string]any{
								"values": map[string]any{
									"host": "testhost",
									"port": json.Number("6379"),
								},
								"secrets": map[string]any{
									"connectionString": "testConnectionString",
								},
								"resources": []any{"outputResourceId1"},
								"outputs":   "invalidField",
							},
						},
					},
					RootModule: &tfjson.StateModule{
						ChildModules: []*tfjson.StateModule{
							{
								Resources: []*tfjson.StateResource{
									{
										AttributeValues: map[string]any{
											"id": "outputResourceId2",
										},
									},
								},
							},
						},
					},
				},
			},
			expectedResponse: &recipes.RecipeOutput{},
			expectedErr:      errors.New("json: unknown field \"outputs\""),
		},
		{
			desc:             "nil state",
			state:            nil,
			expectedResponse: &recipes.RecipeOutput{},
			expectedErr:      errors.New("terraform state is empty"),
		},
		{
			desc:             "empty state",
			state:            &tfjson.State{},
			expectedResponse: &recipes.RecipeOutput{},
			expectedErr:      errors.New("terraform state is empty"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			recipeResponse, err := d.prepareRecipeResponse(tt.state)
			require.Equal(t, tt.expectedErr, err)
			require.Equal(t, tt.expectedResponse, recipeResponse)
		})
	}
}
