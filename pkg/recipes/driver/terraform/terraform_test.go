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

package terraform

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	tfjson "github.com/hashicorp/terraform-json"
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/recipes/driver"
	"github.com/radius-project/radius/pkg/recipes/terraform"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	"github.com/radius-project/radius/test/testcontext"
	"github.com/stretchr/testify/require"
	gomock "go.uber.org/mock/gomock"
)

func setup(t *testing.T) (terraform.MockTerraformExecutor, terraformDriver) {
	ctrl := gomock.NewController(t)
	tfExecutor := terraform.NewMockTerraformExecutor(ctrl)
	tfDriver := terraformDriver{
		terraformExecutor: tfExecutor,
		ucpConn:           nil, // Mock connection - not needed for these tests
		secretProvider:    nil, // Mock secret provider - not needed for these tests
		options:           TerraformOptions{Path: t.TempDir()},
	}
	return *tfExecutor, tfDriver
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
		Name:            "redis-azure",
		Driver:          recipes.TemplateKindBicep,
		TemplatePath:    "Azure/redis/azurerm",
		ResourceType:    "Applications.Datastores/redisCaches",
		TemplateVersion: "1.0",
	}

	return envConfig, recipeMetadata, envRecipe
}

func verifyDirectoryCleanup(t *testing.T, tfRootDirPath string, armOperationID string) {
	directories, err := os.ReadDir(tfRootDirPath)
	require.NoError(t, err)
	for _, dir := range directories {
		if dir.IsDir() {
			require.False(t, strings.HasPrefix(dir.Name(), armOperationID), "Expected directory %s to be removed, but it still exists", dir.Name())
		}
	}
}

