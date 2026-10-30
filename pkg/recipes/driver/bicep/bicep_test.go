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

package bicep

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armdeployments/v3"
	"github.com/go-logr/logr"
	"github.com/go-logr/logr/funcr"
	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	corerp_datamodel "github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/portableresources/processors"
	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/recipes/driver"
	"github.com/radius-project/radius/pkg/recipes/kubernetes/clusteraccess"
	"github.com/radius-project/radius/pkg/recipes/recipecontext"
	recipes_util "github.com/radius-project/radius/pkg/recipes/util"
	"github.com/radius-project/radius/pkg/rp/util/registrytest"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	clients "github.com/radius-project/radius/pkg/sdk/clients"
	"github.com/radius-project/radius/pkg/ucp/resources"
	resources_kubernetes "github.com/radius-project/radius/pkg/ucp/resources/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"oras.land/oras-go/v2/registry/remote/auth"
)

type deploymentClientSpy struct {
	clients.ResourceDeploymentsClient
	createOrUpdateCalls int
	deployment          clients.Deployment
	response            *clients.ClientCreateOrUpdateResponse
}

type deploymentPollerStub struct {
	clients.Poller[clients.ClientCreateOrUpdateResponse]
	response clients.ClientCreateOrUpdateResponse
}

func (p *deploymentPollerStub) PollUntilDone(context.Context, *clients.PollUntilDoneOptions) (clients.ClientCreateOrUpdateResponse, error) {
	return p.response, nil
}

type clusterAccessResolverStub struct {
	config                *rest.Config
	err                   error
	receivedConfiguration *recipes.Configuration
	resolveCalls          int
}

func (s *clusterAccessResolverStub) Resolve(_ context.Context, configuration *recipes.Configuration) (*rest.Config, error) {
	s.resolveCalls++
	s.receivedConfiguration = configuration
	return s.config, s.err
}

func (s *clusterAccessResolverStub) ResolveKubeconfigSource(context.Context, *recipes.Configuration) (clusteraccess.KubeconfigSource, error) {
	return clusteraccess.KubeconfigSource{}, errors.New("unexpected ResolveKubeconfigSource call")
}

func (s *deploymentClientSpy) CreateOrUpdate(_ context.Context, deployment clients.Deployment, _, _ string) (clients.Poller[clients.ClientCreateOrUpdateResponse], error) {
	s.createOrUpdateCalls++
	s.deployment = deployment
	if s.response != nil {
		return &deploymentPollerStub{response: *s.response}, nil
	}
	return nil, errors.New("unexpected deployment")
}

func Test_CreateRecipeParameters_NoContextParameter(t *testing.T) {
	devParams := map[string]any{}
	operatorParams := map[string]any{}
	expectedParams := map[string]any{}

	actualParams := createRecipeParameters(devParams, operatorParams, false, nil)
	require.Equal(t, expectedParams, actualParams)
}

func Test_CreateRecipeParameters_ParameterConflict(t *testing.T) {
	devParams := map[string]any{
		"throughput": 400,
		"port":       2030,
		"name":       "test-parameters",
	}
	operatorParams := map[string]any{
		"port":     2040,
		"name":     "test-parameters-conflict",
		"location": "us-east1",
	}
	expectedParams := map[string]any{
		"throughput": map[string]any{
			"value": 400,
		},
		"port": map[string]any{
			"value": 2030,
		},
		"name": map[string]any{
			"value": "test-parameters",
		},
		"location": map[string]any{
			"value": "us-east1",
		},
	}

	actualParams := createRecipeParameters(devParams, operatorParams, false, nil)
	require.Equal(t, expectedParams, actualParams)
}

func Test_CreateRecipeParameters_WithContextParameter(t *testing.T) {
	devParams := map[string]any{
		"throughput": 400,
		"port":       2030,
		"name":       "test-parameters",
	}
	recipeContext := recipecontext.Context{
		Resource: recipecontext.Resource{
			ResourceInfo: recipecontext.ResourceInfo{
				ID:   "/subscriptions/testSub/resourceGroups/testGroup/providers/applications.datastores/mongodatabases/mongo0",
				Name: "mongo0",
			},
			Type: "Applications.Datastores/mongoDatabases",
		},
		Application: recipecontext.ResourceInfo{
			ID:   "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/applications/testApplication",
			Name: "testApplication",
		},
		Environment: recipecontext.ResourceInfo{
			ID:   "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/environments/env0",
			Name: "env0",
		},
		Runtime: recipes.RuntimeConfiguration{
			Kubernetes: &recipes.KubernetesRuntime{
				EnvironmentNamespace: "radius-test-env",
				Namespace:            "radius-test-app",
			},
		},
	}

	expectedParams := map[string]any{
		"throughput": map[string]any{
			"value": 400,
		},
		"port": map[string]any{
			"value": 2030,
		},
		"name": map[string]any{
			"value": "test-parameters",
		},
		"context": map[string]any{
			"value": recipeContext,
		},
	}
	actualParams := createRecipeParameters(devParams, nil, true, &recipeContext)
	require.Equal(t, expectedParams, actualParams)
}

func Test_CreateRecipeParameters_EmptyResourceParameters(t *testing.T) {
	operatorParams := map[string]any{
		"throughput": 400,
		"port":       2030,
		"name":       "test-parameters",
	}
	recipeContext := recipecontext.Context{
		Resource: recipecontext.Resource{
			ResourceInfo: recipecontext.ResourceInfo{
				ID:   "/subscriptions/testSub/resourceGroups/testGroup/providers/applications.datastores/mongodatabases/mongo0",
				Name: "mongo0",
			},
			Type: "Applications.Datastores/mongoDatabases",
		},
		Application: recipecontext.ResourceInfo{
			ID:   "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/applications/testApplication",
			Name: "testApplication",
		},
		Environment: recipecontext.ResourceInfo{
			ID:   "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/environments/env0",
			Name: "env0",
		},
		Runtime: recipes.RuntimeConfiguration{
			Kubernetes: &recipes.KubernetesRuntime{
				EnvironmentNamespace: "radius-test-env",
				Namespace:            "radius-test-app",
			},
		},
	}

	expectedParams := map[string]any{
		"throughput": map[string]any{
			"value": 400,
		},
		"port": map[string]any{
			"value": 2030,
		},
		"name": map[string]any{
			"value": "test-parameters",
		},
		"context": map[string]any{
			"value": recipeContext,
		},
	}
	actualParams := createRecipeParameters(nil, operatorParams, true, &recipeContext)
	require.Equal(t, expectedParams, actualParams)
}

func Test_CreateRecipeParameters_ResourceAndEnvParameters(t *testing.T) {
	operatorParams := map[string]any{
		"throughput": 400,
		"port":       2030,
		"name":       "test-parameters",
	}
	devParams := map[string]any{
		"throughput": 800,
		"port":       2060,
	}
	recipeContext := recipecontext.Context{
		Resource: recipecontext.Resource{
			ResourceInfo: recipecontext.ResourceInfo{
				ID:   "/subscriptions/testSub/resourceGroups/testGroup/providers/applications.datastores/mongodatabases/mongo0",
				Name: "mongo0",
			},
			Type: "Applications.Datastores/mongoDatabases",
		},
		Application: recipecontext.ResourceInfo{
			ID:   "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/applications/testApplication",
			Name: "testApplication",
		},
		Environment: recipecontext.ResourceInfo{
			ID:   "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/environments/env0",
			Name: "env0",
		},
		Runtime: recipes.RuntimeConfiguration{
			Kubernetes: &recipes.KubernetesRuntime{
				EnvironmentNamespace: "radius-test-env",
				Namespace:            "radius-test-app",
			},
		},
	}

	expectedParams := map[string]any{
		"throughput": map[string]any{
			"value": 800,
		},
		"port": map[string]any{
			"value": 2060,
		},
		"name": map[string]any{
			"value": "test-parameters",
		},
		"context": map[string]any{
			"value": recipeContext,
		},
	}
	actualParams := createRecipeParameters(devParams, operatorParams, true, &recipeContext)
	require.Equal(t, expectedParams, actualParams)
}

func Test_createDeploymentID(t *testing.T) {
	expected, err := resources.ParseResource("/planes/radius/local/resourceGroups/cool-group/providers/Microsoft.Resources/deployments/test-deployment")
	require.NoError(t, err)

	actual, err := createDeploymentID("/planes/radius/local/resourceGroups/cool-group/providers/Applications.Datastores/mongoDatabases/test-db", "test-deployment")
	require.NoError(t, err)
	require.Equal(t, expected, actual)
}

func Test_createProviderConfig_defaults(t *testing.T) {
	expected := clients.NewDefaultProviderConfig("test-rg")
	actual := newProviderConfig("test-rg", corerp_datamodel.Providers{})
	require.Equal(t, expected, actual)
}

func Test_createProviderConfig_hasProviders(t *testing.T) {
	aws := "/planes/aws/aws/accounts/000/regions/cool-region"
	azure := "/subscriptions/000/resourceGroups/cool-azure-group"
	providers := corerp_datamodel.Providers{
		Azure: corerp_datamodel.ProvidersAzure{Scope: azure},
		AWS:   corerp_datamodel.ProvidersAWS{Scope: aws},
	}

	expected := clients.NewDefaultProviderConfig("test-rg")
	expected.Az = &clients.Az{
		Type:  clients.ProviderTypeAzure,
		Value: clients.Value{Scope: azure},
	}
	expected.AWS = &clients.AWS{
		Type:  clients.ProviderTypeAWS,
		Value: clients.Value{Scope: aws},
	}
	actual := newProviderConfig("test-rg", providers)
	require.Equal(t, expected, actual)
}

func Test_Bicep_PrepareRecipeResponse_Success(t *testing.T) {
	d := &bicepDriver{}

	resources := []*armdeployments.ResourceReference{
		{
			ID: new("outputResourceId"),
		},
	}

	response := map[string]any{}
	value := map[string]any{}
	value["resources"] = []any{"testId1", "testId2"}
	value["secrets"] = map[string]any{
		"username":         "testUser",
		"password":         "testPassword",
		"connectionString": "test-connection-string",
	}
	value["values"] = map[string]any{
		"host": "myrediscache.redis.cache.windows.net",
		"port": float64(6379), // This will be a float64 not an int in real scenarios, it's read from JSON.
	}
	response["result"] = map[string]any{
		"value": value,
	}
	expectedResponse := &recipes.RecipeOutput{
		Resources: []string{"testId1", "testId2", "outputResourceId"},
		Secrets: map[string]any{
			"username":         "testUser",
			"password":         "testPassword",
			"connectionString": "test-connection-string",
		},
		Values: map[string]any{
			"host": "myrediscache.redis.cache.windows.net",
			"port": float64(6379),
		},
		Status: &rpv1.RecipeStatus{
			TemplateKind: recipes.TemplateKindBicep,
			TemplatePath: "radiusdev.azurecr.io/recipes/functionaltest/parameters/mongodatabases/azure:1.0",
		},
	}

	opts := driver.ExecuteOptions{
		Definition: recipes.EnvironmentDefinition{
			Name:         "mongo-azure",
			Driver:       recipes.TemplateKindBicep,
			TemplatePath: "radiusdev.azurecr.io/recipes/functionaltest/parameters/mongodatabases/azure:1.0",
			ResourceType: "Applications.Datastores/mongoDatabases",
		},
		PrevState: []string{},
	}
	actualResponse, err := d.prepareRecipeResponse(opts.BaseOptions.Definition, response, resources)
	require.NoError(t, err)
	require.Equal(t, expectedResponse, actualResponse)
}

