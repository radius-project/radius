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

package engine

import (
	"errors"
	"fmt"
	"testing"

	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/recipes/configloader"
	recipedriver "github.com/radius-project/radius/pkg/recipes/driver"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/radius-project/radius/test/testcontext"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func setup(t *testing.T) (engine, configloader.MockConfigurationLoader, recipedriver.MockDriver, recipedriver.MockDriverWithSecrets, configloader.MockSecretsLoader) {
	ctrl := gomock.NewController(t)
	cfgLoader := configloader.NewMockConfigurationLoader(ctrl)
	secretLoader := configloader.NewMockSecretsLoader(ctrl)
	mDriver := recipedriver.NewMockDriver(ctrl)
	mDriverWithSecrets := recipedriver.NewMockDriverWithSecrets(ctrl)
	options := Options{
		ConfigurationLoader: cfgLoader,
		SecretsLoader:       secretLoader,
		Drivers: map[string]recipedriver.Driver{
			recipes.TemplateKindBicep:     mDriver,
			recipes.TemplateKindTerraform: mDriverWithSecrets,
		},
	}
	engine := engine{
		options: options,
	}
	return engine, *cfgLoader, *mDriver, *mDriverWithSecrets, *secretLoader
}

func Test_Engine_Execute_Success(t *testing.T) {
	recipeMetadata := recipes.ResourceMetadata{
		Name:          "mongo-azure",
		ApplicationID: "/planes/radius/local/resourcegroups/test-rg/providers/applications.core/applications/app1",
		EnvironmentID: "/planes/radius/local/resourcegroups/test-rg/providers/applications.core/environments/env1",
		ResourceID:    "/planes/radius/local/resourceGroups/test-rg/providers/Microsoft.Resources/deployments/recipe",
		Parameters: map[string]any{
			"resourceName": "resource1",
		},
		ConnectedResourcesProperties: map[string]map[string]any{
			"database": {
				"name": "db",
			},
		},
	}
	prevState := []string{
		"/subscriptions/test-sub/resourceGroups/test-rg/providers/System.Test/testResources/test1",
	}
	envConfig := &recipes.Configuration{
		Runtime: recipes.RuntimeConfiguration{
			Kubernetes: &recipes.KubernetesRuntime{
				Namespace: "default",
			},
		},
		Providers: datamodel.Providers{
			Azure: datamodel.ProvidersAzure{
				Scope: "scope",
			},
		},
	}
	recipeResult := &recipes.RecipeOutput{
		Resources: []string{"mongoStorageAccount", "mongoDatabase"},
		Secrets: map[string]any{
			"connectionString": "mongodb://testUser:testPassword@testAccount1.mongo.cosmos.azure.com:10255",
		},
		Values: map[string]any{
			"host": "testAccount1.mongo.cosmos.azure.com",
			"port": 10255,
		},
	}
	recipeDefinition := &recipes.EnvironmentDefinition{
		Driver:       recipes.TemplateKindBicep,
		TemplatePath: "ghcr.io/radius-project/dev/recipes/functionaltest/basic/mongodatabases/azure:1.0",
		ResourceType: "Applications.Datastores/mongoDatabases",
	}
	ctx := testcontext.New(t)
	engine, configLoader, driver, _, _ := setup(t)

	configLoader.EXPECT().
		LoadConfiguration(ctx, recipeMetadata).
		Times(1).
		Return(envConfig, nil)
	configLoader.EXPECT().
		LoadRecipe(ctx, &recipeMetadata).
		Times(1).
		Return(recipeDefinition, nil)
	driver.EXPECT().
		Execute(ctx, recipedriver.ExecuteOptions{
			BaseOptions: recipedriver.BaseOptions{
				Configuration: *envConfig,
				Recipe:        recipeMetadata,
				Definition:    *recipeDefinition,
			},
			PrevState: prevState,
		}).
		Times(1).
		Return(recipeResult, nil)

	result, err := engine.Execute(ctx, ExecuteOptions{
		BaseOptions: BaseOptions{
			Recipe: recipeMetadata,
		},
		PreviousState: prevState,
	})
	require.NoError(t, err)
	require.Equal(t, result, recipeResult)
}

func Test_Engine_Execute_SimulatedEnv_Success(t *testing.T) {
	recipeMetadata := recipes.ResourceMetadata{
		Name:          "mongo-azure",
		ApplicationID: "/planes/radius/local/resourcegroups/test-rg/providers/applications.core/applications/app1",
		EnvironmentID: "/planes/radius/local/resourcegroups/test-rg/providers/applications.core/environments/env1",
		ResourceID:    "/planes/radius/local/resourceGroups/test-rg/providers/Microsoft.Resources/deployments/recipe",
		Parameters: map[string]any{
			"resourceName": "resource1",
		},
	}

	prevState := []string{
		"/subscriptions/test-sub/resourceGroups/test-rg/providers/System.Test/testResources/test1",
	}

	envConfig := &recipes.Configuration{
		Runtime: recipes.RuntimeConfiguration{
			Kubernetes: &recipes.KubernetesRuntime{
				Namespace: "default",
			},
		},
		Providers: datamodel.Providers{
			Azure: datamodel.ProvidersAzure{
				Scope: "scope",
			},
		},
		Simulated: true,
	}

	ctx := testcontext.New(t)
	engine, configLoader, _, _, _ := setup(t)

	configLoader.EXPECT().
		LoadConfiguration(ctx, recipeMetadata).
		Times(1).
		Return(envConfig, nil)

	// Note: LoadRecipe is not called as the environment is simulated

	result, err := engine.Execute(ctx, ExecuteOptions{
		BaseOptions: BaseOptions{
			Recipe: recipeMetadata,
		},
		PreviousState: prevState,
	})
	require.NoError(t, err)
	require.Nil(t, result)
}

