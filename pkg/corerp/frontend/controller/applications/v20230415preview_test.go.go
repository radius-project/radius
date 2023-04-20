// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package applications

import (
	"encoding/json"

	v20230415preview "github.com/project-radius/radius/pkg/corerp/api/v20230415preview"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/test/testutil"
)

const testHeaderfile = "requestheaders20230415preview.json"

func getTestModels20230415preview() (*v20230415preview.ApplicationResource, *datamodel.Application, *v20230415preview.ApplicationResource) {
	rawInput := testutil.ReadFixture("application20230415preview_input.json")
	appInput := &v20230415preview.ApplicationResource{}
	_ = json.Unmarshal(rawInput, appInput)

	rawDataModel := testutil.ReadFixture("application20230415preview_datamodel.json")
	appDataModel := &datamodel.Application{}
	_ = json.Unmarshal(rawDataModel, appDataModel)

	rawExpectedOutput := testutil.ReadFixture("application20230415preview_output.json")
	expectedOutput := &v20230415preview.ApplicationResource{}
	_ = json.Unmarshal(rawExpectedOutput, expectedOutput)
	return appInput, appDataModel, expectedOutput
}