func Test_Bicep_PrepareRecipeResponse_EmptySecret(t *testing.T) {
	d := &bicepDriver{}

	resources := []*armdeployments.ResourceReference{
		{
			ID: new("outputResourceId"),
		},
	}

	response := map[string]any{}
	value := map[string]any{}
	value["resources"] = []any{"testId1", "testId2"}
	value["values"] = map[string]any{
		"host": "myrediscache.redis.cache.windows.net",
		"port": float64(6379), // This will be a float64 not an int in real scenarios, it's read from JSON.
	}
	response["result"] = map[string]any{
		"value": value,
	}
	expectedResponse := &recipes.RecipeOutput{
		Resources: []string{"testId1", "testId2", "outputResourceId"},
		Secrets:   map[string]any{},
		Values: map[string]any{
			"host": "myrediscache.redis.cache.windows.net",
			"port": float64(6379),
		},
		Status: &rpv1.RecipeStatus{
			TemplateKind: recipes.TemplateKindBicep,
			TemplatePath: "radiusdev.azurecr.io/recipes/functionaltest/parameters/mongodatabases/azure:1.0",
		},
	}

	actualResponse, err := d.prepareRecipeResponse(recipes.EnvironmentDefinition{TemplatePath: "radiusdev.azurecr.io/recipes/functionaltest/parameters/mongodatabases/azure:1.0"}, response, resources)
	require.NoError(t, err)
	require.Equal(t, expectedResponse, actualResponse)
}

func Test_Bicep_PrepareRecipeResponse_EmptyResult(t *testing.T) {
	d := &bicepDriver{}

	resources := []*armdeployments.ResourceReference{
		{
			ID: new("outputResourceId"),
		},
	}
	response := map[string]any{}
	expectedResponse := &recipes.RecipeOutput{
		Resources: []string{"outputResourceId"},
		Status: &rpv1.RecipeStatus{
			TemplateKind: recipes.TemplateKindBicep,
			TemplatePath: "radiusdev.azurecr.io/recipes/functionaltest/parameters/mongodatabases/azure:1.0",
		},
	}

	actualResponse, err := d.prepareRecipeResponse(recipes.EnvironmentDefinition{TemplatePath: "radiusdev.azurecr.io/recipes/functionaltest/parameters/mongodatabases/azure:1.0"}, response, resources)
	require.NoError(t, err)
	require.Equal(t, expectedResponse, actualResponse)
}

func setupDeleteInputs(t *testing.T) (bicepDriver, *processors.MockResourceClient, *clusterAccessResolverStub, *[]*rest.Config) {
	ctrl := gomock.NewController(t)
	client := processors.NewMockResourceClient(ctrl)
	resolver := &clusterAccessResolverStub{
		config: &rest.Config{Host: "https://target.example.com"},
	}
	factoryConfigs := &[]*rest.Config{}

	driver := bicepDriver{
		clusterAccessResolver: resolver,
		resourceClientFactory: func(config *rest.Config) processors.ResourceClient {
			*factoryConfigs = append(*factoryConfigs, config)
			return client
		},
		options: BicepOptions{
			DeleteRetryCount:        0,
			DeleteRetryDelaySeconds: 1,
		},
	}

	return driver, client, resolver, factoryConfigs
}

func Test_Bicep_Delete_Success(t *testing.T) {
	ctx := t.Context()
	driverBicep, client, resolver, factoryConfigs := setupDeleteInputs(t)
	azureResourceID := "/subscriptions/test-sub/resourceGroups/test-rg/providers/Microsoft.Storage/storageAccounts/test-account"
	outputResources := []rpv1.OutputResource{
		{
			LocalID: "RecipeResource0",
			ID: resources_kubernetes.IDFromParts(
				resources_kubernetes.PlaneNameTODO,
				"apps",
				"Deployment",
				"recipe-app",
				"redis"),
			RadiusManaged: new(true),
		},
		{
			LocalID: "RecipeResource1",
			ID: resources_kubernetes.IDFromParts(
				resources_kubernetes.PlaneNameTODO,
				"",
				"Service",
				"recipe-app",
				"redis"),
			// We don't expect a call to delete to be made when RadiusManaged is false.
			RadiusManaged: new(false),
		},
		{
			LocalID:       "RecipeResource2",
			ID:            resources.MustParse(azureResourceID),
			RadiusManaged: new(true),
		},
	}
	client.EXPECT().Delete(gomock.Any(), "/planes/kubernetes/local/namespaces/recipe-app/providers/apps/Deployment/redis").Times(1).Return(nil)
	client.EXPECT().Delete(gomock.Any(), azureResourceID).Times(1).Return(nil)
	configuration := recipes.Configuration{
		Runtime: recipes.RuntimeConfiguration{
			Kubernetes: &recipes.KubernetesRuntime{Namespace: "recipe-app"},
		},
	}

	err := driverBicep.Delete(ctx, driver.DeleteOptions{
		Configuration:   configuration,
		OutputResources: outputResources,
	})
	require.NoError(t, err)
	require.Equal(t, 1, resolver.resolveCalls)
	require.Equal(t, &configuration, resolver.receivedConfiguration)
	require.Equal(t, []*rest.Config{resolver.config}, *factoryConfigs)
}

func Test_Bicep_Delete_UnmanagedOutputsDoNotCreateResourceClient(t *testing.T) {
	outputResources := []rpv1.OutputResource{
		{
			ID: resources_kubernetes.IDFromParts(
				resources_kubernetes.PlaneNameTODO,
				"apps",
				"Deployment",
				"recipe-app",
				"redis"),
			RadiusManaged: new(false),
		},
	}

	err := (&bicepDriver{}).Delete(t.Context(), driver.DeleteOptions{
		OutputResources: outputResources,
	})
	require.NoError(t, err)
}

func Test_Bicep_Delete_Error(t *testing.T) {
	ctx := t.Context()
	driverBicep, client, _, _ := setupDeleteInputs(t)
	outputResources := []rpv1.OutputResource{
		{
			ID: resources_kubernetes.IDFromParts(
				resources_kubernetes.PlaneNameTODO,
				"core",
				"Deployment",
				"recipe-app",
				"redis"),
			RadiusManaged: new(true),
		},
	}
	recipeError := recipes.RecipeError{
		ErrorDetails: v1.ErrorDetails{
			Code:    recipes.RecipeDeletionFailed,
			Message: fmt.Sprintf("failed to delete resource %q after 1 attempt(s), last error: could not find API version for type %q, no supported API versions", outputResources[0].ID.String(), outputResources[0].GetResourceType().Type),
		},
	}
	client.EXPECT().
		Delete(gomock.Any(), "/planes/kubernetes/local/namespaces/recipe-app/providers/core/Deployment/redis").
		Return(fmt.Errorf("could not find API version for type %q, no supported API versions", outputResources[0].GetResourceType().Type)).
		Times(1)

	err := driverBicep.Delete(ctx, driver.DeleteOptions{
		OutputResources: outputResources,
	})
	require.Error(t, err)
	require.Equal(t, err, &recipeError)
}

func Test_Bicep_Delete_MultipleOutputResources(t *testing.T) {
	ids := []string{
		"/subscriptions/test-sub/resourceGroups/test-rg/providers/Microsoft.Storage/storageAccounts/a",
		"/subscriptions/test-sub/resourceGroups/test-rg/providers/Microsoft.Storage/storageAccounts/b",
		"/subscriptions/test-sub/resourceGroups/test-rg/providers/Microsoft.Storage/storageAccounts/c",
	}

	tests := []struct {
		name            string
		deleteErrors    map[string]error
		expectedMessage string
	}{
		{
			name: "every delete succeeds",
		},
		{
			name:         "one delete fails",
			deleteErrors: map[string]error{ids[1]: errors.New("b is locked")},
			expectedMessage: fmt.Sprintf("failed to delete resource %q after 1 attempt(s), last error: b is locked",
				ids[1]),
		},
		{
			name: "several deletes fail",
			deleteErrors: map[string]error{
				ids[2]: errors.New("c is locked"),
				ids[0]: errors.New("a is locked"),
			},
			expectedMessage: fmt.Sprintf("failed to delete resource %q after 1 attempt(s), last error: a is locked\n"+
				"failed to delete resource %q after 1 attempt(s), last error: c is locked",
				ids[0], ids[2]),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			driverBicep, client, _, _ := setupDeleteInputs(t)
			outputResources := []rpv1.OutputResource{}
			for _, id := range ids {
				outputResources = append(outputResources, rpv1.OutputResource{ID: resources.MustParse(id), RadiusManaged: new(true)})
				client.EXPECT().Delete(gomock.Any(), id).Return(tc.deleteErrors[id]).Times(1)
			}

			err := driverBicep.Delete(t.Context(), driver.DeleteOptions{OutputResources: outputResources})
			if tc.expectedMessage == "" {
				require.NoError(t, err)
				return
			}
			require.Equal(t, &recipes.RecipeError{
				ErrorDetails: v1.ErrorDetails{
					Code:    recipes.RecipeDeletionFailed,
					Message: tc.expectedMessage,
				},
			}, err)
		})
	}
}