func Test_Engine_Execute_Failure(t *testing.T) {
	recipeMetadata := recipes.ResourceMetadata{
		Name:          "mongo-azure",
		ApplicationID: "/planes/radius/local/resourcegroups/test-rg/providers/applications.core/applications/app1",
		EnvironmentID: "/planes/radius/local/resourcegroups/test-rg/providers/applications.core/environments/env1",
		ResourceID:    "/planes/radius/local/resourceGroups/test-rg/providers/Microsoft.Resources/deployments/recipe",
		Parameters: map[string]any{
			"resourceName": "resource1",
		},
	}
	prevState := []string{
		"/subscriptions/test-sub/resourceGroups/test-rg/providers/System.Test/testResources/test1",
	}
	envConfig := &recipes.Configuration{
		Runtime: recipes.RuntimeConfiguration{
			Kubernetes: &recipes.KubernetesRuntime{
				Namespace: "default",
			},
		},
		Providers: datamodel.Providers{
			Azure: datamodel.ProvidersAzure{
				Scope: "scope",
			},
		},
	}
	recipeDefinition := &recipes.EnvironmentDefinition{
		Driver:       recipes.TemplateKindBicep,
		TemplatePath: "ghcr.io/radius-project/dev/recipes/functionaltest/basic/mongodatabases/azure:1.0",
		ResourceType: "Applications.Datastores/mongoDatabases",
	}
	ctx := testcontext.New(t)
	engine, configLoader, driver, _, _ := setup(t)

	configLoader.EXPECT().
		LoadConfiguration(ctx, recipeMetadata).
		Times(1).
		Return(envConfig, nil)
	configLoader.EXPECT().
		LoadRecipe(ctx, &recipeMetadata).
		Times(1).
		Return(recipeDefinition, nil)
	driver.EXPECT().
		Execute(ctx, recipedriver.ExecuteOptions{
			BaseOptions: recipedriver.BaseOptions{
				Configuration: *envConfig,
				Recipe:        recipeMetadata,
				Definition:    *recipeDefinition,
			},
			PrevState: prevState,
		}).
		Times(1).
		Return(nil, errors.New("failed to execute recipe"))

	result, err := engine.Execute(ctx, ExecuteOptions{
		BaseOptions: BaseOptions{
			Recipe: recipeMetadata,
		},
		PreviousState: prevState,
	})
	require.Nil(t, result)
	require.Error(t, err)
	require.Equal(t, err.Error(), "failed to execute recipe")
}

