// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package httproutes

import (
	"encoding/json"

	"github.com/project-radius/radius/pkg/corerp/api/v20230415preview"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/test/testutil"
)

const testHeaderfile = "requestheaders20230415preview.json"

func getTestModels20230415preview() (*v20230415preview.HTTPRouteResource, *datamodel.HTTPRoute, *v20230415preview.HTTPRouteResource) {
	rawInput := testutil.ReadFixture("httproute20230415preview_input.json")
	hrtInput := &v20230415preview.HTTPRouteResource{}
	_ = json.Unmarshal(rawInput, hrtInput)

	rawDataModel := testutil.ReadFixture("httproute20230415preview_datamodel.json")
	hrtDataModel := &datamodel.HTTPRoute{}
	_ = json.Unmarshal(rawDataModel, hrtDataModel)

	rawExpectedOutput := testutil.ReadFixture("httproute20230415preview_output.json")
	expectedOutput := &v20230415preview.HTTPRouteResource{}
	_ = json.Unmarshal(rawExpectedOutput, expectedOutput)

	return hrtInput, hrtDataModel, expectedOutput
}