// Test_Bicep_Delete_RetriesEachResource checks that each output resource gets its own retries.
func Test_Bicep_Delete_RetriesEachResource(t *testing.T) {
	driverBicep, client, _, _ := setupDeleteInputs(t)
	driverBicep.options.DeleteRetryCount = 2
	driverBicep.options.DeleteRetryDelaySeconds = 0

	alwaysFails := "/subscriptions/test-sub/resourceGroups/test-rg/providers/Microsoft.Storage/storageAccounts/always-fails"
	failsOnce := "/subscriptions/test-sub/resourceGroups/test-rg/providers/Microsoft.Storage/storageAccounts/fails-once"
	succeeds := "/subscriptions/test-sub/resourceGroups/test-rg/providers/Microsoft.Storage/storageAccounts/succeeds"
	outputResources := []rpv1.OutputResource{
		{ID: resources.MustParse(alwaysFails), RadiusManaged: new(true)},
		{ID: resources.MustParse(failsOnce), RadiusManaged: new(true)},
		{ID: resources.MustParse(succeeds), RadiusManaged: new(true)},
	}

	client.EXPECT().Delete(gomock.Any(), alwaysFails).Return(errors.New("always-fails is locked")).Times(3)
	gomock.InOrder(
		client.EXPECT().Delete(gomock.Any(), failsOnce).Return(errors.New("fails-once is busy")),
		client.EXPECT().Delete(gomock.Any(), failsOnce).Return(nil),
	)
	client.EXPECT().Delete(gomock.Any(), succeeds).Return(nil).Times(1)

	err := driverBicep.Delete(t.Context(), driver.DeleteOptions{OutputResources: outputResources})
	require.Equal(t, &recipes.RecipeError{
		ErrorDetails: v1.ErrorDetails{
			Code:    recipes.RecipeDeletionFailed,
			Message: fmt.Sprintf("failed to delete resource %q after 3 attempt(s), last error: always-fails is locked", alwaysFails),
		},
	}, err)
}

// Test_Bicep_Execute_GarbageCollectionDeletesEveryStaleResource checks that when a redeploy drops output resources,
// one failed delete does not stop the other stale resources from being deleted.
func Test_Bicep_Execute_GarbageCollectionDeletesEveryStaleResource(t *testing.T) {
	ts := registrytest.NewFakeRegistryServer(t)
	t.Cleanup(ts.CloseServer)

	driverBicep, client, _, _ := setupDeleteInputs(t)
	driverBicep.DeploymentClient = &deploymentClientSpy{
		response: &clients.ClientCreateOrUpdateResponse{
			Properties: &armdeployments.DeploymentPropertiesExtended{},
		},
	}
	driverBicep.RegistryClient = ts.TestServer.Client()

	failing := "/subscriptions/test-sub/resourceGroups/test-rg/providers/Microsoft.Storage/storageAccounts/failing"
	stale := "/subscriptions/test-sub/resourceGroups/test-rg/providers/Microsoft.Storage/storageAccounts/stale"
	client.EXPECT().Delete(gomock.Any(), failing).Return(errors.New("failing is locked")).Times(1)
	client.EXPECT().Delete(gomock.Any(), stale).Return(nil).Times(1)

	_, err := driverBicep.Execute(t.Context(), driver.ExecuteOptions{
		BaseOptions: driver.BaseOptions{
			Recipe: recipes.ResourceMetadata{
				Name:          "test-resource",
				ResourceID:    "/planes/radius/local/resourceGroups/test-rg/providers/Test.Resources/resources/test-resource",
				EnvironmentID: "/planes/radius/local/resourceGroups/test-rg/providers/Radius.Core/environments/env",
			},
			Definition: recipes.EnvironmentDefinition{
				Name:         "direct-module",
				Driver:       recipes.TemplateKindBicep,
				TemplatePath: ts.TestImageURL,
			},
		},
		PrevState: []string{failing, stale},
	})
	recipeErr := &recipes.RecipeError{}
	require.ErrorAs(t, err, &recipeErr)
	require.Equal(t, recipes.RecipeGarbageCollectionFailed, recipeErr.ErrorDetails.Code)
	require.Equal(t, fmt.Sprintf("code RecipeDeletionFailed: err failed to delete resource %q after 1 attempt(s), last error: failing is locked", failing), recipeErr.ErrorDetails.Message)
}

// Test_Bicep_Delete_FailureDoesNotCancelOtherDeletes checks that a delete still running when another delete runs out
// of retries keeps a live context and finishes.
func Test_Bicep_Delete_FailureDoesNotCancelOtherDeletes(t *testing.T) {
	driverBicep, client, _, _ := setupDeleteInputs(t)
	driverBicep.options.DeleteRetryCount = 1
	driverBicep.options.DeleteRetryDelaySeconds = 0

	failing := "/subscriptions/test-sub/resourceGroups/test-rg/providers/Microsoft.Storage/storageAccounts/failing"
	slow := "/subscriptions/test-sub/resourceGroups/test-rg/providers/Microsoft.Storage/storageAccounts/slow"
	outputResources := []rpv1.OutputResource{
		{ID: resources.MustParse(failing), RadiusManaged: new(true)},
		{ID: resources.MustParse(slow), RadiusManaged: new(true)},
	}

	failingAttempts := atomic.Int32{}
	failingDone := make(chan struct{})
	client.EXPECT().Delete(gomock.Any(), failing).Times(2).DoAndReturn(func(context.Context, string) error {
		if failingAttempts.Add(1) == 2 {
			close(failingDone)
		}
		return errors.New("failing is locked")
	})

	slowDeleted := atomic.Bool{}
	client.EXPECT().Delete(gomock.Any(), slow).Times(1).DoAndReturn(func(ctx context.Context, _ string) error {
		<-failingDone
		// Give a canceled context time to show up before finishing the delete.
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
		slowDeleted.Store(true)
		return nil
	})

	err := driverBicep.Delete(t.Context(), driver.DeleteOptions{OutputResources: outputResources})
	require.Equal(t, &recipes.RecipeError{
		ErrorDetails: v1.ErrorDetails{
			Code:    recipes.RecipeDeletionFailed,
			Message: fmt.Sprintf("failed to delete resource %q after 2 attempt(s), last error: failing is locked", failing),
		},
	}, err)
	require.True(t, slowDeleted.Load())
}

func Test_Bicep_Delete_TargetClusterResolutionError(t *testing.T) {
	ctx := t.Context()
	driverBicep, _, resolver, factoryConfigs := setupDeleteInputs(t)
	resolver.err = errors.New("target kubeconfig is unavailable")
	outputResources := []rpv1.OutputResource{
		{
			ID: resources_kubernetes.IDFromParts(
				resources_kubernetes.PlaneNameTODO,
				"apps",
				"Deployment",
				"recipe-app",
				"redis"),
			RadiusManaged: new(true),
		},
	}

	err := driverBicep.Delete(ctx, driver.DeleteOptions{
		OutputResources: outputResources,
	})
	require.Error(t, err)
	require.ErrorContains(t, err, "failed to resolve target cluster for recipe output deletion")
	require.ErrorContains(t, err, "target kubeconfig is unavailable")
	require.Empty(t, *factoryConfigs)
}

func Test_Bicep_Delete_NilTargetClusterConfig(t *testing.T) {
	ctx := t.Context()
	driverBicep, _, resolver, factoryConfigs := setupDeleteInputs(t)
	resolver.config = nil
	outputResources := []rpv1.OutputResource{
		{
			ID: resources_kubernetes.IDFromParts(
				resources_kubernetes.PlaneNameTODO,
				"apps",
				"Deployment",
				"recipe-app",
				"redis"),
			RadiusManaged: new(true),
		},
	}

	err := driverBicep.Delete(ctx, driver.DeleteOptions{
		OutputResources: outputResources,
	})
	require.Error(t, err)
	require.ErrorContains(t, err, "cluster access resolver returned a nil Kubernetes configuration")
	require.Empty(t, *factoryConfigs)
}

func Test_Bicep_Delete_NonKubernetesOutputDoesNotResolveTargetCluster(t *testing.T) {
	ctx := t.Context()
	driverBicep, client, resolver, factoryConfigs := setupDeleteInputs(t)
	resolver.err = errors.New("target kubeconfig is unavailable")
	resourceID := "/subscriptions/test-sub/resourceGroups/test-rg/providers/Microsoft.Storage/storageAccounts/test-account"
	outputResources := []rpv1.OutputResource{
		{
			ID:            resources.MustParse(resourceID),
			RadiusManaged: new(true),
		},
	}
	client.EXPECT().Delete(gomock.Any(), resourceID).Times(1).Return(nil)

	err := driverBicep.Delete(ctx, driver.DeleteOptions{
		OutputResources: outputResources,
	})
	require.NoError(t, err)
	require.Zero(t, resolver.resolveCalls)
	require.Equal(t, []*rest.Config{nil}, *factoryConfigs)
}

func Test_Bicep_GetRecipeMetadata_Success(t *testing.T) {
	ts := registrytest.NewFakeRegistryServer(t)
	t.Cleanup(ts.CloseServer)

	ctx := t.Context()
	driverBicep := &bicepDriver{RegistryClient: ts.TestServer.Client()}
	recipeDefinition := recipes.EnvironmentDefinition{
		Name:         "mongo-azure",
		Driver:       recipes.TemplateKindBicep,
		TemplatePath: ts.TestImageURL,
		ResourceType: "Applications.Datastores/mongoDatabases",
	}

	expectedOutput := map[string]any{
		"documentdbName": map[string]any{"type": "string"},
		"location":       map[string]any{"defaultValue": "[resourceGroup().location]", "type": "string"},
	}

	recipeData, err := driverBicep.GetRecipeMetadata(ctx, driver.BaseOptions{
		Recipe:     recipes.ResourceMetadata{},
		Definition: recipeDefinition,
	})

	require.NoError(t, err)
	require.Equal(t, expectedOutput, recipeData["parameters"])
}

func Test_Bicep_GetRecipeMetadata_Error(t *testing.T) {
	ts := registrytest.NewFakeRegistryServer(t)
	t.Cleanup(ts.CloseServer)

	ctx := t.Context()
	driverBicep := &bicepDriver{RegistryClient: ts.TestServer.Client()}
	recipeDefinition := recipes.EnvironmentDefinition{
		Name:         "mongo-azure",
		Driver:       recipes.TemplateKindBicep,
		TemplatePath: ts.TestServer.URL + "/nonexisting:latest",
		ResourceType: "Applications.Datastores/mongoDatabases",
	}

	_, actualErr := driverBicep.GetRecipeMetadata(ctx, driver.BaseOptions{
		Recipe:     recipes.ResourceMetadata{},
		Definition: recipeDefinition,
	})
	expErr := recipes.RecipeError{
		ErrorDetails: v1.ErrorDetails{
			Code:    recipes.RecipeLanguageFailure,
			Message: "failed to fetch repository from the path \"https://<REPLACE_HOST>/nonexisting:latest\": <REPLACE_HOST>/nonexisting:latest: not found",
		},
		DeploymentStatus: "setupError",
	}
	expErr.ErrorDetails.Message = strings.Replace(expErr.ErrorDetails.Message, "<REPLACE_HOST>", ts.URL.Host, -1)
	require.Equal(t, actualErr, &expErr)
}