func Test_Engine_Terraform_Success(t *testing.T) {
	recipeMetadata := recipes.ResourceMetadata{
		Name:          "mongo-azure",
		ApplicationID: "/planes/radius/local/resourcegroups/test-rg/providers/applications.core/applications/app1",
		EnvironmentID: "/planes/radius/local/resourcegroups/test-rg/providers/applications.core/environments/env1",
		ResourceID:    "/planes/deployments/local/resourceGroups/test-rg/providers/Microsoft.Resources/deployments/recipe",
		Parameters: map[string]any{
			"resourceName": "resource1",
		},
	}
	prevState := []string{
		"/subscriptions/test-sub/resourceGroups/test-rg/providers/System.Test/testResources/test1",
	}
	envConfig := &recipes.Configuration{
		Runtime: recipes.RuntimeConfiguration{
			Kubernetes: &recipes.KubernetesRuntime{
				Namespace: "default",
			},
		},
		Providers: datamodel.Providers{
			Azure: datamodel.ProvidersAzure{
				Scope: "scope",
			},
		},
	}
	recipeResult := &recipes.RecipeOutput{
		Resources: []string{"mongoStorageAccount", "mongoDatabase"},
		Secrets: map[string]any{
			"connectionString": "mongodb://testUser:testPassword@testAccount1.mongo.cosmos.azure.com:10255",
		},
		Values: map[string]any{
			"host": "testAccount1.mongo.cosmos.azure.com",
			"port": 10255,
		},
	}
	recipeDefinition := &recipes.EnvironmentDefinition{
		Driver:          recipes.TemplateKindTerraform,
		TemplatePath:    "Azure/redis/azurerm",
		TemplateVersion: "1.1.0",
		ResourceType:    "Applications.Datastores/mongoDatabases",
	}
	ctx := testcontext.New(t)
	engine, configLoader, _, driverWithSecrets, _ := setup(t)

	configLoader.EXPECT().
		LoadConfiguration(ctx, recipeMetadata).
		Times(1).
		Return(envConfig, nil)
	configLoader.EXPECT().
		LoadRecipe(ctx, &recipeMetadata).
		Times(1).
		Return(recipeDefinition, nil)
	driverWithSecrets.EXPECT().
		FindSecretIDs(ctx, *envConfig, *recipeDefinition).
		Times(1).
		Return(nil, nil)
	driverWithSecrets.EXPECT().
		Execute(ctx, recipedriver.ExecuteOptions{
			BaseOptions: recipedriver.BaseOptions{
				Configuration: *envConfig,
				Recipe:        recipeMetadata,
				Definition:    *recipeDefinition,
			},
			PrevState: prevState,
		}).
		Times(1).
		Return(recipeResult, nil)

	result, err := engine.Execute(ctx, ExecuteOptions{
		BaseOptions: BaseOptions{
			Recipe: recipeMetadata,
		},
		PreviousState: prevState,
	})
	require.NoError(t, err)
	require.Equal(t, result, recipeResult)
}
func Test_Engine_Terraform_Failure(t *testing.T) {
	tests := []struct {
		name                   string
		errFindSecretRefs      error
		errLoadSecrets         error
		errLoadSecretsNotFound error
		errExecute             error
		expectedErrMsg         string
	}{
		{
			name:              "find secret references failed",
			errFindSecretRefs: fmt.Errorf("failed to parse git url %s", "git://https://dev.azure.com/mongo-recipe/recipe"),
			expectedErrMsg:    "failed to parse git url git://https://dev.azure.com/mongo-recipe/recipe",
		},
		{
			name:           "failed loading secrets",
			errLoadSecrets: fmt.Errorf("%q is a valid resource id but does not refer to a resource", "secretstoreid1"),
			expectedErrMsg: "code LoadSecretsFailed: err failed to fetch secrets for Terraform recipe git://https://dev.azure.com/mongo-recipe/recipe deployment: \"secretstoreid1\" is a valid resource id but does not refer to a resource",
		},
		{
			name:                   "failed loading secrets - secret store id not found",
			errLoadSecretsNotFound: fmt.Errorf("a secret key was not found in secret store 'secretstoreid1'"),
			expectedErrMsg:         "code LoadSecretsFailed: err failed to fetch secrets for Terraform recipe git://https://dev.azure.com/mongo-recipe/recipe deployment: a secret key was not found in secret store 'secretstoreid1'",
		},
		{
			name:           "find secret references failed",
			errExecute:     errors.New("failed to add git config"),
			expectedErrMsg: "failed to add git config",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			recipeMetadata := recipes.ResourceMetadata{
				Name:          "mongo-azure",
				ApplicationID: "/planes/radius/local/resourcegroups/test-rg/providers/applications.core/applications/app1",
				EnvironmentID: "/planes/radius/local/resourcegroups/test-rg/providers/applications.core/environments/env1",
				ResourceID:    "/planes/radius/local/resourceGroups/test-rg/providers/Microsoft.Resources/deployments/recipe",
				Parameters: map[string]any{
					"resourceName": "resource1",
				},
			}
			envConfig := &recipes.Configuration{
				Runtime: recipes.RuntimeConfiguration{
					Kubernetes: &recipes.KubernetesRuntime{
						Namespace: "default",
					},
				},
				Providers: datamodel.Providers{
					Azure: datamodel.ProvidersAzure{
						Scope: "scope",
					},
				},
				RecipeConfig: datamodel.RecipeConfigProperties{
					Terraform: datamodel.TerraformConfigProperties{
						Authentication: datamodel.AuthConfig{
							Git: datamodel.GitAuthConfig{
								PAT: map[string]datamodel.SecretConfig{
									"dev.azure.com": {
										Secret: "/planes/radius/local/resourcegroups/default/providers/Applications.Core/secretStores/azdevopsgit",
									},
								},
							},
						},
					},
				},
			}
			recipeResult := &recipes.RecipeOutput{
				Resources: []string{"mongoStorageAccount", "mongoDatabase"},
				Secrets: map[string]any{
					"connectionString": "mongodb://testUser:testPassword@testAccount1.mongo.cosmos.azure.com:10255",
				},
				Values: map[string]any{
					"host": "testAccount1.mongo.cosmos.azure.com",
					"port": 10255,
				},
			}
			recipeDefinition := &recipes.EnvironmentDefinition{
				Driver:       recipes.TemplateKindTerraform,
				TemplatePath: "git://https://dev.azure.com/mongo-recipe/recipe",
				ResourceType: "Applications.Datastores/mongoDatabases",
			}
			prevState := []string{
				"/subscriptions/test-sub/resourceGroups/test-rg/providers/System.Test/testResources/test1",
			}
			ctx := testcontext.New(t)
			engine, configLoader, _, driverWithSecrets, secretsLoader := setup(t)
			configLoader.EXPECT().
				LoadConfiguration(ctx, recipeMetadata).
				Times(1).
				Return(envConfig, nil)
			configLoader.EXPECT().
				LoadRecipe(ctx, &recipeMetadata).
				Times(1).
				Return(recipeDefinition, nil)

			if tc.errFindSecretRefs != nil {
				driverWithSecrets.EXPECT().
					FindSecretIDs(ctx, *envConfig, *recipeDefinition).
					Times(1).
					Return(nil, tc.errFindSecretRefs)
			} else {
				driverWithSecrets.EXPECT().
					FindSecretIDs(ctx, *envConfig, *recipeDefinition).
					Times(1).
					Return(map[string][]string{"/planes/radius/local/resourcegroups/default/providers/Applications.Core/secretStores/azdevopsgit": {"username", "pat"}}, nil)

				if tc.errLoadSecrets != nil {
					secretsLoader.EXPECT().
						LoadSecrets(ctx, gomock.Any()).
						Times(1).
						Return(nil, tc.errLoadSecrets)
				} else if tc.errLoadSecretsNotFound != nil {
					secretsLoader.EXPECT().
						LoadSecrets(ctx, gomock.Any()).
						Times(1).
						Return(nil, tc.errLoadSecretsNotFound)
				} else {
					secretsLoader.EXPECT().
						LoadSecrets(ctx, gomock.Any()).
						Times(1).
						Return(nil, nil)
					if tc.errExecute != nil {
						driverWithSecrets.EXPECT().
							Execute(ctx, recipedriver.ExecuteOptions{
								BaseOptions: recipedriver.BaseOptions{
									Configuration: *envConfig,
									Recipe:        recipeMetadata,
									Definition:    *recipeDefinition,
								},
								PrevState: prevState,
							}).
							Times(1).
							Return(nil, tc.errExecute)
					} else {
						driverWithSecrets.EXPECT().
							Execute(ctx, recipedriver.ExecuteOptions{
								BaseOptions: recipedriver.BaseOptions{
									Configuration: *envConfig,
									Recipe:        recipeMetadata,
									Definition:    *recipeDefinition,
								},
								PrevState: prevState,
							}).
							Times(1).
							Return(recipeResult, nil)
					}
				}
			}

			result, err := engine.Execute(ctx, ExecuteOptions{
				BaseOptions: BaseOptions{
					Recipe: recipeMetadata,
				},
				PreviousState: prevState,
			})
			if tc.errFindSecretRefs != nil || tc.errLoadSecrets != nil || tc.errExecute != nil || tc.errLoadSecretsNotFound != nil {
				require.EqualError(t, err, tc.expectedErrMsg)
			} else {
				require.NoError(t, err)
				require.Equal(t, result, recipeResult)
			}
		})
	}
}