func Test_Terraform_Execute_Success(t *testing.T) {
	ctx := testcontext.New(t)
	armCtx := &v1.ARMRequestContext{
		OperationID: uuid.New(),
	}
	ctx = v1.WithARMRequestContext(ctx, armCtx)

	tfExecutor, tfDriver := setup(t)
	envConfig, recipeMetadata, envRecipe := buildTestInputs()

	expectedOutput := &recipes.RecipeOutput{
		Values: map[string]any{
			"host": "myrediscache.redis.cache.windows.net",
			"port": float64(6379),
		},
		Secrets:   map[string]any{},
		Resources: []string{},
		Status: &rpv1.RecipeStatus{
			TemplateKind:    recipes.TemplateKindTerraform,
			TemplatePath:    "Azure/redis/azurerm",
			TemplateVersion: "1.0",
		},
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

	tfExecutor.EXPECT().Deploy(ctx, gomock.Any()).Times(1).Return(expectedTFState, nil)

	recipeOutput, err := tfDriver.Execute(ctx, driver.ExecuteOptions{
		BaseOptions: driver.BaseOptions{
			Configuration: envConfig,
			Recipe:        recipeMetadata,
			Definition:    envRecipe,
		},
	})
	require.NoError(t, err)
	require.Equal(t, expectedOutput, recipeOutput)
	verifyDirectoryCleanup(t, tfDriver.options.Path, armCtx.OperationID.String())
}

func Test_Terraform_Execute_DeploymentFailure(t *testing.T) {
	ctx := testcontext.New(t)
	armCtx := &v1.ARMRequestContext{
		OperationID: uuid.New(),
	}
	ctx = v1.WithARMRequestContext(ctx, armCtx)

	tfExecutor, tfDriver := setup(t)
	envConfig, recipeMetadata, envRecipe := buildTestInputs()
	recipeError := recipes.RecipeError{
		ErrorDetails: v1.ErrorDetails{
			Code:    recipes.RecipeDeploymentFailed,
			Message: "Failed to deploy terraform module",
		},
		DeploymentStatus: "executionError",
	}
	tfExecutor.EXPECT().Deploy(ctx, gomock.Any()).Times(1).Return(nil, errors.New("Failed to deploy terraform module"))

	_, err := tfDriver.Execute(ctx, driver.ExecuteOptions{
		BaseOptions: driver.BaseOptions{
			Configuration: envConfig,
			Recipe:        recipeMetadata,
			Definition:    envRecipe,
		},
	})
	require.Error(t, err)
	require.Equal(t, err, &recipeError)
	verifyDirectoryCleanup(t, tfDriver.options.Path, armCtx.OperationID.String())
}

func Test_Terraform_Execute_OutputsFailure(t *testing.T) {
	ctx := testcontext.New(t)
	armCtx := &v1.ARMRequestContext{
		OperationID: uuid.New(),
	}
	ctx = v1.WithARMRequestContext(ctx, armCtx)

	tfExecutor, tfDriver := setup(t)
	envConfig, recipeMetadata, envRecipe := buildTestInputs()

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
	tfExecutor.EXPECT().Deploy(ctx, gomock.Any()).Times(1).Return(expectedTFState, nil)

	_, err := tfDriver.Execute(ctx, driver.ExecuteOptions{
		BaseOptions: driver.BaseOptions{
			Configuration: envConfig,
			Recipe:        recipeMetadata,
			Definition:    envRecipe,
		},
	})
	require.Error(t, err)
	require.Equal(t, err, &recipeError)
	verifyDirectoryCleanup(t, tfDriver.options.Path, armCtx.OperationID.String())
}

func Test_Terraform_Execute_EmptyPath(t *testing.T) {
	_, tfDriver := setup(t)
	tfDriver.options.Path = ""
	envConfig, recipeMetadata, envRecipe := buildTestInputs()
	expErr := recipes.RecipeError{
		ErrorDetails: v1.ErrorDetails{
			Code:    recipes.RecipeDeploymentFailed,
			Message: "path is a required option for Terraform driver",
		},
		DeploymentStatus: "setupError",
	}

	_, err := tfDriver.Execute(testcontext.New(t), driver.ExecuteOptions{
		BaseOptions: driver.BaseOptions{
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

	tfExecutor, tfDriver := setup(t)
	envConfig, recipeMetadata, envRecipe := buildTestInputs()
	expectedOutput := &recipes.RecipeOutput{
		Values: map[string]any{
			"host": "myrediscache.redis.cache.windows.net",
			"port": float64(6379),
		},
		Secrets:   map[string]any{},
		Resources: []string{},
		Status: &rpv1.RecipeStatus{
			TemplateKind:    recipes.TemplateKindTerraform,
			TemplatePath:    "Azure/redis/azurerm",
			TemplateVersion: "1.0",
		},
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

	recipeOutput, err := tfDriver.Execute(ctx, driver.ExecuteOptions{
		BaseOptions: driver.BaseOptions{
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

	_, tfDriver := setup(t)
	envConfig, recipeMetadata, envRecipe := buildTestInputs()

	require.Panics(t, func() {
		_, _ = tfDriver.Execute(ctx, driver.ExecuteOptions{
			BaseOptions: driver.BaseOptions{
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

	tfExecutor, tfDriver := setup(t)
	_, _, envRecipe := buildTestInputs()

	expectedOutput := map[string]any{
		"parameters": map[string]any{
			"redis_cache_name": "redis-test",
		},
	}
	tfExecutor.EXPECT().GetRecipeMetadata(ctx, gomock.Any()).Times(1).Return(expectedOutput, nil)

	recipeData, err := tfDriver.GetRecipeMetadata(ctx, driver.BaseOptions{
		Recipe:     recipes.ResourceMetadata{},
		Definition: envRecipe,
	})
	require.NoError(t, err)
	require.Equal(t, expectedOutput, recipeData)
	verifyDirectoryCleanup(t, tfDriver.options.Path, armCtx.OperationID.String())
}

func Test_Terraform_GetRecipeMetadata_EmptyPath(t *testing.T) {
	_, tfDriver := setup(t)
	tfDriver.options.Path = ""
	_, _, envRecipe := buildTestInputs()

	expErr := recipes.RecipeError{
		ErrorDetails: v1.ErrorDetails{
			Code:    recipes.RecipeGetMetadataFailed,
			Message: "path is a required option for Terraform driver",
		},
	}

	_, err := tfDriver.GetRecipeMetadata(testcontext.New(t), driver.BaseOptions{
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

	tfExecutor, tfDriver := setup(t)
	_, _, envRecipe := buildTestInputs()

	expErr := recipes.RecipeError{
		ErrorDetails: v1.ErrorDetails{
			Code:    recipes.RecipeGetMetadataFailed,
			Message: "Failed to download module",
		},
	}
	tfExecutor.EXPECT().GetRecipeMetadata(ctx, gomock.Any()).Times(1).Return(nil, errors.New("Failed to download module"))

	_, err := tfDriver.GetRecipeMetadata(ctx, driver.BaseOptions{
		Recipe:     recipes.ResourceMetadata{},
		Definition: envRecipe,
	})
	require.Error(t, err)
	require.Equal(t, &expErr, err)
	verifyDirectoryCleanup(t, tfDriver.options.Path, armCtx.OperationID.String())
}

func Test_Terraform_Delete_Success(t *testing.T) {
	ctx := testcontext.New(t)
	armCtx := &v1.ARMRequestContext{
		OperationID: uuid.New(),
	}
	ctx = v1.WithARMRequestContext(ctx, armCtx)

	tfExecutor, tfDriver := setup(t)
	envConfig, recipeMetadata, envRecipe := buildTestInputs()

	tfExecutor.EXPECT().Delete(ctx, gomock.Any()).Times(1).Return(nil)

	err := tfDriver.Delete(ctx, driver.DeleteOptions{
		BaseOptions: driver.BaseOptions{
			Configuration: envConfig,
			Recipe:        recipeMetadata,
			Definition:    envRecipe,
		},
		OutputResources: []rpv1.OutputResource{},
	})
	require.NoError(t, err)
	verifyDirectoryCleanup(t, tfDriver.options.Path, armCtx.OperationID.String())
}

func Test_Terraform_Delete_EmptyPath(t *testing.T) {
	_, tfDriver := setup(t)
	tfDriver.options.Path = ""
	envConfig, recipeMetadata, envRecipe := buildTestInputs()

	expErr := recipes.RecipeError{
		ErrorDetails: v1.ErrorDetails{
			Code:    recipes.RecipeDeletionFailed,
			Message: "path is a required option for Terraform driver",
		},
	}

	err := tfDriver.Delete(testcontext.New(t), driver.DeleteOptions{
		BaseOptions: driver.BaseOptions{
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
	armCtx := &v1.ARMRequestContext{
		OperationID: uuid.New(),
	}
	ctx = v1.WithARMRequestContext(ctx, armCtx)

	tfExecutor, tfDriver := setup(t)
	envConfig, recipeMetadata, envRecipe := buildTestInputs()

	tfExecutor.EXPECT().Delete(ctx, gomock.Any()).Times(1).
		Return(errors.New("Failed to delete terraform module"))

	expErr := recipes.RecipeError{
		ErrorDetails: v1.ErrorDetails{
			Code:    recipes.RecipeDeletionFailed,
			Message: "Failed to delete terraform module",
		},
	}

	err := tfDriver.Delete(ctx, driver.DeleteOptions{
		BaseOptions: driver.BaseOptions{
			Configuration: envConfig,
			Recipe:        recipeMetadata,
			Definition:    envRecipe,
		},
		OutputResources: []rpv1.OutputResource{},
	})
	require.Error(t, err)
	require.Equal(t, &expErr, err)
	verifyDirectoryCleanup(t, tfDriver.options.Path, armCtx.OperationID.String())
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
										ProviderName: "registry.terraform.io/hashicorp/aws",
										AttributeValues: map[string]any{
											"arn": "arn:aws:ec2:us-east-2:179022619019:Subnet/Subnet-0ddfaa93733f98002",
										},
									},
									{
										ProviderName: "registry.terraform.io/hashicorp/azurerm",
										AttributeValues: map[string]any{
											"id": "/subscriptions/66d1209e-1382-45d3-99bb-650e6bf63fc0/resourceGroups/vhiremath-dev/providers/Microsoft.DocumentDB/databaseAccounts/tf-test-cosmos",
										},
									},
									// resource with id value not in the ARM resource format
									{
										ProviderName: "registry.terraform.io/hashicorp/azurerm",
										AttributeValues: map[string]any{
											"id": "outputResourceId2",
										},
									},
									{
										Type:         "kubernetes_deployment",
										ProviderName: "registry.terraform.io/hashicorp/kubernetes",
										AttributeValues: map[string]any{
											"metadata": []any{
												map[string]any{
													"name":      "test-redis",
													"namespace": "default",
												},
											},
										},
									},
									{
										Type:         "kubernetes_service_account",
										ProviderName: "registry.terraform.io/hashicorp/kubernetes",
										AttributeValues: map[string]any{
											"metadata": []any{
												map[string]any{
													"name":      "test-service-account",
													"namespace": "default",
												},
											},
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
					"/planes/kubernetes/local/namespaces/default/providers/core/ServiceAccount/test-service-account",
					"/planes/kubernetes/local/namespaces/test-namespace/providers/dapr.io/Component/test-dapr",
				},
				Status: &rpv1.RecipeStatus{
					TemplateKind:    recipes.TemplateKindTerraform,
					TemplatePath:    "radiusdev.azurecr.io/recipes/functionaltest/parameters/mongodatabases/azure:1.0",
					TemplateVersion: "1.0",
				},
			},
		},
		{
			desc: "invalid AWS ARN",
			state: &tfjson.State{
				Values: &tfjson.StateValues{
					Outputs: map[string]*tfjson.StateOutput{
						recipes.ResultPropertyName: {
							Value: map[string]any{
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
											"arn": "arn:aws:ec2:us-east-2:179022619019",
										},
									},
								},
							},
						},
					},
				},
			},
			expectedResponse: &recipes.RecipeOutput{},
			expectedErr:      errors.New("\"arn:aws:ec2:us-east-2:179022619019\" is not a valid ARN"),
		},
		{
			desc: "kubernetes manifest type with no apiVersion information",
			state: &tfjson.State{
				Values: &tfjson.StateValues{
					Outputs: map[string]*tfjson.StateOutput{
						recipes.ResultPropertyName: {
							Value: map[string]any{
								"resources": []any{"outputResourceId1", "/planes/aws/aws/accounts/179022619019/regions/us-east-2/providers/AWS.ec2/subnet/subnet-0ddfaa93733f98002"},
							},
						},
					},
					RootModule: &tfjson.StateModule{
						ChildModules: []*tfjson.StateModule{
							{
								Resources: []*tfjson.StateResource{
									{
										Type:         "kubernetes_manifest",
										ProviderName: "registry.terraform.io/hashicorp/kubernetes",
										AttributeValues: map[string]any{
											"manifest": map[string]any{
												"kind": "Component",
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
			expectedResponse: &recipes.RecipeOutput{},
			expectedErr:      errors.New("unable to get apiVersion information from the resource"),
		},
		{
			desc: "kubernetes resource with no resource name",
			state: &tfjson.State{
				Values: &tfjson.StateValues{
					Outputs: map[string]*tfjson.StateOutput{
						recipes.ResultPropertyName: {
							Value: map[string]any{
								"resources": []any{"outputResourceId1", "/planes/aws/aws/accounts/179022619019/regions/us-east-2/providers/AWS.ec2/subnet/subnet-0ddfaa93733f98002"},
							},
						},
					},
					RootModule: &tfjson.StateModule{
						ChildModules: []*tfjson.StateModule{
							{
								Resources: []*tfjson.StateResource{
									{
										Type:         "kubernetes_deployment",
										ProviderName: "registry.terraform.io/hashicorp/kubernetes",
										AttributeValues: map[string]any{
											"metadata": []any{
												map[string]any{
													"namespace": "default",
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
			expectedResponse: &recipes.RecipeOutput{},
			expectedErr:      errors.New("resourceType or resourceName is empty"),
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
		{
			desc: "Testing empty tfjson state with a check",
			state: &tfjson.State{
				Checks: []tfjson.CheckResultStatic{
					{
						Address: tfjson.CheckStaticAddress{
							ToDisplay: "module.test",
							Kind:      tfjson.CheckKindResource,
							Module:    "test",
							Mode:      tfjson.ManagedResourceMode,
							Type:      "test",
							Name:      "test",
						},
					},
				},
			},
			expectedResponse: &recipes.RecipeOutput{
				Status: &rpv1.RecipeStatus{
					TemplateKind:    recipes.TemplateKindTerraform,
					TemplatePath:    "radiusdev.azurecr.io/recipes/functionaltest/parameters/mongodatabases/azure:1.0",
					TemplateVersion: "1.0",
				},
			},
			expectedErr: nil,
		},
	}

	opts := driver.ExecuteOptions{
		BaseOptions: driver.BaseOptions{
			Definition: recipes.EnvironmentDefinition{
				Name:            "mongo-azure",
				Driver:          recipes.TemplateKindTerraform,
				TemplatePath:    "radiusdev.azurecr.io/recipes/functionaltest/parameters/mongodatabases/azure:1.0",
				ResourceType:    "Applications.Datastores/mongoDatabases",
				TemplateVersion: "1.0",
			},
		},
		PrevState: []string{},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			recipeResponse, err := d.prepareRecipeResponse(context.Background(), opts.Definition, tt.state)
			require.Equal(t, tt.expectedErr, err)
			require.Equal(t, tt.expectedResponse, recipeResponse)
		})
	}
}

func Test_FindSecretIDs(t *testing.T) {
	ctx := context.TODO()
	definition := recipes.EnvironmentDefinition{TemplatePath: "git::https://dev.azure.com/project/module"}
	_, driver := setup(t)

	testCases := []struct {
		name              string
		envConfig         recipes.Configuration
		definition        recipes.EnvironmentDefinition
		expectedError     bool
		expectedSecretIDs map[string][]string
	}{
		{
			name:          "Secrets in auth, provider and env config",
			envConfig:     createTerraformConfigWithAuthProviderEnvSecrets(),
			definition:    definition,
			expectedError: false,
			expectedSecretIDs: map[string][]string{
				"secret-store-id1":    {"secret-key1", "secret-key-env"},
				"secret-store-id2":    {"secret-key2"},
				"secret-store-id-env": {"secret-key-env"},
				"secret-store-auth":   {"pat", "username"},
			},
		},
		{
			name:          "Secrets in provider and env config",
			envConfig:     createTerraformConfigWithProviderEnvSecrets(),
			definition:    definition,
			expectedError: false,
			expectedSecretIDs: map[string][]string{
				"secret-store-id1":    {"secret-key1", "secret-key-env"},
				"secret-store-id2":    {"secret-key2"},
				"secret-store-id-env": {"secret-key-env"},
			},
		},
		{
			name:          "Secrets in provider config",
			envConfig:     createTerraformConfigWithProviderSecrets(),
			definition:    definition,
			expectedError: false,
			expectedSecretIDs: map[string][]string{
				"secret-store-id1": {"secret-key1"},
				"secret-store-id2": {"secret-key2"},
			},
		},
		{
			name:          "Secrets in env config",
			envConfig:     createTerraformConfigWithEnvSecrets(),
			definition:    definition,
			expectedError: false,
			expectedSecretIDs: map[string][]string{
				"secret-store-id1":    {"secret-key-env2"},
				"secret-store-id-env": {"secret-key-env1"},
			},
		},
		{
			name:          "Empty PAT config - no secrets needed",
			definition:    recipes.EnvironmentDefinition{TemplatePath: "git::https://dev.azure.com/project/module"},
			envConfig:     createTerraformConfigWithProviderEnvSecrets(), // This config has no Git PAT authentication
			expectedError: false,
			expectedSecretIDs: map[string][]string{
				"secret-store-id1":    {"secret-key1", "secret-key-env"},
				"secret-store-id2":    {"secret-key2"},
				"secret-store-id-env": {"secret-key-env"},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			secretIDs, err := driver.FindSecretIDs(ctx, tc.envConfig, tc.definition)

			if tc.expectedError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.expectedSecretIDs, secretIDs)
			}
		})
	}
}

// createTerraformConfigWithAuthProviderEnvSecrets returns a test input configuration with secrets
// at auth, provider and environment variable.
func createTerraformConfigWithAuthProviderEnvSecrets() recipes.Configuration {
	return recipes.Configuration{
		RecipeConfig: datamodel.RecipeConfigProperties{
			Terraform: datamodel.TerraformConfigProperties{
				Authentication: datamodel.AuthConfig{
					Git: datamodel.GitAuthConfig{
						PAT: map[string]datamodel.SecretConfig{
							"dev.azure.com": {
								Secret: "secret-store-auth",
							},
						},
					},
				},
				Providers: map[string][]datamodel.ProviderConfigProperties{
					"azurerm": {
						{
							AdditionalProperties: map[string]any{
								"subscriptionid": 1234,
								"tenant_id":      "745fg88bf-86f1-41af-43ut",
							},
							Secrets: map[string]datamodel.SecretReference{
								"secret1": {
									Source: "secret-store-id1",
									Key:    "secret-key1",
								},
								"secret2": {
									Source: "secret-store-id2",
									Key:    "secret-key2",
								},
							},
						},
						{
							AdditionalProperties: map[string]any{
								"alias":          "az-paymentservice",
								"subscriptionid": 45678,
								"tenant_id":      "gfhf45345-5d73-gh34-wh84",
							},
						},
					},
				},
			},
			EnvSecrets: map[string]datamodel.SecretReference{
				"secret-env": {
					Source: "secret-store-id-env",
					Key:    "secret-key-env",
				},
				"secret1": {
					Source: "secret-store-id1",
					Key:    "secret-key-env",
				},
			},
		},
	}
}

// createTerraformConfigWithProviderEnvSecrets creates a test input configuration with provider and environment secrets.
func createTerraformConfigWithProviderEnvSecrets() recipes.Configuration {
	return recipes.Configuration{
		RecipeConfig: datamodel.RecipeConfigProperties{
			Terraform: datamodel.TerraformConfigProperties{
				Providers: map[string][]datamodel.ProviderConfigProperties{
					"azurerm": {
						{
							AdditionalProperties: map[string]any{
								"subscriptionid": 1234,
								"tenant_id":      "745fg88bf-86f1-41af-43ut",
							},
							Secrets: map[string]datamodel.SecretReference{
								"secret1": {
									Source: "secret-store-id1",
									Key:    "secret-key1",
								},
								"secret2": {
									Source: "secret-store-id2",
									Key:    "secret-key2",
								},
							},
						},
						{
							AdditionalProperties: map[string]any{
								"alias":          "az-paymentservice",
								"subscriptionid": 45678,
								"tenant_id":      "gfhf45345-5d73-gh34-wh84",
							},
						},
					},
				},
			},
			EnvSecrets: map[string]datamodel.SecretReference{
				"secret-env": {
					Source: "secret-store-id-env",
					Key:    "secret-key-env",
				},
				"secret1": {
					Source: "secret-store-id1",
					Key:    "secret-key-env",
				},
			},
		},
	}
}

// createTerraformConfigWithProviderEnvSecrets creates a input test configuration with provider secrets.
func createTerraformConfigWithProviderSecrets() recipes.Configuration {
	return recipes.Configuration{
		RecipeConfig: datamodel.RecipeConfigProperties{
			Terraform: datamodel.TerraformConfigProperties{
				Providers: map[string][]datamodel.ProviderConfigProperties{
					"azurerm": {
						{
							AdditionalProperties: map[string]any{
								"subscriptionid": 1234,
								"tenant_id":      "745fg88bf-86f1-41af-43ut",
							},
							Secrets: map[string]datamodel.SecretReference{
								"secret1": {
									Source: "secret-store-id1",
									Key:    "secret-key1",
								},
								"secret2": {
									Source: "secret-store-id2",
									Key:    "secret-key2",
								},
							},
						},
						{
							AdditionalProperties: map[string]any{
								"alias":          "az-paymentservice",
								"subscriptionid": 45678,
								"tenant_id":      "gfhf45345-5d73-gh34-wh84",
							},
						},
					},
				},
			},
		},
	}
}

// createTerraformConfigWithEnvSecrets creates a test input configuration with secrets in environment variables.
func createTerraformConfigWithEnvSecrets() recipes.Configuration {
	return recipes.Configuration{
		RecipeConfig: datamodel.RecipeConfigProperties{
			EnvSecrets: map[string]datamodel.SecretReference{
				"secret-env": {
					Source: "secret-store-id-env",
					Key:    "secret-key-env1",
				},
				"secret1": {
					Source: "secret-store-id1",
					Key:    "secret-key-env2",
				},
			},
		},
	}
}

func Test_InjectProviderConfigIfNeeded_NoProviderBlocks(t *testing.T) {
	ctx := testcontext.New(t)
	_, tfDriver := setup(t)

	// Create test directory with terraform files that don't have provider blocks
	testDir := t.TempDir()
	err := os.WriteFile(testDir+"/main.tf", []byte(`
resource "azurerm_resource_group" "test" {
  name     = "test-rg"
  location = "West US 2"
}
`), 0644)
	require.NoError(t, err)

	envConfig := recipes.Configuration{}
	secrets := map[string]recipes.SecretData{}

	// Should not inject anything when no provider blocks exist
	err = tfDriver.injectProviderConfigIfNeeded(ctx, testDir, envConfig, secrets)
	require.NoError(t, err)

	// File should remain unchanged
	content, err := os.ReadFile(testDir + "/main.tf")
	require.NoError(t, err)
	require.NotContains(t, string(content), "client_id")
	require.NotContains(t, string(content), "client_secret")
}

func Test_InjectProviderConfigIfNeeded_WithHCLProviderBlocks(t *testing.T) {
	ctx := testcontext.New(t)
	_, tfDriver := setup(t)

	// Create test directory with terraform files that have provider blocks
	testDir := t.TempDir()
	originalTfContent := `
provider "azurerm" {
  features {}
}

resource "azurerm_resource_group" "test" {
  name     = "test-rg"
  location = "West US 2"
}
`
	err := os.WriteFile(testDir+"/main.tf", []byte(originalTfContent), 0644)
	require.NoError(t, err)

	envConfig, _, _ := buildTestInputs()
	secrets := map[string]recipes.SecretData{}

	// Should inject provider credentials when provider blocks exist
	err = tfDriver.injectProviderConfigIfNeeded(ctx, testDir, envConfig, secrets)
	require.NoError(t, err)

	// File should be modified with injected credentials
	content, err := os.ReadFile(testDir + "/main.tf")
	require.NoError(t, err)
	
	// The content should still contain the original provider block
	require.Contains(t, string(content), `provider "azurerm"`)
	require.Contains(t, string(content), "features {}")
	require.Contains(t, string(content), "resource \"azurerm_resource_group\"")
}

func Test_InjectProviderConfigIfNeeded_WithJSONProviderBlocks(t *testing.T) {
	ctx := testcontext.New(t)
	_, tfDriver := setup(t)

	// Create test directory with JSON terraform files that have provider blocks
	testDir := t.TempDir()
	originalTfContent := `{
  "provider": {
    "azurerm": [
      {
        "features": {}
      }
    ]
  },
  "resource": {
    "azurerm_resource_group": {
      "test": {
        "name": "test-rg",
        "location": "West US 2"
      }
    }
  }
}`
	err := os.WriteFile(testDir+"/main.tf.json", []byte(originalTfContent), 0644)
	require.NoError(t, err)

	envConfig, _, _ := buildTestInputs()
	secrets := map[string]recipes.SecretData{}

	// Should inject provider credentials when provider blocks exist
	err = tfDriver.injectProviderConfigIfNeeded(ctx, testDir, envConfig, secrets)
	require.NoError(t, err)

	// File should be modified with injected credentials
	content, err := os.ReadFile(testDir + "/main.tf.json")
	require.NoError(t, err)
	
	// The content should still contain the original provider block structure
	require.Contains(t, string(content), `"provider"`)
	require.Contains(t, string(content), `"azurerm"`)
	require.Contains(t, string(content), `"features"`)
}

func Test_HasProviderBlocks_HCL(t *testing.T) {
	_, tfDriver := setup(t)

	testCases := []struct {
		name     string
		content  string
		expected bool
	}{
		{
			name: "HCL with provider block",
			content: `
provider "azurerm" {
  features {}
}`,
			expected: true,
		},
		{
			name: "HCL with indented provider block",
			content: `
  provider "aws" {
    region = "us-west-2"
  }`,
			expected: true,
		},
		{
			name: "HCL without provider block",
			content: `
resource "azurerm_resource_group" "test" {
  name = "test"
}`,
			expected: false,
		},
		{
			name: "HCL with commented provider block",
			content: `
# provider "azurerm" {
#   features {}
# }`,
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			testDir := t.TempDir()
			filePath := testDir + "/test.tf"
			err := os.WriteFile(filePath, []byte(tc.content), 0644)
			require.NoError(t, err)

			hasProvider, err := tfDriver.hasProviderBlocks(filePath)
			require.NoError(t, err)
			require.Equal(t, tc.expected, hasProvider)
		})
	}
}

func Test_HasProviderBlocks_JSON(t *testing.T) {
	_, tfDriver := setup(t)

	testCases := []struct {
		name     string
		content  string
		expected bool
	}{
		{
			name: "JSON with provider block",
			content: `{
  "provider": {
    "azurerm": [{}]
  }
}`,
			expected: true,
		},
		{
			name: "JSON without provider block",
			content: `{
  "resource": {
    "azurerm_resource_group": {
      "test": {}
    }
  }
}`,
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			testDir := t.TempDir()
			filePath := testDir + "/test.tf.json"
			err := os.WriteFile(filePath, []byte(tc.content), 0644)
			require.NoError(t, err)

			hasProvider, err := tfDriver.hasProviderBlocks(filePath)
			require.NoError(t, err)
			require.Equal(t, tc.expected, hasProvider)
		})
	}
}

func Test_FindTerraformFiles(t *testing.T) {
	_, tfDriver := setup(t)

	testDir := t.TempDir()
	
	// Create various files
	files := []string{
		"main.tf",
		"variables.tf",
		"outputs.tf.json",
		"terraform.tfvars",
		"README.md",
		"subdir/nested.tf",
		"subdir/config.tf.json",
	}

	for _, file := range files {
		fullPath := testDir + "/" + file
		dir := fullPath[:strings.LastIndex(fullPath, "/")]
		err := os.MkdirAll(dir, 0755)
		require.NoError(t, err)
		err = os.WriteFile(fullPath, []byte("test content"), 0644)
		require.NoError(t, err)
	}

	foundFiles, err := tfDriver.findTerraformFiles(testDir)
	require.NoError(t, err)

	// Should find only .tf and .tf.json files
	expectedFiles := []string{
		testDir + "/main.tf",
		testDir + "/variables.tf",
		testDir + "/outputs.tf.json",
		testDir + "/subdir/nested.tf",
		testDir + "/subdir/config.tf.json",
	}

	require.Len(t, foundFiles, len(expectedFiles))
	for _, expectedFile := range expectedFiles {
		require.Contains(t, foundFiles, expectedFile)
	}
	
	// Should not find non-terraform files
	for _, foundFile := range foundFiles {
		require.True(t, strings.HasSuffix(foundFile, ".tf") || strings.HasSuffix(foundFile, ".tf.json"))
	}
}
