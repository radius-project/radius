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

package environments

import (
	"encoding/json"

	v20231001preview "github.com/radius-project/radius/pkg/corerp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/test/testutil"
)

const testHeaderfile = "requestheaders20231001preview.json"
const testHeaderfilegetrecipemetadata = "requestheadersgetrecipemetadata20231001preview.json"
const testHeaderfilegetrecipemetadatanotexisting = "requestheadersgetrecipemetadatanotexisting20231001preview.json"

func getTestModels20231001preview() (*v20231001preview.EnvironmentResource, *datamodel.Environment, *v20231001preview.EnvironmentResource) {
	rawInput := testutil.ReadFixture("environment20231001preview_input.json")
	envInput := &v20231001preview.EnvironmentResource{}
	_ = json.Unmarshal(rawInput, envInput)

	rawDataModel := testutil.ReadFixture("environment20231001preview_datamodel.json")
	envDataModel := &datamodel.Environment{}
	_ = json.Unmarshal(rawDataModel, envDataModel)

	rawExpectedOutput := testutil.ReadFixture("environment20231001preview_output.json")
	expectedOutput := &v20231001preview.EnvironmentResource{}
	_ = json.Unmarshal(rawExpectedOutput, expectedOutput)

	return envInput, envDataModel, expectedOutput
}

func getTestModelsGetRecipeMetadata20231001preview() (*v20231001preview.RecipeGetMetadata, *datamodel.Environment, *v20231001preview.RecipeGetMetadataResponse) {
	rawInput := testutil.ReadFixture("environmentgetrecipemetadata20231001preview_input.json")
	envInput := &v20231001preview.RecipeGetMetadata{}
	_ = json.Unmarshal(rawInput, envInput)

	rawExistingDataModel := testutil.ReadFixture("environmentgetrecipemetadata20231001preview_datamodel.json")
	envExistingDataModel := &datamodel.Environment{}
	_ = json.Unmarshal(rawExistingDataModel, envExistingDataModel)

	rawExpectedOutput := testutil.ReadFixture("environmentgetrecipemetadata20231001preview_output.json")
	expectedOutput := &v20231001preview.RecipeGetMetadataResponse{}
	_ = json.Unmarshal(rawExpectedOutput, expectedOutput)

	return envInput, envExistingDataModel, expectedOutput
}

func getTestModelsGetTFRecipeMetadata20231001preview() (*v20231001preview.RecipeGetMetadata, *datamodel.Environment, *v20231001preview.RecipeGetMetadataResponse) {
	rawInput := testutil.ReadFixture("environmentgetrecipemetadata20231001preview_input_terraform.json")
	envInput := &v20231001preview.RecipeGetMetadata{}
	_ = json.Unmarshal(rawInput, envInput)

	rawExistingDataModel := testutil.ReadFixture("environmentgetrecipemetadata20231001preview_datamodel.json")
	envExistingDataModel := &datamodel.Environment{}
	_ = json.Unmarshal(rawExistingDataModel, envExistingDataModel)

	rawExpectedOutput := testutil.ReadFixture("environmentgetrecipemetadata20231001preview_output_terraform.json")
	expectedOutput := &v20231001preview.RecipeGetMetadataResponse{}
	_ = json.Unmarshal(rawExpectedOutput, expectedOutput)

	return envInput, envExistingDataModel, expectedOutput
}

func getTestModelsGetRecipeMetadataForNonExistingRecipe20231001preview() (*v20231001preview.RecipeGetMetadata, *datamodel.Environment) {
	rawInput := testutil.ReadFixture("environmentgetmetadatanonexistingrecipe20231001preview_input.json")
	envInput := &v20231001preview.RecipeGetMetadata{}
	_ = json.Unmarshal(rawInput, envInput)

	rawExistingDataModel := testutil.ReadFixture("environmentgetrecipemetadata20231001preview_datamodel.json")
	envExistingDataModel := &datamodel.Environment{}
	_ = json.Unmarshal(rawExistingDataModel, envExistingDataModel)

	return envInput, envExistingDataModel
}