func Test_Engine_InvalidDriver(t *testing.T) {
	ctx := testcontext.New(t)
	engine, configLoader, _, _, _ := setup(t)

	envConfig := &recipes.Configuration{
		Runtime: recipes.RuntimeConfiguration{
			Kubernetes: &recipes.KubernetesRuntime{
				Namespace: "default",
			},
		},
		Providers: datamodel.Providers{
			Azure: datamodel.ProvidersAzure{
				Scope: "scope",
			},
		},
	}

	recipeDefinition := &recipes.EnvironmentDefinition{
		Driver:       "invalid",
		TemplatePath: "ghcr.io/radius-project/dev/recipes/functionaltest/basic/mongodatabases/azure:1.0",
		ResourceType: "Applications.Datastores/mongoDatabases",
	}

	recipeMetadata := recipes.ResourceMetadata{
		Name:          "mongo-azure",
		ApplicationID: "/planes/radius/local/resourcegroups/test-rg/providers/applications.core/applications/app1",
		EnvironmentID: "/planes/radius/local/resourcegroups/test-rg/providers/applications.core/environments/env1",
		ResourceID:    "/planes/radius/local/resourceGroups/test-rg/providers/Microsoft.Resources/deployments/recipe",
		Parameters: map[string]any{
			"resourceName": "resource1",
		},
	}
	prevState := []string{
		"/subscriptions/test-sub/resourceGroups/test-rg/providers/System.Test/testResources/test1",
	}

	configLoader.EXPECT().
		LoadConfiguration(ctx, recipeMetadata).
		Times(1).
		Return(envConfig, nil)

	configLoader.EXPECT().
		LoadRecipe(ctx, &recipeMetadata).
		Times(1).
		Return(recipeDefinition, nil)
	_, err := engine.Execute(ctx, ExecuteOptions{
		BaseOptions: BaseOptions{
			Recipe: recipeMetadata,
		},
		PreviousState: prevState,
	})
	require.Error(t, err)
	require.Equal(t, "code DriverNotFoundFailure: err could not find driver `invalid`", err.Error())
}

func Test_Engine_Lookup_Error(t *testing.T) {
	ctx := testcontext.New(t)
	engine, configLoader, _, _, _ := setup(t)

	envConfig := &recipes.Configuration{
		Runtime: recipes.RuntimeConfiguration{
			Kubernetes: &recipes.KubernetesRuntime{
				Namespace: "default",
			},
		},
		Providers: datamodel.Providers{
			Azure: datamodel.ProvidersAzure{
				Scope: "scope",
			},
		},
	}

	recipeMetadata := recipes.ResourceMetadata{
		Name:          "mongo-azure",
		ApplicationID: "/planes/radius/local/resourcegroups/test-rg/providers/applications.core/applications/app1",
		EnvironmentID: "/planes/radius/local/resourcegroups/test-rg/providers/applications.core/environments/env1",
		ResourceID:    "/planes/radius/local/resourceGroups/test-rg/providers/Microsoft.Resources/deployments/recipe",
		Parameters: map[string]any{
			"resourceName": "resource1",
		},
	}
	prevState := []string{
		"/subscriptions/test-sub/resourceGroups/test-rg/providers/System.Test/testResources/test1",
	}

	configLoader.EXPECT().
		LoadConfiguration(ctx, recipeMetadata).
		Times(1).
		Return(envConfig, nil)

	configLoader.EXPECT().
		LoadRecipe(ctx, &recipeMetadata).
		Times(1).
		Return(nil, errors.New("could not find recipe mongo-azure in environment env1"))

	_, err := engine.Execute(ctx, ExecuteOptions{
		BaseOptions: BaseOptions{
			Recipe: recipeMetadata,
		},
		PreviousState: prevState,
	})
	require.Error(t, err)
}

func Test_Engine_Load_Error(t *testing.T) {
	ctx := testcontext.New(t)
	engine, configLoader, _, _, _ := setup(t)

	recipeMetadata := recipes.ResourceMetadata{
		Name:          "mongo-azure",
		ApplicationID: "/planes/radius/local/resourcegroups/test-rg/providers/applications.core/applications/app1",
		EnvironmentID: "/planes/radius/local/resourcegroups/test-rg/providers/applications.core/environments/env1",
		ResourceID:    "/planes/radius/local/resourceGroups/test-rg/providers/Microsoft.Resources/deployments/recipe",
		Parameters: map[string]any{
			"resourceName": "resource1",
		},
	}
	prevState := []string{
		"/subscriptions/test-sub/resourceGroups/test-rg/providers/System.Test/testResources/test1",
	}

	configLoader.EXPECT().
		LoadConfiguration(ctx, recipeMetadata).
		Times(1).
		Return(nil, errors.New("unable to fetch namespace information"))

	_, err := engine.Execute(ctx, ExecuteOptions{
		BaseOptions: BaseOptions{
			Recipe: recipeMetadata,
		},
		PreviousState: prevState,
	})
	require.Error(t, err)
}

