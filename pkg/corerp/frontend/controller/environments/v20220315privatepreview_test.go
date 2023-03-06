// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package environments

import (
	"encoding/json"

	v20220315privatepreview "github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/test/testutil"
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

func getTestModelsWithDevRecipes20220315privatepreview() (*v20220315privatepreview.EnvironmentResource, *datamodel.Environment, *v20220315privatepreview.EnvironmentResource) {
	rawInput := testutil.ReadFixture("environmentwithdevrecipes20220315privatepreview_input.json")
	envInput := &v20220315privatepreview.EnvironmentResource{}
	_ = json.Unmarshal(rawInput, envInput)

	rawDataModel := testutil.ReadFixture("environmentwithdevrecipes20220315privatepreview_datamodel.json")
	envDataModel := &datamodel.Environment{}
	_ = json.Unmarshal(rawDataModel, envDataModel)

	rawExpectedOutput := testutil.ReadFixture("environmentwithdevrecipes20220315privatepreview_output.json")
	expectedOutput := &v20220315privatepreview.EnvironmentResource{}
	_ = json.Unmarshal(rawExpectedOutput, expectedOutput)

	return envInput, envDataModel, expectedOutput
}

func getTestModelsAppendDevRecipes20220315privatepreview() (*v20220315privatepreview.EnvironmentResource, *datamodel.Environment, *v20220315privatepreview.EnvironmentResource) {
	rawInput := testutil.ReadFixture("environmentappenddevrecipes20220315privatepreview_input.json")
	envInput := &v20220315privatepreview.EnvironmentResource{}
	_ = json.Unmarshal(rawInput, envInput)

	rawDataModel := testutil.ReadFixture("environmentappenddevrecipes20220315privatepreview_datamodel.json")
	envDataModel := &datamodel.Environment{}
	_ = json.Unmarshal(rawDataModel, envDataModel)

	rawExpectedOutput := testutil.ReadFixture("environmentappenddevrecipes20220315privatepreview_output.json")
	expectedOutput := &v20220315privatepreview.EnvironmentResource{}
	_ = json.Unmarshal(rawExpectedOutput, expectedOutput)

	return envInput, envDataModel, expectedOutput
}

func getTestModelsAppendDevRecipesToExisting20220315privatepreview() (*datamodel.Environment, *v20220315privatepreview.EnvironmentResource, *datamodel.Environment, *v20220315privatepreview.EnvironmentResource) {

	rawExistingDataModel := testutil.ReadFixture("environmentappenddevrecipestoexistingoriginal20220315privatepreview_datamodel.json")
	envExistingDataModel := &datamodel.Environment{}
	_ = json.Unmarshal(rawExistingDataModel, envExistingDataModel)

	rawInput := testutil.ReadFixture("environmentappenddevrecipestoexisting20220315privatepreview_input.json")
	envInput := &v20220315privatepreview.EnvironmentResource{}
	_ = json.Unmarshal(rawInput, envInput)

	rawDataModel := testutil.ReadFixture("environmentappenddevrecipestoexisting20220315privatepreview_datamodel.json")
	envDataModel := &datamodel.Environment{}
	_ = json.Unmarshal(rawDataModel, envDataModel)

	rawExpectedOutput := testutil.ReadFixture("environmentappenddevrecipestoexisting20220315privatepreview_output.json")
	expectedOutput := &v20220315privatepreview.EnvironmentResource{}
	_ = json.Unmarshal(rawExpectedOutput, expectedOutput)

	return envExistingDataModel, envInput, envDataModel, expectedOutput
}

func getTestModelsUserRecipesConflictWithReservedNames20220315privatepreview() *v20220315privatepreview.EnvironmentResource {
	rawInput := testutil.ReadFixture("environmentuserrecipesconflictwithreservednames20220315privatepreview_input.json")
	envInput := &v20220315privatepreview.EnvironmentResource{}
	_ = json.Unmarshal(rawInput, envInput)

	return envInput
}

func getTestModelsExistingUserRecipesConflictWithReservedNames20220315privatepreview() (*datamodel.Environment, *v20220315privatepreview.EnvironmentResource) {

	rawExistingDataModel := testutil.ReadFixture("environmentuserrecipesconflictwithreservednamesoriginal20220315privatepreview_datamodel.json")
	envExistingDataModel := &datamodel.Environment{}
	_ = json.Unmarshal(rawExistingDataModel, envExistingDataModel)

	rawInput := testutil.ReadFixture("environmentuserrecipesconflictwithreservednames20220315privatepreview_input.json")
	envInput := &v20220315privatepreview.EnvironmentResource{}
	_ = json.Unmarshal(rawInput, envInput)

	return envExistingDataModel, envInput
}

func getTestModelsGetRecipeMetadata20220315privatepreview() (*datamodel.Environment, *v20220315privatepreview.EnvironmentResource) {
	rawExistingDataModel := testutil.ReadFixture("environmentgetrecipemetadata20220315privatepreview_datamodel.json")
	envExistingDataModel := &datamodel.Environment{}
	_ = json.Unmarshal(rawExistingDataModel, envExistingDataModel)

	rawExpectedOutput := testutil.ReadFixture("environmentgetrecipemetadata20220315privatepreview_output.json")
	expectedOutput := &v20220315privatepreview.EnvironmentResource{}
	_ = json.Unmarshal(rawExpectedOutput, expectedOutput)

	return envExistingDataModel, expectedOutput
}
