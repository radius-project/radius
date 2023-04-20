// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package environments

import (
	"encoding/json"

	v20230415preview "github.com/project-radius/radius/pkg/corerp/api/v20230415preview"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/test/testutil"
)

const testHeaderfile = "requestheaders20230415preview.json"
const testHeaderfilegetrecipemetadata = "requestheadersgetrecipemetadata20230415preview.json"
const testHeaderfilegetrecipemetadatanotexisting = "requestheadersgetrecipemetadatanotexisting20230415preview.json"

func getTestModels20230415preview() (*v20230415preview.EnvironmentResource, *datamodel.Environment, *v20230415preview.EnvironmentResource) {
	rawInput := testutil.ReadFixture("environment20230415preview_input.json")
	envInput := &v20230415preview.EnvironmentResource{}
	_ = json.Unmarshal(rawInput, envInput)

	rawDataModel := testutil.ReadFixture("environment20230415preview_datamodel.json")
	envDataModel := &datamodel.Environment{}
	_ = json.Unmarshal(rawDataModel, envDataModel)

	rawExpectedOutput := testutil.ReadFixture("environment20230415preview_output.json")
	expectedOutput := &v20230415preview.EnvironmentResource{}
	_ = json.Unmarshal(rawExpectedOutput, expectedOutput)

	return envInput, envDataModel, expectedOutput
}

func getTestModelsWithDevRecipes20230415preview() (*v20230415preview.EnvironmentResource, *datamodel.Environment, *v20230415preview.EnvironmentResource) {
	rawInput := testutil.ReadFixture("environmentwithdevrecipes20230415preview_input.json")
	envInput := &v20230415preview.EnvironmentResource{}
	_ = json.Unmarshal(rawInput, envInput)

	rawDataModel := testutil.ReadFixture("environmentwithdevrecipes20230415preview_datamodel.json")
	envDataModel := &datamodel.Environment{}
	_ = json.Unmarshal(rawDataModel, envDataModel)

	rawExpectedOutput := testutil.ReadFixture("environmentwithdevrecipes20230415preview_output.json")
	expectedOutput := &v20230415preview.EnvironmentResource{}
	_ = json.Unmarshal(rawExpectedOutput, expectedOutput)

	return envInput, envDataModel, expectedOutput
}

func getTestModelsAppendDevRecipes20230415preview() (*v20230415preview.EnvironmentResource, *datamodel.Environment, *v20230415preview.EnvironmentResource) {
	rawInput := testutil.ReadFixture("environmentappenddevrecipes20230415preview_input.json")
	envInput := &v20230415preview.EnvironmentResource{}
	_ = json.Unmarshal(rawInput, envInput)

	rawDataModel := testutil.ReadFixture("environmentappenddevrecipes20230415preview_datamodel.json")
	envDataModel := &datamodel.Environment{}
	_ = json.Unmarshal(rawDataModel, envDataModel)

	rawExpectedOutput := testutil.ReadFixture("environmentappenddevrecipes20230415preview_output.json")
	expectedOutput := &v20230415preview.EnvironmentResource{}
	_ = json.Unmarshal(rawExpectedOutput, expectedOutput)

	return envInput, envDataModel, expectedOutput
}

func getTestModelsAppendDevRecipesToExisting20230415preview() (*datamodel.Environment, *v20230415preview.EnvironmentResource, *datamodel.Environment, *v20230415preview.EnvironmentResource) {

	rawExistingDataModel := testutil.ReadFixture("environmentappenddevrecipestoexistingoriginal20230415preview_datamodel.json")
	envExistingDataModel := &datamodel.Environment{}
	_ = json.Unmarshal(rawExistingDataModel, envExistingDataModel)

	rawInput := testutil.ReadFixture("environmentappenddevrecipestoexisting20230415preview_input.json")
	envInput := &v20230415preview.EnvironmentResource{}
	_ = json.Unmarshal(rawInput, envInput)

	rawDataModel := testutil.ReadFixture("environmentappenddevrecipestoexisting20230415preview_datamodel.json")
	envDataModel := &datamodel.Environment{}
	_ = json.Unmarshal(rawDataModel, envDataModel)

	rawExpectedOutput := testutil.ReadFixture("environmentappenddevrecipestoexisting20230415preview_output.json")
	expectedOutput := &v20230415preview.EnvironmentResource{}
	_ = json.Unmarshal(rawExpectedOutput, expectedOutput)

	return envExistingDataModel, envInput, envDataModel, expectedOutput
}

func getTestModelsUserRecipesConflictWithReservedNames20230415preview() *v20230415preview.EnvironmentResource {
	rawInput := testutil.ReadFixture("environmentuserrecipesconflictwithreservednames20230415preview_input.json")
	envInput := &v20230415preview.EnvironmentResource{}
	_ = json.Unmarshal(rawInput, envInput)

	return envInput
}

func getTestModelsExistingUserRecipesConflictWithReservedNames20230415preview() (*datamodel.Environment, *v20230415preview.EnvironmentResource) {

	rawExistingDataModel := testutil.ReadFixture("environmentuserrecipesconflictwithreservednamesoriginal20230415preview_datamodel.json")
	envExistingDataModel := &datamodel.Environment{}
	_ = json.Unmarshal(rawExistingDataModel, envExistingDataModel)

	rawInput := testutil.ReadFixture("environmentuserrecipesconflictwithreservednames20230415preview_input.json")
	envInput := &v20230415preview.EnvironmentResource{}
	_ = json.Unmarshal(rawInput, envInput)

	return envExistingDataModel, envInput
}

func getTestModelsGetRecipeMetadata20230415preview() (*datamodel.Environment, *v20230415preview.EnvironmentResource) {
	rawExistingDataModel := testutil.ReadFixture("environmentgetrecipemetadata20230415preview_datamodel.json")
	envExistingDataModel := &datamodel.Environment{}
	_ = json.Unmarshal(rawExistingDataModel, envExistingDataModel)

	rawExpectedOutput := testutil.ReadFixture("environmentgetrecipemetadata20230415preview_output.json")
	expectedOutput := &v20230415preview.EnvironmentResource{}
	_ = json.Unmarshal(rawExpectedOutput, expectedOutput)

	return envExistingDataModel, expectedOutput
}