func Test_Engine_Delete_Success(t *testing.T) {
	recipeMetadata, recipeDefinition, outputResources := getRecipeInputs()

	envConfig := &recipes.Configuration{
		Runtime: recipes.RuntimeConfiguration{
			Kubernetes: &recipes.KubernetesRuntime{
				Namespace: "default",
			},
		},
		Providers: datamodel.Providers{
			Azure: datamodel.ProvidersAzure{
				Scope: "scope",
			},
		},
	}

	ctx := testcontext.New(t)
	engine, configLoader, driver, _, _ := setup(t)

	configLoader.EXPECT().
		LoadRecipe(ctx, &recipeMetadata).
		Times(1).
		Return(&recipeDefinition, nil)

	configLoader.EXPECT().
		LoadConfiguration(ctx, recipeMetadata).
		Times(1).
		Return(envConfig, nil)

	driver.EXPECT().
		Delete(ctx, recipedriver.DeleteOptions{
			BaseOptions: recipedriver.BaseOptions{
				Configuration: *envConfig,
				Recipe:        recipeMetadata,
				Definition:    recipeDefinition,
			},
			OutputResources: outputResources,
		}).
		Times(1).
		Return(nil)

	err := engine.Delete(ctx, DeleteOptions{
		BaseOptions: BaseOptions{
			Recipe: recipeMetadata,
		},
		OutputResources: outputResources,
	})
	require.NoError(t, err)
}

func Test_Engine_Delete_SimulatedEnv_Success(t *testing.T) {
	recipeMetadata, _, outputResources := getRecipeInputs()

	envConfig := &recipes.Configuration{
		Runtime: recipes.RuntimeConfiguration{
			Kubernetes: &recipes.KubernetesRuntime{
				Namespace: "default",
			},
		},
		Providers: datamodel.Providers{
			Azure: datamodel.ProvidersAzure{
				Scope: "scope",
			},
		},
		Simulated: true,
	}

	ctx := testcontext.New(t)
	engine, configLoader, _, _, _ := setup(t)

	configLoader.EXPECT().
		LoadConfiguration(ctx, recipeMetadata).
		Times(1).
		Return(envConfig, nil)

	err := engine.Delete(ctx, DeleteOptions{
		BaseOptions: BaseOptions{
			Recipe: recipeMetadata,
		},
		OutputResources: outputResources,
	})
	require.NoError(t, err)
}

func Test_Engine_Delete_Error(t *testing.T) {
	recipeMetadata, recipeDefinition, outputResources := getRecipeInputs()

	envConfig := &recipes.Configuration{
		Runtime: recipes.RuntimeConfiguration{
			Kubernetes: &recipes.KubernetesRuntime{
				Namespace: "default",
			},
		},
		Providers: datamodel.Providers{
			Azure: datamodel.ProvidersAzure{
				Scope: "scope",
			},
		},
	}

	ctx := testcontext.New(t)
	engine, configLoader, driver, _, _ := setup(t)

	configLoader.EXPECT().
		LoadRecipe(ctx, &recipeMetadata).
		Times(1).
		Return(&recipeDefinition, nil)

	configLoader.EXPECT().
		LoadConfiguration(ctx, recipeMetadata).
		Times(1).
		Return(envConfig, nil)

	driver.EXPECT().
		Delete(ctx, recipedriver.DeleteOptions{
			BaseOptions: recipedriver.BaseOptions{
				Configuration: *envConfig,
				Recipe:        recipeMetadata,
				Definition:    recipeDefinition,
			},
			OutputResources: outputResources,
		}).
		Times(1).
		Return(fmt.Errorf("could not find API version for type %q, no supported API versions",
			outputResources[0].ID))

	err := engine.Delete(ctx, DeleteOptions{
		BaseOptions: BaseOptions{
			Recipe: recipeMetadata,
		},
		OutputResources: outputResources,
	})
	require.Error(t, err)
}

func Test_Delete_InvalidDriver(t *testing.T) {
	recipeMetadata, recipeDefinition, outputResources := getRecipeInputs()
	recipeDefinition.Driver = "invalid"

	ctx := testcontext.New(t)
	engine, configLoader, _, _, _ := setup(t)

	envConfig := &recipes.Configuration{
		Runtime: recipes.RuntimeConfiguration{
			Kubernetes: &recipes.KubernetesRuntime{
				Namespace: "default",
			},
		},
		Providers: datamodel.Providers{
			Azure: datamodel.ProvidersAzure{
				Scope: "scope",
			},
		},
	}

	configLoader.EXPECT().
		LoadConfiguration(ctx, recipeMetadata).
		Times(1).
		Return(envConfig, nil)

	configLoader.EXPECT().
		LoadRecipe(ctx, &recipeMetadata).
		Times(1).
		Return(&recipeDefinition, nil)
	err := engine.Delete(ctx, DeleteOptions{
		BaseOptions: BaseOptions{
			Recipe: recipeMetadata,
		},
		OutputResources: outputResources,
	})
	require.Error(t, err)
	require.Equal(t, "code DriverNotFoundFailure: err could not find driver `invalid`", err.Error())
}

