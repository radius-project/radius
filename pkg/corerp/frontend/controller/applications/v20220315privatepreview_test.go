// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package applications

import (
	"encoding/json"

	v20220315privatepreview "github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	radiustesting "github.com/project-radius/radius/pkg/corerp/testing"
)

const testHeaderfile = "requestheaders20220315privatepreview.json"

func getTestModels20220315privatepreview() (*v20220315privatepreview.ApplicationResource, *datamodel.Application, *v20220315privatepreview.ApplicationResource) {
	rawInput := radiustesting.ReadFixture("application20220315privatepreview_input.json")
	appInput := &v20220315privatepreview.ApplicationResource{}
	_ = json.Unmarshal(rawInput, appInput)

	rawDataModel := radiustesting.ReadFixture("application20220315privatepreview_datamodel.json")
	appDataModel := &datamodel.Application{}
	_ = json.Unmarshal(rawDataModel, appDataModel)

	rawExpectedOutput := radiustesting.ReadFixture("application20220315privatepreview_output.json")
	expectedOutput := &v20220315privatepreview.ApplicationResource{}
	_ = json.Unmarshal(rawExpectedOutput, expectedOutput)
	return appInput, appDataModel, expectedOutput
}
