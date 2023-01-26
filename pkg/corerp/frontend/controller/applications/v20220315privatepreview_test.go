// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package applications

import (
	"encoding/json"

	v20220315privatepreview "github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/test/testutil"
)

const testHeaderfile = "requestheaders20220315privatepreview.json"

func getTestModels20220315privatepreview() (*v20220315privatepreview.ApplicationResource, *datamodel.Application, *v20220315privatepreview.ApplicationResource) {
	rawInput := testutil.ReadFixture("application20220315privatepreview_input.json")
	appInput := &v20220315privatepreview.ApplicationResource{}
	_ = json.Unmarshal(rawInput, appInput)

	rawDataModel := testutil.ReadFixture("application20220315privatepreview_datamodel.json")
	appDataModel := &datamodel.Application{}
	_ = json.Unmarshal(rawDataModel, appDataModel)

	rawExpectedOutput := testutil.ReadFixture("application20220315privatepreview_output.json")
	expectedOutput := &v20220315privatepreview.ApplicationResource{}
	_ = json.Unmarshal(rawExpectedOutput, expectedOutput)
	return appInput, appDataModel, expectedOutput
}