func Test_Delete_Lookup_Error(t *testing.T) {
	ctx := testcontext.New(t)
	engine, configLoader, _, _, _ := setup(t)
	recipeMetadata, _, outputResources := getRecipeInputs()

	envConfig := &recipes.Configuration{
		Runtime: recipes.RuntimeConfiguration{
			Kubernetes: &recipes.KubernetesRuntime{
				Namespace: "default",
			},
		},
		Providers: datamodel.Providers{
			Azure: datamodel.ProvidersAzure{
				Scope: "scope",
			},
		},
	}

	configLoader.EXPECT().
		LoadConfiguration(ctx, recipeMetadata).
		Times(1).
		Return(envConfig, nil)

	configLoader.EXPECT().
		LoadRecipe(ctx, &recipeMetadata).
		Times(1).
		Return(nil, errors.New("could not find recipe mongo-azure in environment env1"))
	err := engine.Delete(ctx, DeleteOptions{
		BaseOptions: BaseOptions{
			Recipe: recipeMetadata,
		},
		OutputResources: outputResources,
	})
	require.Error(t, err)
}

func Test_Engine_GetRecipeMetadata_Success(t *testing.T) {
	recipeMetadata, recipeDefinition, _ := getRecipeInputs()
	envConfig := &recipes.Configuration{
		Runtime: recipes.RuntimeConfiguration{
			Kubernetes: &recipes.KubernetesRuntime{
				Namespace: "default",
			},
		},
		Providers: datamodel.Providers{
			Azure: datamodel.ProvidersAzure{
				Scope: "scope",
			},
		},
	}
	ctx := testcontext.New(t)
	engine, configLoader, driver, _, _ := setup(t)
	outputParams := map[string]any{"parameters": recipeDefinition.Parameters}

	configLoader.EXPECT().
		LoadConfiguration(ctx, recipeMetadata).
		Times(1).
		Return(envConfig, nil)
	driver.EXPECT().GetRecipeMetadata(ctx, recipedriver.BaseOptions{
		Recipe:        recipes.ResourceMetadata{},
		Definition:    recipeDefinition,
		Configuration: *envConfig,
	}).Times(1).Return(outputParams, nil)

	recipeData, err := engine.GetRecipeMetadata(ctx, GetRecipeMetadataOptions{
		BaseOptions: BaseOptions{
			Recipe: recipeMetadata,
		},
		RecipeDefinition: recipeDefinition,
	})
	require.NoError(t, err)
	require.Equal(t, outputParams, recipeData)
}
func Test_Engine_GetRecipeMetadata_Private_Module_Success(t *testing.T) {
	recipeMetadata := recipes.ResourceMetadata{
		Name:          "mongo-azure",
		ApplicationID: "/planes/radius/local/resourcegroups/test-rg/providers/applications.core/applications/app1",
		EnvironmentID: "/planes/radius/local/resourcegroups/test-rg/providers/applications.core/environments/env1",
		ResourceID:    "/planes/radius/local/resourceGroups/test-rg/providers/Microsoft.Resources/deployments/recipe",
		Parameters: map[string]any{
			"resourceName": "resource1",
		},
	}
	recipeDefinition := &recipes.EnvironmentDefinition{
		Driver:       recipes.TemplateKindTerraform,
		TemplatePath: "git://https://dev.azure.com/mongo-recipe/recipe",
		ResourceType: "Applications.Datastores/mongoDatabases",
	}
	envConfig := &recipes.Configuration{
		Runtime: recipes.RuntimeConfiguration{
			Kubernetes: &recipes.KubernetesRuntime{
				Namespace: "default",
			},
		},
		Providers: datamodel.Providers{
			Azure: datamodel.ProvidersAzure{
				Scope: "scope",
			},
		},
		RecipeConfig: datamodel.RecipeConfigProperties{
			Terraform: datamodel.TerraformConfigProperties{
				Authentication: datamodel.AuthConfig{
					Git: datamodel.GitAuthConfig{
						PAT: map[string]datamodel.SecretConfig{
							"dev.azure.com": {
								Secret: "/planes/radius/local/resourcegroups/default/providers/Applications.Core/secretStores/azdevopsgit",
							},
						},
					},
				},
			},
		},
	}
	ctx := testcontext.New(t)
	engine, configLoader, _, driverWithSecrets, secretsLoader := setup(t)
	outputParams := map[string]any{"parameters": recipeDefinition.Parameters}

	configLoader.EXPECT().
		LoadConfiguration(ctx, recipeMetadata).
		Times(1).
		Return(envConfig, nil)
	driverWithSecrets.EXPECT().
		FindSecretIDs(ctx, *envConfig, *recipeDefinition).
		Times(1).
		Return(map[string][]string{"/planes/radius/local/resourcegroups/default/providers/Applications.Core/secretStores/azdevopsgit": {"username", "pat"}}, nil)
	secretsLoader.EXPECT().
		LoadSecrets(ctx, map[string][]string{"/planes/radius/local/resourcegroups/default/providers/Applications.Core/secretStores/azdevopsgit": {"username", "pat"}}).
		Times(1).
		Return(nil, nil)
	driverWithSecrets.EXPECT().GetRecipeMetadata(ctx, recipedriver.BaseOptions{
		Recipe:        recipes.ResourceMetadata{},
		Definition:    *recipeDefinition,
		Configuration: *envConfig,
	}).Times(1).Return(outputParams, nil)

	recipeData, err := engine.GetRecipeMetadata(ctx, GetRecipeMetadataOptions{
		BaseOptions: BaseOptions{
			Recipe: recipeMetadata,
		},
		RecipeDefinition: *recipeDefinition,
	})
	require.NoError(t, err)
	require.Equal(t, outputParams, recipeData)
}

