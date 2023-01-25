// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package extenders

import (
	"encoding/json"

	"github.com/project-radius/radius/pkg/linkrp/api/v20220315privatepreview"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/test/testutil"
)

const testHeaderfile = "20220315privatepreview_requestheaders.json"

func getTestModels20220315privatepreview() (input *v20220315privatepreview.ExtenderResource, dataModel *datamodel.Extender, output *v20220315privatepreview.ExtenderResource) {
	rawInput := testutil.ReadFixture("20220315privatepreview_input.json")
	input = &v20220315privatepreview.ExtenderResource{}
	_ = json.Unmarshal(rawInput, input)

	rawDataModel := testutil.ReadFixture("20220315privatepreview_datamodel.json")
	dataModel = &datamodel.Extender{}
	_ = json.Unmarshal(rawDataModel, dataModel)

	rawExpectedOutput := testutil.ReadFixture("20220315privatepreview_output.json")
	output = &v20220315privatepreview.ExtenderResource{}
	_ = json.Unmarshal(rawExpectedOutput, output)

	return input, dataModel, output
}

func getTestModelsForGetAndListApis20220315privatepreview() (input *v20220315privatepreview.ExtenderResource, dataModel *datamodel.Extender, output *v20220315privatepreview.ExtenderResponseResource) {
	rawInput := testutil.ReadFixture("20220315privatepreview_input.json")
	input = &v20220315privatepreview.ExtenderResource{}
	_ = json.Unmarshal(rawInput, input)

	rawDataModel := testutil.ReadFixture("20220315privatepreview_datamodel.json")
	dataModel = &datamodel.Extender{}
	_ = json.Unmarshal(rawDataModel, dataModel)

	rawExpectedOutput := testutil.ReadFixture("20220315privatepreviewgetandlist_output.json")
	output = &v20220315privatepreview.ExtenderResponseResource{}
	_ = json.Unmarshal(rawExpectedOutput, output)

	return input, dataModel, output
}
