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

package dynamic

import (
	"encoding/json"
	"net/http"
	"slices"
	"testing"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/dynamicrp"
	"github.com/radius-project/radius/pkg/dynamicrp/testhost"
	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/recipes/configloader"
	"github.com/radius-project/radius/pkg/recipes/driver"
	recipes_util "github.com/radius-project/radius/pkg/recipes/util"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	ucptesthost "github.com/radius-project/radius/pkg/ucp/testhost"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

const (
	testEnvironmentID      = testResourceGroupID + "/providers/Radius.Core/environments/env"
	testOtherEnvironmentID = testResourceGroupID + "/providers/Radius.Core/environments/other-env"
	oldOutputResourceID    = "/planes/kubernetes/local/namespaces/app/providers/core/ConfigMap/old"
	newOutputResourceID    = "/planes/kubernetes/local/namespaces/app/providers/core/ConfigMap/new"
	clientOutputResourceID = "/planes/kubernetes/local/namespaces/kube-system/providers/core/ConfigMap/client-sent"
)

// startRecipeHost starts Dynamic RP with a recipe resource type whose schema rejects undeclared properties.
func startRecipeHost(t *testing.T) (*driver.MockDriver, *ucptesthost.TestHost) {
	t.Helper()
	ctrl := gomock.NewController(t)
	mockDriver := driver.NewMockDriver(ctrl)
	mockConfigLoader := configloader.NewMockConfigurationLoader(ctrl)
	_, ucp := testhost.Start(t, testhost.TestHostOptionFunc(func(options *dynamicrp.Options) {
		options.Recipes.Drivers = map[string]func(*dynamicrp.Options) (driver.Driver, error){
			"test": func(*dynamicrp.Options) (driver.Driver, error) {
				return mockDriver, nil
			},
		}
		options.Recipes.ConfigurationLoader = mockConfigLoader
	}))

	createRadiusPlane(ucp)
	createResourceProvider(ucp)
	createRecipeResourceType(ucp)
	createAPIVersion(ucp, recipeResourceTypeName, map[string]any{
		"type": "object",
		"properties": map[string]any{
			"environment":  map[string]any{"type": "string"},
			"physicalName": map[string]any{"type": "string"},
		},
		"additionalProperties": false,
	})
	createLocation(ucp, recipeResourceTypeName)
	createResourceGroup(ucp)

	mockConfigLoader.EXPECT().LoadRecipe(gomock.Any(), gomock.Any()).
		Return(&recipes.EnvironmentDefinition{
			Name:         "default",
			Driver:       "test",
			ResourceType: resourceProviderNamespace + "/" + recipeResourceTypeName,
			TemplatePath: "test-path",
		}, nil).AnyTimes()
	mockConfigLoader.EXPECT().LoadConfiguration(gomock.Any(), gomock.Any()).
		Return(&recipes.Configuration{}, nil).AnyTimes()

	return mockDriver, ucp
}

func recipeResource(environmentID string, physicalName string) map[string]any {
	return map[string]any{
		"properties": map[string]any{
			"environment":  environmentID,
			"physicalName": physicalName,
		},
	}
}

func recipeOutput(resourceIDs ...string) *recipes.RecipeOutput {
	return &recipes.RecipeOutput{
		Resources: resourceIDs,
		Status:    &rpv1.RecipeStatus{TemplateKind: "test", TemplatePath: "test-path"},
	}
}

func deployRecipeResource(t *testing.T, ucp *ucptesthost.TestHost, resource map[string]any) any {
	t.Helper()
	response := ucp.MakeTypedRequest(http.MethodPut, testRecipeResourceURL, resource)
	response.EqualsStatusCode(http.StatusCreated)
	response.WaitForOperationComplete(nil)
	return getRecipeResourceProperties(t, ucp)["provisioningState"]
}

func getRecipeResourceProperties(t *testing.T, ucp *ucptesthost.TestHost) map[string]any {
	t.Helper()
	response := ucp.MakeRequest(http.MethodGet, testRecipeResourceURL, nil)
	response.EqualsStatusCode(http.StatusOK)
	resource := map[string]any{}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &resource))
	return resource["properties"].(map[string]any)
}