func Test_Bicep_Execute_InvalidOutputMappingDoesNotDeploy(t *testing.T) {
	ts := registrytest.NewFakeRegistryServer(t)
	t.Cleanup(ts.CloseServer)

	deploymentClient := &deploymentClientSpy{}
	driverBicep := &bicepDriver{
		DeploymentClient: deploymentClient,
		RegistryClient:   ts.TestServer.Client(),
	}

	output, err := driverBicep.Execute(t.Context(), driver.ExecuteOptions{
		Recipe: recipes.ResourceMetadata{
			Name:          "mongo",
			ResourceID:    "/planes/radius/local/resourceGroups/test-rg/providers/Applications.Datastores/mongoDatabases/mongo",
			EnvironmentID: "/planes/radius/local/resourceGroups/test-rg/providers/Applications.Core/environments/env",
		},
		Definition: recipes.EnvironmentDefinition{
			Name:          "mongo-azure",
			Driver:        recipes.TemplateKindBicep,
			TemplatePath:  ts.TestImageURL,
			ResourceType:  "Applications.Datastores/mongoDatabases",
			SecretOutputs: map[string]string{"connectionString": "primaryConnectionString"},
		},
	})

	require.Nil(t, output)
	require.Equal(t, &recipes.RecipeError{
		ErrorDetails: v1.ErrorDetails{
			Code:    recipes.InvalidRecipeOutputs,
			Message: `recipe "mongo-azure" for resource type "Applications.Datastores/mongoDatabases": invalid outputs mapping: no declared module output matches secrets["connectionString"] -> "primaryConnectionString"; available module outputs: none`,
		},
		DeploymentStatus: recipes_util.RecipeSetupError,
	}, err)
	require.Zero(t, deploymentClient.createOrUpdateCalls)
}

func Test_Bicep_Execute_DirectModuleParameters(t *testing.T) {
	t.Parallel()

	for _, kind := range []string{"None", "CanNotDelete"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()

			ts := registrytest.NewFakeRegistryServer(t)
			t.Cleanup(ts.CloseServer)

			deploymentClient := &deploymentClientSpy{}
			driverBicep := &bicepDriver{
				DeploymentClient: deploymentClient,
				RegistryClient:   ts.TestServer.Client(),
			}

			_, err := driverBicep.Execute(t.Context(), driver.ExecuteOptions{
				Recipe: recipes.ResourceMetadata{
					Name:          "direct-module",
					ResourceID:    "/planes/radius/local/resourceGroups/test-rg/providers/Test.Resources/resources/direct-module",
					EnvironmentID: "/planes/radius/local/resourceGroups/test-rg/providers/Radius.Core/environments/env",
				},
				Definition: recipes.EnvironmentDefinition{
					Name:         "direct-module",
					Driver:       recipes.TemplateKindBicep,
					TemplatePath: ts.TestImageURL,
					ResourceType: "Test.Resources/resources",
					Parameters: map[string]any{
						"name":  "{{context.resource.name}}",
						"lock":  map[string]any{"kind": kind},
						"kinds": []any{"None", "CanNotDelete"},
					},
				},
			})

			// Stop at the deployment boundary and inspect the serialized ARM payload.
			require.Error(t, err)
			require.Equal(t, 1, deploymentClient.createOrUpdateCalls)
			payload, err := json.Marshal(deploymentClient.deployment.Properties.Parameters)
			require.NoError(t, err)
			require.JSONEq(t, fmt.Sprintf(`{
				"name": {"value": "direct-module"},
				"lock": {"value": {"kind": %q}},
				"kinds": {"value": ["None", "CanNotDelete"]}
			}`, kind), string(payload))
		})
	}
}

func Test_Bicep_Execute_LogsDeploymentOutputResourceIDs(t *testing.T) {
	t.Parallel()

	const lockID = "/subscriptions/test-sub/resourceGroups/test-rg/providers/Microsoft.Authorization/locks/test-lock"
	for _, ids := range [][]string{{}, {lockID}} {
		t.Run(fmt.Sprintf("%d outputs", len(ids)), func(t *testing.T) {
			t.Parallel()

			ts := registrytest.NewFakeRegistryServer(t)
			t.Cleanup(ts.CloseServer)
			outputs := make([]*armdeployments.ResourceReference, 0, len(ids))
			for _, id := range ids {
				outputs = append(outputs, &armdeployments.ResourceReference{ID: new(id)})
			}
			deploymentClient := &deploymentClientSpy{
				response: &clients.ClientCreateOrUpdateResponse{
					Properties: &armdeployments.DeploymentPropertiesExtended{
						OutputResources: outputs,
						Outputs: map[string]any{
							"password": map[string]any{"type": "securestring", "value": "sensitive-output-value"},
						},
					},
				},
			}
			d := &bicepDriver{DeploymentClient: deploymentClient, RegistryClient: ts.TestServer.Client()}
			var logs []string
			ctx := logr.NewContext(t.Context(), funcr.NewJSON(func(line string) {
				logs = append(logs, line)
			}, funcr.Options{}))

			result, err := d.Execute(ctx, driver.ExecuteOptions{
				Recipe: recipes.ResourceMetadata{
					Name:          "test-resource",
					ResourceID:    "/planes/radius/local/resourceGroups/test-rg/providers/Test.Resources/resources/test-resource",
					EnvironmentID: "/planes/radius/local/resourceGroups/test-rg/providers/Radius.Core/environments/env",
				},
				Definition: recipes.EnvironmentDefinition{
					Name:         "direct-module",
					Driver:       recipes.TemplateKindBicep,
					TemplatePath: ts.TestImageURL,
					Parameters:   map[string]any{"password": "sensitive-parameter-value"},
				},
			})
			require.NoError(t, err)
			require.ElementsMatch(t, ids, result.Resources)
			require.Equal(t, "sensitive-output-value", result.Secrets["password"])

			var diagnostic map[string]any
			for _, line := range logs {
				require.NotContains(t, line, "sensitive-output-value")
				require.NotContains(t, line, "sensitive-parameter-value")
				if strings.Contains(line, "Deployment engine returned recipe output resource IDs") {
					require.NoError(t, json.Unmarshal([]byte(line), &diagnostic))
				}
			}
			require.NotNil(t, diagnostic)
			require.Equal(t, "direct-module", diagnostic["recipeName"])
			require.Contains(t, diagnostic["deploymentID"], "/providers/Microsoft.Resources/deployments/recipe")
			require.Equal(t, float64(len(ids)), diagnostic["outputResourceCount"])
			expectedIDs := make([]any, 0, len(ids))
			for _, id := range ids {
				expectedIDs = append(expectedIDs, id)
			}
			require.Equal(t, expectedIDs, diagnostic["outputResourceIDs"])
		})
	}
}

func Test_Bicep_Execute_UnresolvedExpressionDoesNotDeploy(t *testing.T) {
	ts := registrytest.NewFakeRegistryServer(t)
	t.Cleanup(ts.CloseServer)

	deploymentClient := &deploymentClientSpy{}
	driverBicep := &bicepDriver{
		DeploymentClient: deploymentClient,
		RegistryClient:   ts.TestServer.Client(),
	}

	output, err := driverBicep.Execute(t.Context(), driver.ExecuteOptions{
		Recipe: recipes.ResourceMetadata{
			Name:          "mongo",
			ResourceID:    "/planes/radius/local/resourceGroups/test-rg/providers/Applications.Datastores/mongoDatabases/mongo",
			EnvironmentID: "/planes/radius/local/resourceGroups/test-rg/providers/Applications.Core/environments/env",
		},
		Definition: recipes.EnvironmentDefinition{
			Name:         "mongo-azure",
			Driver:       recipes.TemplateKindBicep,
			TemplatePath: ts.TestImageURL,
			ResourceType: "Applications.Datastores/mongoDatabases",
			Parameters: map[string]any{
				"documentdbName": "{{context.resource.unknown}}",
			},
		},
	})

	require.Nil(t, output)
	require.Equal(t, &recipes.RecipeError{
		ErrorDetails: v1.ErrorDetails{
			Code:    recipes.RecipeDeploymentFailed,
			Message: `failed to resolve recipe parameter expressions: parameter "documentdbName": unresolved expression {{context.resource.unknown}}: unknown context path "context.resource.unknown"`,
		},
		DeploymentStatus: recipes_util.ExecutionError,
	}, err)
	require.Zero(t, deploymentClient.createOrUpdateCalls)
}

func Test_ValidateOutputMappings(t *testing.T) {
	tests := []struct {
		name          string
		definition    recipes.EnvironmentDefinition
		recipeData    map[string]any
		expectedError string
	}{
		{
			name: "accepts declared outputs",
			definition: recipes.EnvironmentDefinition{
				Outputs: map[string]string{"endpoint": "endpoint", "password": "password"},
			},
			recipeData: map[string]any{
				recipeOutputs: map[string]any{
					"endpoint": map[string]any{"type": "string"},
					"password": map[string]any{"type": "securestring"},
				},
			},
		},
		{
			name: "rejects undeclared output",
			definition: recipes.EnvironmentDefinition{
				Name:          "test-recipe",
				ResourceType:  "Test.Resources/widgets",
				SecretOutputs: map[string]string{"connectionString": "missing"},
			},
			recipeData: map[string]any{
				recipeOutputs: map[string]any{
					"endpoint": map[string]any{"type": "string"},
				},
			},
			expectedError: `recipe "test-recipe" for resource type "Test.Resources/widgets": invalid outputs mapping: no declared module output matches secrets["connectionString"] -> "missing"; available module outputs: "endpoint"`,
		},
		{
			name: "rejects non-object outputs collection",
			definition: recipes.EnvironmentDefinition{
				Name:         "test-recipe",
				ResourceType: "Test.Resources/widgets",
				Outputs:      map[string]string{"host": "endpoint"},
			},
			recipeData:    map[string]any{recipeOutputs: "invalid"},
			expectedError: `recipe "test-recipe" for resource type "Test.Resources/widgets": recipe outputs must be an object`,
		},
		{
			name: "rejects mapping when template declares no outputs",
			definition: recipes.EnvironmentDefinition{
				Name:         "test-recipe",
				ResourceType: "Test.Resources/widgets",
				Outputs:      map[string]string{"host": "endpoint"},
			},
			recipeData:    map[string]any{},
			expectedError: `recipe "test-recipe" for resource type "Test.Resources/widgets": invalid outputs mapping: no declared module output matches outputs["host"] -> "endpoint"; available module outputs: none`,
		},
		{
			name:       "skips template output parsing when no mappings are configured",
			recipeData: map[string]any{recipeOutputs: "invalid"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateOutputMappings(tt.definition, tt.recipeData)
			if tt.expectedError != "" {
				require.EqualError(t, err, tt.expectedError)
				return
			}

			require.NoError(t, err)
		})
	}
}

