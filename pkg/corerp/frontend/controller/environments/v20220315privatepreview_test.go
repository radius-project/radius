// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package environments

import (
	"encoding/json"

	v20220315privatepreview "github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	radiustesting "github.com/project-radius/radius/pkg/corerp/testing"
)

const testHeaderfile = "requestheaders20220315privatepreview.json"

func getTestModels20220315privatepreview() (*v20220315privatepreview.EnvironmentResource, *datamodel.Environment, *v20220315privatepreview.EnvironmentResource) {
	rawInput := radiustesting.ReadFixture("environment20220315privatepreview_input.json")
	envInput := &v20220315privatepreview.EnvironmentResource{}
	_ = json.Unmarshal(rawInput, envInput)

	rawDataModel := radiustesting.ReadFixture("environment20220315privatepreview_datamodel.json")
	envDataModel := &datamodel.Environment{}
	_ = json.Unmarshal(rawDataModel, envDataModel)

	rawExpectedOutput := radiustesting.ReadFixture("environment20220315privatepreview_output.json")
	expectedOutput := &v20220315privatepreview.EnvironmentResource{}
	_ = json.Unmarshal(rawExpectedOutput, expectedOutput)

	return envInput, envDataModel, expectedOutput
}

func getTestModelsWithDevRecipes20220315privatepreview() (*v20220315privatepreview.EnvironmentResource, *datamodel.Environment, *v20220315privatepreview.EnvironmentResource) {
	rawInput := radiustesting.ReadFixture("environmentwithdevrecipes20220315privatepreview_input.json")
	envInput := &v20220315privatepreview.EnvironmentResource{}
	_ = json.Unmarshal(rawInput, envInput)

	rawDataModel := radiustesting.ReadFixture("environmentwithdevrecipes20220315privatepreview_datamodel.json")
	envDataModel := &datamodel.Environment{}
	_ = json.Unmarshal(rawDataModel, envDataModel)

	rawExpectedOutput := radiustesting.ReadFixture("environmentwithdevrecipes20220315privatepreview_output.json")
	expectedOutput := &v20220315privatepreview.EnvironmentResource{}
	_ = json.Unmarshal(rawExpectedOutput, expectedOutput)

	return envInput, envDataModel, expectedOutput
}

func getTestModelsAppendDevRecipes20220315privatepreview() (*v20220315privatepreview.EnvironmentResource, *datamodel.Environment, *v20220315privatepreview.EnvironmentResource) {
	rawInput := radiustesting.ReadFixture("environmentappenddevrecipes20220315privatepreview_input.json")
	envInput := &v20220315privatepreview.EnvironmentResource{}
	_ = json.Unmarshal(rawInput, envInput)

	rawDataModel := radiustesting.ReadFixture("environmentappenddevrecipes20220315privatepreview_datamodel.json")
	envDataModel := &datamodel.Environment{}
	_ = json.Unmarshal(rawDataModel, envDataModel)

	rawExpectedOutput := radiustesting.ReadFixture("environmentappenddevrecipes20220315privatepreview_output.json")
	expectedOutput := &v20220315privatepreview.EnvironmentResource{}
	_ = json.Unmarshal(rawExpectedOutput, expectedOutput)

	return envInput, envDataModel, expectedOutput
}

func getTestModelsAppendDevRecipesToExisting20220315privatepreview() (*datamodel.Environment, *v20220315privatepreview.EnvironmentResource, *datamodel.Environment, *v20220315privatepreview.EnvironmentResource) {

	rawExistingDataModel := radiustesting.ReadFixture("environmentappenddevrecipestoexistingoriginal20220315privatepreview_datamodel.json")
	envExistingDataModel := &datamodel.Environment{}
	_ = json.Unmarshal(rawExistingDataModel, envExistingDataModel)

	rawInput := radiustesting.ReadFixture("environmentappenddevrecipestoexisting20220315privatepreview_input.json")
	envInput := &v20220315privatepreview.EnvironmentResource{}
	_ = json.Unmarshal(rawInput, envInput)

	rawDataModel := radiustesting.ReadFixture("environmentappenddevrecipestoexisting20220315privatepreview_datamodel.json")
	envDataModel := &datamodel.Environment{}
	_ = json.Unmarshal(rawDataModel, envDataModel)

	rawExpectedOutput := radiustesting.ReadFixture("environmentappenddevrecipestoexisting20220315privatepreview_output.json")
	expectedOutput := &v20220315privatepreview.EnvironmentResource{}
	_ = json.Unmarshal(rawExpectedOutput, expectedOutput)

	return envExistingDataModel, envInput, envDataModel, expectedOutput
}