func storedOutputResourceIDs(t *testing.T, ucp *ucptesthost.TestHost) []string {
	t.Helper()
	ids := []string{}
	status, _ := getRecipeResourceProperties(t, ucp)["status"].(map[string]any)
	outputResources, _ := status["outputResources"].([]any)
	for _, outputResource := range outputResources {
		ids = append(ids, outputResource.(map[string]any)["id"].(string))
	}
	return ids
}

func outputResourceIDs(outputResources []rpv1.OutputResource) []string {
	ids := []string{}
	for _, outputResource := range outputResources {
		ids = append(ids, outputResource.ID.String())
	}
	return ids
}

func Test_Dynamic_Resource_Recipe_Redeploy_TracksOutputResources(t *testing.T) {
	mockDriver, ucp := startRecipeHost(t)

	withPrevState := func(physicalName string, prevState ...string) any {
		return gomock.Cond(func(options driver.ExecuteOptions) bool {
			return options.Recipe.Properties["physicalName"] == physicalName && slices.Equal(options.PrevState, prevState)
		})
	}
	gomock.InOrder(
		mockDriver.EXPECT().Execute(gomock.Any(), withPrevState("old")).
			Return(recipeOutput(oldOutputResourceID), nil),
		// The recipe removes what it no longer returns, so it must get the stored outputs, not the request's.
		mockDriver.EXPECT().Execute(gomock.Any(), withPrevState("new", oldOutputResourceID)).
			Return(recipeOutput(newOutputResourceID), nil),
		mockDriver.EXPECT().Execute(gomock.Any(), withPrevState("failing", newOutputResourceID)).
			Return(nil, recipes.NewRecipeError(recipes.RecipeDeploymentFailed, "deployment failed", recipes_util.ExecutionError, nil)),
		mockDriver.EXPECT().Delete(gomock.Any(), gomock.Cond(func(options driver.DeleteOptions) bool {
			return slices.Equal(outputResourceIDs(options.OutputResources), []string{newOutputResourceID})
		})).Return(nil),
	)

	require.Equal(t, string(v1.ProvisioningStateSucceeded), deployRecipeResource(t, ucp, recipeResource(testEnvironmentID, "old")))
	require.Equal(t, []string{oldOutputResourceID}, storedOutputResourceIDs(t, ucp))

	renamed := recipeResource(testEnvironmentID, "new")
	renamed["properties"].(map[string]any)["status"] = map[string]any{
		"outputResources": []any{map[string]any{"id": clientOutputResourceID, "radiusManaged": true}},
	}
	require.Equal(t, string(v1.ProvisioningStateSucceeded), deployRecipeResource(t, ucp, renamed))
	require.Equal(t, []string{newOutputResourceID}, storedOutputResourceIDs(t, ucp))

	require.Equal(t, string(v1.ProvisioningStateFailed), deployRecipeResource(t, ucp, recipeResource(testEnvironmentID, "failing")))
	require.Equal(t, []string{newOutputResourceID}, storedOutputResourceIDs(t, ucp))

	response := ucp.MakeRequest(http.MethodDelete, testRecipeResourceURL, nil)
	response.WaitForOperationComplete(nil)
	response = ucp.MakeRequest(http.MethodGet, testRecipeResourceURL, nil)
	response.EqualsErrorCode(http.StatusNotFound, v1.CodeNotFound)
}

func Test_Dynamic_Resource_Recipe_Redeploy_RejectsEnvironmentChange(t *testing.T) {
	mockDriver, ucp := startRecipeHost(t)
	mockDriver.EXPECT().Execute(gomock.Any(), gomock.Any()).Return(recipeOutput(oldOutputResourceID), nil).Times(1)

	require.Equal(t, string(v1.ProvisioningStateSucceeded), deployRecipeResource(t, ucp, recipeResource(testEnvironmentID, "old")))

	response := ucp.MakeTypedRequest(http.MethodPut, testRecipeResourceURL, recipeResource(testOtherEnvironmentID, "old"))
	response.EqualsErrorCode(http.StatusBadRequest, v1.CodeInvalid)
	require.Equal(t, []string{oldOutputResourceID}, storedOutputResourceIDs(t, ucp))
}