func Test_GetGCOutputResources(t *testing.T) {
	d := &bicepDriver{}
	before := []string{
		"/subscriptions/test-sub/resourceGroups/test-rg/providers/System.Test/testResources/resource1",
		"/subscriptions/test-sub/resourceGroups/test-rg/providers/System.Test/testResources/resource2",
	}
	after := []string{
		"/subscriptions/test-sub/resourceGroups/test-rg/providers/System.Test/testResources/resource1",
		"/subscriptions/test-sub/resourceGroups/test-rg/providers/System.Test/testResources/resource3",
	}

	expId := "/subscriptions/test-sub/resourceGroups/test-rg/providers/System.Test/testResources/resource2"
	id, err := resources.Parse(expId)
	require.NoError(t, err)
	exp := []rpv1.OutputResource{
		{
			ID:            id,
			RadiusManaged: new(true),
		},
	}
	res, err := d.getGCOutputResources(after, before)
	require.NoError(t, err)
	require.Equal(t, exp, res)
}

func Test_GetGCOutputResources_AWSReadOnly(t *testing.T) {
	d := &bicepDriver{}
	image := "/planes/aws/aws/accounts/000000000000/regions/us-west-2/providers/AWS.EC2/Image/ami-0123456789abcdef0"
	bucket := "/planes/aws/aws/accounts/000000000000/regions/us-west-2/providers/AWS.S3/Bucket/my-bucket"
	exp := []rpv1.OutputResource{
		{ID: resources.MustParse(image), RadiusManaged: new(false)},
		{ID: resources.MustParse(bucket), RadiusManaged: new(true)},
	}
	res, err := d.getGCOutputResources(nil, []string{image, bucket})
	require.NoError(t, err)
	require.Equal(t, exp, res)
}

func Test_Bicep_Execute_GarbageCollectionSkipsAWSReadOnly(t *testing.T) {
	ts := registrytest.NewFakeRegistryServer(t)
	t.Cleanup(ts.CloseServer)

	image := "/planes/aws/aws/accounts/000000000000/regions/us-west-2/providers/AWS.EC2/Image/ami-0123456789abcdef0"
	bucket := "/planes/aws/aws/accounts/000000000000/regions/us-west-2/providers/AWS.S3/Bucket/my-bucket"
	d, client, _, _ := setupDeleteInputs(t)
	d.DeploymentClient = &deploymentClientSpy{
		response: &clients.ClientCreateOrUpdateResponse{Properties: &armdeployments.DeploymentPropertiesExtended{}},
	}
	d.RegistryClient = ts.TestServer.Client()
	// Only the bucket is deleted. The mock fails the test on a Delete call for the image.
	client.EXPECT().Delete(gomock.Any(), bucket).Times(1).Return(nil)

	result, err := d.Execute(t.Context(), driver.ExecuteOptions{
		BaseOptions: driver.BaseOptions{
			Recipe: recipes.ResourceMetadata{
				Name:          "test-resource",
				ResourceID:    "/planes/radius/local/resourceGroups/test-rg/providers/Test.Resources/resources/test-resource",
				EnvironmentID: "/planes/radius/local/resourceGroups/test-rg/providers/Radius.Core/environments/env",
			},
			Definition: recipes.EnvironmentDefinition{
				Name:         "aws-recipe",
				Driver:       recipes.TemplateKindBicep,
				TemplatePath: ts.TestImageURL,
			},
		},
		PrevState: []string{image, bucket},
	})
	require.NoError(t, err)
	require.Empty(t, result.Resources)
}

func Test_GetGCOutputResources_NoDiff(t *testing.T) {
	d := &bicepDriver{}
	before := []string{
		"/subscriptions/test-sub/resourceGroups/test-rg/providers/System.Test/testResources/resource1",
		"/subscriptions/test-sub/resourceGroups/test-rg/providers/System.Test/testResources/resource2",
	}
	after := []string{
		"/subscriptions/test-sub/resourceGroups/test-rg/providers/System.Test/testResources/resource1",
		"/subscriptions/test-sub/resourceGroups/test-rg/providers/System.Test/testResources/resource2",
	}
	exp := []rpv1.OutputResource{}
	res, err := d.getGCOutputResources(after, before)
	require.NoError(t, err)
	require.Equal(t, exp, res)
}

func Test_Bicep_Delete_Success_AfterRetry(t *testing.T) {
	ctx := t.Context()
	driverBicep, client, _, _ := setupDeleteInputs(t)
	driverBicep.options.DeleteRetryCount = 1

	outputResources := []rpv1.OutputResource{
		{
			ID: resources_kubernetes.IDFromParts(
				resources_kubernetes.PlaneNameTODO,
				"core",
				"Deployment",
				"recipe-app",
				"redis"),
			RadiusManaged: new(true),
		},
	}

	gomock.InOrder(
		client.EXPECT().
			Delete(gomock.Any(), "/planes/kubernetes/local/namespaces/recipe-app/providers/core/Deployment/redis").
			Return(fmt.Errorf("failed to delete resource after 1 attempt(s), last error: could not find API version for type %q, no supported API versions", outputResources[0].GetResourceType().Type)),
		client.EXPECT().
			Delete(gomock.Any(), "/planes/kubernetes/local/namespaces/recipe-app/providers/core/Deployment/redis").
			Return(nil),
	)

	err := driverBicep.Delete(ctx, driver.DeleteOptions{
		OutputResources: outputResources,
	})
	require.NoError(t, err)
}

