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

	v20220315privatepreview "github.com/radius-project/radius/pkg/corerp/api/v20220315privatepreview"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/test/testutil"
)

const testHeaderfile = "requestheaders20220315privatepreview.json"
const testHeaderfilegetrecipemetadata = "requestheadersgetrecipemetadata20220315privatepreview.json"
const testHeaderfilegetrecipemetadatanotexisting = "requestheadersgetrecipemetadatanotexisting20220315privatepreview.json"

func getTestModels20220315privatepreview() (*v20220315privatepreview.EnvironmentResource, *datamodel.Environment, *v20220315privatepreview.EnvironmentResource) {
	rawInput := testutil.ReadFixture("environment20220315privatepreview_input.json")
	envInput := &v20220315privatepreview.EnvironmentResource{}
	_ = json.Unmarshal(rawInput, envInput)

	rawDataModel := testutil.ReadFixture("environment20220315privatepreview_datamodel.json")
	envDataModel := &datamodel.Environment{}
	_ = json.Unmarshal(rawDataModel, envDataModel)

	rawExpectedOutput := testutil.ReadFixture("environment20220315privatepreview_output.json")
	expectedOutput := &v20220315privatepreview.EnvironmentResource{}
	_ = json.Unmarshal(rawExpectedOutput, expectedOutput)

	return envInput, envDataModel, expectedOutput
}

func getTestModelsGetRecipeMetadata20220315privatepreview() (*v20220315privatepreview.RecipeGetMetadata, *v20220315privatepreview.RecipeGetMetadata, *datamodel.Environment, *v20220315privatepreview.RecipeGetMetadataResponse, *v20220315privatepreview.RecipeGetMetadataResponse) {
	rawInput := testutil.ReadFixture("environmentgetrecipemetadata20220315privatepreview_input.json")
	envInput := &v20220315privatepreview.RecipeGetMetadata{}
	_ = json.Unmarshal(rawInput, envInput)

	rawTFInput := testutil.ReadFixture("environmentgetrecipemetadata20220315privatepreview_input_terraform.json")
	envTFInput := &v20220315privatepreview.RecipeGetMetadata{}
	_ = json.Unmarshal(rawTFInput, envTFInput)

	rawExistingDataModel := testutil.ReadFixture("environmentgetrecipemetadata20220315privatepreview_datamodel.json")
	envExistingDataModel := &datamodel.Environment{}
	_ = json.Unmarshal(rawExistingDataModel, envExistingDataModel)

	rawExpectedOutput := testutil.ReadFixture("environmentgetrecipemetadata20220315privatepreview_output.json")
	expectedOutput := &v20220315privatepreview.RecipeGetMetadataResponse{}
	_ = json.Unmarshal(rawExpectedOutput, expectedOutput)

	rawExpectedTFOutput := testutil.ReadFixture("environmentgetrecipemetadata20220315privatepreview_output_terraform.json")
	expectedTFOutput := &v20220315privatepreview.RecipeGetMetadataResponse{}
	_ = json.Unmarshal(rawExpectedTFOutput, expectedTFOutput)

	return envInput, envTFInput, envExistingDataModel, expectedOutput, expectedTFOutput
}

func getTestModelsGetRecipeMetadataForNonExistingRecipe20220315privatepreview() (*v20220315privatepreview.RecipeGetMetadata, *datamodel.Environment) {
	rawInput := testutil.ReadFixture("environmentgetmetadatanonexistingrecipe20220315privatepreview_input.json")
	envInput := &v20220315privatepreview.RecipeGetMetadata{}
	_ = json.Unmarshal(rawInput, envInput)

	rawExistingDataModel := testutil.ReadFixture("environmentgetrecipemetadata20220315privatepreview_datamodel.json")
	envExistingDataModel := &datamodel.Environment{}
	_ = json.Unmarshal(rawExistingDataModel, envExistingDataModel)

	return envInput, envExistingDataModel
}