func Test_GetRecipeMetadata_Driver_Error(t *testing.T) {
	recipeMetadata, recipeDefinition, _ := getRecipeInputs()
	envConfig := &recipes.Configuration{
		Runtime: recipes.RuntimeConfiguration{
			Kubernetes: &recipes.KubernetesRuntime{
				Namespace: "default",
			},
		},
		Providers: datamodel.Providers{
			Azure: datamodel.ProvidersAzure{
				Scope: "scope",
			},
		},
	}
	ctx := testcontext.New(t)
	engine, configLoader, driver, _, _ := setup(t)

	configLoader.EXPECT().
		LoadConfiguration(ctx, recipeMetadata).
		Times(1).
		Return(envConfig, nil)
	driver.EXPECT().GetRecipeMetadata(ctx, recipedriver.BaseOptions{
		Recipe:        recipes.ResourceMetadata{},
		Definition:    recipeDefinition,
		Configuration: *envConfig,
	}).Times(1).Return(nil, errors.New("driver failure"))

	_, err := engine.GetRecipeMetadata(ctx, GetRecipeMetadataOptions{
		BaseOptions: BaseOptions{
			Recipe: recipeMetadata,
		},
		RecipeDefinition: recipeDefinition,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "driver failure")
}

func Test_GetRecipeMetadata_Driver_InvalidDriver(t *testing.T) {
	_, recipeDefinition, _ := getRecipeInputs()
	recipeDefinition.Driver = "invalid"
	recipeMetadata := recipes.ResourceMetadata{
		EnvironmentID: "/planes/radius/local/resourcegroups/test-rg/providers/applications.core/environments/env1",
	}
	ctx := testcontext.New(t)
	engine, configLoader, _, _, _ := setup(t)
	envConfig := &recipes.Configuration{
		Runtime: recipes.RuntimeConfiguration{
			Kubernetes: &recipes.KubernetesRuntime{
				Namespace: "default",
			},
		},
		Providers: datamodel.Providers{
			Azure: datamodel.ProvidersAzure{
				Scope: "scope",
			},
		},
	}
	configLoader.EXPECT().
		LoadConfiguration(ctx, recipeMetadata).
		Times(1).
		Return(envConfig, nil)
	_, err := engine.GetRecipeMetadata(ctx, GetRecipeMetadataOptions{
		BaseOptions: BaseOptions{
			Recipe: recipeMetadata,
		},
		RecipeDefinition: recipeDefinition,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "could not find driver invalid")
}

func getRecipeInputs() (recipes.ResourceMetadata, recipes.EnvironmentDefinition, []rpv1.OutputResource) {
	recipeMetadata := recipes.ResourceMetadata{
		Name:          "mongo-azure",
		ApplicationID: "/planes/radius/local/resourcegroups/test-rg/providers/applications.core/applications/app1",
		EnvironmentID: "/planes/radius/local/resourcegroups/test-rg/providers/applications.core/environments/env1",
		ResourceID:    "/planes/radius/local/resourceGroups/test-rg/providers/Applications.Datastores/mongoDatabases/test-db",
		Parameters: map[string]any{
			"resourceName": "resource1",
		},
	}

	recipeDefinition := recipes.EnvironmentDefinition{
		Driver:       recipes.TemplateKindBicep,
		TemplatePath: "ghcr.io/radius-project/dev/recipes/functionaltest/basic/mongodatabases/azure:1.0",
		ResourceType: "Applications.Datastores/mongoDatabases",
	}

	outputResources := []rpv1.OutputResource{
		{
			ID: resources.MustParse("/subscriptions/test-sub/resourcegroups/test-rg/providers/Microsoft.DocumentDB/accounts/test-account"),
		},
	}
	return recipeMetadata, recipeDefinition, outputResources
}

func Test_Engine_Execute_With_Secrets_Success(t *testing.T) {
	recipeMetadata := recipes.ResourceMetadata{
		Name:          "mongo-azure",
		ApplicationID: "/planes/radius/local/resourcegroups/test-rg/providers/applications.core/applications/app1",
		EnvironmentID: "/planes/radius/local/resourcegroups/test-rg/providers/applications.core/environments/env1",
		ResourceID:    "/planes/radius/local/resourceGroups/test-rg/providers/Microsoft.Resources/deployments/recipe",
		Parameters: map[string]any{
			"resourceName": "resource1",
		},
	}
	prevState := []string{
		"/subscriptions/test-sub/resourceGroups/test-rg/providers/System.Test/testResources/test1",
	}
	envConfig := &recipes.Configuration{
		Runtime: recipes.RuntimeConfiguration{
			Kubernetes: &recipes.KubernetesRuntime{
				Namespace: "default",
			},
		},
		Providers: datamodel.Providers{
			Azure: datamodel.ProvidersAzure{
				Scope: "scope",
			},
		},
		RecipeConfig: datamodel.RecipeConfigProperties{
			Terraform: datamodel.TerraformConfigProperties{
				Authentication: datamodel.AuthConfig{
					Git: datamodel.GitAuthConfig{
						PAT: map[string]datamodel.SecretConfig{
							"dev.azure.com": {
								Secret: "/planes/radius/local/resourcegroups/default/providers/Applications.Core/secretStores/azdevopsgit",
							},
						},
					},
				},
			},
		},
	}
	recipeResult := &recipes.RecipeOutput{
		Resources: []string{"mongoStorageAccount", "mongoDatabase"},
		Secrets: map[string]any{
			"connectionString": "mongodb://testUser:testPassword@testAccount1.mongo.cosmos.azure.com:10255",
		},
		Values: map[string]any{
			"host": "testAccount1.mongo.cosmos.azure.com",
			"port": 10255,
		},
	}
	recipeDefinition := &recipes.EnvironmentDefinition{
		Driver:       recipes.TemplateKindTerraform,
		TemplatePath: "git://https://dev.azure.com/mongo-recipe/recipe",
		ResourceType: "Applications.Datastores/mongoDatabases",
	}
	ctx := testcontext.New(t)
	engine, configLoader, _, driverWithSecrets, secretsLoader := setup(t)
	configLoader.EXPECT().
		LoadConfiguration(ctx, recipeMetadata).
		Times(1).
		Return(envConfig, nil)
	configLoader.EXPECT().
		LoadRecipe(ctx, &recipeMetadata).
		Times(1).
		Return(recipeDefinition, nil)
	driverWithSecrets.EXPECT().
		FindSecretIDs(ctx, *envConfig, *recipeDefinition).
		Times(1).
		Return(map[string][]string{"/planes/radius/local/resourcegroups/default/providers/Applications.Core/secretStores/azdevopsgit": {"username", "pat"}}, nil)
	secretsLoader.EXPECT().
		LoadSecrets(ctx, gomock.Any()).
		Times(1).
		Return(nil, nil)
	driverWithSecrets.EXPECT().
		Execute(ctx, recipedriver.ExecuteOptions{
			BaseOptions: recipedriver.BaseOptions{
				Configuration: *envConfig,
				Recipe:        recipeMetadata,
				Definition:    *recipeDefinition,
			},
			PrevState: prevState,
		}).
		Times(1).
		Return(recipeResult, nil)

	result, err := engine.Execute(ctx, ExecuteOptions{
		BaseOptions: BaseOptions{
			Recipe: recipeMetadata,
		},
		PreviousState: prevState,
	})
	require.NoError(t, err)
	require.Equal(t, result, recipeResult)
}

func Test_Engine_Delete_With_Secrets_Success(t *testing.T) {
	_, _, outputResources := getRecipeInputs()
	recipeMetadata := recipes.ResourceMetadata{
		Name:          "mongo-azure",
		ApplicationID: "/planes/radius/local/resourcegroups/test-rg/providers/applications.core/applications/app1",
		EnvironmentID: "/planes/radius/local/resourcegroups/test-rg/providers/applications.core/environments/env1",
		ResourceID:    "/planes/radius/local/resourceGroups/test-rg/providers/Microsoft.Resources/deployments/recipe",
		Parameters: map[string]any{
			"resourceName": "resource1",
		},
	}
	envConfig := &recipes.Configuration{
		Runtime: recipes.RuntimeConfiguration{
			Kubernetes: &recipes.KubernetesRuntime{
				Namespace: "default",
			},
		},
		Providers: datamodel.Providers{
			Azure: datamodel.ProvidersAzure{
				Scope: "scope",
			},
		},
		RecipeConfig: datamodel.RecipeConfigProperties{
			Terraform: datamodel.TerraformConfigProperties{
				Authentication: datamodel.AuthConfig{
					Git: datamodel.GitAuthConfig{
						PAT: map[string]datamodel.SecretConfig{
							"dev.azure.com": {
								Secret: "/planes/radius/local/resourcegroups/default/providers/Applications.Core/secretStores/azdevopsgit",
							},
						},
					},
				},
			},
		},
	}
	recipeDefinition := &recipes.EnvironmentDefinition{
		Driver:       recipes.TemplateKindTerraform,
		TemplatePath: "git://https://dev.azure.com/mongo-recipe/recipe",
		ResourceType: "Applications.Datastores/mongoDatabases",
	}
	ctx := testcontext.New(t)
	engine, configLoader, _, driverWithSecrets, secretsLoader := setup(t)

	configLoader.EXPECT().
		LoadRecipe(ctx, &recipeMetadata).
		Times(1).
		Return(recipeDefinition, nil)

	configLoader.EXPECT().
		LoadConfiguration(ctx, recipeMetadata).
		Times(1).
		Return(envConfig, nil)
	driverWithSecrets.EXPECT().
		FindSecretIDs(ctx, *envConfig, *recipeDefinition).
		Times(1).
		Return(map[string][]string{"/planes/radius/local/resourcegroups/default/providers/Applications.Core/secretStores/azdevopsgit": {"username", "pat"}}, nil)
	secretsLoader.EXPECT().
		LoadSecrets(ctx, map[string][]string{"/planes/radius/local/resourcegroups/default/providers/Applications.Core/secretStores/azdevopsgit": {"username", "pat"}}).
		Times(1).
		Return(nil, nil)
	driverWithSecrets.EXPECT().
		Delete(ctx, recipedriver.DeleteOptions{
			BaseOptions: recipedriver.BaseOptions{
				Configuration: *envConfig,
				Recipe:        recipeMetadata,
				Definition:    *recipeDefinition,
			},
			OutputResources: outputResources,
		}).
		Times(1).
		Return(nil)

	err := engine.Delete(ctx, DeleteOptions{
		BaseOptions: BaseOptions{
			Recipe: recipeMetadata,
		},
		OutputResources: outputResources,
	})
	require.NoError(t, err)
}