func Test_Bicep_PrepareRecipeResponse_DirectModule(t *testing.T) {
	d := &bicepDriver{}

	tests := []struct {
		desc             string
		definition       recipes.EnvironmentDefinition
		outputs          any
		resources        []*armdeployments.ResourceReference
		expectedResponse *recipes.RecipeOutput
		expectedErr      error
	}{
		{
			desc: "direct module with outputs mapping",
			definition: recipes.EnvironmentDefinition{
				TemplatePath: "br:mcr.microsoft.com/bicep/avm/res/storage/storage-account:0.14.3",
				Outputs:      map[string]string{"endpoint": "storageEndpoint", "name": "storageName"},
			},
			outputs: map[string]any{
				"storageEndpoint": map[string]any{"type": "string", "value": "https://sa.blob.core.windows.net"},
				"storageName":     map[string]any{"type": "string", "value": "mystorageaccount"},
				"extra":           map[string]any{"type": "string", "value": "ignored"},
			},
			resources: []*armdeployments.ResourceReference{
				{ID: new("/subscriptions/sub/resourceGroups/rg/providers/Microsoft.Storage/storageAccounts/sa")},
			},
			expectedResponse: &recipes.RecipeOutput{
				Values:    map[string]any{"endpoint": "https://sa.blob.core.windows.net", "name": "mystorageaccount"},
				Secrets:   map[string]any{},
				Resources: []string{"/subscriptions/sub/resourceGroups/rg/providers/Microsoft.Storage/storageAccounts/sa"},
				Status: &rpv1.RecipeStatus{
					TemplateKind: recipes.TemplateKindBicep,
					TemplatePath: "br:mcr.microsoft.com/bicep/avm/res/storage/storage-account:0.14.3",
				},
			},
		},
		{
			desc: "direct module without outputs mapping - pass through all",
			definition: recipes.EnvironmentDefinition{
				TemplatePath: "br:mcr.microsoft.com/bicep/avm/res/storage/storage-account:0.14.3",
			},
			outputs: map[string]any{
				"storageEndpoint": map[string]any{"type": "string", "value": "https://sa.blob.core.windows.net"},
				"storageName":     map[string]any{"type": "string", "value": "mystorageaccount"},
			},
			resources:   nil,
			expectedErr: nil,
			expectedResponse: &recipes.RecipeOutput{
				Values: map[string]any{
					"storageEndpoint": "https://sa.blob.core.windows.net",
					"storageName":     "mystorageaccount",
				},
				Secrets: map[string]any{},
				Status: &rpv1.RecipeStatus{
					TemplateKind: recipes.TemplateKindBicep,
					TemplatePath: "br:mcr.microsoft.com/bicep/avm/res/storage/storage-account:0.14.3",
				},
			},
		},
		{
			desc: "direct module without mapping - secure outputs routed to secrets",
			definition: recipes.EnvironmentDefinition{
				TemplatePath: "br:mcr.microsoft.com/bicep/avm/res/storage/storage-account:0.14.3",
			},
			outputs: map[string]any{
				"endpoint":  map[string]any{"type": "string", "value": "https://sa.blob.core.windows.net"},
				"accessKey": map[string]any{"type": "securestring", "value": "s3cr3t"},
				"config":    map[string]any{"type": "secureObject", "value": map[string]any{"token": "abc"}},
			},
			resources: nil,
			expectedResponse: &recipes.RecipeOutput{
				Values: map[string]any{"endpoint": "https://sa.blob.core.windows.net"},
				Secrets: map[string]any{
					"accessKey": "s3cr3t",
					"config":    map[string]any{"token": "abc"},
				},
				Status: &rpv1.RecipeStatus{
					TemplateKind: recipes.TemplateKindBicep,
					TemplatePath: "br:mcr.microsoft.com/bicep/avm/res/storage/storage-account:0.14.3",
				},
			},
		},
		{
			desc: "direct module with mapping - secure output mapped to secret property",
			definition: recipes.EnvironmentDefinition{
				TemplatePath: "br:mcr.microsoft.com/bicep/avm/res/storage/storage-account:0.14.3",
				Outputs:      map[string]string{"endpoint": "storageEndpoint", "password": "accessKey"},
			},
			outputs: map[string]any{
				"storageEndpoint": map[string]any{"type": "string", "value": "https://sa.blob.core.windows.net"},
				"accessKey":       map[string]any{"type": "securestring", "value": "s3cr3t"},
			},
			resources: nil,
			expectedResponse: &recipes.RecipeOutput{
				Values:  map[string]any{"endpoint": "https://sa.blob.core.windows.net"},
				Secrets: map[string]any{"password": "s3cr3t"},
				Status: &rpv1.RecipeStatus{
					TemplateKind: recipes.TemplateKindBicep,
					TemplatePath: "br:mcr.microsoft.com/bicep/avm/res/storage/storage-account:0.14.3",
				},
			},
		},
		{
			desc: "direct module with secretOutputs mapping - plain output forced to secret (AVM)",
			definition: recipes.EnvironmentDefinition{
				TemplatePath:  "br:mcr.microsoft.com/bicep/avm/res/event-hub/namespace:0.14.2",
				Outputs:       map[string]string{"host": "name"},
				SecretOutputs: map[string]string{"connectionString": "primaryConnectionString"},
			},
			outputs: map[string]any{
				// The AVM module declares primaryConnectionString as a plain string output, not securestring.
				"name":                    map[string]any{"type": "string", "value": "myhub"},
				"primaryConnectionString": map[string]any{"type": "string", "value": "Endpoint=sb://myhub/;SharedAccessKey=abc"},
			},
			resources: nil,
			expectedResponse: &recipes.RecipeOutput{
				Values:  map[string]any{"host": "myhub"},
				Secrets: map[string]any{"connectionString": "Endpoint=sb://myhub/;SharedAccessKey=abc"},
				Status: &rpv1.RecipeStatus{
					TemplateKind: recipes.TemplateKindBicep,
					TemplatePath: "br:mcr.microsoft.com/bicep/avm/res/event-hub/namespace:0.14.2",
				},
			},
		},
		{
			desc: "module with result and no outputs mapping - wrapped behavior",
			definition: recipes.EnvironmentDefinition{
				TemplatePath: "radiusdev.azurecr.io/recipes/mongo:1.0",
			},
			outputs: map[string]any{
				"result": map[string]any{
					"value": map[string]any{
						"values":    map[string]any{"host": "mongohost"},
						"secrets":   map[string]any{"connStr": "mongodb://..."},
						"resources": []any{"res1"},
					},
				},
			},
			resources: nil,
			expectedResponse: &recipes.RecipeOutput{
				Values:    map[string]any{"host": "mongohost"},
				Secrets:   map[string]any{"connStr": "mongodb://..."},
				Resources: []string{"res1"},
				Status: &rpv1.RecipeStatus{
					TemplateKind: recipes.TemplateKindBicep,
					TemplatePath: "radiusdev.azurecr.io/recipes/mongo:1.0",
				},
			},
		},
		{
			desc: "module with both result and outputs mapping - outputs wins",
			definition: recipes.EnvironmentDefinition{
				TemplatePath: "br:myregistry/module:1.0",
				Outputs:      map[string]string{"host": "hostname"},
			},
			outputs: map[string]any{
				"result": map[string]any{
					"value": map[string]any{"values": map[string]any{"host": "from-result"}},
				},
				"hostname": map[string]any{"type": "string", "value": "from-arm-output"},
			},
			resources: nil,
			expectedResponse: &recipes.RecipeOutput{
				Values:  map[string]any{"host": "from-arm-output"},
				Secrets: map[string]any{},
				Status: &rpv1.RecipeStatus{
					TemplateKind: recipes.TemplateKindBicep,
					TemplatePath: "br:myregistry/module:1.0",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			resp, err := d.prepareRecipeResponse(tt.definition, tt.outputs, tt.resources)
			require.Equal(t, tt.expectedErr, err)
			require.Equal(t, tt.expectedResponse, resp)
		})
	}
}

func Test_Bicep_PrepareRecipeResponse_MissingSecretOutput(t *testing.T) {
	d := &bicepDriver{}
	definition := recipes.EnvironmentDefinition{
		Name:          "eventhub",
		ResourceType:  "Demo.Messaging/kafka",
		SecretOutputs: map[string]string{"connectionString": "primaryConnectionString"},
	}
	outputs := map[string]any{
		"name":                    map[string]any{"type": "string", "value": "myhub"},
		"primaryConnectionString": map[string]any{"type": "string", "value": nil},
	}

	response, err := d.prepareRecipeResponse(definition, outputs, nil)
	require.Equal(t, &recipes.RecipeOutput{}, response)
	require.EqualError(t, err, `recipe "eventhub" for resource type "Demo.Messaging/kafka": invalid outputs mapping: missing deployment output values for secrets["connectionString"] -> "primaryConnectionString"; available deployment outputs: "name"`)

	var missingOutputErr *recipes_util.MissingOutputValuesError
	require.ErrorAs(t, err, &missingOutputErr)
}

func Test_WrapARMParameters(t *testing.T) {
	tests := []struct {
		name     string
		params   map[string]any
		expected map[string]any
	}{
		{
			name:     "nil params returns empty map",
			params:   nil,
			expected: map[string]any{},
		},
		{
			name:   "wraps values in ARM format",
			params: map[string]any{"name": "mysa", "sku": "Standard"},
			expected: map[string]any{
				"name": map[string]any{"value": "mysa"},
				"sku":  map[string]any{"value": "Standard"},
			},
		},
		{
			name:   "skips nil values",
			params: map[string]any{"name": "mysa", "optional": nil},
			expected: map[string]any{
				"name": map[string]any{"value": "mysa"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := wrapARMParameters(tt.params)
			require.Equal(t, tt.expected, result)
		})
	}
}

func Test_Bicep_Execute_AddsAWSProviderResourceIDs(t *testing.T) {
	const bucketID = "/planes/aws/aws/accounts/123456789012/regions/us-west-2/providers/AWS.S3/Bucket/my-bucket"
	const queueID = "/planes/aws/aws/accounts/123456789012/regions/us-west-2/providers/AWS.SQS/Queue/my-queue"

	ts := registrytest.NewFakeRegistryServer(t)
	t.Cleanup(ts.CloseServer)
	deploymentClient := &deploymentClientSpy{
		response: &clients.ClientCreateOrUpdateResponse{
			Properties: &armdeployments.DeploymentPropertiesExtended{
				// The recipe deploys the bucket and returns an existing queue in result.resources.
				OutputResources: []*armdeployments.ResourceReference{{ID: new(bucketID)}},
				Outputs: map[string]any{
					recipes.ResultPropertyName: map[string]any{
						"type":  "object",
						"value": map[string]any{"resources": []any{queueID}},
					},
				},
			},
		},
	}

	ctrl := gomock.NewController(t)
	client := processors.NewMockResourceClient(ctrl)
	client.EXPECT().GetProperties(gomock.Any(), bucketID).Return(map[string]any{"Arn": "arn:aws:s3:::my-bucket", "BucketName": "my-bucket"}, nil)
	client.EXPECT().GetProperties(gomock.Any(), queueID).Return(map[string]any{"Arn": "arn:aws:sqs:us-west-2:123456789012:my-queue"}, nil)

	d := &bicepDriver{
		DeploymentClient:      deploymentClient,
		RegistryClient:        ts.TestServer.Client(),
		resourceClientFactory: func(*rest.Config) processors.ResourceClient { return client },
	}

	result, err := d.Execute(t.Context(), driver.ExecuteOptions{
		Recipe: recipes.ResourceMetadata{
			Name:          "test-resource",
			ResourceID:    "/planes/radius/local/resourceGroups/test-rg/providers/Test.Resources/resources/test-resource",
			EnvironmentID: "/planes/radius/local/resourceGroups/test-rg/providers/Radius.Core/environments/env",
		},
		Definition: recipes.EnvironmentDefinition{
			Name:         "aws-recipe",
			Driver:       recipes.TemplateKindBicep,
			TemplatePath: ts.TestImageURL,
		},
	})
	require.NoError(t, err)
	require.ElementsMatch(t, []string{bucketID, queueID}, result.Resources)
	require.Equal(t, "arn:aws:s3:::my-bucket", result.ProviderResourceID(bucketID))
	require.Equal(t, "arn:aws:sqs:us-west-2:123456789012:my-queue", result.ProviderResourceID(queueID))
}

func Test_Bicep_AddAWSProviderResourceIDs(t *testing.T) {
	const (
		prefix = "/planes/aws/aws/accounts/123456789012/regions/us-west-2/providers/"
		// AWS::S3::Bucket keeps its ARN in Arn.
		bucketID = prefix + "AWS.S3/Bucket/my-bucket"
		// AWS::SNS::Topic keeps its ARN in TopicArn, a <ResourceName>Arn property.
		topicID = prefix + "AWS.SNS/Topic/arn:aws:sns:us-west-2:123456789012:my-topic"
		// AWS::IAM::ManagedPolicy keeps its ARN in PolicyArn, its read-only primary identifier.
		policyID = prefix + "AWS.IAM/ManagedPolicy/my-policy"
		// AWS::GameLift::ContainerFleet keeps its ARN in FleetArn, a hand-reviewed decision. Its other *Arn
		// properties are container group definitions.
		fleetID = prefix + "AWS.GameLift/ContainerFleet/fleet-1"
		// AWS::EC2::VPC has no ARN property, so it isn't read.
		vpcID = prefix + "AWS.EC2/VPC/vpc-0123456789abcdef0"
		// AWS::Redshift::Cluster only has unrelated *Arn properties, so it isn't read.
		clusterID = prefix + "AWS.Redshift/Cluster/my-cluster"
		// AWS::SNS::TopicInlinePolicy is identified by its topic's ARN, which isn't its own ARN.
		topicPolicyID = prefix + "AWS.SNS/TopicInlinePolicy/arn:aws:sns:us-west-2:123456789012:my-topic"
		// AWS::Example::Thing isn't in the catalog, so the Arn, ARN, or arn property is used.
		unknownID = prefix + "AWS.Example/Thing/thing-1"
		// The mapped property isn't an ARN.
		invalidID = prefix + "AWS.S3/Bucket/invalid"
		// The read fails.
		failedID = prefix + "AWS.S3/Bucket/failed"
		azureID  = "/subscriptions/sub/resourceGroups/rg/providers/Microsoft.Storage/storageAccounts/account"
		k8sID    = "/planes/kubernetes/local/namespaces/default/providers/core/Secret/my-secret"
	)

	ctrl := gomock.NewController(t)
	client := processors.NewMockResourceClient(ctrl)
	client.EXPECT().GetProperties(gomock.Any(), bucketID).Return(map[string]any{"Arn": "arn:aws:s3:::my-bucket"}, nil)
	client.EXPECT().GetProperties(gomock.Any(), topicID).Return(map[string]any{"TopicArn": "arn:aws:sns:us-west-2:123456789012:my-topic"}, nil)
	client.EXPECT().GetProperties(gomock.Any(), policyID).Return(map[string]any{"PolicyArn": "arn:aws:iam::123456789012:policy/my-policy"}, nil)
	client.EXPECT().GetProperties(gomock.Any(), fleetID).Return(map[string]any{
		"GameServerContainerGroupDefinitionArn": "arn:aws:gamelift:us-west-2:123456789012:containergroupdefinition/game:1",
		"FleetArn":                              "arn:aws:gamelift:us-west-2:123456789012:containerfleet/fleet-1",
		"Arn":                                   "arn:aws:gamelift:us-west-2:123456789012:not-the-mapped-property",
	}, nil)
	client.EXPECT().GetProperties(gomock.Any(), unknownID).Return(map[string]any{"ARN": "arn:aws:example:us-west-2:123456789012:thing/thing-1"}, nil)
	client.EXPECT().GetProperties(gomock.Any(), invalidID).Return(map[string]any{"Arn": "not-an-arn"}, nil)
	client.EXPECT().GetProperties(gomock.Any(), failedID).Return(nil, errors.New("access denied"))

	factoryCalls := 0
	d := &bicepDriver{resourceClientFactory: func(config *rest.Config) processors.ResourceClient {
		require.Nil(t, config)
		factoryCalls++
		return client
	}}

	output := &recipes.RecipeOutput{Resources: []string{
		bucketID, topicID, policyID, fleetID, vpcID, clusterID, topicPolicyID, unknownID, invalidID, failedID, azureID, k8sID, "not-a-resource-id",
	}}
	d.addAWSProviderResourceIDs(t.Context(), output)

	require.Equal(t, 1, factoryCalls)
	require.Equal(t, map[string]string{
		bucketID:  "arn:aws:s3:::my-bucket",
		topicID:   "arn:aws:sns:us-west-2:123456789012:my-topic",
		policyID:  "arn:aws:iam::123456789012:policy/my-policy",
		fleetID:   "arn:aws:gamelift:us-west-2:123456789012:containerfleet/fleet-1",
		unknownID: "arn:aws:example:us-west-2:123456789012:thing/thing-1",
	}, output.ProviderResourceIDs)
}

func Test_Bicep_AddAWSProviderResourceIDs_NoResourceClient(t *testing.T) {
	const bucketID = "/planes/aws/aws/accounts/123456789012/regions/global/providers/AWS.S3/Bucket/my-bucket"

	d := &bicepDriver{}
	output := &recipes.RecipeOutput{Resources: []string{bucketID}}
	d.addAWSProviderResourceIDs(t.Context(), output)

	require.Empty(t, output.ProviderResourceIDs)
}

func Test_Bicep_AddAWSProviderResourceIDs_FactoryReturnsNil(t *testing.T) {
	const bucketID = "/planes/aws/aws/accounts/123456789012/regions/global/providers/AWS.S3/Bucket/my-bucket"

	d := &bicepDriver{resourceClientFactory: func(*rest.Config) processors.ResourceClient { return nil }}
	output := &recipes.RecipeOutput{Resources: []string{bucketID}}
	d.addAWSProviderResourceIDs(t.Context(), output)

	require.Empty(t, output.ProviderResourceIDs)
}

// Cloud Control returns a log group ARN with a trailing ":*". The driver stores it without the suffix, the same value
// the Terraform driver stores for the same log group.
func Test_Bicep_AddAWSProviderResourceIDs_LogGroup(t *testing.T) {
	const logGroupID = "/planes/aws/aws/accounts/179022619019/regions/us-east-2/providers/AWS.Logs/LogGroup/orders"

	ctrl := gomock.NewController(t)
	client := processors.NewMockResourceClient(ctrl)
	client.EXPECT().GetProperties(gomock.Any(), logGroupID).Return(map[string]any{"Arn": "arn:aws:logs:us-east-2:179022619019:log-group:orders:*"}, nil)

	d := &bicepDriver{resourceClientFactory: func(*rest.Config) processors.ResourceClient { return client }}
	output := &recipes.RecipeOutput{Resources: []string{logGroupID}}
	d.addAWSProviderResourceIDs(t.Context(), output)

	require.Equal(t, "arn:aws:logs:us-east-2:179022619019:log-group:orders", output.ProviderResourceID(logGroupID))
}

func Test_Bicep_AddAWSProviderResourceIDs_IDsDifferingByCase(t *testing.T) {
	const lowerID = "/planes/aws/aws/accounts/123456789012/regions/us-west-2/providers/AWS.SNS/Topic/arn:aws:sns:us-west-2:123456789012:orders"
	const upperID = "/planes/aws/aws/accounts/123456789012/regions/us-west-2/providers/AWS.SNS/Topic/arn:aws:sns:us-west-2:123456789012:Orders"

	ctrl := gomock.NewController(t)
	client := processors.NewMockResourceClient(ctrl)
	client.EXPECT().GetProperties(gomock.Any(), lowerID).Return(map[string]any{"TopicArn": "arn:aws:sns:us-west-2:123456789012:orders"}, nil)
	client.EXPECT().GetProperties(gomock.Any(), upperID).Return(map[string]any{"TopicArn": "arn:aws:sns:us-west-2:123456789012:Orders"}, nil)

	d := &bicepDriver{resourceClientFactory: func(*rest.Config) processors.ResourceClient { return client }}
	output := &recipes.RecipeOutput{Resources: []string{lowerID, upperID}}
	d.addAWSProviderResourceIDs(t.Context(), output)

	require.Equal(t, "arn:aws:sns:us-west-2:123456789012:orders", output.ProviderResourceID(lowerID))
	require.Equal(t, "arn:aws:sns:us-west-2:123456789012:Orders", output.ProviderResourceID(upperID))
}

const (
	fakeRegistryUser     = "user"
	fakeRegistryPassword = "pass"
	publicRegistryHost   = "public.example.com"
	dockerHubHost        = "registry-1.docker.io"
	dockerHubAuthHost    = "auth.docker.io"
	bearerPrefix         = "Bearer "
)

// registryRequest records one request that fakeRegistries received.
type registryRequest struct{ scheme, host, authorization string }

func (r registryRequest) sentCredentials() bool {
	return r.authorization == "Basic" || r.authorization == bearerPrefix+"pull"
}

// fakeRegistries replaces http.DefaultTransport, which ORAS uses for every request the
// driver makes. Docker Hub issues a "pull" token for valid basic credentials and an
// "anonymous" token otherwise. Anonymous tokens can pull only library/ repositories.
// public.example.com needs no credentials. Every other host needs basic credentials.
type fakeRegistries struct {
	mu       sync.Mutex
	requests []registryRequest
	manifest []byte
	layer    []byte
}

func newFakeRegistries(t *testing.T) *fakeRegistries {
	t.Helper()
	layer := []byte(`{"parameters":{"name":{"type":"string"}}}`)
	f := &fakeRegistries{
		layer:    layer,
		manifest: []byte(`{"layers":[{"digest":"` + digest.FromBytes(layer).String() + `"}]}`),
	}
	original := http.DefaultTransport
	http.DefaultTransport = f
	t.Cleanup(func() { http.DefaultTransport = original })
	return f
}

func (f *fakeRegistries) RoundTrip(req *http.Request) (*http.Response, error) {
	recorder := httptest.NewRecorder()
	f.serve(recorder, req)
	resp := recorder.Result()
	resp.Request = req
	return resp, nil
}

func (f *fakeRegistries) take() []registryRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	requests := f.requests
	f.requests = nil
	return requests
}

func (f *fakeRegistries) serve(w http.ResponseWriter, r *http.Request) {
	authorization := r.Header.Get("Authorization")
	if strings.HasPrefix(authorization, "Basic ") {
		authorization = "Basic"
	}
	f.mu.Lock()
	f.requests = append(f.requests, registryRequest{scheme: r.URL.Scheme, host: r.URL.Host, authorization: authorization})
	f.mu.Unlock()

	switch r.URL.Host {
	case dockerHubAuthHost:
		token := "anonymous"
		if user, password, ok := r.BasicAuth(); ok {
			if user != fakeRegistryUser || password != fakeRegistryPassword {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			token = "pull"
		}
		// A failed write means the client went away. The client then fails the test.
		_ = json.NewEncoder(w).Encode(map[string]string{"token": token})
		return
	case dockerHubHost:
		official := strings.HasPrefix(r.URL.Path, "/v2/library/")
		if authorization != bearerPrefix+"pull" && (!official || authorization != bearerPrefix+"anonymous") {
			w.Header().Set("WWW-Authenticate", bearerPrefix+`realm="https://auth.docker.io/token",service="registry.docker.io"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
	case publicRegistryHost:
	default:
		if user, password, ok := r.BasicAuth(); !ok || user != fakeRegistryUser || password != fakeRegistryPassword {
			w.Header().Set("WWW-Authenticate", `Basic realm="private"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
	}

	var body []byte
	var contentType string
	switch {
	case strings.Contains(r.URL.Path, "/manifests/"):
		body, contentType = f.manifest, ocispec.MediaTypeImageManifest
	case strings.Contains(r.URL.Path, "/blobs/"):
		body, contentType = f.layer, ocispec.MediaTypeImageLayer
	default:
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Docker-Content-Digest", digest.FromBytes(body).String())
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	if r.Method == http.MethodGet {
		// A failed write means the client went away. The client then fails the test.
		_, _ = w.Write(body)
	}
}

type registrySecret struct{ secretType, user, password string }

var (
	validRegistrySecret   = registrySecret{"basicAuthentication", fakeRegistryUser, fakeRegistryPassword}
	wrongPasswordSecret   = registrySecret{"basicAuthentication", fakeRegistryUser, "wrong"}
	unsupportedSecretType = registrySecret{"notAType", fakeRegistryUser, fakeRegistryPassword}
)

type registryAuthCase struct {
	name      string
	path      string
	plainHTTP bool
	// keys are the registry authentication keys that all point to the same secret.
	keys   []string
	secret *registrySecret
	// wantCode, when set, is the expected error code. wantErr, when set, is a substring of the error.
	// When both are empty, the call must succeed with the recipe parameters.
	wantCode string
	wantErr  string
	// wantCreds lists exactly the hosts that receive credentials.
	wantCreds []string
	// wantHosts, when set, lists exactly the registry hosts that are contacted.
	wantHosts  []string
	noRequests bool
}

func (tc registryAuthCase) options() driver.BaseOptions {
	secret := validRegistrySecret
	if tc.secret != nil {
		secret = *tc.secret
	}
	authentication := map[string]corerp_datamodel.RegistrySecretConfig{}
	for _, key := range tc.keys {
		authentication[key] = corerp_datamodel.RegistrySecretConfig{Secret: "creds"}
	}
	return driver.BaseOptions{
		Configuration: recipes.Configuration{RecipeConfig: corerp_datamodel.RecipeConfigProperties{
			Bicep: corerp_datamodel.BicepConfigProperties{Authentication: authentication},
		}},
		Secrets: map[string]recipes.SecretData{
			"creds": {Type: secret.secretType, Data: map[string]string{"username": secret.user, "password": secret.password}},
		},
		Definition: recipes.EnvironmentDefinition{
			Name:         "redis",
			Driver:       recipes.TemplateKindBicep,
			TemplatePath: tc.path,
			PlainHTTP:    tc.plainHTTP,
			ResourceType: "Applications.Datastores/redisCaches",
		},
	}
}

// errorCode returns the code of a recipe or client error, or "" for any other error.
func errorCode(err error) string {
	if recipeErr, ok := errors.AsType[*recipes.RecipeError](err); ok {
		return recipeErr.ErrorDetails.Code
	}
	if clientErr, ok := errors.AsType[*v1.ErrClientRP](err); ok {
		return clientErr.Code
	}
	return ""
}

// runRegistryAuthCase calls the driver and checks the result, which hosts received
// credentials, and that plainHttp, not the scheme in the path, selects the transport.
func runRegistryAuthCase(t *testing.T, f *fakeRegistries, tc registryAuthCase, call func(driver.BaseOptions) (map[string]any, error)) {
	t.Run(tc.name, func(t *testing.T) {
		// The anonymous client caches Docker Hub tokens across calls.
		cache := auth.DefaultClient.Cache
		auth.DefaultClient.Cache = auth.NewCache()
		t.Cleanup(func() { auth.DefaultClient.Cache = cache })
		f.take()

		data, err := call(tc.options())

		requests := f.take()
		wantScheme := "https"
		if tc.plainHTTP {
			wantScheme = "http"
		}
		creds, hosts := map[string]bool{}, map[string]bool{}
		for _, r := range requests {
			if r.host != dockerHubAuthHost {
				require.Equalf(t, wantScheme, r.scheme, "scheme for %s", r.host)
				hosts[r.host] = true
			}
			if r.sentCredentials() {
				creds[r.host] = true
			}
		}
		if tc.noRequests {
			require.Empty(t, requests, "no registry or token request is expected")
		} else {
			require.NotEmpty(t, requests)
		}
		require.ElementsMatch(t, tc.wantCreds, slices.Collect(maps.Keys(creds)), "hosts that received credentials")
		if tc.wantHosts != nil {
			require.ElementsMatch(t, tc.wantHosts, slices.Collect(maps.Keys(hosts)), "registry hosts contacted")
		}

		if tc.wantCode == "" && tc.wantErr == "" {
			require.NoError(t, err)
			require.Equal(t, map[string]any{"name": map[string]any{"type": "string"}}, data["parameters"])
			return
		}
		require.Error(t, err)
		require.Equal(t, tc.wantCode, errorCode(err), err.Error())
		require.ErrorContains(t, err, tc.wantErr)
	})
}

// Test_Bicep_GetRecipeMetadata_RegistryAuthentication replaces http.DefaultTransport,
// so it must not run in parallel.
func Test_Bicep_GetRecipeMetadata_RegistryAuthentication(t *testing.T) {
	f := newFakeRegistries(t)
	manifestDigest := digest.FromBytes(f.manifest).String()
	private := "registry.example.com:5000"
	acr := "myreg.azurecr.io"
	dockerHub := []string{dockerHubAuthHost, dockerHubHost}
	fetchFailed := recipes.RecipeLanguageFailure

	testset := []registryAuthCase{
		// Private registry with basic authentication.
		{name: "port and tag", path: private + "/recipes/redis:1.0", keys: []string{private}, wantCreds: []string{private}},
		{name: "port and digest", path: private + "/recipes/redis@" + manifestDigest, keys: []string{private}, wantCreds: []string{private}},
		{name: "tag and digest", path: acr + "/recipes/redis:1.0@" + manifestDigest, keys: []string{acr}, wantCreds: []string{acr}},
		{name: "IPv6 and port", path: "[::1]:5000/recipes/redis:1.0", keys: []string{"[::1]:5000"}, wantCreds: []string{"[::1]:5000"}},
		{name: "http scheme with plainHttp", path: "http://" + private + "/recipes/redis:1.0", plainHTTP: true, keys: []string{private}, wantCreds: []string{private}},
		{name: "https scheme", path: "https://" + acr + "/recipes/redis:1.0", keys: []string{acr}, wantCreds: []string{acr}},
		{name: "https scheme and digest", path: "https://" + acr + "/recipes/redis@" + manifestDigest, keys: []string{acr}, wantCreds: []string{acr}},
		{name: "https scheme, plainHttp true sends http", path: "https://" + private + "/recipes/redis:1.0", plainHTTP: true, keys: []string{private}, wantCreds: []string{private}},
		{name: "http scheme, plainHttp false sends https", path: "http://" + private + "/recipes/redis:1.0", keys: []string{private}, wantCreds: []string{private}},

		// Registry without credentials. plainHttp selects the scheme.
		{name: "public, https scheme, plainHttp true sends http", path: "https://" + publicRegistryHost + "/recipes/redis:1.0", plainHTTP: true},
		{name: "public, http scheme, plainHttp false sends https", path: "http://" + publicRegistryHost + "/recipes/redis:1.0"},

		// Credentials stay with their registry.
		{name: "key for another registry", path: "other.example.com/recipes/redis:1.0", keys: []string{acr}, wantCode: fetchFailed, wantHosts: []string{"other.example.com"}},
		{name: "public registry, key for another registry", path: publicRegistryHost + "/recipes/redis:1.0", keys: []string{acr}},

		// Docker Hub. The key is docker.io and requests go to registry-1.docker.io.
		{name: "Docker Hub docker.io namespace", path: "docker.io/myuser/redis:1.0", keys: []string{"docker.io"}, wantCreds: dockerHub},
		{name: "Docker Hub https scheme", path: "https://docker.io/myuser/redis:1.0", keys: []string{"docker.io"}, wantCreds: dockerHub},
		{name: "Docker Hub short name", path: "myuser/redis:1.0", keys: []string{"docker.io"}, wantCreds: dockerHub, wantHosts: []string{dockerHubHost}},
		{name: "Docker Hub short name and digest", path: "myuser/redis@" + manifestDigest, keys: []string{"docker.io"}, wantCreds: dockerHub},
		{name: "Docker Hub official image, no authentication", path: "redis:1.0", wantHosts: []string{dockerHubHost}},
		{name: "Docker Hub official image digest, no authentication", path: "redis@" + manifestDigest},
		{name: "Docker Hub official image with docker.io key sends credentials", path: "redis:1.0", keys: []string{"docker.io"}, wantCreds: dockerHub},
		{name: "Docker Hub index.docker.io path, docker.io key", path: "index.docker.io/myuser/redis:1.0", keys: []string{"docker.io"}, wantCreds: dockerHub},
		{name: "Docker Hub registry-1.docker.io path and key", path: "registry-1.docker.io/myuser/redis:1.0", keys: []string{"registry-1.docker.io"}, wantCreds: dockerHub},

		// Keys that do not match the normalized registry.
		{name: "Docker Hub namespace key", path: "myuser/redis:1.0", keys: []string{"myuser"}, wantCode: fetchFailed},
		{name: "Docker Hub index.docker.io key", path: "index.docker.io/myuser/redis:1.0", keys: []string{"index.docker.io"}, wantCode: fetchFailed},
		{name: "key case differs", path: "MyReg.example.com/recipes/redis:1.0", keys: []string{"myreg.example.com"}, wantCode: fetchFailed},
		{name: "explicit 443, key without port", path: acr + ":443/recipes/redis:1.0", keys: []string{acr}, wantCode: fetchFailed},
		{name: "key with scheme", path: acr + "/recipes/redis:1.0", keys: []string{"https://" + acr}, wantCode: fetchFailed},
		{name: "in-cluster name without dot or port goes to Docker Hub", path: "registry/recipes/redis:1.0", keys: []string{"registry"}, wantCode: fetchFailed, wantHosts: []string{dockerHubHost}},

		// Paths that are not valid send no registry or token request.
		{name: "uppercase scheme", path: "HTTPS://" + acr + "/recipes/redis:1.0", keys: []string{acr}, wantCode: v1.CodeInvalid, wantErr: "invalid path", noRequests: true},
		{name: "oci scheme", path: "oci://" + acr + "/recipes/redis:1.0", keys: []string{acr}, wantCode: v1.CodeInvalid, wantErr: "invalid path", noRequests: true},
		{name: "bicep br: syntax", path: "br:" + acr + "/recipes/redis:1.0", keys: []string{acr}, wantCode: v1.CodeInvalid, wantErr: "invalid path", noRequests: true},
		{name: "bicep br: syntax skips auth client setup", path: "br:" + acr + "/recipes/redis:1.0", keys: []string{acr}, secret: &unsupportedSecretType, wantCode: v1.CodeInvalid, wantErr: "invalid path", noRequests: true},
		{name: "uppercase repository", path: acr + "/Recipes/redis:1.0", keys: []string{acr}, wantCode: v1.CodeInvalid, wantErr: "invalid path", noRequests: true},
		{name: "bracketed host that is not an IP address", path: "[000]:0000/000000", wantCode: v1.CodeInvalid, wantErr: "invalid path", noRequests: true},

		// Authentication failures on valid paths still fail.
		{name: "unsupported secret type", path: acr + "/recipes/redis:1.0", keys: []string{acr}, secret: &unsupportedSecretType, wantErr: "invalid type", noRequests: true},
		{name: "wrong password", path: private + "/recipes/redis:1.0", keys: []string{private}, secret: &wrongPasswordSecret, wantCode: fetchFailed, wantCreds: []string{private}},
		{name: "https scheme, wrong password", path: "https://" + acr + "/recipes/redis:1.0", keys: []string{acr}, secret: &wrongPasswordSecret, wantCode: fetchFailed, wantCreds: []string{acr}},
	}
	for _, tc := range testset {
		runRegistryAuthCase(t, f, tc, func(opts driver.BaseOptions) (map[string]any, error) {
			return (&bicepDriver{}).GetRecipeMetadata(t.Context(), opts)
		})
	}
}

// Test_Bicep_Execute_RegistryAuthenticationErrors checks that Execute wraps download
// failures as RecipeDownloadFailed and returns auth client errors unchanged. It replaces
// http.DefaultTransport, so it must not run in parallel.
func Test_Bicep_Execute_RegistryAuthenticationErrors(t *testing.T) {
	f := newFakeRegistries(t)
	acr := "myreg.azurecr.io"
	testset := []registryAuthCase{
		{name: "bicep br: syntax", path: "br:" + acr + "/recipes/redis:1.0", keys: []string{acr}, wantCode: recipes.RecipeDownloadFailed, wantErr: "invalid path", noRequests: true},
		{name: "uppercase repository", path: acr + "/Recipes/redis:1.0", keys: []string{acr}, wantCode: recipes.RecipeDownloadFailed, wantErr: "invalid path", noRequests: true},
		{name: "bracketed host that is not an IP address", path: "[000]:0000/000000", wantCode: recipes.RecipeDownloadFailed, wantErr: "invalid path", noRequests: true},
		{name: "https scheme, wrong password", path: "https://" + acr + "/recipes/redis:1.0", keys: []string{acr}, secret: &wrongPasswordSecret, wantCode: recipes.RecipeDownloadFailed, wantCreds: []string{acr}},
		{name: "unsupported secret type", path: acr + "/recipes/redis:1.0", keys: []string{acr}, secret: &unsupportedSecretType, wantErr: "invalid type", noRequests: true},
	}
	for _, tc := range testset {
		runRegistryAuthCase(t, f, tc, func(opts driver.BaseOptions) (map[string]any, error) {
			_, err := (&bicepDriver{}).Execute(t.Context(), driver.ExecuteOptions{BaseOptions: opts})
			return nil, err
		})
	}
}
