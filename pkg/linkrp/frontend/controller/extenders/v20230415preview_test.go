// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package extenders

import (
	"encoding/json"

	"github.com/project-radius/radius/pkg/linkrp/api/v20230415preview"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/test/testutil"
)

const testHeaderfile = "20230415preview_requestheaders.json"

func getTestModels20230415preview() (input *v20230415preview.ExtenderResource, dataModel *datamodel.Extender, output *v20230415preview.ExtenderResource) {
	rawInput := testutil.ReadFixture("20230415preview_input.json")
	input = &v20230415preview.ExtenderResource{}
	_ = json.Unmarshal(rawInput, input)

	rawDataModel := testutil.ReadFixture("20230415preview_datamodel.json")
	dataModel = &datamodel.Extender{}
	_ = json.Unmarshal(rawDataModel, dataModel)

	rawExpectedOutput := testutil.ReadFixture("20230415preview_output.json")
	output = &v20230415preview.ExtenderResource{}
	_ = json.Unmarshal(rawExpectedOutput, output)

	return input, dataModel, output
}

func getTestModelsForGetAndListApis20230415preview() (input *v20230415preview.ExtenderResource, dataModel *datamodel.Extender, output *v20230415preview.ExtenderResponseResource) {
	rawInput := testutil.ReadFixture("20230415preview_input.json")
	input = &v20230415preview.ExtenderResource{}
	_ = json.Unmarshal(rawInput, input)

	rawDataModel := testutil.ReadFixture("20230415preview_datamodel.json")
	dataModel = &datamodel.Extender{}
	_ = json.Unmarshal(rawDataModel, dataModel)

	rawExpectedOutput := testutil.ReadFixture("20230415previewgetandlist_output.json")
	output = &v20230415preview.ExtenderResponseResource{}
	_ = json.Unmarshal(rawExpectedOutput, output)

	return input, dataModel, output
}
